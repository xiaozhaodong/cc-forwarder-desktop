package handlers

import (
	"time"
)

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
