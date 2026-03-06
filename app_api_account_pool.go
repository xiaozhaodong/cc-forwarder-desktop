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
	ID            int64  `json:"id"`
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

// CreateUpstreamAccountInput 创建/更新账号输入
type CreateUpstreamAccountInput struct {
	ProviderType  string `json:"provider_type"`
	AccountName   string `json:"account_name"`
	CredentialRaw string `json:"credential_raw"`
	BaseURL       string `json:"base_url"`
	Priority      int    `json:"priority"`
	Enabled       bool   `json:"enabled"`
}

// TestUpstreamAccountResult 测试连通性结果
type TestUpstreamAccountResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
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
