package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/store"
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

type mockAccountPoolService struct {
	accounts []*store.UpstreamAccountRecord
	listErr  error

	mu             sync.Mutex
	successCalls   []int64
	authFailCalls  []accountAuthCall
	transientCalls []accountTransientCall
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

func (m *mockAccountPoolService) MarkAccountSuccess(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.successCalls = append(m.successCalls, id)
	return nil
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
	if regularClient1.Timeout <= 0 {
		t.Fatalf("expected regular client timeout to be set, got %v", regularClient1.Timeout)
	}
	if sseClient.Timeout != 0 {
		t.Fatalf("expected SSE client timeout to be disabled, got %v", sseClient.Timeout)
	}
}

func TestAccountPipeline_429FailoverThenSuccess(t *testing.T) {
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

	service := &mockAccountPoolService{
		accounts: []*store.UpstreamAccountRecord{
			{ID: 11, AccountName: "acc-a", ProviderType: "api_key", CredentialRaw: "sk-a", BaseURL: firstServer.URL, Enabled: true},
			{ID: 12, AccountName: "acc-b", ProviderType: "api_key", CredentialRaw: "sk-b", BaseURL: secondServer.URL, Enabled: true},
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

	if len(service.transientCalls) != 1 || service.transientCalls[0].id != 11 {
		t.Fatalf("expected transient failure on account 11, got %+v", service.transientCalls)
	}
	if len(service.successCalls) != 1 || service.successCalls[0] != 12 {
		t.Fatalf("expected success on account 12, got %+v", service.successCalls)
	}
	if len(service.authFailCalls) != 0 {
		t.Fatalf("expected no auth-failed call, got %+v", service.authFailCalls)
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
	if !strings.Contains(rec.Body.String(), "response.completed") {
		t.Fatalf("expected streamed response to contain response.completed event, got %s", rec.Body.String())
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
