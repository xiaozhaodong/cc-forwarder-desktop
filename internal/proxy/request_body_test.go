package proxy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/tracking"
)

func TestReadRequestBodyWithPrealloc_ReadsBody(t *testing.T) {
	body := `{"model":"gpt-5.4","input":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(body))

	got, err := readRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("readRequestBodyWithPrealloc returned error: %v", err)
	}
	if string(got) != body {
		t.Fatalf("expected body %q, got %q", body, string(got))
	}
}

func TestServeHTTP_RequestBodyTooLarge(t *testing.T) {
	cfg := &config.Config{
		RequestBodyMaxBytes: 8,
	}
	endpointManager := endpoint.NewManager(cfg)
	handler := NewHandler(endpointManager, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "too large") {
		t.Fatalf("expected body too large message, got %q", rec.Body.String())
	}
}

func TestRequestBodyMaxBytes_DefaultDisabled(t *testing.T) {
	cfg := &config.Config{}
	if got := requestBodyMaxBytes(cfg); got != 0 {
		t.Fatalf("expected default request body max bytes to be disabled, got %d", got)
	}
}

func TestServeHTTP_RequestBodyTooLarge_TracksRequest(t *testing.T) {
	tracker, err := tracking.NewUsageTracker(&tracking.Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      10,
		BatchSize:       5,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
	})
	if err != nil {
		t.Fatalf("failed to create usage tracker: %v", err)
	}
	defer tracker.Close()

	cfg := &config.Config{
		RequestBodyMaxBytes: 8,
	}
	endpointManager := endpoint.NewManager(cfg)
	handler := NewHandler(endpointManager, cfg)
	handler.SetUsageTracker(tracker)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5.4"}`))
	req = req.WithContext(context.WithValue(req.Context(), "conn_id", "req-body-too-large"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}

	start := time.Now().Add(-time.Minute)
	end := time.Now().Add(time.Minute)
	details, _, err := tracker.QueryRequestDetailsWithHotPool(context.Background(), &tracking.QueryOptions{
		StartDate: &start,
		EndDate:   &end,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("failed to query request details: %v", err)
	}

	for _, detail := range details {
		if detail.RequestID == "req-body-too-large" {
			if detail.FailureReason == "" {
				t.Fatalf("expected failure reason to be recorded for oversized body")
			}
			return
		}
	}

	t.Fatalf("expected oversized request to be tracked")
}
