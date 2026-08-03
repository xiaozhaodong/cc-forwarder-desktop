// app_api_chart.go - 图表数据 API (Wails Bindings)
// 包含请求趋势、响应时间、端点健康状态、端点成本等图表数据

package main

import (
	"context"
	"time"
)

// ============================================================
// 图表数据 API
// ============================================================

// ChartDataPoint 图表数据点
type ChartDataPoint struct {
	Time      string  `json:"time"`
	Total     int64   `json:"total"`
	Success   int64   `json:"success"`
	Fail      int64   `json:"fail"`
	Avg       float64 `json:"avg"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	Value     int64   `json:"value"`
	Timestamp string  `json:"timestamp"`
}

// GetRequestTrendChart 获取请求趋势图表数据
func (a *App) GetRequestTrendChart(minutes int) []ChartDataPoint {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.monitoringMiddleware == nil {
		if a.logger != nil {
			a.logger.Warn("GetRequestTrendChart: monitoringMiddleware is nil")
		}
		return []ChartDataPoint{}
	}

	metrics := a.monitoringMiddleware.GetMetrics()
	if metrics == nil {
		if a.logger != nil {
			a.logger.Warn("GetRequestTrendChart: metrics is nil")
		}
		return []ChartDataPoint{}
	}

	// 直接在原始 *Metrics 上调用，而不是获取副本
	requestHistory := metrics.GetChartDataForRequestHistory(minutes)

	if a.logger != nil {
		a.logger.Info("📊 GetRequestTrendChart",
			"minutes", minutes,
			"history_points", len(requestHistory))
	}

	result := make([]ChartDataPoint, len(requestHistory))
	for i, point := range requestHistory {
		result[i] = ChartDataPoint{
			Time:    point.Timestamp.Format("15:04"),
			Total:   point.Total,
			Success: point.Successful,
			Fail:    point.Failed,
		}
	}

	return result
}

// GetResponseTimeChart 获取响应时间图表数据
func (a *App) GetResponseTimeChart(minutes int) []ChartDataPoint {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.monitoringMiddleware == nil {
		return []ChartDataPoint{}
	}

	metrics := a.monitoringMiddleware.GetMetrics()
	responseHistory := metrics.GetChartDataForResponseTime(minutes)

	result := make([]ChartDataPoint, len(responseHistory))
	for i, point := range responseHistory {
		result[i] = ChartDataPoint{
			Time: point.Timestamp.Format("15:04"),
			Avg:  float64(point.AverageTime) / float64(time.Millisecond),
			Min:  float64(point.MinTime) / float64(time.Millisecond),
			Max:  float64(point.MaxTime) / float64(time.Millisecond),
		}
	}

	return result
}

// GetConnectionActivityChart 获取连接活动图表数据
func (a *App) GetConnectionActivityChart(minutes int) []ChartDataPoint {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.monitoringMiddleware == nil {
		return []ChartDataPoint{}
	}

	// 连接活动图表使用请求历史数据
	metrics := a.monitoringMiddleware.GetMetrics()
	requestHistory := metrics.GetChartDataForRequestHistory(minutes)

	result := make([]ChartDataPoint, len(requestHistory))
	for i, point := range requestHistory {
		result[i] = ChartDataPoint{
			Time:  point.Timestamp.Format("15:04"),
			Value: point.Total, // 使用总请求数作为连接活动指标
		}
	}

	return result
}

// ============================================================
// 端点最近连通性状态图表 API
// ============================================================

// EndpointHealthData 端点最近连通性状态数据结构
type EndpointHealthData struct {
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
	Total     int `json:"total"`
}

// GetEndpointHealthChart 获取端点最近连通性状态图表数据
func (a *App) GetEndpointHealthChart() EndpointHealthData {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointManager == nil {
		return EndpointHealthData{}
	}

	endpoints := a.endpointManager.GetAllEndpoints()

	healthyCount := 0
	unhealthyCount := 0

	for _, endpoint := range endpoints {
		status := endpoint.GetStatus()
		if status.NeverChecked {
			continue
		}
		if status.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}

	return EndpointHealthData{
		Healthy:   healthyCount,
		Unhealthy: unhealthyCount,
		Total:     len(endpoints),
	}
}

// ============================================================
// 端点成本图表 API
// ============================================================

// EndpointCostItem 端点成本数据项（用于前端图表）
type EndpointCostItem struct {
	Name          string  `json:"name"`
	RequestFamily string  `json:"request_family"`
	Tokens        int64   `json:"tokens"`
	Cost          float64 `json:"cost"`
}

// GetEndpointCosts 获取当日端点成本数据
func (a *App) GetEndpointCosts() []EndpointCostItem {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.usageTracker == nil {
		return []EndpointCostItem{}
	}

	// 获取当日日期
	date := time.Now().Format("2006-01-02")

	// 查询端点成本数据
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	costs, err := a.usageTracker.GetEndpointCostsForDate(ctx, date)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("获取端点成本数据失败", "error", err)
		}
		return []EndpointCostItem{}
	}

	// 转换为前端期望的格式
	result := make([]EndpointCostItem, len(costs))
	for i, cost := range costs {
		name := cost.UpstreamName
		if name == "" {
			name = "未知上游"
		}

		// 计算总 Token
		totalTokens := cost.InputTokens + cost.OutputTokens + cost.CacheCreationTokens + cost.CacheReadTokens

		result[i] = EndpointCostItem{
			Name:          name,
			RequestFamily: cost.RequestFamily,
			Tokens:        totalTokens,
			Cost:          cost.TotalCostUSD,
		}
	}

	return result
}
