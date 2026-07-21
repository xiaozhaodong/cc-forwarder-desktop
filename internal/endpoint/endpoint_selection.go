// endpoint_selection.go - 端点选择/路由功能
// 包含健康端点获取、故障转移端点选择、排序策略等

package endpoint

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

func (m *Manager) GetHealthyEndpoints() []*Endpoint {
	return m.PrepareRouteCandidates(context.Background(), RouteRequestProfile{}).Candidates
}

func (m *Manager) GetHealthyEndpointsForRoute(profile RouteRequestProfile) []*Endpoint {
	return m.PrepareRouteCandidates(context.Background(), profile).Candidates
}

func findEndpointInSnapshot(snapshot []*Endpoint, name string) *Endpoint {
	for _, endpoint := range snapshot {
		if endpoint.Config.Name == name {
			return endpoint
		}
	}
	return nil
}

func (m *Manager) shouldSkipByRouteProfile(endpoint *Endpoint, profile *RouteRequestProfile) bool {
	if endpoint == nil || profile == nil {
		return false
	}
	if hit, reason := m.routeState.HasNegativeHit(endpoint.Config.Name, *profile); hit {
		slog.Info(fmt.Sprintf("⏭️ [路由负向缓存] 跳过端点: %s, reason=%s, model=%s, path=%s",
			endpoint.Config.Name, reason, profile.Model, profile.Path))
		return true
	}
	return false
}

func (m *Manager) FilterRouteCandidates(endpoints []*Endpoint, profile RouteRequestProfile) []*Endpoint {
	if len(endpoints) == 0 {
		return endpoints
	}

	override := m.routeOverride.Snapshot()
	filtered := make([]*Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint == nil {
			continue
		}
		if override.Mode == RouteModeManualFixed && override.EndpointName != "" && endpoint.Config.Name != override.EndpointName {
			continue
		}
		if m.shouldSkipByRouteProfile(endpoint, &profile) {
			continue
		}
		filtered = append(filtered, endpoint)
	}
	return m.applyRouteOverrideOrder(filtered)
}

func (m *Manager) applyRouteOverrideOrder(endpoints []*Endpoint) []*Endpoint {
	override := m.routeOverride.Snapshot()
	if override.Mode != RouteModeManualPreferred || override.EndpointName == "" || len(endpoints) < 2 {
		return endpoints
	}

	sort.SliceStable(endpoints, func(i, j int) bool {
		if endpoints[i].Config.Name == override.EndpointName {
			return true
		}
		if endpoints[j].Config.Name == override.EndpointName {
			return false
		}
		return false
	})
	return endpoints
}

// sortHealthyEndpoints sorts healthy endpoints based on strategy with optional logging
func (m *Manager) sortHealthyEndpoints(healthy []*Endpoint, showLogs bool) []*Endpoint {
	cfg := m.getConfigSnapshot()

	// Sort based on strategy
	switch cfg.Strategy.Type {
	case "priority":
		sort.Slice(healthy, func(i, j int) bool {
			return healthy[i].Config.Priority < healthy[j].Config.Priority
		})
	case "fastest":
		// Log endpoint latencies for fastest strategy (only if showLogs is true)
		if len(healthy) > 1 && showLogs {
			slog.Info("📊 [Fastest Strategy] 基于最近记录的响应时间排序:")
			for _, ep := range healthy {
				ep.mutex.RLock()
				responseTime := ep.Status.ResponseTime
				ep.mutex.RUnlock()
				slog.Info(fmt.Sprintf("  ⏱️ %s - 延迟: %dms (来源: 最近连通性记录)",
					ep.Config.Name, responseTime.Milliseconds()))
			}
		}

		type endpointLatency struct {
			endpoint     *Endpoint
			responseTime time.Duration
		}

		latencies := make([]endpointLatency, 0, len(healthy))
		for _, ep := range healthy {
			ep.mutex.RLock()
			responseTime := ep.Status.ResponseTime
			ep.mutex.RUnlock()
			latencies = append(latencies, endpointLatency{
				endpoint:     ep,
				responseTime: responseTime,
			})
		}

		sort.SliceStable(latencies, func(i, j int) bool {
			if latencies[i].responseTime == latencies[j].responseTime {
				return latencies[i].endpoint.Config.Priority < latencies[j].endpoint.Config.Priority
			}
			return latencies[i].responseTime < latencies[j].responseTime
		})

		for i := range latencies {
			healthy[i] = latencies[i].endpoint
		}
	}

	return healthy
}

// GetFastestEndpointsWithRealTimeTest 保留旧 API，候选与排序统一由新调度器负责。
func (m *Manager) GetFastestEndpointsWithRealTimeTest(ctx context.Context) []*Endpoint {
	return m.GetFastestEndpointsWithRealTimeTestForRoute(ctx, RouteRequestProfile{})
}

func (m *Manager) GetFastestEndpointsWithRealTimeTestForRoute(ctx context.Context, profile RouteRequestProfile) []*Endpoint {
	return m.PrepareRouteCandidates(ctx, profile).Candidates
}

// GetEndpointByName 返回当前 active 且可路由的同名端点。
func (m *Manager) GetEndpointByName(name string) *Endpoint {
	activeName, _ := m.GetActiveEndpointSelection()
	if activeName != name {
		return nil
	}
	endpoint := m.GetEndpointByNameAny(name)
	if endpoint == nil || !m.IsEndpointRoutable(endpoint) || endpoint.IsPaused() {
		return nil
	}
	return endpoint
}

// GetEndpointByNameAny returns an endpoint by name from all endpoints (ignoring group status)
func (m *Manager) GetEndpointByNameAny(name string) *Endpoint {
	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()

	for _, endpoint := range m.endpoints {
		if endpoint.Config.Name == name {
			return endpoint
		}
	}
	return nil
}

// GetAllEndpoints returns all endpoints (deprecated: use GetEndpoints instead)
func (m *Manager) GetAllEndpoints() []*Endpoint {
	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()

	result := make([]*Endpoint, len(m.endpoints))
	copy(result, m.endpoints)
	return result
}

// GetEndpoints returns all endpoints for Web interface
func (m *Manager) GetEndpoints() []*Endpoint {
	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()

	result := make([]*Endpoint, len(m.endpoints))
	copy(result, m.endpoints)
	return result
}

// GetEndpointStatus returns the status of an endpoint by name
func (m *Manager) GetEndpointStatus(name string) EndpointStatus {
	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()

	for _, ep := range m.endpoints {
		if ep.Config.Name == name {
			ep.mutex.RLock()
			status := ep.Status
			ep.mutex.RUnlock()
			return status
		}
	}
	return EndpointStatus{}
}

// GetEndpointCount 返回当前端点数量（v5.0+ 新增）
func (m *Manager) GetEndpointCount() int {
	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()
	return len(m.endpoints)
}
