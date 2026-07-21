package endpoint

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cc-forwarder/internal/store"
)

// RuntimeWriter 串行协调 activeEndpoint 的全部持久化写入（方案 §4.3）。
// 覆盖手动激活、自动迁移、端点页启停与端点删除；单 goroutine 消费队列，
// 显式维护 lastPersisted 状态作为手动失败回滚基准与过期判定依据。
//
// 三项契约（第五轮评审开工门禁，不得简化）：
//  1. lastPersistedEndpoint / lastPersistedRevision 为回滚基准；
//  2. 自动任务失败按周期重入队重试，直至成功或出现更新 revision；
//  3. 过期任务分流——自动任务静默丢弃，等待 ACK 的手动任务返回 ErrSuperseded。

// ErrSuperseded 手动任务在执行前被更新的 revision 取代
var ErrSuperseded = errors.New("active endpoint change superseded by a newer revision")

// ErrRuntimeWriterClosed writer 已关闭，不再接受任务
var ErrRuntimeWriterClosed = errors.New("endpoint runtime writer is closed")

const (
	endpointWriterQueueCapacity  = 64
	endpointWriterRetryInterval  = 200 * time.Millisecond
	endpointWriterExecuteTimeout = 5 * time.Second
)

type endpointTaskKind int

const (
	// endpointTaskActivate 单事务「禁用其余 + 启用目标」
	endpointTaskActivate endpointTaskKind = iota
	// endpointTaskDisable 停用单个端点（端点页停用 active 路径）
	endpointTaskDisable
	// endpointTaskDelete 删除端点行（删除协调，v7）
	endpointTaskDelete
)

type endpointRuntimeTask struct {
	kind     endpointTaskKind
	name     string
	revision int64 // activate/disable 关联的 activeRevision；delete 不参与过期判定（为 0）
	manual   bool
	ack      chan error // manual 时非 nil（buffered 1）
}

// RuntimeWriter activeEndpoint 持久化协调器
type RuntimeWriter struct {
	store store.EndpointStore

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	tasks chan endpointRuntimeTask

	lifecycleMu sync.RWMutex // 协调 submit 与 Close：通过门禁的任务必须先完成入队再允许取消消费者
	closing     bool

	mu                    sync.Mutex
	retryTask             *endpointRuntimeTask
	lastPersistedEndpoint string
	lastPersistedRevision int64
	lastEstablished       bool

	isCurrentRevision func(revision int64) bool
}

// NewRuntimeWriter 创建并启动 writer。st 为 nil 时返回 nil（非 SQLite 模式，仅内存生效）。
func NewRuntimeWriter(st store.EndpointStore, isCurrentRevision func(int64) bool) *RuntimeWriter {
	if st == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &RuntimeWriter{
		store:             st,
		ctx:               ctx,
		cancel:            cancel,
		tasks:             make(chan endpointRuntimeTask, endpointWriterQueueCapacity),
		isCurrentRevision: isCurrentRevision,
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// InitializeLastPersisted 启动恢复后初始化 persisted 基准（§4.6）。
// 多 enabled 脏数据修复成功前不得调用（lastPersisted 保持未确立）。
func (w *RuntimeWriter) InitializeLastPersisted(name string, revision int64) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastPersistedEndpoint = name
	w.lastPersistedRevision = revision
	w.lastEstablished = true
}

// LastPersisted 返回最后确认落库状态。established=false 表示基准未确立（degraded）。
func (w *RuntimeWriter) LastPersisted() (name string, revision int64, established bool) {
	if w == nil {
		return "", 0, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastPersistedEndpoint, w.lastPersistedRevision, w.lastEstablished
}

// EnqueueManual 入队手动任务并阻塞等待 ACK。
// 返回 nil ⇒ 已落库；ErrSuperseded ⇒ 被更新变更取代；其他错误 ⇒ 落库失败（回滚由调用方执行）。
func (w *RuntimeWriter) EnqueueManual(kind endpointTaskKind, name string, revision int64) error {
	if w == nil {
		return nil
	}
	task := endpointRuntimeTask{
		kind:     kind,
		name:     name,
		revision: revision,
		manual:   true,
		ack:      make(chan error, 1),
	}
	if err := w.submit(task); err != nil {
		return err
	}
	return <-task.ack
}

// EnqueueAuto 入队自动任务（fire-and-forget；失败由重试机制兜底）
func (w *RuntimeWriter) EnqueueAuto(kind endpointTaskKind, name string, revision int64) {
	if w == nil {
		return
	}
	task := endpointRuntimeTask{kind: kind, name: name, revision: revision}
	if err := w.submit(task); err != nil {
		slog.Warn("端点运行时写入队失败", "endpoint", name, "revision", revision, "error", err)
	}
}

func (w *RuntimeWriter) submit(task endpointRuntimeTask) error {
	w.lifecycleMu.RLock()
	defer w.lifecycleMu.RUnlock()
	if w.closing {
		return ErrRuntimeWriterClosed
	}

	select {
	case w.tasks <- task:
		return nil
	case <-w.ctx.Done():
		return ErrRuntimeWriterClosed
	}
}

// Close 停止 writer 并 flush 所有排队任务（对照账号池 writer 的 Close flush 语义）
func (w *RuntimeWriter) Close() error {
	if w == nil {
		return nil
	}
	w.lifecycleMu.Lock()
	if w.closing {
		w.lifecycleMu.Unlock()
		w.wg.Wait()
		return nil
	}
	w.closing = true
	w.cancel()
	w.lifecycleMu.Unlock()
	w.wg.Wait()
	return nil
}

func (w *RuntimeWriter) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(endpointWriterRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case task := <-w.tasks:
			w.execute(task)
		case <-ticker.C:
			w.retryPendingAuto()
		case <-w.ctx.Done():
			w.drainAndFlush()
			return
		}
	}
}

// drainAndFlush 关闭前处理完所有已入队任务与待重试任务
func (w *RuntimeWriter) drainAndFlush() {
	for {
		select {
		case task := <-w.tasks:
			w.execute(task)
		default:
			w.retryPendingAuto()
			return
		}
	}
}

func (w *RuntimeWriter) retryPendingAuto() {
	w.mu.Lock()
	task := w.retryTask
	w.retryTask = nil
	w.mu.Unlock()
	if task == nil {
		return
	}
	w.execute(*task)
}

func (w *RuntimeWriter) execute(task endpointRuntimeTask) {
	// 过期任务分流：自动静默丢弃，手动 ACK ErrSuperseded
	if task.kind != endpointTaskDelete && w.isCurrentRevision != nil && !w.isCurrentRevision(task.revision) {
		if task.manual {
			task.ack <- ErrSuperseded
		} else {
			slog.Debug("端点运行时写任务已过期，静默丢弃", "endpoint", task.name, "revision", task.revision)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), endpointWriterExecuteTimeout)
	err := w.apply(ctx, task)
	cancel()

	if err == nil {
		w.recordPersisted(task)
		if task.manual {
			task.ack <- nil
		}
		return
	}

	if task.manual {
		task.ack <- err
		return
	}

	// 自动任务失败重入队（v6 修订）：revision 仍为 current 时按周期重试，
	// 不依赖"下一次 FullSuccess"——active 不变时后续成功不会再触发迁移。
	slog.Warn("端点运行时写回失败，稍后重试", "endpoint", task.name, "revision", task.revision, "error", err)
	w.mu.Lock()
	w.retryTask = &task
	w.mu.Unlock()
}

func (w *RuntimeWriter) apply(ctx context.Context, task endpointRuntimeTask) error {
	switch task.kind {
	case endpointTaskActivate:
		return w.store.ActivateExclusive(ctx, task.name)
	case endpointTaskDisable:
		return w.store.SetEnabled(ctx, task.name, false)
	case endpointTaskDelete:
		return w.store.Delete(ctx, task.name)
	default:
		return fmt.Errorf("unknown endpoint runtime task kind: %d", task.kind)
	}
}

func (w *RuntimeWriter) recordPersisted(task endpointRuntimeTask) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch task.kind {
	case endpointTaskActivate:
		w.lastPersistedEndpoint = task.name
		w.lastPersistedRevision = task.revision
		w.lastEstablished = true
	case endpointTaskDisable:
		// 停用 active 后 DB 无 enabled 行：persisted 状态为「无 active」
		w.lastPersistedEndpoint = ""
		w.lastPersistedRevision = task.revision
		w.lastEstablished = true
	case endpointTaskDelete:
		// v7：被删端点若正是 lastPersisted 目标，基准更新为空，
		// 防止后续手动失败回滚到不存在的端点。
		if w.lastPersistedEndpoint == task.name {
			w.lastPersistedEndpoint = ""
		}
	}
}
