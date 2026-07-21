package proxy

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"cc-forwarder/internal/proxy/handlers"
)

func responseWithStatus(status int, headers map[string]string) *http.Response {
	resp := &http.Response{StatusCode: status, Header: http.Header{}}
	for key, value := range headers {
		resp.Header.Set(key, value)
	}
	return resp
}

func tracedState(wroteHeaders bool) *upstreamTraceState {
	state := &upstreamTraceState{}
	if wroteHeaders {
		state.markWroteHeaders()
	}
	return state
}

func TestDecideEndpointForwardOutcome_Table(t *testing.T) {
	cases := []struct {
		name             string
		forwardErr       error
		resp             *http.Response
		trace            *upstreamTraceState
		bodySample       string
		wantAction       EndpointForwardAction
		wantMark         EndpointFailureMark
		wantFailureClass handlers.FailureClass
		wantRetryAfter   time.Duration
	}{
		{
			name:       "P0 连接失败（WroteHeaders 前）重放安全换候选",
			forwardErr: errors.New("dial tcp: connection refused"),
			trace:      tracedState(false),
			wantAction: EndpointForwardNextCandidate,
			wantMark:   EndpointMarkFailureWindow,
		},
		{
			name:       "P0 本地准备失败（trace 为 nil）视为未发出",
			forwardErr: errors.New("failed to apply endpoint auth"),
			trace:      nil,
			wantAction: EndpointForwardNextCandidate,
			wantMark:   EndpointMarkFailureWindow,
		},
		{
			name:       "P0 WroteHeaders 后失败为歧义，穿透",
			forwardErr: errors.New("unexpected EOF"),
			trace:      tracedState(true),
			wantAction: EndpointForwardPassthroughError,
			wantMark:   EndpointMarkFailureWindow,
		},
		{
			name:       "401 鉴权失败换候选并长冷却",
			resp:       responseWithStatus(http.StatusUnauthorized, nil),
			trace:      tracedState(true),
			wantAction: EndpointForwardNextCandidate,
			wantMark:   EndpointMarkAuthCooldown,
		},
		{
			name:       "403 权限失败换候选并长冷却",
			resp:       responseWithStatus(http.StatusForbidden, nil),
			trace:      tracedState(true),
			wantAction: EndpointForwardNextCandidate,
			wantMark:   EndpointMarkAuthCooldown,
		},
		{
			name:       "429 无 Retry-After 换候选并短冷却",
			resp:       responseWithStatus(http.StatusTooManyRequests, nil),
			trace:      tracedState(true),
			wantAction: EndpointForwardNextCandidate,
			wantMark:   EndpointMarkRateLimitCooldown,
		},
		{
			name:           "429 带 Retry-After 使用响应头值",
			resp:           responseWithStatus(http.StatusTooManyRequests, map[string]string{"Retry-After": "5"}),
			trace:          tracedState(true),
			wantAction:     EndpointForwardNextCandidate,
			wantMark:       EndpointMarkRateLimitCooldown,
			wantRetryAfter: 5 * time.Second,
		},
		{
			name:             "413 换候选并写负缓存",
			resp:             responseWithStatus(http.StatusRequestEntityTooLarge, nil),
			trace:            tracedState(true),
			wantAction:       EndpointForwardNextCandidate,
			wantMark:         EndpointMarkNegativeCache,
			wantFailureClass: handlers.FailureClassPayloadTooLarge,
		},
		{
			name:             "400 模型不支持换候选并写负缓存",
			resp:             responseWithStatus(http.StatusBadRequest, nil),
			trace:            tracedState(true),
			bodySample:       `{"error":{"message":"Model not found: gpt-x"}}`,
			wantAction:       EndpointForwardNextCandidate,
			wantMark:         EndpointMarkNegativeCache,
			wantFailureClass: handlers.FailureClassModelUnsupported,
		},
		{
			name:             "400 schema 不兼容换候选并写负缓存",
			resp:             responseWithStatus(http.StatusBadRequest, nil),
			trace:            tracedState(true),
			bodySample:       `{"error":{"message":"extra inputs are not permitted"}}`,
			wantAction:       EndpointForwardNextCandidate,
			wantMark:         EndpointMarkNegativeCache,
			wantFailureClass: handlers.FailureClassSchemaIncompatible,
		},
		{
			name:       "404 普通客户端错误原样透传不记录",
			resp:       responseWithStatus(http.StatusNotFound, nil),
			trace:      tracedState(true),
			wantAction: EndpointForwardPassthroughRaw,
			wantMark:   EndpointMarkNone,
		},
		{
			name:       "500 服务器错误穿透并计失败窗口",
			resp:       responseWithStatus(http.StatusInternalServerError, nil),
			trace:      tracedState(true),
			wantAction: EndpointForwardPassthroughError,
			wantMark:   EndpointMarkFailureWindow,
		},
		{
			name:       "503 服务器错误穿透并计失败窗口",
			resp:       responseWithStatus(http.StatusServiceUnavailable, nil),
			trace:      tracedState(true),
			wantAction: EndpointForwardPassthroughError,
			wantMark:   EndpointMarkFailureWindow,
		},
		{
			name:       "200 进入响应处理",
			resp:       responseWithStatus(http.StatusOK, nil),
			trace:      tracedState(true),
			wantAction: EndpointForwardProcess,
			wantMark:   EndpointMarkNone,
		},
		{
			name:       "无错误无响应按歧义保守处理",
			resp:       nil,
			trace:      tracedState(true),
			wantAction: EndpointForwardPassthroughError,
			wantMark:   EndpointMarkFailureWindow,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := decideEndpointForwardOutcome(tc.forwardErr, tc.resp, tc.trace, tc.bodySample)
			if decision.Action != tc.wantAction {
				t.Fatalf("action: got %v want %v (reason=%s)", decision.Action, tc.wantAction, decision.Reason)
			}
			if decision.Mark != tc.wantMark {
				t.Fatalf("mark: got %v want %v (reason=%s)", decision.Mark, tc.wantMark, decision.Reason)
			}
			if decision.FailureClass != tc.wantFailureClass {
				t.Fatalf("failureClass: got %q want %q", decision.FailureClass, tc.wantFailureClass)
			}
			if decision.RetryAfter != tc.wantRetryAfter {
				t.Fatalf("retryAfter: got %v want %v", decision.RetryAfter, tc.wantRetryAfter)
			}
		})
	}
}

func TestEndpointRateLimitCooldown(t *testing.T) {
	withHeader := EndpointFailureDecision{Mark: EndpointMarkRateLimitCooldown, RetryAfter: 5 * time.Second}
	if got := endpointRateLimitCooldown(withHeader); got != 5*time.Second {
		t.Fatalf("expected Retry-After driven cooldown, got %v", got)
	}
	withoutHeader := EndpointFailureDecision{Mark: EndpointMarkRateLimitCooldown}
	if got := endpointRateLimitCooldown(withoutHeader); got != defaultEndpointRateLimitCooldown {
		t.Fatalf("expected default rate limit cooldown, got %v", got)
	}
	if endpointAuthCooldown() != defaultEndpointAuthCooldown {
		t.Fatalf("unexpected auth cooldown")
	}
}
