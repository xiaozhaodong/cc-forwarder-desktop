// app_api_account_pool.go - v6.0+ 账号池管理 API (Wails Bindings)
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cc-forwarder/internal/store"
)

// SubscriptionSourceInfo 订阅源信息
type SubscriptionSourceInfo struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Enabled    bool   `json:"enabled"`
	SyncMode   string `json:"sync_mode"`
	LastSyncAt string `json:"last_sync_at"`
	LastStatus string `json:"last_status"`
	LastError  string `json:"last_error"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// UpstreamAccountInfo 上游账号信息
type UpstreamAccountInfo struct {
	ID            int64  `json:"id"`
	SourceID      *int64 `json:"source_id,omitempty"`
	SourceName    string `json:"source_name"`
	ProviderType  string `json:"provider_type"`
	AccountName   string `json:"account_name"`
	CredentialRaw string `json:"credential_raw"`
	BaseURL       string `json:"base_url"`
	Priority      int    `json:"priority"`
	Enabled       bool   `json:"enabled"`
	State         string `json:"state"`
	CooldownUntil string `json:"cooldown_until"`
	FailCount     int    `json:"fail_count"`
	LastSuccessAt string `json:"last_success_at"`
	LastError     string `json:"last_error"`
	Fingerprint   string `json:"fingerprint"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// CreateSubscriptionSourceInput 创建/更新订阅源输入
type CreateSubscriptionSourceInput struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Enabled  bool   `json:"enabled"`
	SyncMode string `json:"sync_mode"`
}

// CreateUpstreamAccountInput 创建/更新账号输入
type CreateUpstreamAccountInput struct {
	SourceID      *int64 `json:"source_id,omitempty"`
	ProviderType  string `json:"provider_type"`
	AccountName   string `json:"account_name"`
	CredentialRaw string `json:"credential_raw"`
	BaseURL       string `json:"base_url"`
	Priority      int    `json:"priority"`
	Enabled       bool   `json:"enabled"`
}

// SyncSubscriptionSourceResultInfo 手动同步结果
type SyncSubscriptionSourceResultInfo struct {
	SourceID int64 `json:"source_id"`
	Added    int   `json:"added"`
	Updated  int   `json:"updated"`
	Disabled int   `json:"disabled"`
}

// TestUpstreamAccountResult 测试连通性结果
type TestUpstreamAccountResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// GetSubscriptionSources 获取订阅源列表
func (a *App) GetSubscriptionSources() ([]SubscriptionSourceInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return nil, fmt.Errorf("账号池服务未启用 (需要开启 usage_tracking)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	records, err := a.accountPoolService.ListSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取订阅源失败: %w", err)
	}

	out := make([]SubscriptionSourceInfo, 0, len(records))
	for _, rec := range records {
		out = append(out, SubscriptionSourceInfo{
			ID:         rec.ID,
			Name:       rec.Name,
			URL:        rec.URL,
			Enabled:    rec.Enabled,
			SyncMode:   rec.SyncMode,
			LastSyncAt: formatOptionalTime(rec.LastSyncAt),
			LastStatus: rec.LastStatus,
			LastError:  rec.LastError,
			CreatedAt:  formatTime(rec.CreatedAt),
			UpdatedAt:  formatTime(rec.UpdatedAt),
		})
	}
	return out, nil
}

// CreateSubscriptionSource 创建订阅源
func (a *App) CreateSubscriptionSource(input CreateSubscriptionSourceInput) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := a.accountPoolService.CreateSource(ctx, &store.SubscriptionSourceRecord{
		Name:     strings.TrimSpace(input.Name),
		URL:      strings.TrimSpace(input.URL),
		Enabled:  input.Enabled,
		SyncMode: strings.TrimSpace(input.SyncMode),
	})
	if err != nil {
		return fmt.Errorf("创建订阅源失败: %w", err)
	}
	return nil
}

// UpdateSubscriptionSource 更新订阅源
func (a *App) UpdateSubscriptionSource(id int64, input CreateSubscriptionSourceInput) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := a.accountPoolService.UpdateSource(ctx, &store.SubscriptionSourceRecord{
		ID:       id,
		Name:     strings.TrimSpace(input.Name),
		URL:      strings.TrimSpace(input.URL),
		Enabled:  input.Enabled,
		SyncMode: strings.TrimSpace(input.SyncMode),
	})
	if err != nil {
		return fmt.Errorf("更新订阅源失败: %w", err)
	}
	return nil
}

// DeleteSubscriptionSource 删除订阅源
func (a *App) DeleteSubscriptionSource(id int64) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.accountPoolService.DeleteSource(ctx, id)
}

// ToggleSubscriptionSource 切换订阅源启用状态
func (a *App) ToggleSubscriptionSource(id int64, enabled bool) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.accountPoolService.ToggleSource(ctx, id, enabled)
}

// SyncSubscriptionSource 手动同步订阅源
func (a *App) SyncSubscriptionSource(id int64) (SyncSubscriptionSourceResultInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.accountPoolService == nil {
		return SyncSubscriptionSourceResultInfo{}, fmt.Errorf("账号池服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := a.accountPoolService.SyncSubscriptionSource(ctx, id)
	if err != nil {
		return SyncSubscriptionSourceResultInfo{}, fmt.Errorf("同步订阅源失败: %w", err)
	}
	return SyncSubscriptionSourceResultInfo{
		SourceID: result.SourceID,
		Added:    result.Added,
		Updated:  result.Updated,
		Disabled: result.Disabled,
	}, nil
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
			ID:            rec.ID,
			SourceID:      rec.SourceID,
			SourceName:    rec.SourceName,
			ProviderType:  rec.ProviderType,
			AccountName:   rec.AccountName,
			CredentialRaw: rec.CredentialRaw,
			BaseURL:       rec.BaseURL,
			Priority:      rec.Priority,
			Enabled:       rec.Enabled,
			State:         rec.State,
			CooldownUntil: formatOptionalTime(rec.CooldownUntil),
			FailCount:     rec.FailCount,
			LastSuccessAt: formatOptionalTime(rec.LastSuccessAt),
			LastError:     rec.LastError,
			Fingerprint:   rec.Fingerprint,
			CreatedAt:     formatTime(rec.CreatedAt),
			UpdatedAt:     formatTime(rec.UpdatedAt),
		})
	}
	return out, nil
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
		SourceID:      input.SourceID,
		ProviderType:  strings.TrimSpace(input.ProviderType),
		AccountName:   strings.TrimSpace(input.AccountName),
		CredentialRaw: strings.TrimSpace(input.CredentialRaw),
		BaseURL:       strings.TrimSpace(input.BaseURL),
		Priority:      input.Priority,
		Enabled:       input.Enabled,
		State:         "active",
	})
	if err != nil {
		return fmt.Errorf("创建账号失败: %w", err)
	}
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

	existing.SourceID = input.SourceID
	existing.ProviderType = strings.TrimSpace(input.ProviderType)
	existing.AccountName = strings.TrimSpace(input.AccountName)
	existing.CredentialRaw = strings.TrimSpace(input.CredentialRaw)
	existing.BaseURL = strings.TrimSpace(input.BaseURL)
	existing.Priority = input.Priority
	existing.Enabled = input.Enabled
	if existing.Enabled && existing.State == "disabled_auth" {
		existing.State = "active"
	}
	if err := a.accountPoolService.UpdateAccount(ctx, existing); err != nil {
		return fmt.Errorf("更新账号失败: %w", err)
	}
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
	return a.accountPoolService.DeleteAccount(ctx, id)
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
