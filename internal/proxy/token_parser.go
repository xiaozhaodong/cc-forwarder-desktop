package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cc-forwarder/internal/monitor"
	"cc-forwarder/internal/tracking"
)

// ParseResult 解析结果结构体
// 用于将Token解析与状态记录分离，支持职责纯化
type ParseResult struct {
	TokenUsage  *tracking.TokenUsage
	ModelName   string
	ErrorInfo   *ErrorInfo
	IsCompleted bool
	Status      string
}

// ErrorInfo 错误信息结构体
type ErrorInfo struct {
	Type    string
	Message string
}

// TokenParserInterface 统一的Token解析接口
// 根据STREAMING_REFACTOR_PROPOSAL.md方案设计
type TokenParserInterface interface {
	ParseMessageStart(line string) *ModelInfo
	ParseMessageDelta(line string) *tracking.TokenUsage
	SetModel(modelName string)
	GetFinalUsage() *tracking.TokenUsage
	Reset()

	// V2 职责纯化方法
	ParseSSELineV2(line string) *ParseResult
}

// ModelInfo 模型信息结构体
type ModelInfo struct {
	Model string `json:"model"`
}

// UsageData 表示Claude API SSE事件中的usage字段
type UsageData struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"` // 总数（向后兼容）
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	// v5.0+: 分开的缓存创建详情
	CacheCreation *CacheCreationDetail `json:"cache_creation,omitempty"`
}

// CacheCreationDetail 表示缓存创建详情（分开的 5m/1h）
type CacheCreationDetail struct {
	Ephemeral5mInputTokens int64 `json:"ephemeral_5m_input_tokens"` // 5分钟缓存 tokens
	Ephemeral1hInputTokens int64 `json:"ephemeral_1h_input_tokens"` // 1小时缓存 tokens
}

// MessageStartData 表示message_start事件中的message对象
type MessageStartData struct {
	ID      string        `json:"id"`
	Type    string        `json:"type"`
	Role    string        `json:"role"`
	Model   string        `json:"model"`
	Content []interface{} `json:"content"`
	Usage   *UsageData    `json:"usage,omitempty"`
}

// MessageStart 表示message_start事件的结构
type MessageStart struct {
	Type    string            `json:"type"`
	Message *MessageStartData `json:"message"`
}

// MessageDelta 表示message_delta事件的结构
type MessageDelta struct {
	Type  string      `json:"type"`
	Delta interface{} `json:"delta"`
	Usage *UsageData  `json:"usage,omitempty"`
}

// SSEErrorData 表示SSE流中error事件的结构
type SSEErrorData struct {
	Type  string `json:"type"`
	Error struct {
		Type      string `json:"type"`
		Message   string `json:"message"`
		RequestID string `json:"request_id,omitempty"`
	} `json:"error"`
}

// 请求处理状态常量
const (
	StatusCompleted    = "completed"     // 真正成功（有Token或正常响应）
	StatusErrorAPI     = "error_api"     // API层错误（overloaded等）
	StatusErrorNetwork = "error_network" // 网络层错误（超时等）
	StatusProcessing   = "processing"    // 处理中
)

// TokenParser 处理SSE事件的解析以提取token使用信息
// 实现TokenParserInterface接口
type TokenParser struct {
	// 用于收集多行JSON数据的缓冲区
	eventBuffer    strings.Builder
	currentEvent   string
	collectingData bool
	// 用于日志记录的请求ID
	requestID string
	// 从message_start事件中提取的模型名称
	modelName string
	// 用于记录token使用和成本的跟踪器
	usageTracker *tracking.UsageTracker
	// 用于计算持续时间的开始时间
	startTime time.Time
	// 用于累积的最终token使用量
	finalUsage *tracking.TokenUsage
	// 用于处理中断的部分使用量
	partialUsage *tracking.TokenUsage

	// 🆕 [流完整性追踪] 2025-12-11
	hasMessageStart      bool // 是否收到 message_start 事件
	hasMessageDeltaUsage bool // 是否收到带 usage 的 message_delta 事件
	hasMessageStop       bool // 是否收到 message_stop 事件
	// 是否已经上报过“无Token响应”（避免 message_delta 高频重复日志）
	nonTokenResponseReported bool
}

// fixMalformedEventType 修复格式错误的事件类型
// 处理如 "content_event: message_delta" 这样的格式错误，提取最后一个有效的事件名称
func (tp *TokenParser) fixMalformedEventType(eventType string) string {
	// 🔧 [格式错误修复] 处理格式错误的事件行，如 "event: content_event: message_delta"
	// 从事件类型中提取最后一个有效的事件名称
	if strings.Contains(eventType, ":") {
		parts := strings.Split(eventType, ":")
		// 取最后一个非空部分作为真正的事件类型
		for i := len(parts) - 1; i >= 0; i-- {
			cleanPart := strings.TrimSpace(parts[i])
			if cleanPart != "" {
				if tp.requestID != "" {
					slog.Warn(fmt.Sprintf("⚠️ [格式错误修复] [%s] 检测到格式错误的事件行，修正为: %s", tp.requestID, cleanPart))
				}
				return cleanPart
			}
		}
	}
	return eventType
}

// NewTokenParser 创建新的token解析器实例
func NewTokenParser() *TokenParser {
	return &TokenParser{
		startTime: time.Now(),
	}
}

// NewTokenParserWithRequestID 创建带请求ID的新token解析器实例
func NewTokenParserWithRequestID(requestID string) *TokenParser {
	return &TokenParser{
		requestID: requestID,
		startTime: time.Now(),
	}
}

// NewTokenParserWithUsageTracker 创建带使用跟踪器的新token解析器实例
func NewTokenParserWithUsageTracker(requestID string, usageTracker *tracking.UsageTracker) *TokenParser {
	return &TokenParser{
		requestID:    requestID,
		usageTracker: usageTracker,
		startTime:    time.Now(),
	}
}

// ParseSSELineV2 新版本的SSE解析方法
// 返回 ParseResult 而不是直接调用 usageTracker
func (tp *TokenParser) ParseSSELineV2(line string) *ParseResult {
	line = strings.TrimSpace(line)

	// 处理事件类型行 - 支持 "event: " 和 "event:" 两种格式
	if strings.HasPrefix(line, "event:") {
		var eventType string
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else {
			eventType = strings.TrimPrefix(line, "event:")
		}

		// 使用公共方法修复格式错误的事件类型
		eventType = tp.fixMalformedEventType(eventType)

		tp.currentEvent = eventType

		// 🆕 [流完整性追踪] 检测 message_stop 事件
		if eventType == "message_stop" {
			tp.hasMessageStop = true
		}

		// 为message_start（模型信息）、message_delta/content_block_delta（使用量）和error事件收集数据
		tp.collectingData = eventType == "message_delta" || eventType == "content_block_delta" || eventType == "message_start" || eventType == "error"
		tp.eventBuffer.Reset()
		return nil
	}

	// 处理数据行 - 支持 "data: " 和 "data:" 两种格式
	if strings.HasPrefix(line, "data:") && tp.collectingData {
		var dataContent string
		if strings.HasPrefix(line, "data: ") {
			dataContent = strings.TrimPrefix(line, "data: ")
		} else {
			dataContent = strings.TrimPrefix(line, "data:")
		}
		tp.eventBuffer.WriteString(dataContent)
		return nil
	}

	// 处理表示SSE事件结束的空行
	if line == "" && tp.collectingData && tp.eventBuffer.Len() > 0 {
		switch tp.currentEvent {
		case "message_start":
			// 仅解析message_start以获取模型信息（不需要ParseResult）
			tp.parseMessageStart()
			return nil
		case "message_delta", "content_block_delta":
			// 使用新的V2方法解析message_delta/content_block_delta
			return tp.parseMessageDeltaV2()
		case "error":
			// 使用新的V2方法解析error事件
			return tp.parseErrorEventV2()
		}
	}

	return nil
}

// ParseSSELine 处理SSE流中的单行数据，如果找到则提取token使用信息
func (tp *TokenParser) ParseSSELine(line string) *monitor.TokenUsage {
	line = strings.TrimSpace(line)

	// 处理事件类型行 - 支持 "event: " 和 "event:" 两种格式
	if strings.HasPrefix(line, "event:") {
		var eventType string
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else {
			eventType = strings.TrimPrefix(line, "event:")
		}

		// 使用公共方法修复格式错误的事件类型
		eventType = tp.fixMalformedEventType(eventType)

		tp.currentEvent = eventType
		// 为message_start（模型信息）、message_delta/content_block_delta（使用量）和error事件收集数据
		tp.collectingData = eventType == "message_delta" || eventType == "content_block_delta" || eventType == "message_start" || eventType == "error"
		tp.eventBuffer.Reset()
		return nil
	}

	// 处理数据行 - 支持 "data: " 和 "data:" 两种格式
	if strings.HasPrefix(line, "data:") && tp.collectingData {
		var dataContent string
		if strings.HasPrefix(line, "data: ") {
			dataContent = strings.TrimPrefix(line, "data: ")
		} else {
			dataContent = strings.TrimPrefix(line, "data:")
		}
		tp.eventBuffer.WriteString(dataContent)
		return nil
	}

	// 处理表示SSE事件结束的空行
	if line == "" && tp.collectingData && tp.eventBuffer.Len() > 0 {
		switch tp.currentEvent {
		case "message_start":
			// 解析message_start以获取模型信息和token使用量
			return tp.parseMessageStart()
		case "message_delta", "content_block_delta":
			// 解析message_delta/content_block_delta以获取使用信息
			return tp.parseMessageDelta()
		case "error":
			// 解析error事件并记录为API错误
			// 🚫 修复：注释掉违规的直接usageTracker调用，让生命周期管理器处理
			// tp.parseErrorEvent()
			slog.Info(fmt.Sprintf("❌ [错误事件] [%s] 检测到API错误事件", tp.requestID))
			return nil // error事件不返回TokenUsage
		}
	}

	return nil
}

// parseMessageStart 解析收集的message_start JSON数据以仅提取模型信息
func (tp *TokenParser) parseMessageStart() *monitor.TokenUsage {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	// 🆕 [流完整性追踪] 标记收到 message_start 事件
	tp.hasMessageStart = true

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return nil
	}

	// 解析JSON数据
	var messageStart MessageStart
	if err := json.Unmarshal([]byte(jsonData), &messageStart); err != nil {
		return nil
	}

	// 如果可用，提取模型名称
	if messageStart.Message != nil && messageStart.Message.Model != "" {
		tp.modelName = messageStart.Message.Model

		// 记录模型提取 - 始终包含requestID
		slog.Info(fmt.Sprintf("🎯 [模型提取] [%s] 从message_start事件中提取模型信息: %s",
			tp.requestID, tp.modelName))
	}

	// 🆕 [流式Token修复] 从message_start事件中提取usage信息作为初始值
	// 这样确保即使流被中断，也能保存有效的token使用信息
	if messageStart.Message != nil && messageStart.Message.Usage != nil {
		usage := messageStart.Message.Usage

		// v5.0+: 提取分开的 5m/1h 缓存 tokens
		var cache5m, cache1h int64
		if usage.CacheCreation != nil {
			cache5m = usage.CacheCreation.Ephemeral5mInputTokens
			cache1h = usage.CacheCreation.Ephemeral1hInputTokens
		}

		// 总缓存创建 tokens = 5m + 1h（向后兼容）
		cacheCreationTotal := usage.CacheCreationInputTokens
		if cacheCreationTotal == 0 && (cache5m > 0 || cache1h > 0) {
			cacheCreationTotal = cache5m + cache1h
		}

		tp.partialUsage = &tracking.TokenUsage{
			InputTokens:           usage.InputTokens,
			OutputTokens:          usage.OutputTokens,
			CacheCreationTokens:   cacheCreationTotal,
			CacheReadTokens:       usage.CacheReadInputTokens,
			CacheCreation5mTokens: cache5m,
			CacheCreation1hTokens: cache1h,
		}

		slog.Info(fmt.Sprintf("🎯 [Usage初始化] [%s] 从message_start提取token信息: input=%d, output=%d, cache_create=%d (5m=%d, 1h=%d), cache_read=%d",
			tp.requestID, tp.partialUsage.InputTokens, tp.partialUsage.OutputTokens,
			tp.partialUsage.CacheCreationTokens, cache5m, cache1h, tp.partialUsage.CacheReadTokens))
	}

	return nil
}

// parseMessageDeltaV2 新版本的message_delta解析方法
// 返回 ParseResult 而不是直接调用 usageTracker
func (tp *TokenParser) parseMessageDeltaV2() *ParseResult {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return nil
	}

	// 解析JSON数据
	var messageDelta MessageDelta
	if err := json.Unmarshal([]byte(jsonData), &messageDelta); err != nil {
		return nil
	}

	// 检查此message_delta是否包含使用信息
	if messageDelta.Usage == nil {
		// message_start 已有 usage 时，message_delta 无 usage 属于正常中间态，不记录“无Token响应”
		if tp.partialUsage != nil {
			return nil
		}

		// ⚠️ 兼容性处理：对于非Claude端点，message_delta可能不包含usage信息
		// 这种情况下需要fallback机制来标记请求完成
		if tp.requestID != "" {
			// 如果未从message_start提取模型，则使用"default"作为模型名称
			modelName := tp.modelName
			if modelName == "" {
				modelName = "default"
			}

			// 仅首次上报，避免日志污染
			if tp.nonTokenResponseReported {
				return nil
			}
			tp.nonTokenResponseReported = true

			slog.Info(fmt.Sprintf("🎯 [无Token响应] [%s] message_delta事件不包含token信息，标记为完成 - 模型: %s",
				tp.requestID, modelName))

			// 返回空Token的完成结果
			return &ParseResult{
				TokenUsage: &tracking.TokenUsage{
					InputTokens:         0,
					OutputTokens:        0,
					CacheCreationTokens: 0,
					CacheReadTokens:     0,
				},
				ModelName:   modelName,
				IsCompleted: true,
				Status:      "non_token_response",
			}
		}
		return nil
	}

	// 🆕 [流完整性追踪] 标记收到带 usage 的 message_delta 事件
	tp.hasMessageDeltaUsage = true

	// 🚀 [智能合并] 实现message_start和message_delta的token信息智能合并
	// 策略：
	// 1. input/output tokens: 优先使用message_delta的值（更准确的最终统计）
	// 2. cache tokens: 如果message_delta为0，保留message_start的值（防止缓存信息丢失）

	usage := messageDelta.Usage

	// 获取message_delta中的基础值
	inputTokens := usage.InputTokens
	outputTokens := usage.OutputTokens
	cacheCreationTokens := usage.CacheCreationInputTokens
	cacheReadTokens := usage.CacheReadInputTokens

	// v5.0+: 提取分开的 5m/1h 缓存 tokens
	var cache5m, cache1h int64
	if usage.CacheCreation != nil {
		cache5m = usage.CacheCreation.Ephemeral5mInputTokens
		cache1h = usage.CacheCreation.Ephemeral1hInputTokens
	}

	// 智能合并缓存token：如果message_delta中为0，但message_start中有值，则保留message_start的值
	if cacheCreationTokens == 0 && tp.partialUsage != nil && tp.partialUsage.CacheCreationTokens > 0 {
		cacheCreationTokens = tp.partialUsage.CacheCreationTokens
		slog.Info(fmt.Sprintf("🔄 [缓存合并] [%s] 保留message_start中的cache_creation_tokens: %d",
			tp.requestID, cacheCreationTokens))
	}

	// v5.0+: 智能合并 5m/1h 缓存
	if cache5m == 0 && tp.partialUsage != nil && tp.partialUsage.CacheCreation5mTokens > 0 {
		cache5m = tp.partialUsage.CacheCreation5mTokens
		slog.Info(fmt.Sprintf("🔄 [缓存合并] [%s] 保留message_start中的cache_5m_tokens: %d",
			tp.requestID, cache5m))
	}
	if cache1h == 0 && tp.partialUsage != nil && tp.partialUsage.CacheCreation1hTokens > 0 {
		cache1h = tp.partialUsage.CacheCreation1hTokens
		slog.Info(fmt.Sprintf("🔄 [缓存合并] [%s] 保留message_start中的cache_1h_tokens: %d",
			tp.requestID, cache1h))
	}

	// 如果有分开的 5m/1h 但没有总量，计算总量
	if cacheCreationTokens == 0 && (cache5m > 0 || cache1h > 0) {
		cacheCreationTokens = cache5m + cache1h
	}

	if cacheReadTokens == 0 && tp.partialUsage != nil && tp.partialUsage.CacheReadTokens > 0 {
		cacheReadTokens = tp.partialUsage.CacheReadTokens
		slog.Info(fmt.Sprintf("🔄 [缓存合并] [%s] 保留message_start中的cache_read_tokens: %d",
			tp.requestID, cacheReadTokens))
	}

	// 输入token合并：如果message_delta中为0，但message_start中有值，则保留message_start的值
	if inputTokens == 0 && tp.partialUsage != nil && tp.partialUsage.InputTokens > 0 {
		inputTokens = tp.partialUsage.InputTokens
		slog.Info(fmt.Sprintf("🔄 [输入合并] [%s] 保留message_start中的input_tokens: %d",
			tp.requestID, inputTokens))
	}

	// ✅ 设置finalUsage供GetFinalUsage()方法使用
	tp.finalUsage = &tracking.TokenUsage{
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		CacheCreationTokens:   cacheCreationTokens,
		CacheReadTokens:       cacheReadTokens,
		CacheCreation5mTokens: cache5m,
		CacheCreation1hTokens: cache1h,
	}

	// 如果有分开的缓存信息，记录日志
	if cache5m > 0 || cache1h > 0 {
		slog.Info(fmt.Sprintf("🎯 [缓存分离] [%s] 5m缓存=%d, 1h缓存=%d, 总计=%d",
			tp.requestID, cache5m, cache1h, cacheCreationTokens))
	}

	// 返回解析结果而不是直接记录
	return &ParseResult{
		TokenUsage:  tp.finalUsage,
		ModelName:   tp.modelName,
		IsCompleted: true,
		Status:      "completed",
	}
}

// parseMessageDelta 解析收集的message_delta JSON数据以获取完整的token使用信息
func (tp *TokenParser) parseMessageDelta() *monitor.TokenUsage {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return nil
	}

	// 解析JSON数据
	var messageDelta MessageDelta
	if err := json.Unmarshal([]byte(jsonData), &messageDelta); err != nil {
		return nil
	}

	// 检查此message_delta是否包含使用信息
	if messageDelta.Usage == nil {
		// message_start 已有 usage 时，message_delta 无 usage 属于正常中间态，不记录“无Token响应”
		if tp.partialUsage != nil {
			return nil
		}

		// ⚠️ 兼容性处理：对于非Claude端点，message_delta可能不包含usage信息
		// 这种情况下需要fallback机制来标记请求完成
		if tp.requestID != "" {
			// 如果未从message_start提取模型，则使用"default"作为模型名称
			modelName := tp.modelName
			if modelName == "" {
				modelName = "default"
			}

			// 仅首次记录，避免 message_delta 高频重复日志
			if tp.nonTokenResponseReported {
				return nil
			}
			tp.nonTokenResponseReported = true

			slog.Info(fmt.Sprintf("🎯 [无Token响应] [%s] message_delta事件不包含token信息，标记为完成 - 模型: %s",
				tp.requestID, modelName))
		}
		return nil
	}

	// 转换为我们的TokenUsage格式
	usage := messageDelta.Usage

	// v5.0+: 提取分开的 5m/1h 缓存 tokens
	var cache5m, cache1h int64
	if usage.CacheCreation != nil {
		cache5m = usage.CacheCreation.Ephemeral5mInputTokens
		cache1h = usage.CacheCreation.Ephemeral1hInputTokens
	}

	// 计算总缓存创建 tokens
	cacheCreationTotal := usage.CacheCreationInputTokens
	if cacheCreationTotal == 0 && (cache5m > 0 || cache1h > 0) {
		cacheCreationTotal = cache5m + cache1h
	}

	tokenUsage := &monitor.TokenUsage{
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		CacheCreationTokens: cacheCreationTotal,
		CacheReadTokens:     usage.CacheReadInputTokens,
	}

	// ✅ 设置finalUsage供GetFinalUsage()方法使用
	tp.finalUsage = &tracking.TokenUsage{
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		CacheCreationTokens:   cacheCreationTotal,
		CacheReadTokens:       usage.CacheReadInputTokens,
		CacheCreation5mTokens: cache5m,
		CacheCreation1hTokens: cache1h,
	}

	// 移除重复的日志记录 - 由StreamProcessor统一处理
	// if tp.requestID != "" {
	//	slog.Info(fmt.Sprintf("🪙 [Token使用统计] [%s] 从message_delta事件中提取完整令牌使用情况 -%s 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d",
	//		tp.requestID, modelInfo, tokenUsage.InputTokens, tokenUsage.OutputTokens, tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
	// } else {
	//	slog.Info(fmt.Sprintf("🪙 [Token使用统计] 从message_delta事件中提取完整令牌使用情况 -%s 输入: %d, 输出: %d, 缓存创建: %d, 缓存读取: %d",
	//		modelInfo, tokenUsage.InputTokens, tokenUsage.OutputTokens, tokenUsage.CacheCreationTokens, tokenUsage.CacheReadTokens))
	// }

	// ✅ TokenParser只负责解析，不直接调用usage tracker
	// usage tracker的调用由上层（StreamProcessor或Handler）统一管理
	// if tp.usageTracker != nil && tp.requestID != "" {
	//	// 计算创建解析器以来的持续时间
	//	duration := time.Since(tp.startTime)
	//
	//	// 转换monitor.TokenUsage为tracking.TokenUsage
	//	trackingTokens := &tracking.TokenUsage{
	//		InputTokens:         tokenUsage.InputTokens,
	//		OutputTokens:        tokenUsage.OutputTokens,
	//		CacheCreationTokens: tokenUsage.CacheCreationTokens,
	//		CacheReadTokens:     tokenUsage.CacheReadTokens,
	//	}
	//
	//	// 记录完成的token使用和成本信息
	//	tp.usageTracker.RecordRequestComplete(tp.requestID, tp.modelName, trackingTokens, duration)
	// }

	return tokenUsage
}

// Reset 清除解析器状态
func (tp *TokenParser) Reset() {
	tp.eventBuffer.Reset()
	tp.currentEvent = ""
	tp.collectingData = false
	tp.finalUsage = nil
	tp.partialUsage = nil
	tp.startTime = time.Now()
	// 🆕 [流完整性追踪] 重置完整性追踪字段
	tp.hasMessageStart = false
	tp.hasMessageDeltaUsage = false
	tp.hasMessageStop = false
	tp.nonTokenResponseReported = false
}

// parseErrorEventV2 新版本的错误事件解析方法
// 返回 ParseResult 而不是直接调用 usageTracker
func (tp *TokenParser) parseErrorEventV2() *ParseResult {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return nil
	}

	// 解析错误JSON数据
	var errorData SSEErrorData
	if err := json.Unmarshal([]byte(jsonData), &errorData); err != nil {
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("⚠️ [SSE错误解析] [%s] 无法解析错误数据: %s", tp.requestID, jsonData))
		}
		return nil
	}

	// 提取错误类型和消息
	errorType := errorData.Error.Type
	errorMessage := errorData.Error.Message
	if errorType == "" {
		errorType = "unknown_error"
	}
	if errorMessage == "" {
		errorMessage = "Unknown error"
	}

	// 记录API错误
	if tp.requestID != "" {
		slog.Info(fmt.Sprintf("❌ [API错误] [%s] 错误类型: %s, 错误信息: %s",
			tp.requestID, errorType, errorMessage))
	} else {
		slog.Info(fmt.Sprintf("❌ [API错误] 错误类型: %s, 错误信息: %s",
			errorType, errorMessage))
	}

	// 返回解析结果而不是直接记录到 usageTracker
	errorModelName := fmt.Sprintf("error:%s", errorType)

	return &ParseResult{
		TokenUsage: &tracking.TokenUsage{
			InputTokens:         0,
			OutputTokens:        0,
			CacheCreationTokens: 0,
			CacheReadTokens:     0,
		},
		ModelName:   errorModelName,
		ErrorInfo:   &ErrorInfo{Type: errorType, Message: errorMessage},
		IsCompleted: true,
		Status:      StatusErrorAPI,
	}
}

// parseErrorEvent 解析SSE错误事件并将其记录为API错误
func (tp *TokenParser) parseErrorEvent() {
	defer func() {
		tp.eventBuffer.Reset()
		tp.collectingData = false
		tp.currentEvent = ""
	}()

	jsonData := tp.eventBuffer.String()
	if jsonData == "" {
		return
	}

	// 解析错误JSON数据
	var errorData SSEErrorData
	if err := json.Unmarshal([]byte(jsonData), &errorData); err != nil {
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("⚠️ [SSE错误解析] [%s] 无法解析错误数据: %s", tp.requestID, jsonData))
		}
		return
	}

	// 提取错误类型和消息
	errorType := errorData.Error.Type
	errorMessage := errorData.Error.Message
	if errorType == "" {
		errorType = "unknown_error"
	}
	if errorMessage == "" {
		errorMessage = "Unknown error"
	}

	// 记录API错误
	if tp.requestID != "" {
		slog.Info(fmt.Sprintf("❌ [API错误] [%s] 错误类型: %s, 错误信息: %s",
			tp.requestID, errorType, errorMessage))
	} else {
		slog.Info(fmt.Sprintf("❌ [API错误] 错误类型: %s, 错误信息: %s",
			errorType, errorMessage))
	}

	// ℹ️ 返回错误信息，由Handler通过生命周期管理器记录
	// 不再直接调用usageTracker，遵循架构原则
	// TokenParser只负责解析和返回结果，不直接记录到数据库
	if tp.requestID != "" {
		slog.Info(fmt.Sprintf("🏁 [API错误解析] [%s] 错误信息已解析: %s - %s", tp.requestID, errorType, errorMessage))
	}

	// 更新内部状态，由上层通过生命周期管理器记录完成状态
	tp.finalUsage = &tracking.TokenUsage{
		InputTokens:         0,
		OutputTokens:        0,
		CacheCreationTokens: 0,
		CacheReadTokens:     0,
	}
	tp.modelName = fmt.Sprintf("error:%s", errorType)
	// TokenParser不需要维护status字段，由上层处理
}

// SetModelName 允许直接设置模型名称（用于JSON响应解析）
func (tp *TokenParser) SetModelName(modelName string) {
	tp.modelName = modelName
}

// ParseMessageStart 实现接口方法 - 解析message_start事件提取模型信息
func (tp *TokenParser) ParseMessageStart(line string) *ModelInfo {
	if !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := line[6:] // 移除 "data: "
	if strings.TrimSpace(data) == "[DONE]" {
		return nil
	}

	var event map[string]interface{}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return nil
	}

	if event["type"] == "message_start" {
		if message, ok := event["message"].(map[string]interface{}); ok {
			if model, ok := message["model"].(string); ok {
				tp.SetModel(model)
				return &ModelInfo{Model: model}
			}
		}
	}

	return nil
}

// ParseMessageDelta 实现接口方法 - 解析message_delta事件提取Token使用统计
func (tp *TokenParser) ParseMessageDelta(line string) *tracking.TokenUsage {
	if !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := line[6:] // 移除 "data: "
	if strings.TrimSpace(data) == "[DONE]" {
		return nil
	}

	var event map[string]interface{}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return nil
	}

	if eventType, ok := event["type"].(string); ok && (eventType == "message_delta" || eventType == "content_block_delta") {
		if usage, ok := event["usage"].(map[string]interface{}); ok {
			tokenUsage := &tracking.TokenUsage{}

			if inputTokens, ok := usage["input_tokens"].(float64); ok {
				tokenUsage.InputTokens = int64(inputTokens)
			}
			if outputTokens, ok := usage["output_tokens"].(float64); ok {
				tokenUsage.OutputTokens = int64(outputTokens)
			}
			if cacheCreation, ok := usage["cache_creation_input_tokens"].(float64); ok {
				tokenUsage.CacheCreationTokens = int64(cacheCreation)
			}
			if cacheRead, ok := usage["cache_read_input_tokens"].(float64); ok {
				tokenUsage.CacheReadTokens = int64(cacheRead)
			}

			// 保存最终使用统计
			tp.finalUsage = tokenUsage

			return tokenUsage
		}
	}

	return nil
}

// SetModel 实现接口方法 - 设置模型名称
func (tp *TokenParser) SetModel(modelName string) {
	tp.modelName = modelName
}

// GetFinalUsage 实现接口方法 - 获取最终Token使用统计
// 🆕 [流式Token修复] 支持fallback机制：finalUsage > partialUsage > nil
func (tp *TokenParser) GetFinalUsage() *tracking.TokenUsage {
	// 优先返回finalUsage（来自message_delta）
	if tp.finalUsage != nil {
		return tp.finalUsage
	}

	// Fallback到partialUsage（来自message_start）
	if tp.partialUsage != nil {
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("🚨 [中断恢复] [%s] 使用message_start中的usage信息作为最终结果", tp.requestID))
		}
		return tp.partialUsage
	}

	return nil
}

// GetModelName 获取模型名称
func (tp *TokenParser) GetModelName() string {
	return tp.modelName
}

// IsFallbackUsed 检查是否使用了fallback机制
// 返回true表示使用了message_start的数据而不是完整的message_delta数据
func (tp *TokenParser) IsFallbackUsed() bool {
	return tp.finalUsage == nil && tp.partialUsage != nil
}

// StreamCompleteness 流完整性状态
// 🆕 [流完整性追踪] 2025-12-11
type StreamCompleteness struct {
	IsComplete    bool   // 是否完整
	Reason        string // 不完整的原因（用于日志）
	FailureReason string // 数据库 failure_reason 值
}

// StreamIncompleteError 流不完整错误（结构化错误类型）
// 🆕 [流完整性追踪] 2025-12-11
// 用于替代字符串解析，提供类型安全的错误传递
type StreamIncompleteError struct {
	FailureReason string // 数据库 failure_reason 值（incomplete_stream 或 stream_truncated）
	ModelName     string // 模型名称
	Reason        string // 不完整的原因（用于日志）
}

// Error 实现 error 接口
func (e *StreamIncompleteError) Error() string {
	return fmt.Sprintf("stream incomplete: %s (model: %s)", e.Reason, e.ModelName)
}

// GetFailureReason 获取 failure_reason（实现 StreamIncompleteErrorInterface）
func (e *StreamIncompleteError) GetFailureReason() string {
	return e.FailureReason
}

// GetModelName 获取模型名称（实现 StreamIncompleteErrorInterface）
func (e *StreamIncompleteError) GetModelName() string {
	return e.ModelName
}

// GetReason 获取不完整的原因（实现 StreamIncompleteErrorInterface）
func (e *StreamIncompleteError) GetReason() string {
	return e.Reason
}

// IsStreamIncompleteError 检查错误是否为 StreamIncompleteError
func IsStreamIncompleteError(err error) (*StreamIncompleteError, bool) {
	var streamErr *StreamIncompleteError
	if errors.As(err, &streamErr) {
		return streamErr, true
	}
	return nil, false
}

// GetStreamCompleteness 获取流完整性状态
// 🆕 [流完整性追踪] 2025-12-11
// 根据接收到的 SSE 事件判断流是否完整：
// - 完整流：收到 message_start + message_delta(usage) + message_stop
// - 不完整流：缺少 message_start、message_stop 或 message_delta(usage)
func (tp *TokenParser) GetStreamCompleteness() StreamCompleteness {
	// 🔧 [边界条件修复] 2025-12-11
	// 首先检查是否收到 message_start，这是所有有效响应的起点
	if !tp.hasMessageStart {
		return StreamCompleteness{
			IsComplete:    false,
			Reason:        "未收到 message_start 事件",
			FailureReason: "stream_truncated",
		}
	}

	// 检查是否收到必要的结束事件
	if !tp.hasMessageStop {
		// 没有 message_stop，判断是缺少结束事件还是内容截断
		if tp.hasMessageDeltaUsage {
			// 有 usage 但没有 stop，可能是轻微不完整
			return StreamCompleteness{
				IsComplete:    false,
				Reason:        "缺少 message_stop 事件",
				FailureReason: "incomplete_stream",
			}
		}
		// 只有 start，响应被截断
		return StreamCompleteness{
			IsComplete:    false,
			Reason:        "响应被截断，缺少 message_delta 和 message_stop",
			FailureReason: "stream_truncated",
		}
	}

	// 检查是否有完整的 usage 信息
	if !tp.hasMessageDeltaUsage && tp.IsFallbackUsed() {
		return StreamCompleteness{
			IsComplete:    false,
			Reason:        "使用了 message_start fallback，缺少最终 usage",
			FailureReason: "incomplete_stream",
		}
	}

	// 流完整
	return StreamCompleteness{
		IsComplete:    true,
		Reason:        "",
		FailureReason: "",
	}
}

// IsStreamComplete 简单判断流是否完整
// 🆕 [流完整性追踪] 2025-12-11
func (tp *TokenParser) IsStreamComplete() bool {
	return tp.GetStreamCompleteness().IsComplete
}

// GetPartialUsage 获取部分Token使用统计（用于网络中断恢复）
func (tp *TokenParser) GetPartialUsage() *tracking.TokenUsage {
	if tp.partialUsage != nil {
		return tp.partialUsage
	}
	return tp.finalUsage
}

// FlushPendingEvent 强制解析缓存中的待处理事件
// 在流结束或连接中断时调用，确保不会因为缺少终止空行而丢失 usage 信息
func (tp *TokenParser) FlushPendingEvent() *ParseResult {
	// 只有在收集数据且缓存非空时才需要flush
	if !tp.collectingData || tp.eventBuffer.Len() == 0 {
		return nil
	}

	// 根据当前事件类型调用相应的解析方法
	switch tp.currentEvent {
	case "message_delta", "content_block_delta":
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("🔄 [事件Flush] [%s] 强制解析缓存的delta事件: %s", tp.requestID, tp.currentEvent))
		}
		return tp.parseMessageDeltaV2()
	case "message_start":
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("🔄 [事件Flush] [%s] 强制解析缓存的message_start事件", tp.requestID))
		}
		tp.parseMessageStart()
		return nil
	case "error":
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("🔄 [事件Flush] [%s] 强制解析缓存的error事件", tp.requestID))
		}
		return tp.parseErrorEventV2()
	default:
		if tp.requestID != "" {
			slog.Info(fmt.Sprintf("⚠️ [事件Flush] [%s] 未知事件类型: %s, 跳过解析", tp.requestID, tp.currentEvent))
		}
		return nil
	}
}
