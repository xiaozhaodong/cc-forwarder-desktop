package proxy

import (
	"context"
	"math"
	"net/http"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/proxy/handlers"
)

// RetryManager 重试管理器
// 负责重试算法逻辑、基于错误分类的重试决策、指数退避延迟计算
// 不涉及状态管理和数据库操作
type RetryManager struct {
	config        *config.Config
	errorRecovery *ErrorRecoveryManager
	endpointMgr   *endpoint.Manager
}

// NewRetryManager 创建重试管理器
func NewRetryManager(cfg *config.Config, errorRecovery *ErrorRecoveryManager, endpointMgr *endpoint.Manager) *RetryManager {
	return &RetryManager{
		config:        cfg,
		errorRecovery: errorRecovery,
		endpointMgr:   endpointMgr,
	}
}

// ShouldRetry 基于错误分类的重试决策
// 参数:
//   - errorCtx: 错误上下文信息
//   - attempt: 当前尝试次数（从1开始）
//
// 返回:
//   - bool: 是否应该重试
//   - time.Duration: 重试延迟时间
func (rm *RetryManager) ShouldRetry(errorCtx *handlers.ErrorContext, attempt int) (bool, time.Duration) {
	// 超过最大重试次数
	if attempt >= rm.config.Retry.MaxAttempts {
		return false, 0
	}

	// 基于错误类型判断
	switch errorCtx.ErrorType {
	case handlers.ErrorTypeNetwork, handlers.ErrorTypeConnectionTimeout, handlers.ErrorTypeServerError:
		// 网络错误、连接超时、服务器错误可重试
		return true, rm.calculateBackoff(attempt)
	case handlers.ErrorTypeEOF, handlers.ErrorTypeResponseTimeout, handlers.ErrorTypeTimeout:
		// EOF、响应超时不可重试（避免重复计费）
		return false, 0
	case handlers.ErrorTypeHTTP, handlers.ErrorTypeClientCancel:
		// HTTP错误（4xx）、客户端取消不可重试
		return false, 0
	case handlers.ErrorTypeAuth:
		// 2025-12-10: 认证/权限错误不在同一端点重试（故障转移在 ShouldRetryWithDecision 处理）
		return false, 0
	case handlers.ErrorTypeRateLimit:
		// 限流错误可重试，但使用更长的延迟
		return true, rm.calculateRateLimitBackoff(attempt)
	case handlers.ErrorTypeStream:
		// 流处理错误不可重试（数据已部分发送）
		return false, 0
	case handlers.ErrorTypeParsing:
		// 解析错误可重试
		return true, rm.calculateBackoff(attempt)
	case handlers.ErrorTypeNoHealthyEndpoints:
		// 健康检查限制错误 - 特殊策略：允许至少一次实际转发尝试，忽略健康检查状态
		if attempt < 1 {
			return true, 0 // 立即重试，不延迟
		}
		return false, 0 // 只允许一次尝试
	default:
		// 未知错误不重试（保守策略）
		return false, 0
	}
}

// GetHealthyEndpoints 获取健康端点列表
func (rm *RetryManager) GetHealthyEndpoints(ctx context.Context) []*endpoint.Endpoint {
	// 如果启用了快速测试且策略为fastest，使用实时测试结果
	if rm.endpointMgr.GetConfig().Strategy.Type == "fastest" && rm.endpointMgr.GetConfig().Strategy.FastTestEnabled {
		return rm.endpointMgr.GetFastestEndpointsWithRealTimeTest(ctx)
	}
	// 否则返回健康的端点
	return rm.endpointMgr.GetHealthyEndpoints()
}

// calculateBackoff 计算指数退避延迟
func (rm *RetryManager) calculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return rm.config.Retry.BaseDelay
	}

	baseDelay := rm.config.Retry.BaseDelay
	maxDelay := rm.config.Retry.MaxDelay
	multiplier := rm.config.Retry.Multiplier

	// 指数退避: baseDelay * (multiplier ^ (attempt-1))
	delay := time.Duration(float64(baseDelay) * math.Pow(multiplier, float64(attempt-1)))

	// 限制最大延迟
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// calculateRateLimitBackoff 计算限流错误的退避延迟
// 限流错误使用更保守的延迟策略
func (rm *RetryManager) calculateRateLimitBackoff(attempt int) time.Duration {
	baseDelay := rm.config.Retry.BaseDelay
	maxDelay := rm.config.Retry.MaxDelay

	// 限流错误使用更长的基础延迟
	rateLimitBaseDelay := baseDelay * 3

	// 限流错误的指数退避系数更大
	delay := time.Duration(float64(rateLimitBaseDelay) * math.Pow(2.5, float64(attempt-1)))

	// 限制最大延迟，但允许更长的等待时间
	rateLimitMaxDelay := maxDelay * 2
	if delay > rateLimitMaxDelay {
		delay = rateLimitMaxDelay
	}

	return delay
}

// GetMaxAttempts 获取最大重试次数
func (rm *RetryManager) GetMaxAttempts() int {
	return rm.config.Retry.MaxAttempts
}

// GetConfig 获取配置信息（用于测试）
func (rm *RetryManager) GetConfig() *config.Config {
	return rm.config
}

// GetErrorRecoveryManager 获取错误恢复管理器（用于测试）
func (rm *RetryManager) GetErrorRecoveryManager() *ErrorRecoveryManager {
	return rm.errorRecovery
}

// GetEndpointManager 获取端点管理器（用于测试）
func (rm *RetryManager) GetEndpointManager() *endpoint.Manager {
	return rm.endpointMgr
}

// ShouldRetryWithDecision 基于错误分类的详细重试决策
//
// 🎯 [请求级穿透架构] 2025-12-25
// 核心原则：每个客户端请求 = 1个上游请求
// - 不在请求内重试（避免计费不一致）
// - 错误直接返回给客户端，附带 Retry-After 头
// - 通过 FailureTracker 记录失败，在下一个请求时智能选择端点
//
// 参数:
//   - errorCtx: 错误上下文信息
//   - localAttempt: 当前端点的尝试次数（从1开始）
//   - globalAttempt: 全局尝试次数
//   - isStreaming: 是否为流式请求
//
// 返回:
//   - handlers.RetryDecision: 详细的重试决策信息
func (rm *RetryManager) ShouldRetryWithDecision(errorCtx *handlers.ErrorContext, localAttempt int, globalAttempt int, isStreaming bool) handlers.RetryDecision {
	// 如果没有错误上下文，默认不重试
	if errorCtx == nil {
		return handlers.RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "completed",
			Reason:            "没有错误，无需重试",
		}
	}

	// 使用 ErrorType 枚举进行类型安全的判断
	switch errorCtx.ErrorType {
	case handlers.ErrorTypeClientCancel:
		// 客户端取消错误 - 立即停止，不重试，不记录
		return handlers.RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "cancelled",
			Reason:            "客户端取消请求，立即停止",
			ShouldRecord:      false, // 客户端取消不是端点问题
		}

	case handlers.ErrorTypeEOF:
		// EOF 错误 - 连接中断，不重试（避免重复计费）
		// 🎯 passthrough: 返回给客户端，记录到 FailureTracker
		return handlers.RetryDecision{
			RetrySameEndpoint:  false,
			SwitchEndpoint:     false,
			SuspendRequest:     false,
			FinalStatus:        "eof_interrupted",
			Reason:             "EOF连接中断，返回客户端重试",
			ShouldRecord:       true,
			RetryAfterSeconds:  5,
		}

	case handlers.ErrorTypeResponseTimeout:
		// 响应超时 - 服务器可能已处理，不重试（避免重复计费）
		return handlers.RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "timeout",
			Reason:            "响应超时，不重试避免重复计费",
			ShouldRecord:      true,
		}

	case handlers.ErrorTypeTimeout:
		// 兼容旧代码的超时错误 - 按响应超时处理，不重试
		return handlers.RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "timeout",
			Reason:            "超时错误，不重试避免重复计费",
			ShouldRecord:      true,
		}

	case handlers.ErrorTypeConnectionTimeout:
		// 🎯 [请求级穿透] 连接超时 - 直接返回客户端，让 SDK 重试
		// 原逻辑：在请求内重试3次 → 新逻辑：passthrough + FailureTracker
		return handlers.RetryDecision{
			RetrySameEndpoint:  false,
			SwitchEndpoint:     false,
			SuspendRequest:     false,
			FinalStatus:        "connection_timeout",
			Reason:             "连接超时，返回客户端重试",
			ShouldRecord:       true, // 记录到 FailureTracker
			RetryAfterSeconds:  5,    // 建议客户端5秒后重试
		}

	case handlers.ErrorTypeNetwork:
		// 🎯 [请求级穿透] 网络错误 - 直接返回客户端，让 SDK 重试
		// 原逻辑：在请求内重试3次 → 新逻辑：passthrough + FailureTracker
		return handlers.RetryDecision{
			RetrySameEndpoint:  false,
			SwitchEndpoint:     false,
			SuspendRequest:     false,
			FinalStatus:        "network_error",
			Reason:             "网络错误，返回客户端重试",
			ShouldRecord:       true,
			RetryAfterSeconds:  5,
		}

	case handlers.ErrorTypeHTTP:
		// HTTP错误：通常是4xx错误，不应重试
		return handlers.RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "error",
			Reason:            "HTTP错误，无需重试",
			ShouldRecord:      false, // 4xx 是请求问题，不是端点问题
		}

	case handlers.ErrorTypeServerError:
		// 🎯 [请求级穿透] 服务器错误(5xx) - 直接返回客户端，让 SDK 重试
		// 原逻辑：在请求内重试3次 → 新逻辑：passthrough + FailureTracker
		return handlers.RetryDecision{
			RetrySameEndpoint:  false,
			SwitchEndpoint:     false,
			SuspendRequest:     false,
			FinalStatus:        "server_error",
			Reason:             "服务器错误，返回客户端重试",
			ShouldRecord:       true, // 记录到 FailureTracker
			RetryAfterSeconds:  5,    // 建议客户端5秒后重试
		}

	case handlers.ErrorTypeStream:
		// 流式错误：响应已接收但解析失败，不重试（数据已部分发送）
		return handlers.RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "stream_error",
			Reason:            "流式处理错误，不重试避免重复计费",
			ShouldRecord:      true,
		}

	case handlers.ErrorTypeAuth:
		// 🎯 [请求级穿透] 认证/权限错误(401/403) - 直接返回客户端
		// 原逻辑：切换端点重试 → 新逻辑：passthrough + FailureTracker
		return handlers.RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "auth_error",
			Reason:            "认证错误，返回客户端处理",
			ShouldRecord:      true, // 记录，下次选择时可能跳过该端点
		}

	case handlers.ErrorTypeRateLimit:
		// 🎯 [请求级穿透] 限流错误(429) - 直接返回客户端，让 SDK 重试
		// 原逻辑：在请求内重试3次+长延迟 → 新逻辑：passthrough + FailureTracker
		return handlers.RetryDecision{
			RetrySameEndpoint:  false,
			SwitchEndpoint:     false,
			SuspendRequest:     false,
			FinalStatus:        "rate_limited",
			Reason:             "限流错误，返回客户端重试",
			ShouldRecord:       true,
			RetryAfterSeconds:  30, // 限流建议较长的重试延迟
		}

	case handlers.ErrorTypeParsing:
		// 🎯 [请求级穿透] 解析错误 - 直接返回客户端
		// 原逻辑：切换端点重试 → 新逻辑：passthrough + FailureTracker
		return handlers.RetryDecision{
			RetrySameEndpoint:  false,
			SwitchEndpoint:     false,
			SuspendRequest:     false,
			FinalStatus:        "parsing_error",
			Reason:             "解析错误，返回客户端重试",
			ShouldRecord:       true,
			RetryAfterSeconds:  5,
		}

	case handlers.ErrorTypeNoHealthyEndpoints:
		// 没有健康端点可用 - 直接返回客户端
		// 客户端重试时，FailureTracker 可能已经刷新，端点可能恢复
		return handlers.RetryDecision{
			RetrySameEndpoint:  false,
			SwitchEndpoint:     false,
			SuspendRequest:     false,
			FinalStatus:        "no_healthy_endpoints",
			Reason:             "没有健康端点，返回客户端稍后重试",
			ShouldRecord:       false, // 这是整体状态，不是单个端点问题
			RetryAfterSeconds:  10,    // 建议等待端点恢复
		}

	default:
		// 未知错误：保守策略，不重试
		return handlers.RetryDecision{
			RetrySameEndpoint: false,
			SwitchEndpoint:    false,
			SuspendRequest:    false,
			FinalStatus:       "error",
			Reason:            "未知错误，保守策略不重试",
			ShouldRecord:      false,
		}
	}
}


// GetDefaultStatusCodeForFinalStatus 根据最终状态获取默认HTTP状态码
func GetDefaultStatusCodeForFinalStatus(finalStatus string) int {
	switch finalStatus {
	case "cancelled":
		return 499 // nginx风格的客户端取消码
	case "auth_error":
		return http.StatusUnauthorized
	case "rate_limited":
		return http.StatusTooManyRequests
	case "error":
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}
