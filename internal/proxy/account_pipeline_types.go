package proxy

import (
	"context"
	"time"

	svc "cc-forwarder/internal/service"
)

// AccountPoolService 账号池调度接口（由 service 层实现）
type AccountPoolService interface {
	PrepareSchedulableAccounts(ctx context.Context, requestID, requestPath string) (svc.AccountScheduleResult, error)
	PreviewSchedulableAccounts(ctx context.Context, requestPath string) (svc.AccountScheduleResult, error)
	CompleteLatestScheduleSnapshot(ctx context.Context, requestID string, accountID int64, accountName, outcome, finalError string) error
	TryEnqueueQuotaRefresh(id int64) bool
	MarkAccountSuccess(ctx context.Context, id int64) error
	MarkAccountSuccessIfNoNewerFailure(ctx context.Context, id int64, attemptStartedAt time.Time) (bool, error)
	MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error
	MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldown time.Duration) error
	MarkAccountUsageLimitExceeded(ctx context.Context, id int64, reason, planType string, resetAt time.Time) error
	RecordAccountSoftFailure(ctx context.Context, id int64, reason, category string, retryAfter time.Duration) (bool, error)
}
