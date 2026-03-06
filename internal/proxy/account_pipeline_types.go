package proxy

import (
	"context"
	"time"

	"cc-forwarder/internal/store"
)

// AccountPoolService 账号池调度接口（由 service 层实现）
type AccountPoolService interface {
	ListSchedulableAccounts(ctx context.Context) ([]*store.UpstreamAccountRecord, error)
	MarkAccountSuccess(ctx context.Context, id int64) error
	MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error
	MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldown time.Duration) error
}
