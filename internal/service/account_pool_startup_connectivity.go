package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

const defaultStartupConnectivityConcurrency = 4

type StartupConnectivityCheckFailure struct {
	AccountID int64  `json:"account_id"`
	Message   string `json:"message"`
}

type StartupConnectivityCheckSummary struct {
	Total        int                               `json:"total"`
	SuccessCount int                               `json:"success_count"`
	FailureCount int                               `json:"failure_count"`
	SkippedCount int                               `json:"skipped_count"`
	Failures     []StartupConnectivityCheckFailure `json:"failures,omitempty"`
}

func (s *AccountPoolService) RunStartupConnectivityChecks(ctx context.Context, concurrency int) StartupConnectivityCheckSummary {
	summary := StartupConnectivityCheckSummary{
		Failures: make([]StartupConnectivityCheckFailure, 0),
	}
	if s == nil {
		return summary
	}

	accounts, err := s.ListAccounts(ctx, true)
	if err != nil {
		slog.Warn("启动连通性检查: 读取账号列表失败", "error", err)
		return summary
	}

	eligible := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			summary.SkippedCount++
			continue
		}
		if !account.Enabled || strings.TrimSpace(account.CredentialRaw) == "" {
			summary.SkippedCount++
			continue
		}
		eligible = append(eligible, account.ID)
	}
	summary.Total = len(eligible)
	if len(eligible) == 0 {
		return summary
	}

	limit := concurrency
	if limit <= 0 {
		limit = defaultStartupConnectivityConcurrency
	}
	if limit > len(eligible) {
		limit = len(eligible)
	}

	runner := s.runStartupConnectivityCheck
	semaphore := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, accountID := range eligible {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(id int64) {
			defer wg.Done()
			defer func() { <-semaphore }()

			err := runner(ctx, id)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				summary.FailureCount++
				summary.Failures = append(summary.Failures, StartupConnectivityCheckFailure{
					AccountID: id,
					Message:   err.Error(),
				})
				slog.Warn("启动连通性检查: 账号检测失败", "account_id", id, "error", err)
				return
			}

			summary.SuccessCount++
		}(accountID)
	}

	wg.Wait()
	return summary
}

func (s *AccountPoolService) runStartupConnectivityCheck(ctx context.Context, id int64) error {
	if s == nil {
		return nil
	}
	if s.startupConnectivityTestRunner != nil {
		return s.startupConnectivityTestRunner(ctx, id)
	}
	return s.TestUpstreamAccount(ctx, id)
}
