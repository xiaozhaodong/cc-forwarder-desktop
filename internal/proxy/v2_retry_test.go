package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
)

// mockEndpointServer 模拟端点服务器，支持配置失败次数
type mockEndpointServer struct {
	server       *httptest.Server
	requestCount int
	failCount    int // 前N次请求返回错误，之后返回成功
	mu           sync.Mutex
}

func newMockEndpointServer(failCount int) *mockEndpointServer {
	mes := &mockEndpointServer{
		failCount: failCount,
	}

	mes.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mes.mu.Lock()
		mes.requestCount++
		currentCount := mes.requestCount
		mes.mu.Unlock()

		// 前failCount次请求返回错误
		if currentCount <= mes.failCount {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "mock server error"}`))
			return
		}

		// 成功响应 - 模拟流式响应
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, `data: {"type": "message_start", "message": {"model": "claude-3-5-haiku"}}`)
		fmt.Fprint(w, "\n\n")
		fmt.Fprint(w, "event: message_delta\n")
		fmt.Fprint(w, `data: {"type": "message_delta", "usage": {"input_tokens": 10, "output_tokens": 20}}`)
		fmt.Fprint(w, "\n\n")
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {}\n\n")
	}))

	return mes
}

func (mes *mockEndpointServer) getRequestCount() int {
	mes.mu.Lock()
	defer mes.mu.Unlock()
	return mes.requestCount
}

func (mes *mockEndpointServer) close() {
	mes.server.Close()
}

// TestV2StreamingRetryLogic 测试V2流式处理的重试逻辑
func TestV2StreamingRetryLogic(t *testing.T) {
	tests := []struct {
		name               string
		maxAttempts        int
		endpointFailCount  int
		expectedRetryCount int
		expectSuccess      bool
	}{
		{
			name:               "第一次尝试成功",
			maxAttempts:        3,
			endpointFailCount:  0,
			expectedRetryCount: 1,
			expectSuccess:      true,
		},
		{
			name:               "服务器错误返回客户端重试",
			maxAttempts:        3,
			endpointFailCount:  1,
			expectedRetryCount: 1,
			expectSuccess:      false,
		},
		{
			name:               "连续服务器错误返回客户端重试",
			maxAttempts:        3,
			endpointFailCount:  2,
			expectedRetryCount: 1,
			expectSuccess:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建模拟服务器
			mockServer := newMockEndpointServer(tt.endpointFailCount)
			defer mockServer.close()

			// 创建配置
			cfg := &config.Config{
				Retry: config.RetryConfig{
					MaxAttempts: tt.maxAttempts,
					BaseDelay:   50 * time.Millisecond, // 减少测试时间
					MaxDelay:    500 * time.Millisecond,
					Multiplier:  2.0,
				},
				Failover: config.FailoverConfig{
					Enabled: true,
				},
				Group: config.GroupConfig{
					AutoSwitchBetweenGroups: true,
				},
				UsageTracking: config.UsageTrackingConfig{
					Enabled: false, // 简化测试
				},
				Endpoints: []config.EndpointConfig{
					{
						Name:     "test-endpoint",
						URL:      mockServer.server.URL,
						Priority: 1,
						Timeout:  5 * time.Second,
						Group:    "test-group",
						Token:    "test-token",
					},
				},
			}

			// 创建端点管理器
			endpointManager := endpoint.NewManager(cfg)

			// 创建处理器
			handler := NewHandler(endpointManager, cfg)

			// 创建测试请求
			requestBody := `{"message": "test streaming request"}`
			req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(requestBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "text/event-stream") // 标记为流式请求

			// 创建响应记录器
			recorder := httptest.NewRecorder()

			// 执行请求（带超时上下文）
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			req = req.WithContext(ctx)

			// 执行处理
			start := time.Now()
			handler.ServeHTTP(recorder, req)
			duration := time.Since(start)

			// 获取实际的请求次数
			actualRetryCount := mockServer.getRequestCount()

			t.Logf("测试 %s:", tt.name)
			t.Logf("  配置最大重试次数: %d", tt.maxAttempts)
			t.Logf("  端点失败次数: %d", tt.endpointFailCount)
			t.Logf("  预期重试次数: %d", tt.expectedRetryCount)
			t.Logf("  实际重试次数: %d", actualRetryCount)
			t.Logf("  响应状态码: %d", recorder.Code)
			t.Logf("  处理时长: %v", duration)
			t.Logf("  响应体: %s", recorder.Body.String())

			// 验证重试次数
			if actualRetryCount != tt.expectedRetryCount {
				t.Errorf("重试次数不符合预期: 预期 %d, 实际 %d", tt.expectedRetryCount, actualRetryCount)
			}

			// 验证最终结果
			if tt.expectSuccess {

				if recorder.Code != http.StatusOK {
					t.Errorf("预期成功但失败: 状态码 %d, 响应: %s", recorder.Code, recorder.Body.String())
				}

				responseBody := recorder.Body.String()
				if !strings.Contains(responseBody, "event: message_start") {
					t.Errorf("成功响应应包含流式数据，但没有找到: %s", responseBody)
				}
			} else {
				responseBody := recorder.Body.String()
				if recorder.Code == http.StatusOK && !strings.Contains(responseBody, "error") {
					t.Errorf("预期失败但成功: 状态码 %d, 响应: %s", recorder.Code, responseBody)
				}

				if recorder.Code != http.StatusInternalServerError {
					t.Errorf("预期返回500: 状态码 %d, 响应: %s", recorder.Code, responseBody)
				}
				if !strings.Contains(responseBody, "data: error:") {
					t.Errorf("预期返回SSE error行: %s", responseBody)
				}
			}
		})
	}
}

func TestV2StreamingRetryWithMultipleEndpoints(t *testing.T) {
	mockServer1 := newMockEndpointServer(3)
	defer mockServer1.close()

	mockServer2 := newMockEndpointServer(0)
	defer mockServer2.close()

	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   50 * time.Millisecond,
			MaxDelay:    500 * time.Millisecond,
			Multiplier:  2.0,
		},
		Failover: config.FailoverConfig{
			Enabled: true,
		},
		Group: config.GroupConfig{
			AutoSwitchBetweenGroups: true,
		},
		UsageTracking: config.UsageTrackingConfig{
			Enabled: false,
		},
		Endpoints: []config.EndpointConfig{
			{
				Name:     "endpoint-1",
				URL:      mockServer1.server.URL,
				Priority: 1,
				Timeout:  5 * time.Second,
				Group:    "test-group",
				Token:    "test-token-1",
			},
			{
				Name:     "endpoint-2",
				URL:      mockServer2.server.URL,
				Priority: 2,
				Timeout:  5 * time.Second,
				Group:    "test-group",
				Token:    "test-token-2",
			},
		},
	}

	endpointManager := endpoint.NewManager(cfg)

	handler := NewHandler(endpointManager, cfg)

	requestBody := `{"message": "test multi-endpoint retry"}`
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(recorder, req)
	duration := time.Since(start)

	endpoint1Requests := mockServer1.getRequestCount()
	endpoint2Requests := mockServer2.getRequestCount()

	t.Logf("多端点重试测试结果:")
	t.Logf("  端点1请求次数: %d", endpoint1Requests)
	t.Logf("  端点2请求次数: %d", endpoint2Requests)
	t.Logf("  最终状态码: %d", recorder.Code)
	t.Logf("  处理时长: %v", duration)
	t.Logf("  响应体: %s", recorder.Body.String())

	if endpoint1Requests != 1 {
		t.Errorf("端点1请求次数错误: 预期 1, 实际 %d", endpoint1Requests)
	}
	if endpoint2Requests != 0 {
		t.Errorf("端点2请求次数错误: 预期 0, 实际 %d", endpoint2Requests)
	}

	responseBody := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("预期返回500: 状态码 %d, 响应: %s", recorder.Code, responseBody)
	}
	if !strings.Contains(responseBody, "data: error:") {
		t.Errorf("预期返回SSE error行: %s", responseBody)
	}
	if strings.Contains(responseBody, "event: message_start") {
		t.Errorf("预期不返回message_start: %s", responseBody)
	}
}
