package proxy

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/accountauth"
	"cc-forwarder/internal/endpoint"
	servicepkg "cc-forwarder/internal/service"
	"cc-forwarder/internal/store"

	_ "modernc.org/sqlite"
)

type accountTransientCall struct {
	id       int64
	reason   string
	cooldown time.Duration
}

type accountAuthCall struct {
	id     int64
	reason string
}

type accountSoftFailureCall struct {
	id         int64
	reason     string
	category   string
	retryAfter time.Duration
}

type accountUsageLimitCall struct {
	id       int64
	reason   string
	planType string
	resetAt  time.Time
}

type accountSchedulePrepareCall struct {
	requestID   string
	requestPath string
}

type accountScheduleCompleteCall struct {
	requestID   string
	accountID   int64
	accountName string
	outcome     string
	finalError  string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type mockAccountPoolService struct {
	accounts []*store.UpstreamAccountRecord
	listErr  error

	mu                sync.Mutex
	previewCalls      []string
	prepareCalls      []accountSchedulePrepareCall
	completeCalls     []accountScheduleCompleteCall
	completeCtxErrs   []error
	quotaRefreshCalls []int64
	successCalls      []int64
	successGuardTimes []time.Time
	successCtxErrs    []error
	authFailCalls     []accountAuthCall
	transientCalls    []accountTransientCall
	softFailureCalls  []accountSoftFailureCall
	usageLimitCalls   []accountUsageLimitCall

	softFailureShouldFailover *bool
}

func (m *mockAccountPoolService) PrepareSchedulableAccounts(ctx context.Context, requestID, requestPath string) ([]*store.UpstreamAccountRecord, error) {
	m.mu.Lock()
	m.prepareCalls = append(m.prepareCalls, accountSchedulePrepareCall{requestID: requestID, requestPath: requestPath})
	m.mu.Unlock()
	return m.ListSchedulableAccounts(ctx)
}

func (m *mockAccountPoolService) PreviewSchedulableAccounts(ctx context.Context, requestPath string) ([]*store.UpstreamAccountRecord, error) {
	m.mu.Lock()
	m.previewCalls = append(m.previewCalls, requestPath)
	m.mu.Unlock()
	return m.ListSchedulableAccounts(ctx)
}

func (m *mockAccountPoolService) ListSchedulableAccounts(ctx context.Context) ([]*store.UpstreamAccountRecord, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	out := make([]*store.UpstreamAccountRecord, 0, len(m.accounts))
	for _, item := range m.accounts {
		cp := *item
		out = append(out, &cp)
	}
	return out, nil
}

func (m *mockAccountPoolService) CompleteLatestScheduleSnapshot(ctx context.Context, requestID string, accountID int64, accountName, outcome, finalError string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completeCtxErrs = append(m.completeCtxErrs, ctx.Err())
	m.completeCalls = append(m.completeCalls, accountScheduleCompleteCall{
		requestID:   requestID,
		accountID:   accountID,
		accountName: accountName,
		outcome:     outcome,
		finalError:  finalError,
	})
	return nil
}

func (m *mockAccountPoolService) TryEnqueueQuotaRefresh(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quotaRefreshCalls = append(m.quotaRefreshCalls, id)
	return true
}

func (m *mockAccountPoolService) MarkAccountSuccess(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.successCtxErrs = append(m.successCtxErrs, ctx.Err())
	m.successCalls = append(m.successCalls, id)
	return nil
}

func (m *mockAccountPoolService) MarkAccountSuccessIfNoNewerFailure(ctx context.Context, id int64, attemptStartedAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.successCtxErrs = append(m.successCtxErrs, ctx.Err())
	m.successCalls = append(m.successCalls, id)
	m.successGuardTimes = append(m.successGuardTimes, attemptStartedAt)
	return true, nil
}

func (m *mockAccountPoolService) MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authFailCalls = append(m.authFailCalls, accountAuthCall{id: id, reason: reason})
	return nil
}

func (m *mockAccountPoolService) MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldown time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transientCalls = append(m.transientCalls, accountTransientCall{id: id, reason: reason, cooldown: cooldown})
	return nil
}

func (m *mockAccountPoolService) MarkAccountUsageLimitExceeded(ctx context.Context, id int64, reason, planType string, resetAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usageLimitCalls = append(m.usageLimitCalls, accountUsageLimitCall{id: id, reason: reason, planType: planType, resetAt: resetAt})
	return nil
}

func (m *mockAccountPoolService) RecordAccountSoftFailure(ctx context.Context, id int64, reason, category string, retryAfter time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.softFailureCalls = append(m.softFailureCalls, accountSoftFailureCall{id: id, reason: reason, category: category, retryAfter: retryAfter})
	if m.softFailureShouldFailover != nil {
		return *m.softFailureShouldFailover, nil
	}
	return true, nil
}

func newAccountPipelineTestHandlerWithEnabled(t *testing.T, fallbackURL string, accountService AccountPoolService, enabled bool) *Handler {
	t.Helper()

	cfg := &config.Config{
		AccountPool: config.AccountPoolConfig{
			Enabled: enabled,
		},
		Streaming: config.StreamingConfig{
			ResponseHeaderTimeout: 5 * time.Second,
		},
		GlobalTimeout: 5 * time.Second,
		Endpoints: []config.EndpointConfig{
			{
				Name:     "fallback-endpoint",
				URL:      fallbackURL,
				Priority: 1,
				Timeout:  30 * time.Second,
				Token:    "fallback-token",
			},
		},
	}

	endpointManager := endpoint.NewManager(cfg)
	handler := NewHandler(endpointManager, cfg)
	handler.SetAccountPoolService(accountService)
	return handler
}

func newAccountPipelineTestHandler(t *testing.T, fallbackURL string, accountService AccountPoolService) *Handler {
	t.Helper()
	return newAccountPipelineTestHandlerWithEnabled(t, fallbackURL, accountService, true)
}

func performResponsesRequest(t *testing.T, handler *Handler) *httptest.ResponseRecorder {
	t.Helper()
	body := bytes.NewBufferString(`{"model":"gpt-4.1","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func performResponsesCompactRequest(t *testing.T, handler *Handler) *httptest.ResponseRecorder {
	t.Helper()
	body := bytes.NewBufferString(`{"model":"gpt-4.1","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func performResponsesStreamingRequest(t *testing.T, handler *Handler) *httptest.ResponseRecorder {
	t.Helper()
	body := bytes.NewBufferString(`{"model":"gpt-4.1","input":"hello","stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func newRealAccountPipelineTestService(t *testing.T, accounts []*store.UpstreamAccountRecord) *servicepkg.AccountPoolService {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	schemaPath := filepath.Join("..", "tracking", "schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema failed: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("exec schema failed: %v", err)
	}

	st := store.NewSQLiteAccountPoolStore(db)
	svc := servicepkg.NewAccountPoolService(st, nil)
	t.Cleanup(func() { _ = svc.Close() })

	for _, account := range accounts {
		if account == nil {
			continue
		}
		if _, err := svc.CreateAccount(context.Background(), account); err != nil {
			t.Fatalf("create account failed: %v", err)
		}
	}

	return svc
}

func TestAccountPipeline_NoEndpointFallbackWhenAccountPoolEmpty(t *testing.T) {
	fallbackHits := 0
	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer fallbackServer.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{},
	}
	handler := newAccountPipelineTestHandler(t, fallbackServer.URL, service)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
	if fallbackHits != 0 {
		t.Fatalf("expected endpoint fallback not to be called, got %d calls", fallbackHits)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj["type"] != "account_pool_unavailable" {
		t.Fatalf("unexpected error type: %#v", errObj["type"])
	}
}

func TestAccountPipeline_DoesNotUseEndpointWhenAccountPoolDisabled(t *testing.T) {
	fallbackHits := 0
	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer fallbackServer.Close()

	handler := newAccountPipelineTestHandlerWithEnabled(t, fallbackServer.URL, nil, false)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
	if fallbackHits != 0 {
		t.Fatalf("expected endpoint fallback not to be called, got %d calls", fallbackHits)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj["type"] != "account_pool_disabled" {
		t.Fatalf("unexpected error type: %#v", errObj["type"])
	}
	if errObj["message"] != "account pool is disabled for Codex /v1/responses and /v1/responses/compact" {
		t.Fatalf("unexpected error message: %#v", errObj["message"])
	}
}

func TestAccountPipeline_DoesNotUseEndpointWhenServiceNotReady(t *testing.T) {
	fallbackHits := 0
	fallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer fallbackServer.Close()

	handler := newAccountPipelineTestHandlerWithEnabled(t, fallbackServer.URL, nil, true)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
	if fallbackHits != 0 {
		t.Fatalf("expected endpoint fallback not to be called, got %d calls", fallbackHits)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	errObj, _ := payload["error"].(map[string]any)
	if errObj["type"] != "account_pool_unavailable" {
		t.Fatalf("unexpected error type: %#v", errObj["type"])
	}
	if errObj["message"] != "account pool service is not initialized for Codex /v1/responses and /v1/responses/compact" {
		t.Fatalf("unexpected error message: %#v", errObj["message"])
	}
}

func TestShouldUseAccountPipeline_RequiresEnabledAndInitializedService(t *testing.T) {
	handler := &Handler{
		config: &config.Config{
			AccountPool: config.AccountPoolConfig{Enabled: true},
		},
	}
	if handler.shouldUseAccountPipeline("/v1/responses") {
		t.Fatal("expected disabled routing when service is nil")
	}

	handler.accountPoolService = &mockAccountPoolService{}
	if !handler.shouldUseAccountPipeline("/v1/responses") {
		t.Fatal("expected account pipeline to be enabled when service is ready")
	}

	handler.config.AccountPool.Enabled = false
	if handler.shouldUseAccountPipeline("/v1/responses") {
		t.Fatal("expected disabled routing when account_pool.enabled=false")
	}
}

func TestShouldUseAccountPipeline_SupportsResponsesCompactPath(t *testing.T) {
	handler := &Handler{
		config: &config.Config{
			AccountPool: config.AccountPoolConfig{Enabled: true},
		},
		accountPoolService: &mockAccountPoolService{},
	}

	if !handler.shouldUseAccountPipeline("/v1/responses/compact") {
		t.Fatal("expected /v1/responses/compact to use account pipeline")
	}

	if !handler.isAccountPipelinePath("/v1/responses/compact") {
		t.Fatal("expected /v1/responses/compact to be treated as account pipeline path")
	}
}

func TestHandler_GetAccountHTTPClient_ReusesSharedTransport(t *testing.T) {
	handler := newAccountPipelineTestHandlerWithEnabled(t, "https://example.com", nil, true)

	regularClient1, err := handler.getAccountHTTPClient(false)
	if err != nil {
		t.Fatalf("getAccountHTTPClient(false) failed: %v", err)
	}
	regularClient2, err := handler.getAccountHTTPClient(false)
	if err != nil {
		t.Fatalf("second getAccountHTTPClient(false) failed: %v", err)
	}
	sseClient, err := handler.getAccountHTTPClient(true)
	if err != nil {
		t.Fatalf("getAccountHTTPClient(true) failed: %v", err)
	}

	if regularClient1 != regularClient2 {
		t.Fatal("expected regular account client to be reused")
	}
	if regularClient1.Transport == nil || sseClient.Transport == nil {
		t.Fatal("expected shared transport to be initialized")
	}
	if regularClient1.Transport != sseClient.Transport {
		t.Fatal("expected regular and SSE clients to share the same transport")
	}
	httpTransport, ok := regularClient1.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected shared transport to be *http.Transport, got %T", regularClient1.Transport)
	}
	if httpTransport.MaxIdleConnsPerHost != 16 {
		t.Fatalf("expected MaxIdleConnsPerHost=16, got %d", httpTransport.MaxIdleConnsPerHost)
	}
	if httpTransport.MaxConnsPerHost != 32 {
		t.Fatalf("expected MaxConnsPerHost=32, got %d", httpTransport.MaxConnsPerHost)
	}
	if regularClient1.Timeout <= 0 {
		t.Fatalf("expected regular client timeout to be set, got %v", regularClient1.Timeout)
	}
	if sseClient.Timeout != 0 {
		t.Fatalf("expected SSE client timeout to be disabled, got %v", sseClient.Timeout)
	}
}

func TestAccountPipeline_PreparesAndCompletesLatestScheduleSnapshot(t *testing.T) {
	upstreamHits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_snapshot","status":"completed","output":[]}`))
	}))
	defer upstream.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 7, AccountName: "snapshot-main", ProviderType: "api_key", CredentialRaw: "sk-snapshot", BaseURL: upstream.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, upstream.URL, service)
	handler.accountHTTPInitOnce.Do(func() {})
	handler.accountHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		upstreamHits++
		body := io.NopCloser(strings.NewReader(`{"id":"resp_oauth","status":"completed","output":[]}`))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       body,
			Request:    req,
		}, nil
	})}
	handler.accountSSEHTTPClient = handler.accountHTTPClient

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamHits != 1 {
		t.Fatalf("expected upstream to be called once, got %d", upstreamHits)
	}
	if len(service.prepareCalls) != 1 {
		t.Fatalf("expected one prepare call, got %+v", service.prepareCalls)
	}
	if service.prepareCalls[0].requestPath != "/v1/responses" {
		t.Fatalf("expected request path /v1/responses, got %+v", service.prepareCalls[0])
	}
	if service.prepareCalls[0].requestID == "" {
		t.Fatalf("expected non-empty request id in prepare call, got %+v", service.prepareCalls[0])
	}
	if len(service.completeCalls) == 0 {
		t.Fatalf("expected complete call, got %+v", service.completeCalls)
	}
	last := service.completeCalls[len(service.completeCalls)-1]
	if last.outcome != servicepkg.AccountScheduleOutcomeSuccess {
		t.Fatalf("expected success outcome, got %+v", last)
	}
	if last.accountID != 7 || last.accountName != "snapshot-main" {
		t.Fatalf("expected selected account to be recorded, got %+v", last)
	}
	if last.requestID == "" {
		t.Fatalf("expected request id in complete call, got %+v", last)
	}
	if len(service.quotaRefreshCalls) != 0 {
		t.Fatalf("expected api_key success not to enqueue quota refresh, got %+v", service.quotaRefreshCalls)
	}
}

func TestAccountPipeline_PreparesCompactRequestPath(t *testing.T) {
	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 8, AccountName: "compact-main", ProviderType: "api_key", CredentialRaw: "sk-compact", BaseURL: "https://example.com", Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, "https://example.com", service)
	handler.accountHTTPInitOnce.Do(func() {})
	handler.accountHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/responses/compact" {
			t.Fatalf("expected upstream path /v1/responses/compact, got %s", req.URL.Path)
		}
		body := io.NopCloser(strings.NewReader(`{"output":[]}`))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       body,
			Request:    req,
		}, nil
	})}
	handler.accountSSEHTTPClient = handler.accountHTTPClient

	rec := performResponsesCompactRequest(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(service.prepareCalls) != 1 {
		t.Fatalf("expected one prepare call, got %+v", service.prepareCalls)
	}
	if service.prepareCalls[0].requestPath != "/v1/responses/compact" {
		t.Fatalf("expected request path /v1/responses/compact, got %+v", service.prepareCalls[0])
	}
}

func TestAccountPipeline_AnyRouterRewritesUnsupportedCodexModelBeforeForward(t *testing.T) {
	var receivedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream body failed: %v", err)
		}
		receivedModel, _ = payload["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","model":"gpt-5.5","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 9, AccountName: "anyrouter", ProviderType: "api_key", CredentialRaw: "sk-anyrouter", BaseURL: upstream.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, upstream.URL, service)

	body := bytes.NewBufferString(`{"model":"gpt-5.4-mini-2026-03-17","input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if receivedModel != "gpt-5.5" {
		t.Fatalf("expected upstream model gpt-5.5, got %q", receivedModel)
	}
}

func TestAccountPipeline_OAuthSuccessEnqueuesQuotaRefresh(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-oauth","refresh_token":"rt-oauth","expires_in":3600}`))
	}))
	defer authServer.Close()

	oldRefreshURL := accountauth.CurrentOpenAIRefreshTokenURLForTest()
	t.Cleanup(func() {
		accountauth.SetOpenAIRefreshTokenURLForTest(oldRefreshURL)
	})
	accountauth.SetOpenAIRefreshTokenURLForTest(authServer.URL)

	upstreamHits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_oauth","status":"completed","output":[]}`))
	}))
	defer upstream.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 17, AccountName: "oauth-main", ProviderType: "chatgpt_refresh_token", CredentialRaw: `{"refresh_token":"rt-1","chatgpt_account_id":"acc-1"}`, BaseURL: upstream.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, upstream.URL, service)
	handler.accountHTTPInitOnce.Do(func() {})
	handler.accountHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		upstreamHits++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_oauth","status":"completed","output":[]}`)),
			Request:    req,
		}, nil
	})}
	handler.accountSSEHTTPClient = handler.accountHTTPClient

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamHits != 1 {
		t.Fatalf("expected upstream to be called once, got %d", upstreamHits)
	}
	if len(service.quotaRefreshCalls) != 1 || service.quotaRefreshCalls[0] != 17 {
		t.Fatalf("expected oauth success to enqueue quota refresh, got %+v", service.quotaRefreshCalls)
	}
}

func TestAccountPipeline_429SoftFailure_RespectsServiceNoFailoverDecision(t *testing.T) {
	firstHits := 0
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
	}))
	defer firstServer.Close()

	secondHits := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_2","status":"completed","output":[]}`))
	}))
	defer secondServer.Close()

	shouldFailover := false
	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 11, AccountName: "acc-a", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: firstServer.URL, Enabled: true},
			{ID: 12, AccountName: "acc-b", ProviderType: "api_key", CredentialRaw: "sk-b", BaseURL: secondServer.URL, Enabled: true},
		},
		softFailureShouldFailover: &shouldFailover,
	}
	handler := newAccountPipelineTestHandler(t, firstServer.URL, service)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected first 429 soft failure to stop on active account, got %d body=%s", rec.Code, rec.Body.String())
	}
	if firstHits != 1 || secondHits != 0 {
		t.Fatalf("expected backup account not to be attempted when service keeps active account, got first=%d second=%d", firstHits, secondHits)
	}

	if len(service.softFailureCalls) != 1 || service.softFailureCalls[0].id != 11 {
		t.Fatalf("expected soft failure on account 11, got %+v", service.softFailureCalls)
	}
	if service.softFailureCalls[0].category != accountSoftFailureCategoryRateLimit {
		t.Fatalf("expected rate_limit soft failure, got %+v", service.softFailureCalls[0])
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no direct transient cooldown for first 429, got %+v", service.transientCalls)
	}
	if len(service.successCalls) != 0 {
		t.Fatalf("expected no backup success while service keeps active account, got %+v", service.successCalls)
	}
	if len(service.authFailCalls) != 0 {
		t.Fatalf("expected no auth-failed call, got %+v", service.authFailCalls)
	}
}

func TestAccountPipeline_UsesServiceReturnedOrderWithoutPriorityReordering(t *testing.T) {
	firstHits := 0
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
	}))
	defer firstServer.Close()

	secondHits := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_service_order","status":"completed","output":[]}`))
	}))
	defer secondServer.Close()

	shouldFailover := false
	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 41, AccountName: "manual-backup", ProviderType: "api_key", CredentialRaw: "sk-backup", BaseURL: firstServer.URL, Priority: 20, Enabled: true},
			{ID: 42, AccountName: "manual-main", ProviderType: "api_key", CredentialRaw: "sk-main", BaseURL: secondServer.URL, Priority: 10, Enabled: true},
		},
		softFailureShouldFailover: &shouldFailover,
	}
	handler := newAccountPipelineTestHandler(t, firstServer.URL, service)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected first returned account to remain active when service rejects failover, got %d body=%s", rec.Code, rec.Body.String())
	}
	if firstHits != 1 || secondHits != 0 {
		t.Fatalf("expected proxy to honor returned order without trying second account, got first=%d second=%d", firstHits, secondHits)
	}
	if len(service.softFailureCalls) != 1 || service.softFailureCalls[0].id != 41 {
		t.Fatalf("expected first returned account to record soft failure, got %+v", service.softFailureCalls)
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no immediate cooldown for first returned account, got %+v", service.transientCalls)
	}
	if len(service.successCalls) != 0 {
		t.Fatalf("expected second returned account not to be attempted, got %+v", service.successCalls)
	}
}

func TestAccountPipeline_UsageLimitReachedUsesResetTimeCooldown(t *testing.T) {
	resetAt := time.Now().Add(90 * time.Minute).Round(time.Second)

	firstHits := 0
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, fmt.Sprintf(`{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","plan_type":"free","resets_at":%d,"resets_in_seconds":5400}}`, resetAt.Unix()), http.StatusTooManyRequests)
	}))
	defer firstServer.Close()

	secondHits := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_after_usage_limit","status":"completed","output":[]}`))
	}))
	defer secondServer.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 61, AccountName: "acc-usage-limit", ProviderType: "api_key", CredentialRaw: "sk-usage-limit", BaseURL: firstServer.URL, Enabled: true},
			{ID: 62, AccountName: "acc-fallback", ProviderType: "api_key", CredentialRaw: "sk-fallback", BaseURL: secondServer.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, firstServer.URL, service)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if firstHits != 1 || secondHits != 1 {
		t.Fatalf("expected both accounts attempted once, got first=%d second=%d", firstHits, secondHits)
	}
	if len(service.usageLimitCalls) != 1 || service.usageLimitCalls[0].id != 61 {
		t.Fatalf("expected usage limit call on account 61, got %+v", service.usageLimitCalls)
	}
	if service.usageLimitCalls[0].planType != "free" {
		t.Fatalf("expected free plan type, got %+v", service.usageLimitCalls[0])
	}
	if got := service.usageLimitCalls[0].resetAt.Unix(); got != resetAt.Unix() {
		t.Fatalf("expected resetAt=%d, got %d", resetAt.Unix(), got)
	}
	if len(service.softFailureCalls) != 0 {
		t.Fatalf("expected no soft failure calls, got %+v", service.softFailureCalls)
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no generic transient failure calls, got %+v", service.transientCalls)
	}
	if len(service.successCalls) != 1 || service.successCalls[0] != 62 {
		t.Fatalf("expected fallback account success, got %+v", service.successCalls)
	}
}

func TestAccountPipeline_Local503NoAvailableProviders_PassthroughWithoutCooldown(t *testing.T) {
	firstHits := 0
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":{"type":"no_available_providers","message":"no_available_providers::ccf_local"}}`, http.StatusServiceUnavailable)
	}))
	defer firstServer.Close()

	secondHits := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"should_not_run"}`))
	}))
	defer secondServer.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 13, AccountName: "acc-a", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: firstServer.URL, Enabled: true},
			{ID: 14, AccountName: "acc-b", ProviderType: "api_key", CredentialRaw: "sk-b", BaseURL: secondServer.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, firstServer.URL, service)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	if firstHits != 1 || secondHits != 0 {
		t.Fatalf("expected only first account attempted, got first=%d second=%d", firstHits, secondHits)
	}
	if !strings.Contains(rec.Body.String(), "no_available_providers") {
		t.Fatalf("expected passthrough body to contain no_available_providers, got %s", rec.Body.String())
	}
	if len(service.softFailureCalls) != 0 {
		t.Fatalf("expected no soft failure calls, got %+v", service.softFailureCalls)
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no transient failure calls, got %+v", service.transientCalls)
	}
	if len(service.authFailCalls) != 0 {
		t.Fatalf("expected no auth-failed calls, got %+v", service.authFailCalls)
	}
	if len(service.successCalls) != 0 {
		t.Fatalf("expected no success calls, got %+v", service.successCalls)
	}
}

func TestAccountPipeline_Generic503RecordedAsServerSoftFailure(t *testing.T) {
	firstHits := 0
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":{"type":"service_unavailable","message":"upstream overloaded"}}`, http.StatusServiceUnavailable)
	}))
	defer firstServer.Close()

	secondHits := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_after_503","status":"completed","output":[]}`))
	}))
	defer secondServer.Close()

	shouldFailover := false
	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 51, AccountName: "acc-a", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: firstServer.URL, Enabled: true},
			{ID: 52, AccountName: "acc-b", ProviderType: "api_key", CredentialRaw: "sk-b", BaseURL: secondServer.URL, Enabled: true},
		},
		softFailureShouldFailover: &shouldFailover,
	}
	handler := newAccountPipelineTestHandler(t, firstServer.URL, service)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected generic 503 soft failure to stay on active account when service rejects failover, got %d body=%s", rec.Code, rec.Body.String())
	}
	if firstHits != 1 || secondHits != 0 {
		t.Fatalf("expected no backup attempt on first generic 503 soft failure, got first=%d second=%d", firstHits, secondHits)
	}
	if len(service.softFailureCalls) != 1 || service.softFailureCalls[0].id != 51 {
		t.Fatalf("expected soft failure for first account, got %+v", service.softFailureCalls)
	}
	if service.softFailureCalls[0].category != accountSoftFailureCategoryServerError {
		t.Fatalf("expected server_error soft failure, got %+v", service.softFailureCalls[0])
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no immediate transient cooldown, got %+v", service.transientCalls)
	}
	if len(service.successCalls) != 0 {
		t.Fatalf("expected second account not to be attempted on first generic 503, got %+v", service.successCalls)
	}
}

func TestAccountPipeline_ServerSoftFailure_DoesNotFailOverBeforeThirdConsecutiveFailure(t *testing.T) {
	firstHits := 0
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":{"type":"service_unavailable","message":"upstream overloaded"}}`, http.StatusServiceUnavailable)
	}))
	defer firstServer.Close()

	secondHits := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_after_threshold","status":"completed","output":[]}`))
	}))
	defer secondServer.Close()

	service := newRealAccountPipelineTestService(t, []*store.UpstreamAccountRecord{
		{ProviderType: "api_key", AccountName: "active-main", CredentialRaw: "sk-main", BaseURL: firstServer.URL, Priority: 10, Enabled: true, State: "active"},
		{ProviderType: "api_key", AccountName: "same-tier-backup", CredentialRaw: "sk-backup", BaseURL: secondServer.URL, Priority: 10, Enabled: true, State: "active"},
	})
	handler := newAccountPipelineTestHandler(t, firstServer.URL, service)

	firstAttempt := performResponsesRequest(t, handler)
	if firstAttempt.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected first soft failure to stop without failover, got %d body=%s", firstAttempt.Code, firstAttempt.Body.String())
	}
	if firstHits != 1 || secondHits != 0 {
		t.Fatalf("expected only active account attempted on first soft failure, got first=%d second=%d", firstHits, secondHits)
	}

	secondAttempt := performResponsesRequest(t, handler)
	if secondAttempt.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected second consecutive soft failure to stop without failover, got %d body=%s", secondAttempt.Code, secondAttempt.Body.String())
	}
	if firstHits != 2 || secondHits != 0 {
		t.Fatalf("expected backup account not attempted before threshold, got first=%d second=%d", firstHits, secondHits)
	}

	thirdAttempt := performResponsesRequest(t, handler)
	if thirdAttempt.Code != http.StatusOK {
		t.Fatalf("expected third consecutive soft failure to fail over, got %d body=%s", thirdAttempt.Code, thirdAttempt.Body.String())
	}
	if firstHits != 3 || secondHits != 1 {
		t.Fatalf("expected backup to be attempted only after threshold, got first=%d second=%d", firstHits, secondHits)
	}
}

func TestAccountPipeline_SuccessResetsTransientFailureThreshold(t *testing.T) {
	var firstHits int
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		w.Header().Set("Content-Type", "application/json")
		switch firstHits {
		case 2:
			_, _ = w.Write([]byte(`{"id":"resp_recovered","status":"completed","output":[]}`))
		default:
			http.Error(w, `{"error":{"type":"service_unavailable","message":"upstream overloaded"}}`, http.StatusServiceUnavailable)
		}
	}))
	defer firstServer.Close()

	secondHits := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_after_reset","status":"completed","output":[]}`))
	}))
	defer secondServer.Close()

	service := newRealAccountPipelineTestService(t, []*store.UpstreamAccountRecord{
		{ProviderType: "api_key", AccountName: "active-main", CredentialRaw: "sk-main", BaseURL: firstServer.URL, Priority: 10, Enabled: true, State: "active"},
		{ProviderType: "api_key", AccountName: "same-tier-backup", CredentialRaw: "sk-backup", BaseURL: secondServer.URL, Priority: 10, Enabled: true, State: "active"},
	})
	handler := newAccountPipelineTestHandler(t, firstServer.URL, service)

	firstAttempt := performResponsesRequest(t, handler)
	if firstAttempt.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected initial transient failure to stay on active account, got %d body=%s", firstAttempt.Code, firstAttempt.Body.String())
	}
	if firstHits != 1 || secondHits != 0 {
		t.Fatalf("expected no failover on first transient failure, got first=%d second=%d", firstHits, secondHits)
	}

	secondAttempt := performResponsesRequest(t, handler)
	if secondAttempt.Code != http.StatusOK {
		t.Fatalf("expected active account recovery success, got %d body=%s", secondAttempt.Code, secondAttempt.Body.String())
	}
	if firstHits != 2 || secondHits != 0 {
		t.Fatalf("expected recovered active account to serve success itself, got first=%d second=%d", firstHits, secondHits)
	}

	thirdAttempt := performResponsesRequest(t, handler)
	if thirdAttempt.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected post-success first failure to stay on active account, got %d body=%s", thirdAttempt.Code, thirdAttempt.Body.String())
	}
	if firstHits != 3 || secondHits != 0 {
		t.Fatalf("expected failover counter reset after success, got first=%d second=%d", firstHits, secondHits)
	}

	fourthAttempt := performResponsesRequest(t, handler)
	if fourthAttempt.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected post-success second failure to stay on active account, got %d body=%s", fourthAttempt.Code, fourthAttempt.Body.String())
	}
	if firstHits != 4 || secondHits != 0 {
		t.Fatalf("expected no failover before the new third consecutive failure, got first=%d second=%d", firstHits, secondHits)
	}
}

func TestAccountPipeline_AuthFailedSwitchToNextAccount(t *testing.T) {
	firstHits := 0
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		http.Error(w, "invalid session", http.StatusUnauthorized)
	}))
	defer firstServer.Close()

	secondHits := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_ok","status":"completed","output":[]}`))
	}))
	defer secondServer.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 21, AccountName: "acc-a", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: firstServer.URL, Enabled: true},
			{ID: 22, AccountName: "acc-b", ProviderType: "api_key", CredentialRaw: "sk-b", BaseURL: secondServer.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, firstServer.URL, service)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if firstHits != 1 || secondHits != 1 {
		t.Fatalf("expected both accounts to be attempted once, got first=%d second=%d", firstHits, secondHits)
	}

	if len(service.authFailCalls) != 1 || service.authFailCalls[0].id != 21 {
		t.Fatalf("expected auth-failed for account 21, got %+v", service.authFailCalls)
	}
	if !strings.Contains(service.authFailCalls[0].reason, "invalid session") {
		t.Fatalf("expected auth-failed reason to contain upstream detail, got %q", service.authFailCalls[0].reason)
	}
	if len(service.successCalls) != 1 || service.successCalls[0] != 22 {
		t.Fatalf("expected success on account 22, got %+v", service.successCalls)
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no transient failure calls, got %+v", service.transientCalls)
	}
}

func TestAccountPipeline_Upstream4xxPassthroughWithoutSwitch(t *testing.T) {
	firstHits := 0
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits++
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
	}))
	defer firstServer.Close()

	secondHits := 0
	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondHits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"should_not_run"}`))
	}))
	defer secondServer.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 31, AccountName: "acc-a", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: firstServer.URL, Enabled: true},
			{ID: 32, AccountName: "acc-b", ProviderType: "api_key", CredentialRaw: "sk-b", BaseURL: secondServer.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, firstServer.URL, service)

	rec := performResponsesRequest(t, handler)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if firstHits != 1 {
		t.Fatalf("expected first account called once, got %d", firstHits)
	}
	if secondHits != 0 {
		t.Fatalf("expected second account not to be called, got %d", secondHits)
	}
	if !strings.Contains(rec.Body.String(), "bad request") {
		t.Fatalf("expected passthrough body to contain upstream error, got %s", rec.Body.String())
	}
	if len(service.successCalls) != 0 || len(service.authFailCalls) != 0 || len(service.transientCalls) != 0 {
		t.Fatalf("expected no account state callbacks for passthrough 4xx, got success=%+v auth=%+v transient=%+v",
			service.successCalls, service.authFailCalls, service.transientCalls)
	}
}

func TestAccountPipeline_ResponsesStreamingCompleted_TreatedAsSuccess(t *testing.T) {
	upstreamHits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("event: response.in_progress\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.in_progress","response":{"id":"resp_1","model":"gpt-5-codex"}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5-codex"},"usage":{"input_tokens":12,"output_tokens":3,"input_tokens_details":{"cached_tokens":0}}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 41, AccountName: "acc-stream", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: upstream.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, upstream.URL, service)

	rec := performResponsesStreamingRequest(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamHits != 1 {
		t.Fatalf("expected upstream called once, got %d", upstreamHits)
	}
	if len(service.successCalls) != 1 || service.successCalls[0] != 41 {
		t.Fatalf("expected success on account 41, got %+v", service.successCalls)
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no transient failures, got %+v", service.transientCalls)
	}
	if len(service.authFailCalls) != 0 {
		t.Fatalf("expected no auth failures, got %+v", service.authFailCalls)
	}
}

func TestAccountPipeline_ResponsesStreamingFailed_RecordedAsServerSoftFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("event: response.created\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-5.4"}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("event: response.failed\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.failed","response":{"id":"resp_1","model":"gpt-5.4","error":{"type":"unknown_error","message":"Unknown error"}}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 71, AccountName: "acc-stream-failed", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: upstream.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, upstream.URL, service)

	rec := performResponsesStreamingRequest(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected streaming response to keep status 200 once headers are sent, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(service.softFailureCalls) != 1 {
		t.Fatalf("expected one soft failure call, got %+v", service.softFailureCalls)
	}
	if service.softFailureCalls[0].id != 71 || service.softFailureCalls[0].category != accountSoftFailureCategoryServerError {
		t.Fatalf("expected server_error soft failure for account 71, got %+v", service.softFailureCalls[0])
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no immediate transient cooldown for stream error, got %+v", service.transientCalls)
	}
	if len(service.successCalls) != 0 {
		t.Fatalf("expected no success callbacks, got %+v", service.successCalls)
	}
}

func TestAccountPipeline_ResponsesStreamingUsageLimitReached_UsesResetTimeCooldown(t *testing.T) {
	resetAt := time.Now().Add(90 * time.Minute).Round(time.Second)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("event: response.created\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.created","response":{"id":"resp_usage_limit","model":"gpt-5.4"}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write([]byte("event: response.failed\n"))
		_, _ = w.Write([]byte(fmt.Sprintf(`data: {"type":"response.failed","response":{"id":"resp_usage_limit","model":"gpt-5.4","error":{"type":"usage_limit_reached","message":"The usage limit has been reached","plan_type":"free","resets_at":%d,"resets_in_seconds":5400}}}`+"\n\n", resetAt.Unix())))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 81, AccountName: "acc-stream-usage-limit", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: upstream.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, upstream.URL, service)

	rec := performResponsesStreamingRequest(t, handler)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected streaming response to keep status 200 once headers are sent, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(service.usageLimitCalls) != 1 {
		t.Fatalf("expected one usage-limit cooldown call, got %+v", service.usageLimitCalls)
	}
	if service.usageLimitCalls[0].id != 81 || service.usageLimitCalls[0].planType != "free" {
		t.Fatalf("unexpected usage-limit call: %+v", service.usageLimitCalls[0])
	}
	if service.usageLimitCalls[0].resetAt.Unix() != resetAt.Unix() {
		t.Fatalf("expected resetAt=%d, got %d", resetAt.Unix(), service.usageLimitCalls[0].resetAt.Unix())
	}
	if len(service.softFailureCalls) != 0 {
		t.Fatalf("expected no soft failure fallback for usage-limit stream, got %+v", service.softFailureCalls)
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no generic transient cooldown, got %+v", service.transientCalls)
	}
}

func TestResolveAccountTargetURL_OAuthUsesChatGPTCodexEndpoint(t *testing.T) {
	acc := &store.UpstreamAccountRecord{
		ProviderType: "chatgpt_refresh_token",
		BaseURL:      "https://api.openai.com",
	}

	targetURL, err := resolveAccountTargetURL(acc, "/v1/responses", "stream=true")
	if err != nil {
		t.Fatalf("resolveAccountTargetURL returned error: %v", err)
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("parse target url failed: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "chatgpt.com" {
		t.Fatalf("expected chatgpt host, got %s://%s", parsed.Scheme, parsed.Host)
	}
	if parsed.Path != "/backend-api/codex/responses" {
		t.Fatalf("unexpected path: %s", parsed.Path)
	}
	if parsed.RawQuery != "stream=true" {
		t.Fatalf("unexpected query: %s", parsed.RawQuery)
	}
}

func TestResolveAccountTargetURL_OAuthUsesChatGPTCodexCompactEndpoint(t *testing.T) {
	acc := &store.UpstreamAccountRecord{
		ProviderType: "chatgpt_refresh_token",
		BaseURL:      "https://api.openai.com",
	}

	targetURL, err := resolveAccountTargetURL(acc, "/v1/responses/compact", "stream=true")
	if err != nil {
		t.Fatalf("resolveAccountTargetURL returned error: %v", err)
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("parse target url failed: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "chatgpt.com" {
		t.Fatalf("expected chatgpt host, got %s://%s", parsed.Scheme, parsed.Host)
	}
	if parsed.Path != "/backend-api/codex/responses/compact" {
		t.Fatalf("unexpected path: %s", parsed.Path)
	}
	if parsed.RawQuery != "stream=true" {
		t.Fatalf("unexpected query: %s", parsed.RawQuery)
	}
}

func TestResolveAccountTargetURL_OAuthUsesChatGPTCodexModelsEndpoint(t *testing.T) {
	acc := &store.UpstreamAccountRecord{
		ProviderType: "chatgpt_refresh_token",
		BaseURL:      "https://api.openai.com",
	}

	targetURL, err := resolveAccountTargetURL(acc, "/v1/models", "client_version=0.131.0")
	if err != nil {
		t.Fatalf("resolveAccountTargetURL returned error: %v", err)
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		t.Fatalf("parse target url failed: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "chatgpt.com" {
		t.Fatalf("expected chatgpt host, got %s://%s", parsed.Scheme, parsed.Host)
	}
	if parsed.Path != "/backend-api/codex/models" {
		t.Fatalf("unexpected path: %s", parsed.Path)
	}
	if parsed.RawQuery != "client_version=0.131.0" {
		t.Fatalf("unexpected query: %s", parsed.RawQuery)
	}
}

func TestApplyOpenAIChatGPTOAuthHeaders_SetsRequiredHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{}`))
	credential := `{"refresh_token":"rt-1","chatgpt_account_id":"acc-1"}`

	applyOpenAIChatGPTOAuthHeaders(req, credential)

	if req.Host != "chatgpt.com" {
		t.Fatalf("expected host=chatgpt.com, got %s", req.Host)
	}
	if req.Header.Get("Accept") != "text/event-stream" {
		t.Fatalf("unexpected Accept header: %s", req.Header.Get("Accept"))
	}
	if req.Header.Get("OpenAI-Beta") != "responses=experimental" {
		t.Fatalf("unexpected OpenAI-Beta header: %s", req.Header.Get("OpenAI-Beta"))
	}
	if req.Header.Get("originator") != "codex_cli_rs" {
		t.Fatalf("unexpected originator header: %s", req.Header.Get("originator"))
	}
	if req.Header.Get("chatgpt-account-id") != "acc-1" {
		t.Fatalf("unexpected chatgpt-account-id header: %s", req.Header.Get("chatgpt-account-id"))
	}
}

func TestApplyOpenAIChatGPTModelsHeaders_SkipsResponsesOnlyHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/codex/models", nil)
	credential := `{"refresh_token":"rt-1","chatgpt_account_id":"acc-1"}`

	applyOpenAIChatGPTModelsHeaders(req, credential)

	if req.Host != "chatgpt.com" {
		t.Fatalf("expected host=chatgpt.com, got %s", req.Host)
	}
	if req.Header.Get("chatgpt-account-id") != "acc-1" {
		t.Fatalf("unexpected chatgpt-account-id header: %s", req.Header.Get("chatgpt-account-id"))
	}
	if req.Header.Get("OpenAI-Beta") != "" {
		t.Fatalf("expected models request not to set OpenAI-Beta, got %s", req.Header.Get("OpenAI-Beta"))
	}
	if req.Header.Get("Accept") == "text/event-stream" {
		t.Fatal("expected models request not to force text/event-stream")
	}
}

func TestPrepareCodexAccountBodyForUpstream_AnyRouterModelAlias(t *testing.T) {
	anyRouterAccount := &store.UpstreamAccountRecord{
		AccountName:   "anyrouter-codex",
		ProviderType:  "api_key",
		CredentialRaw: "sk-anyrouter",
		BaseURL:       "https://api.anyrouter.example",
	}
	regularAccount := &store.UpstreamAccountRecord{
		AccountName:   "openai",
		ProviderType:  "api_key",
		CredentialRaw: "sk-openai",
		BaseURL:       "https://api.openai.com",
	}

	tests := []struct {
		name      string
		account   *store.UpstreamAccountRecord
		path      string
		body      string
		wantModel string
		wantRaw   string
	}{
		{
			name:      "rewrites versioned gpt-5.4-mini for anyrouter responses",
			account:   anyRouterAccount,
			path:      "/v1/responses",
			body:      `{"model":"gpt-5.4-mini-2026-03-17","input":"hello"}`,
			wantModel: "gpt-5.5",
		},
		{
			name:      "rewrites gpt-5.4 for anyrouter compact",
			account:   anyRouterAccount,
			path:      "/v1/responses/compact",
			body:      `{"model":"gpt-5.4","input":"hello"}`,
			wantModel: "gpt-5.5",
		},
		{
			name:      "keeps supported model for anyrouter",
			account:   anyRouterAccount,
			path:      "/v1/responses",
			body:      `{"model":"gpt-5.5","input":"hello"}`,
			wantModel: "gpt-5.5",
		},
		{
			name:      "keeps gpt-5.4 for non-anyrouter account",
			account:   regularAccount,
			path:      "/v1/responses",
			body:      `{"model":"gpt-5.4-mini","input":"hello"}`,
			wantModel: "gpt-5.4-mini",
		},
		{
			name:    "keeps models path untouched",
			account: anyRouterAccount,
			path:    "/v1/models",
			body:    `{"model":"gpt-5.4-mini","input":"hello"}`,
			wantRaw: `{"model":"gpt-5.4-mini","input":"hello"}`,
		},
		{
			name:    "keeps invalid json untouched",
			account: anyRouterAccount,
			path:    "/v1/responses",
			body:    `{"model":`,
			wantRaw: `{"model":`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prepareCodexAccountBodyForUpstream([]byte(tt.body), tt.account, tt.path)
			if tt.wantRaw != "" {
				if string(got) != tt.wantRaw {
					t.Fatalf("expected raw body %q, got %q", tt.wantRaw, string(got))
				}
				return
			}

			var payload map[string]any
			if err := json.Unmarshal(got, &payload); err != nil {
				t.Fatalf("unmarshal rewritten body failed: %v body=%s", err, string(got))
			}
			if payload["model"] != tt.wantModel {
				t.Fatalf("expected model %q, got %q", tt.wantModel, payload["model"])
			}
		})
	}
}

func TestParseAccountStreamStatusError_Cancelled(t *testing.T) {
	status, modelName := parseAccountStreamStatusError(context.Canceled)
	if status != "cancelled" {
		t.Fatalf("expected cancelled status for context.Canceled, got %s", status)
	}
	if modelName != "unknown" {
		t.Fatalf("expected unknown model for context.Canceled, got %s", modelName)
	}

	status, modelName = parseAccountStreamStatusError(errors.New("stream_status:cancelled:model:gpt-5.4: context canceled"))
	if status != "cancelled" {
		t.Fatalf("expected cancelled status for structured stream error, got %s", status)
	}
	if modelName != "gpt-5.4" {
		t.Fatalf("expected gpt-5.4 model for structured stream error, got %s", modelName)
	}
	if !isCancelledAccountStreamError(errors.New("stream_status:cancelled:model:gpt-5.4: context canceled")) {
		t.Fatal("expected structured cancelled stream error to be recognized")
	}
}

func TestReadAndRestoreResponseBody_PreservesFullBody(t *testing.T) {
	fullBody := strings.Repeat("abcdefghij", 300)
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Body:       io.NopCloser(strings.NewReader(fullBody)),
	}

	detail := readAndRestoreResponseBody(resp, 32)
	if len(detail) != 32 {
		t.Fatalf("expected truncated detail length 32, got %d", len(detail))
	}

	restoredBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll restored body failed: %v", err)
	}
	if string(restoredBody) != fullBody {
		t.Fatal("expected restored body to keep full upstream payload")
	}
}

func TestAccountPipeline_ResponsesStreamingCompletedAfterClientCancel_TreatedAsSuccess(t *testing.T) {
	completedWritten := make(chan struct{})
	allowClose := make(chan struct{})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("event: response.in_progress\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.in_progress","response":{"id":"resp_1","model":"gpt-5.4"}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte(`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.4"},"usage":{"input_tokens":21,"output_tokens":4,"input_tokens_details":{"cached_tokens":11}}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		close(completedWritten)
		<-allowClose
	}))
	defer upstream.Close()

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 61, AccountName: "acc-stream-cancel", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: upstream.URL, Enabled: true},
		},
	}
	handler := newAccountPipelineTestHandler(t, upstream.URL, service)

	body := bytes.NewBufferString(`{"model":"gpt-5.4","stream":true,"input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", body)
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	<-completedWritten
	cancel()
	close(allowClose)
	<-done

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(service.successCalls) != 1 || service.successCalls[0] != 61 {
		t.Fatalf("expected success on account 61, got %+v", service.successCalls)
	}
	if len(service.successCtxErrs) != 1 || service.successCtxErrs[0] != nil {
		t.Fatalf("expected success finalization with live context, got %+v", service.successCtxErrs)
	}
	if len(service.completeCalls) == 0 || service.completeCalls[len(service.completeCalls)-1].outcome != servicepkg.AccountScheduleOutcomeSuccess {
		t.Fatalf("expected schedule snapshot success, got %+v", service.completeCalls)
	}
	if len(service.completeCtxErrs) == 0 || service.completeCtxErrs[len(service.completeCtxErrs)-1] != nil {
		t.Fatalf("expected schedule snapshot finalization with live context, got %+v", service.completeCtxErrs)
	}
	if len(service.transientCalls) != 0 {
		t.Fatalf("expected no transient failures, got %+v", service.transientCalls)
	}
}
