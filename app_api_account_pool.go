// app_api_account_pool.go - v6.0+ 账号池管理 API (Wails Bindings)
package main

import (
	"context"
	"encoding/json"
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
	CredentialRawMasked           string   `json:"credential_raw_masked"`
	HasCredential                 bool     `json:"has_credential"`
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

// UpstreamAccountCredentialInfo 单账号显式查看凭据响应。
type UpstreamAccountCredentialInfo struct {
	ID                  int64  `json:"id"`
	CredentialRaw       string `json:"credential_raw"`
	CredentialRawMasked string `json:"credential_raw_masked"`
	HasCredential       bool   `json:"has_credential"`
}

// RefreshUpstreamAccountProfileResult 刷新账号画像结果
type RefreshUpstreamAccountProfileResult struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	QuotaStatus string `json:"quota_status,omitempty"`
}

// MoveUpstreamAccountToTierResult 手动主备切换结果。
type MoveUpstreamAccountToTierResult struct {
	Success bool   `json:"success"`
	Changed bool   `json:"changed"`
	Message string `json:"message"`
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
			CredentialRaw:                 "",
			CredentialRawMasked:           maskCredentialRaw(rec.ProviderType, rec.CredentialRaw),
			HasCredential:                 strings.TrimSpace(rec.CredentialRaw) != "",
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
			CooldownUntil:                 formatOptionalAccountRecordTime(rec.CooldownUntil),
			FailCount:                     rec.FailCount,
			LastSuccessAt:                 formatOptionalAccountRecordTime(rec.LastSuccessAt),
			LastError:                     rec.LastError,
			PlanType:                      rec.PlanType,
			ChatGPTAccountID:              rec.ChatGPTAccountID,
			ChatGPTUserID:                 rec.ChatGPTUserID,
			OrganizationID:                rec.OrganizationID,
			Quota5HUsedPercent:            rec.Quota5HUsedPercent,
			Quota5HResetAt:                formatOptionalAccountRecordTime(rec.Quota5HResetAt),
			QuotaWeeklyUsedPercent:        rec.QuotaWeeklyUsedPercent,
			QuotaWeeklyResetAt:            formatOptionalAccountRecordTime(rec.QuotaWeeklyResetAt),
			QuotaStatus:                   rec.QuotaStatus,
			QuotaRefreshedAt:              formatOptionalAccountRecordTime(rec.QuotaRefreshedAt),
			Fingerprint:                   rec.Fingerprint,
			CreatedAt:                     formatAccountRecordTime(rec.CreatedAt),
			UpdatedAt:                     formatAccountRecordTime(rec.UpdatedAt),
		})
	}
	return out, nil
}

// GetUpstreamAccountCredential 显式读取单个账号的完整凭据。
func (a *App) GetUpstreamAccountCredential(id int64) (UpstreamAccountCredentialInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return UpstreamAccountCredentialInfo{}, fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rec, err := a.accountPoolService.GetAccount(ctx, id)
	if err != nil {
		return UpstreamAccountCredentialInfo{}, fmt.Errorf("读取账号凭据失败: %w", err)
	}
	if rec == nil {
		return UpstreamAccountCredentialInfo{}, fmt.Errorf("账号不存在: %d", id)
	}

	return UpstreamAccountCredentialInfo{
		ID:                  rec.ID,
		CredentialRaw:       rec.CredentialRaw,
		CredentialRawMasked: maskCredentialRaw(rec.ProviderType, rec.CredentialRaw),
		HasCredential:       strings.TrimSpace(rec.CredentialRaw) != "",
	}, nil
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
			LastSuccessAt:           formatOptionalAccountRecordTime(candidate.LastSuccessAt),
			Decision:                candidate.Decision,
			Reason:                  candidate.Reason,
			ReasonDetail:            candidate.ReasonDetail,
			RuntimeOutcome:          candidate.RuntimeOutcome,
			RuntimeError:            candidate.RuntimeError,
		})
	}
	return out, nil
}

// MoveUpstreamAccountToTier 将账号切到主组/备组。
func (a *App) MoveUpstreamAccountToTier(id int64, targetTier string) (MoveUpstreamAccountToTierResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return MoveUpstreamAccountToTierResult{}, fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	targetTierIndex := 0
	switch strings.TrimSpace(strings.ToLower(targetTier)) {
	case "backup":
		targetTierIndex = 1
	case "", "primary":
		targetTierIndex = 0
	default:
		return MoveUpstreamAccountToTierResult{}, fmt.Errorf("不支持的目标层级: %s", targetTier)
	}

	changed, err := a.accountPoolService.MoveAccountToTier(ctx, id, targetTierIndex)
	if err != nil {
		return MoveUpstreamAccountToTierResult{}, fmt.Errorf("切换账号层级失败: %w", err)
	}

	message := "账号层级未发生变化"
	if changed {
		if targetTierIndex == 0 {
			message = "账号已切换到主组"
		} else {
			message = "账号已切换到备组"
		}
	}
	return MoveUpstreamAccountToTierResult{
		Success: true,
		Changed: changed,
		Message: message,
	}, nil
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

func formatOptionalAccountRecordTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return formatAccountRecordTime(*t)
}

func formatAccountRecordTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return store.FormatAccountDisplayTime(t)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func maskCredentialRaw(providerType, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "{") {
		var payload map[string]any
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			for _, key := range []string{"refresh_token", "access_token", "id_token", "token"} {
				if value, ok := payload[key].(string); ok {
					payload[key] = maskSecret(value)
				}
			}
			masked, err := json.Marshal(payload)
			if err == nil {
				return string(masked)
			}
		}
	}

	if strings.EqualFold(strings.TrimSpace(providerType), "api_key") {
		return maskSecret(trimmed)
	}
	return maskSecret(trimmed)
}

func maskSecret(secret string) string {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return ""
	}

	length := len(trimmed)
	switch {
	case length <= 6:
		return strings.Repeat("*", length)
	case length <= 12:
		return trimmed[:2] + strings.Repeat("*", length-4) + trimmed[length-2:]
	case length <= 20:
		return trimmed[:4] + strings.Repeat("*", length-8) + trimmed[length-4:]
	default:
		return trimmed[:6] + strings.Repeat("*", length-10) + trimmed[length-4:]
	}
}
