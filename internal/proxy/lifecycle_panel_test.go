package proxy

import (
	"context"
	"errors"
	"net/http/httptrace"
	"testing"
	"time"

	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/proxy/handlers"
	"cc-forwarder/internal/tracking"
)

// newLifecyclePanelTestTracker 构造启用热池的内存 tracker。
func newLifecyclePanelTestTracker(t *testing.T) *tracking.UsageTracker {
	t.Helper()
	tracker, err := tracking.NewUsageTracker(&tracking.Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      10,
		BatchSize:       5,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
		HotPool: &tracking.HotPoolSettings{
			Enabled:          true,
			MaxAge:           30 * time.Minute,
			MaxSize:          1000,
			CleanupInterval:  time.Minute,
			ArchiveOnCleanup: true,
		},
	})
	if err != nil {
		t.Fatalf("failed to create usage tracker: %v", err)
	}
	t.Cleanup(func() { _ = tracker.Close() })
	return tracker
}

// TestUpstreamWriteMs_PersistedBeforeFirstToken 首字前收到写完回调 → 立即持久化偏移量。
func TestUpstreamWriteMs_PersistedBeforeFirstToken(t *testing.T) {
	tracker := newLifecyclePanelTestTracker(t)

	rlm := NewRequestLifecycleManager(tracker, nil, "req-upstream-write-ok", nil)
	rlm.StartRequest("127.0.0.1", "test-agent", "POST", "/v1/responses", true)

	time.Sleep(10 * time.Millisecond)
	rlm.SetFirstTokenStartTime(time.Now())

	beforeFirstToken, err := tracker.GetRequestLifecycleData(context.Background(), "req-upstream-write-ok")
	if err != nil {
		t.Fatalf("GetRequestLifecycleData before first token failed: %v", err)
	}
	if beforeFirstToken == nil || beforeFirstToken.UpstreamWriteMs == nil {
		t.Fatalf("expected upstream_write_ms persisted before first token, got %+v", beforeFirstToken)
	}
	if beforeFirstToken.Detail.FirstTokenMs != nil {
		t.Fatalf("first_token_ms must still be nil before first token, got %d", *beforeFirstToken.Detail.FirstTokenMs)
	}

	rlm.RecordFirstToken()

	data, err := tracker.GetRequestLifecycleData(context.Background(), "req-upstream-write-ok")
	if err != nil {
		t.Fatalf("GetRequestLifecycleData failed: %v", err)
	}
	if data == nil {
		t.Fatal("expected lifecycle data in hot pool")
	}
	if data.Source != "hot_pool" {
		t.Fatalf("expected hot_pool source, got %q", data.Source)
	}
	if data.Detail.FirstTokenMs == nil {
		t.Fatal("expected first_token_ms recorded")
	}
	if data.UpstreamWriteMs == nil {
		t.Fatal("expected upstream_write_ms retained after first token")
	}
	if *data.UpstreamWriteMs < 0 {
		t.Fatalf("upstream_write_ms must be >= 0, got %d", *data.UpstreamWriteMs)
	}
}

// TestFirstTokenStartTime_RetryUsesLatestWrite 重试只更新 FRT 起点，不覆盖首次 upstream_write_ms。
func TestFirstTokenStartTime_RetryUsesLatestWrite(t *testing.T) {
	tracker := newLifecyclePanelTestTracker(t)
	const requestID = "req-upstream-write-retry"

	rlm := NewRequestLifecycleManager(tracker, nil, requestID, nil)
	rlm.StartRequest("127.0.0.1", "test-agent", "POST", "/v1/responses", true)

	// 用可控时间差明确区分首次尝试（约 1.9s 前）和最终尝试（约 50ms 前）。
	rlm.startTime = time.Now().Add(-2 * time.Second)
	firstWrite := rlm.startTime.Add(100 * time.Millisecond)
	rlm.SetFirstTokenStartTime(firstWrite)

	firstData, err := tracker.GetRequestLifecycleData(context.Background(), requestID)
	if err != nil {
		t.Fatalf("GetRequestLifecycleData after first write failed: %v", err)
	}
	if firstData == nil || firstData.UpstreamWriteMs == nil {
		t.Fatalf("expected first upstream_write_ms, got %+v", firstData)
	}
	firstUpstreamWriteMs := *firstData.UpstreamWriteMs
	if firstUpstreamWriteMs != 100 {
		t.Fatalf("expected first upstream_write_ms to stay at 100ms, got %d", firstUpstreamWriteMs)
	}

	latestWrite := time.Now().Add(-50 * time.Millisecond)
	rlm.SetFirstTokenStartTime(latestWrite)

	rlm.timingMu.RLock()
	actualStart := rlm.firstTokenStartTime
	rlm.timingMu.RUnlock()
	if !actualStart.Equal(latestWrite) {
		t.Fatalf("expected latest successful write as FRT start, got %s want %s", actualStart, latestWrite)
	}

	latestData, err := tracker.GetRequestLifecycleData(context.Background(), requestID)
	if err != nil {
		t.Fatalf("GetRequestLifecycleData after retry write failed: %v", err)
	}
	if latestData == nil || latestData.UpstreamWriteMs == nil {
		t.Fatalf("expected upstream_write_ms retained, got %+v", latestData)
	}
	if *latestData.UpstreamWriteMs != firstUpstreamWriteMs {
		t.Fatalf("retry must not overwrite upstream_write_ms, got %d want %d", *latestData.UpstreamWriteMs, firstUpstreamWriteMs)
	}

	rlm.RecordFirstToken()
	finalData, err := tracker.GetRequestLifecycleData(context.Background(), requestID)
	if err != nil {
		t.Fatalf("GetRequestLifecycleData after first token failed: %v", err)
	}
	if finalData == nil || finalData.Detail.FirstTokenMs == nil {
		t.Fatalf("expected first_token_ms recorded, got %+v", finalData)
	}
	if *finalData.Detail.FirstTokenMs >= 500 {
		t.Fatalf("first_token_ms must use latest retry write, got %dms", *finalData.Detail.FirstTokenMs)
	}
}

// TestUpstreamWriteMs_PersistedWhenRequestFailsBeforeFirstToken 写完后首字前失败仍保留连接阶段数据。
func TestUpstreamWriteMs_PersistedWhenRequestFailsBeforeFirstToken(t *testing.T) {
	tracker := newLifecyclePanelTestTracker(t)
	const requestID = "req-upstream-write-failed"

	rlm := NewRequestLifecycleManager(tracker, nil, requestID, nil)
	rlm.StartRequest("127.0.0.1", "test-agent", "POST", "/v1/responses", true)
	rlm.SetFirstTokenStartTime(time.Now())
	rlm.FailRequest("upstream_error", "failed before first token", 502)

	data, err := tracker.GetRequestLifecycleData(context.Background(), requestID)
	if err != nil {
		t.Fatalf("GetRequestLifecycleData failed: %v", err)
	}
	if data == nil || data.UpstreamWriteMs == nil {
		t.Fatalf("expected upstream_write_ms on pre-first-token failure, got %+v", data)
	}
	if data.Detail.FirstTokenMs != nil {
		t.Fatalf("expected first_token_ms nil on pre-first-token failure, got %d", *data.Detail.FirstTokenMs)
	}
}

// TestUpstreamWriteMs_IgnoresFailedWroteRequest 写请求报错不能被当作完整写出。
func TestUpstreamWriteMs_IgnoresFailedWroteRequest(t *testing.T) {
	tracker := newLifecyclePanelTestTracker(t)
	const requestID = "req-upstream-write-error"

	rlm := NewRequestLifecycleManager(tracker, nil, requestID, nil)
	rlm.StartRequest("127.0.0.1", "test-agent", "POST", "/v1/responses", true)
	traceCtx, _ := handlers.WithUpstreamTrace(context.Background(), rlm.SetFirstTokenStartTime)
	trace := httptrace.ContextClientTrace(traceCtx)
	trace.WroteRequest(httptrace.WroteRequestInfo{Err: errors.New("partial write")})

	data, err := tracker.GetRequestLifecycleData(context.Background(), requestID)
	if err != nil {
		t.Fatalf("GetRequestLifecycleData failed: %v", err)
	}
	if data == nil {
		t.Fatal("expected lifecycle data")
	}
	if data.UpstreamWriteMs != nil {
		t.Fatalf("expected failed WroteRequest to leave upstream_write_ms nil, got %d", *data.UpstreamWriteMs)
	}
}

// TestUpstreamWriteMs_NilWhenCallbackNeverReceived 未收到写完回调 → 不带出，避免误写约 0ms。
func TestUpstreamWriteMs_NilWhenCallbackNeverReceived(t *testing.T) {
	tracker := newLifecyclePanelTestTracker(t)

	rlm := NewRequestLifecycleManager(tracker, nil, "req-upstream-write-miss", nil)
	rlm.StartRequest("127.0.0.1", "test-agent", "POST", "/v1/responses", true)

	rlm.RecordFirstToken()

	data, err := tracker.GetRequestLifecycleData(context.Background(), "req-upstream-write-miss")
	if err != nil {
		t.Fatalf("GetRequestLifecycleData failed: %v", err)
	}
	if data == nil {
		t.Fatal("expected lifecycle data in hot pool")
	}
	if data.Detail.FirstTokenMs == nil {
		t.Fatal("expected first_token_ms recorded")
	}
	if data.UpstreamWriteMs != nil {
		t.Fatalf("expected upstream_write_ms nil, got %d", *data.UpstreamWriteMs)
	}
}

// TestUpstreamWriteMs_LateCallbackIgnored 首字后异常晚到回调 → 忽略且不补写。
func TestUpstreamWriteMs_LateCallbackIgnored(t *testing.T) {
	tracker := newLifecyclePanelTestTracker(t)

	rlm := NewRequestLifecycleManager(tracker, nil, "req-upstream-write-late", nil)
	rlm.StartRequest("127.0.0.1", "test-agent", "POST", "/v1/responses", true)

	rlm.RecordFirstToken()
	rlm.SetFirstTokenStartTime(time.Now()) // 晚于首字，保守忽略

	data, err := tracker.GetRequestLifecycleData(context.Background(), "req-upstream-write-late")
	if err != nil {
		t.Fatalf("GetRequestLifecycleData failed: %v", err)
	}
	if data == nil {
		t.Fatal("expected lifecycle data in hot pool")
	}
	if data.UpstreamWriteMs != nil {
		t.Fatalf("expected upstream_write_ms nil after late callback, got %d", *data.UpstreamWriteMs)
	}
}

// TestRouteDecisionAt_IsRequestScoped 两个请求交错更新时，各自只能持久化自己的决策快照。
func TestRouteDecisionAt_IsRequestScoped(t *testing.T) {
	tracker := newLifecyclePanelTestTracker(t)
	ctx := context.Background()

	requestA := NewRequestLifecycleManager(tracker, nil, "req-route-a", nil)
	requestB := NewRequestLifecycleManager(tracker, nil, "req-route-b", nil)
	requestA.StartRequest("127.0.0.1", "test-agent", "POST", "/v1/messages", true)
	requestB.StartRequest("127.0.0.1", "test-agent", "POST", "/v1/messages", true)

	decisionA := time.Now()
	requestA.SetEndpointAttempt("fallback-a", 1)
	requestA.SetRouteDecision(endpoint.RouteOverrideState{
		Mode:           endpoint.RouteModeManualPreferred,
		EndpointName:   "preferred-a",
		LastDecisionAt: decisionA,
	}, "fallback-a")

	decisionB := decisionA.Add(25 * time.Millisecond)
	requestB.SetEndpointAttempt("endpoint-b", 2)
	requestB.SetRouteDecision(endpoint.RouteOverrideState{
		Mode:           endpoint.RouteModeAuto,
		LastDecisionAt: decisionB,
	}, "endpoint-b")

	// B 后写，A 再更新状态；A 仍必须使用自己的请求级决策时间。
	requestB.UpdateStatus("forwarding", 0, 0)
	requestA.UpdateStatus("forwarding", 0, 0)

	dataA, err := tracker.GetRequestLifecycleData(ctx, "req-route-a")
	if err != nil {
		t.Fatalf("GetRequestLifecycleData A failed: %v", err)
	}
	dataB, err := tracker.GetRequestLifecycleData(ctx, "req-route-b")
	if err != nil {
		t.Fatalf("GetRequestLifecycleData B failed: %v", err)
	}
	if dataA == nil || dataA.Detail.RouteDecisionAt == nil || !dataA.Detail.RouteDecisionAt.Equal(decisionA) {
		t.Fatalf("expected request A decision %s, got %+v", decisionA, dataA)
	}
	if dataB == nil || dataB.Detail.RouteDecisionAt == nil || !dataB.Detail.RouteDecisionAt.Equal(decisionB) {
		t.Fatalf("expected request B decision %s, got %+v", decisionB, dataB)
	}
	if dataA.Detail.RouteMode != endpoint.RouteModeManualPreferred ||
		dataA.Detail.RequestedEndpoint != "preferred-a" ||
		dataA.Detail.EffectiveEndpoint != "fallback-a" ||
		dataA.Detail.FallbackReason != "manual_preferred_fallback" {
		t.Fatalf("unexpected request A route diagnostics: %+v", dataA.Detail)
	}
}
