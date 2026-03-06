package tracking

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryAggregatedRequestStatsWithHotPool(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "usage.db")

	config := &Config{
		Enabled:         true,
		DatabasePath:    dbPath,
		BufferSize:      100,
		BatchSize:       10,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
	}

	tracker, err := NewUsageTracker(config)
	if err != nil {
		t.Fatalf("Failed to create usage tracker: %v", err)
	}
	defer tracker.Close()

	tracker.UpdatePricing(map[string]ModelPricing{
		"test-model": {
			Input:  1000000,
			Output: 1000000,
		},
	})

	requestID := "req-aggregated-completed"
	tracker.RecordRequestStart(requestID, "127.0.0.1", "stats-agent", "POST", "/v1/messages", false)
	tracker.RecordRequestSuccess(requestID, "test-model", &TokenUsage{
		InputTokens:  100,
		OutputTokens: 50,
	}, 200*time.Millisecond)

	activeID := "req-aggregated-hot"
	tracker.RecordRequestStart(activeID, "127.0.0.1", "stats-agent", "POST", "/v1/messages", false)
	if err := tracker.hotPool.Update(activeID, func(req *ActiveRequest) {
		req.Status = "processing"
		req.ModelName = "test-model"
		req.InputTokens = 25
		req.OutputTokens = 5
		req.TotalCostUSD = 1.25
		d := int64(150)
		req.DurationMs = d
	}); err != nil {
		t.Fatalf("UpdateActiveRequest failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	stats, err := tracker.QueryAggregatedRequestStatsWithHotPool(context.Background(), &QueryOptions{
		StartDate: func() *time.Time {
			t := time.Now().Add(-1 * time.Hour)
			return &t
		}(),
		EndDate: func() *time.Time {
			t := time.Now().Add(1 * time.Hour)
			return &t
		}(),
	})
	if err != nil {
		t.Fatalf("QueryAggregatedRequestStatsWithHotPool failed: %v", err)
	}

	if stats.TotalRequests != 2 {
		t.Fatalf("expected 2 total requests, got %d", stats.TotalRequests)
	}
	if stats.SuccessRequests != 2 {
		t.Fatalf("expected 2 success requests, got %d", stats.SuccessRequests)
	}
	if stats.FailedRequests != 0 {
		t.Fatalf("expected 0 failed requests, got %d", stats.FailedRequests)
	}
	if stats.TotalTokens != 180 {
		t.Fatalf("expected total tokens 180, got %d", stats.TotalTokens)
	}
	if stats.TotalCostUSD <= 0 {
		t.Fatalf("expected total cost to include priced records, got %f", stats.TotalCostUSD)
	}
	if stats.DurationCount == 0 {
		t.Fatal("expected duration count to be greater than zero")
	}
}
