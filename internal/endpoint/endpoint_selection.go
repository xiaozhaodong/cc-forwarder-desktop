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

// GetHealthyEndpoints returns a list of healthy endpoints from active groups based on strategy
// v5.0 Desktop: 支持故障转移 - 活跃端点不健康时，返回其他 failover_enabled=true 的健康端点
func (m *Manager) GetHealthyEndpoints() []*Endpoint {
	// v5.0+: 使用快照机制
	m.endpointsMu.RLock()
	snapshot := make([]*Endpoint, len(m.endpoints))
	copy(snapshot, m.endpoints)
	m.endpointsMu.RUnlock()

	// 1. 首先尝试获取活跃组（用户激活的端点）的健康端点
	activeEndpoints := m.groupManager.FilterEndpointsByActiveGroups(snapshot)

	now := time.Now()
	var healthy []*Endpoint
	var failedEndpoints []string // 📊 [失败追踪] 记录达到失败阈值的端点

	for _, endpoint := range activeEndpoints {
		endpoint.mutex.RLock()
		isHealthy := endpoint.Status.Healthy
		// 检查是否在请求冷却中
		inCooldown := !endpoint.Status.CooldownUntil.IsZero() && now.Before(endpoint.Status.CooldownUntil)
		endpoint.mutex.RUnlock()

		// 📊 [失败追踪] 检查是否达到失败阈值
		if m.config.FailureTracker.Enabled && m.failureTracker.ShouldTriggerAction(endpoint.Config.Name) {
			action := m.config.FailureTracker.Action
			switch action {
			case "failover":
				// 🔄 故障转移：跳过该端点，不清除记录
				// 只有当端点真正请求成功时才会清除失败记录（在 lifecycle_manager 中）
				slog.Info(fmt.Sprintf("🔄 [失败追踪] 端点 %s 达到失败阈值，触发故障转移，跳过该端点", endpoint.Config.Name))
				failedEndpoints = append(failedEndpoints, endpoint.Config.Name) // 记录失败端点
			case "suspend":
				// ⏸️ 挂起模式：跳过该端点，不清除记录，基于时间窗口自动恢复
				// 失败记录会在窗口时间后自动过期，无需手动干预
				slog.Info(fmt.Sprintf("⏸️ [失败追踪] 端点 %s 达到失败阈值，挂起模式，跳过该端点", endpoint.Config.Name))
			case "reject":
				// ❌ 拒绝模式：跳过该端点（实际拒绝在 handler 层通过 ShouldRejectRequest() 判断）
				slog.Warn(fmt.Sprintf("❌ [失败追踪] 端点 %s 达到失败阈值（reject 模式）", endpoint.Config.Name))
			}
			continue // 跳过该端点
		}

		if isHealthy && !inCooldown {
			healthy = append(healthy, endpoint)
		} else if inCooldown {
			slog.Debug(fmt.Sprintf("⏭️ [端点选择] 跳过冷却中的端点: %s", endpoint.Config.Name))
		}
	}

	// 2. 如果活跃端点健康且不在冷却中，直接返回
	if len(healthy) > 0 {
		return m.sortHealthyEndpoints(healthy, true)
	}

	// 3. 活跃端点不健康或在冷却中，尝试故障转移
	if !m.config.Failover.Enabled {
		return healthy // 故障转移未启用，返回空列表
	}

	slog.Info("🔄 [故障转移] 活跃端点不可用（不健康或在冷却中），尝试故障转移到其他端点")
	healthy = m.getFailoverEndpoints(activeEndpoints, snapshot)

	if len(healthy) > 0 {
		slog.Info(fmt.Sprintf("✅ [故障转移] 找到 %d 个可用的故障转移端点", len(healthy)))

		// 🔧 [选择阶段故障转移] 对因失败追踪被跳过的端点，设置冷却并激活新端点
		if len(failedEndpoints) > 0 {
			m.executeSelectionFailover(failedEndpoints, healthy[0].Config.Name)
		}
	}

	return m.sortHealthyEndpoints(healthy, true) // 按策略排序
}

// getFailoverEndpoints 获取故障转移端点（排除活跃端点）
// 返回所有 failover_enabled=true 且健康且不在冷却中的非活跃端点
func (m *Manager) getFailoverEndpoints(activeEndpoints, snapshot []*Endpoint) []*Endpoint {
	// 构建活跃端点名称集合
	activeNames := make(map[string]bool, len(activeEndpoints))
	for _, ep := range activeEndpoints {
		activeNames[ep.Config.Name] = true
	}

	now := time.Now()
	var failoverEndpoints []*Endpoint
	for _, endpoint := range snapshot {
		// 跳过活跃端点（已经检查过了）
		if activeNames[endpoint.Config.Name] {
			continue
		}

		// 检查是否参与故障转移（默认为 true）
		failoverEnabled := true
		if endpoint.Config.FailoverEnabled != nil {
			failoverEnabled = *endpoint.Config.FailoverEnabled
		}
		if !failoverEnabled {
			continue
		}

		// 📊 [失败追踪] 检查是否达到失败阈值
		if m.config.FailureTracker.Enabled && m.failureTracker.ShouldTriggerAction(endpoint.Config.Name) {
			slog.Debug(fmt.Sprintf("📊 [失败追踪] 故障转移端点 %s 达到失败阈值，跳过", endpoint.Config.Name))
			continue
		}

		// 检查健康状态和冷却状态
		endpoint.mutex.RLock()
		isHealthy := endpoint.Status.Healthy
		inCooldown := !endpoint.Status.CooldownUntil.IsZero() && now.Before(endpoint.Status.CooldownUntil)
		endpoint.mutex.RUnlock()

		if inCooldown {
			slog.Debug(fmt.Sprintf("⏭️ [故障转移] 跳过冷却中的端点: %s", endpoint.Config.Name))
			continue
		}

		if isHealthy {
			failoverEndpoints = append(failoverEndpoints, endpoint)
		}
	}

	return failoverEndpoints
}

// sortHealthyEndpoints sorts healthy endpoints based on strategy with optional logging
func (m *Manager) sortHealthyEndpoints(healthy []*Endpoint, showLogs bool) []*Endpoint {
	// Sort based on strategy
	switch m.config.Strategy.Type {
	case "priority":
		sort.Slice(healthy, func(i, j int) bool {
			return healthy[i].Config.Priority < healthy[j].Config.Priority
		})
	case "fastest":
		// Log endpoint latencies for fastest strategy (only if showLogs is true)
		if len(healthy) > 1 && showLogs {
			slog.Info("📊 [Fastest Strategy] 基于健康检查的端点延迟排序:")
			for _, ep := range healthy {
				ep.mutex.RLock()
				responseTime := ep.Status.ResponseTime
				ep.mutex.RUnlock()
				slog.Info(fmt.Sprintf("  ⏱️ %s - 延迟: %dms (来源: 定期健康检查)",
					ep.Config.Name, responseTime.Milliseconds()))
			}
		}

		sort.Slice(healthy, func(i, j int) bool {
			healthy[i].mutex.RLock()
			healthy[j].mutex.RLock()
			defer healthy[i].mutex.RUnlock()
			defer healthy[j].mutex.RUnlock()
			return healthy[i].Status.ResponseTime < healthy[j].Status.ResponseTime
		})
	}

	return healthy
}

// GetFastestEndpointsWithRealTimeTest returns endpoints from active groups sorted by real-time testing
// v5.0 Desktop: 支持故障转移 - 活跃端点不健康时，返回其他 failover_enabled=true 的健康端点
func (m *Manager) GetFastestEndpointsWithRealTimeTest(ctx context.Context) []*Endpoint {
	// v5.0+: 使用快照机制
	m.endpointsMu.RLock()
	snapshot := make([]*Endpoint, len(m.endpoints))
	copy(snapshot, m.endpoints)
	m.endpointsMu.RUnlock()

	// 1. 首先尝试获取活跃组（用户激活的端点）的健康端点
	activeEndpoints := m.groupManager.FilterEndpointsByActiveGroups(snapshot)

	var healthy []*Endpoint
	var failedEndpoints []string // 📊 [失败追踪] 记录达到失败阈值的端点

	for _, endpoint := range activeEndpoints {
		// 📊 [失败追踪] 检查是否达到失败阈值
		if m.config.FailureTracker.Enabled && m.failureTracker.ShouldTriggerAction(endpoint.Config.Name) {
			action := m.config.FailureTracker.Action
			if action == "failover" {
				slog.Debug(fmt.Sprintf("📊 [失败追踪] 端点 %s 达到失败阈值，跳过", endpoint.Config.Name))
				failedEndpoints = append(failedEndpoints, endpoint.Config.Name) // 记录失败端点
			}
			continue
		}

		endpoint.mutex.RLock()
		if endpoint.Status.Healthy {
			healthy = append(healthy, endpoint)
		}
		endpoint.mutex.RUnlock()
	}

	// 2. 如果活跃端点不健康，尝试故障转移
	if len(healthy) == 0 && m.config.Failover.Enabled {
		slog.InfoContext(ctx, "🔄 [故障转移] 活跃端点不健康，尝试故障转移到其他端点")
		healthy = m.getFailoverEndpoints(activeEndpoints, snapshot)

		if len(healthy) > 0 {
			slog.InfoContext(ctx, fmt.Sprintf("✅ [故障转移] 找到 %d 个可用的故障转移端点", len(healthy)))

			// 🔧 [选择阶段故障转移] 对因失败追踪被跳过的端点，设置冷却并激活新端点
			if len(failedEndpoints) > 0 {
				m.executeSelectionFailover(failedEndpoints, healthy[0].Config.Name)
			}
		}
	}

	if len(healthy) == 0 {
		return healthy
	}

	// If not using fastest strategy or fast test disabled, apply sorting with logging
	if m.config.Strategy.Type != "fastest" || !m.config.Strategy.FastTestEnabled {
		return m.sortHealthyEndpoints(healthy, true) // Show logs
	}

	// Check if we have cached fast test results first
	testResults, usedCache := m.fastTester.TestEndpointsParallel(ctx, healthy)

	// Only show health check sorting if we're NOT using cache
	if !usedCache && m.config.Strategy.Type == "fastest" && len(healthy) > 1 {
		slog.InfoContext(ctx, "📊 [Fastest Strategy] 基于健康检查的活跃组端点延迟排序:")
		for _, ep := range healthy {
			ep.mutex.RLock()
			responseTime := ep.Status.ResponseTime
			group := ep.Config.Group
			ep.mutex.RUnlock()
			slog.InfoContext(ctx, fmt.Sprintf("  ⏱️ %s (组: %s) - 延迟: %dms (来源: 定期健康检查)",
				ep.Config.Name, group, responseTime.Milliseconds()))
		}
	}

	// Log ALL test results first (including failures) - but only if cache wasn't used
	if len(testResults) > 0 && !usedCache {
		slog.InfoContext(ctx, "🔍 [Fastest Response Mode] 活跃组端点性能测试结果:")
		successCount := 0
		for _, result := range testResults {
			group := result.Endpoint.Config.Group
			if result.Success {
				successCount++
				slog.InfoContext(ctx, fmt.Sprintf("  ✅ 健康 %s (组: %s) - 响应时间: %dms",
					result.Endpoint.Config.Name, group,
					result.ResponseTime.Milliseconds()))
			} else {
				errorMsg := ""
				if result.Error != nil {
					errorMsg = fmt.Sprintf(" - 错误: %s", result.Error.Error())
				}
				slog.InfoContext(ctx, fmt.Sprintf("  ❌ 异常 %s (组: %s) - 响应时间: %dms%s",
					result.Endpoint.Config.Name, group,
					result.ResponseTime.Milliseconds(),
					errorMsg))
			}
		}

		slog.InfoContext(ctx, fmt.Sprintf("📊 [测试摘要] 活跃组测试: %d个端点, 健康: %d个, 异常: %d个",
			len(testResults), successCount, len(testResults)-successCount))
	}

	// Sort by response time (only successful results)
	sortedResults := SortByResponseTime(testResults)

	if len(sortedResults) == 0 {
		slog.WarnContext(ctx, "⚠️ [Fastest Response Mode] 活跃组所有端点测试失败，回退到健康检查模式")
		return healthy // Fall back to health check results if no fast tests succeeded
	}

	// Convert back to endpoint slice
	endpoints := make([]*Endpoint, 0, len(sortedResults))
	for _, result := range sortedResults {
		endpoints = append(endpoints, result.Endpoint)
	}

	// Log the successful endpoint ranking
	if len(endpoints) > 0 {
		// Show the fastest endpoint selection
		fastestEndpoint := endpoints[0]
		var fastestTime int64
		for _, result := range sortedResults {
			if result.Endpoint == fastestEndpoint {
				fastestTime = result.ResponseTime.Milliseconds()
				break
			}
		}

		cacheIndicator := ""
		if usedCache {
			cacheIndicator = " (缓存)"
		}

		slog.InfoContext(ctx, fmt.Sprintf("🚀 [Fastest Response Mode] 选择最快端点: %s - %dms%s",
			fastestEndpoint.Config.Name, fastestTime, cacheIndicator))

		// Show other available endpoints if there are more than one
		if len(endpoints) > 1 && !usedCache {
			slog.InfoContext(ctx, "📋 [备用端点] 其他可用端点:")
			for i := 1; i < len(endpoints); i++ {
				ep := endpoints[i]
				var responseTime int64
				var epGroup string
				for _, result := range sortedResults {
					if result.Endpoint == ep {
						responseTime = result.ResponseTime.Milliseconds()
						epGroup = result.Endpoint.Config.Group
						break
					}
				}
				slog.InfoContext(ctx, fmt.Sprintf("  🔄 备用 %s (组: %s) - 响应时间: %dms",
					ep.Config.Name, epGroup, responseTime))
			}
		}
	}

	return endpoints
}

// GetEndpointByName returns an endpoint by name, only from active groups
func (m *Manager) GetEndpointByName(name string) *Endpoint {
	// v5.0+: 使用快照机制
	m.endpointsMu.RLock()
	snapshot := make([]*Endpoint, len(m.endpoints))
	copy(snapshot, m.endpoints)
	m.endpointsMu.RUnlock()

	// First filter by active groups
	activeEndpoints := m.groupManager.FilterEndpointsByActiveGroups(snapshot)

	// Then find by name
	for _, endpoint := range activeEndpoints {
		if endpoint.Config.Name == name {
			return endpoint
		}
	}
	return nil
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

// executeSelectionFailover 执行选择阶段的故障转移操作
// 当在端点选择阶段检测到失败追踪阈值时调用
// 对失败端点设置冷却并停用，激活新故障转移端点
func (m *Manager) executeSelectionFailover(failedEndpoints []string, newEndpointName string) {
	// 计算冷却时间
	cooldownDuration := m.config.Failover.DefaultCooldown
	if cooldownDuration == 0 {
		cooldownDuration = 10 * time.Minute // 默认 10 分钟
	}

	// 1. 对每个失败端点设置冷却并停用组
	for _, failedName := range failedEndpoints {
		failedEndpoint := m.GetEndpointByNameAny(failedName)
		if failedEndpoint == nil {
			continue
		}

		// 如果端点有自定义冷却时间，使用端点配置
		epCooldown := cooldownDuration
		if failedEndpoint.Config.Cooldown != nil && *failedEndpoint.Config.Cooldown > 0 {
			epCooldown = *failedEndpoint.Config.Cooldown
		}

		// 设置冷却状态
		failedEndpoint.mutex.Lock()
		failedEndpoint.Status.CooldownUntil = time.Now().Add(epCooldown)
		failedEndpoint.Status.CooldownReason = "failure_tracker_threshold"
		failedEndpoint.mutex.Unlock()

		slog.Info(fmt.Sprintf("⏱️ [选择阶段故障转移] 端点 %s 进入冷却，持续 %v", failedName, epCooldown))

		// 停用失败端点的组
		if err := m.groupManager.DeactivateGroup(failedName); err != nil {
			slog.Warn(fmt.Sprintf("⚠️ [选择阶段故障转移] 停用组失败: %v", err))
		}
	}

	// 2. 激活新故障转移端点的组
	if err := m.groupManager.ManualActivateGroup(newEndpointName); err != nil {
		slog.Error(fmt.Sprintf("❌ [选择阶段故障转移] 激活新端点失败: %v", err))
		return
	}

	slog.Info(fmt.Sprintf("✅ [选择阶段故障转移] 已切换到端点: %s", newEndpointName))

	// 3. 调用回调通知 App 层同步数据库（使用第一个失败端点作为代表）
	if m.onFailoverTriggered != nil && len(failedEndpoints) > 0 {
		go m.onFailoverTriggered(failedEndpoints[0], newEndpointName)
	}

	// 4. 触发前端刷新
	if m.onHealthCheckComplete != nil {
		go m.onHealthCheckComplete()
	}
}

