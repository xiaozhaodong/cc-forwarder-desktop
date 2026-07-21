package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/accountauth"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/events"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/proxy/handlers"
	"cc-forwarder/internal/proxy/response"
	"cc-forwarder/internal/tracking"
)

// Context key for endpoint information
type contextKey string

const EndpointContextKey = contextKey("endpoint")

// Handler handles HTTP proxy requests
type Handler struct {
	endpointManager               *endpoint.Manager
	config                        *config.Config
	usageTracker                  *tracking.UsageTracker
	accountPoolService            AccountPoolService
	codexModelsProvider           CodexModelListProvider
	imageGenerationConfigProvider ImageGenerationConfigProvider
	monitoringMiddleware          *middleware.MonitoringMiddleware
	responseProcessor             *response.Processor
	tokenAnalyzer                 *response.TokenAnalyzer
	forwarder                     *handlers.Forwarder
	refreshTokenManager           *accountauth.OpenAIRefreshTokenManager
	eventBus                      events.EventBus // EventBus事件总线
	accountHTTPInitOnce           sync.Once
	accountHTTPInitErr            error
	accountHTTPTransport          *http.Transport
	accountHTTPClient             *http.Client
	accountSSEHTTPClient          *http.Client
	imageHTTPInitOnce             sync.Once
	imageHTTPInitErr              error
	imageHTTPTransport            *http.Transport
	imageHTTPClient               *http.Client
	imageDirectHTTPClientFactory  func(int, int) (*http.Client, func(), error)
	// 🛡️ 出站隐私过滤（可选注入，nil 时不影响原有链路）
	privacyFilter handlers.PrivacyFilter
}

type CodexModelListProvider interface {
	GetCodexModelListResponse(ctx context.Context) ([]byte, bool, error)
}

// ImageGenerationConfigProvider 提供独立图像生成上游配置。
type ImageGenerationConfigProvider interface {
	GetImageGenerationConfig(ctx context.Context) (ImageGenerationConfig, error)
}

// TokenParserProviderImpl 实现TokenParserProvider接口
type TokenParserProviderImpl struct{}

// NewTokenParser 创建新的TokenParser实例
func (p *TokenParserProviderImpl) NewTokenParser() response.TokenParser {
	return NewTokenParser()
}

// NewTokenParserWithUsageTracker 创建带有UsageTracker的TokenParser实例
func (p *TokenParserProviderImpl) NewTokenParserWithUsageTracker(requestID string, usageTracker *tracking.UsageTracker) response.TokenParser {
	return NewTokenParserWithUsageTracker(requestID, usageTracker)
}

// NewHandler creates a new proxy handler
func NewHandler(endpointManager *endpoint.Manager, cfg *config.Config) *Handler {
	// 创建forwarder
	forwarder := handlers.NewForwarder(cfg, endpointManager)

	h := &Handler{
		endpointManager:     endpointManager,
		config:              cfg,
		responseProcessor:   response.NewProcessor(),
		forwarder:           forwarder,
		refreshTokenManager: accountauth.NewOpenAIRefreshTokenManager(cfg),
	}

	// 初始化 token analyzer
	provider := &TokenParserProviderImpl{}
	h.tokenAnalyzer = response.NewTokenAnalyzer(nil, nil, provider)

	return h
}

// SetMonitoringMiddleware 设置监控中间件用于重试跟踪
func (h *Handler) SetMonitoringMiddleware(mm *middleware.MonitoringMiddleware) {
	h.monitoringMiddleware = mm

	// 同时更新tokenAnalyzer的monitoringMiddleware
	if h.tokenAnalyzer != nil {
		provider := &TokenParserProviderImpl{}
		h.tokenAnalyzer = response.NewTokenAnalyzer(h.usageTracker, mm, provider)
	}
}

// monitoringMiddlewareForAnalyzer 返回用于 TokenAnalyzer 的监控中间件。
// 未设置时返回纯 nil interface，避免 typed-nil 指针通过类型断言后被调用。
func (h *Handler) monitoringMiddlewareForAnalyzer() any {
	if h.monitoringMiddleware == nil {
		return nil
	}
	return h.monitoringMiddleware
}

// SetUsageTracker sets the usage tracker for request tracking
func (h *Handler) SetUsageTracker(ut *tracking.UsageTracker) {
	h.usageTracker = ut

	provider := &TokenParserProviderImpl{}
	h.tokenAnalyzer = response.NewTokenAnalyzer(ut, h.monitoringMiddlewareForAnalyzer(), provider)
}

// SetEventBus 设置EventBus事件总线
func (h *Handler) SetEventBus(eventBus events.EventBus) {
	h.eventBus = eventBus
}

// SetAccountPoolService 设置账号池服务
func (h *Handler) SetAccountPoolService(service AccountPoolService) {
	h.accountPoolService = service
}

// SetPrivacyFilter 注入出站隐私过滤依赖，并传递给 endpoint 常规/流式与账号池链路。
// 必须在 SetUsageTracker 之后保持有效（SetUsageTracker 重建 handler 时会重新注入）。
func (h *Handler) SetPrivacyFilter(filter handlers.PrivacyFilter) {
	h.privacyFilter = filter
	if h.forwarder != nil {
		h.forwarder.SetPrivacyFilter(filter)
	}
}

// SetCodexModelListProvider 设置本地 Codex 模型目录提供器。
func (h *Handler) SetCodexModelListProvider(provider CodexModelListProvider) {
	h.codexModelsProvider = provider
}

// SetImageGenerationConfigProvider 设置独立图像生成配置提供器。
func (h *Handler) SetImageGenerationConfigProvider(provider ImageGenerationConfigProvider) {
	h.imageGenerationConfigProvider = provider
}

// extractModelFromRequestBody 从请求体中提取模型名称
// 仅对已知会携带 model 字段的路径进行解析，避免不必要的JSON解析开销
func (h *Handler) extractModelFromRequestBody(bodyBytes []byte, path string) string {
	// 仅对已知携带 model 的请求尝试解析
	if !strings.Contains(path, "/v1/messages") && !h.isAccountPipelinePath(path) && path != openAIImagesGenerationsPath {
		return ""
	}

	// 避免解析空请求体
	if len(bodyBytes) == 0 {
		return ""
	}

	var requestBody struct {
		Model string `json:"model"`
	}

	if err := json.Unmarshal(bodyBytes, &requestBody); err == nil && requestBody.Model != "" {
		return requestBody.Model
	}

	return ""
}

// ServeHTTP implements the http.Handler interface
// 统一请求分发逻辑 - 整合流式处理、错误恢复和生命周期管理
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 🔢 [count_tokens拦截] 特殊处理count_tokens端点
	if r.URL.Path == "/v1/messages/count_tokens" && h.config.TokenCounting.Enabled {
		ctx := r.Context()
		connID, _ := r.Context().Value("conn_id").(string)

		// 读取请求体
		var bodyBytes []byte
		if r.Body != nil {
			if limit := requestBodyMaxBytes(h.config); limit > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			var err error
			bodyBytes, err = readRequestBodyWithPrealloc(r)
			if err != nil {
				if maxErr, ok := extractMaxBytesError(err); ok {
					http.Error(w, buildRequestBodyTooLargeMessage(maxErr.Limit), http.StatusRequestEntityTooLarge)
					return
				}
				http.Error(w, "Failed to read request body", http.StatusInternalServerError)
				return
			}
			r.Body.Close()
		}

		// 🛡️ count_tokens 走提前拦截分支，也必须挂载请求级隐私状态。
		// 这样同一请求在多个支持端点之间尝试时能复用过滤结果，并传递 requestID。
		if h.privacyFilter != nil {
			privacyState := handlers.NewPrivacyRequestState(connID)
			*r = *r.WithContext(handlers.WithPrivacyRequestState(r.Context(), privacyState))
			ctx = r.Context()
		}

		// 使用CountTokensHandler处理
		countTokensHandler := handlers.NewCountTokensHandler(h.config, h.endpointManager, h.forwarder)
		countTokensHandler.Handle(ctx, w, r, bodyBytes, connID)
		return
	}

	// 创建请求上下文
	ctx := r.Context()

	// 获取连接ID
	connID := ""
	if connIDValue, ok := r.Context().Value("conn_id").(string); ok {
		connID = connIDValue
	}

	if h.isCodexModelsRequest(r) {
		h.handleCodexModelsRequest(ctx, w, r)
		return
	}

	// 创建统一的请求生命周期管理器
	lifecycleManager := NewRequestLifecycleManager(h.usageTracker, h.monitoringMiddleware, connID, h.eventBus)
	// Codex /v1/responses 链路分离，不挂载 endpoint 失败追踪语义
	if !h.isAccountPipelinePath(r.URL.Path) && !isImageAPIPath(r.URL.Path) {
		// 📊 [失败追踪] 设置端点管理器，用于记录成功/失败
		lifecycleManager.SetEndpointManager(h.endpointManager)
	}

	clientIP := r.RemoteAddr
	userAgent := r.Header.Get("User-Agent")
	requestStarted := false
	startTrackedRequest := func(isStreaming bool) {
		if requestStarted {
			return
		}
		lifecycleManager.StartRequest(clientIP, userAgent, r.Method, r.URL.Path, isStreaming)
		requestStarted = true
	}

	// 克隆请求体用于重试
	var bodyBytes []byte
	if r.Body != nil {
		if limit := requestBodyMaxBytes(h.config); limit > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		var err error
		bodyBytes, err = readRequestBodyWithPrealloc(r)
		if err != nil {
			if maxErr, ok := extractMaxBytesError(err); ok {
				startTrackedRequest(false)
				lifecycleManager.HandleError(fmt.Errorf("request body too large: %w", err))
				http.Error(w, buildRequestBodyTooLargeMessage(maxErr.Limit), http.StatusRequestEntityTooLarge)
				return
			}
			startTrackedRequest(false)
			lifecycleManager.HandleError(err)
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}
		r.Body.Close()
	}

	// 🛡️ 请求级隐私过滤状态：同一 requestID + scopeFingerprint 重试时复用扫描结果
	if h.privacyFilter != nil {
		privacyState := handlers.NewPrivacyRequestState(lifecycleManager.GetRequestID())
		*r = *r.WithContext(handlers.WithPrivacyRequestState(r.Context(), privacyState))
	}

	// 异步解析请求体中的模型名称（不阻塞主转发流程）
	go func(body []byte, path string) {
		if modelName := h.extractModelFromRequestBody(body, path); modelName != "" {
			lifecycleManager.SetModel(modelName)
		}
	}(bodyBytes, r.URL.Path)

	// 检测是否为SSE流式请求
	isSSE := h.detectSSERequest(r, bodyBytes)

	// 开始请求跟踪（传递流式标记）
	startTrackedRequest(isSSE)

	if r.URL.Path == openAIImagesGenerationsPath {
		h.handleImageGeneration(ctx, w, r, bodyBytes, lifecycleManager)
		return
	}
	if r.URL.Path == openAIImagesEditsPath {
		h.handleImageEdit(ctx, w, r, bodyBytes, lifecycleManager)
		return
	}

	// Codex /v1/responses 仅由账号池链路处理，不回退到 endpoint
	if h.shouldUseAccountPipeline(r.URL.Path) {
		h.handleAccountPipeline(ctx, w, r, bodyBytes, lifecycleManager)
		return
	}
	if h.isAccountPipelinePath(r.URL.Path) {
		h.handleUnavailableAccountPipeline(w, lifecycleManager)
		return
	}

	// 统一端点转发管线（v7：streaming/regular 合并，仅 P2 处理分叉）
	h.handleEndpointPipeline(ctx, w, r, bodyBytes, lifecycleManager, isSSE)
}

func (h *Handler) isCodexModelsRequest(r *http.Request) bool {
	return r != nil && r.Method == http.MethodGet && r.URL.Path == "/v1/models"
}

func (h *Handler) handleCodexModelsRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if h.tryWriteLocalCodexModels(ctx, w, r) {
		return
	}

	// /v1/models 是模型目录查询，不属于 token 消耗请求；穿透上游时也不写请求追踪表。
	if h.handleCodexModelsAccountPassthrough(ctx, w, r) {
		return
	}
	h.handleCodexModelsPassthrough(ctx, w, r)
}

func (h *Handler) handleCodexModelsAccountPassthrough(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.config == nil || !h.config.AccountPool.Enabled {
		return false
	}
	if h.accountPoolService == nil {
		writeCodexModelsError(w, http.StatusServiceUnavailable, "account_pool_unavailable", "account pool service is not initialized for Codex /v1/models")
		return true
	}

	preview, err := h.accountPoolService.PreviewSchedulableAccounts(ctx, r.URL.Path)
	if err != nil {
		writeCodexModelsError(w, http.StatusServiceUnavailable, "account_pool_unavailable", "failed to load schedulable account for Codex /v1/models")
		return true
	}
	accounts := preview.Accounts
	if len(accounts) == 0 {
		writeCodexModelsError(w, http.StatusServiceUnavailable, "account_pool_unavailable", "no schedulable account for Codex /v1/models")
		return true
	}

	var lastErr error
	for _, acc := range accounts {
		if acc == nil {
			continue
		}

		attemptStartedAt := time.Now()
		resp, upstreamCancel, _, err := h.forwardRequestToAccount(ctx, r, nil, acc, false, nil)
		releaseUpstream := func() {
			if upstreamCancel != nil {
				upstreamCancel()
				upstreamCancel = nil
			}
		}
		if err != nil {
			releaseUpstream()
			lastErr = err
			if !h.shouldFailOverAfterSoftFailure(ctx, acc.ID, err.Error(), accountSoftFailureCategoryServerError, 0) {
				break
			}
			continue
		}
		if resp == nil {
			releaseUpstream()
			lastErr = fmt.Errorf("empty response from account %d", acc.ID)
			if !h.shouldFailOverAfterSoftFailure(ctx, acc.ID, lastErr.Error(), accountSoftFailureCategoryServerError, 0) {
				break
			}
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			detail := readAndCloseResponseBody(resp, 1024)
			releaseUpstream()
			if detail == "" {
				detail = fmt.Sprintf("upstream returned %d", resp.StatusCode)
			}
			lastErr = fmt.Errorf("auth failed: %s", detail)
			_ = h.accountPoolService.MarkAccountAuthFailed(ctx, acc.ID, detail)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := h.accountRateLimitRetryAfter(resp)
			detail := readAndCloseResponseBody(resp, 1024)
			releaseUpstream()
			if detail == "" {
				detail = fmt.Sprintf("upstream returned %d", resp.StatusCode)
			}
			lastErr = fmt.Errorf("upstream retryable error: %s", detail)
			if usageLimit, ok := parseAccountUsageLimitWindow(detail, time.Now()); ok {
				_ = h.accountPoolService.MarkAccountUsageLimitExceeded(ctx, acc.ID, detail, usageLimit.planType, usageLimit.resetAt)
				continue
			}
			if !h.shouldFailOverAfterSoftFailure(ctx, acc.ID, detail, accountSoftFailureCategoryRateLimit, retryAfter) {
				break
			}
			continue
		}

		if resp.StatusCode >= 500 {
			detail := readAndCloseResponseBody(resp, 1024)
			releaseUpstream()
			if detail == "" {
				detail = fmt.Sprintf("upstream returned %d", resp.StatusCode)
			}
			lastErr = fmt.Errorf("upstream retryable error: %s", detail)
			if !h.shouldFailOverAfterSoftFailure(ctx, acc.ID, detail, accountSoftFailureCategoryServerError, 0) {
				break
			}
			continue
		}

		if resp.StatusCode >= 400 {
			if err := h.writeRawResponse(w, resp); err != nil {
				releaseUpstream()
				writeCodexModelsError(w, http.StatusBadGateway, "account_pool_response_error", "failed to read upstream model list from account pool")
				return true
			}
			releaseUpstream()
			return true
		}

		if err := h.writeRawResponse(w, resp); err != nil {
			releaseUpstream()
			writeCodexModelsError(w, http.StatusBadGateway, "account_pool_response_error", "failed to read upstream model list from account pool")
			return true
		}
		releaseUpstream()
		finalizeCtx, finalizeCancel := h.newAccountSuccessContext()
		_, _ = h.accountPoolService.MarkAccountSuccessIfNoNewerFailure(finalizeCtx, acc.ID, attemptStartedAt)
		if accountauth.IsChatGPTOAuthProvider(acc.ProviderType) {
			h.accountPoolService.TryEnqueueQuotaRefresh(acc.ID)
		}
		finalizeCancel()
		return true
	}

	reason := "all account pool candidates failed for Codex /v1/models"
	if lastErr != nil {
		reason = lastErr.Error()
	}
	writeCodexModelsError(w, http.StatusBadGateway, "account_pool_upstream_error", reason)
	return true
}

func (h *Handler) handleCodexModelsPassthrough(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	endpoints := h.getCodexModelEndpoints()
	if len(endpoints) == 0 {
		writeCodexModelsError(w, http.StatusServiceUnavailable, "codex_upstream_unavailable", "no Codex upstream endpoint available for /v1/models")
		return
	}

	ep := endpoints[0]
	resp, err := h.forwarder.ForwardRequestToEndpoint(ctx, r, nil, ep)
	if err != nil {
		h.endpointManager.RecordFailure(ep.Config.Name)
		writeCodexModelsError(w, http.StatusBadGateway, "codex_upstream_error", fmt.Sprintf("failed to fetch upstream model list: %v", err))
		return
	}
	defer resp.Body.Close()

	responseBytes, err := h.responseProcessor.ProcessResponseBody(resp)
	if err != nil {
		h.endpointManager.RecordFailure(ep.Config.Name)
		writeCodexModelsError(w, http.StatusBadGateway, "codex_upstream_response_error", "failed to read upstream model list")
		return
	}

	h.responseProcessor.CopyResponseHeaders(resp, w)
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(responseBytes); err != nil {
		h.endpointManager.RecordFailure(ep.Config.Name)
		return
	}
	h.endpointManager.RecordSuccess(ep.Config.Name)
}

func (h *Handler) getCodexModelEndpoints() []*endpoint.Endpoint {
	if h == nil || h.endpointManager == nil {
		return nil
	}

	all := h.endpointManager.GetAllEndpoints()
	codexEndpoints := make([]*endpoint.Endpoint, 0, len(all))
	for _, ep := range all {
		if isEndpointEnabled(ep) && h.endpointManager.IsEndpointRoutable(ep) && isCodexModelEndpoint(ep) {
			codexEndpoints = append(codexEndpoints, ep)
		}
	}
	sort.SliceStable(codexEndpoints, func(i, j int) bool {
		leftScore := codexModelEndpointScore(codexEndpoints[i])
		rightScore := codexModelEndpointScore(codexEndpoints[j])
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return codexEndpoints[i].Config.Priority < codexEndpoints[j].Config.Priority
	})
	return codexEndpoints
}

func isEndpointEnabled(ep *endpoint.Endpoint) bool {
	if ep == nil || ep.Config.Enabled == nil {
		return true
	}
	return *ep.Config.Enabled
}

func isCodexModelEndpoint(ep *endpoint.Endpoint) bool {
	return codexModelEndpointScore(ep) > 0
}

func codexModelEndpointScore(ep *endpoint.Endpoint) int {
	if ep == nil {
		return 0
	}

	channel := strings.ToLower(strings.TrimSpace(ep.Config.Channel))
	name := strings.ToLower(strings.TrimSpace(ep.Config.Name))
	rawURL := strings.ToLower(strings.TrimSpace(ep.Config.URL))
	haystack := strings.Join([]string{channel, name, rawURL}, " ")

	strongCodexHints := []string{
		"codex",
		"coderelay",
		"code-relay",
		"responses",
	}
	for _, hint := range strongCodexHints {
		if strings.Contains(haystack, hint) {
			return 100
		}
	}

	if isClaudeCodeEndpointHint(channel, name, rawURL) {
		return 0
	}

	openAIHints := []string{
		"api.openai.com",
		"openai",
		"chatgpt",
	}
	for _, hint := range openAIHints {
		if strings.Contains(haystack, hint) {
			return 50
		}
	}

	return 0
}

func isClaudeCodeEndpointHint(channel, name, rawURL string) bool {
	if isCCLabel(channel) || isCCLabel(name) {
		return true
	}

	haystack := strings.Join([]string{channel, name, rawURL}, " ")
	claudeHints := []string{
		"claude",
		"anthropic",
		"sonnet",
		"opus",
		"haiku",
		"/v1/messages",
	}
	for _, hint := range claudeHints {
		if strings.Contains(haystack, hint) {
			return true
		}
	}
	return false
}

func isCCLabel(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	return value == "cc" ||
		strings.HasPrefix(value, "cc-") ||
		strings.HasSuffix(value, "-cc") ||
		strings.Contains(value, "-cc-") ||
		strings.Contains(value, "_cc_")
}

func writeCodexModelsError(w http.ResponseWriter, statusCode int, errType, message string) {
	writeAccountPipelineError(w, statusCode, errType, message)
}

func (h *Handler) tryWriteLocalCodexModels(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if !h.isCodexModelsRequest(r) || h.codexModelsProvider == nil {
		return false
	}

	responseBytes, handled, err := h.codexModelsProvider.GetCodexModelListResponse(ctx)
	if !handled {
		return false
	}

	if err != nil {
		writeCodexModelsError(w, http.StatusInternalServerError, "codex_models_error", "failed to load local Codex model catalog")
		return true
	}

	if len(responseBytes) == 0 {
		responseBytes = []byte(`{"object":"list","data":[]}`)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(responseBytes); err != nil {
		return true
	}

	return true
}

// shouldUseAccountPipeline 判断当前请求是否应走账号池链路
func (h *Handler) shouldUseAccountPipeline(path string) bool {
	return h.isAccountPipelinePath(path) &&
		h.config != nil &&
		h.config.AccountPool.Enabled &&
		h.accountPoolService != nil
}

func (h *Handler) isAccountPipelinePath(path string) bool {
	return path == "/v1/responses" || path == "/v1/responses/compact"
}

func (h *Handler) handleUnavailableAccountPipeline(w http.ResponseWriter, lifecycleManager *RequestLifecycleManager) {
	errType := "account_pool_disabled"
	message := "account pool is disabled for Codex /v1/responses and /v1/responses/compact"
	failureKey := "account_pool_disabled"

	if h.config != nil && h.config.AccountPool.Enabled {
		errType = "account_pool_unavailable"
		failureKey = "account_pool_not_ready"
		message = "account pool service is not initialized for Codex /v1/responses and /v1/responses/compact"
	}

	lifecycleManager.SetUpstream("account", "account-pool", "account-pool", 0)
	lifecycleManager.FailRequest(failureKey, message, http.StatusServiceUnavailable)
	writeAccountPipelineError(w, http.StatusServiceUnavailable, errType, message)
}

// detectSSERequest 统一SSE请求检测逻辑
func (h *Handler) detectSSERequest(r *http.Request, bodyBytes []byte) bool {
	// 检查多种SSE请求模式:
	acceptHeader := r.Header.Get("Accept")
	cacheControlHeader := r.Header.Get("Cache-Control")
	streamHeader := r.Header.Get("stream")

	// 1. Accept头包含text/event-stream
	if strings.Contains(acceptHeader, "text/event-stream") {
		return true
	}

	// 2. stream头设置为true（显式流式标记）
	if strings.EqualFold(strings.TrimSpace(streamHeader), "true") {
		return true
	}

	// 3. 请求体包含stream参数为true（Anthropic/OpenAI常见格式）
	bodyStr := string(bodyBytes)
	if strings.Contains(bodyStr, `"stream":true`) || strings.Contains(bodyStr, `"stream": true`) {
		return true
	}
	if r.URL.Path == openAIImagesEditsPath && imageEditMultipartStreamEnabled(bodyBytes, r.Header.Get("Content-Type")) {
		return true
	}

	// 4. Cache-Control: no-cache 仅作为辅助信号，避免单独误判普通API请求
	if strings.Contains(cacheControlHeader, "no-cache") &&
		(strings.Contains(acceptHeader, "text/event-stream") || strings.EqualFold(strings.TrimSpace(streamHeader), "true")) {
		return true
	}

	return false
}

// UpdateConfig updates the handler configuration
func (h *Handler) UpdateConfig(cfg *config.Config) {
	h.config = cfg
}

// noOpFlusher 是一个不执行实际flush操作的flusher实现
// 用于测试和不支持Flusher的环境
type noOpFlusher struct{}

func (f *noOpFlusher) Flush() {
	// 不执行任何操作，避免panic但保持流式处理逻辑
}
