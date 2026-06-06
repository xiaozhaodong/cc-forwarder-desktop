package handlers

import (
	"context"
	"net/http"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/tracking"
)

// SuspensionResult 挂起等待结果类型
// 用于区分WaitForEndpointRecovery的不同结果状态
type SuspensionResult int

const (
	SuspensionSuccess   SuspensionResult = iota // 成功恢复
	SuspensionTimeout                           // 等待超时
	SuspensionCancelled                         // 用户取消
)

// String 返回SuspensionResult的字符串表示
func (sr SuspensionResult) String() string {
	switch sr {
	case SuspensionSuccess:
		return "success"
	case SuspensionTimeout:
		return "timeout"
	case SuspensionCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// RequestLifecycleManager 请求生命周期管理器接口
// 修改版本：添加CompleteRequest和HandleNonTokenResponse方法以支持生命周期管理器架构
type RequestLifecycleManager interface {
	GetRequestID() string
	SetEndpoint(name, group, channel string)
	SetModel(modelName string)                       // 简单设置模型
	SetModelWithComparison(modelName, source string) // 带对比的设置模型
	HasModel() bool                                  // 检查是否已有模型
	UpdateStatus(status string, endpointIndex, statusCode int)
	HandleError(err error)
	PrepareErrorContext(errorCtx *ErrorContext)
	// 新增方法：统一的请求完成入口
	CompleteRequest(tokens *tracking.TokenUsage)
	// 🆕 [流完整性追踪] 2025-12-11: 带数据质量标记的请求完成
	CompleteRequestWithQuality(tokens *tracking.TokenUsage, failureReason string)
	RecordFirstToken()
	HandleNonTokenResponse(responseContent string)
	// 失败请求Token记录方法：只记录Token统计，不改变请求状态
	RecordTokensForFailedRequest(tokens *tracking.TokenUsage, failureReason string)
	// 🔢 [语义修复] 新增尝试计数管理方法
	IncrementAttempt() int // 线程安全地增加尝试计数，返回当前计数
	GetAttemptCount() int  // 线程安全地获取当前尝试次数
	// 🚀 [状态机重构] Phase 4: 新增状态管理方法
	MapErrorTypeToFailureReason(errorType ErrorType) string         // 映射ErrorType到failure_reason
	FailRequest(failureReason, errorDetail string, httpStatus int)  // 标记请求为最终失败
	CancelRequest(cancelReason string, tokens *tracking.TokenUsage) // 标记请求被取消
}

// ErrorRecoveryManager 错误恢复管理器接口
type ErrorRecoveryManager interface {
	ClassifyError(err error, connID, endpointName, groupName string, attemptCount int) ErrorContext
	HandleFinalFailure(errorCtx ErrorContext)
	GetErrorTypeName(errorType ErrorType) string
}

// ErrorContext 错误上下文信息
type ErrorContext struct {
	RequestID      string
	EndpointName   string
	GroupName      string
	AttemptCount   int
	ErrorType      ErrorType
	OriginalError  error
	RetryableAfter time.Duration
	MaxRetries     int
}

// ErrorType 错误类型枚举
// 注意：值顺序必须与 proxy/error_recovery.go 中的 ErrorType 完全一致
type ErrorType int

const (
	ErrorTypeUnknown            ErrorType = iota // 0: 未知错误
	ErrorTypeNetwork                             // 1: 网络错误（连接失败等，可重试）
	ErrorTypeEOF                                 // 2: EOF 错误（连接中断，不可重试，避免重复计费）
	ErrorTypeConnectionTimeout                   // 3: 连接超时（可重试，未开始处理）
	ErrorTypeResponseTimeout                     // 4: 响应超时（不可重试，可能已计费）
	ErrorTypeTimeout                             // 5: 超时错误（兼容旧代码）
	ErrorTypeHTTP                                // 6: HTTP错误
	ErrorTypeServerError                         // 7: 服务器错误（5xx）
	ErrorTypeStream                              // 8: 流式处理错误
	ErrorTypeAuth                                // 9: 认证错误
	ErrorTypeRateLimit                           // 10: 限流错误
	ErrorTypeParsing                             // 11: 解析错误
	ErrorTypeClientCancel                        // 12: 客户端取消错误
	ErrorTypeNoHealthyEndpoints                  // 13: 没有健康端点可用
)

type FailureClass string

const (
	FailureClassNone                   FailureClass = ""
	FailureClassClientCancel           FailureClass = "client_cancel"
	FailureClassModelUnsupported       FailureClass = "model_unsupported"
	FailureClassSchemaIncompatible     FailureClass = "schema_incompatible"
	FailureClassPayloadTooLarge        FailureClass = "payload_too_large"
	FailureClassCountTokensUnsupported FailureClass = "count_tokens_unsupported"
)

// StreamIncompleteErrorInterface 流不完整错误接口
// 🆕 [流完整性追踪] 2025-12-11
// 用于跨包检查流不完整错误，避免循环导入
type StreamIncompleteErrorInterface interface {
	error
	GetFailureReason() string // 获取 failure_reason（incomplete_stream 或 stream_truncated）
	GetModelName() string     // 获取模型名称
	GetReason() string        // 获取不完整的原因（用于日志）
}

// TokenParser Token解析器接口
type TokenParser interface {
	ParseSSELine(line string) *monitor.TokenUsage // 返回TokenUsage类型
	SetModelName(model string)
}

// StreamProcessor 流式处理器接口
// 修改版本：返回Token使用信息和模型名称而非直接记录到usageTracker
type StreamProcessor interface {
	ProcessStreamWithRetry(ctx context.Context, resp *http.Response) (*tracking.TokenUsage, string, error)
	EnableDownstreamTailDrain(timeout time.Duration, cancelUpstream context.CancelFunc)
	SetFirstTokenRecorder(recorder func())
}

// RetryHandler 重试处理器接口
type RetryHandler interface {
	ExecuteWithContext(ctx context.Context, operation func(*endpoint.Endpoint, string) (*http.Response, error), connID string) (*http.Response, error)
	ShouldSuspendRequest(ctx context.Context) bool
	WaitForGroupSwitch(ctx context.Context, connID string) bool
	SetEndpointManager(manager interface{})
	SetUsageTracker(tracker *tracking.UsageTracker)
}

// TokenParserFactory Token解析器工厂接口
type TokenParserFactory interface {
	NewTokenParserWithUsageTracker(connID string, usageTracker *tracking.UsageTracker) TokenParser
}

// StreamProcessorFactory 流式处理器工厂接口
type StreamProcessorFactory interface {
	NewStreamProcessor(tokenParser TokenParser, usageTracker *tracking.UsageTracker,
		w http.ResponseWriter, flusher http.Flusher, requestID, endpoint string) StreamProcessor
}

// ErrorRecoveryFactory 错误恢复管理器工厂接口
type ErrorRecoveryFactory interface {
	NewErrorRecoveryManager(usageTracker *tracking.UsageTracker) ErrorRecoveryManager
}

// RetryHandlerFactory 重试处理器工厂接口
type RetryHandlerFactory interface {
	NewRetryHandler(config interface{}) RetryHandler
}

// TokenAnalyzer Token分析器接口
type TokenAnalyzer interface {
	AnalyzeResponseForTokens(ctx context.Context, responseBody, endpointName string, r *http.Request)
	AnalyzeResponseForTokensUnified(responseBytes []byte, connID, endpointName string) (*tracking.TokenUsage, string)
}

// ResponseProcessor 响应处理器接口
type ResponseProcessor interface {
	CopyResponseHeaders(resp *http.Response, w http.ResponseWriter)
	ProcessResponseBody(resp *http.Response) ([]byte, error)
	ReadAndDecompressResponse(ctx context.Context, resp *http.Response, endpointName string) ([]byte, error)
}

// TokenAnalyzerFactory Token分析器工厂接口
type TokenAnalyzerFactory interface {
	NewTokenAnalyzer(usageTracker *tracking.UsageTracker) TokenAnalyzer
}

// ResponseProcessorFactory 响应处理器工厂接口
type ResponseProcessorFactory interface {
	NewResponseProcessor() ResponseProcessor
}

// RetryManagerFactory 重试管理器工厂接口
type RetryManagerFactory interface {
	NewRetryManager() RetryManager
}

// SuspensionManagerFactory 挂起管理器工厂接口
type SuspensionManagerFactory interface {
	NewSuspensionManager() SuspensionManager
}

// RetryDecision 统一重试决策结果
// 包含重试策略的完整决策信息，用于替代原有的复杂RetryController机制
//
// 🎯 [请求级穿透架构] 2025-12-25
// 新增 ShouldRecord 和 RetryAfterSeconds 字段，支持：
// - 将错误直接返回给客户端（Claude Code SDK 自行重试）
// - 通过 FailureTracker 滑动窗口记录端点失败
// - 在下一个请求时智能跳过故障端点
type RetryDecision struct {
	RetrySameEndpoint bool          // 是否继续在当前端点重试（passthrough架构下通常为false）
	SwitchEndpoint    bool          // 是否切换到下一端点（passthrough架构下通常为false）
	SuspendRequest    bool          // 是否尝试挂起请求
	Delay             time.Duration // 重试延迟时间（仅在请求内重试时使用）
	FinalStatus       string        // 若终止，应记录的最终状态
	Reason            string        // 决策原因（用于日志）
	FailureClass      FailureClass  // 失败分类（用于路由状态和负向缓存）
	ShouldRecord      bool          // 是否应记录到 FailureTracker（用于后续请求的端点选择）
	RetryAfterSeconds int           // 建议客户端重试的延迟秒数（通过 Retry-After 响应头返回）
}

// RetryManager 重试管理器接口
type RetryManager interface {
	ShouldRetry(errorCtx *ErrorContext, attempt int) (bool, time.Duration)
	GetHealthyEndpoints(ctx context.Context) []*endpoint.Endpoint
	GetMaxAttempts() int
	// ShouldRetryWithDecision 统一重试决策方法
	// 完全复制retry/policy.go的决策逻辑，确保行为一致
	// errorCtx: 错误上下文信息
	// localAttempt: 当前端点的尝试次数（从1开始，用于退避计算）
	// globalAttempt: 全局尝试次数（用于限流策略）
	// isStreaming: 是否为流式请求
	ShouldRetryWithDecision(errorCtx *ErrorContext, localAttempt int, globalAttempt int, isStreaming bool) RetryDecision
}

// SuspensionManager 挂起管理器接口
type SuspensionManager interface {
	ShouldSuspend(ctx context.Context) bool
	WaitForGroupSwitch(ctx context.Context, connID string) bool
	WaitForEndpointRecovery(ctx context.Context, connID, failedEndpoint string) bool // 🚀 [端点自愈] 新增端点恢复等待方法
	// 🎯 [挂起取消区分] 新增带结果的端点恢复等待方法，能区分成功/超时/取消
	WaitForEndpointRecoveryWithResult(ctx context.Context, connID, failedEndpoint string) SuspensionResult
	GetSuspendedRequestsCount() int
	// 🔧 [热更新] 更新配置
	UpdateConfig(cfg *config.Config)
}

// GetDefaultStatusCodeForFinalStatus 根据最终状态获取默认HTTP状态码
// 用于在RetryDecision中没有明确状态码时提供合理默认值
//
// 工具函数签名（应在具体实现中定义）:
// func GetDefaultStatusCodeForFinalStatus(finalStatus string) int
