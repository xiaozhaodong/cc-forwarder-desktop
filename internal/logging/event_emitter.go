package logging

import (
	"context"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// EventEmitter Wails 事件发射器
// 负责将日志通过 Wails Runtime Events 推送到前端
type EventEmitter struct {
	ctx       context.Context
	enabled   bool
	batchSize int          // 批量发送大小
	buffer    []LogEntry   // 缓冲区
	ticker    *time.Ticker // 定时器
	mu        sync.Mutex
	stopChan  chan struct{}
	stopped   bool // 防止重复关闭
}

// NewEventEmitter 创建事件发射器
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		batchSize: 10, // 每批最多10条
		buffer:    make([]LogEntry, 0, 10),
		stopChan:  make(chan struct{}),
	}
}

// Start 启动事件发射器（前端订阅后调用）
func (e *EventEmitter) Start(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.enabled {
		return // 已启动
	}

	e.ctx = ctx
	e.enabled = true
	e.stopped = false                                 // 重置停止标志
	e.stopChan = make(chan struct{})                  // 重新创建 channel
	e.ticker = time.NewTicker(100 * time.Millisecond) // 100ms刷新一次

	// 启动批量发送goroutine
	go e.batchSendLoop()
}

// Stop 停止事件发射器
func (e *EventEmitter) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled || e.stopped {
		return // 已经停止，避免重复关闭
	}

	e.enabled = false
	e.stopped = true

	if e.ticker != nil {
		e.ticker.Stop()
	}

	close(e.stopChan)

	// 刷新剩余日志
	e.flushLocked()
}

// Emit 发射一条日志事件
func (e *EventEmitter) Emit(entry LogEntry) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.enabled {
		return
	}

	// 添加到缓冲区
	e.buffer = append(e.buffer, entry)

	// 如果缓冲区满了，立即发送
	if len(e.buffer) >= e.batchSize {
		e.flushLocked()
	}
}

// batchSendLoop 批量发送循环
func (e *EventEmitter) batchSendLoop() {
	for {
		select {
		case <-e.ticker.C:
			e.mu.Lock()
			e.flushLocked()
			e.mu.Unlock()
		case <-e.stopChan:
			return
		}
	}
}

// flushLocked 刷新缓冲区（需要持有锁）
func (e *EventEmitter) flushLocked() {
	if len(e.buffer) == 0 {
		return
	}

	// 批量发送
	if e.ctx != nil {
		runtime.EventsEmit(e.ctx, "log:batch", e.buffer)
	}

	// 清空缓冲区
	e.buffer = e.buffer[:0]
}

// IsEnabled 返回是否已启用
func (e *EventEmitter) IsEnabled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.enabled
}
