package main

import (
	"testing"
	"time"

	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/monitor"
	timezonepolicy "cc-forwarder/internal/timezone"
	"cc-forwarder/internal/tracking"
)

func newUsageStatsTestApp(t *testing.T) *App {
	t.Helper()

	app := NewApp()
	app.monitoringMiddleware = middleware.NewMonitoringMiddleware(nil)

	tracker, err := tracking.NewUsageTracker(&tracking.Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      16,
		BatchSize:       4,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
		HotPool: &tracking.HotPoolSettings{
			Enabled: true,
		},
	})
	if err != nil {
		t.Fatalf("failed to create usage tracker: %v", err)
	}
	t.Cleanup(func() {
		_ = tracker.Close()
	})

	app.usageTracker = tracker
	return app
}

func recordRuntimeStats(app *App, tokens *monitor.TokenUsage) {
	connID := app.monitoringMiddleware.RecordRequest("endpoint-a", "127.0.0.1", "codex-cli", "POST", "/v1/messages")
	app.monitoringMiddleware.RecordResponse(connID, 200, 250*time.Millisecond, 128, "endpoint-a")
	if tokens != nil {
		app.monitoringMiddleware.GetMetrics().RecordTokenUsage(connID, "endpoint-a", tokens)
	}
}

func TestGetUsageStats_ReturnsZeroForCurrentWindowWhenAggregateIsZero(t *testing.T) {
	app := newUsageStatsTestApp(t)
	recordRuntimeStats(app, &monitor.TokenUsage{
		InputTokens:  10,
		OutputTokens: 5,
	})

	now := time.Now()
	result, err := app.GetUsageStats(UsageStatsQueryParams{
		Period:     "30d",
		SourceView: "all",
		StartDate:  now.Add(-1 * time.Hour).Format("2006-01-02T15:04:05-07:00"),
		EndDate:    now.Add(1 * time.Hour).Format("2006-01-02T15:04:05-07:00"),
	})
	if err != nil {
		t.Fatalf("GetUsageStats failed: %v", err)
	}

	if result.TotalRequests != 0 {
		t.Fatalf("expected zero aggregate to stay zero without runtime fallback, got %d requests", result.TotalRequests)
	}
	if result.TotalTokens != 0 {
		t.Fatalf("expected zero aggregate to stay zero without runtime fallback, got %d tokens", result.TotalTokens)
	}
}

func TestGetUsageStats_DoesNotFallbackToRuntimeForHistoricalWindow(t *testing.T) {
	app := newUsageStatsTestApp(t)
	recordRuntimeStats(app, &monitor.TokenUsage{
		InputTokens:  10,
		OutputTokens: 5,
	})

	now := time.Now()
	result, err := app.GetUsageStats(UsageStatsQueryParams{
		Period:     "30d",
		SourceView: "all",
		StartDate:  now.Add(-48 * time.Hour).Format("2006-01-02T15:04:05-07:00"),
		EndDate:    now.Add(-24 * time.Hour).Format("2006-01-02T15:04:05-07:00"),
	})
	if err != nil {
		t.Fatalf("GetUsageStats failed: %v", err)
	}

	if result.TotalRequests != 0 {
		t.Fatalf("expected historical zero result to stay zero, got %d", result.TotalRequests)
	}
	if result.TotalTokens != 0 {
		t.Fatalf("expected historical zero tokens, got %d", result.TotalTokens)
	}
}

func TestGetUsageStats_DoesNotFallbackToRuntimeWhenFiltered(t *testing.T) {
	app := newUsageStatsTestApp(t)
	recordRuntimeStats(app, &monitor.TokenUsage{
		InputTokens:  10,
		OutputTokens: 5,
	})

	now := time.Now()
	result, err := app.GetUsageStats(UsageStatsQueryParams{
		Period:     "30d",
		SourceView: "all",
		Model:      "gpt-5",
		StartDate:  now.Add(-1 * time.Hour).Format("2006-01-02T15:04:05-07:00"),
		EndDate:    now.Add(1 * time.Hour).Format("2006-01-02T15:04:05-07:00"),
	})
	if err != nil {
		t.Fatalf("GetUsageStats failed: %v", err)
	}

	if result.TotalRequests != 0 {
		t.Fatalf("expected filtered zero result to stay zero, got %d", result.TotalRequests)
	}
	if result.TotalTokens != 0 {
		t.Fatalf("expected filtered zero tokens, got %d", result.TotalTokens)
	}
}

func TestGetRequestsUsesConfiguredDSTDayRangeAndReturnsUTC(t *testing.T) {
	policy, err := timezonepolicy.New("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tracker, err := tracking.NewUsageTrackerWithPolicy(&tracking.Config{
		Enabled: true, DatabasePath: ":memory:", BufferSize: 16, BatchSize: 4,
		FlushInterval: time.Hour, HotPool: &tracking.HotPoolSettings{Enabled: false},
	}, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer tracker.Close()

	for _, row := range []struct{ id, start string }{
		{id: "dst-start", start: "2026-03-08T05:00:00.000000Z"},
		{id: "dst-last", start: "2026-03-09T03:59:59.999999Z"},
		{id: "dst-end-exclusive", start: "2026-03-09T04:00:00.000000Z"},
	} {
		if _, err := tracker.GetWriteDB().Exec(`INSERT INTO request_logs (
			request_id, start_time, status, request_family, upstream_type, upstream_name
		) VALUES (?, ?, 'completed', 'claude', 'endpoint', 'dst-endpoint')`, row.id, row.start); err != nil {
			t.Fatal(err)
		}
	}

	app := NewApp()
	app.timezonePolicy = policy
	app.usageTracker = tracker
	result, err := app.GetRequests(RequestQueryParams{
		StartDate: "2026-03-08T00:00", EndDate: "2026-03-09T00:00", Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || len(result.Requests) != 2 {
		t.Fatalf("DST day requests = total:%d rows:%d", result.Total, len(result.Requests))
	}
	for _, request := range result.Requests {
		if request.RequestID == "dst-end-exclusive" {
			t.Fatal("half-open end boundary was included")
		}
		if request.Timestamp == "" || request.Timestamp[len(request.Timestamp)-1] != 'Z' {
			t.Fatalf("request timestamp is not UTC: %q", request.Timestamp)
		}
	}
}
