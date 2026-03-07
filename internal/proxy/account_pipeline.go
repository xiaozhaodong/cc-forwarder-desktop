package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cc-forwarder/internal/accountauth"
	"cc-forwarder/internal/proxy/handlers"
	"cc-forwarder/internal/store"
	"cc-forwarder/internal/transport"
)

const defaultAccountRetryCooldown = 30 * time.Second

const (
	chatGPTCodexResponsesURL = "https://chatgpt.com/backend-api/codex/responses"
	openAIBetaResponsesValue = "responses=experimental"
	defaultOAuthOriginator   = "codex_cli_rs"
)

// handleAccountPipeline 账号池链路（阶段2）
func (h *Handler) handleAccountPipeline(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager *RequestLifecycleManager) {
	if h.accountPoolService == nil {
		lifecycleManager.SetUpstream("account", "account-pool", "account-pool", 0)
		lifecycleManager.FailRequest("account_pool_not_ready", "account pool service is not initialized", http.StatusServiceUnavailable)
		writeAccountPipelineError(w, http.StatusServiceUnavailable, "account_pool_unavailable", "account pool service is not initialized")
		return
	}

	accounts, err := h.accountPoolService.ListSchedulableAccounts(ctx)
	if err != nil {
		lifecycleManager.SetUpstream("account", "account-pool", "account-pool", 0)
		lifecycleManager.FailRequest("account_pool_load_failed", err.Error(), http.StatusServiceUnavailable)
		writeAccountPipelineError(w, http.StatusServiceUnavailable, "account_pool_unavailable", "failed to load schedulable accounts")
		return
	}
	if len(accounts) == 0 {
		lifecycleManager.SetUpstream("account", "account-pool", "account-pool", 0)
		lifecycleManager.FailRequest("account_pool_empty", "no schedulable account", http.StatusServiceUnavailable)
		writeAccountPipelineError(w, http.StatusServiceUnavailable, "account_pool_unavailable", "no schedulable account")
		return
	}

	isSSE := h.detectSSERequest(r, bodyBytes)
	var lastErr error

	for idx, acc := range accounts {
		accountName := acc.AccountName
		if strings.TrimSpace(accountName) == "" {
			accountName = fmt.Sprintf("account-%d", acc.ID)
		}

		lifecycleManager.SetUpstream("account", "", accountName, acc.ID)
		lifecycleManager.SetEndpoint(accountName, "", "account-pool")
		lifecycleManager.UpdateStatus("forwarding", idx, 0)

		resp, forwardErr := h.forwardRequestToAccount(ctx, r, bodyBytes, acc, isSSE)
		if forwardErr != nil {
			lastErr = forwardErr
			_ = h.accountPoolService.MarkAccountTransientFailure(ctx, acc.ID, forwardErr.Error(), defaultAccountRetryCooldown)
			continue
		}

		if resp == nil {
			lastErr = fmt.Errorf("empty response from account %d", acc.ID)
			_ = h.accountPoolService.MarkAccountTransientFailure(ctx, acc.ID, lastErr.Error(), defaultAccountRetryCooldown)
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			detail := readAndCloseResponseBody(resp, 1024)
			if detail == "" {
				detail = fmt.Sprintf("upstream returned %d", resp.StatusCode)
			}
			lastErr = fmt.Errorf("auth failed: %s", detail)
			_ = h.accountPoolService.MarkAccountAuthFailed(ctx, acc.ID, detail)
			continue
		}

		if isNoAvailableProviders503Response(resp) {
			detail := readAndRestoreResponseBody(resp, 1024)
			if detail == "" {
				detail = "no_available_providers"
			}
			if writeErr := h.writeRawResponse(w, resp); writeErr != nil {
				lifecycleManager.FailRequest("account_pipeline_response_write_error", writeErr.Error(), resp.StatusCode)
				return
			}
			lifecycleManager.FailRequest("upstream_no_available_providers", detail, resp.StatusCode)
			return
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			detail := readAndCloseResponseBody(resp, 1024)
			if detail == "" {
				detail = fmt.Sprintf("upstream returned %d", resp.StatusCode)
			}
			lastErr = fmt.Errorf("upstream retryable error: %s", detail)
			_ = h.accountPoolService.MarkAccountTransientFailure(ctx, acc.ID, detail, defaultAccountRetryCooldown)
			continue
		}

		// 其余 4xx 视为客户端请求问题，直接透传，不进行账号切换
		if resp.StatusCode >= 400 {
			if writeErr := h.writeRawResponse(w, resp); writeErr != nil {
				lifecycleManager.FailRequest("account_pipeline_response_write_error", writeErr.Error(), resp.StatusCode)
				_ = h.accountPoolService.MarkAccountTransientFailure(ctx, acc.ID, writeErr.Error(), defaultAccountRetryCooldown)
				return
			}
			lifecycleManager.FailRequest("upstream_client_error", fmt.Sprintf("upstream returned %d", resp.StatusCode), resp.StatusCode)
			return
		}

		lifecycleManager.UpdateStatus("processing", idx, resp.StatusCode)
		var processErr error
		if isSSE {
			processErr = h.processAccountStreamingResponse(ctx, w, resp, lifecycleManager, accountName)
		} else {
			processErr = h.processAccountRegularResponse(w, resp, lifecycleManager, accountName, r)
		}
		if processErr != nil {
			if isCancelledAccountStreamError(processErr) {
				return
			}
			lastErr = processErr
			_ = h.accountPoolService.MarkAccountTransientFailure(ctx, acc.ID, processErr.Error(), 10*time.Second)
			// 流式响应一旦开始输出，无法切换到下一个账号，直接终止
			if isSSE {
				return
			}
			continue
		}

		_ = h.accountPoolService.MarkAccountSuccess(ctx, acc.ID)
		return
	}

	reason := "all accounts failed"
	if lastErr != nil {
		reason = lastErr.Error()
	}
	lifecycleManager.SetUpstream("account", "account-pool", "account-pool", 0)
	lifecycleManager.FailRequest("account_pool_exhausted", reason, http.StatusServiceUnavailable)
	writeAccountPipelineError(w, http.StatusServiceUnavailable, "account_pool_unavailable", reason)
}

func (h *Handler) forwardRequestToAccount(ctx context.Context, r *http.Request, bodyBytes []byte, acc *store.UpstreamAccountRecord, isSSE bool) (*http.Response, error) {
	targetURL, err := resolveAccountTargetURL(acc, r.URL.Path, r.URL.RawQuery)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	copyRequestHeadersForAccount(r, req)
	if err := accountauth.ApplyAccountAuth(ctx, req, acc.ProviderType, acc.CredentialRaw, h.refreshTokenManager); err != nil {
		return nil, fmt.Errorf("failed to apply account auth: %w", err)
	}
	if accountauth.IsChatGPTOAuthProvider(acc.ProviderType) {
		if accountauth.ExtractChatGPTAccountID(acc.CredentialRaw) == "" {
			return nil, fmt.Errorf("chatgpt_account_id is missing from credential")
		}
		applyOpenAIChatGPTOAuthHeaders(req, acc.CredentialRaw)
	}

	client, err := h.getAccountHTTPClient(isSSE)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

func isNoAvailableProviders503Response(resp *http.Response) bool {
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		return false
	}
	bodyText := readAndRestoreResponseBody(resp, 1024)
	lower := strings.ToLower(strings.TrimSpace(bodyText))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "no_available_providers") ||
		strings.Contains(lower, "no available providers")
}

func readAndRestoreResponseBody(resp *http.Response, limit int64) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return strings.TrimSpace(string(body))
}

func (h *Handler) getAccountHTTPClient(isSSE bool) (*http.Client, error) {
	h.accountHTTPInitOnce.Do(func() {
		if h.config == nil {
			h.accountHTTPInitErr = fmt.Errorf("handler config is nil")
			return
		}

		httpTransport, err := transport.CreateTransport(h.config)
		if err != nil {
			h.accountHTTPInitErr = fmt.Errorf("failed to create shared account transport: %w", err)
			return
		}

		httpTransport.DisableKeepAlives = false
		httpTransport.MaxIdleConns = 100
		httpTransport.MaxIdleConnsPerHost = 10
		httpTransport.IdleConnTimeout = 90 * time.Second
		httpTransport.TLSHandshakeTimeout = 10 * time.Second
		httpTransport.ExpectContinueTimeout = 1 * time.Second
		httpTransport.DisableCompression = true
		httpTransport.WriteBufferSize = 4096
		httpTransport.ReadBufferSize = 4096
		responseHeaderTimeout := h.config.Streaming.ResponseHeaderTimeout
		if responseHeaderTimeout == 0 {
			responseHeaderTimeout = 60 * time.Second
		}
		httpTransport.ResponseHeaderTimeout = responseHeaderTimeout

		timeout := h.config.GlobalTimeout
		if timeout <= 0 {
			timeout = 300 * time.Second
		}

		h.accountHTTPTransport = httpTransport
		h.accountHTTPClient = &http.Client{
			Timeout:   timeout,
			Transport: httpTransport,
		}
		h.accountSSEHTTPClient = &http.Client{
			Timeout:   0,
			Transport: httpTransport,
		}
	})

	if h.accountHTTPInitErr != nil {
		return nil, h.accountHTTPInitErr
	}
	if isSSE {
		return h.accountSSEHTTPClient, nil
	}
	return h.accountHTTPClient, nil
}

func resolveAccountTargetURL(acc *store.UpstreamAccountRecord, path, rawQuery string) (string, error) {
	if acc != nil && accountauth.IsChatGPTOAuthProvider(acc.ProviderType) {
		upstreamURL, err := url.Parse(chatGPTCodexResponsesURL)
		if err != nil {
			return "", err
		}
		upstreamURL.RawQuery = rawQuery
		return upstreamURL.String(), nil
	}
	baseURL := ""
	if acc != nil {
		baseURL = acc.BaseURL
	}
	return joinBaseURLAndPath(baseURL, path, rawQuery)
}

func (h *Handler) processAccountRegularResponse(w http.ResponseWriter, resp *http.Response, lifecycleManager *RequestLifecycleManager, endpointName string, r *http.Request) error {
	defer resp.Body.Close()

	responseBytes, err := h.responseProcessor.ProcessResponseBody(resp)
	if err != nil {
		lifecycleManager.FailRequest("account_response_read_error", err.Error(), http.StatusBadGateway)
		return fmt.Errorf("failed to read upstream response: %w", err)
	}

	if len(responseBytes) == 0 && r.URL.Path == "/v1/responses" {
		lifecycleManager.FailRequest("empty_response_body", "Upstream returned 2xx with empty body", http.StatusBadGateway)
		return fmt.Errorf("empty response body")
	}

	h.responseProcessor.CopyResponseHeaders(resp, w)
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(responseBytes); err != nil {
		lifecycleManager.FailRequest("account_response_write_error", err.Error(), resp.StatusCode)
		return fmt.Errorf("failed to write response: %w", err)
	}

	tokenUsage, modelName := h.tokenAnalyzer.AnalyzeResponseForTokensUnified(responseBytes, lifecycleManager.GetRequestID(), endpointName)
	if tokenUsage != nil {
		if modelName != "" && modelName != "unknown" {
			lifecycleManager.SetModelWithComparison(modelName, "账号常规响应解析")
		}
		lifecycleManager.CompleteRequest(tokenUsage)
		return nil
	}

	lifecycleManager.HandleNonTokenResponse(string(responseBytes))
	return nil
}

func (h *Handler) processAccountStreamingResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, lifecycleManager *RequestLifecycleManager, endpointName string) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		resp.Body.Close()
		lifecycleManager.FailRequest("stream_flusher_unsupported", "response writer does not support flusher", http.StatusInternalServerError)
		return fmt.Errorf("response writer does not support flusher")
	}

	for key, values := range resp.Header {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	flusher.Flush()

	tokenParser := NewTokenParserWithUsageTracker(lifecycleManager.GetRequestID(), h.usageTracker)
	processor := NewStreamProcessor(tokenParser, h.usageTracker, w, flusher, lifecycleManager.GetRequestID(), endpointName)

	tokenUsage, modelName, err := processor.ProcessStreamWithRetry(ctx, resp)
	if modelName != "" && modelName != "unknown" {
		lifecycleManager.SetModelWithComparison(modelName, "账号流式响应解析")
	}
	if err != nil {
		if streamErr, ok := err.(handlers.StreamIncompleteErrorInterface); ok {
			parsedModelName := streamErr.GetModelName()
			failureReason := streamErr.GetFailureReason()

			if parsedModelName != "unknown" && parsedModelName != "" && parsedModelName != "default" {
				lifecycleManager.SetModelWithComparison(parsedModelName, "account_stream_incomplete")
			} else if modelName != "unknown" && modelName != "" && modelName != "default" {
				lifecycleManager.SetModelWithComparison(modelName, "account_stream_processor")
			}

			lifecycleManager.CompleteRequestWithQuality(tokenUsage, failureReason)
			return nil
		}

		status, parsedModelName := parseAccountStreamStatusError(err)
		if parsedModelName != "unknown" && parsedModelName != "" && parsedModelName != "default" {
			lifecycleManager.SetModelWithComparison(parsedModelName, "account_stream_status")
		} else if modelName != "unknown" && modelName != "" && modelName != "default" {
			lifecycleManager.SetModelWithComparison(modelName, "account_stream_processor")
		}

		if status == "cancelled" {
			lifecycleManager.CancelRequest("account stream processing cancelled", tokenUsage)
			return context.Canceled
		}

		if tokenUsage != nil {
			lifecycleManager.RecordTokensForFailedRequest(tokenUsage, "account_stream_error")
		}
		lifecycleManager.FailRequest("account_stream_error", err.Error(), http.StatusBadGateway)
		return err
	}
	lifecycleManager.CompleteRequest(tokenUsage)
	return nil
}

func (h *Handler) writeRawResponse(w http.ResponseWriter, resp *http.Response) error {
	defer resp.Body.Close()
	body, err := h.responseProcessor.ProcessResponseBody(resp)
	if err != nil {
		return err
	}
	h.responseProcessor.CopyResponseHeaders(resp, w)
	w.WriteHeader(resp.StatusCode)
	_, err = w.Write(body)
	return err
}

func copyRequestHeadersForAccount(src *http.Request, dst *http.Request) {
	skipHeaders := map[string]bool{
		"host":          true,
		"authorization": true,
		"x-api-key":     true,
		"cookie":        true,
	}
	for key, values := range src.Header {
		if skipHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			dst.Header.Add(key, value)
		}
	}
	if u, err := url.Parse(dst.URL.String()); err == nil {
		dst.Header.Set("Host", u.Host)
		dst.Host = u.Host
	}
}

func applyOpenAIChatGPTOAuthHeaders(req *http.Request, credentialRaw string) {
	if req == nil {
		return
	}
	req.Host = "chatgpt.com"
	req.Header.Set("Host", "chatgpt.com")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", openAIBetaResponsesValue)
	req.Header.Set("originator", defaultOAuthOriginator)
	if accountID := accountauth.ExtractChatGPTAccountID(credentialRaw); accountID != "" {
		req.Header.Set("chatgpt-account-id", accountID)
	}
}

func joinBaseURLAndPath(baseURL, path, rawQuery string) (string, error) {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://api.openai.com"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}
	u.RawQuery = rawQuery
	return u.String(), nil
}

func readAndCloseResponseBody(resp *http.Response, limit int64) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
	return strings.TrimSpace(string(raw))
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailers", "transfer-encoding", "upgrade", "content-length":
		return true
	default:
		return false
	}
}

func writeAccountPipelineError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"type":    errType,
			"message": message,
		},
	})
}

func parseAccountStreamStatusError(err error) (string, string) {
	if err == nil {
		return "error", "unknown"
	}

	status := "error"
	modelName := "unknown"
	errText := err.Error()
	if strings.HasPrefix(errText, "stream_status:") {
		parts := strings.SplitN(errText, ":", 5)
		if len(parts) >= 4 {
			status = parts[1]
			if parts[2] == "model" && parts[3] != "" {
				modelName = parts[3]
			}
		}
	} else if strings.Contains(strings.ToLower(errText), "context canceled") {
		status = "cancelled"
	}

	return status, modelName
}

func isCancelledAccountStreamError(err error) bool {
	status, _ := parseAccountStreamStatusError(err)
	return status == "cancelled"
}
