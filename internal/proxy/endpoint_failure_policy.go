package proxy

import (
	"net/http"
	"strings"
	"time"

	"cc-forwarder/internal/proxy/handlers"
)

// 端点侧统一故障决策表（方案 §3.1）。
// 纯决策，不执行任何副作用；标记与写回由转发管线按 Mark 执行（Phase 4 接线）。

// EndpointForwardAction 决策动作
type EndpointForwardAction int

const (
	// EndpointForwardProcess 2xx/3xx：进入响应处理阶段
	EndpointForwardProcess EndpointForwardAction = iota
	// EndpointForwardNextCandidate 重放安全：换下一候选
	EndpointForwardNextCandidate
	// EndpointForwardPassthroughError 歧义失败/5xx：以错误语义回客户端
	//（「真实码→500+Retry-After」转换仅发生在此动作的写回点）
	EndpointForwardPassthroughError
	// EndpointForwardPassthroughRaw 其余 4xx：原样透传上游响应
	EndpointForwardPassthroughRaw
)

// EndpointFailureMark 端点状态标记类别
type EndpointFailureMark int

const (
	// EndpointMarkNone 不记录
	EndpointMarkNone EndpointFailureMark = iota
	// EndpointMarkFailureWindow failCount++，达阈值进入冷却
	EndpointMarkFailureWindow
	// EndpointMarkAuthCooldown 鉴权类长冷却（默认 30m）
	EndpointMarkAuthCooldown
	// EndpointMarkRateLimitCooldown 限流短冷却 = Retry-After（无头默认 60s)；不计故障阈值
	EndpointMarkRateLimitCooldown
	// EndpointMarkNegativeCache 写路由负缓存（FailureClass），不计失败窗口
	EndpointMarkNegativeCache
)

const (
	defaultEndpointAuthCooldown      = 30 * time.Minute
	defaultEndpointRateLimitCooldown = 60 * time.Second
)

// EndpointFailureDecision 一次转发结果的决策
type EndpointFailureDecision struct {
	Action       EndpointForwardAction
	Mark         EndpointFailureMark
	FailureClass handlers.FailureClass // Mark 为 NegativeCache 时有效
	RetryAfter   time.Duration         // 429 时解析的 Retry-After（0 = 未提供，用默认）
	Reason       string
}

// decideEndpointForwardOutcome 按 §3.1 决策表对一次端点转发结果分类。
//   - forwardErr 非 nil 时为 P0 连接阶段：以 trace.WroteHeaders 分界重放安全/歧义
//   - resp 非 nil 时为 P1 响应头阶段：按状态码区分「确定未执行」与「歧义」
//   - respBodySample 供 4xx 的模型不支持 / schema 不兼容文本判定（调用方窥读并复原 body）
func decideEndpointForwardOutcome(forwardErr error, resp *http.Response, trace *upstreamTraceState, respBodySample string) EndpointFailureDecision {
	if forwardErr != nil {
		if !trace.WroteHeaders() {
			return EndpointFailureDecision{
				Action: EndpointForwardNextCandidate,
				Mark:   EndpointMarkFailureWindow,
				Reason: "connection_failed_before_wrote_headers",
			}
		}
		return EndpointFailureDecision{
			Action: EndpointForwardPassthroughError,
			Mark:   EndpointMarkFailureWindow,
			Reason: "ambiguous_failure_after_wrote_headers",
		}
	}

	if resp == nil {
		// 无错误也无响应：按歧义失败保守处理
		return EndpointFailureDecision{
			Action: EndpointForwardPassthroughError,
			Mark:   EndpointMarkFailureWindow,
			Reason: "empty_response",
		}
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return EndpointFailureDecision{
			Action: EndpointForwardNextCandidate,
			Mark:   EndpointMarkAuthCooldown,
			Reason: "auth_rejected",
		}
	case resp.StatusCode == http.StatusTooManyRequests:
		return EndpointFailureDecision{
			Action:     EndpointForwardNextCandidate,
			Mark:       EndpointMarkRateLimitCooldown,
			RetryAfter: parseAccountRetryAfter(resp),
			Reason:     "rate_limited",
		}
	case resp.StatusCode == http.StatusRequestEntityTooLarge:
		return EndpointFailureDecision{
			Action:       EndpointForwardNextCandidate,
			Mark:         EndpointMarkNegativeCache,
			FailureClass: handlers.FailureClassPayloadTooLarge,
			Reason:       "payload_too_large",
		}
	case resp.StatusCode >= http.StatusInternalServerError:
		return EndpointFailureDecision{
			Action: EndpointForwardPassthroughError,
			Mark:   EndpointMarkFailureWindow,
			Reason: "server_error",
		}
	case resp.StatusCode >= http.StatusBadRequest:
		lowerBody := normalizeEndpointFailureBody(respBodySample)
		if isModelUnsupportedError(lowerBody) {
			return EndpointFailureDecision{
				Action:       EndpointForwardNextCandidate,
				Mark:         EndpointMarkNegativeCache,
				FailureClass: handlers.FailureClassModelUnsupported,
				Reason:       "model_unsupported",
			}
		}
		if isSchemaIncompatibleError(lowerBody) {
			return EndpointFailureDecision{
				Action:       EndpointForwardNextCandidate,
				Mark:         EndpointMarkNegativeCache,
				FailureClass: handlers.FailureClassSchemaIncompatible,
				Reason:       "schema_incompatible",
			}
		}
		return EndpointFailureDecision{
			Action: EndpointForwardPassthroughRaw,
			Mark:   EndpointMarkNone,
			Reason: "client_error_passthrough",
		}
	default:
		return EndpointFailureDecision{
			Action: EndpointForwardProcess,
			Mark:   EndpointMarkNone,
			Reason: "process_response",
		}
	}
}

// endpointAuthCooldown 鉴权类冷却时长
func endpointAuthCooldown() time.Duration {
	return defaultEndpointAuthCooldown
}

// endpointRateLimitCooldown 限流冷却时长：优先 Retry-After，无头用默认
func endpointRateLimitCooldown(decision EndpointFailureDecision) time.Duration {
	if decision.RetryAfter > 0 {
		return decision.RetryAfter
	}
	return defaultEndpointRateLimitCooldown
}

func normalizeEndpointFailureBody(body string) string {
	return strings.ToLower(body)
}
