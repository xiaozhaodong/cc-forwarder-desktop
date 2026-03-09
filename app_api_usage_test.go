package main

import (
	"testing"
	"time"

	"cc-forwarder/internal/middleware"
	"cc-forwarder/internal/monitor"
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

func TestGetUsageStats_FallsBackToRuntimeForCurrentWindowWhenAggregateIsZero(t *testing.T) {
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

	if result.TotalRequests != 1 {
		t.Fatalf("expected runtime fallback to provide 1 request, got %d", result.TotalRequests)
	}
	if result.TotalTokens != 15 {
		t.Fatalf("expected runtime fallback to provide 15 tokens, got %d", result.TotalTokens)
	}
	if result.SuccessRate != 100 {
		t.Fatalf("expected runtime fallback success rate 100, got %f", result.SuccessRate)
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
