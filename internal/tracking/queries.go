package tracking

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type requestQuerySource string

const (
	requestQuerySourceAll      requestQuerySource = "all"
	requestQuerySourceEndpoint requestQuerySource = "endpoint"
	requestQuerySourceAccount  requestQuerySource = "account"
	requestQuerySourceRaw      requestQuerySource = "raw"
)

const requestDetailsSelectBase = `SELECT id, request_id,
	COALESCE(client_ip, '') as client_ip,
	COALESCE(user_agent, '') as user_agent,
	method, path, start_time, end_time, duration_ms, first_token_ms,
	COALESCE(channel, '') as channel,
	COALESCE(endpoint_name, '') as endpoint_name,
	COALESCE(group_name, '') as group_name,
	COALESCE(upstream_type, 'endpoint') as upstream_type,
	COALESCE(upstream_source_name, '') as upstream_source_name,
	COALESCE(upstream_name, '') as upstream_name,
	COALESCE(upstream_id, 0) as upstream_id,
	COALESCE(route_mode, 'auto') as route_mode,
	COALESCE(requested_endpoint, '') as requested_endpoint,
	COALESCE(effective_endpoint, '') as effective_endpoint,
	COALESCE(fallback_reason, '') as fallback_reason,
	route_decision_at,
	COALESCE(model_name, '') as model_name,
	COALESCE(is_streaming, false) as is_streaming,
	status, http_status_code, retry_count,
	COALESCE(failure_reason, '') as failure_reason,
	COALESCE(last_failure_reason, '') as last_failure_reason,
	COALESCE(cancel_reason, '') as cancel_reason,
	input_tokens, output_tokens,
	cache_creation_tokens, COALESCE(cache_creation_5m_tokens, 0) as cache_creation_5m_tokens, COALESCE(cache_creation_1h_tokens, 0) as cache_creation_1h_tokens,
	cache_read_tokens,
	input_cost_usd, output_cost_usd, cache_creation_cost_usd,
	cache_read_cost_usd, total_cost_usd,
	created_at, updated_at
	FROM request_logs WHERE 1=1`

func resolveRequestQuerySource(upstreamType string) requestQuerySource {
	switch upstreamType {
	case "", "all":
		return requestQuerySourceAll
	case "endpoint":
		return requestQuerySourceEndpoint
	case "account":
		return requestQuerySourceAccount
	default:
		return requestQuerySourceRaw
	}
}

func appendRequestStatusFilter(query string, args []interface{}, status string) (string, []interface{}) {
	// v3.5.0 状态机重构：状态过滤兼容旧状态
	switch status {
	case "completed":
		query += " AND status = ?"
		args = append(args, "completed")
	case "failed":
		query += " AND status IN ('failed', 'error', 'auth_error', 'rate_limited', 'server_error', 'network_error', 'stream_error', 'timeout')"
	case "processing":
		query += " AND status = ?"
		args = append(args, "processing")
	case "cancelled":
		query += " AND status = ?"
		args = append(args, "cancelled")
	case "suspended":
		query += " AND status = ?"
		args = append(args, "suspended")
	case "pending":
		query += " AND status = ?"
		args = append(args, "pending")
	case "forwarding":
		query += " AND status = ?"
		args = append(args, "forwarding")
	case "retry":
		query += " AND status = ?"
		args = append(args, "retry")
	default:
		query += " AND status = ?"
		args = append(args, status)
	}
	return query, args
}

func appendRequestQueryFilters(query string, args []interface{}, opts *QueryOptions, source requestQuerySource) (string, []interface{}) {
	if opts == nil {
		return query, args
	}

	if opts.StartDate != nil {
		query += " AND start_time >= ?"
		args = append(args, opts.StartDate.Format("2006-01-02 15:04:05-07:00"))
	}
	if opts.EndDate != nil {
		query += " AND start_time <= ?"
		args = append(args, opts.EndDate.Format("2006-01-02 15:04:05-07:00"))
	}
	if opts.ModelName != "" {
		query += " AND model_name = ?"
		args = append(args, opts.ModelName)
	}

	if opts.Channel != "" {
		query += " AND channel = ?"
		args = append(args, opts.Channel)
	}
	if opts.EndpointName != "" {
		query += " AND endpoint_name = ?"
		args = append(args, opts.EndpointName)
	}
	if opts.GroupName != "" {
		query += " AND group_name = ?"
		args = append(args, opts.GroupName)
	}

	switch source {
	case requestQuerySourceEndpoint:
		query += " AND COALESCE(upstream_type, 'endpoint') = 'endpoint'"
	case requestQuerySourceAccount:
		query += " AND COALESCE(upstream_type, 'endpoint') = 'account'"
	case requestQuerySourceRaw:
		if opts.UpstreamType != "" {
			query += " AND COALESCE(upstream_type, 'endpoint') = ?"
			args = append(args, opts.UpstreamType)
		}
	}

	if opts.Status != "" {
		query, args = appendRequestStatusFilter(query, args, opts.Status)
	}

	return query, args
}

func requestDetailsBaseSelectForSource(source requestQuerySource) string {
	return requestDetailsSelectBase
}

func requestCountBaseQueryForSource(source requestQuerySource) string {
	return "SELECT COUNT(*) FROM request_logs WHERE 1=1"
}

func appendRequestIDInFilter(query string, args []interface{}, requestIDs []string) (string, []interface{}) {
	if len(requestIDs) == 0 {
		query += " AND 1=0"
		return query, args
	}
	placeholders := strings.Repeat("?,", len(requestIDs))
	placeholders = strings.TrimSuffix(placeholders, ",")
	query += " AND request_id IN (" + placeholders + ")"
	for _, id := range requestIDs {
		args = append(args, id)
	}
	return query, args
}

func (ut *UsageTracker) scanRequestDetailsRows(rows *sql.Rows) ([]RequestDetail, error) {
	var details []RequestDetail
	for rows.Next() {
		var detail RequestDetail
		err := rows.Scan(
			&detail.ID, &detail.RequestID,
			&detail.ClientIP, &detail.UserAgent, &detail.Method, &detail.Path,
			&detail.StartTime, &detail.EndTime, &detail.DurationMs, &detail.FirstTokenMs,
			&detail.Channel, &detail.EndpointName, &detail.GroupName,
			&detail.UpstreamType, &detail.UpstreamSourceName, &detail.UpstreamName, &detail.UpstreamID,
			&detail.RouteMode, &detail.RequestedEndpoint, &detail.EffectiveEndpoint, &detail.FallbackReason, &detail.RouteDecisionAt,
			&detail.ModelName, &detail.IsStreaming,
			&detail.Status, &detail.HTTPStatusCode, &detail.RetryCount,
			&detail.FailureReason, &detail.LastFailureReason, &detail.CancelReason,
			&detail.InputTokens, &detail.OutputTokens,
			&detail.CacheCreationTokens, &detail.CacheCreation5mTokens, &detail.CacheCreation1hTokens, &detail.CacheReadTokens,
			&detail.InputCostUSD, &detail.OutputCostUSD,
			&detail.CacheCreationCostUSD, &detail.CacheReadCostUSD, &detail.TotalCostUSD,
			&detail.CreatedAt, &detail.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request detail: %w", err)
		}
		details = append(details, detail)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating request detail rows: %w", err)
	}
	return details, nil
}

func (ut *UsageTracker) queryRequestDetailsBySource(ctx context.Context, opts *QueryOptions, source requestQuerySource) ([]RequestDetail, error) {
	query := requestDetailsBaseSelectForSource(source)
	args := make([]interface{}, 0, 16)
	query, args = appendRequestQueryFilters(query, args, opts, source)
	query += " ORDER BY start_time DESC"

	if opts != nil {
		if opts.Limit > 0 {
			query += " LIMIT ?"
			args = append(args, opts.Limit)
		}
		if opts.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, opts.Offset)
		}
	}

	rows, err := ut.readDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query request details: %w", err)
	}
	defer rows.Close()

	return ut.scanRequestDetailsRows(rows)
}

func (ut *UsageTracker) countRequestDetailsBySource(ctx context.Context, opts *QueryOptions, source requestQuerySource) (int, error) {
	query := requestCountBaseQueryForSource(source)
	args := make([]interface{}, 0, 16)
	query, args = appendRequestQueryFilters(query, args, opts, source)

	var count int
	if err := ut.readDB.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count request details: %w", err)
	}
	return count, nil
}

func (ut *UsageTracker) countRequestDetailsBySourceAndRequestIDs(ctx context.Context, opts *QueryOptions, source requestQuerySource, requestIDs []string) (int, error) {
	query := requestCountBaseQueryForSource(source)
	args := make([]interface{}, 0, 16+len(requestIDs))
	query, args = appendRequestQueryFilters(query, args, opts, source)
	query, args = appendRequestIDInFilter(query, args, requestIDs)

	var count int
	if err := ut.readDB.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count request details by request IDs: %w", err)
	}
	return count, nil
}

func (ut *UsageTracker) countRequestDetailsOverlapsByRequestIDs(ctx context.Context, opts *QueryOptions, requestIDs []string) (int, error) {
	if ut.readDB == nil {
		return 0, fmt.Errorf("read database not initialized")
	}
	if len(requestIDs) == 0 {
		return 0, nil
	}

	if opts == nil {
		opts = &QueryOptions{}
	}

	source := resolveRequestQuerySource(opts.UpstreamType)
	switch source {
	case requestQuerySourceAll, requestQuerySourceEndpoint, requestQuerySourceAccount, requestQuerySourceRaw:
		return ut.countRequestDetailsBySourceAndRequestIDs(ctx, opts, source, requestIDs)
	default:
		return ut.countRequestDetailsBySourceAndRequestIDs(ctx, opts, requestQuerySourceRaw, requestIDs)
	}
}

// QueryOptions represents options for querying usage data
type QueryOptions struct {
	StartDate    *time.Time
	EndDate      *time.Time
	ModelName    string
	Channel      string
	EndpointName string
	GroupName    string
	UpstreamType string
	Status       string
	Limit        int
	Offset       int
}

// UsageSummary represents a summary of usage data
type UsageSummary struct {
	Date         string `json:"date"`
	ModelName    string `json:"model_name"`
	EndpointName string `json:"endpoint_name"`
	GroupName    string `json:"group_name"`
	RequestCount int    `json:"request_count"`
	SuccessCount int    `json:"success_count"`
	ErrorCount   int    `json:"error_count"`

	TotalInputTokens         int64   `json:"total_input_tokens"`
	TotalOutputTokens        int64   `json:"total_output_tokens"`
	TotalCacheCreationTokens int64   `json:"total_cache_creation_tokens"`
	TotalCacheReadTokens     int64   `json:"total_cache_read_tokens"`
	TotalCostUSD             float64 `json:"total_cost_usd"`

	AvgDurationMs float64   `json:"avg_duration_ms"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// RequestDetail represents a detailed request record
type RequestDetail struct {
	ID        int64  `json:"id"`
	RequestID string `json:"request_id"`
	ClientIP  string `json:"client_ip"`
	UserAgent string `json:"user_agent"`
	Method    string `json:"method"`
	Path      string `json:"path"`

	StartTime    time.Time  `json:"start_time"`
	EndTime      *time.Time `json:"end_time"`
	DurationMs   *int64     `json:"duration_ms"`
	FirstTokenMs *int64     `json:"first_token_ms"`

	Channel            string     `json:"channel"` // 渠道标签
	EndpointName       string     `json:"endpoint_name"`
	GroupName          string     `json:"group_name"`
	UpstreamType       string     `json:"upstream_type"`
	UpstreamSourceName string     `json:"upstream_source_name"`
	UpstreamName       string     `json:"upstream_name"`
	UpstreamID         int64      `json:"upstream_id"`
	RouteMode          string     `json:"route_mode"`
	RequestedEndpoint  string     `json:"requested_endpoint"`
	EffectiveEndpoint  string     `json:"effective_endpoint"`
	FallbackReason     string     `json:"fallback_reason"`
	RouteDecisionAt    *time.Time `json:"route_decision_at"`
	ModelName          string     `json:"model_name"`
	IsStreaming        bool       `json:"is_streaming"` // 是否为流式请求

	Status         string `json:"status"`
	HTTPStatusCode *int   `json:"http_status_code"`
	RetryCount     int    `json:"retry_count"`

	// v3.5.0状态机重构新增字段 - 错误原因分离
	FailureReason     string `json:"failure_reason"`      // 失败原因类型
	LastFailureReason string `json:"last_failure_reason"` // 最后一次失败的详细信息
	CancelReason      string `json:"cancel_reason"`       // 取消原因

	InputTokens           int64 `json:"input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	CacheCreationTokens   int64 `json:"cache_creation_tokens"`    // 总缓存创建（向后兼容）
	CacheCreation5mTokens int64 `json:"cache_creation_5m_tokens"` // v5.0.1: 5分钟缓存
	CacheCreation1hTokens int64 `json:"cache_creation_1h_tokens"` // v5.0.1: 1小时缓存
	CacheReadTokens       int64 `json:"cache_read_tokens"`

	InputCostUSD         float64 `json:"input_cost_usd"`
	OutputCostUSD        float64 `json:"output_cost_usd"`
	CacheCreationCostUSD float64 `json:"cache_creation_cost_usd"`
	CacheReadCostUSD     float64 `json:"cache_read_cost_usd"`
	TotalCostUSD         float64 `json:"total_cost_usd"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UsageStats represents aggregated usage statistics
type UsageStats struct {
	Period        string  `json:"period"`
	TotalRequests int     `json:"total_requests"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDuration   float64 `json:"avg_duration_ms"`
	TotalCost     float64 `json:"total_cost_usd"`
}

// AggregatedRequestStats 表示带筛选条件的请求聚合统计。
type AggregatedRequestStats struct {
	TotalRequests   int64
	SuccessRequests int64
	FailedRequests  int64
	TotalTokens     int64
	TotalCostUSD    float64
	TotalDurationMs int64
	DurationCount   int64
}

const aggregatedRequestStatsSelectBase = `SELECT
	COUNT(*) as total_requests,
	COALESCE(SUM(CASE WHEN status IN ('completed', 'processing') THEN 1 ELSE 0 END), 0) as success_requests,
	COALESCE(SUM(CASE WHEN status IN ('failed', 'error', 'auth_error', 'rate_limited', 'server_error', 'network_error', 'stream_error', 'timeout') THEN 1 ELSE 0 END), 0) as failed_requests,
	COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
	COALESCE(SUM(total_cost_usd), 0.0) as total_cost_usd,
	COALESCE(SUM(CASE WHEN duration_ms IS NOT NULL AND duration_ms > 0 THEN duration_ms ELSE 0 END), 0) as total_duration_ms,
	COALESCE(SUM(CASE WHEN duration_ms IS NOT NULL AND duration_ms > 0 THEN 1 ELSE 0 END), 0) as duration_count
	FROM request_logs WHERE 1=1`

func aggregatedRequestStatsBaseQueryForSource(source requestQuerySource) string {
	return aggregatedRequestStatsSelectBase
}

func (stats *AggregatedRequestStats) add(other AggregatedRequestStats) {
	stats.TotalRequests += other.TotalRequests
	stats.SuccessRequests += other.SuccessRequests
	stats.FailedRequests += other.FailedRequests
	stats.TotalTokens += other.TotalTokens
	stats.TotalCostUSD += other.TotalCostUSD
	stats.TotalDurationMs += other.TotalDurationMs
	stats.DurationCount += other.DurationCount
}

func (stats *AggregatedRequestStats) subtract(other AggregatedRequestStats) {
	stats.TotalRequests -= other.TotalRequests
	stats.SuccessRequests -= other.SuccessRequests
	stats.FailedRequests -= other.FailedRequests
	stats.TotalTokens -= other.TotalTokens
	stats.TotalCostUSD -= other.TotalCostUSD
	stats.TotalDurationMs -= other.TotalDurationMs
	stats.DurationCount -= other.DurationCount

	if stats.TotalRequests < 0 {
		stats.TotalRequests = 0
	}
	if stats.SuccessRequests < 0 {
		stats.SuccessRequests = 0
	}
	if stats.FailedRequests < 0 {
		stats.FailedRequests = 0
	}
	if stats.TotalTokens < 0 {
		stats.TotalTokens = 0
	}
	if stats.TotalCostUSD < 0 {
		stats.TotalCostUSD = 0
	}
	if stats.TotalDurationMs < 0 {
		stats.TotalDurationMs = 0
	}
	if stats.DurationCount < 0 {
		stats.DurationCount = 0
	}
}

func aggregatedStatsFromRequestDetails(details []RequestDetail) AggregatedRequestStats {
	var stats AggregatedRequestStats
	for _, req := range details {
		stats.TotalRequests++
		switch req.Status {
		case "completed", "processing":
			stats.SuccessRequests++
		case "failed", "error", "auth_error", "rate_limited", "server_error", "network_error", "stream_error", "timeout":
			stats.FailedRequests++
		}
		stats.TotalTokens += req.InputTokens + req.OutputTokens + req.CacheCreationTokens + req.CacheReadTokens
		stats.TotalCostUSD += req.TotalCostUSD
		if req.DurationMs != nil && *req.DurationMs > 0 {
			stats.TotalDurationMs += *req.DurationMs
			stats.DurationCount++
		}
	}
	return stats
}

func (ut *UsageTracker) queryAggregatedRequestStatsBySource(ctx context.Context, opts *QueryOptions, source requestQuerySource) (*AggregatedRequestStats, error) {
	query := aggregatedRequestStatsBaseQueryForSource(source)
	args := make([]interface{}, 0, 16)
	query, args = appendRequestQueryFilters(query, args, opts, source)

	var stats AggregatedRequestStats
	err := ut.readDB.QueryRowContext(ctx, query, args...).Scan(
		&stats.TotalRequests,
		&stats.SuccessRequests,
		&stats.FailedRequests,
		&stats.TotalTokens,
		&stats.TotalCostUSD,
		&stats.TotalDurationMs,
		&stats.DurationCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query aggregated request stats: %w", err)
	}
	return &stats, nil
}

func (ut *UsageTracker) queryAggregatedRequestStatsBySourceAndRequestIDs(ctx context.Context, opts *QueryOptions, source requestQuerySource, requestIDs []string) (*AggregatedRequestStats, error) {
	if len(requestIDs) == 0 {
		return &AggregatedRequestStats{}, nil
	}

	query := aggregatedRequestStatsBaseQueryForSource(source)
	args := make([]interface{}, 0, 16+len(requestIDs))
	query, args = appendRequestQueryFilters(query, args, opts, source)
	query, args = appendRequestIDInFilter(query, args, requestIDs)

	var stats AggregatedRequestStats
	err := ut.readDB.QueryRowContext(ctx, query, args...).Scan(
		&stats.TotalRequests,
		&stats.SuccessRequests,
		&stats.FailedRequests,
		&stats.TotalTokens,
		&stats.TotalCostUSD,
		&stats.TotalDurationMs,
		&stats.DurationCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query aggregated request stats by request IDs: %w", err)
	}
	return &stats, nil
}

// QueryAggregatedRequestStatsWithHotPool 查询带筛选条件的请求聚合统计（数据库 + 热池）。
func (ut *UsageTracker) QueryAggregatedRequestStatsWithHotPool(ctx context.Context, opts *QueryOptions) (*AggregatedRequestStats, error) {
	if ut.readDB == nil {
		return nil, fmt.Errorf("read database not initialized")
	}
	if opts == nil {
		opts = &QueryOptions{}
	}

	source := resolveRequestQuerySource(opts.UpstreamType)
	var stats AggregatedRequestStats

	switch source {
	case requestQuerySourceAll, requestQuerySourceEndpoint, requestQuerySourceAccount, requestQuerySourceRaw:
		sourceStats, err := ut.queryAggregatedRequestStatsBySource(ctx, opts, source)
		if err != nil {
			return nil, err
		}
		stats.add(*sourceStats)
	default:
		sourceStats, err := ut.queryAggregatedRequestStatsBySource(ctx, opts, requestQuerySourceRaw)
		if err != nil {
			return nil, err
		}
		stats.add(*sourceStats)
	}

	hotPoolRequests := ut.getFilteredHotPoolRequests(opts)
	if len(hotPoolRequests) == 0 {
		return &stats, nil
	}

	hotPoolIDs := make(map[string]struct{}, len(hotPoolRequests))
	uniqueHotPoolRequests := make([]RequestDetail, 0, len(hotPoolRequests))
	hotPoolRequestIDs := make([]string, 0, len(hotPoolRequests))
	for _, req := range hotPoolRequests {
		if _, exists := hotPoolIDs[req.RequestID]; exists {
			continue
		}
		hotPoolIDs[req.RequestID] = struct{}{}
		uniqueHotPoolRequests = append(uniqueHotPoolRequests, req)
		hotPoolRequestIDs = append(hotPoolRequestIDs, req.RequestID)
	}

	switch source {
	case requestQuerySourceAll, requestQuerySourceEndpoint, requestQuerySourceAccount, requestQuerySourceRaw:
		overlapStats, err := ut.queryAggregatedRequestStatsBySourceAndRequestIDs(ctx, opts, source, hotPoolRequestIDs)
		if err != nil {
			return nil, err
		}
		stats.subtract(*overlapStats)
	default:
		overlapStats, err := ut.queryAggregatedRequestStatsBySourceAndRequestIDs(ctx, opts, requestQuerySourceRaw, hotPoolRequestIDs)
		if err != nil {
			return nil, err
		}
		stats.subtract(*overlapStats)
	}

	hotPoolStats := aggregatedStatsFromRequestDetails(uniqueHotPoolRequests)
	stats.add(hotPoolStats)

	return &stats, nil
}

// EndpointCostSummary represents cost summary data for an endpoint
type EndpointCostSummary struct {
	EndpointName string  `json:"endpoint_name"`
	GroupName    string  `json:"group_name"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	RequestCount int     `json:"request_count"`
	SuccessCount int     `json:"success_count"`

	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`

	InputCostUSD         float64 `json:"input_cost_usd"`
	OutputCostUSD        float64 `json:"output_cost_usd"`
	CacheCreationCostUSD float64 `json:"cache_creation_cost_usd"`
	CacheReadCostUSD     float64 `json:"cache_read_cost_usd"`
}

// GetDB returns the read database connection for external queries (读写分离：返回读连接)
func (ut *UsageTracker) GetDB() *sql.DB {
	return ut.readDB
}

// GetReadDB returns the read database connection
func (ut *UsageTracker) GetReadDB() *sql.DB {
	return ut.readDB
}

// GetWriteDB returns the write database connection (仅用于特殊情况)
func (ut *UsageTracker) GetWriteDB() *sql.DB {
	return ut.writeDB
}

// QueryUsageSummary queries usage summary data
func (ut *UsageTracker) QueryUsageSummary(ctx context.Context, opts *QueryOptions) ([]UsageSummary, error) {
	if ut.readDB == nil {
		return nil, fmt.Errorf("read database not initialized")
	}
	if opts == nil {
		opts = &QueryOptions{}
	}

	query := `SELECT date, model_name, endpoint_name, 
		COALESCE(group_name, '') as group_name,
		request_count, success_count, error_count,
		total_input_tokens, total_output_tokens, 
		total_cache_creation_tokens, total_cache_read_tokens,
		total_cost_usd, COALESCE(avg_duration_ms, 0.0) as avg_duration_ms, created_at, updated_at
		FROM usage_summary WHERE 1=1`

	var args []interface{}

	if opts.StartDate != nil {
		query += " AND date >= ?"
		args = append(args, opts.StartDate.Format("2006-01-02"))
	}
	if opts.EndDate != nil {
		query += " AND date <= ?"
		args = append(args, opts.EndDate.Format("2006-01-02"))
	}
	if opts.ModelName != "" {
		query += " AND model_name = ?"
		args = append(args, opts.ModelName)
	}
	if opts.EndpointName != "" {
		query += " AND endpoint_name = ?"
		args = append(args, opts.EndpointName)
	}
	if opts.GroupName != "" {
		query += " AND group_name = ?"
		args = append(args, opts.GroupName)
	}

	query += " ORDER BY date DESC, total_cost_usd DESC"

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := ut.readDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage summary: %w", err)
	}
	defer rows.Close()

	var summaries []UsageSummary
	for rows.Next() {
		var summary UsageSummary
		err := rows.Scan(
			&summary.Date, &summary.ModelName, &summary.EndpointName, &summary.GroupName,
			&summary.RequestCount, &summary.SuccessCount, &summary.ErrorCount,
			&summary.TotalInputTokens, &summary.TotalOutputTokens,
			&summary.TotalCacheCreationTokens, &summary.TotalCacheReadTokens,
			&summary.TotalCostUSD, &summary.AvgDurationMs,
			&summary.CreatedAt, &summary.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage summary: %w", err)
		}
		summaries = append(summaries, summary)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage summary rows: %w", err)
	}

	return summaries, nil
}

// QueryRequestDetails queries detailed request records
func (ut *UsageTracker) QueryRequestDetails(ctx context.Context, opts *QueryOptions) ([]RequestDetail, error) {
	if ut.readDB == nil {
		return nil, fmt.Errorf("read database not initialized")
	}
	if opts == nil {
		opts = &QueryOptions{}
	}

	source := resolveRequestQuerySource(opts.UpstreamType)
	switch source {
	case requestQuerySourceAll, requestQuerySourceEndpoint, requestQuerySourceAccount, requestQuerySourceRaw:
		return ut.queryRequestDetailsBySource(ctx, opts, source)
	default:
		return ut.queryRequestDetailsBySource(ctx, opts, requestQuerySourceRaw)
	}
}

// QueryUsageStats queries aggregated usage statistics
func (ut *UsageTracker) QueryUsageStats(ctx context.Context, period string) (*UsageStats, error) {
	if ut.readDB == nil {
		return nil, fmt.Errorf("read database not initialized")
	}

	// Calculate date range based on period
	endDate := time.Now()
	var startDate time.Time

	switch period {
	case "1d":
		startDate = endDate.AddDate(0, 0, -1)
	case "7d":
		startDate = endDate.AddDate(0, 0, -7)
	case "30d":
		startDate = endDate.AddDate(0, 0, -30)
	case "90d":
		startDate = endDate.AddDate(0, 0, -90)
	default:
		startDate = endDate.AddDate(0, 0, -7) // default to 7 days
	}

	query := `SELECT 
		COUNT(*) as total_requests,
		CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) * 100 as success_rate,
		AVG(CASE WHEN duration_ms IS NOT NULL THEN duration_ms ELSE 0 END) as avg_duration,
		SUM(total_cost_usd) as total_cost
		FROM request_logs 
		WHERE start_time >= ? AND start_time <= ?`

	var stats UsageStats
	stats.Period = period

	err := ut.readDB.QueryRowContext(ctx, query, startDate, endDate).Scan(
		&stats.TotalRequests,
		&stats.SuccessRate,
		&stats.AvgDuration,
		&stats.TotalCost,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage stats: %w", err)
	}

	// 添加调试日志
	slog.Debug("Usage stats query result",
		"total_requests", stats.TotalRequests,
		"success_rate", stats.SuccessRate,
		"period", period,
		"start_date", startDate,
		"end_date", endDate)

	return &stats, nil
}

// CountRequestDetails returns the total count of request details matching the query options
func (ut *UsageTracker) CountRequestDetails(ctx context.Context, opts *QueryOptions) (int, error) {
	if ut.readDB == nil {
		return 0, fmt.Errorf("read database not initialized")
	}
	if opts == nil {
		opts = &QueryOptions{}
	}

	source := resolveRequestQuerySource(opts.UpstreamType)
	switch source {
	case requestQuerySourceAll:
		endpointCount, err := ut.countRequestDetailsBySource(ctx, opts, requestQuerySourceEndpoint)
		if err != nil {
			return 0, err
		}
		accountCount, err := ut.countRequestDetailsBySource(ctx, opts, requestQuerySourceAccount)
		if err != nil {
			return 0, err
		}
		return endpointCount + accountCount, nil
	case requestQuerySourceEndpoint, requestQuerySourceAccount, requestQuerySourceRaw:
		return ut.countRequestDetailsBySource(ctx, opts, source)
	default:
		return ut.countRequestDetailsBySource(ctx, opts, requestQuerySourceRaw)
	}
}

// GetEndpointCostsForDate queries endpoint cost summary data for a specific date
func (ut *UsageTracker) GetEndpointCostsForDate(ctx context.Context, date string) ([]EndpointCostSummary, error) {
	if ut.readDB == nil {
		return nil, fmt.Errorf("read database not initialized")
	}

	// Validate date format
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err)
	}

	slog.Debug("Querying endpoint costs for date", "date", date)

	// Convert date to time range for timezone-aware querying
	// Parse the date and create start/end times in local timezone
	startTime, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("failed to parse date: %w", err)
	}

	// Use local timezone for the date range
	location := time.Local
	startOfDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 0, 0, 0, 0, location)
	endOfDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 23, 59, 59, 999999999, location)

	query := `SELECT
		COALESCE(endpoint_name, '') as endpoint_name,
		COALESCE(group_name, '') as group_name,
		COALESCE(SUM(input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens), 0) as total_tokens,
		COALESCE(SUM(total_cost_usd), 0.0) as total_cost_usd,
		COUNT(*) as request_count,
		SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as success_count,
		COALESCE(SUM(input_tokens), 0) as input_tokens,
		COALESCE(SUM(output_tokens), 0) as output_tokens,
		COALESCE(SUM(cache_creation_tokens), 0) as cache_creation_tokens,
		COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
		COALESCE(SUM(input_cost_usd), 0.0) as input_cost_usd,
		COALESCE(SUM(output_cost_usd), 0.0) as output_cost_usd,
		COALESCE(SUM(cache_creation_cost_usd), 0.0) as cache_creation_cost_usd,
		COALESCE(SUM(cache_read_cost_usd), 0.0) as cache_read_cost_usd
		FROM request_logs
		WHERE start_time >= ? AND start_time <= ?
		GROUP BY endpoint_name, group_name
		ORDER BY total_cost_usd DESC`

	rows, err := ut.readDB.QueryContext(ctx, query, startOfDay, endOfDay)
	if err != nil {
		slog.Error("Failed to query endpoint costs", "error", err, "date", date, "start_time", startOfDay, "end_time", endOfDay)
		return nil, fmt.Errorf("failed to query endpoint costs for date %s: %w", date, err)
	}
	defer rows.Close()

	var costs []EndpointCostSummary
	for rows.Next() {
		var cost EndpointCostSummary
		err := rows.Scan(
			&cost.EndpointName, &cost.GroupName, &cost.TotalTokens, &cost.TotalCostUSD,
			&cost.RequestCount, &cost.SuccessCount,
			&cost.InputTokens, &cost.OutputTokens,
			&cost.CacheCreationTokens, &cost.CacheReadTokens,
			&cost.InputCostUSD, &cost.OutputCostUSD,
			&cost.CacheCreationCostUSD, &cost.CacheReadCostUSD,
		)
		if err != nil {
			slog.Error("Failed to scan endpoint cost row", "error", err)
			return nil, fmt.Errorf("failed to scan endpoint cost: %w", err)
		}
		costs = append(costs, cost)
	}

	if err = rows.Err(); err != nil {
		slog.Error("Error iterating endpoint cost rows", "error", err)
		return nil, fmt.Errorf("error iterating endpoint cost rows: %w", err)
	}

	slog.Debug("Successfully queried endpoint costs",
		"date", date,
		"endpoint_count", len(costs),
		"total_endpoints", len(costs))

	return costs, nil
}
