package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/tracking"

	"github.com/google/uuid"
)

// StreamingHandler 流式请求处理器
// 负责处理所有流式请求，包括错误恢复、重试机制和流式数据转发
type StreamingHandler struct {
	config                   *config.Config
	endpointManager          *endpoint.Manager
	forwarder                *Forwarder
	usageTracker             *tracking.UsageTracker
	tokenParserFactory       TokenParserFactory
	streamProcessorFactory   StreamProcessorFactory
	errorRecoveryFactory     ErrorRecoveryFactory
	retryManagerFactory      RetryManagerFactory
	suspensionManagerFactory SuspensionManagerFactory
	// 🔧 [修复] 共享SuspensionManager实例，确保全局挂起限制生效
	sharedSuspensionManager SuspensionManager
}

const defaultClaudeStreamingTailDrainTimeout = time.Second
const maxStreamingExecutionRestarts = 16

// NewStreamingHandler 创建新的StreamingHandler实例
func NewStreamingHandler(
	cfg *config.Config,
	endpointManager *endpoint.Manager,
	forwarder *Forwarder,
	usageTracker *tracking.UsageTracker,
	tokenParserFactory TokenParserFactory,
	streamProcessorFactory StreamProcessorFactory,
	errorRecoveryFactory ErrorRecoveryFactory,
	retryManagerFactory RetryManagerFactory,
	suspensionManagerFactory SuspensionManagerFactory,
	// 🔧 [Critical修复] 直接接受共享的SuspensionManager实例
	sharedSuspensionManager SuspensionManager,
) *StreamingHandler {
	return &StreamingHandler{
		config:                   cfg,
		endpointManager:          endpointManager,
		forwarder:                forwarder,
		usageTracker:             usageTracker,
		tokenParserFactory:       tokenParserFactory,
		streamProcessorFactory:   streamProcessorFactory,
		errorRecoveryFactory:     errorRecoveryFactory,
		retryManagerFactory:      retryManagerFactory,
		suspensionManagerFactory: suspensionManagerFactory,
		// 🔧 [Critical修复] 使用传入的共享SuspensionManager实例
		// 确保流式请求与常规请求共享同一个全局挂起计数器
		sharedSuspensionManager: sharedSuspensionManager,
	}
}

// noOpFlusher 是一个不执行实际flush操作的flusher实现
type noOpFlusher struct{}

func (f *noOpFlusher) Flush() {
	// 不执行任何操作，避免panic但保持流式处理逻辑
}

// sendAnthropicRetryableError 发送 Anthropic API 标准格式的可重试错误事件
// 使用 overloaded_error 类型，Claude Code 等客户端会识别并自动重试
func sendAnthropicRetryableError(w http.ResponseWriter, flusher http.Flusher, message string) {
	// 发送 SSE 格式的 error 事件
	// 格式: event: error\ndata: {"type":"error","error":{"type":"xxx","message":"xxx"}}\n\n
	fmt.Fprintf(w, "event: error\n")
	fmt.Fprintf(w, "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"%s\"}}\n\n",
		escapeJSONString(message))
	flusher.Flush()
}

// sendStreamInterruptedMessage 发送流中断消息，触发客户端自动重试
// 模拟上游代理的行为：当 EOF 发生时，发送一个完整的新消息（第二个 message_start）
// Claude Code 会识别这种模式并自动重试请求
// 参考: req-fd3a8dd9.debug 中观察到的上游代理行为
func sendStreamInterruptedMessage(w http.ResponseWriter, flusher http.Flusher, message string, modelName string) {
	// 使用 UUID 作为 message id（模拟上游代理的行为，与正常的 msg_ 前缀不同）
	msgID := uuid.New().String()

	// 1. message_start - 新消息开始（使用 Anthropic 格式的 usage 字段）
	fmt.Fprintf(w, "event: message_start\n")
	fmt.Fprintf(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"%s\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"%s\",\"content\":[],\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n",
		msgID, escapeJSONString(modelName))
	flusher.Flush()

	// 2. content_block_start - 开始 text 类型的内容块
	fmt.Fprintf(w, "event: content_block_start\n")
	fmt.Fprintf(w, "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
	flusher.Flush()

	// 3. content_block_delta - 发送错误信息内容
	fmt.Fprintf(w, "event: content_block_delta\n")
	fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"%s\"}}\n\n",
		escapeJSONString(message))
	flusher.Flush()

	// 4. content_block_stop - 内容块结束
	fmt.Fprintf(w, "event: content_block_stop\n")
	fmt.Fprintf(w, "data: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
	flusher.Flush()

	// 5. message_delta - 消息增量更新（包含 stop_reason 和 usage，让客户端认为流完整）
	fmt.Fprintf(w, "event: message_delta\n")
	fmt.Fprintf(w, "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":0}}\n\n")
	flusher.Flush()

	// 6. message_stop - 消息结束（关键！让客户端识别为完整消息从而触发重试）
	fmt.Fprintf(w, "event: message_stop\n")
	fmt.Fprintf(w, "data: {\"type\":\"message_stop\"}\n\n")
	flusher.Flush()
}

// isStreamingEOFError 检查错误是否为流式传输过程中的 EOF 错误
// 注意：此函数已弃用，主逻辑已改为对所有流式错误统一触发重试（2025-12-25）
// 保留此函数仅供测试使用
func isStreamingEOFError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	// 检查是否包含 stream_status 前缀（表示已进入流式处理阶段）且包含 eof
	return strings.Contains(errStr, "stream_status:") && strings.Contains(errStr, "eof")
}

// escapeJSONString 对字符串进行 JSON 转义
func escapeJSONString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// HandleStreamingRequest 统一流式请求处理
// 使用V2架构整合错误恢复机制和生命周期管理的流式处理
func (sh *StreamingHandler) HandleStreamingRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()

	slog.Info(fmt.Sprintf("🌊 [流式架构] [%s] 使用streaming v2架构", connID))
	slog.Info(fmt.Sprintf("🌊 [流式处理] [%s] 开始流式请求处理", connID))
	sh.handleStreamingV2(ctx, w, r, bodyBytes, lifecycleManager)
}

// handleStreamingV2 流式处理（带错误恢复）
func (sh *StreamingHandler) handleStreamingV2(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager RequestLifecycleManager) {
	connID := lifecycleManager.GetRequestID()

	// 设置流式响应头
	sh.setStreamingHeaders(w)

	// 获取Flusher - 如果不支持，使用无flush模式继续流式处理
	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Warn(fmt.Sprintf("🌊 [Flusher不支持] [%s] 将使用无flush模式的流式处理", connID))
		// 创建一个mock flusher，不执行实际flush操作
		flusher = &noOpFlusher{}
	}

	// 继续执行流式请求处理
	sh.executeStreamingWithRetry(ctx, w, r, bodyBytes, lifecycleManager, flusher)
}

func (sh *StreamingHandler) shouldEnableTailDrain(r *http.Request) bool {
	return r != nil && r.URL != nil && r.URL.Path == "/v1/messages"
}

func (sh *StreamingHandler) tailDrainTimeout() time.Duration {
	return defaultClaudeStreamingTailDrainTimeout
}

// setStreamingHeaders 设置流式响应头
func (sh *StreamingHandler) setStreamingHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")
}

// executeStreamingWithRetry 执行带重试的流式处理
func (sh *StreamingHandler) executeStreamingWithRetry(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager RequestLifecycleManager, flusher http.Flusher) {
restartLoop:
	for restartCount := 0; restartCount < maxStreamingExecutionRestarts; restartCount++ {
		connID := lifecycleManager.GetRequestID()
		var lastFailedEndpoint string // 🚀 [端点自愈] 追踪最后失败的端点

		// 🎯 [失败追踪] reject 模式：检查是否应该直接拒绝请求
		if shouldReject, rejectedEndpoint := sh.endpointManager.ShouldRejectRequest(); shouldReject {
			slog.Warn(fmt.Sprintf("❌ [失败追踪] [%s] 端点 %s 达到失败阈值，拒绝请求（reject 模式）", connID, rejectedEndpoint))
			lifecycleManager.FailRequest("rejected_by_failure_tracker",
				fmt.Sprintf("Endpoint %s reached failure threshold, request rejected", rejectedEndpoint),
				http.StatusServiceUnavailable)
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "data: error: Service temporarily unavailable - endpoint %s failure threshold reached\n\n", rejectedEndpoint)
			flusher.Flush()
			return
		}

		var endpoints []*endpoint.Endpoint
		if sh.endpointManager.GetConfig().Strategy.Type == "fastest" && sh.endpointManager.GetConfig().Strategy.FastTestEnabled {
			endpoints = sh.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
		} else {
			endpoints = sh.endpointManager.GetHealthyEndpoints()
		}

		if len(endpoints) == 0 {
			// 创建特殊错误，交给错误分类和重试系统处理
			noHealthyErr := fmt.Errorf("no endpoints available")
			errorRecovery := sh.errorRecoveryFactory.NewErrorRecoveryManager(sh.usageTracker)
			errorCtx := errorRecovery.ClassifyError(noHealthyErr, connID, "", "", 0)

			if errorCtx.ErrorType == ErrorTypeNoHealthyEndpoints {
				// 🔍 [健康检查回退] 忽略健康状态（可能误判），但仍然尊重失败追踪和冷却
				allActiveEndpoints := sh.endpointManager.GetGroupManager().FilterEndpointsByActiveGroups(
					sh.endpointManager.GetAllEndpoints())

				// 过滤：跳过失败追踪器标记的端点和冷却中的端点
				var fallbackEndpoints []*endpoint.Endpoint
				for _, ep := range allActiveEndpoints {
					// 📊 [失败追踪] 检查是否达到失败阈值（真实请求失败，必须跳过）
					if sh.endpointManager.GetConfig().FailureTracker.Enabled &&
						sh.endpointManager.ShouldTriggerFailureAction(ep.Config.Name) {
						slog.Debug(fmt.Sprintf("⏭️ [健康检查回退] 跳过失败追踪标记的端点: %s", ep.Config.Name))
						continue
					}

					// 检查冷却状态
					if ep.IsInCooldown() {
						slog.Debug(fmt.Sprintf("⏭️ [健康检查回退] 跳过冷却中的端点: %s", ep.Config.Name))
						continue
					}

					fallbackEndpoints = append(fallbackEndpoints, ep)
				}

				if len(fallbackEndpoints) > 0 {
					slog.InfoContext(ctx, fmt.Sprintf("🔄 [健康检查回退] [%s] 忽略健康状态，尝试 %d 个端点（已过滤失败和冷却）",
						connID, len(fallbackEndpoints)))
					endpoints = fallbackEndpoints
					// 继续正常处理流程
				} else {
					// 🔧 [挂起修复] 2025-12-25: 所有端点不可用时，应该触发挂起而非直接返回 503
					// 保持 endpoints 为空数组，让代码继续执行到后面的挂起逻辑
					slog.InfoContext(ctx, fmt.Sprintf("⏸️ [端点不可用] [%s] 所有端点因失败追踪或冷却被过滤，准备进入挂起逻辑",
						connID))
					lifecycleManager.HandleError(noHealthyErr)
					endpoints = []*endpoint.Endpoint{} // 空数组，跳过端点处理循环
					// 不要 return，继续执行到后面的挂起逻辑
				}
			} else {
				// 🔧 [挂起修复] 2025-12-25: 其他错误导致端点不可用，也应该触发挂起
				slog.InfoContext(ctx, fmt.Sprintf("⏸️ [端点不可用] [%s] 无可用端点，准备进入挂起逻辑",
					connID))
				lifecycleManager.HandleError(noHealthyErr)
				endpoints = []*endpoint.Endpoint{} // 空数组，跳过端点处理循环
				// 不要 return，继续执行到后面的挂起逻辑
			}
		}

		slog.Info(fmt.Sprintf("🌊 [流式开始] [%s] 流式请求开始，端点数: %d", connID, len(endpoints)))

		// 🔧 [重试逻辑修复] 对每个端点进行max_attempts次重试，而不是只尝试一次
		// 尝试端点直到成功
		var lastErr error           // 声明在外层作用域，供最终错误处理使用
		var lastResp *http.Response // 🔧 [修复] 添加lastResp变量，用于获取真实HTTP状态码
		// 🔢 [重构] 移除currentAttemptCount变量，统一由LifecycleManager管理计数
		for i := 0; i < len(endpoints); i++ {
			ep := endpoints[i]
			lastFailedEndpoint = ep.Config.Name // 🚀 [端点自愈] 记录当前尝试的端点
			// 更新生命周期管理器信息
			lifecycleManager.SetEndpoint(ep.Config.Name, ep.Config.Group, ep.Config.Channel)
			lifecycleManager.UpdateStatus("forwarding", i, 0)

			// 🔧 [端点上下文修复] 立即设置端点信息到请求上下文，确保所有分支（成功/失败/取消）的日志都能正确记录端点
			*r = *r.WithContext(context.WithValue(r.Context(), "selected_endpoint", ep.Config.Name))

			// ✅ [同端点重试] 对当前端点进行max_attempts次重试
			endpointSuccess := false
			var attempt int                 // 声明在外部，循环结束后仍可访问
			var lastDecision *RetryDecision // 保存最后的重试决策，用于外层逻辑

			for attempt = 1; attempt <= sh.config.Retry.MaxAttempts; attempt++ {
				// 检查是否被取消
				select {
				case <-ctx.Done():
					slog.Info(fmt.Sprintf("🚫 [客户端取消检测] [%s] 检测到客户端取消，立即停止重试", connID))
					lifecycleManager.CancelRequest("client disconnected", nil)

					// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
					*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
					fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
					flusher.Flush()
					return
				default:
				}

				// 尝试连接端点
				tailDrainEnabled := sh.shouldEnableTailDrain(r)
				var upstreamCancel context.CancelFunc
				var resp *http.Response
				var err error
				if tailDrainEnabled {
					resp, upstreamCancel, err = sh.forwarder.ForwardStreamingRequestToEndpoint(r, bodyBytes, ep)
				} else {
					resp, err = sh.forwarder.ForwardRequestToEndpoint(ctx, r, bodyBytes, ep)
				}
				releaseUpstream := func() {
					if upstreamCancel != nil {
						upstreamCancel()
						upstreamCancel = nil
					}
				}
				// 🔧 [修复] 保存最后的响应，用于获取真实HTTP状态码
				lastResp = resp
				if err == nil && IsSuccessStatus(resp.StatusCode) {
					defer releaseUpstream()
					// 🔢 [成功计数] 成功的尝试记录到生命周期管理器
					lifecycleManager.IncrementAttempt()
					currentAttemptCount := lifecycleManager.GetAttemptCount()

					// ✅ [重试决策] 成功请求的决策日志 - 保持监控完整性
					slog.Info(fmt.Sprintf("✅ [重试决策] 请求成功完成 request_id=%s endpoint=%s attempt=%d reason=请求成功完成",
						connID, ep.Config.Name, currentAttemptCount))

					// 🔧 [失败追踪] 成功记录移至 lifecycle_manager.CompleteRequest()
					// 确保流式响应完全处理后才清空失败计数，避免在流式读写阶段失败时过早清空失败窗口

					// ✅ 成功！开始处理响应
					endpointSuccess = true
					slog.Info(fmt.Sprintf("✅ [流式成功] [%s] 端点: %s , 尝试次数: %d",
						connID, ep.Config.Name, currentAttemptCount))

					lifecycleManager.UpdateStatus("processing", currentAttemptCount, resp.StatusCode)

					// 处理流式响应 - 使用现有的流式处理逻辑
					w.WriteHeader(resp.StatusCode)

					// 创建Token解析器和流式处理器
					tokenParser := sh.tokenParserFactory.NewTokenParserWithUsageTracker(connID, sh.usageTracker)
					processor := sh.streamProcessorFactory.NewStreamProcessor(tokenParser, sh.usageTracker, w, flusher, connID, ep.Config.Name)
					if tailDrainEnabled {
						processor.EnableDownstreamTailDrain(sh.tailDrainTimeout(), upstreamCancel)
					}

					slog.Info(fmt.Sprintf("🚀 [开始流式处理] [%s] 端点: %s", connID, ep.Config.Name))

					// 执行流式处理并获取Token信息和模型名称
					finalTokenUsage, modelName, err := processor.ProcessStreamWithRetry(ctx, resp)
					if err != nil {
						// 🔧 [结构化错误处理] 2025-12-11: 优先使用接口断言处理流不完整错误
						if streamErr, ok := err.(StreamIncompleteErrorInterface); ok {
							// 流不完整但请求已完成，需要标记 failure_reason
							parsedModelName := streamErr.GetModelName()
							failureReason := streamErr.GetFailureReason()

							// 🔧 [模型回退修复] 2025-12-26: 优先使用请求体中的模型，而非流解析的 "default"/"unknown"
							// 当没有收到 message_start 事件时，流解析器返回 "unknown"，这会导致费用计算错误
							// 此时应该使用请求体中提取的模型（已经在 lifecycleManager 中设置）
							if parsedModelName != "unknown" && parsedModelName != "" && parsedModelName != "default" {
								lifecycleManager.SetModelWithComparison(parsedModelName, "stream_incomplete")
							} else if modelName != "unknown" && modelName != "" && modelName != "default" {
								lifecycleManager.SetModelWithComparison(modelName, "stream_processor")
							} else {
								// 流解析模型无效，使用请求体中的模型（已在 lifecycleManager 中）
								if lifecycleManager.HasModel() {
									slog.Info(fmt.Sprintf("🔄 [模型回退] [%s] 流解析模型无效(%s)，使用请求体模型",
										connID, parsedModelName))
								} else {
									slog.Warn(fmt.Sprintf("⚠️ [模型缺失] [%s] 流解析模型: %s, 请求体也无模型",
										connID, parsedModelName))
								}
							}

							// 使用 CompleteRequestWithQuality 完成请求并标记数据质量问题
							lifecycleManager.CompleteRequestWithQuality(finalTokenUsage, failureReason)

							slog.Info(fmt.Sprintf("⚠️ [流不完整但已完成] [%s] 端点: %s, failure_reason: %s",
								connID, ep.Config.Name, failureReason))
							return
						}

						// 处理其他类型的错误（保留原有逻辑用于兼容）
						var status, parsedModelName string = "error", "unknown"

						// ✅ 从错误信息中提取状态和模型信息
						if strings.HasPrefix(err.Error(), "stream_status:") {
							parts := strings.SplitN(err.Error(), ":", 5)
							if len(parts) >= 4 {
								status = parts[1] // 状态：cancelled, timeout, error
								if parts[2] == "model" && len(parts) > 3 && parts[3] != "" {
									parsedModelName = parts[3] // 模型：claude-sonnet-4-20250514
								}
							}
						}

						// ✅ 确保生命周期管理器获得正确的模型信息
						// 优先使用从错误包装器中解析的模型信息
						if parsedModelName != "unknown" && parsedModelName != "" {
							lifecycleManager.SetModelWithComparison(parsedModelName, "stream_status")
						} else if modelName != "unknown" && modelName != "" {
							// ✅ 如果错误包装器中没有模型信息，使用ProcessStreamWithRetry返回的模型信息
							lifecycleManager.SetModelWithComparison(modelName, "stream_processor")
						}

						// 🚀 [状态机重构] Phase 4: 统一使用HandleError处理错误，遵循状态错误分离原则
						// 设置failure_reason，让错误分类器正确识别stream_status错误
						lifecycleManager.HandleError(err)

						// 🚀 [HTTP状态码修复] 流式API错误应该映射为207 Multi-Status
						statusCode := GetStatusCodeFromError(err, resp)
						if status == "error" || status == "stream_error" {
							statusCode = http.StatusMultiStatus // 207: HTTP连接成功，但API业务层面有错误
						} else if status == "cancelled" {
							statusCode = 499 // 客户端取消
						}

						// 🚀 [语义修复] 区分取消和失败的不同处理方式
						if status == "cancelled" {
							// 取消请求：直接传递Token信息给CancelRequest，保持语义一致性
							// 避免先调用RecordTokensForFailedRequest再CancelRequest的语义矛盾
							lifecycleManager.CancelRequest("stream processing cancelled", finalTokenUsage)
						} else {
							// 流式错误：先记录失败Token，再使用FailRequest设置最终状态
							if finalTokenUsage != nil {
								lifecycleManager.RecordTokensForFailedRequest(finalTokenUsage, status)
							} else {
								// 无Token信息，仅记录失败状态
								slog.Info(fmt.Sprintf("❌ [流式失败无Token] [%s] 端点: %s, 状态: %s, 无Token信息可保存",
									connID, ep.Config.Name, status))
							}
							// 使用FailRequest设置最终状态为failed
							// 这样status=failed, failure_reason=stream_error, http_status=207
							lifecycleManager.FailRequest(status, err.Error(), statusCode)
						}

						// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
						*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", statusCode))

						slog.Warn(fmt.Sprintf("🔄 [流式处理失败] [%s] 端点: %s, 状态: %s, 模型: %s, 错误: %v",
							connID, ep.Config.Name, status, parsedModelName, err))

						// 📊 [失败追踪] 记录流式阶段错误到 FailureTracker（取消除外）
						// 流式阶段的 EOF、stream_error 等错误也应该记录，用于下次请求跳过该端点
						if status != "cancelled" {
							failCount := sh.endpointManager.RecordFailure(ep.Config.Name)
							slog.Info(fmt.Sprintf("📊 [失败追踪] [%s] 端点 %s 流式阶段失败，窗口内失败次数: %d",
								connID, ep.Config.Name, failCount))
						}

						// 根据状态决定是否发送错误信息
						if status == "cancelled" {
							fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
							flusher.Flush()
						} else if sh.config.RequestSuspend.EOFRetryHint {
							// 🔄 [流式错误重试提示] 2025-12-25: 流式传输过程中的所有错误都发送中断消息触发客户端自动重试
							// 不再区分 EOF 或其他错误类型，统一处理以提高客户端重试成功率
							// 优先使用从错误信息中解析的模型名称（parsedModelName），因为它是流处理过程中解析的
							// 其次使用 ProcessStreamWithRetry 返回的 modelName
							interruptModelName := parsedModelName
							if interruptModelName == "" || interruptModelName == "unknown" {
								interruptModelName = modelName
							}
							if interruptModelName == "" || interruptModelName == "unknown" {
								interruptModelName = "claude-3-5-sonnet-20241022" // 默认模型
							}
							slog.Info(fmt.Sprintf("🔄 [流式错误重试提示] [%s] 流式传输中断（%s），发送中断消息触发客户端重试, 模型: %s", connID, status, interruptModelName))
							sendStreamInterruptedMessage(w, flusher, "Connection interrupted", interruptModelName)
						} else {
							// 配置未开启重试提示，使用简单错误格式
							fmt.Fprintf(w, "data: error: 流式处理失败: %v\n\n", err)
							flusher.Flush()
						}
						return
					}

					// ✅ 流式处理成功完成，使用生命周期管理器完成请求
					if finalTokenUsage != nil {
						// 设置模型名称并通过生命周期管理器完成请求
						// 使用对比方法，检测并警告模型不一致情况
						if modelName != "unknown" && modelName != "" {
							lifecycleManager.SetModelWithComparison(modelName, "流式响应解析")
						}
						lifecycleManager.CompleteRequest(finalTokenUsage)
					} else {
						// 没有Token信息，使用HandleNonTokenResponse处理
						lifecycleManager.HandleNonTokenResponse("")
					}
					return
				}

				releaseUpstream()

				// ❌ 出现错误，记录尝试次数
				globalAttemptCount := lifecycleManager.IncrementAttempt()
				lastErr = err

				// 错误处理 - 先构造HTTP状态码错误（保持现有逻辑）
				if err == nil && resp != nil && !IsSuccessStatus(resp.StatusCode) {
					closeErr := resp.Body.Close() // 立即关闭非成功响应体
					if closeErr != nil {
						slog.Warn(fmt.Sprintf("⚠️ [响应体关闭失败] [%s] 端点: %s, Close错误: %v", connID, ep.Config.Name, closeErr))
					}
					// 构造HTTP状态码错误，确保RetryManager能正确分类429等状态
					lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
				} else if err != nil && resp != nil {
					closeErr := resp.Body.Close()
					if closeErr != nil {
						slog.Warn(fmt.Sprintf("⚠️ [错误响应体关闭失败] [%s] 端点: %s, Close错误: %v", connID, ep.Config.Name, closeErr))
					}
				}

				// 🔧 使用增强的RetryManager进行统一决策
				errorRecovery := sh.errorRecoveryFactory.NewErrorRecoveryManager(sh.usageTracker)
				errorCtx := errorRecovery.ClassifyError(lastErr, connID, ep.Config.Name, ep.Config.Group, attempt-1)

				// 🚀 [状态机重构] Phase 4: 分离状态转换与失败原因记录
				// 预设错误上下文（避免重复分类），由HandleError统一记录失败原因
				lifecycleManager.PrepareErrorContext(&errorCtx)
				lifecycleManager.HandleError(lastErr)

				// 创建重试管理器
				retryMgr := sh.retryManagerFactory.NewRetryManager()
				// 🔢 [关键修复] 分离局部和全局计数语义
				// attempt: 当前端点内的尝试次数，用于退避计算
				// globalAttemptCount: 全局尝试次数，用于限流策略
				decision := retryMgr.ShouldRetryWithDecision(&errorCtx, attempt, globalAttemptCount, true) // 流式请求: isStreaming=true
				lastDecision = &decision                                                                   // 保存决策，供外层逻辑使用

				// 检查决策结果
				if decision.FinalStatus == "cancelled" {
					// 🔧 [修复] 添加生命周期状态更新
					lifecycleManager.CancelRequest("client disconnected", nil)
					slog.Info(fmt.Sprintf("🚫 [客户端取消检测] [%s] 检测到客户端取消，立即停止重试", connID))

					// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
					*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
					fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
					flusher.Flush()
					return
				}

				// 处理挂起决策
				if decision.SuspendRequest {
					if sh.sharedSuspensionManager.ShouldSuspend(ctx) {
						// 🚀 [状态机重构] Phase 4: 挂起时更新状态
						lifecycleManager.UpdateStatus("suspended", -1, 0)
						slog.Info(fmt.Sprintf("⏸️ [流式挂起] [%s] 原因: %s，失败端点: %s", connID, decision.Reason, ep.Config.Name))
						fmt.Fprintf(w, "data: suspend: 请求已挂起，等待端点 %s 恢复或组切换...\n\n", ep.Config.Name)
						flusher.Flush()

						// 🚀 [端点自愈] 使用新的端点恢复等待方法，能区分成功/超时/取消
						result := sh.sharedSuspensionManager.WaitForEndpointRecoveryWithResult(ctx, connID, ep.Config.Name)
						switch result {
						case SuspensionSuccess:
							slog.Info(fmt.Sprintf("🎯 [恢复成功] [%s] 端点 %s 已恢复或组已切换，重新开始处理", connID, ep.Config.Name))
							fmt.Fprintf(w, "data: resume: 端点已恢复，重新开始处理...\n\n")
							flusher.Flush()
							continue restartLoop
						case SuspensionCancelled:
							// 🎯 [挂起取消区分] 用户在挂起期间取消请求，应该记录为取消而非失败
							slog.Info(fmt.Sprintf("🚫 [挂起期间取消] [%s] 用户在挂起期间取消请求", connID))
							// 🔧 [状态码修复] 设置取消状态码到上下文用于日志记录
							*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
							lifecycleManager.CancelRequest("suspended then cancelled", nil)
							fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
							flusher.Flush()
							return
						case SuspensionTimeout:
							// 🔧 [修复] 添加生命周期状态更新
							currentAttemptCount := lifecycleManager.GetAttemptCount()
							lifecycleManager.UpdateStatus("error", currentAttemptCount, http.StatusBadGateway)
							slog.Warn(fmt.Sprintf("⏰ [挂起超时] [%s] 等待端点恢复或组切换超时", connID))
							fmt.Fprintf(w, "data: error: 挂起等待超时\n\n")
							flusher.Flush()
							return
						}
					}
				}

				if !decision.RetrySameEndpoint {
					if decision.SwitchEndpoint {
						slog.Info(fmt.Sprintf("🔀 [切换端点] [%s] 当前端点: %s, 原因: %s",
							connID, ep.Config.Name, decision.Reason))
						break // 尝试下一个端点（passthrough架构下不会进入此分支）
					} else {
						// 🎯 [请求级穿透] 2025-12-25 - 阶段1错误（连接阶段）
						// 核心原则：每个客户端请求 = 1个上游请求
						// 错误直接返回给客户端，让 Claude Code SDK 自行重试

						// 记录到 FailureTracker（如果需要）
						if decision.ShouldRecord {
							failCount := sh.endpointManager.RecordFailure(ep.Config.Name)
							slog.Info(fmt.Sprintf("📊 [失败追踪] [%s] 端点 %s 失败，窗口内失败次数: %d",
								connID, ep.Config.Name, failCount))
						}

						// 获取真实状态码（用于内部记录）
						statusCode := GetStatusCodeFromError(lastErr, lastResp)
						if statusCode == 0 {
							switch decision.FinalStatus {
							case "cancelled":
								statusCode = 499
							case "auth_error":
								statusCode = http.StatusUnauthorized
							case "rate_limited":
								statusCode = http.StatusTooManyRequests
							default:
								statusCode = http.StatusBadGateway
							}
						}

						// 🎯 [客户端重试控制] 2025-12-25
						// 对于可重试错误，统一返回 500 + Retry-After: 失败阈值
						// 避免 Claude Code SDK 的不同重试策略干扰 ccf 的故障转移逻辑
						clientStatusCode := statusCode
						retryAfter := decision.RetryAfterSeconds
						if errorCtx.ErrorType == ErrorTypeRateLimit || errorCtx.ErrorType == ErrorTypeServerError {
							clientStatusCode = http.StatusInternalServerError // 统一返回 500
							// 从配置读取失败阈值作为重试延迟
							if threshold := sh.endpointManager.GetConfig().FailureTracker.Threshold; threshold > 0 {
								retryAfter = threshold
							} else {
								retryAfter = 3 // 默认 3 秒
							}
							slog.Debug(fmt.Sprintf("🔄 [客户端重试控制] [%s] 真实状态: %d, 返回: 500, Retry-After: %d",
								connID, statusCode, retryAfter))
						}

						// 添加 Retry-After 头
						if retryAfter > 0 {
							w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
						}

						// 标记请求失败（使用真实状态码）
						failureReason := lifecycleManager.MapErrorTypeToFailureReason(errorCtx.ErrorType)
						lifecycleManager.FailRequest(failureReason, lastErr.Error(), statusCode)

						// 终止重试，使用 SSE 格式返回错误（使用转换后的状态码）
						slog.Info(fmt.Sprintf("🛑 [终止重试] [%s] 端点: %s, 状态: %s, 返回状态码: %d, 原因: %s",
							connID, ep.Config.Name, decision.FinalStatus, clientStatusCode, decision.Reason))

						// 🔧 [SSE 格式修复] 使用 SSE 格式返回错误，而不是 http.Error（会覆盖 Content-Type）
						w.WriteHeader(clientStatusCode)
						fmt.Fprintf(w, "data: error: %s\n\n", decision.Reason)
						flusher.Flush()
						return
					}
				}

				// 🚀 [状态机重构] Phase 4: 重试状态管理
				if decision.RetrySameEndpoint && attempt < sh.config.Retry.MaxAttempts {
					// 更新为重试状态
					lifecycleManager.UpdateStatus("retry", globalAttemptCount, 0)

					// 如果不是最后一次尝试，等待重试延迟
					slog.Info(fmt.Sprintf("⏳ [等待重试] [%s] 端点: %s, 延迟: %v, 原因: %s",
						connID, ep.Config.Name, decision.Delay, decision.Reason))

					// 向客户端发送重试信息
					fmt.Fprintf(w, "data: retry: 重试端点 %s (尝试 %d/%d)，等待 %v...\n\n",
						ep.Config.Name, attempt+1, sh.config.Retry.MaxAttempts, decision.Delay)
					flusher.Flush()

					// 等待延迟，同时检查取消
					select {
					case <-ctx.Done():
						slog.Info(fmt.Sprintf("🚫 [重试取消] [%s] 等待重试期间检测到取消", connID))
						lifecycleManager.CancelRequest("client disconnected during retry delay", nil)

						// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
						*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
						fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
						flusher.Flush()
						return
					case <-time.After(decision.Delay):
						// 继续下一次重试
					}
				}
			}

			// 🔧 当前端点所有重试都失败了
			if !endpointSuccess {
				// 修复计数逻辑：处理提前break和自然跑满两种情况
				actualAttempts := attempt
				if actualAttempts > sh.config.Retry.MaxAttempts {
					actualAttempts = sh.config.Retry.MaxAttempts
				}

				// 🚀 [改进版方案1] 使用已保存的重试决策，避免重复错误分类
				var willSwitchEndpoint bool = true
				if lastDecision != nil {
					willSwitchEndpoint = lastDecision.SwitchEndpoint

					// 对于不切换端点的决策（如HTTP错误、流式错误等），直接终止
					if !willSwitchEndpoint && lastDecision.FinalStatus != "" {
						slog.Info(fmt.Sprintf("❌ [决策终止] [%s] %s，不尝试其他端点", connID, lastDecision.Reason))
						// 🚀 [状态机重构] Phase 4: 使用FailRequest方法标记最终失败
						failureReason := "unknown_error"
						if lastErr != nil {
							// 重新分类错误以获取准确的失败原因
							errorRecovery := sh.errorRecoveryFactory.NewErrorRecoveryManager(sh.usageTracker)
							errorCtx := errorRecovery.ClassifyError(lastErr, connID, "", "", 0)
							failureReason = lifecycleManager.MapErrorTypeToFailureReason(errorCtx.ErrorType)
						}
						// 获取真实的HTTP状态码
						statusCode := GetStatusCodeFromError(lastErr, lastResp)
						if statusCode == 0 {
							// 根据决策状态设置合适的默认状态码
							if lastDecision != nil && lastDecision.FinalStatus != "" {
								switch lastDecision.FinalStatus {
								case "cancelled":
									statusCode = 499 // nginx风格的客户端取消码
								case "auth_error":
									statusCode = http.StatusUnauthorized
								case "rate_limited":
									statusCode = http.StatusTooManyRequests
								default:
									statusCode = http.StatusBadGateway
								}
							} else {
								statusCode = http.StatusBadGateway
							}
						}
						lifecycleManager.FailRequest(failureReason, lastDecision.Reason, statusCode)
						fmt.Fprintf(w, "data: error: %s\n\n", lastDecision.Reason)
						flusher.Flush()
						return
					}
				}

				// 根据是否会切换端点来显示不同的日志
				if actualAttempts == 1 {
					if willSwitchEndpoint {
						slog.Warn(fmt.Sprintf("❌ [端点失败] [%s] 端点: %s 第1次尝试失败，切换端点",
							connID, ep.Config.Name))
					} else {
						slog.Warn(fmt.Sprintf("❌ [端点失败] [%s] 端点: %s 第1次尝试失败，直接终止",
							connID, ep.Config.Name))
					}
				} else {
					slog.Warn(fmt.Sprintf("❌ [端点失败] [%s] 端点: %s 共尝试 %d 次均失败",
						connID, ep.Config.Name, actualAttempts))
				}

				// 如果不是最后一个端点，尝试下一个端点
				if i < len(endpoints)-1 {
					fmt.Fprintf(w, "data: retry: 切换到备用端点: %s\n\n", endpoints[i+1].Config.Name)
					flusher.Flush()
					continue
				}
			}
		}

		// 🔄 [请求级故障转移] 所有端点都失败了，尝试触发故障转移
		if lastFailedEndpoint != "" {
			newEndpointName, err := sh.endpointManager.TriggerRequestFailover(
				lastFailedEndpoint,
				"all_retries_exhausted",
			)

			if err == nil && newEndpointName != "" {
				slog.Info(fmt.Sprintf("🔄 [请求级故障转移] [%s] 端点 %s 进入冷却，切换到 %s",
					connID, lastFailedEndpoint, newEndpointName))
				// 故障转移成功，重新获取端点列表继续处理
				fmt.Fprintf(w, "data: failover: 端点 %s 故障，已切换到 %s\n\n", lastFailedEndpoint, newEndpointName)
				flusher.Flush()
				continue restartLoop
			} else if err != nil {
				slog.Warn(fmt.Sprintf("⚠️ [请求级故障转移失败] [%s] 端点: %s, 错误: %v",
					connID, lastFailedEndpoint, err))
			}
		}

		// 🔧 所有当前端点都失败，检查是否应该挂起请求
		// 注意：客户端取消错误已在上面统一处理，这里不会执行到

		// 🔧 [修复] 使用共享的SuspensionManager实例，确保全局挂起限制生效
		suspensionMgr := sh.sharedSuspensionManager

		// 检查是否应该挂起请求
		if suspensionMgr.ShouldSuspend(ctx) {
			currentEndpoints := sh.endpointManager.GetHealthyEndpoints()
			if cfg := sh.endpointManager.GetConfig(); cfg != nil && cfg.Strategy.Type == "fastest" && cfg.Strategy.FastTestEnabled {
				currentEndpoints = sh.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
			}

			// 🚀 [状态机重构] Phase 4: 挂起时更新状态（移除重复的失败原因记录）
			lifecycleManager.UpdateStatus("suspended", -1, 0)
			fmt.Fprintf(w, "data: suspend: 当前所有组均不可用，请求已挂起等待组切换...\n\n")
			flusher.Flush()

			// 🔢 [语义修复] 在日志中记录端点数量信息，但不影响重试计数语义
			actualAttemptCount := lifecycleManager.GetAttemptCount()
			slog.Info(fmt.Sprintf("⏸️ [流式挂起] [%s] 请求已挂起，尝试次数: %d, 可用端点数: %d, 最后失败端点: %s",
				connID, actualAttemptCount, len(currentEndpoints), lastFailedEndpoint))

			// 🚀 [端点自愈] 等待端点恢复，能区分成功/超时/取消
			result := suspensionMgr.WaitForEndpointRecoveryWithResult(ctx, connID, lastFailedEndpoint)
			switch result {
			case SuspensionSuccess:
				slog.Info(fmt.Sprintf("🚀 [挂起恢复] [%s] 端点 %s 已恢复或组切换完成，重新获取端点", connID, lastFailedEndpoint))
				fmt.Fprintf(w, "data: resume: 组切换完成，恢复处理...\n\n")
				flusher.Flush()

				var newEndpoints []*endpoint.Endpoint
				if sh.endpointManager.GetConfig().Strategy.Type == "fastest" && sh.endpointManager.GetConfig().Strategy.FastTestEnabled {
					newEndpoints = sh.endpointManager.GetFastestEndpointsWithRealTimeTest(ctx)
				} else {
					newEndpoints = sh.endpointManager.GetHealthyEndpoints()
				}

				if len(newEndpoints) > 0 {
					// 更新端点列表，重新开始处理
					endpoints = newEndpoints
					slog.Info(fmt.Sprintf("🔄 [重新开始] [%s] 获取到 %d 个新端点，重新开始流式处理", connID, len(newEndpoints)))

					// 🔧 [生命周期修复] 恢复时必须更新生命周期管理器的端点信息
					// 设置第一个新端点的信息到生命周期管理器
					firstEndpoint := newEndpoints[0]
					lifecycleManager.SetEndpoint(firstEndpoint.Config.Name, firstEndpoint.Config.Group, firstEndpoint.Config.Channel)

					continue restartLoop
				}
			case SuspensionCancelled:
				// 🎯 [挂起取消区分] 用户在挂起期间取消请求，应该记录为取消而非失败
				slog.Info(fmt.Sprintf("🚫 [挂起期间取消] [%s] 用户在挂起期间取消请求", connID))
				// 🔧 [状态码修复] 设置取消状态码到上下文用于日志记录
				*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
				lifecycleManager.CancelRequest("suspended then cancelled", nil)
				fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
				flusher.Flush()
				return
			case SuspensionTimeout:
				slog.Warn(fmt.Sprintf("⏰ [挂起超时] [%s] 挂起等待超时", connID))
				// 继续执行下面的失败处理逻辑
			}
		}

		// 🚀 [状态机重构] Phase 4: 最终失败处理
		// 所有端点都失败了，使用FailRequest方法标记最终失败（修复：使用GetStatusCodeFromError计算正确状态码）
		statusCode := GetStatusCodeFromError(lastErr, lastResp)
		if statusCode == 0 {
			statusCode = http.StatusBadGateway // 端点耗尽的默认状态码
		}

		// 🔧 [日志状态码] 设置真实错误码到上下文用于日志记录
		*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", statusCode))

		lifecycleManager.FailRequest("endpoint_exhausted", "All endpoints failed, last error: "+fmt.Sprintf("%v", lastErr), statusCode)
		fmt.Fprintf(w, "data: error: All endpoints failed, last error: %v\n\n", lastErr)
		flusher.Flush()
		return
	}

	connID := lifecycleManager.GetRequestID()
	slog.Error(fmt.Sprintf("❌ [流式重启超限] [%s] 连续重启超过上限 %d 次，终止处理", connID, maxStreamingExecutionRestarts))
	lifecycleManager.FailRequest("streaming_restart_limit_exceeded", "streaming retry restart limit exceeded", http.StatusServiceUnavailable)
	fmt.Fprintf(w, "data: error: streaming retry restart limit exceeded\n\n")
	flusher.Flush()
}
