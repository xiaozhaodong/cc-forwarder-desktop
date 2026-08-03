package proxy

import (
	"bytes"
	"context"
	"encoding/json"
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
	return newEndpointPipelineTestHandlerWithAction(t, "failover", endpoints...)
}

func newEndpointPipelineTestHandlerWithAction(t *testing.T, failureTrackerAction string, endpoints ...config.EndpointConfig) (*Handler, *endpoint.Manager) {
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
			Action:     failureTrackerAction,
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

// [Phase3 §11.2 D9] fallback 成功只更新运行时 retained，active/legacy 状态不变
func TestEndpointPipeline_ConnectionFailure_FailsOverAndRetainsWithoutMigratingActive(t *testing.T) {
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
	if active, _ := manager.GetActiveEndpointSelection(); active != "primary" {
		t.Fatalf("v8: fallback success must not migrate active, got %q", active)
	}
	if got := manager.RetainedInTier(2); got != "backup" {
		t.Fatalf("expected backup retained in tier 2, got %q", got)
	}
	if got := manager.GetFailureStats()["primary"]; got != 1 {
		t.Fatalf("expected primary failure count 1, got %d", got)
	}
	snapshot := requireLatestEndpointScheduleSnapshot(t, manager, endpoint.EndpointScheduleOutcomeSuccess, "backup")
	if snapshot.Decisions[0].RuntimeOutcome != endpoint.EndpointScheduleRuntimeTryNext || snapshot.Decisions[1].RuntimeOutcome != endpoint.EndpointScheduleOutcomeSuccess {
		t.Fatalf("unexpected failover attempt decisions: %+v", snapshot.Decisions)
	}
}

// [Phase3 §11.2] auth failover 成功后 active 不迁移；auth cooldown 断言保留
func TestEndpointPipeline_Unauthorized_FailsOverWithoutMigratingActive(t *testing.T) {
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
	if active, _ := manager.GetActiveEndpointSelection(); active != "primary" {
		t.Fatalf("v8: fallback success must not migrate active, got %q", active)
	}
	if primaryEndpoint := manager.GetEndpointByNameAny("primary"); primaryEndpoint == nil || !primaryEndpoint.IsInCooldown() {
		t.Fatal("expected unauthorized primary endpoint to enter auth cooldown")
	}
}

func TestEndpointPipeline_ModelRewriteUsesOriginalBodyForEachCandidate(t *testing.T) {
	var primaryModel, backupModel string
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode primary body failed: %v", err)
		}
		primaryModel, _ = payload["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
	}))
	t.Cleanup(primary.Close)
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode backup body failed: %v", err)
		}
		backupModel, _ = payload["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(backup.Close)

	primaryConfig := endpointPipelineConfig("primary", primary.URL, 1)
	primaryConfig.ModelRewriteRules = `[{"paths":["/v1/messages"],"match":"exact","from":"claude-test","to":"primary-model"}]`
	backupConfig := endpointPipelineConfig("backup", backup.URL, 2)
	backupConfig.ModelRewriteRules = `[{"paths":["/v1/messages"],"match":"exact","from":"claude-test","to":"backup-model"}]`
	handler, manager := newEndpointPipelineTestHandler(t, primaryConfig, backupConfig)
	manager.RestoreActiveEndpoint("primary")

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected backup success, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if primaryModel != "primary-model" || backupModel != "backup-model" {
		t.Fatalf("expected endpoint-specific rewrites from original body, got primary=%q backup=%q", primaryModel, backupModel)
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

// [Phase1 §9.2] 普通 429：同端点最多重试一次；两次均 429 只记一次 rate_limit 软失败，
// 阈值前不换端点，把规范化 429（保留 Retry-After）返回客户端，不进入冷却。
func TestEndpointPipeline_Ordinary429RetriesSameEndpointThenReturns429(t *testing.T) {
	var primaryHits, backupHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&primaryHits, 1)
		w.Header().Set("Retry-After", "1")
		http.Error(w, `{"error":{"type":"rate_limit_error"}}`, http.StatusTooManyRequests)
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
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected normalized 429, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("expected Retry-After preserved, got %q", got)
	}
	if got := atomic.LoadInt32(&primaryHits); got != 2 {
		t.Fatalf("expected exactly one same-endpoint retry (2 upstream hits), got %d", got)
	}
	if got := atomic.LoadInt32(&backupHits); got != 0 {
		t.Fatalf("threshold not reached: must not switch endpoint, backup hits=%d", got)
	}
	if primaryEndpoint := manager.GetEndpointByNameAny("primary"); primaryEndpoint.IsInCooldown() {
		t.Fatal("single logical 429 must not enter cooldown before threshold")
	}
	if got := manager.GetFailureStats()["primary"]; got != 1 {
		t.Fatalf("two upstream 429 must settle as one soft failure, got %d", got)
	}
	if active, _ := manager.GetActiveEndpointSelection(); active != "primary" {
		t.Fatalf("429 must not change active selection, got %q", active)
	}
	requireLatestEndpointScheduleSnapshot(t, manager, endpoint.EndpointScheduleOutcomeRateLimited, "primary")
}

// [Phase1 §9.2 规则 6] 首次 429 后本地重试成功：返回 200，本次不记软失败
func TestEndpointPipeline_Ordinary429RetrySucceedsWithoutSoftFailure(t *testing.T) {
	var primaryHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&primaryHits, 1) == 1 {
			http.Error(w, `{"error":{"type":"rate_limit_error"}}`, http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(primary.Close)

	handler, manager := newEndpointPipelineTestHandler(t,
		endpointPipelineConfig("primary", primary.URL, 1),
	)
	manager.RestoreActiveEndpoint("primary")

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected retry success 200, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := atomic.LoadInt32(&primaryHits); got != 2 {
		t.Fatalf("expected two upstream hits (429 then 200), got %d", got)
	}
	if got := manager.GetFailureStats()["primary"]; got != 0 {
		t.Fatalf("retry success must not record soft failure, got %d", got)
	}
}

// [Phase1 §9.2 规则 9] 第 3 个逻辑 429 达阈值：进入冷却并允许本请求内换下一候选
func TestEndpointPipeline_Ordinary429ThirdLogicalFailureTripsCooldownAndFailsOver(t *testing.T) {
	var primaryHits, backupHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&primaryHits, 1)
		http.Error(w, `{"error":{"type":"rate_limit_error"}}`, http.StatusTooManyRequests)
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

	// 前两个逻辑失败（每个客户端请求只记一次）
	for i := 0; i < 2; i++ {
		recorder := performEndpointPipelineRequest(t, handler, false)
		if recorder.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d: expected 429 before threshold, got %d", i+1, recorder.Code)
		}
	}
	if got := manager.GetFailureStats()["primary"]; got != 2 {
		t.Fatalf("expected 2 soft failures before threshold, got %d", got)
	}

	// 第 3 个逻辑失败触发阈值：冷却 + 换候选成功
	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected threshold-triggered failover success, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := atomic.LoadInt32(&backupHits); got != 1 {
		t.Fatalf("expected backup hit after threshold, got %d", got)
	}
	if primaryEndpoint := manager.GetEndpointByNameAny("primary"); !primaryEndpoint.IsInCooldown() {
		t.Fatal("expected primary in cooldown after threshold")
	}
	if got := manager.GetFailureStats()["primary"]; got != 0 {
		t.Fatalf("threshold trip should clear the triggering window, got %d", got)
	}
}

// [Phase1 §8.5] 请求内尝试预算：最多尝试 3 个不同端点，不穿透剩余候选
func TestEndpointPipeline_CandidateAttemptBudgetStopsAtThree(t *testing.T) {
	var fourthHits int32
	fourth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&fourthHits, 1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(fourth.Close)

	handler, manager := newEndpointPipelineTestHandler(t,
		endpointPipelineConfig("e1", endpointPipelineClosedURL(t), 1),
		endpointPipelineConfig("e2", endpointPipelineClosedURL(t), 2),
		endpointPipelineConfig("e3", endpointPipelineClosedURL(t), 3),
		endpointPipelineConfig("e4", fourth.URL, 4),
	)
	manager.RestoreActiveEndpoint("e1")

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 after budget exhausted, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got := atomic.LoadInt32(&fourthHits); got != 0 {
		t.Fatalf("4th endpoint beyond budget must not be attempted, hits=%d", got)
	}
	if !strings.Contains(recorder.Body.String(), "budget exhausted") {
		t.Fatalf("expected budget exhausted reason, got %s", recorder.Body.String())
	}
}

// [Phase1 D15 §12] reject 语义：第一逻辑候选处于软失败阈值型冷却即拒绝，
// 不尝试健康备用端点。
func TestEndpointPipeline_RejectModeRejectsWhenFirstLogicalCandidateTripped(t *testing.T) {
	var backupHits int32
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&backupHits, 1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(backup.Close)

	handler, manager := newEndpointPipelineTestHandlerWithAction(t, "reject",
		endpointPipelineConfig("primary", endpointPipelineClosedURL(t), 1),
		endpointPipelineConfig("backup", backup.URL, 2),
	)
	manager.RestoreActiveEndpoint("primary")
	manager.SetEndpointCooldown("primary", time.Minute,
		endpoint.SoftFailureCooldownReason(endpoint.SoftFailureCategoryRateLimit))

	recorder := performEndpointPipelineRequest(t, handler, false)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected reject-mode 503, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "failure threshold reached") {
		t.Fatalf("expected failure threshold message, got %s", recorder.Body.String())
	}
	if got := atomic.LoadInt32(&backupHits); got != 0 {
		t.Fatalf("reject mode must skip healthy backup, hits=%d", got)
	}
	requireLatestEndpointScheduleSnapshot(t, manager, endpoint.EndpointScheduleOutcomeRejectedByFailureTracker, "")
}
