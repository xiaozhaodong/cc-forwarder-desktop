// app_api_account_pool.go - v6.0+ 账号池管理 API (Wails Bindings)
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cc-forwarder/internal/store"
)

// UpstreamAccountInfo 上游账号信息
type UpstreamAccountInfo struct {
	ID                            int64    `json:"id"`
	ProviderType                  string   `json:"provider_type"`
	AccountName                   string   `json:"account_name"`
	CredentialRaw                 string   `json:"credential_raw"`
	BaseURL                       string   `json:"base_url"`
	CostMultiplier                float64  `json:"cost_multiplier"`
	InputCostMultiplier           float64  `json:"input_cost_multiplier"`
	OutputCostMultiplier          float64  `json:"output_cost_multiplier"`
	CacheCreationCostMultiplier   float64  `json:"cache_creation_cost_multiplier"`
	CacheCreationCostMultiplier1h float64  `json:"cache_creation_cost_multiplier_1h"`
	CacheReadCostMultiplier       float64  `json:"cache_read_cost_multiplier"`
	Priority                      int      `json:"priority"`
	Enabled                       bool     `json:"enabled"`
	State                         string   `json:"state"`
	CooldownUntil                 string   `json:"cooldown_until"`
	FailCount                     int      `json:"fail_count"`
	LastSuccessAt                 string   `json:"last_success_at"`
	LastError                     string   `json:"last_error"`
	PlanType                      string   `json:"plan_type"`
	ChatGPTAccountID              string   `json:"chatgpt_account_id"`
	ChatGPTUserID                 string   `json:"chatgpt_user_id"`
	OrganizationID                string   `json:"organization_id"`
	Quota5HUsedPercent            *float64 `json:"quota_5h_used_percent,omitempty"`
	Quota5HResetAt                string   `json:"quota_5h_reset_at"`
	QuotaWeeklyUsedPercent        *float64 `json:"quota_weekly_used_percent,omitempty"`
	QuotaWeeklyResetAt            string   `json:"quota_weekly_reset_at"`
	QuotaStatus                   string   `json:"quota_status"`
	QuotaRefreshedAt              string   `json:"quota_refreshed_at"`
	Fingerprint                   string   `json:"fingerprint"`
	CreatedAt                     string   `json:"created_at"`
	UpdatedAt                     string   `json:"updated_at"`
}

// CreateUpstreamAccountInput 创建/更新账号输入
type CreateUpstreamAccountInput struct {
	ProviderType                  string  `json:"provider_type"`
	AccountName                   string  `json:"account_name"`
	CredentialRaw                 string  `json:"credential_raw"`
	BaseURL                       string  `json:"base_url"`
	CostMultiplier                float64 `json:"cost_multiplier"`
	InputCostMultiplier           float64 `json:"input_cost_multiplier"`
	OutputCostMultiplier          float64 `json:"output_cost_multiplier"`
	CacheCreationCostMultiplier   float64 `json:"cache_creation_cost_multiplier"`
	CacheCreationCostMultiplier1h float64 `json:"cache_creation_cost_multiplier_1h"`
	CacheReadCostMultiplier       float64 `json:"cache_read_cost_multiplier"`
	Priority                      int     `json:"priority"`
	Enabled                       bool    `json:"enabled"`
}

// TestUpstreamAccountResult 测试连通性结果
type TestUpstreamAccountResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// RefreshUpstreamAccountProfileResult 刷新账号画像结果
type RefreshUpstreamAccountProfileResult struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	QuotaStatus string `json:"quota_status,omitempty"`
}

// AccountScheduleCandidateDecisionInfo 最近一次调度候选账号决策信息
type AccountScheduleCandidateDecisionInfo struct {
	AccountID               int64    `json:"account_id"`
	AccountName             string   `json:"account_name"`
	ProviderType            string   `json:"provider_type"`
	Priority                int      `json:"priority"`
	TierIndex               int      `json:"tier_index"`
	TierLabel               string   `json:"tier_label"`
	QuotaStatus             string   `json:"quota_status"`
	EffectiveQuotaRemaining *float64 `json:"effective_quota_remaining,omitempty"`
	FailCount               int      `json:"fail_count"`
	LastSuccessAt           string   `json:"last_success_at"`
	Decision                string   `json:"decision"`
	Reason                  string   `json:"reason"`
	ReasonDetail            string   `json:"reason_detail"`
	RuntimeOutcome          string   `json:"runtime_outcome,omitempty"`
	RuntimeError            string   `json:"runtime_error,omitempty"`
}

// LatestAccountScheduleSnapshotInfo 最近一次调度快照信息
type LatestAccountScheduleSnapshotInfo struct {
	HasSnapshot             bool                                   `json:"has_snapshot"`
	RequestID               string                                 `json:"request_id,omitempty"`
	CapturedAt              string                                 `json:"captured_at"`
	UpdatedAt               string                                 `json:"updated_at"`
	RequestPath             string                                 `json:"request_path"`
	SelectedPriority        int                                    `json:"selected_priority"`
	SelectedTierIndex       int                                    `json:"selected_tier_index"`
	SelectedTierLabel       string                                 `json:"selected_tier_label"`
	DegradedToLowerPriority bool                                   `json:"degraded_to_lower_priority"`
	SelectedAccountID       int64                                  `json:"selected_account_id"`
	SelectedAccountName     string                                 `json:"selected_account_name"`
	FinalOutcome            string                                 `json:"final_outcome"`
	FinalError              string                                 `json:"final_error"`
	Summary                 string                                 `json:"summary"`
	Candidates              []AccountScheduleCandidateDecisionInfo `json:"candidates"`
}

// GetUpstreamAccounts 获取账号列表
func (a *App) GetUpstreamAccounts() ([]UpstreamAccountInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return nil, fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	records, err := a.accountPoolService.ListAccounts(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("获取账号列表失败: %w", err)
	}

	out := make([]UpstreamAccountInfo, 0, len(records))
	for _, rec := range records {
		out = append(out, UpstreamAccountInfo{
			ID:                            rec.ID,
			ProviderType:                  rec.ProviderType,
			AccountName:                   rec.AccountName,
			CredentialRaw:                 rec.CredentialRaw,
			BaseURL:                       rec.BaseURL,
			CostMultiplier:                rec.CostMultiplier,
			InputCostMultiplier:           rec.InputCostMultiplier,
			OutputCostMultiplier:          rec.OutputCostMultiplier,
			CacheCreationCostMultiplier:   rec.CacheCreationCostMultiplier,
			CacheCreationCostMultiplier1h: rec.CacheCreationCostMultiplier1h,
			CacheReadCostMultiplier:       rec.CacheReadCostMultiplier,
			Priority:                      rec.Priority,
			Enabled:                       rec.Enabled,
			State:                         rec.State,
			CooldownUntil:                 formatOptionalTime(rec.CooldownUntil),
			FailCount:                     rec.FailCount,
			LastSuccessAt:                 formatOptionalTime(rec.LastSuccessAt),
			LastError:                     rec.LastError,
			PlanType:                      rec.PlanType,
			ChatGPTAccountID:              rec.ChatGPTAccountID,
			ChatGPTUserID:                 rec.ChatGPTUserID,
			OrganizationID:                rec.OrganizationID,
			Quota5HUsedPercent:            rec.Quota5HUsedPercent,
			Quota5HResetAt:                formatOptionalTime(rec.Quota5HResetAt),
			QuotaWeeklyUsedPercent:        rec.QuotaWeeklyUsedPercent,
			QuotaWeeklyResetAt:            formatOptionalTime(rec.QuotaWeeklyResetAt),
			QuotaStatus:                   rec.QuotaStatus,
			QuotaRefreshedAt:              formatOptionalTime(rec.QuotaRefreshedAt),
			Fingerprint:                   rec.Fingerprint,
			CreatedAt:                     formatTime(rec.CreatedAt),
			UpdatedAt:                     formatTime(rec.UpdatedAt),
		})
	}
	return out, nil
}

// GetLatestAccountScheduleSnapshot 获取最近一次账号池调度快照
func (a *App) GetLatestAccountScheduleSnapshot() (LatestAccountScheduleSnapshotInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return LatestAccountScheduleSnapshotInfo{}, fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snapshot, err := a.accountPoolService.GetLatestAccountScheduleSnapshot(ctx)
	if err != nil {
		return LatestAccountScheduleSnapshotInfo{}, fmt.Errorf("获取最近一次调度快照失败: %w", err)
	}
	if snapshot == nil {
		return LatestAccountScheduleSnapshotInfo{HasSnapshot: false}, nil
	}

	out := LatestAccountScheduleSnapshotInfo{
		HasSnapshot:             true,
		RequestID:               snapshot.RequestID,
		CapturedAt:              formatTime(snapshot.CapturedAt),
		UpdatedAt:               formatTime(snapshot.UpdatedAt),
		RequestPath:             snapshot.RequestPath,
		SelectedPriority:        snapshot.SelectedPriority,
		SelectedTierIndex:       snapshot.SelectedTierIndex,
		SelectedTierLabel:       snapshot.SelectedTierLabel,
		DegradedToLowerPriority: snapshot.DegradedToLowerPriority,
		SelectedAccountID:       snapshot.SelectedAccountID,
		SelectedAccountName:     snapshot.SelectedAccountName,
		FinalOutcome:            snapshot.FinalOutcome,
		FinalError:              snapshot.FinalError,
		Summary:                 snapshot.Summary,
		Candidates:              make([]AccountScheduleCandidateDecisionInfo, 0, len(snapshot.Candidates)),
	}
	for _, candidate := range snapshot.Candidates {
		out.Candidates = append(out.Candidates, AccountScheduleCandidateDecisionInfo{
			AccountID:               candidate.AccountID,
			AccountName:             candidate.AccountName,
			ProviderType:            candidate.ProviderType,
			Priority:                candidate.Priority,
			TierIndex:               candidate.TierIndex,
			TierLabel:               candidate.TierLabel,
			QuotaStatus:             candidate.QuotaStatus,
			EffectiveQuotaRemaining: candidate.EffectiveQuotaRemaining,
			FailCount:               candidate.FailCount,
			LastSuccessAt:           formatOptionalTime(candidate.LastSuccessAt),
			Decision:                candidate.Decision,
			Reason:                  candidate.Reason,
			ReasonDetail:            candidate.ReasonDetail,
			RuntimeOutcome:          candidate.RuntimeOutcome,
			RuntimeError:            candidate.RuntimeError,
		})
	}
	return out, nil
}

// RefreshUpstreamAccountProfile 刷新账号画像与 quota 信息
func (a *App) RefreshUpstreamAccountProfile(id int64) (RefreshUpstreamAccountProfileResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return RefreshUpstreamAccountProfileResult{}, fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := a.accountPoolService.RefreshAccountProfile(ctx, id)
	if err != nil {
		return RefreshUpstreamAccountProfileResult{}, err
	}
	return RefreshUpstreamAccountProfileResult{
		Success:     result.Success,
		Message:     result.Message,
		QuotaStatus: result.QuotaStatus,
	}, nil
}

// CreateUpstreamAccount 创建账号
func (a *App) CreateUpstreamAccount(input CreateUpstreamAccountInput) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := a.accountPoolService.CreateAccount(ctx, &store.UpstreamAccountRecord{
		ProviderType:                  strings.TrimSpace(input.ProviderType),
		AccountName:                   strings.TrimSpace(input.AccountName),
		CredentialRaw:                 strings.TrimSpace(input.CredentialRaw),
		BaseURL:                       strings.TrimSpace(input.BaseURL),
		CostMultiplier:                input.CostMultiplier,
		InputCostMultiplier:           input.InputCostMultiplier,
		OutputCostMultiplier:          input.OutputCostMultiplier,
		CacheCreationCostMultiplier:   input.CacheCreationCostMultiplier,
		CacheCreationCostMultiplier1h: input.CacheCreationCostMultiplier1h,
		CacheReadCostMultiplier:       input.CacheReadCostMultiplier,
		Priority:                      input.Priority,
		Enabled:                       input.Enabled,
		State:                         "active",
	})
	if err != nil {
		return fmt.Errorf("创建账号失败: %w", err)
	}
	a.syncAccountMultipliersToTrackerAsync()
	return nil
}

// UpdateUpstreamAccount 更新账号
func (a *App) UpdateUpstreamAccount(id int64, input CreateUpstreamAccountInput) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	existing, err := a.accountPoolService.GetAccount(ctx, id)
	if err != nil {
		return fmt.Errorf("读取账号失败: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("账号不存在: %d", id)
	}

	existing.ProviderType = strings.TrimSpace(input.ProviderType)
	existing.AccountName = strings.TrimSpace(input.AccountName)
	existing.CredentialRaw = strings.TrimSpace(input.CredentialRaw)
	existing.BaseURL = strings.TrimSpace(input.BaseURL)
	existing.CostMultiplier = input.CostMultiplier
	existing.InputCostMultiplier = input.InputCostMultiplier
	existing.OutputCostMultiplier = input.OutputCostMultiplier
	existing.CacheCreationCostMultiplier = input.CacheCreationCostMultiplier
	existing.CacheCreationCostMultiplier1h = input.CacheCreationCostMultiplier1h
	existing.CacheReadCostMultiplier = input.CacheReadCostMultiplier
	existing.Priority = input.Priority
	existing.Enabled = input.Enabled
	if existing.Enabled && existing.State == "disabled_auth" {
		existing.State = "active"
	}
	if err := a.accountPoolService.UpdateAccount(ctx, existing); err != nil {
		return fmt.Errorf("更新账号失败: %w", err)
	}
	a.syncAccountMultipliersToTrackerAsync()
	return nil
}

// DeleteUpstreamAccount 删除账号
func (a *App) DeleteUpstreamAccount(id int64) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.accountPoolService.DeleteAccount(ctx, id); err != nil {
		return err
	}
	a.syncAccountMultipliersToTrackerAsync()
	return nil
}

// ToggleUpstreamAccount 启停账号
func (a *App) ToggleUpstreamAccount(id int64, enabled bool) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.accountPoolService.ToggleAccount(ctx, id, enabled)
}

// TestUpstreamAccount 测试账号连通性
func (a *App) TestUpstreamAccount(id int64) (TestUpstreamAccountResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return TestUpstreamAccountResult{}, fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err := a.accountPoolService.TestUpstreamAccount(ctx, id)
	if err != nil {
		return TestUpstreamAccountResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return TestUpstreamAccountResult{
		Success: true,
		Message: "连通性测试通过",
	}, nil
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return formatTime(*t)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
