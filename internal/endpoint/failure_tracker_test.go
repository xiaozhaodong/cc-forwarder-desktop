package endpoint

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func setupDebugLogger(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	old := slog.Default()
	slog.SetDefault(logger)

	restore := func() {
		slog.SetDefault(old)
	}

	return &buf, restore
}

func TestFailureTrackerRecordSuccess_LogsOnlyWhenFailuresExist(t *testing.T) {
	buf, restore := setupDebugLogger(t)
	defer restore()

	tracker := NewFailureTracker(true, time.Minute, 3)
	endpointName := "ep-1"

	// 无失败记录时，不应打印“清空失败记录”
	tracker.RecordSuccess(endpointName)
	if strings.Contains(buf.String(), "端点成功，清空失败记录") {
		t.Fatalf("expected no clear log when no failures exist, got logs: %s", buf.String())
	}

	buf.Reset()

	// 有失败记录时，应打印“清空失败记录”
	count := tracker.RecordFailure(endpointName)
	if count != 1 {
		t.Fatalf("expected failure count=1, got %d", count)
	}

	tracker.RecordSuccess(endpointName)
	logs := buf.String()
	if !strings.Contains(logs, "端点成功，清空失败记录") {
		t.Fatalf("expected clear log when failures exist, got logs: %s", logs)
	}

	if len(tracker.GetStats()) != 0 {
		t.Fatalf("expected stats to be empty after success clear, got %v", tracker.GetStats())
	}
}

func TestFailureTrackerRecordSuccess_DoesNotLogForExpiredFailures(t *testing.T) {
	buf, restore := setupDebugLogger(t)
	defer restore()

	tracker := NewFailureTracker(true, 100*time.Millisecond, 3)
	endpointName := "ep-expired"

	// 人工注入过期失败记录
	tracker.endpointFailures[endpointName] = []time.Time{
		time.Now().Add(-2 * time.Second),
	}

	tracker.RecordSuccess(endpointName)

	if strings.Contains(buf.String(), "端点成功，清空失败记录") {
		t.Fatalf("expected no clear log when only expired failures exist, got logs: %s", buf.String())
	}

	if _, exists := tracker.endpointFailures[endpointName]; exists {
		t.Fatalf("expected expired failure records to be cleaned")
	}
}
