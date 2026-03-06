// Package service 提供账号池业务服务
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

// SyncSubscriptionSourceResult 手动同步结果
type SyncSubscriptionSourceResult struct {
	SourceID int64 `json:"source_id"`
	Added    int   `json:"added"`
	Updated  int   `json:"updated"`
	Disabled int   `json:"disabled"`
}

// ParsedUpstreamAccount 订阅源解析出的账号
type ParsedUpstreamAccount struct {
	AccountName  string
	ProviderType string
	Credential   string
	BaseURL      string
	Priority     int
}

// AccountPoolService 账号池服务
type AccountPoolService struct {
	store               store.AccountPoolStore
	config              *config.Config
	refreshTokenManager *accountauth.OpenAIRefreshTokenManager
}

// NewAccountPoolService 创建账号池服务
func NewAccountPoolService(st store.AccountPoolStore, cfg *config.Config) *AccountPoolService {
	return &AccountPoolService{
		store:               st,
		config:              cfg,
		refreshTokenManager: accountauth.NewOpenAIRefreshTokenManager(cfg),
	}
}

// ===== 订阅源 =====

func (s *AccountPoolService) ListSources(ctx context.Context) ([]*store.SubscriptionSourceRecord, error) {
	return s.store.ListSources(ctx)
}

func (s *AccountPoolService) GetSource(ctx context.Context, id int64) (*store.SubscriptionSourceRecord, error) {
	return s.store.GetSource(ctx, id)
}

func (s *AccountPoolService) CreateSource(ctx context.Context, rec *store.SubscriptionSourceRecord) (*store.SubscriptionSourceRecord, error) {
	if rec == nil {
		return nil, fmt.Errorf("source record is nil")
	}
	rec.Name = strings.TrimSpace(rec.Name)
	rec.URL = strings.TrimSpace(rec.URL)
	if rec.SyncMode == "" {
		rec.SyncMode = "manual"
	}
	return s.store.CreateSource(ctx, rec)
}

func (s *AccountPoolService) UpdateSource(ctx context.Context, rec *store.SubscriptionSourceRecord) error {
	if rec == nil {
		return fmt.Errorf("source record is nil")
	}
	rec.Name = strings.TrimSpace(rec.Name)
	rec.URL = strings.TrimSpace(rec.URL)
	if rec.SyncMode == "" {
		rec.SyncMode = "manual"
	}
	return s.store.UpdateSource(ctx, rec)
}

func (s *AccountPoolService) DeleteSource(ctx context.Context, id int64) error {
	return s.store.DeleteSource(ctx, id)
}

func (s *AccountPoolService) ToggleSource(ctx context.Context, id int64, enabled bool) error {
	return s.store.ToggleSource(ctx, id, enabled)
}

// SyncSubscriptionSource 手动同步订阅源
func (s *AccountPoolService) SyncSubscriptionSource(ctx context.Context, sourceID int64) (*SyncSubscriptionSourceResult, error) {
	source, err := s.store.GetSource(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("获取订阅源失败: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("订阅源不存在: %d", sourceID)
	}

	startedAt := time.Now()
	logRec, _ := s.store.CreateSyncLog(ctx, &store.SyncLogRecord{
		SourceID:  sourceID,
		StartedAt: startedAt,
		Result:    "running",
	})

	finish := func(result string, added, updated, disabled int, summary string) {
		finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = s.store.UpdateSourceSyncStatus(finishCtx, sourceID, result, summary, time.Now())
		if logRec != nil {
			_ = s.store.FinishSyncLog(finishCtx, logRec.ID, result, added, updated, disabled, summary, time.Now())
		}
	}

	raw, err := s.fetchSourcePayload(ctx, source.URL)
	if err != nil {
		finish("failed", 0, 0, 0, err.Error())
		return nil, fmt.Errorf("拉取订阅源失败: %w", err)
	}

	parsed, err := ParseSourceAccounts(raw)
	if err != nil {
		finish("failed", 0, 0, 0, err.Error())
		return nil, fmt.Errorf("解析订阅数据失败: %w", err)
	}
	if len(parsed) == 0 {
		finish("failed", 0, 0, 0, "no valid accounts found")
		return nil, fmt.Errorf("订阅源未解析出可用账号")
	}

	existingBySource, err := s.store.ListAccountsBySource(ctx, sourceID)
	if err != nil {
		finish("failed", 0, 0, 0, err.Error())
		return nil, fmt.Errorf("读取现有账号失败: %w", err)
	}
	existingMap := make(map[string]*store.UpstreamAccountRecord, len(existingBySource))
	for _, rec := range existingBySource {
		existingMap[rec.Fingerprint] = rec
	}

	keepSet := make(map[string]struct{}, len(parsed))
	keepFingerprints := make([]string, 0, len(parsed))
	addedCount := 0
	updatedCount := 0

	for i, item := range parsed {
		if strings.TrimSpace(item.Credential) == "" {
			continue
		}
		if strings.TrimSpace(item.AccountName) == "" {
			item.AccountName = fmt.Sprintf("account-%d", i+1)
		}
		if strings.TrimSpace(item.ProviderType) == "" {
			item.ProviderType = accountauth.InferProviderType(item.Credential)
		}
		fp := store.GenerateAccountFingerprint(item.ProviderType, item.Credential, item.BaseURL)
		if _, exists := keepSet[fp]; exists {
			continue
		}
		keepSet[fp] = struct{}{}
		keepFingerprints = append(keepFingerprints, fp)

		rec, exists := existingMap[fp]
		if exists {
			rec.SourceID = &sourceID
			rec.AccountName = item.AccountName
			rec.ProviderType = item.ProviderType
			rec.CredentialRaw = item.Credential
			if item.BaseURL != "" {
				rec.BaseURL = item.BaseURL
			}
			if item.Priority > 0 {
				rec.Priority = item.Priority
			}
			if rec.State != "disabled_auth" {
				rec.Enabled = true
				rec.State = "active"
			}
			rec.Fingerprint = fp
			if updateErr := s.store.UpdateAccount(ctx, rec); updateErr != nil {
				finish("failed", addedCount, updatedCount, 0, updateErr.Error())
				return nil, fmt.Errorf("更新账号失败: %w", updateErr)
			}
			updatedCount++
			continue
		}

		globalExisting, findErr := s.store.FindAccountByFingerprint(ctx, fp)
		if findErr != nil {
			finish("failed", addedCount, updatedCount, 0, findErr.Error())
			return nil, fmt.Errorf("查询账号指纹失败: %w", findErr)
		}
		if globalExisting != nil {
			globalExisting.SourceID = &sourceID
			globalExisting.AccountName = item.AccountName
			globalExisting.ProviderType = item.ProviderType
			globalExisting.CredentialRaw = item.Credential
			if item.BaseURL != "" {
				globalExisting.BaseURL = item.BaseURL
			}
			if item.Priority > 0 {
				globalExisting.Priority = item.Priority
			}
			if globalExisting.State != "disabled_auth" {
				globalExisting.Enabled = true
				globalExisting.State = "active"
			}
			globalExisting.Fingerprint = fp
			if updateErr := s.store.UpdateAccount(ctx, globalExisting); updateErr != nil {
				finish("failed", addedCount, updatedCount, 0, updateErr.Error())
				return nil, fmt.Errorf("更新账号失败: %w", updateErr)
			}
			updatedCount++
			continue
		}

		priority := item.Priority
		if priority <= 0 {
			priority = 100 + i
		}
		_, createErr := s.store.CreateAccount(ctx, &store.UpstreamAccountRecord{
			SourceID:      &sourceID,
			ProviderType:  item.ProviderType,
			AccountName:   item.AccountName,
			CredentialRaw: item.Credential,
			BaseURL:       item.BaseURL,
			Priority:      priority,
			Enabled:       true,
			State:         "active",
			Fingerprint:   fp,
		})
		if createErr != nil {
			finish("failed", addedCount, updatedCount, 0, createErr.Error())
			return nil, fmt.Errorf("创建账号失败: %w", createErr)
		}
		addedCount++
	}

	disabledCount, err := s.store.DisableAccountsBySourceExcept(ctx, sourceID, keepFingerprints)
	if err != nil {
		finish("failed", addedCount, updatedCount, 0, err.Error())
		return nil, fmt.Errorf("禁用旧账号失败: %w", err)
	}

	finish("success", addedCount, updatedCount, disabledCount, "")
	return &SyncSubscriptionSourceResult{
		SourceID: sourceID,
		Added:    addedCount,
		Updated:  updatedCount,
		Disabled: disabledCount,
	}, nil
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
	return s.store.UpdateAccount(ctx, rec)
}

func (s *AccountPoolService) DeleteAccount(ctx context.Context, id int64) error {
	return s.store.DeleteAccount(ctx, id)
}

func (s *AccountPoolService) ToggleAccount(ctx context.Context, id int64, enabled bool) error {
	return s.store.ToggleAccount(ctx, id, enabled)
}

// ===== 调度/状态回写（供 proxy 使用） =====

func (s *AccountPoolService) ListSchedulableAccounts(ctx context.Context) ([]*store.UpstreamAccountRecord, error) {
	return s.store.ListSchedulableAccounts(ctx, time.Now())
}

func (s *AccountPoolService) MarkAccountSuccess(ctx context.Context, id int64) error {
	return s.store.MarkAccountSuccess(ctx, id, time.Now())
}

func (s *AccountPoolService) MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error {
	return s.store.MarkAccountAuthFailed(ctx, id, reason)
}

func (s *AccountPoolService) MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldown time.Duration) error {
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return s.store.MarkAccountTransientFailure(ctx, id, reason, time.Now().Add(cooldown))
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
		targetURL = defaultChatGPTCodexTestURL
	} else {
		base := strings.TrimSuffix(strings.TrimSpace(acc.BaseURL), "/")
		if base == "" {
			base = "https://api.openai.com"
		}
		targetURL = base + "/v1/responses"
	}

	// 连通性测试按账号类型走实际上游（OAuth: chatgpt codex，API Key: /v1/responses）。
	// 使用极简 payload：鉴权/权限通过时通常返回 400（缺少 model），仍可判定“账号可达”。
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

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	bodyText := strings.TrimSpace(string(body))

	switch resp.StatusCode {
	case http.StatusBadRequest:
		// 账号常见可用场景：鉴权成功但 payload 不完整（例如缺少 model）会返回 400。
		return nil
	case http.StatusTooManyRequests:
		// 账号可达但被限流，不作为鉴权失败。
		return nil
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

func (s *AccountPoolService) fetchSourcePayload(ctx context.Context, sourceURL string) ([]byte, error) {
	client, err := s.buildHTTPClient(20 * time.Second)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(sourceURL), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
}

// ParseSourceAccounts 解析订阅源返回内容
func ParseSourceAccounts(raw []byte) ([]ParsedUpstreamAccount, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, fmt.Errorf("订阅内容为空")
	}

	if accounts, ok := parseJSONAccounts(text); ok && len(accounts) > 0 {
		return accounts, nil
	}
	if accounts := parsePlainTextAccounts(text); len(accounts) > 0 {
		return accounts, nil
	}
	return nil, fmt.Errorf("无法识别的订阅格式")
}

func parseJSONAccounts(text string) ([]ParsedUpstreamAccount, bool) {
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, false
	}
	accounts := collectAccounts(raw)
	return normalizeParsedAccounts(accounts), true
}

func collectAccounts(raw any) []ParsedUpstreamAccount {
	switch v := raw.(type) {
	case []any:
		out := make([]ParsedUpstreamAccount, 0, len(v))
		for _, item := range v {
			if acc, ok := parseAccountFromAny(item); ok {
				out = append(out, acc)
			}
		}
		return out
	case map[string]any:
		for _, key := range []string{"accounts", "data", "items", "list"} {
			if nested, ok := v[key]; ok {
				if out := collectAccounts(nested); len(out) > 0 {
					return out
				}
			}
		}
		if acc, ok := parseAccountFromMap(v); ok {
			return []ParsedUpstreamAccount{acc}
		}
	}
	return nil
}

func parseAccountFromAny(raw any) (ParsedUpstreamAccount, bool) {
	switch v := raw.(type) {
	case map[string]any:
		return parseAccountFromMap(v)
	default:
		return ParsedUpstreamAccount{}, false
	}
}

func parseAccountFromMap(m map[string]any) (ParsedUpstreamAccount, bool) {
	getStr := func(keys ...string) string {
		for _, key := range keys {
			if val, ok := m[key]; ok {
				if str, ok := val.(string); ok {
					trim := strings.TrimSpace(str)
					if trim != "" {
						return trim
					}
				}
			}
		}
		return ""
	}
	getInt := func(keys ...string) int {
		for _, key := range keys {
			if val, ok := m[key]; ok {
				switch num := val.(type) {
				case float64:
					return int(num)
				case int:
					return num
				case string:
					if parsed, err := strconv.Atoi(strings.TrimSpace(num)); err == nil {
						return parsed
					}
				}
			}
		}
		return 0
	}

	credential := getStr("credential_raw", "credential", "api_key", "token", "refresh_token", "rt", "session_cookie", "cookie")
	if credential == "" {
		if nested, ok := m["credentials"].(map[string]any); ok {
			credential = stringFromMap(nested, "credential_raw", "credential", "api_key", "token", "refresh_token", "rt", "session_cookie", "cookie")
		}
	}
	if credential == "" {
		return ParsedUpstreamAccount{}, false
	}

	provider := getStr("provider_type", "provider", "auth_type", "type")
	if provider == "" {
		provider = accountauth.InferProviderType(credential)
	}
	baseURL := getStr("base_url", "upstream_url", "url")
	accountName := getStr("account_name", "name", "account")
	priority := getInt("priority")

	return ParsedUpstreamAccount{
		AccountName:  accountName,
		ProviderType: provider,
		Credential:   credential,
		BaseURL:      baseURL,
		Priority:     priority,
	}, true
}

func parsePlainTextAccounts(text string) []ParsedUpstreamAccount {
	lines := strings.Split(text, "\n")
	out := make([]ParsedUpstreamAccount, 0, len(lines))
	for idx, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}

		var acc ParsedUpstreamAccount
		if strings.Contains(trim, ",") || strings.Contains(trim, "|") {
			delimiter := ","
			if strings.Contains(trim, "|") {
				delimiter = "|"
			}
			parts := splitAndTrim(trim, delimiter)
			switch {
			case len(parts) >= 4:
				acc = ParsedUpstreamAccount{
					AccountName:  parts[0],
					ProviderType: accountauth.NormalizeProviderType(parts[1]),
					Credential:   parts[2],
					BaseURL:      parts[3],
				}
				if len(parts) >= 5 {
					if p, err := strconv.Atoi(parts[4]); err == nil {
						acc.Priority = p
					}
				}
			case len(parts) == 3:
				acc = ParsedUpstreamAccount{
					AccountName:  parts[0],
					ProviderType: accountauth.NormalizeProviderType(parts[1]),
					Credential:   parts[2],
				}
			case len(parts) == 2:
				acc = ParsedUpstreamAccount{
					AccountName: parts[0],
					Credential:  parts[1],
				}
			case len(parts) == 1:
				acc = ParsedUpstreamAccount{
					Credential: parts[0],
				}
			}
		} else {
			acc = ParsedUpstreamAccount{Credential: trim}
		}

		if strings.TrimSpace(acc.Credential) == "" {
			continue
		}
		if strings.TrimSpace(acc.AccountName) == "" {
			acc.AccountName = fmt.Sprintf("account-%d", idx+1)
		}
		if strings.TrimSpace(acc.ProviderType) == "" {
			acc.ProviderType = accountauth.InferProviderType(acc.Credential)
		}
		out = append(out, acc)
	}
	return normalizeParsedAccounts(out)
}

func normalizeParsedAccounts(in []ParsedUpstreamAccount) []ParsedUpstreamAccount {
	out := make([]ParsedUpstreamAccount, 0, len(in))
	for _, item := range in {
		cred := strings.TrimSpace(item.Credential)
		if cred == "" {
			continue
		}
		provider := accountauth.NormalizeProviderType(item.ProviderType)
		if provider == "" {
			provider = accountauth.InferProviderType(cred)
		}
		accountName := strings.TrimSpace(item.AccountName)
		if accountName == "" {
			accountName = "unnamed-account"
		}
		baseURL := strings.TrimSpace(item.BaseURL)
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		out = append(out, ParsedUpstreamAccount{
			AccountName:  accountName,
			ProviderType: provider,
			Credential:   cred,
			BaseURL:      strings.TrimSuffix(baseURL, "/"),
			Priority:     item.Priority,
		})
	}
	return out
}

func splitAndTrim(line, delimiter string) []string {
	parts := strings.Split(line, delimiter)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trim := strings.TrimSpace(p)
		if trim != "" {
			out = append(out, trim)
		}
	}
	return out
}

func stringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if val, ok := m[key]; ok {
			if str, ok := val.(string); ok {
				trim := strings.TrimSpace(str)
				if trim != "" {
					return trim
				}
			}
		}
	}
	return ""
}
