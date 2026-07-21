package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/proxy/handlers"
)

// 端点侧扁平转发管线（方案 §4.4）：单层候选循环替代旧 streaming/regular 双循环壳。
// 候选由 PrepareRouteCandidates 提供（无写副作用），失败分类走 §3.1 决策表，
// 响应处理返回显式 EndpointProcessOutcome，仅 FullSuccess 驱动 activeEndpoint CAS 迁移。

// EndpointProcessOutcome P2 响应处理的显式结局（替代 "err == nil 即成功"）
type EndpointProcessOutcome int

const (
	// ProcessOutcomeFullSuccess 完整成功
	ProcessOutcomeFullSuccess EndpointProcessOutcome = iota
	// ProcessOutcomeQualityIncomplete 流不完整但已完成（CompleteRequestWithQuality 路径）；不迁移 activeEndpoint
	ProcessOutcomeQualityIncomplete
	// ProcessOutcomeFailedAfterCommit 已开始输出后失败（FailRequest 路径）
	ProcessOutcomeFailedAfterCommit
	// ProcessOutcomeCancelled 客户端取消
	ProcessOutcomeCancelled
)

const (
	// endpointPassthroughRetryAfterSeconds 穿透错误的建议客户端重试等待
	endpointPassthroughRetryAfterSeconds = 5
	endpointClientErrorBodySampleLimit   = 2048
	endpointStreamTailDrainTimeout       = time.Second
)

// handleEndpointPipeline 端点转发主入口（streaming 与 regular 合并，仅 Process 分叉）
func (h *Handler) handleEndpointPipeline(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager *RequestLifecycleManager, isSSE bool) {
	connID := lifecycleManager.GetRequestID()

	// reject 模式前置检查（保留现状语义）
	if shouldReject, rejectedEndpoint := h.endpointManager.ShouldRejectRequest(); shouldReject {
		h.endpointManager.BeginEndpointScheduleSnapshot(connID, r.URL.Path, nil)
		slog.Warn(fmt.Sprintf("❌ [失败追踪] [%s] 端点 %s 达到失败阈值，拒绝请求（reject 模式）", connID, rejectedEndpoint))
		message := fmt.Sprintf("Service temporarily unavailable: endpoint %s failure threshold reached", rejectedEndpoint)
		lifecycleManager.FailRequest("rejected_by_failure_tracker",
			fmt.Sprintf("Endpoint %s reached failure threshold, request rejected", rejectedEndpoint),
			http.StatusServiceUnavailable)
		h.endpointManager.CompleteEndpointScheduleSnapshot(connID, "", endpoint.EndpointScheduleOutcomeRejectedByFailureTracker, message)
		http.Error(w, message, http.StatusServiceUnavailable)
		return
	}

	var flusher http.Flusher
	hasCommitted := func() bool { return false }
	if isSSE {
		w, flusher, hasCommitted = handlers.NewStreamingResponseWriter(w)
		setEndpointStreamingHeaders(w)
	}

	profile := endpoint.BuildRouteRequestProfile(r.URL.Path, bodyBytes)
	result := h.endpointManager.PrepareRouteCandidates(ctx, profile)
	h.endpointManager.BeginEndpointScheduleSnapshot(connID, r.URL.Path, result.Snapshot)

	if len(result.Candidates) == 0 {
		if block := h.endpointManager.GetManualFixedRouteBlock(profile); block != nil {
			h.endpointManager.NoteRouteDecision(block.Endpoint, block.Reason)
			lifecycleManager.FailRequest(block.Reason, block.Message, block.StatusCode)
			h.endpointManager.CompleteEndpointScheduleSnapshot(connID, "", endpoint.EndpointScheduleOutcomeManualFixedBlocked, block.Message)
			h.writeEndpointPipelineError(w, isSSE, flusher, block.StatusCode,
				fmt.Sprintf("%s: %s", block.Code, block.Message), block.RetryAfter)
			return
		}
		retryAfter := h.endpointRetryAfterSeconds(result.Snapshot)
		lifecycleManager.FailRequest("no_endpoints_available", "no routable endpoint candidates", http.StatusServiceUnavailable)
		h.endpointManager.CompleteEndpointScheduleSnapshot(connID, "", endpoint.EndpointScheduleOutcomeNoCandidates, "no routable endpoint candidates")
		h.writeEndpointPipelineError(w, isSSE, flusher, http.StatusServiceUnavailable,
			"Service Unavailable: no routable endpoint candidates", retryAfter)
		return
	}

	var lastErr error
	for i, ep := range result.Candidates {
		select {
		case <-ctx.Done():
			lifecycleManager.CancelRequest("client disconnected", nil)
			h.endpointManager.CompleteEndpointScheduleSnapshot(connID, "", endpoint.EndpointScheduleOutcomeCancelled, "client disconnected")
			*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", 499))
			if isSSE && hasCommitted() {
				fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
				flusher.Flush()
			}
			return
		default:
		}

		lifecycleManager.SetEndpoint(ep.Config.Name, ep.Config.Group, ep.Config.Channel)
		h.endpointManager.RecordEndpointScheduleAttempt(connID, ep.Config.Name, endpoint.EndpointScheduleRuntimeAttempting, "")
		h.endpointManager.NoteRouteDecision(ep.Config.Name, "")
		lifecycleManager.UpdateStatus("forwarding", i, 0)
		*r = *r.WithContext(context.WithValue(r.Context(), "selected_endpoint", ep.Config.Name))

		resp, upstreamCancel, traceState, forwardErr := h.forwarder.ForwardForPipeline(
			ctx, r, bodyBytes, ep, isSSE, isSSE && shouldEnableEndpointTailDrain(r), lifecycleManager.SetFirstTokenStartTime)
		releaseUpstream := func() {
			if upstreamCancel != nil {
				upstreamCancel()
				upstreamCancel = nil
			}
		}

		// 🛡️ 隐私策略短路：本地策略拒绝不是上游失败，不换候选、不标记端点
		if policyErr := handlers.AsPrivacyPolicyError(forwardErr); policyErr != nil {
			releaseUpstream()
			slog.Warn(fmt.Sprintf("🛡️ [隐私保护] [%s] 请求被策略拒绝: %s", connID, policyErr.Code))
			*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", policyErr.StatusCode))
			lifecycleManager.FailRequest(handlers.PrivacyFailureReason(policyErr), policyErr.Message, policyErr.StatusCode)
			h.endpointManager.CompleteEndpointScheduleSnapshot(connID, ep.Config.Name, endpoint.EndpointScheduleOutcomePrivacyBlocked, policyErr.Message)
			h.writeEndpointPipelineError(w, isSSE, flusher, policyErr.StatusCode,
				fmt.Sprintf("%s: %s", policyErr.Code, policyErr.Message), 0)
			return
		}

		// 4xx 分类需要 body 样本（窥读并复原，供模型不支持 / schema 不兼容判定）
		bodySample := ""
		if forwardErr == nil && resp != nil && resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError {
			bodySample = readAndRestoreResponseBody(resp, endpointClientErrorBodySampleLimit)
		}
		decision := decideEndpointForwardOutcome(forwardErr, resp, traceState, bodySample)
		lifecycleManager.IncrementAttempt()

		switch decision.Action {
		case EndpointForwardNextCandidate:
			detail := endpointFailureDetail(forwardErr, resp, ep.Config.Name)
			closeEndpointResponse(resp)
			releaseUpstream()
			lastErr = fmt.Errorf("%s: %s", decision.Reason, detail)
			h.markEndpointFailure(ep, decision, profile, detail)
			h.endpointManager.RecordEndpointScheduleAttempt(connID, ep.Config.Name, endpoint.EndpointScheduleRuntimeTryNext, detail)
			slog.Warn(fmt.Sprintf("🔄 [端点管线] [%s] 端点 %s 失败（%s），换下一候选 (%d/%d)",
				connID, ep.Config.Name, decision.Reason, i+1, len(result.Candidates)))
			continue

		case EndpointForwardPassthroughError:
			detail := endpointFailureDetail(forwardErr, resp, ep.Config.Name)
			closeEndpointResponse(resp)
			releaseUpstream()
			h.markEndpointFailure(ep, decision, profile, detail)
			// 「真实码→500 + Retry-After」客户端重试控制转换仅此一份（§3.1）
			*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", http.StatusInternalServerError))
			lifecycleManager.FailRequest(decision.Reason, detail, http.StatusInternalServerError)
			h.endpointManager.CompleteEndpointScheduleSnapshot(connID, ep.Config.Name, endpoint.EndpointScheduleOutcomePassthroughError, detail)
			h.writeEndpointPipelineError(w, isSSE, flusher, http.StatusInternalServerError,
				fmt.Sprintf("upstream failure (%s): %s", decision.Reason, detail), endpointPassthroughRetryAfterSeconds)
			return

		case EndpointForwardPassthroughRaw:
			detail := fmt.Sprintf("upstream returned %d", resp.StatusCode)
			relayErr := h.relayEndpointRawResponse(w, resp, lifecycleManager, isSSE, flusher)
			releaseUpstream()
			if relayErr != nil {
				h.endpointManager.CompleteEndpointScheduleSnapshot(connID, ep.Config.Name, endpoint.EndpointScheduleOutcomePassthroughError, relayErr.Error())
			} else {
				h.endpointManager.CompleteEndpointScheduleSnapshot(connID, ep.Config.Name, endpoint.EndpointScheduleOutcomePassthroughRaw, detail)
			}
			return

		default: // EndpointForwardProcess
			var outcome EndpointProcessOutcome
			var outcomeDetail string
			if isSSE {
				outcome, outcomeDetail = h.processEndpointStreamingResponse(ctx, w, r, resp, flusher, lifecycleManager, ep.Config.Name, upstreamCancel)
			} else {
				outcome, outcomeDetail = h.processEndpointRegularResponse(w, r, resp, lifecycleManager, ep.Config.Name)
			}
			releaseUpstream()
			h.endpointManager.CompleteEndpointScheduleSnapshot(connID, ep.Config.Name, endpointScheduleOutcomeName(outcome), outcomeDetail)
			// Outcome 门控：仅完整成功驱动 activeEndpoint 迁移（QualityIncomplete 不足以证明端点优于当前 active）
			if outcome == ProcessOutcomeFullSuccess && ep.Config.Name != result.ActiveEndpointAtSelection {
				if h.endpointManager.TryMigrateActiveEndpoint(ep.Config.Name, result.ActiveRevision, result.RouteOverrideRevision) {
					slog.Info(fmt.Sprintf("🔀 [成功驱动迁移] [%s] activeEndpoint: %s → %s",
						connID, result.ActiveEndpointAtSelection, ep.Config.Name))
				}
			}
			return
		}
	}

	// 全部候选失败：503 + Retry-After（快照 availableAt）
	reason := "all endpoint candidates failed"
	if lastErr != nil {
		reason = lastErr.Error()
	}
	retryAfter := h.endpointRetryAfterSeconds(result.Snapshot)
	lifecycleManager.FailRequest("all_endpoints_failed", reason, http.StatusServiceUnavailable)
	h.endpointManager.CompleteEndpointScheduleSnapshot(connID, "", endpoint.EndpointScheduleOutcomeAllCandidatesFailed, reason)
	h.writeEndpointPipelineError(w, isSSE, flusher, http.StatusServiceUnavailable,
		"Service Unavailable: "+reason, retryAfter)
}

// markEndpointFailure 按决策标记端点状态（§3.1 状态标记列）
func (h *Handler) markEndpointFailure(ep *endpoint.Endpoint, decision EndpointFailureDecision, profile endpoint.RouteRequestProfile, detail string) {
	name := ep.Config.Name
	switch decision.Mark {
	case EndpointMarkFailureWindow:
		failCount := h.endpointManager.RecordFailure(name)
		slog.Info(fmt.Sprintf("📊 [失败追踪] 端点 %s 记录失败（%s），窗口内失败次数: %d", name, decision.Reason, failCount))
	case EndpointMarkAuthCooldown:
		h.endpointManager.SetEndpointCooldown(name, endpointAuthCooldown(), decision.Reason)
		slog.Warn(fmt.Sprintf("🔑 [鉴权冷却] 端点 %s 进入长冷却（%s）: %s", name, endpointAuthCooldown(), detail))
	case EndpointMarkRateLimitCooldown:
		cooldown := endpointRateLimitCooldown(decision)
		h.endpointManager.SetEndpointCooldown(name, cooldown, decision.Reason)
		slog.Warn(fmt.Sprintf("⏳ [限流冷却] 端点 %s 短冷却 %s（不计故障阈值）", name, cooldown))
	case EndpointMarkNegativeCache:
		h.endpointManager.RecordNegativeRouteHit(name, string(decision.FailureClass), profile, detail)
		slog.Info(fmt.Sprintf("🧭 [路由负缓存] 端点 %s 记录 %s（不计失败窗口）", name, decision.FailureClass))
	}
}

// processEndpointStreamingResponse P2 流式响应处理（自旧循环壳迁移，内部逻辑只搬不改；
// 新增仅为 EndpointProcessOutcome 返回值包装）
func (h *Handler) processEndpointStreamingResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, resp *http.Response, flusher http.Flusher, lifecycleManager *RequestLifecycleManager, endpointName string, upstreamCancel context.CancelFunc) (EndpointProcessOutcome, string) {
	connID := lifecycleManager.GetRequestID()
	lifecycleManager.UpdateStatus("processing", lifecycleManager.GetAttemptCount(), resp.StatusCode)
	w.WriteHeader(resp.StatusCode)

	tokenParser := NewTokenParserWithUsageTracker(connID, h.usageTracker)
	processor := NewStreamProcessor(tokenParser, h.usageTracker, w, flusher, connID, endpointName)
	processor.SetStreamTimingRecorders(lifecycleManager.RecordFirstTokenAndReturn, lifecycleManager.RecordStreamCompletion)
	if upstreamCancel != nil {
		processor.EnableDownstreamTailDrain(endpointStreamTailDrainTimeout, upstreamCancel)
	}

	slog.Info(fmt.Sprintf("🚀 [开始流式处理] [%s] 端点: %s", connID, endpointName))
	finalTokenUsage, modelName, err := processor.ProcessStreamWithRetry(ctx, resp)
	if err != nil {
		if streamErr, ok := err.(handlers.StreamIncompleteErrorInterface); ok {
			// 流不完整但请求已完成：按完成记账并标记 failure_reason（不计失败、不迁移 active）
			parsedModelName := streamErr.GetModelName()
			failureReason := streamErr.GetFailureReason()
			if parsedModelName != "unknown" && parsedModelName != "" && parsedModelName != "default" {
				lifecycleManager.SetModelWithComparison(parsedModelName, "stream_incomplete")
			} else if modelName != "unknown" && modelName != "" && modelName != "default" {
				lifecycleManager.SetModelWithComparison(modelName, "stream_processor")
			}
			lifecycleManager.CompleteRequestWithQuality(finalTokenUsage, failureReason)
			slog.Info(fmt.Sprintf("⚠️ [流不完整但已完成] [%s] 端点: %s, failure_reason: %s",
				connID, endpointName, failureReason))
			return ProcessOutcomeQualityIncomplete, failureReason
		}

		status, parsedModelName := "error", "unknown"
		if strings.HasPrefix(err.Error(), "stream_status:") {
			parts := strings.SplitN(err.Error(), ":", 5)
			if len(parts) >= 4 {
				status = parts[1]
				if parts[2] == "model" && parts[3] != "" {
					parsedModelName = parts[3]
				}
			}
		}
		if parsedModelName != "unknown" && parsedModelName != "" {
			lifecycleManager.SetModelWithComparison(parsedModelName, "stream_status")
		} else if modelName != "unknown" && modelName != "" {
			lifecycleManager.SetModelWithComparison(modelName, "stream_processor")
		}

		lifecycleManager.HandleError(err)
		statusCode := handlers.GetStatusCodeFromError(err, resp)
		if status == "error" || status == "stream_error" {
			statusCode = http.StatusMultiStatus
		} else if status == "cancelled" {
			statusCode = 499
		}

		if status == "cancelled" {
			lifecycleManager.CancelRequest("stream processing cancelled", finalTokenUsage)
			*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", statusCode))
			fmt.Fprintf(w, "data: cancelled: 客户端取消请求\n\n")
			flusher.Flush()
			return ProcessOutcomeCancelled, err.Error()
		}

		if finalTokenUsage != nil {
			lifecycleManager.RecordTokensForFailedRequest(finalTokenUsage, status)
		}
		lifecycleManager.FailRequest(status, err.Error(), statusCode)
		*r = *r.WithContext(context.WithValue(r.Context(), "final_status_code", statusCode))
		slog.Warn(fmt.Sprintf("🔄 [流式处理失败] [%s] 端点: %s, 状态: %s, 错误: %v", connID, endpointName, status, err))

		// 📊 [失败追踪] P2 流式失败计入失败窗口（§3.1，取消除外——已提前返回）
		failCount := h.endpointManager.RecordFailure(endpointName)
		slog.Info(fmt.Sprintf("📊 [失败追踪] [%s] 端点 %s 流式阶段失败，窗口内失败次数: %d", connID, endpointName, failCount))

		if h.config.Streaming.EOFRetryHint {
			interruptModelName := parsedModelName
			if interruptModelName == "" || interruptModelName == "unknown" {
				interruptModelName = modelName
			}
			if interruptModelName == "" || interruptModelName == "unknown" {
				interruptModelName = "claude-3-5-sonnet-20241022"
			}
			slog.Info(fmt.Sprintf("🔄 [流式错误重试提示] [%s] 发送中断消息触发客户端重试, 模型: %s", connID, interruptModelName))
			handlers.SendStreamInterruptedMessage(w, flusher, "Connection interrupted", interruptModelName)
		} else {
			fmt.Fprintf(w, "data: error: 流式处理失败: %v\n\n", err)
			flusher.Flush()
		}
		return ProcessOutcomeFailedAfterCommit, err.Error()
	}

	if finalTokenUsage != nil {
		if modelName != "unknown" && modelName != "" {
			lifecycleManager.SetModelWithComparison(modelName, "流式响应解析")
		}
		lifecycleManager.CompleteRequest(finalTokenUsage)
	} else {
		lifecycleManager.HandleNonTokenResponse("")
	}
	return ProcessOutcomeFullSuccess, ""
}

// processEndpointRegularResponse P2 常规响应处理（自旧循环壳迁移，内部逻辑只搬不改）
func (h *Handler) processEndpointRegularResponse(w http.ResponseWriter, r *http.Request, resp *http.Response, lifecycleManager *RequestLifecycleManager, endpointName string) (EndpointProcessOutcome, string) {
	connID := lifecycleManager.GetRequestID()
	lifecycleManager.UpdateStatus("processing", lifecycleManager.GetAttemptCount(), resp.StatusCode)
	defer resp.Body.Close()

	responseBytes, err := h.responseProcessor.ProcessResponseBody(resp)
	if err != nil {
		lifecycleManager.FailRequest("response_read_error", err.Error(), http.StatusBadGateway)
		http.Error(w, "Failed to read upstream response", http.StatusBadGateway)
		return ProcessOutcomeFailedAfterCommit, err.Error()
	}

	if len(responseBytes) == 0 && r.URL.Path == "/v1/messages" {
		slog.Error(fmt.Sprintf("❌ [空响应体] [%s] 端点 %s 返回 200 但响应体为空，判定为上游异常", connID, endpointName))
		lifecycleManager.FailRequest("empty_response_body", "Upstream returned 200 with empty body", http.StatusBadGateway)
		http.Error(w, "Upstream returned empty response", http.StatusBadGateway)
		return ProcessOutcomeFailedAfterCommit, "Upstream returned 200 with empty body"
	}

	h.responseProcessor.CopyResponseHeaders(resp, w)
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(responseBytes); err != nil {
		lifecycleManager.HandleError(fmt.Errorf("failed to write response: %w", err))
		slog.Error("Failed to write response to client", "request_id", connID, "error", err)
		lifecycleManager.FailRequest("response_write_error", err.Error(), resp.StatusCode)
		return ProcessOutcomeFailedAfterCommit, err.Error()
	}

	if r.URL.Path == "/v1/messages/count_tokens" {
		lifecycleManager.CompleteRequest(nil)
		return ProcessOutcomeFullSuccess, ""
	}

	tokenUsage, modelName := h.tokenAnalyzer.AnalyzeResponseForTokensUnified(responseBytes, connID, endpointName)
	if tokenUsage != nil {
		if modelName != "unknown" && modelName != "" {
			lifecycleManager.SetModelWithComparison(modelName, "常规响应解析")
		}
		lifecycleManager.CompleteRequest(tokenUsage)
	} else {
		lifecycleManager.HandleNonTokenResponse(string(responseBytes))
	}
	return ProcessOutcomeFullSuccess, ""
}

// relayEndpointRawResponse 原样透传上游 4xx（客户端请求问题，不记录端点状态）
func (h *Handler) relayEndpointRawResponse(w http.ResponseWriter, resp *http.Response, lifecycleManager *RequestLifecycleManager, isSSE bool, flusher http.Flusher) error {
	detail := fmt.Sprintf("upstream returned %d", resp.StatusCode)
	if err := h.writeRawResponse(w, resp); err != nil {
		lifecycleManager.FailRequest("endpoint_response_write_error", err.Error(), resp.StatusCode)
		return err
	}
	if isSSE && flusher != nil {
		flusher.Flush()
	}
	lifecycleManager.FailRequest("upstream_client_error", detail, resp.StatusCode)
	return nil
}

func endpointScheduleOutcomeName(outcome EndpointProcessOutcome) string {
	switch outcome {
	case ProcessOutcomeFullSuccess:
		return endpoint.EndpointScheduleOutcomeSuccess
	case ProcessOutcomeQualityIncomplete:
		return endpoint.EndpointScheduleOutcomeQualityIncomplete
	case ProcessOutcomeCancelled:
		return endpoint.EndpointScheduleOutcomeCancelled
	default:
		return endpoint.EndpointScheduleOutcomeFailedAfterCommit
	}
}

// endpointRetryAfterSeconds 从调度快照的 skipped availableAt 计算 Retry-After；
// 无任何 availableAt 时用 FailureTracker 窗口兜底（§3.1 补充规则）
func (h *Handler) endpointRetryAfterSeconds(snapshot *endpoint.EndpointScheduleSnapshot) int {
	if earliest := snapshot.EarliestAvailableAt(); !earliest.IsZero() {
		if seconds := int(time.Until(earliest).Seconds()) + 1; seconds > 0 {
			return seconds
		}
		return 1
	}
	if h.config != nil && h.config.FailureTracker.TimeWindow > 0 {
		return int(h.config.FailureTracker.TimeWindow.Seconds())
	}
	return 30
}

// writeEndpointPipelineError 错误写回（SSE 与常规分叉）
func (h *Handler) writeEndpointPipelineError(w http.ResponseWriter, isSSE bool, flusher http.Flusher, statusCode int, message string, retryAfterSeconds int) {
	if retryAfterSeconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	}
	if isSSE && flusher != nil {
		handlers.WriteStreamingTerminalError(w, flusher, statusCode, message)
		return
	}
	http.Error(w, message, statusCode)
}

func setEndpointStreamingHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")
}

func shouldEnableEndpointTailDrain(r *http.Request) bool {
	return r != nil && r.URL != nil && r.URL.Path == "/v1/messages"
}

func endpointFailureDetail(forwardErr error, resp *http.Response, endpointName string) string {
	if forwardErr != nil {
		return forwardErr.Error()
	}
	if resp == nil {
		return "empty response"
	}
	if detail := handlers.BuildUpstreamErrorDetail(resp, endpointName, 512); detail != "" {
		return detail
	}
	return fmt.Sprintf("upstream returned %d", resp.StatusCode)
}

func closeEndpointResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}
