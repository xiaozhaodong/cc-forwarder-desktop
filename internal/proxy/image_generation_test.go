package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/tracking"
)

type staticImageGenerationConfigProvider struct {
	config ImageGenerationConfig
	err    error
}

func (p staticImageGenerationConfigProvider) GetImageGenerationConfig(context.Context) (ImageGenerationConfig, error) {
	return p.config, p.err
}

func TestImageGeneration_ForwardsConfiguredProviderAndTracksRequest(t *testing.T) {
	var receivedAuth string
	var receivedPath string
	var receivedBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode upstream request failed: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"aW1hZ2U="}]}`))
	}))
	defer upstream.Close()

	tracker := newImageGenerationTestTracker(t)
	defer tracker.Close()
	endpointManager := endpoint.NewManager(&config.Config{})
	handler := NewHandler(endpointManager, &config.Config{})
	handler.SetMonitoringMiddleware(middleware.NewMonitoringMiddleware(endpointManager))
	handler.SetUsageTracker(tracker)
	handler.SetImageGenerationConfigProvider(staticImageGenerationConfigProvider{config: ImageGenerationConfig{
		Enabled:       true,
		EndpointURL:   upstream.URL + openAIImagesGenerationsPath,
		APIKey:        "secret-key",
		Model:         "gpt-image-2",
		FixedPriceUSD: 0.25,
		Timeout:       5 * time.Second,
	}})

	req := httptest.NewRequest(http.MethodPost, openAIImagesGenerationsPath, strings.NewReader(`{"prompt":"draw a test icon","quality":"high"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "conn_id", "req-image-generation"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if receivedPath != openAIImagesGenerationsPath {
		t.Fatalf("expected upstream path %s, got %s", openAIImagesGenerationsPath, receivedPath)
	}
	if receivedAuth != "Bearer secret-key" {
		t.Fatalf("unexpected upstream authorization header: %q", receivedAuth)
	}
	if receivedBody["model"] != "gpt-image-2" || receivedBody["prompt"] != "draw a test icon" {
		t.Fatalf("unexpected upstream body: %+v", receivedBody)
	}

	start := time.Now().Add(-time.Minute)
	end := time.Now().Add(time.Minute)
	details, _, err := tracker.QueryRequestDetailsWithHotPool(context.Background(), &tracking.QueryOptions{
		StartDate: &start,
		EndDate:   &end,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("query tracked image request failed: %v", err)
	}
	for _, detail := range details {
		if detail.RequestID != "req-image-generation" {
			continue
		}
		if detail.Status != "completed" || detail.Path != openAIImagesGenerationsPath {
			t.Fatalf("unexpected tracked status/path: %+v", detail)
		}
		if detail.Channel != "image" || detail.UpstreamSourceName != "image_generation" {
			t.Fatalf("unexpected tracked image source: %+v", detail)
		}
		if detail.ModelName != "gpt-image-2" {
			t.Fatalf("unexpected tracked model: %q", detail.ModelName)
		}
		if detail.TotalCostUSD != 0.25 {
			t.Fatalf("expected tracked image cost 0.25, got %f", detail.TotalCostUSD)
		}
		return
	}
	t.Fatal("expected image generation request in request tracking")
}

func TestImageGeneration_DisabledReturnsExplicitErrorWithoutEndpointFallback(t *testing.T) {
	fallbackHits := 0
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHits++
		w.WriteHeader(http.StatusOK)
	}))
	defer fallback.Close()

	cfg := &config.Config{Endpoints: []config.EndpointConfig{{
		Name: "fallback", URL: fallback.URL,
	}}}
	handler := NewHandler(endpoint.NewManager(cfg), cfg)
	handler.SetImageGenerationConfigProvider(staticImageGenerationConfigProvider{})
	req := httptest.NewRequest(http.MethodPost, openAIImagesGenerationsPath, strings.NewReader(`{"prompt":"test"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	if fallbackHits != 0 {
		t.Fatalf("expected no endpoint fallback, got %d hits", fallbackHits)
	}
	if !strings.Contains(rec.Body.String(), "image_generation_not_configured") {
		t.Fatalf("unexpected error body: %s", rec.Body.String())
	}
}

func TestPrepareImageGenerationRequestBody_PreservesExplicitModel(t *testing.T) {
	body := []byte(`{"model":"custom-image-model","prompt":"test","n":1}`)
	prepared, model, err := prepareImageGenerationRequestBody(body, "gpt-image-2")
	if err != nil {
		t.Fatalf("prepare request failed: %v", err)
	}
	if model != "custom-image-model" || string(prepared) != string(body) {
		t.Fatalf("expected explicit model request to remain unchanged, model=%s body=%s", model, prepared)
	}
}

func newImageGenerationTestTracker(t *testing.T) *tracking.UsageTracker {
	t.Helper()
	tracker, err := tracking.NewUsageTracker(&tracking.Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      20,
		BatchSize:       5,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
	})
	if err != nil {
		t.Fatalf("create usage tracker failed: %v", err)
	}
	return tracker
}
