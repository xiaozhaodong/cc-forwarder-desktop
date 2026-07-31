package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/modelrewrite"
	"cc-forwarder/internal/transport"
)

const (
	anyrouteChannel         = "anyroute"
	coderelayChannel        = "coderelay"
	anthropicBetaHeader     = "anthropic-beta"
	anyrouteContextBetaFlag = "context-1m-2025-08-07"
	mywechatEndpointName    = "mywechat"
	cacheControlKey         = "cache_control"
	cacheControlScopeKey    = "scope"
	contextManagementKey    = "context_management"
)

// Forwarder 负责HTTP请求转发和头部处理
type Forwarder struct {
	config          *config.Config
	endpointManager *endpoint.Manager
	transportOnce   sync.Once
	transportErr    error
	httpTransport   *http.Transport
	// 🛡️ 出站隐私过滤（可选注入，nil 时不影响原有链路）
	privacyFilter PrivacyFilter
}

// SetPrivacyFilter 注入出站隐私过滤依赖
func (f *Forwarder) SetPrivacyFilter(filter PrivacyFilter) {
	f.privacyFilter = filter
}

// NewForwarder 创建新的Forwarder实例
func NewForwarder(cfg *config.Config, endpointManager *endpoint.Manager) *Forwarder {
	return &Forwarder{
		config:          cfg,
		endpointManager: endpointManager,
	}
}

// ForwardRequestToEndpoint 转发请求到指定端点
func (f *Forwarder) ForwardRequestToEndpoint(ctx context.Context, r *http.Request, bodyBytes []byte, ep *endpoint.Endpoint) (*http.Response, error) {
	bodyBytes = prepareBodyForEndpoint(r.URL.Path, bodyBytes, ep)

	// 🛡️ 出站隐私过滤（PolicyError 由调用方短路，不进入 retry/failover）
	bodyBytes, err := ApplyPrivacyFilterForEndpoint(f.privacyFilter, r, bodyBytes, ep)
	if err != nil {
		return nil, err
	}

	// 创建目标URL
	targetURL := ep.Config.URL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 复制和修改头部
	f.CopyHeaders(r, req, ep)

	// 创建HTTP传输
	httpTransport, err := f.buildStreamingTransport()
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	client := &http.Client{
		Timeout:   0, // 流式请求无超时
		Transport: httpTransport,
	}

	// 执行请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// 检查响应状态
	if resp.StatusCode >= 400 {
		detail := BuildUpstreamErrorDetail(resp, ep.Config.Name, defaultUpstreamErrorPreviewLimit)
		resp.Body.Close()
		if detail == "" {
			return nil, fmt.Errorf("endpoint returned error: %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("endpoint returned error: %d %s", resp.StatusCode, detail)
	}

	return resp, nil
}

// ForwardStreamingRequestToEndpoint 为流式请求创建独立的 upstream context。
// 这样下游客户端断开后，调用方仍可短时继续 drain 上游尾部，以补齐终止事件和 usage。
func (f *Forwarder) ForwardStreamingRequestToEndpoint(r *http.Request, bodyBytes []byte, ep *endpoint.Endpoint, onWroteRequest ...func(time.Time)) (*http.Response, context.CancelFunc, error) {
	upstreamCtx, release := context.WithCancel(context.Background())
	if len(onWroteRequest) > 0 && onWroteRequest[0] != nil {
		trace := &httptrace.ClientTrace{
			WroteRequest: func(httptrace.WroteRequestInfo) {
				onWroteRequest[0](time.Now())
			},
		}
		upstreamCtx = httptrace.WithClientTrace(upstreamCtx, trace)
	}
	bodyBytes = prepareBodyForEndpoint(r.URL.Path, bodyBytes, ep)

	// 🛡️ 出站隐私过滤（PolicyError 由调用方短路，不进入 retry/failover）
	bodyBytes, err := ApplyPrivacyFilterForEndpoint(f.privacyFilter, r, bodyBytes, ep)
	if err != nil {
		release()
		return nil, nil, err
	}

	// 创建目标URL
	targetURL := ep.Config.URL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(upstreamCtx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		release()
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 复制和修改头部
	f.CopyHeaders(r, req, ep)

	httpTransport, err := f.buildStreamingTransport()
	if err != nil {
		release()
		return nil, nil, fmt.Errorf("failed to create transport: %w", err)
	}

	client := &http.Client{
		Timeout:   0,
		Transport: httpTransport,
	}

	resp, err := client.Do(req)
	if err != nil {
		release()
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		detail := BuildUpstreamErrorDetail(resp, ep.Config.Name, defaultUpstreamErrorPreviewLimit)
		resp.Body.Close()
		release()
		if detail == "" {
			return nil, nil, fmt.Errorf("endpoint returned error: %d", resp.StatusCode)
		}
		return nil, nil, fmt.Errorf("endpoint returned error: %d %s", resp.StatusCode, detail)
	}

	return resp, release, nil
}

func prepareBodyForEndpoint(path string, bodyBytes []byte, ep *endpoint.Endpoint) []byte {
	bodyBytes = rewriteEndpointModel(path, bodyBytes, ep)
	if isCoderelayChannel(ep) {
		bodyBytes = stripCoderelayUnsupportedFields(bodyBytes, ep)
	}
	return bodyBytes
}

func rewriteEndpointModel(path string, bodyBytes []byte, ep *endpoint.Endpoint) []byte {
	if ep == nil || strings.TrimSpace(ep.Config.ModelRewriteRules) == "" || !isClaudeMessagesPath(path) || len(bytes.TrimSpace(bodyBytes)) == 0 {
		return bodyBytes
	}

	var payload map[string]any
	if !decodeSingleJSON(bodyBytes, &payload) {
		return bodyBytes
	}
	model, ok := payload["model"].(string)
	if !ok {
		return bodyBytes
	}
	target, rewritten, err := modelrewrite.Rewrite(ep.Config.ModelRewriteRules, path, model)
	if err != nil {
		slog.Warn("⚠️ [端点模型改写] 忽略无效规则", "endpoint", ep.Config.Name, "error", err)
		return bodyBytes
	}
	if !rewritten {
		return bodyBytes
	}

	payload["model"] = target
	rewrittenBody, err := json.Marshal(payload)
	if err != nil {
		return bodyBytes
	}
	slog.Debug("🔀 [端点模型改写] 已替换上游模型", "endpoint", ep.Config.Name, "from", model, "to", target, "path", path)
	return rewrittenBody
}

func isClaudeMessagesPath(path string) bool {
	return path == "/v1/messages" || path == "/v1/messages/count_tokens"
}

func isCoderelayChannel(ep *endpoint.Endpoint) bool {
	if ep == nil {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(ep.Config.Channel), coderelayChannel)
}

func stripCoderelayUnsupportedFields(bodyBytes []byte, ep *endpoint.Endpoint) []byte {
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		return bodyBytes
	}

	var payload any
	if !decodeSingleJSON(bodyBytes, &payload) {
		return bodyBytes
	}

	changed := deleteCacheControlScope(payload)
	if isMywechatEndpoint(ep) && deleteTopLevelKey(payload, contextManagementKey) {
		changed = true
	}
	if !changed {
		return bodyBytes
	}

	sanitizedBodyBytes, err := json.Marshal(payload)
	if err != nil {
		return bodyBytes
	}
	return sanitizedBodyBytes
}

func decodeSingleJSON(bodyBytes []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return false
	}
	var trailing any
	return decoder.Decode(&trailing) == io.EOF
}

func isMywechatEndpoint(ep *endpoint.Endpoint) bool {
	if ep == nil {
		return false
	}

	return strings.EqualFold(strings.TrimSpace(ep.Config.Name), mywechatEndpointName)
}

func deleteTopLevelKey(value any, key string) bool {
	node, ok := value.(map[string]any)
	if !ok {
		return false
	}

	if _, exists := node[key]; !exists {
		return false
	}
	delete(node, key)
	return true
}

func deleteCacheControlScope(value any) bool {
	return deleteCacheControlScopeValue(value, false)
}

func deleteCacheControlScopeValue(value any, insideCacheControl bool) bool {
	changed := false

	switch node := value.(type) {
	case map[string]any:
		if insideCacheControl {
			if _, exists := node[cacheControlScopeKey]; exists {
				delete(node, cacheControlScopeKey)
				changed = true
			}
		}
		for key, child := range node {
			if deleteCacheControlScopeValue(child, insideCacheControl || key == cacheControlKey) {
				changed = true
			}
		}
	case []any:
		for _, child := range node {
			if deleteCacheControlScopeValue(child, insideCacheControl) {
				changed = true
			}
		}
	}

	return changed
}

func (f *Forwarder) buildStreamingTransport() (*http.Transport, error) {
	f.transportOnce.Do(func() {
		httpTransport, err := transport.CreateTransport(f.config)
		if err != nil {
			f.transportErr = err
			return
		}

		// 优化传输设置用于流式处理
		httpTransport.DisableKeepAlives = false
		httpTransport.MaxIdleConns = 10
		httpTransport.MaxIdleConnsPerHost = 16
		httpTransport.MaxConnsPerHost = 32
		httpTransport.IdleConnTimeout = 0 // 无空闲超时
		httpTransport.TLSHandshakeTimeout = 10 * time.Second
		httpTransport.ExpectContinueTimeout = 1 * time.Second

		// 从配置中读取响应头超时时间，默认60秒
		responseHeaderTimeout := f.config.Streaming.ResponseHeaderTimeout
		if responseHeaderTimeout == 0 {
			responseHeaderTimeout = 60 * time.Second
		}
		httpTransport.ResponseHeaderTimeout = responseHeaderTimeout

		httpTransport.DisableCompression = true // 禁用压缩以防缓冲延迟
		httpTransport.WriteBufferSize = 4096    // 较小的写缓冲区
		httpTransport.ReadBufferSize = 4096     // 较小的读缓冲区

		f.httpTransport = httpTransport
	})

	if f.transportErr != nil {
		return nil, f.transportErr
	}
	return f.httpTransport, nil
}

// CopyHeaders 复制头部逻辑
func (f *Forwarder) CopyHeaders(src *http.Request, dst *http.Request, ep *endpoint.Endpoint) {
	// List of headers to skip/remove
	skipHeaders := map[string]bool{
		"host":          true, // We'll set this based on target endpoint
		"authorization": true, // We'll add our own if configured
		"x-api-key":     true, // Remove sensitive client API keys
	}

	// Copy all headers except those we want to skip
	for key, values := range src.Header {
		if skipHeaders[strings.ToLower(key)] {
			continue
		}

		for _, value := range values {
			dst.Header.Add(key, value)
		}
	}

	// Set Host header based on target endpoint URL
	if u, err := url.Parse(ep.Config.URL); err == nil {
		dst.Header.Set("Host", u.Host)
		// Also set the Host field directly on the request for proper HTTP/1.1 behavior
		dst.Host = u.Host
	}

	// Add or override Authorization header with dynamically resolved token
	token := f.endpointManager.GetTokenForEndpoint(ep)
	if token != "" {
		dst.Header.Set("Authorization", "Bearer "+token)
	}

	// Add or override X-Api-Key header with dynamically resolved api-key
	apiKey := f.endpointManager.GetApiKeyForEndpoint(ep)
	if apiKey != "" {
		dst.Header.Set("X-Api-Key", apiKey)
	}

	// Add custom headers from endpoint configuration
	for key, value := range ep.Config.Headers {
		dst.Header.Set(key, value)
	}
	if strings.EqualFold(strings.TrimSpace(ep.Config.Channel), anyrouteChannel) {
		ensureBetaFlag(dst.Header, anyrouteContextBetaFlag)
	}

	// Remove hop-by-hop headers
	hopByHopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}

	for _, header := range hopByHopHeaders {
		dst.Header.Del(header)
	}
}

// ensureBetaFlag 确保 anthropic-beta 头包含指定标志：
// 头不存在时直接设置；已存在但缺该标志时追加，保留客户端原有标志；已包含则不动。
func ensureBetaFlag(header http.Header, flag string) {
	existing := header.Get(anthropicBetaHeader)
	if existing == "" {
		header.Set(anthropicBetaHeader, flag)
		return
	}
	for _, part := range strings.Split(existing, ",") {
		if strings.EqualFold(strings.TrimSpace(part), flag) {
			return
		}
	}
	header.Set(anthropicBetaHeader, existing+","+flag)
}

// ForwardForPipeline 新端点转发管线专用（方案 §4.4）：
//   - 挂载 GotConn / TLSHandshakeDone / WroteHeaders / WroteRequest 四事件 trace，
//     供决策表以 WroteHeaders 判定重放安全边界；
//   - 保留 4xx/5xx 响应结构（分类交给决策表，不在此转换为 error）；
//   - detachUpstream 为 true 时创建独立 upstream context（下游断开后仍可短时 drain 尾部），返回其 release；
//   - 非 SSE 请求恢复端点级总超时，SSE 保持无总超时，避免长流被硬切断。
func (f *Forwarder) ForwardForPipeline(ctx context.Context, r *http.Request, bodyBytes []byte, ep *endpoint.Endpoint, isSSE, detachUpstream bool, onWroteRequest func(time.Time)) (*http.Response, context.CancelFunc, *UpstreamTraceState, error) {
	bodyBytes = prepareBodyForEndpoint(r.URL.Path, bodyBytes, ep)

	// 🛡️ 出站隐私过滤（PolicyError 由调用方短路，不进入换候选/标记）
	bodyBytes, err := ApplyPrivacyFilterForEndpoint(f.privacyFilter, r, bodyBytes, ep)
	if err != nil {
		return nil, nil, nil, err
	}

	requestCtx := ctx
	var release context.CancelFunc
	if detachUpstream {
		requestCtx, release = context.WithCancel(context.Background())
	}
	requestCtx, traceState := WithUpstreamTrace(requestCtx, onWroteRequest)

	targetURL := ep.Config.URL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	req, err := http.NewRequestWithContext(requestCtx, r.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		if release != nil {
			release()
		}
		return nil, nil, traceState, fmt.Errorf("failed to create request: %w", err)
	}
	f.CopyHeaders(r, req, ep)

	httpTransport, err := f.buildStreamingTransport()
	if err != nil {
		if release != nil {
			release()
		}
		return nil, nil, traceState, fmt.Errorf("failed to create transport: %w", err)
	}
	requestTimeout := time.Duration(0)
	if !isSSE {
		requestTimeout = ep.Config.Timeout
	}
	client := &http.Client{
		Timeout:   requestTimeout,
		Transport: httpTransport,
	}

	resp, err := client.Do(req)
	if err != nil {
		if release != nil {
			release()
		}
		return nil, nil, traceState, fmt.Errorf("request failed: %w", err)
	}
	return resp, release, traceState, nil
}
