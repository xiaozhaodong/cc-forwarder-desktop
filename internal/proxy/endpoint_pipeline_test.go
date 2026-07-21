package proxy

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/privacy"
)

func newEndpointPipelineTestHandler(t *testing.T, endpoints ...config.EndpointConfig) (*Handler, *endpoint.Manager) {
	t.Helper()

	cfg := &config.Config{
		Strategy: config.StrategyConfig{Type: "priority"},
		Failover: config.FailoverConfig{
			Enabled:         true,
			DefaultCooldown: 10 * time.Minute,
		},
		FailureTracker: config.FailureTrackerConfig{
			Enabled:    true,
			TimeWindow: 5 * time.Minute,
			Threshold:  3,
			Action:     "failover",
		},
		Streaming: config.StreamingConfig{
			ResponseHeaderTimeout: 2 * time.Second,
		},
		GlobalTimeout: 2 * time.Second,
		Endpoints:     endpoints,
	}

	manager := endpoint.NewManager(cfg)
	t.Cleanup(manager.Stop)
	return NewHandler(manager, cfg), manager
}

func endpointPipelineConfig(name, url string, priority int) config.EndpointConfig {
	return config.EndpointConfig{
		Name:     name,
		URL:      url,
		Priority: priority,
		Timeout:  2 * time.Second,
	}
}

func endpointPipelineClosedURL(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for closed endpoint URL: %v", err)
	}
	url := "http://" + listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close endpoint listener: %v", err)
	}
	return url
}

func performEndpointPipelineRequest(t *testing.T, handler *Handler, streaming bool) *httptest.ResponseRecorder {
	t.Helper()

	body := `{"model":"claude-test","messages":[]}`
	if streaming {
		body = `{"model":"claude-test","messages":[],"stream":true}`
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), "conn_id", "req-endpoint-pipeline"))
	req.Header.Set("Content-Type", "application/json")
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func requireLatestEndpointScheduleSnapshot(t *testing.T, manager *endpoint.Manager, outcome, selectedEndpoint string) *endpoint.EndpointScheduleSnapshot {
	t.Helper()
	snapshot := manager.GetLatestEndpointScheduleSnapshot()
	if snapshot == nil {
		t.Fatal("expected latest endpoint schedule snapshot")
	}
	if snapshot.RequestID != "req-endpoint-pipeline" || snapshot.RequestPath != "/v1/messages" {
		t.Fatalf("unexpected snapshot request metadata: %+v", snapshot)
	}
	if snapshot.FinalOutcome != outcome || snapshot.SelectedEndpoint != selectedEndpoint {
		t.Fatalf("unexpected snapshot final state: %+v", snapshot)
	}
	return snapshot
}

func TestEndpointPipeline_ConnectionFailureBeforeWroteHeaders_FailsOverAndMigratesActive(t *testing.T) {
	var backupHits int32
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&backupHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(backup.Close)

	handler, manager := newEndpointPipelineTestHandler(t,
		endpointPipelineConfig("primary", endpointPipelineClosedURL(t), 1),
		endpointPipelineConfig("backup", backup.URL, 2),
	)
	manager.RestoreActiveEndpoint("primary")

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected backup success, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := atomic.LoadInt32(&backupHits); got != 1 {
		t.Fatalf("expected backup to be called once, got %d", got)
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "backup" {
		t.Fatalf("expected active endpoint to migrate to backup, got %q", active)
	}
	if got := manager.GetFailureStats()["primary"]; got != 1 {
		t.Fatalf("expected primary failure count 1, got %d", got)
	}
	snapshot := requireLatestEndpointScheduleSnapshot(t, manager, endpoint.EndpointScheduleOutcomeSuccess, "backup")
	if snapshot.ActiveEndpointAtSelection != "primary" || snapshot.Decisions[0].RuntimeOutcome != endpoint.EndpointScheduleRuntimeTryNext || snapshot.Decisions[1].RuntimeOutcome != endpoint.EndpointScheduleOutcomeSuccess {
		t.Fatalf("unexpected failover attempt decisions: %+v", snapshot.Decisions)
	}
}

func TestEndpointPipeline_Unauthorized_FailsOverAndMigratesActive(t *testing.T) {
	var primaryHits, backupHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&primaryHits, 1)
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	}))
	t.Cleanup(primary.Close)
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&backupHits, 1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(backup.Close)

	handler, manager := newEndpointPipelineTestHandler(t,
		endpointPipelineConfig("primary", primary.URL, 1),
		endpointPipelineConfig("backup", backup.URL, 2),
	)
	manager.RestoreActiveEndpoint("primary")

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected backup success, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := atomic.LoadInt32(&primaryHits); got != 1 {
		t.Fatalf("expected primary to be called once, got %d", got)
	}
	if got := atomic.LoadInt32(&backupHits); got != 1 {
		t.Fatalf("expected backup to be called once, got %d", got)
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "backup" {
		t.Fatalf("expected active endpoint to migrate to backup, got %q", active)
	}
	if primaryEndpoint := manager.GetEndpointByNameAny("primary"); primaryEndpoint == nil || !primaryEndpoint.IsInCooldown() {
		t.Fatal("expected unauthorized primary endpoint to enter auth cooldown")
	}
}

func TestEndpointPipeline_Generic5xx_ReturnsRetryableErrorWithoutFailover(t *testing.T) {
	var primaryHits, backupHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&primaryHits, 1)
		http.Error(w, `{"error":"upstream unavailable"}`, http.StatusServiceUnavailable)
	}))
	t.Cleanup(primary.Close)
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&backupHits, 1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(backup.Close)

	handler, manager := newEndpointPipelineTestHandler(t,
		endpointPipelineConfig("primary", primary.URL, 1),
		endpointPipelineConfig("backup", backup.URL, 2),
	)
	manager.RestoreActiveEndpoint("primary")

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected generic 5xx to become 500, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Retry-After"); got != "5" {
		t.Fatalf("expected Retry-After=5, got %q", got)
	}
	if got := atomic.LoadInt32(&primaryHits); got != 1 {
		t.Fatalf("expected primary to be called once, got %d", got)
	}
	if got := atomic.LoadInt32(&backupHits); got != 0 {
		t.Fatalf("expected no failover after ambiguous 5xx, backup hits=%d", got)
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "primary" {
		t.Fatalf("expected active endpoint to remain primary, got %q", active)
	}
	if got := manager.GetFailureStats()["primary"]; got != 1 {
		t.Fatalf("expected primary failure count 1, got %d", got)
	}
	requireLatestEndpointScheduleSnapshot(t, manager, endpoint.EndpointScheduleOutcomePassthroughError, "primary")
}

func TestEndpointPipeline_NotFound_PassesThroughWithoutFailover(t *testing.T) {
	var primaryHits, backupHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&primaryHits, 1)
		w.Header().Set("X-Upstream", "primary")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	t.Cleanup(primary.Close)
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&backupHits, 1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(backup.Close)

	handler, manager := newEndpointPipelineTestHandler(t,
		endpointPipelineConfig("primary", primary.URL, 1),
		endpointPipelineConfig("backup", backup.URL, 2),
	)
	manager.RestoreActiveEndpoint("primary")

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected raw 404 passthrough, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Upstream") != "primary" {
		t.Fatalf("expected upstream headers to be preserved, got %v", recorder.Header())
	}
	if !strings.Contains(recorder.Body.String(), `"error":"not found"`) {
		t.Fatalf("expected upstream body to be preserved, got %s", recorder.Body.String())
	}
	if got := atomic.LoadInt32(&primaryHits); got != 1 {
		t.Fatalf("expected primary to be called once, got %d", got)
	}
	if got := atomic.LoadInt32(&backupHits); got != 0 {
		t.Fatalf("expected no failover for 404, backup hits=%d", got)
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "primary" {
		t.Fatalf("expected active endpoint to remain primary, got %q", active)
	}
	if got := manager.GetFailureStats()["primary"]; got != 0 {
		t.Fatalf("expected 404 not to count as endpoint failure, got %d", got)
	}
	requireLatestEndpointScheduleSnapshot(t, manager, endpoint.EndpointScheduleOutcomePassthroughRaw, "primary")
}

func TestEndpointPipeline_QualityIncomplete_DoesNotMigrateActive(t *testing.T) {
	var backupHits int32
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&backupHits, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-test\",\"content\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":10,\"output_tokens\":5}}\n\n"))
	}))
	t.Cleanup(backup.Close)

	handler, manager := newEndpointPipelineTestHandler(t,
		endpointPipelineConfig("primary", endpointPipelineClosedURL(t), 1),
		endpointPipelineConfig("backup", backup.URL, 2),
	)
	manager.RestoreActiveEndpoint("primary")

	recorder := performEndpointPipelineRequest(t, handler, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected incomplete stream to remain a completed response, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := atomic.LoadInt32(&backupHits); got != 1 {
		t.Fatalf("expected backup to be called once, got %d", got)
	}
	if !strings.Contains(recorder.Body.String(), "event: message_delta") {
		t.Fatalf("expected incomplete upstream stream to be relayed, got %s", recorder.Body.String())
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "primary" {
		t.Fatalf("expected QualityIncomplete not to migrate active endpoint, got %q", active)
	}
	snapshot := requireLatestEndpointScheduleSnapshot(t, manager, endpoint.EndpointScheduleOutcomeQualityIncomplete, "backup")
	if snapshot.FinalError == "" {
		t.Fatal("expected incomplete stream reason in snapshot")
	}
}

func TestEndpointPipeline_NoCandidatesRecordsSnapshot(t *testing.T) {
	handler, manager := newEndpointPipelineTestHandler(t)

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without candidates, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	snapshot := requireLatestEndpointScheduleSnapshot(t, manager, endpoint.EndpointScheduleOutcomeNoCandidates, "")
	if len(snapshot.Decisions) != 0 {
		t.Fatalf("expected no candidate decisions, got %+v", snapshot.Decisions)
	}
}

func TestEndpointPipeline_PrivacyBlockRecordsSnapshotWithoutFailover(t *testing.T) {
	var upstreamHits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&upstreamHits, 1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	handler, manager := newEndpointPipelineTestHandler(t,
		endpointPipelineConfig("primary", upstream.URL, 1),
		endpointPipelineConfig("backup", upstream.URL, 2),
	)
	manager.RestoreActiveEndpoint("primary")
	handler.SetPrivacyFilter(&stubPrivacyFilter{
		err: &privacy.PolicyError{StatusCode: http.StatusRequestEntityTooLarge, Code: privacy.CodeScanBodyTooLarge, Message: "scannable text too large"},
	})

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected privacy 413, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := atomic.LoadInt32(&upstreamHits); got != 0 {
		t.Fatalf("privacy block must not reach upstream, hits=%d", got)
	}
	requireLatestEndpointScheduleSnapshot(t, manager, endpoint.EndpointScheduleOutcomePrivacyBlocked, "primary")
}
