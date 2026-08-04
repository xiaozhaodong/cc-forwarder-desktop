package tracking

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	timezonepolicy "cc-forwarder/internal/timezone"
)

type usageSummaryKey struct {
	date          string
	modelName     string
	requestFamily string
	upstreamType  string
	upstreamName  string
	upstreamID    int64
}

type usageSummaryAccumulator struct {
	UsageSummary
	durationTotal int64
	durationCount int64
}

func (ut *UsageTracker) summaryCovers(name, startDate, endDate string) bool {
	ut.summaryMu.RLock()
	defer ut.summaryMu.RUnlock()
	return ut.summaryReadyTimezone == name &&
		ut.summaryStartDate != "" && ut.summaryEndDate != "" &&
		startDate >= ut.summaryStartDate && endDate <= ut.summaryEndDate
}

func (ut *UsageTracker) setSummaryCoverage(name, startDate, endDate string) {
	ut.summaryMu.Lock()
	ut.summaryReadyTimezone = name
	ut.summaryStartDate = startDate
	ut.summaryEndDate = endDate
	ut.summaryMu.Unlock()
}

// OnTimezoneChanged 使旧时区缓存立即失效，并异步构建活动时区缓存。
func (ut *UsageTracker) OnTimezoneChanged() {
	if ut == nil || ut.timezonePolicy == nil {
		return
	}
	ut.setSummaryCoverage("", "", "")
	ut.summaryWg.Add(1)
	go func() {
		defer ut.summaryWg.Done()
		select {
		case <-ut.ctx.Done():
			return
		default:
			ut.updateUsageSummary()
		}
	}()
}

func (ut *UsageTracker) rebuildRecentUsageSummary(ctx context.Context, days int) error {
	if ut == nil || ut.readDB == nil || ut.writeDB == nil || ut.timezonePolicy == nil {
		return fmt.Errorf("usage summary dependencies are not initialized")
	}
	if days < 1 {
		days = 7
	}
	ut.summaryRebuildMu.Lock()
	defer ut.summaryRebuildMu.Unlock()

	policy := ut.timezonePolicy.Snapshot()
	timezoneName := policy.Name()
	today := policy.BusinessDate(time.Now())
	todayValue, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return err
	}
	startDate := todayValue.AddDate(0, 0, -(days - 1)).Format(time.DateOnly)
	startUTC, _, err := policy.DayRange(startDate)
	if err != nil {
		return err
	}
	_, endUTC, err := policy.DayRange(today)
	if err != nil {
		return err
	}

	summaries, err := ut.aggregateUsageSummary(ctx, startUTC, endUTC, nil, policy)
	if err != nil {
		return err
	}
	ut.writeMu.Lock()
	defer ut.writeMu.Unlock()
	tx, err := ut.writeDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_summary WHERE timezone_name = ?`, timezoneName); err != nil {
		return err
	}
	for _, summary := range summaries {
		if _, err := tx.ExecContext(ctx, `INSERT INTO usage_summary (
			timezone_name, date, model_name, request_family, upstream_type, upstream_name, upstream_id,
			request_count, success_count, error_count, total_input_tokens, total_output_tokens,
			total_cache_creation_tokens, total_cache_read_tokens, total_cost_usd, avg_duration_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			timezoneName, summary.Date, summary.ModelName, summary.RequestFamily,
			summary.UpstreamType, summary.UpstreamName, summary.UpstreamID,
			summary.RequestCount, summary.SuccessCount, summary.ErrorCount,
			summary.TotalInputTokens, summary.TotalOutputTokens,
			summary.TotalCacheCreationTokens, summary.TotalCacheReadTokens,
			summary.TotalCostUSD, summary.AvgDurationMs,
		); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_summary WHERE timezone_name <> ?`, timezoneName); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// 若重建期间活动时区再次变化，不能把旧结果标记为就绪。
	if ut.timezonePolicy.Name() == timezoneName {
		ut.setSummaryCoverage(timezoneName, startDate, today)
	}
	return nil
}

func (ut *UsageTracker) aggregateUsageSummary(ctx context.Context, startUTC, endUTC time.Time, opts *QueryOptions, policy *timezonepolicy.Policy) ([]UsageSummary, error) {
	if policy == nil || policy.Name() == "" {
		return nil, fmt.Errorf("timezone policy snapshot is not initialized")
	}
	query := `SELECT CAST(start_time AS TEXT), COALESCE(model_name, ''), COALESCE(request_family, 'other'),
		COALESCE(upstream_type, ''), COALESCE(upstream_name, ''), COALESCE(upstream_id, 0),
		status, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
		total_cost_usd, duration_ms
		FROM request_logs WHERE start_time >= ? AND start_time < ?`
	args := []any{timezonepolicy.FormatStorage(startUTC), timezonepolicy.FormatStorage(endUTC)}
	if opts != nil {
		if opts.ModelName != "" {
			query += " AND model_name = ?"
			args = append(args, opts.ModelName)
		}
		if opts.RequestFamily != "" {
			query += " AND request_family = ?"
			args = append(args, normalizeRequestFamily(opts.RequestFamily))
		}
		if opts.UpstreamName != "" {
			query += " AND upstream_name = ?"
			args = append(args, opts.UpstreamName)
		}
	}
	rows, err := ut.readDB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make(map[usageSummaryKey]*usageSummaryAccumulator)
	for rows.Next() {
		var start timezonepolicy.DBTime
		var model, family, upstreamType, upstreamName, status string
		var upstreamID int64
		var input, output, cacheCreation, cacheRead int64
		var cost float64
		var duration sql.NullInt64
		if err := rows.Scan(&start, &model, &family, &upstreamType, &upstreamName, &upstreamID,
			&status, &input, &output, &cacheCreation, &cacheRead, &cost, &duration); err != nil {
			return nil, err
		}
		key := usageSummaryKey{
			date: policy.BusinessDate(start.Time), modelName: model,
			requestFamily: family, upstreamType: upstreamType, upstreamName: upstreamName, upstreamID: upstreamID,
		}
		acc := groups[key]
		if acc == nil {
			acc = &usageSummaryAccumulator{UsageSummary: UsageSummary{
				TimezoneName: policy.Name(), Date: key.date, ModelName: model,
				RequestFamily: family, UpstreamType: upstreamType, UpstreamName: upstreamName, UpstreamID: upstreamID,
			}}
			groups[key] = acc
		}
		acc.RequestCount++
		if status == "completed" {
			acc.SuccessCount++
		}
		if isFailedRequestStatus(status) {
			acc.ErrorCount++
		}
		acc.TotalInputTokens += input
		acc.TotalOutputTokens += output
		acc.TotalCacheCreationTokens += cacheCreation
		acc.TotalCacheReadTokens += cacheRead
		acc.TotalCostUSD += cost
		if duration.Valid && duration.Int64 > 0 {
			acc.durationTotal += duration.Int64
			acc.durationCount++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]UsageSummary, 0, len(groups))
	for _, acc := range groups {
		if acc.durationCount > 0 {
			acc.AvgDurationMs = float64(acc.durationTotal) / float64(acc.durationCount)
		}
		result = append(result, acc.UsageSummary)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Date != result[j].Date {
			return result[i].Date > result[j].Date
		}
		return result[i].TotalCostUSD > result[j].TotalCostUSD
	})
	return paginateUsageSummaries(result, opts), nil
}

func paginateUsageSummaries(summaries []UsageSummary, opts *QueryOptions) []UsageSummary {
	if opts == nil || len(summaries) == 0 {
		return summaries
	}
	start := opts.Offset
	if start < 0 {
		start = 0
	}
	if start >= len(summaries) {
		return []UsageSummary{}
	}
	end := len(summaries)
	if opts.Limit > 0 && start+opts.Limit < end {
		end = start + opts.Limit
	}
	return summaries[start:end]
}

func isFailedRequestStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "failed", "error", "auth_error", "rate_limited", "server_error", "network_error", "stream_error", "timeout":
		return true
	default:
		return false
	}
}
