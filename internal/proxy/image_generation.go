package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cc-forwarder/internal/privacy"
	"cc-forwarder/internal/proxy/handlers"
	"cc-forwarder/internal/transport"
)

const openAIImagesGenerationsPath = "/v1/images/generations"

// ImageGenerationConfig 是独立、单上游的图像生成配置。
type ImageGenerationConfig struct {
	Enabled       bool
	DirectConnect bool
	DirectPortMin int
	DirectPortMax int
	EndpointURL   string
	APIKey        string
	Model         string
	FixedPriceUSD float64
	Timeout       time.Duration
}

type imageGenerationRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

func (h *Handler) handleImageGeneration(ctx context.Context, w http.ResponseWriter, r *http.Request, bodyBytes []byte, lifecycleManager *RequestLifecycleManager) {
	lifecycleManager.SetUpstream("endpoint", "image_generation", "image-generation", 0)
	lifecycleManager.SetEndpoint("image-generation", "", "image")
	if r.Method != http.MethodPost {
		h.failImageGenerationRequest(w, lifecycleManager, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported")
		return
	}

	config, err := h.loadImageGenerationConfig(ctx)
	if err != nil {
		h.failImageGenerationRequest(w, lifecycleManager, http.StatusInternalServerError, "image_generation_config_error", err.Error())
		return
	}
	providerName := imageGenerationProviderName(config.EndpointURL)
	lifecycleManager.SetUpstream("endpoint", "image_generation", providerName, 0)
	lifecycleManager.SetEndpoint(providerName, "", "image")

	if !config.Enabled || strings.TrimSpace(config.EndpointURL) == "" || strings.TrimSpace(config.APIKey) == "" {
		h.failImageGenerationRequest(w, lifecycleManager, http.StatusServiceUnavailable, "image_generation_not_configured", "image generation provider is not configured or enabled")
		return
	}
	if err := validateImageGenerationEndpoint(config.EndpointURL); err != nil {
		h.failImageGenerationRequest(w, lifecycleManager, http.StatusServiceUnavailable, "image_generation_invalid_config", err.Error())
		return
	}

	requestBody, model, err := prepareImageGenerationRequestBody(bodyBytes, config.Model)
	if err != nil {
		h.failImageGenerationRequest(w, lifecycleManager, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	lifecycleManager.SetModel(model)

	requestBody, err = h.applyImageGenerationPrivacyFilter(r, requestBody, providerName)
	if err != nil {
		if policyErr := handlers.AsPrivacyPolicyError(err); policyErr != nil {
			lifecycleManager.FailRequest(handlers.PrivacyFailureReason(policyErr), policyErr.Message, policyErr.StatusCode)
			writeImageGenerationError(w, policyErr.StatusCode, policyErr.Code, policyErr.Message)
			return
		}
		h.failImageGenerationRequest(w, lifecycleManager, http.StatusInternalServerError, "privacy_filter_error", err.Error())
		return
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	requestCtx = withWroteRequestTrace(requestCtx, lifecycleManager.SetFirstTokenStartTime)

	upstreamReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, config.EndpointURL, bytes.NewReader(requestBody))
	if err != nil {
		h.failImageGenerationRequest(w, lifecycleManager, http.StatusInternalServerError, "image_generation_request_error", err.Error())
		return
	}
	copyImageGenerationRequestHeaders(r, upstreamReq)
	upstreamReq.Header.Set("Authorization", "Bearer "+config.APIKey)
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "application/json")

	client, releaseClient, err := h.getImageGenerationHTTPClient(config)
	if err != nil {
		h.failImageGenerationRequest(w, lifecycleManager, http.StatusInternalServerError, "image_generation_transport_error", err.Error())
		return
	}
	defer releaseClient()
	lifecycleManager.UpdateStatus("forwarding", 0, 0)
	resp, err := client.Do(upstreamReq)
	if err != nil {
		status := http.StatusBadGateway
		code := "image_generation_upstream_error"
		if requestCtx.Err() == context.DeadlineExceeded {
			status = http.StatusGatewayTimeout
			code = "image_generation_timeout"
		}
		h.failImageGenerationRequest(w, lifecycleManager, status, code, err.Error())
		return
	}
	defer resp.Body.Close()

	h.responseProcessor.CopyResponseHeaders(resp, w)
	w.Header().Del("Content-Length")
	lifecycleManager.UpdateStatus("processing", 0, resp.StatusCode)
	if resp.StatusCode >= http.StatusBadRequest {
		bufferedBody := bufio.NewReaderSize(resp.Body, 8192)
		responsePreview, _ := bufferedBody.Peek(8192)
		w.WriteHeader(resp.StatusCode)
		if _, writeErr := io.Copy(w, bufferedBody); writeErr != nil {
			lifecycleManager.FailRequest("image_generation_response_write_error", writeErr.Error(), resp.StatusCode)
			return
		}
		reason := summarizeImageGenerationUpstreamError(responsePreview, resp.StatusCode)
		lifecycleManager.FailRequest("image_generation_upstream_error", reason, resp.StatusCode)
		return
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		lifecycleManager.FailRequest("image_generation_response_write_error", err.Error(), resp.StatusCode)
		return
	}
	lifecycleManager.CompleteRequestWithCost(config.FixedPriceUSD)
}

func (h *Handler) loadImageGenerationConfig(ctx context.Context) (ImageGenerationConfig, error) {
	if h == nil || h.imageGenerationConfigProvider == nil {
		return ImageGenerationConfig{}, nil
	}
	return h.imageGenerationConfigProvider.GetImageGenerationConfig(ctx)
}

func (h *Handler) getImageGenerationHTTPClient(config ImageGenerationConfig) (*http.Client, func(), error) {
	if config.DirectConnect {
		if h.imageDirectHTTPClientFactory != nil {
			return h.imageDirectHTTPClientFactory(config.DirectPortMin, config.DirectPortMax)
		}
		directTransport, err := transport.CreateSourcePortTransport(config.DirectPortMin, config.DirectPortMax)
		if err != nil {
			return nil, nil, err
		}
		directTransport.DisableCompression = true
		client := &http.Client{Transport: directTransport}
		return client, directTransport.CloseIdleConnections, nil
	}
	h.imageHTTPInitOnce.Do(func() {
		if h.config == nil {
			h.imageHTTPInitErr = fmt.Errorf("handler config is nil")
			return
		}
		h.imageHTTPTransport, h.imageHTTPInitErr = transport.CreateTransport(h.config)
		if h.imageHTTPInitErr != nil {
			return
		}
		h.imageHTTPTransport.DisableCompression = true
		h.imageHTTPClient = &http.Client{Transport: h.imageHTTPTransport}
	})
	return h.imageHTTPClient, func() {}, h.imageHTTPInitErr
}

func prepareImageGenerationRequestBody(bodyBytes []byte, defaultModel string) ([]byte, string, error) {
	var request imageGenerationRequest
	if err := json.Unmarshal(bodyBytes, &request); err != nil {
		return nil, "", fmt.Errorf("request body must be valid JSON: %w", err)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, "", fmt.Errorf("prompt is required")
	}
	model := strings.TrimSpace(request.Model)
	if model != "" {
		return bodyBytes, model, nil
	}
	model = strings.TrimSpace(defaultModel)
	if model == "" {
		model = "gpt-image-2"
	}
	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, "", fmt.Errorf("request body must be a JSON object: %w", err)
	}
	payload["model"] = model
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("encode image generation request: %w", err)
	}
	return encoded, model, nil
}

func (h *Handler) applyImageGenerationPrivacyFilter(r *http.Request, bodyBytes []byte, providerName string) ([]byte, error) {
	if h == nil || h.privacyFilter == nil {
		return bodyBytes, nil
	}
	request := privacy.Request{
		Path:         openAIImagesGenerationsPath,
		Method:       http.MethodPost,
		UpstreamType: privacy.UpstreamTypeEndpoint,
		EndpointName: providerName,
		ContentType:  r.Header.Get("Content-Type"),
	}
	return handlers.ApplyPrivacyFilter(h.privacyFilter, r, request, bodyBytes)
}

func copyImageGenerationRequestHeaders(src, dst *http.Request) {
	for key, values := range src.Header {
		switch strings.ToLower(key) {
		case "host", "authorization", "x-api-key", "cookie", "content-length", "accept", "content-type":
			continue
		}
		for _, value := range values {
			dst.Header.Add(key, value)
		}
	}
}

func validateImageGenerationEndpoint(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return fmt.Errorf("image generation endpoint must be a valid http/https URL")
	}
	return nil
}

func imageGenerationProviderName(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return "image-generation"
}

func (h *Handler) failImageGenerationRequest(w http.ResponseWriter, lifecycleManager *RequestLifecycleManager, status int, code, message string) {
	lifecycleManager.FailRequest(code, message, status)
	writeImageGenerationError(w, status, code, message)
}

func writeImageGenerationError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "image_generation_error",
			"code":    code,
		},
	})
}

func summarizeImageGenerationUpstreamError(body []byte, status int) string {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Message != "" {
		if envelope.Error.Code != "" {
			return envelope.Error.Code + ": " + envelope.Error.Message
		}
		return envelope.Error.Message
	}
	return fmt.Sprintf("image generation upstream returned HTTP %d", status)
}
