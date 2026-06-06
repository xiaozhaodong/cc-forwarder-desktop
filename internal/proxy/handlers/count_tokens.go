package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/transport"
)

// CountTokensHandler 处理 /v1/messages/count_tokens 请求
// 策略：优先转发到标记了 supports_count_tokens 的端点，失败则本地估算
type CountTokensHandler struct {
	config          *config.Config
	endpointManager *endpoint.Manager
	forwarder       *Forwarder
}

// NewCountTokensHandler 创建 CountTokensHandler
func NewCountTokensHandler(cfg *config.Config, em *endpoint.Manager, f *Forwarder) *CountTokensHandler {
	return &CountTokensHandler{
		config:          cfg,
		endpointManager: em,
		forwarder:       f,
	}
}

// CountTokensRequest 定义 count_tokens 请求结构
type CountTokensRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	System   interface{}              `json:"system,omitempty"`
	Tools    []interface{}            `json:"tools,omitempty"`
}

// CountTokensResponse 定义响应结构
type CountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

// Handle 处理count_tokens请求 - 极简逻辑
func (h *CountTokensHandler) Handle(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, connID string) {
	slog.Info(fmt.Sprintf("🔢 [Token计数] [%s] 收到count_tokens请求", connID))
	routeProfile := endpoint.BuildRouteRequestProfile(r.URL.Path, bodyBytes)

	// 1. 找配置了 supports_count_tokens: true 的端点
	supportedEndpoints := h.getSupportedEndpoints(routeProfile)

	// 2. 如果有，尝试转发
	if len(supportedEndpoints) > 0 {
		if result, ok := h.tryForward(ctx, r, bodyBytes, supportedEndpoints, connID); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(result)
			slog.Info(fmt.Sprintf("✅ [Token计数-转发] [%s] 转发成功", connID))
			return
		}
		// 转发失败，降级到估算
		slog.Warn(fmt.Sprintf("⚠️ [Token计数] [%s] 转发失败，降级到本地估算", connID))
	} else {
		slog.Info(fmt.Sprintf("🔍 [Token计数] [%s] 无支持端点，使用本地估算", connID))
	}

	// 3. 本地估算
	h.respondWithEstimation(w, bodyBytes, connID)
}

// getSupportedEndpoints 获取支持count_tokens的端点
func (h *CountTokensHandler) getSupportedEndpoints(profile endpoint.RouteRequestProfile) []*endpoint.Endpoint {
	allEndpoints := h.endpointManager.GetHealthyEndpointsForRoute(profile)
	var supported []*endpoint.Endpoint

	for _, ep := range allEndpoints {
		if ep.Config.SupportsCountTokens {
			supported = append(supported, ep)
		}
	}

	return supported
}

// tryForward 尝试转发到支持的端点
func (h *CountTokensHandler) tryForward(ctx context.Context, r *http.Request, bodyBytes []byte, endpoints []*endpoint.Endpoint, connID string) ([]byte, bool) {
	for _, ep := range endpoints {
		targetURL := ep.Config.URL + "/v1/messages/count_tokens"
		req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			continue
		}

		h.forwarder.CopyHeaders(r, req, ep)

		httpTransport, err := transport.CreateTransport(h.config)
		if err != nil {
			continue
		}

		client := &http.Client{
			Timeout:   ep.Config.Timeout,
			Transport: httpTransport,
		}

		resp, err := client.Do(req)
		if err != nil {
			slog.Debug(fmt.Sprintf("❌ [转发失败] [%s] 端点: %s, 错误: %v", connID, ep.Config.Name, err))
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			if bodyBytes, err := io.ReadAll(resp.Body); err == nil {
				slog.Info(fmt.Sprintf("✅ [转发成功] [%s] 端点: %s", connID, ep.Config.Name))
				return bodyBytes, true
			}
			continue
		}

		errorBody, _ := io.ReadAll(resp.Body)
		if isCountTokensUnsupported(resp.StatusCode, string(errorBody)) {
			profile := endpoint.BuildRouteRequestProfile(r.URL.Path, bodyBytes)
			h.endpointManager.RecordNegativeRouteHit(ep.Config.Name, endpoint.FailureClassCountTokensUnsupported, profile, string(errorBody))
			slog.Info(fmt.Sprintf("🧭 [Token计数] [%s] 端点 %s 不支持 count_tokens，写入负向缓存", connID, ep.Config.Name))
		}
	}

	return nil, false
}

func isCountTokensUnsupported(statusCode int, body string) bool {
	if statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed || statusCode == http.StatusNotImplemented {
		return true
	}
	lower := strings.ToLower(body)
	mentionsCountTokens := strings.Contains(lower, "count_tokens") ||
		strings.Contains(lower, "count tokens") ||
		strings.Contains(lower, "count token") ||
		strings.Contains(lower, "token counting")
	if !mentionsCountTokens {
		return false
	}
	return strings.Contains(lower, "not implemented") ||
		strings.Contains(lower, "unsupported") ||
		strings.Contains(lower, "not supported")
}

// respondWithEstimation 返回本地估算结果
func (h *CountTokensHandler) respondWithEstimation(w http.ResponseWriter, bodyBytes []byte, connID string) {
	tokens, err := h.estimateTokens(bodyBytes)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to estimate tokens: %v", err), http.StatusBadRequest)
		return
	}

	response := CountTokensResponse{InputTokens: tokens}
	responseBytes, _ := json.Marshal(response)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Token-Estimation", "true") // 标记这是估算值
	w.WriteHeader(http.StatusOK)
	w.Write(responseBytes)

	slog.Info(fmt.Sprintf("📊 [Token估算] [%s] 估算结果: %d tokens", connID, tokens))
}

// estimateTokens 本地估算token数量
func (h *CountTokensHandler) estimateTokens(bodyBytes []byte) (int, error) {
	var req CountTokensRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		return 0, fmt.Errorf("invalid request body: %w", err)
	}

	totalChars := 0

	// 统计消息内容
	for _, msg := range req.Messages {
		if content, ok := msg["content"].(string); ok {
			totalChars += utf8.RuneCountInString(content)
		}
	}

	// 统计系统提示
	if req.System != nil {
		switch sys := req.System.(type) {
		case string:
			totalChars += utf8.RuneCountInString(sys)
		case []interface{}:
			for _, item := range sys {
				if str, ok := item.(string); ok {
					totalChars += utf8.RuneCountInString(str)
				}
			}
		}
	}

	// 工具定义开销 (每个工具约100 tokens)
	if len(req.Tools) > 0 {
		totalChars += len(req.Tools) * 400
	}

	// 应用估算比例
	estimatedTokens := int(float64(totalChars) / h.config.TokenCounting.EstimationRatio)

	// 基础开销
	estimatedTokens += 50

	return estimatedTokens, nil
}
