package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
)

func TestForwarder_ForwardRequestToEndpoint(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头
		if r.Header.Get("Authorization") == "" {
			t.Errorf("Expected Authorization header to be set")
		}

		// 验证Host头
		expectedHost := r.Host
		if expectedHost == "" {
			t.Errorf("Expected Host header to be set")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	// 创建配置
	cfg := &config.Config{}

	// 创建端点配置
	endpointConfig := config.EndpointConfig{
		Name:     "test-endpoint",
		URL:      server.URL,
		Token:    "test-token",
		Timeout:  30 * time.Second,
		Priority: 1,
	}

	// 创建端点管理器
	endpointManager := endpoint.NewManager(cfg)
	ep := &endpoint.Endpoint{Config: endpointConfig}

	// 创建Forwarder
	forwarder := NewForwarder(cfg, endpointManager)

	// 创建测试请求
	bodyBytes := []byte(`{"message": "test"}`)
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	// 执行转发
	ctx := context.Background()
	resp, err := forwarder.ForwardRequestToEndpoint(ctx, req, bodyBytes, ep)

	if err != nil {
		t.Fatalf("ForwardRequestToEndpoint failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected response, got nil")
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// 验证响应体
	defer resp.Body.Close()
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	responseBody := string(body[:n])

	if responseBody != "test response" {
		t.Errorf("Expected 'test response', got '%s'", responseBody)
	}
}

func TestForwarder_ForwardStreamingRequestToEndpoint_UsesIndependentContext(t *testing.T) {
	requestCancelled := make(chan struct{}, 1)
	releaseServer := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\"}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}

		select {
		case <-r.Context().Done():
			requestCancelled <- struct{}{}
		case <-releaseServer:
		}
	}))
	defer server.Close()

	cfg := &config.Config{}
	endpointManager := endpoint.NewManager(cfg)
	ep := &endpoint.Endpoint{Config: config.EndpointConfig{Name: "test-endpoint", URL: server.URL, Timeout: 30 * time.Second, Priority: 1}}
	forwarder := NewForwarder(cfg, endpointManager)

	bodyBytes := []byte(`{"message": "test"}`)
	parentCtx, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(bodyBytes)).WithContext(parentCtx)

	resp, upstreamCancel, err := forwarder.ForwardStreamingRequestToEndpoint(req, bodyBytes, ep)
	if err != nil {
		t.Fatalf("ForwardStreamingRequestToEndpoint failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if upstreamCancel == nil {
		t.Fatal("expected upstream cancel func, got nil")
	}

	cancelParent()
	select {
	case <-requestCancelled:
		t.Fatal("expected upstream request context to outlive parent request cancellation")
	case <-time.After(120 * time.Millisecond):
	}

	upstreamCancel()
	select {
	case <-requestCancelled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected upstream cancel func to cancel request context")
	}

	close(releaseServer)
	_ = resp.Body.Close()
}

func TestForwarder_ForwardRequestToEndpoint_IncludesUpstreamErrorDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("request-id", "req_upstream_123")
		w.Header().Set("Retry-After", "7")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"quota exceeded"}}`))
	}))
	defer server.Close()

	cfg := &config.Config{}
	endpointManager := endpoint.NewManager(cfg)
	ep := &endpoint.Endpoint{Config: config.EndpointConfig{Name: "test-endpoint", URL: server.URL, Timeout: 30 * time.Second, Priority: 1}}
	forwarder := NewForwarder(cfg, endpointManager)

	bodyBytes := []byte(`{"message":"test"}`)
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(bodyBytes))

	resp, err := forwarder.ForwardRequestToEndpoint(context.Background(), req, bodyBytes, ep)
	if err == nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("expected upstream error, got nil")
	}

	errorText := err.Error()
	expectedParts := []string{
		"endpoint=test-endpoint",
		"status=429",
		"request_id=req_upstream_123",
		"retry_after=7",
		"quota exceeded",
	}
	for _, part := range expectedParts {
		if !strings.Contains(errorText, part) {
			t.Fatalf("expected error to contain %q, got %q", part, errorText)
		}
	}
}

func TestForwarder_PrepareBodyForEndpoint_StripsCoderelayCacheControlScope(t *testing.T) {
	bodyBytes := []byte(`{
		"system": [
			{
				"type": "text",
				"text": "system prompt",
				"cache_control": {
					"type": "ephemeral",
					"ttl": "1h",
					"scope": "conversation",
					"vendor_extension": {
						"scope": "global"
					}
				}
			}
		],
		"messages": [
			{
				"role": "user",
				"content": [
					{
						"type": "text",
						"text": "hello",
						"cache_control": {
							"type": "ephemeral",
							"scope": "conversation",
							"rules": [
								{
									"scope": "global"
								}
							]
						}
					}
				]
			}
		],
		"tools": [
			{
				"name": "lookup",
				"cache_control": [
					{
						"type": "ephemeral",
						"scope": "global"
					}
				]
			}
		],
		"metadata": {
			"scope": "keep"
		}
	}`)

	tests := []struct {
		name string
		ep   *endpoint.Endpoint
	}{
		{
			name: "channel",
			ep:   &endpoint.Endpoint{Config: config.EndpointConfig{Name: "relay-proxy", Channel: "coderelay"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prepareBodyForEndpoint("/v1/messages", bodyBytes, tt.ep)

			var payload any
			if err := json.Unmarshal(got, &payload); err != nil {
				t.Fatalf("sanitized body should still be JSON: %v", err)
			}

			cacheControlCount, scopeCount := countCacheControlScopes(payload)
			if cacheControlCount != 3 {
				t.Fatalf("expected 3 cache_control objects, got %d", cacheControlCount)
			}
			if scopeCount != 0 {
				t.Fatalf("expected cache_control.scope to be stripped, got %d remaining", scopeCount)
			}

			root, ok := payload.(map[string]any)
			if !ok {
				t.Fatal("expected root object")
			}
			metadata, ok := root["metadata"].(map[string]any)
			if !ok {
				t.Fatal("expected metadata object")
			}
			if metadata["scope"] != "keep" {
				t.Fatalf("expected non-cache_control scope to be preserved, got %v", metadata["scope"])
			}
		})
	}
}

func TestForwarder_PrepareBodyForEndpoint_RewritesConfiguredClaudeModel(t *testing.T) {
	bodyBytes := []byte(`{"model":"claude-sonnet-4-5","messages":[],"max_tokens":1024}`)
	rules := `[{"paths":["/v1/messages","/v1/messages/count_tokens"],"match":"exact","from":"claude-sonnet-4-5","to":"provider-sonnet"}]`
	ep := &endpoint.Endpoint{Config: config.EndpointConfig{Name: "rewrite-endpoint", ModelRewriteRules: rules}}

	for _, path := range []string{"/v1/messages", "/v1/messages/count_tokens"} {
		got := prepareBodyForEndpoint(path, bodyBytes, ep)
		var payload map[string]any
		if err := json.Unmarshal(got, &payload); err != nil {
			t.Fatalf("unmarshal rewritten body failed: %v", err)
		}
		if payload["model"] != "provider-sonnet" {
			t.Fatalf("path %s expected provider-sonnet, got %v", path, payload["model"])
		}
	}

	if got := prepareBodyForEndpoint("/v1/responses", bodyBytes, ep); !bytes.Equal(got, bodyBytes) {
		t.Fatalf("non-Claude path must stay unchanged, got %s", string(got))
	}
	if got := prepareBodyForEndpoint("/v1/messages", []byte(`{"model":"claude-sonnet-4-5-latest"}`), ep); string(got) != `{"model":"claude-sonnet-4-5-latest"}` {
		t.Fatalf("exact rule must not match suffix model, got %s", string(got))
	}

	invalidRuleEndpoint := &endpoint.Endpoint{Config: config.EndpointConfig{Name: "invalid-rules", ModelRewriteRules: `{"from":`}}
	if got := prepareBodyForEndpoint("/v1/messages", bodyBytes, invalidRuleEndpoint); !bytes.Equal(got, bodyBytes) {
		t.Fatalf("invalid saved rules must preserve request body, got %s", string(got))
	}

	trailingJSONBody := []byte(`{"model":"claude-sonnet-4-5"}{"messages":[]}`)
	if got := prepareBodyForEndpoint("/v1/messages", trailingJSONBody, ep); !bytes.Equal(got, trailingJSONBody) {
		t.Fatalf("body with trailing JSON must stay unchanged, got %s", string(got))
	}
}

func TestForwarder_PrepareBodyForEndpoint_StripsMywechatContextManagement(t *testing.T) {
	bodyBytes := []byte(`{
		"model": "claude-opus-4-7",
		"stream": true,
		"context_management": {
			"edits": [
				{
					"type": "clear_tool_uses_20250919"
				}
			]
		},
		"system": [
			{
				"type": "text",
				"text": "system prompt",
				"cache_control": {
					"type": "ephemeral",
					"scope": "conversation"
				}
			}
		],
		"metadata": {
			"context_management": "keep"
		}
	}`)
	ep := &endpoint.Endpoint{Config: config.EndpointConfig{Name: "mywechat", Channel: "coderelay"}}

	got := prepareBodyForEndpoint("/v1/messages", bodyBytes, ep)

	var payload any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("sanitized body should still be JSON: %v", err)
	}
	root, ok := payload.(map[string]any)
	if !ok {
		t.Fatal("expected root object")
	}
	if _, exists := root[contextManagementKey]; exists {
		t.Fatalf("expected top-level %s to be stripped, got %s", contextManagementKey, string(got))
	}
	metadata, ok := root["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected metadata object")
	}
	if metadata[contextManagementKey] != "keep" {
		t.Fatalf("expected nested metadata context_management to be preserved, got %v", metadata[contextManagementKey])
	}
	_, scopeCount := countCacheControlScopes(payload)
	if scopeCount != 0 {
		t.Fatalf("expected cache_control.scope to still be stripped, got %d remaining", scopeCount)
	}
}

func TestForwarder_PrepareBodyForEndpoint_KeepsContextManagementForOtherCoderelayEndpoints(t *testing.T) {
	bodyBytes := []byte(`{"context_management":{"edits":[]},"system":[{"cache_control":{"type":"ephemeral","scope":"conversation"}}]}`)
	ep := &endpoint.Endpoint{Config: config.EndpointConfig{Name: "relay-proxy", Channel: "coderelay"}}

	got := prepareBodyForEndpoint("/v1/messages", bodyBytes, ep)

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("sanitized body should still be JSON: %v", err)
	}
	if _, exists := payload[contextManagementKey]; !exists {
		t.Fatalf("expected %s to be preserved for non-mywechat coderelay endpoint, got %s", contextManagementKey, string(got))
	}
	_, scopeCount := countCacheControlScopes(payload)
	if scopeCount != 0 {
		t.Fatalf("expected cache_control.scope to be stripped, got %d remaining", scopeCount)
	}
}

func TestForwarder_PrepareBodyForEndpoint_KeepsOtherChannelsUntouched(t *testing.T) {
	bodyBytes := []byte(`{"system":[{"cache_control":{"type":"ephemeral","scope":"conversation"}}]}`)
	ep := &endpoint.Endpoint{Config: config.EndpointConfig{Name: "coderelay", Channel: "custom"}}

	got := prepareBodyForEndpoint("/v1/messages", bodyBytes, ep)

	if !bytes.Equal(got, bodyBytes) {
		t.Fatalf("expected non-coderelay channel body to stay untouched, got %s", string(got))
	}
}

func TestForwarder_Pipeline_StripsCoderelayCacheControlScope(t *testing.T) {
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cfg := &config.Config{}
	endpointManager := endpoint.NewManager(cfg)
	t.Cleanup(endpointManager.Stop)
	forwarder := NewForwarder(cfg, endpointManager)
	target := acquireForwarderTestTarget(t, endpointManager, config.EndpointConfig{
		Name:    "mywechat",
		URL:     server.URL,
		Channel: "coderelay",
		Timeout: 30 * time.Second,
	})
	bodyBytes := []byte(`{"system":[{"cache_control":{"type":"ephemeral","nested":{"scope":"global"}}}],"metadata":{"scope":"keep"}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(bodyBytes))

	resp, _, _, err := forwarder.ForwardForPipeline(context.Background(), req, bodyBytes, target, false, false, nil)
	if err != nil {
		t.Fatalf("ForwardForPipeline failed: %v", err)
	}
	defer resp.Body.Close()

	var payload any
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("sanitized body should still be JSON: %v", err)
	}

	_, scopeCount := countCacheControlScopes(payload)
	if scopeCount != 0 {
		t.Fatalf("expected regular handler to strip cache_control scope, got %d remaining", scopeCount)
	}
	root, ok := payload.(map[string]any)
	if !ok {
		t.Fatal("expected root object")
	}
	metadata, ok := root["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected metadata object")
	}
	if metadata["scope"] != "keep" {
		t.Fatalf("expected non-cache_control scope to be preserved, got %v", metadata["scope"])
	}
}

func TestForwarder_ForwardForPipelineUsesAdmittedSnapshotAfterLiveEdit(t *testing.T) {
	oldRequest := make(chan http.Header, 1)
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldRequest <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(oldServer.Close)

	newRequest := make(chan struct{}, 1)
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		newRequest <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(newServer.Close)

	cfg := &config.Config{}
	manager := endpoint.NewManager(cfg)
	t.Cleanup(manager.Stop)
	forwarder := NewForwarder(cfg, manager)
	target := acquireForwarderTestTarget(t, manager, config.EndpointConfig{
		Name:    "snapshot",
		URL:     oldServer.URL,
		Token:   "old-token",
		Headers: map[string]string{"X-Snapshot": "old"},
		Timeout: time.Second,
	})

	if err := manager.UpdateEndpointConfig("snapshot", config.EndpointConfig{
		Name:    "snapshot",
		URL:     newServer.URL,
		Token:   "new-token",
		Headers: map[string]string{"X-Snapshot": "new"},
		Timeout: time.Second,
	}); err != nil {
		t.Fatalf("update live endpoint: %v", err)
	}

	body := []byte(`{"model":"claude-test","messages":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	resp, _, _, err := forwarder.ForwardForPipeline(context.Background(), req, body, target, false, false, nil)
	if err != nil {
		t.Fatalf("forward admitted snapshot: %v", err)
	}
	defer resp.Body.Close()

	select {
	case headers := <-oldRequest:
		if got := headers.Get("Authorization"); got != "Bearer old-token" {
			t.Fatalf("expected snapshotted token, got %q", got)
		}
		if got := headers.Get("X-Snapshot"); got != "old" {
			t.Fatalf("expected snapshotted custom header, got %q", got)
		}
	default:
		t.Fatal("expected request to reach the admitted URL")
	}
	select {
	case <-newRequest:
		t.Fatal("request must not use the live config edited after admission")
	default:
	}
}

func TestForwarder_CopyAttemptHeadersDoesNotResolveCredentialsAgain(t *testing.T) {
	cfg := &config.Config{}
	manager := endpoint.NewManager(cfg)
	t.Cleanup(manager.Stop)
	forwarder := NewForwarder(cfg, manager)
	target := acquireForwarderTestTarget(t, manager, config.EndpointConfig{
		Name:    "snapshot-empty-credential",
		URL:     "https://upstream.example.com",
		Group:   "shared",
		Timeout: time.Second,
	})
	if err := manager.AddEndpoint(config.EndpointConfig{
		Name:   "late-credential-source",
		Group:  "shared",
		Token:  "late-token",
		ApiKey: "late-key",
	}); err != nil {
		t.Fatalf("add late credential source: %v", err)
	}

	src := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	dst := httptest.NewRequest(http.MethodPost, "https://upstream.example.com/v1/messages", nil)
	forwarder.CopyAttemptHeaders(src, dst, target)
	if got := dst.Header.Get("Authorization"); got != "" {
		t.Fatalf("empty credential snapshot must stay empty, got authorization %q", got)
	}
	if got := dst.Header.Get("X-Api-Key"); got != "" {
		t.Fatalf("empty credential snapshot must stay empty, got api key %q", got)
	}
}

func TestForwarder_ForwardForPipeline_NonSSETimeoutCoversResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	cfg := &config.Config{Streaming: config.StreamingConfig{ResponseHeaderTimeout: time.Second}}
	manager := endpoint.NewManager(cfg)
	t.Cleanup(manager.Stop)
	forwarder := NewForwarder(cfg, manager)
	target := acquireForwarderTestTarget(t, manager, config.EndpointConfig{
		Name:    "regular-timeout",
		URL:     server.URL,
		Timeout: 80 * time.Millisecond,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(`{"stream":false}`)))

	resp, release, _, err := forwarder.ForwardForPipeline(context.Background(), req, []byte(`{"stream":false}`), target, false, false, nil)
	if err != nil {
		t.Fatalf("expected response headers before timeout, got %v", err)
	}
	if release != nil {
		t.Fatal("non-SSE request must not detach upstream context")
	}
	defer resp.Body.Close()

	started := time.Now()
	_, err = io.ReadAll(resp.Body)
	if err == nil {
		t.Fatal("expected response body read to stop at endpoint timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("response body timeout took too long: %v", elapsed)
	}
}

func TestForwarder_ForwardForPipeline_SSEIgnoresEndpointTotalTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(120 * time.Millisecond)
		_, _ = w.Write([]byte("data: done\n\n"))
	}))
	defer server.Close()

	cfg := &config.Config{Streaming: config.StreamingConfig{ResponseHeaderTimeout: time.Second}}
	manager := endpoint.NewManager(cfg)
	t.Cleanup(manager.Stop)
	forwarder := NewForwarder(cfg, manager)
	target := acquireForwarderTestTarget(t, manager, config.EndpointConfig{
		Name:    "stream-no-total-timeout",
		URL:     server.URL,
		Timeout: 40 * time.Millisecond,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(`{"stream":true}`)))

	resp, release, _, err := forwarder.ForwardForPipeline(context.Background(), req, []byte(`{"stream":true}`), target, true, false, nil)
	if err != nil {
		t.Fatalf("SSE request should not use endpoint total timeout: %v", err)
	}
	if release != nil {
		t.Fatal("tail drain disabled should keep the caller context")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("SSE body should outlive endpoint timeout: %v", err)
	}
	if string(body) != "data: done\n\n" {
		t.Fatalf("unexpected SSE body: %q", string(body))
	}
}

func acquireForwarderTestTarget(t *testing.T, manager *endpoint.Manager, cfg config.EndpointConfig) *endpoint.EndpointAttemptTarget {
	t.Helper()
	if err := manager.AddEndpoint(cfg); err != nil {
		t.Fatalf("add test endpoint: %v", err)
	}
	result := manager.PrepareRouteCandidates(context.Background(), endpoint.BuildRouteRequestProfile("/v1/messages", nil))
	if len(result.Plans) != 1 {
		t.Fatalf("expected one test attempt plan, got %d", len(result.Plans))
	}
	admission, err := manager.AcquireEndpointAttempt(result.Plans[0])
	if err != nil {
		t.Fatalf("acquire test attempt: %v", err)
	}
	t.Cleanup(admission.Release)
	return admission.Target
}

func countCacheControlScopes(value any) (int, int) {
	return countCacheControlScopesValue(value, false)
}

func countCacheControlScopesValue(value any, insideCacheControl bool) (int, int) {
	cacheControlCount := 0
	scopeCount := 0

	switch node := value.(type) {
	case map[string]any:
		if insideCacheControl {
			if _, exists := node[cacheControlScopeKey]; exists {
				scopeCount++
			}
		}
		for key, child := range node {
			childInsideCacheControl := insideCacheControl || key == cacheControlKey
			if key == cacheControlKey {
				switch cacheControl := child.(type) {
				case map[string]any:
					cacheControlCount++
				case []any:
					for _, item := range cacheControl {
						if _, ok := item.(map[string]any); ok {
							cacheControlCount++
						}
					}
				}
			}

			childCacheControlCount, childScopeCount := countCacheControlScopesValue(child, childInsideCacheControl)
			cacheControlCount += childCacheControlCount
			scopeCount += childScopeCount
		}
	case []any:
		for _, child := range node {
			childCacheControlCount, childScopeCount := countCacheControlScopesValue(child, insideCacheControl)
			cacheControlCount += childCacheControlCount
			scopeCount += childScopeCount
		}
	}

	return cacheControlCount, scopeCount
}

func TestForwarder_BuildStreamingTransport_ReusesTransport(t *testing.T) {
	cfg := &config.Config{}
	endpointManager := endpoint.NewManager(cfg)
	forwarder := NewForwarder(cfg, endpointManager)

	first, err := forwarder.buildStreamingTransport()
	if err != nil {
		t.Fatalf("first buildStreamingTransport failed: %v", err)
	}
	second, err := forwarder.buildStreamingTransport()
	if err != nil {
		t.Fatalf("second buildStreamingTransport failed: %v", err)
	}

	if first == nil || second == nil {
		t.Fatal("expected non-nil transports")
	}
	if first != second {
		t.Fatal("expected buildStreamingTransport to reuse the same transport instance")
	}
	if first.MaxIdleConnsPerHost != 16 {
		t.Fatalf("expected MaxIdleConnsPerHost=16, got %d", first.MaxIdleConnsPerHost)
	}
	if first.MaxConnsPerHost != 32 {
		t.Fatalf("expected MaxConnsPerHost=32, got %d", first.MaxConnsPerHost)
	}
}

func TestForwarder_CopyHeaders(t *testing.T) {
	// 创建配置
	cfg := &config.Config{}

	// 创建端点配置
	endpointConfig := config.EndpointConfig{
		Name:   "test-endpoint",
		URL:    "https://api.example.com",
		Token:  "test-token",
		ApiKey: "test-api-key",
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
		},
	}

	// 创建端点管理器
	endpointManager := endpoint.NewManager(cfg)
	ep := &endpoint.Endpoint{Config: endpointConfig}

	// 创建Forwarder
	forwarder := NewForwarder(cfg, endpointManager)

	// 创建源请求
	srcReq := httptest.NewRequest("POST", "/test", nil)
	srcReq.Header.Set("Content-Type", "application/json")
	srcReq.Header.Set("User-Agent", "Test-Client")
	srcReq.Header.Set("Authorization", "Bearer client-token") // 应该被覆盖
	srcReq.Header.Set("X-API-Key", "client-api-key")          // 应该被移除

	// 创建目标请求
	dstReq := httptest.NewRequest("POST", "https://api.example.com/test", nil)

	// 执行头部复制
	forwarder.CopyHeaders(srcReq, dstReq, ep)

	// 验证结果
	if dstReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type to be preserved")
	}

	if dstReq.Header.Get("User-Agent") != "Test-Client" {
		t.Errorf("Expected User-Agent to be preserved")
	}

	if dstReq.Header.Get("Authorization") != "Bearer test-token" {
		t.Errorf("Expected Authorization to be replaced with endpoint token")
	}

	if dstReq.Header.Get("X-Api-Key") != "test-api-key" {
		t.Errorf("Expected X-Api-Key to be set from endpoint config")
	}

	if dstReq.Header.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("Expected custom header to be added")
	}

	if dstReq.Header.Get("Host") != "api.example.com" {
		t.Errorf("Expected Host header to be set correctly")
	}

	// 验证敏感头部被移除
	if dstReq.Header.Get("X-API-Key") == "client-api-key" {
		t.Errorf("Expected client X-API-Key to be removed")
	}
}

func TestForwarder_CopyHeaders_AddsAnyrouteBetaHeader(t *testing.T) {
	cfg := &config.Config{}
	endpointManager := endpoint.NewManager(cfg)
	forwarder := NewForwarder(cfg, endpointManager)

	ep := &endpoint.Endpoint{Config: config.EndpointConfig{
		Name:    "anyroute-endpoint",
		URL:     "https://anyrouter.top",
		Channel: "anyroute",
	}}
	srcReq := httptest.NewRequest("POST", "/v1/messages", nil)
	dstReq := httptest.NewRequest("POST", "https://anyrouter.top/v1/messages", nil)

	forwarder.CopyHeaders(srcReq, dstReq, ep)

	if got := dstReq.Header.Get("anthropic-beta"); got != "context-1m-2025-08-07" {
		t.Fatalf("Expected anyroute anthropic-beta header, got %q", got)
	}
}

func TestForwarder_CopyHeaders_AddsAnyrouteBetaHeaderWithCaseInsensitiveChannel(t *testing.T) {
	cfg := &config.Config{}
	endpointManager := endpoint.NewManager(cfg)
	forwarder := NewForwarder(cfg, endpointManager)

	ep := &endpoint.Endpoint{Config: config.EndpointConfig{
		Name:    "anyroute-endpoint",
		URL:     "https://anyrouter.top",
		Channel: " AnyRoute ",
	}}
	srcReq := httptest.NewRequest("POST", "/v1/messages", nil)
	dstReq := httptest.NewRequest("POST", "https://anyrouter.top/v1/messages", nil)

	forwarder.CopyHeaders(srcReq, dstReq, ep)

	if got := dstReq.Header.Get("anthropic-beta"); got != "context-1m-2025-08-07" {
		t.Fatalf("Expected anyroute anthropic-beta header, got %q", got)
	}
}

func TestForwarder_CopyHeaders_AppendsAnyrouteBetaFlagToConfiguredHeader(t *testing.T) {
	cfg := &config.Config{}
	endpointManager := endpoint.NewManager(cfg)
	forwarder := NewForwarder(cfg, endpointManager)

	ep := &endpoint.Endpoint{Config: config.EndpointConfig{
		Name:    "anyroute-endpoint",
		URL:     "https://anyrouter.top",
		Channel: "anyroute",
		Headers: map[string]string{
			"anthropic-beta": "custom-beta",
		},
	}}
	srcReq := httptest.NewRequest("POST", "/v1/messages", nil)
	dstReq := httptest.NewRequest("POST", "https://anyrouter.top/v1/messages", nil)

	forwarder.CopyHeaders(srcReq, dstReq, ep)

	if got := dstReq.Header.Get("anthropic-beta"); got != "custom-beta,context-1m-2025-08-07" {
		t.Fatalf("Expected anyroute beta flag appended to configured header, got %q", got)
	}
}

func TestForwarder_CopyHeaders_AppendsAnyrouteBetaFlagToClientHeader(t *testing.T) {
	cfg := &config.Config{}
	endpointManager := endpoint.NewManager(cfg)
	forwarder := NewForwarder(cfg, endpointManager)

	ep := &endpoint.Endpoint{Config: config.EndpointConfig{
		Name:    "anyroute-endpoint",
		URL:     "https://anyrouter.top",
		Channel: "anyroute",
	}}
	srcReq := httptest.NewRequest("POST", "/v1/messages", nil)
	srcReq.Header.Set("anthropic-beta", "claude-code-20250219,interleaved-thinking-2025-05-14")
	dstReq := httptest.NewRequest("POST", "https://anyrouter.top/v1/messages", nil)

	forwarder.CopyHeaders(srcReq, dstReq, ep)

	want := "claude-code-20250219,interleaved-thinking-2025-05-14,context-1m-2025-08-07"
	if got := dstReq.Header.Get("anthropic-beta"); got != want {
		t.Fatalf("Expected anyroute beta flag appended to client header, got %q", got)
	}
}

func TestForwarder_CopyHeaders_DoesNotDuplicateAnyrouteBetaFlag(t *testing.T) {
	cfg := &config.Config{}
	endpointManager := endpoint.NewManager(cfg)
	forwarder := NewForwarder(cfg, endpointManager)

	ep := &endpoint.Endpoint{Config: config.EndpointConfig{
		Name:    "anyroute-endpoint",
		URL:     "https://anyrouter.top",
		Channel: "anyroute",
	}}
	srcReq := httptest.NewRequest("POST", "/v1/messages", nil)
	srcReq.Header.Set("anthropic-beta", "claude-code-20250219, Context-1M-2025-08-07")
	dstReq := httptest.NewRequest("POST", "https://anyrouter.top/v1/messages", nil)

	forwarder.CopyHeaders(srcReq, dstReq, ep)

	want := "claude-code-20250219, Context-1M-2025-08-07"
	if got := dstReq.Header.Get("anthropic-beta"); got != want {
		t.Fatalf("Expected anthropic-beta header unchanged when flag already present, got %q", got)
	}
}
