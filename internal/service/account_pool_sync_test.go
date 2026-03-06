package service

import (
	"context"
	"testing"
	"time"

	"cc-forwarder/internal/store"
)

type syncFinishStore struct {
	source       *store.SubscriptionSourceRecord
	updateCalls  int
	finishCalls  int
	updateCtxErr error
	finishCtxErr error
}

func (m *syncFinishStore) CreateSource(ctx context.Context, record *store.SubscriptionSourceRecord) (*store.SubscriptionSourceRecord, error) {
	return nil, nil
}
func (m *syncFinishStore) UpdateSource(ctx context.Context, record *store.SubscriptionSourceRecord) error {
	return nil
}
func (m *syncFinishStore) DeleteSource(ctx context.Context, id int64) error { return nil }
func (m *syncFinishStore) GetSource(ctx context.Context, id int64) (*store.SubscriptionSourceRecord, error) {
	return m.source, nil
}
func (m *syncFinishStore) ListSources(ctx context.Context) ([]*store.SubscriptionSourceRecord, error) {
	return nil, nil
}
func (m *syncFinishStore) ToggleSource(ctx context.Context, id int64, enabled bool) error { return nil }
func (m *syncFinishStore) UpdateSourceSyncStatus(ctx context.Context, id int64, status, lastError string, syncAt time.Time) error {
	m.updateCalls++
	m.updateCtxErr = ctx.Err()
	return nil
}
func (m *syncFinishStore) CreateAccount(ctx context.Context, record *store.UpstreamAccountRecord) (*store.UpstreamAccountRecord, error) {
	return nil, nil
}
func (m *syncFinishStore) UpdateAccount(ctx context.Context, record *store.UpstreamAccountRecord) error {
	return nil
}
func (m *syncFinishStore) DeleteAccount(ctx context.Context, id int64) error { return nil }
func (m *syncFinishStore) GetAccount(ctx context.Context, id int64) (*store.UpstreamAccountRecord, error) {
	return nil, nil
}
func (m *syncFinishStore) ListAccounts(ctx context.Context, includeDisabled bool) ([]*store.UpstreamAccountRecord, error) {
	return nil, nil
}
func (m *syncFinishStore) ListAccountsBySource(ctx context.Context, sourceID int64) ([]*store.UpstreamAccountRecord, error) {
	return nil, nil
}
func (m *syncFinishStore) ListSchedulableAccounts(ctx context.Context, now time.Time) ([]*store.UpstreamAccountRecord, error) {
	return nil, nil
}
func (m *syncFinishStore) FindAccountByFingerprint(ctx context.Context, fingerprint string) (*store.UpstreamAccountRecord, error) {
	return nil, nil
}
func (m *syncFinishStore) ToggleAccount(ctx context.Context, id int64, enabled bool) error {
	return nil
}
func (m *syncFinishStore) MarkAccountSuccess(ctx context.Context, id int64, successAt time.Time) error {
	return nil
}
func (m *syncFinishStore) MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error {
	return nil
}
func (m *syncFinishStore) MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldownUntil time.Time) error {
	return nil
}
func (m *syncFinishStore) DisableAccountsBySourceExcept(ctx context.Context, sourceID int64, keepFingerprints []string) (int, error) {
	return 0, nil
}
func (m *syncFinishStore) CreateSyncLog(ctx context.Context, record *store.SyncLogRecord) (*store.SyncLogRecord, error) {
	return &store.SyncLogRecord{ID: 1}, nil
}
func (m *syncFinishStore) FinishSyncLog(ctx context.Context, id int64, result string, added, updated, disabled int, errorSummary string, finishedAt time.Time) error {
	m.finishCalls++
	m.finishCtxErr = ctx.Err()
	return nil
}
func (m *syncFinishStore) ListRecentSyncLogs(ctx context.Context, sourceID int64, limit int) ([]*store.SyncLogRecord, error) {
	return nil, nil
}

func TestSyncSubscriptionSource_FinishUsesIndependentContext(t *testing.T) {
	mockStore := &syncFinishStore{
		source: &store.SubscriptionSourceRecord{
			ID:  1,
			URL: "https://example.com/source.txt",
		},
	}
	service := NewAccountPoolService(mockStore, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.SyncSubscriptionSource(ctx, 1); err == nil {
		t.Fatal("expected sync to fail when request context is canceled")
	}

	if mockStore.updateCalls == 0 {
		t.Fatal("expected sync status update to be attempted")
	}
	if mockStore.finishCalls == 0 {
		t.Fatal("expected sync log finish to be attempted")
	}
	if mockStore.updateCtxErr != nil {
		t.Fatalf("expected finish status update to use independent context, got ctx err: %v", mockStore.updateCtxErr)
	}
	if mockStore.finishCtxErr != nil {
		t.Fatalf("expected finish log write to use independent context, got ctx err: %v", mockStore.finishCtxErr)
	}
}
