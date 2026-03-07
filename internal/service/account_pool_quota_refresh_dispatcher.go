package service

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultAccountPoolQuotaRefreshSuccessCooldown = 30 * time.Second
	defaultAccountPoolQuotaRefreshFailureCooldown = 5 * time.Minute
	defaultAccountPoolQuotaRefreshTimeout         = 20 * time.Second
	defaultAccountPoolQuotaRefreshQueueSize       = 128
)

type accountPoolQuotaRefreshState struct {
	queued      bool
	inFlight    bool
	lastSuccess time.Time
	lastFailure time.Time
}

type accountPoolQuotaRefreshDispatcher struct {
	ctx       context.Context
	cancel    context.CancelFunc
	refreshFn func(context.Context, int64) (AccountProfileRefreshResult, error)
	now       func() time.Time
	queue     chan int64

	mu     sync.Mutex
	states map[int64]*accountPoolQuotaRefreshState
	closed bool
	wg     sync.WaitGroup
}

func newAccountPoolQuotaRefreshDispatcher(parent context.Context, refreshFn func(context.Context, int64) (AccountProfileRefreshResult, error)) *accountPoolQuotaRefreshDispatcher {
	if refreshFn == nil {
		return nil
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	d := &accountPoolQuotaRefreshDispatcher{
		ctx:       ctx,
		cancel:    cancel,
		refreshFn: refreshFn,
		now:       time.Now,
		queue:     make(chan int64, defaultAccountPoolQuotaRefreshQueueSize),
		states:    make(map[int64]*accountPoolQuotaRefreshState),
	}
	d.wg.Add(1)
	go d.run()
	return d
}

func (d *accountPoolQuotaRefreshDispatcher) Close() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	d.mu.Unlock()
	d.cancel()
	d.wg.Wait()
	return nil
}

func (d *accountPoolQuotaRefreshDispatcher) TryEnqueue(accountID int64) bool {
	if d == nil || accountID <= 0 {
		return false
	}

	now := d.now()
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		slog.Debug("账号池 quota 异步刷新跳过：dispatcher 已关闭", "account_id", accountID)
		return false
	}
	d.pruneExpiredStatesLocked(now)
	state := d.stateForAccount(accountID)
	if state.queued || state.inFlight {
		d.mu.Unlock()
		slog.Debug("账号池 quota 异步刷新跳过：账号已排队或执行中", "account_id", accountID)
		return false
	}
	if !state.lastSuccess.IsZero() && now.Sub(state.lastSuccess) < defaultAccountPoolQuotaRefreshSuccessCooldown {
		d.mu.Unlock()
		slog.Debug("账号池 quota 异步刷新跳过：命中成功冷却", "account_id", accountID)
		return false
	}
	if !state.lastFailure.IsZero() && now.Sub(state.lastFailure) < defaultAccountPoolQuotaRefreshFailureCooldown {
		d.mu.Unlock()
		slog.Debug("账号池 quota 异步刷新跳过：命中失败冷却", "account_id", accountID)
		return false
	}
	state.queued = true
	d.mu.Unlock()

	select {
	case d.queue <- accountID:
		return true
	case <-d.ctx.Done():
		d.mu.Lock()
		state := d.stateForAccount(accountID)
		state.queued = false
		d.mu.Unlock()
		slog.Debug("账号池 quota 异步刷新跳过：dispatcher 上下文已取消", "account_id", accountID)
		return false
	default:
		d.mu.Lock()
		state := d.stateForAccount(accountID)
		state.queued = false
		d.mu.Unlock()
		slog.Warn("账号池 quota 异步刷新投递失败：队列已满", "account_id", accountID)
		return false
	}
}

func (d *accountPoolQuotaRefreshDispatcher) run() {
	defer d.wg.Done()
	for {
		select {
		case <-d.ctx.Done():
			return
		case accountID := <-d.queue:
			d.markDequeued(accountID)
			d.refreshAccount(accountID)
		}
	}
}

func (d *accountPoolQuotaRefreshDispatcher) refreshAccount(accountID int64) {
	ctx, cancel := context.WithTimeout(d.ctx, defaultAccountPoolQuotaRefreshTimeout)
	defer cancel()

	result, err := d.refreshFn(ctx, accountID)
	now := d.now()

	d.mu.Lock()
	state := d.stateForAccount(accountID)
	state.inFlight = false
	if err == nil && result.Success {
		state.lastSuccess = now
		state.lastFailure = time.Time{}
	} else {
		state.lastFailure = now
	}
	d.mu.Unlock()

	if err != nil {
		slog.Debug("账号池 quota 异步刷新失败", "account_id", accountID, "error", err)
		return
	}
	if !result.Success {
		slog.Debug("账号池 quota 异步刷新未成功", "account_id", accountID, "message", result.Message, "quota_status", result.QuotaStatus)
	}
}

func (d *accountPoolQuotaRefreshDispatcher) markDequeued(accountID int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := d.stateForAccount(accountID)
	state.queued = false
	state.inFlight = true
}

func (d *accountPoolQuotaRefreshDispatcher) pruneExpiredStatesLocked(now time.Time) {
	for accountID, state := range d.states {
		if state == nil {
			delete(d.states, accountID)
			continue
		}
		if state.queued || state.inFlight {
			continue
		}
		successExpired := state.lastSuccess.IsZero() || now.Sub(state.lastSuccess) >= defaultAccountPoolQuotaRefreshSuccessCooldown
		failureExpired := state.lastFailure.IsZero() || now.Sub(state.lastFailure) >= defaultAccountPoolQuotaRefreshFailureCooldown
		if successExpired && failureExpired {
			delete(d.states, accountID)
		}
	}
}

func (d *accountPoolQuotaRefreshDispatcher) stateForAccount(accountID int64) *accountPoolQuotaRefreshState {
	state := d.states[accountID]
	if state == nil {
		state = &accountPoolQuotaRefreshState{}
		d.states[accountID] = state
	}
	return state
}
