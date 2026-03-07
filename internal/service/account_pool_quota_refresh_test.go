package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"cc-forwarder/internal/store"
)

func TestAccountPoolQuotaRefreshDispatcher_DeduplicatesInFlightAccount(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	dispatcher := newAccountPoolQuotaRefreshDispatcher(context.Background(), func(ctx context.Context, id int64) (AccountProfileRefreshResult, error) {
		started <- struct{}{}
		<-release
		return AccountProfileRefreshResult{Success: true}, nil
	})
	if dispatcher == nil {
		t.Fatal("expected dispatcher")
	}
	defer dispatcher.Close()

	if ok := dispatcher.TryEnqueue(1); !ok {
		t.Fatal("expected first enqueue to succeed")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("expected refresh to start")
	}

	if ok := dispatcher.TryEnqueue(1); ok {
		t.Fatal("expected duplicate in-flight enqueue to be rejected")
	}
	close(release)
}

func TestAccountPoolQuotaRefreshDispatcher_RespectsSuccessCooldown(t *testing.T) {
	var current atomic.Int64
	base := time.Now()
	current.Store(base.UnixNano())
	finished := make(chan struct{}, 2)
	dispatcher := newAccountPoolQuotaRefreshDispatcher(context.Background(), func(ctx context.Context, id int64) (AccountProfileRefreshResult, error) {
		finished <- struct{}{}
		return AccountProfileRefreshResult{Success: true}, nil
	})
	if dispatcher == nil {
		t.Fatal("expected dispatcher")
	}
	defer dispatcher.Close()
	dispatcher.now = func() time.Time { return time.Unix(0, current.Load()) }

	if ok := dispatcher.TryEnqueue(1); !ok {
		t.Fatal("expected first enqueue to succeed")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("expected first refresh to finish")
	}

	if ok := dispatcher.TryEnqueue(1); ok {
		t.Fatal("expected enqueue inside success cooldown to be rejected")
	}

	current.Store(base.Add(defaultAccountPoolQuotaRefreshSuccessCooldown + time.Second).UnixNano())
	if ok := dispatcher.TryEnqueue(1); !ok {
		t.Fatal("expected enqueue after success cooldown to succeed")
	}
}

func TestAccountPoolQuotaRefreshDispatcher_RespectsFailureCooldown(t *testing.T) {
	var current atomic.Int64
	base := time.Now()
	current.Store(base.UnixNano())
	finished := make(chan struct{}, 2)
	dispatcher := newAccountPoolQuotaRefreshDispatcher(context.Background(), func(ctx context.Context, id int64) (AccountProfileRefreshResult, error) {
		finished <- struct{}{}
		return AccountProfileRefreshResult{Success: false, Message: "refresh failed"}, nil
	})
	if dispatcher == nil {
		t.Fatal("expected dispatcher")
	}
	defer dispatcher.Close()
	dispatcher.now = func() time.Time { return time.Unix(0, current.Load()) }

	if ok := dispatcher.TryEnqueue(1); !ok {
		t.Fatal("expected first enqueue to succeed")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("expected first refresh to finish")
	}

	if ok := dispatcher.TryEnqueue(1); ok {
		t.Fatal("expected enqueue inside failure cooldown to be rejected")
	}

	current.Store(base.Add(defaultAccountPoolQuotaRefreshFailureCooldown + time.Second).UnixNano())
	if ok := dispatcher.TryEnqueue(1); !ok {
		t.Fatal("expected enqueue after failure cooldown to succeed")
	}
}

func TestAccountPoolQuotaRefreshDispatcher_PrunesExpiredIdleStates(t *testing.T) {
	dispatcher := newAccountPoolQuotaRefreshDispatcher(context.Background(), func(ctx context.Context, id int64) (AccountProfileRefreshResult, error) {
		return AccountProfileRefreshResult{Success: true}, nil
	})
	if dispatcher == nil {
		t.Fatal("expected dispatcher")
	}
	defer dispatcher.Close()

	base := time.Now()
	dispatcher.now = func() time.Time { return base }
	dispatcher.states[11] = &accountPoolQuotaRefreshState{
		lastSuccess: base.Add(-defaultAccountPoolQuotaRefreshSuccessCooldown - time.Second),
	}
	dispatcher.states[22] = &accountPoolQuotaRefreshState{
		lastFailure: base.Add(-defaultAccountPoolQuotaRefreshFailureCooldown - time.Second),
	}
	dispatcher.states[33] = &accountPoolQuotaRefreshState{
		queued: true,
	}

	if ok := dispatcher.TryEnqueue(44); !ok {
		t.Fatal("expected enqueue to succeed")
	}

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if _, ok := dispatcher.states[11]; ok {
		t.Fatal("expected expired success state to be pruned")
	}
	if _, ok := dispatcher.states[22]; ok {
		t.Fatal("expected expired failure state to be pruned")
	}
	if _, ok := dispatcher.states[33]; !ok {
		t.Fatal("expected queued state to be retained")
	}
	if _, ok := dispatcher.states[44]; !ok {
		t.Fatal("expected newly enqueued state to exist")
	}
}

func TestSelectQuotaProbeAccountIDs_SelectsOldestEligibleOAuthAccounts(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-10 * time.Minute)
	stale := now.Add(-2 * time.Hour)
	accounts := []*store.UpstreamAccountRecord{
		{ID: 1, ProviderType: "api_key", Enabled: true, State: "active"},
		{ID: 2, ProviderType: "chatgpt_refresh_token", Enabled: true, State: "disabled_auth"},
		{ID: 3, ProviderType: "chatgpt_refresh_token", Enabled: true, State: "active", QuotaRefreshedAt: &fresh},
		{ID: 4, ProviderType: "chatgpt_refresh_token", Enabled: true, State: "active", QuotaRefreshedAt: &stale},
		{ID: 5, ProviderType: "chatgpt_refresh_token", Enabled: true, State: "active"},
		{ID: 6, ProviderType: "chatgpt_refresh_token", Enabled: false, State: "active"},
	}

	ids := selectQuotaProbeAccountIDs(accounts, now, defaultAccountPoolQuotaProbeStaleAge, 2)
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %v", ids)
	}
	if ids[0] != 5 || ids[1] != 4 {
		t.Fatalf("expected nil-refreshed account first then oldest stale account, got %v", ids)
	}
}
