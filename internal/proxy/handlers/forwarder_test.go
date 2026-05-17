package handlers

import (
	"bytes"
	"context"
	"encoding/json"
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
					"scope": "conversation"
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
							"scope": "conversation"
						}
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
			got := prepareBodyForEndpoint(bodyBytes, tt.ep)

			var payload any
			if err := json.Unmarshal(got, &payload); err != nil {
				t.Fatalf("sanitized body should still be JSON: %v", err)
			}

			cacheControlCount, scopeCount := countCacheControlScopes(payload)
			if cacheControlCount != 2 {
				t.Fatalf("expected 2 cache_control objects, got %d", cacheControlCount)
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

func TestForwarder_PrepareBodyForEndpoint_KeepsOtherChannelsUntouched(t *testing.T) {
	bodyBytes := []byte(`{"system":[{"cache_control":{"type":"ephemeral","scope":"conversation"}}]}`)
	ep := &endpoint.Endpoint{Config: config.EndpointConfig{Name: "coderelay", Channel: "custom"}}

	got := prepareBodyForEndpoint(bodyBytes, ep)

	if !bytes.Equal(got, bodyBytes) {
		t.Fatalf("expected non-coderelay channel body to stay untouched, got %s", string(got))
	}
}

func countCacheControlScopes(value any) (int, int) {
	cacheControlCount := 0
	scopeCount := 0

	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if key == cacheControlKey {
				if cacheControl, ok := child.(map[string]any); ok {
					cacheControlCount++
					if _, exists := cacheControl[cacheControlScopeKey]; exists {
						scopeCount++
					}
				}
			}

			childCacheControlCount, childScopeCount := countCacheControlScopes(child)
			cacheControlCount += childCacheControlCount
			scopeCount += childScopeCount
		}
	case []any:
		for _, child := range node {
			childCacheControlCount, childScopeCount := countCacheControlScopes(child)
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

func TestForwarder_CopyHeaders_PreservesConfiguredAnthropicBetaHeader(t *testing.T) {
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

	if got := dstReq.Header.Get("anthropic-beta"); got != "custom-beta" {
		t.Fatalf("Expected configured anthropic-beta header to be preserved, got %q", got)
	}
}
