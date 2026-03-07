// Package service 提供账号池业务服务
package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/accountauth"
	"cc-forwarder/internal/store"
	"cc-forwarder/internal/transport"
)

const (
	defaultOpenAIResponsesURL   = "https://api.openai.com/v1/responses"
	defaultChatGPTCodexTestURL  = "https://chatgpt.com/backend-api/codex/responses"
	defaultOpenAIBetaHeader     = "responses=experimental"
	defaultOAuthOriginatorValue = "codex_cli_rs"
)

var chatGPTCodexTestURL = defaultChatGPTCodexTestURL

// AccountPoolService 账号池服务
type AccountPoolService struct {
	store                  store.AccountPoolStore
	config                 *config.Config
	refreshTokenManager    *accountauth.OpenAIRefreshTokenManager
	scheduleSnapshots      *latestAccountScheduleSnapshotStore
	softFailureTracker     *accountSoftFailureTracker
	quotaRefreshDispatcher *accountPoolQuotaRefreshDispatcher
	quotaProbeScheduler    *accountPoolQuotaProbeScheduler
}

// NewAccountPoolService 创建账号池服务
func NewAccountPoolService(st store.AccountPoolStore, cfg *config.Config) *AccountPoolService {
	svc := &AccountPoolService{
		store:               st,
		config:              cfg,
		refreshTokenManager: accountauth.NewOpenAIRefreshTokenManager(cfg),
		scheduleSnapshots:   newLatestAccountScheduleSnapshotStore(),
		softFailureTracker:  newAccountSoftFailureTracker(),
	}
	svc.quotaRefreshDispatcher = newAccountPoolQuotaRefreshDispatcher(context.Background(), svc.RefreshAccountProfile)
	if cfg != nil && cfg.AccountPool.Enabled {
		svc.quotaProbeScheduler = newAccountPoolQuotaProbeScheduler(context.Background(), func(ctx context.Context) ([]*store.UpstreamAccountRecord, error) {
			return svc.ListAccounts(ctx, false)
		}, svc.TryEnqueueQuotaRefresh)
	}
	return svc
}

// ===== 账号 =====

func (s *AccountPoolService) ListAccounts(ctx context.Context, includeDisabled bool) ([]*store.UpstreamAccountRecord, error) {
	return s.store.ListAccounts(ctx, includeDisabled)
}

func (s *AccountPoolService) GetAccount(ctx context.Context, id int64) (*store.UpstreamAccountRecord, error) {
	return s.store.GetAccount(ctx, id)
}

func (s *AccountPoolService) CreateAccount(ctx context.Context, rec *store.UpstreamAccountRecord) (*store.UpstreamAccountRecord, error) {
	if rec == nil {
		return nil, fmt.Errorf("account record is nil")
	}
	rec.AccountName = strings.TrimSpace(rec.AccountName)
	rec.ProviderType = strings.TrimSpace(rec.ProviderType)
	rec.CredentialRaw = strings.TrimSpace(rec.CredentialRaw)
	if rec.AccountName == "" {
		return nil, fmt.Errorf("账号名称不能为空")
	}
	if rec.CredentialRaw == "" {
		return nil, fmt.Errorf("账号凭据不能为空")
	}
	if rec.ProviderType == "" {
		rec.ProviderType = accountauth.InferProviderType(rec.CredentialRaw)
	}
	s.populateAccountProfile(rec)
	return s.store.CreateAccount(ctx, rec)
}

func (s *AccountPoolService) UpdateAccount(ctx context.Context, rec *store.UpstreamAccountRecord) error {
	if rec == nil {
		return fmt.Errorf("account record is nil")
	}
	rec.AccountName = strings.TrimSpace(rec.AccountName)
	rec.ProviderType = strings.TrimSpace(rec.ProviderType)
	rec.CredentialRaw = strings.TrimSpace(rec.CredentialRaw)
	if rec.AccountName == "" {
		return fmt.Errorf("账号名称不能为空")
	}
	if rec.CredentialRaw == "" {
		return fmt.Errorf("账号凭据不能为空")
	}
	if rec.ProviderType == "" {
		rec.ProviderType = accountauth.InferProviderType(rec.CredentialRaw)
	}
	s.populateAccountProfile(rec)
	return s.store.UpdateAccount(ctx, rec)
}

func (s *AccountPoolService) DeleteAccount(ctx context.Context, id int64) error {
	return s.store.DeleteAccount(ctx, id)
}

func (s *AccountPoolService) ToggleAccount(ctx context.Context, id int64, enabled bool) error {
	return s.store.ToggleAccount(ctx, id, enabled)
}

func (s *AccountPoolService) Close() error {
	var firstErr error
	if s.quotaProbeScheduler != nil {
		if err := s.quotaProbeScheduler.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.quotaRefreshDispatcher != nil {
		if err := s.quotaRefreshDispatcher.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *AccountPoolService) TryEnqueueQuotaRefresh(id int64) bool {
	if s == nil || s.quotaRefreshDispatcher == nil {
		return false
	}
	return s.quotaRefreshDispatcher.TryEnqueue(id)
}

// ===== 调度/状态回写（供 proxy 使用） =====

func (s *AccountPoolService) ListSchedulableAccounts(ctx context.Context) ([]*store.UpstreamAccountRecord, error) {
	return s.prepareSchedulableAccounts(ctx, "", "")
}

func (s *AccountPoolService) MarkAccountSuccess(ctx context.Context, id int64) error {
	if err := s.store.MarkAccountSuccess(ctx, id, time.Now()); err != nil {
		return err
	}
	if s.softFailureTracker != nil {
		s.softFailureTracker.Clear(id)
	}
	return nil
}

func (s *AccountPoolService) MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error {
	if err := s.store.MarkAccountAuthFailed(ctx, id, reason); err != nil {
		return err
	}
	if s.softFailureTracker != nil {
		s.softFailureTracker.Clear(id)
	}
	return nil
}

func (s *AccountPoolService) MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldown time.Duration) error {
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	if err := s.store.MarkAccountTransientFailure(ctx, id, reason, time.Now().Add(cooldown)); err != nil {
		return err
	}
	if s.softFailureTracker != nil {
		s.softFailureTracker.Clear(id)
	}
	return nil
}

func (s *AccountPoolService) RecordAccountSoftFailure(ctx context.Context, id int64, reason, category string, retryAfter time.Duration) error {
	if id <= 0 {
		return fmt.Errorf("invalid account id: %d", id)
	}

	tracker := s.softFailureTracker
	if tracker == nil {
		tracker = newAccountSoftFailureTracker()
		s.softFailureTracker = tracker
	}

	now := tracker.Now()
	count := tracker.Record(id)
	if count < tracker.threshold {
		return nil
	}

	cooldown := resolveAccountSoftFailureCooldown(category, retryAfter)
	if err := s.store.MarkAccountTransientFailure(ctx, id, reason, now.Add(cooldown)); err != nil {
		return err
	}
	tracker.Clear(id)
	return nil
}

// TestUpstreamAccount 测试账号连通性
func (s *AccountPoolService) TestUpstreamAccount(ctx context.Context, id int64) error {
	acc, err := s.store.GetAccount(ctx, id)
	if err != nil {
		return fmt.Errorf("获取账号失败: %w", err)
	}
	if acc == nil {
		return fmt.Errorf("账号不存在: %d", id)
	}

	providerType := accountauth.NormalizeProviderType(acc.ProviderType)
	isOAuth := providerType == accountauth.ProviderChatGPTRefreshToken

	targetURL := defaultOpenAIResponsesURL
	if isOAuth {
		targetURL = chatGPTCodexTestURL
	} else {
		base := strings.TrimSuffix(strings.TrimSpace(acc.BaseURL), "/")
		if base == "" {
			base = "https://api.openai.com"
		}
		targetURL = base + "/v1/responses"
	}

	payload := strings.NewReader(`{"input":"ping"}`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, payload)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if err := accountauth.ApplyAccountAuth(ctx, req, acc.ProviderType, acc.CredentialRaw, s.refreshTokenManager); err != nil {
		return fmt.Errorf("应用账号鉴权失败: %w", err)
	}
	if isOAuth {
		accountID := accountauth.ExtractChatGPTAccountID(acc.CredentialRaw)
		if accountID == "" {
			return fmt.Errorf("OAuth 账号缺少 chatgpt_account_id，请重新授权后保存完整凭据")
		}
		req.Host = "chatgpt.com"
		req.Header.Set("Host", "chatgpt.com")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("OpenAI-Beta", defaultOpenAIBetaHeader)
		req.Header.Set("originator", defaultOAuthOriginatorValue)
		req.Header.Set("chatgpt-account-id", accountID)
	}

	client, err := s.buildHTTPClient(15 * time.Second)
	if err != nil {
		return fmt.Errorf("创建 HTTP 客户端失败: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连通性测试失败: %w", err)
	}
	defer resp.Body.Close()

	reachable := (resp.StatusCode >= 200 && resp.StatusCode < 400) ||
		resp.StatusCode == http.StatusBadRequest ||
		resp.StatusCode == http.StatusTooManyRequests
	if reachable {
		if err := s.MarkAccountSuccess(ctx, id); err != nil {
			return fmt.Errorf("写回账号连通成功时间失败: %w", err)
		}
		if isOAuth {
			_, _ = s.RefreshAccountProfile(ctx, id)
		}
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	bodyText := strings.TrimSpace(string(body))
	if isNoAvailableProviders503(resp.StatusCode, bodyText) {
		if err := s.MarkAccountSuccess(ctx, id); err != nil {
			return fmt.Errorf("写回账号连通成功时间失败: %w", err)
		}
		if isOAuth {
			_, _ = s.RefreshAccountProfile(ctx, id)
		}
		return nil
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		if !isOAuth && isMissingResponsesWriteScope(bodyText) {
			return fmt.Errorf("鉴权通过但权限不足 (%d): 缺少 api.responses.write，请重新走 OAuth 授权并更新 RT。原始响应: %s", resp.StatusCode, bodyText)
		}
		return fmt.Errorf("鉴权或权限不足 (%d): %s", resp.StatusCode, bodyText)
	default:
		return fmt.Errorf("上游返回状态码 %d: %s", resp.StatusCode, bodyText)
	}
}

func isMissingResponsesWriteScope(bodyText string) bool {
	lower := strings.ToLower(strings.TrimSpace(bodyText))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "api.responses.write") &&
		(strings.Contains(lower, "missing scopes") || strings.Contains(lower, "insufficient permissions"))
}

func isNoAvailableProviders503(statusCode int, bodyText string) bool {
	if statusCode != http.StatusServiceUnavailable {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(bodyText))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "no_available_providers") ||
		strings.Contains(lower, "no available providers")
}

func (s *AccountPoolService) buildHTTPClient(timeout time.Duration) (*http.Client, error) {
	if s.config == nil {
		return &http.Client{Timeout: timeout}, nil
	}
	tp, err := transport.CreateTransport(s.config)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: tp,
	}, nil
}
