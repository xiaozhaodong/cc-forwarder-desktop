package tracking

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSourceRoutingTracker(t *testing.T) *UsageTracker {
	t.Helper()

	cfg := &Config{
		Enabled:         true,
		DatabasePath:    ":memory:",
		BufferSize:      32,
		BatchSize:       8,
		FlushInterval:   50 * time.Millisecond,
		MaxRetry:        3,
		CleanupInterval: 24 * time.Hour,
		RetentionDays:   30,
	}

	tracker, err := NewUsageTracker(cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = tracker.Close()
	})
	return tracker
}

func insertEndpointRequestLog(t *testing.T, tracker *UsageTracker, requestID string, startTime time.Time) {
	t.Helper()
	db := tracker.GetWriteDB()
	require.NotNil(t, db)

	_, err := db.Exec(`
		INSERT INTO request_logs (
			request_id, start_time, status, upstream_type,
			endpoint_name, group_name, model_name
		) VALUES (?, ?, 'completed', 'endpoint', ?, ?, ?)`,
		requestID,
		startTime.Format("2006-01-02 15:04:05"),
		"ep-"+requestID,
		"grp-"+requestID,
		"gpt-4.1",
	)
	require.NoError(t, err)
}

func insertLegacyAccountRequestLog(t *testing.T, tracker *UsageTracker, requestID string, startTime time.Time) {
	t.Helper()
	db := tracker.GetWriteDB()
	require.NotNil(t, db)

	_, err := db.Exec(`
		INSERT INTO request_logs (
			request_id, start_time, status, upstream_type,
			upstream_source_name, upstream_name, model_name
		) VALUES (?, ?, 'completed', 'account', ?, ?, ?)`,
		requestID,
		startTime.Format("2006-01-02 15:04:05"),
		"legacy-src",
		"legacy-acc",
		"gpt-4.1",
	)
	require.NoError(t, err)
}

func insertAccountRequestLog(t *testing.T, tracker *UsageTracker, requestID string, startTime time.Time, sourceName, accountName string) {
	t.Helper()
	db := tracker.GetWriteDB()
	require.NotNil(t, db)

	_, err := db.Exec(`
		INSERT INTO account_request_logs (
			request_id, account_id, source_name, account_name,
			start_time, status, model_name
		) VALUES (?, NULL, ?, ?, ?, 'completed', ?)`,
		requestID,
		sourceName,
		accountName,
		startTime.Format("2006-01-02 15:04:05"),
		"gpt-4.1",
	)
	require.NoError(t, err)
}

func requestIDs(details []RequestDetail) map[string]struct{} {
	out := make(map[string]struct{}, len(details))
	for _, d := range details {
		out[d.RequestID] = struct{}{}
	}
	return out
}

func TestQueryRequestDetails_AccountSourceUsesAccountTableOnly(t *testing.T) {
	tracker := newSourceRoutingTracker(t)
	now := time.Now()

	insertAccountRequestLog(t, tracker, "req-account-table", now.Add(-2*time.Minute), "pool-a", "acc-a")
	insertLegacyAccountRequestLog(t, tracker, "req-account-legacy-in-request-logs", now.Add(-1*time.Minute))
	insertEndpointRequestLog(t, tracker, "req-endpoint-table", now.Add(-30*time.Second))

	ctx := context.Background()

	accountDetails, err := tracker.QueryRequestDetails(ctx, &QueryOptions{
		UpstreamType: "account",
		Limit:        20,
	})
	require.NoError(t, err)
	require.Len(t, accountDetails, 1)
	assert.Equal(t, "req-account-table", accountDetails[0].RequestID)
	assert.Equal(t, "account", accountDetails[0].UpstreamType)
	assert.Equal(t, "pool-a", accountDetails[0].UpstreamSourceName)
	assert.Equal(t, "acc-a", accountDetails[0].UpstreamName)

	accountCount, err := tracker.CountRequestDetails(ctx, &QueryOptions{
		UpstreamType: "account",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, accountCount)

	allDetails, err := tracker.QueryRequestDetails(ctx, &QueryOptions{
		UpstreamType: "",
		Limit:        20,
	})
	require.NoError(t, err)
	assert.Len(t, allDetails, 2)
	allIDs := requestIDs(allDetails)
	_, hasEndpoint := allIDs["req-endpoint-table"]
	_, hasAccount := allIDs["req-account-table"]
	_, hasLegacy := allIDs["req-account-legacy-in-request-logs"]
	assert.True(t, hasEndpoint)
	assert.True(t, hasAccount)
	assert.False(t, hasLegacy, "all view should not read account rows from request_logs")

	allCount, err := tracker.CountRequestDetails(ctx, &QueryOptions{
		UpstreamType: "",
	})
	require.NoError(t, err)
	assert.Equal(t, 2, allCount)
}

func TestQueryRequestDetailsWithHotPool_AccountSourceDedupAndPagination(t *testing.T) {
	tracker := newSourceRoutingTracker(t)
	now := time.Now()

	insertAccountRequestLog(t, tracker, "req-account-db", now.Add(-5*time.Minute), "pool-db", "acc-db")
	insertAccountRequestLog(t, tracker, "req-account-hot", now.Add(-4*time.Minute), "pool-db", "acc-hot")

	tracker.RecordRequestStart("req-account-hot", "127.0.0.1", "codex-cli", "POST", "/v1/responses", true)
	upstreamType := "account"
	sourceName := "pool-hot"
	accountName := "acc-hot"
	accountID := int64(9001)
	status := "processing"
	httpStatus := 200
	tracker.RecordRequestUpdate("req-account-hot", UpdateOptions{
		UpstreamType:       &upstreamType,
		UpstreamSourceName: &sourceName,
		UpstreamName:       &accountName,
		UpstreamID:         &accountID,
		Status:             &status,
		HttpStatus:         &httpStatus,
	})

	ctx := context.Background()
	details, total, err := tracker.QueryRequestDetailsWithHotPool(ctx, &QueryOptions{
		UpstreamType: "account",
		Limit:        10,
		Offset:       0,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total, "hot pool + db overlap should be de-duplicated")
	assert.Len(t, details, 2)
	ids := requestIDs(details)
	_, hasHot := ids["req-account-hot"]
	_, hasDB := ids["req-account-db"]
	assert.True(t, hasHot)
	assert.True(t, hasDB)

	page2, total2, err := tracker.QueryRequestDetailsWithHotPool(ctx, &QueryOptions{
		UpstreamType: "account",
		Limit:        1,
		Offset:       1,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total2)
	require.Len(t, page2, 1)
	assert.Equal(t, "req-account-db", page2[0].RequestID)
}
