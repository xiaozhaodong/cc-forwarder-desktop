package endpoint

import (
	"context"
	"sort"
	"time"
)

// 端点调度器（方案 §4.2）：纯读候选计算，无路由状态写副作用
//（不修改冷却 / 激活 / 失败追踪 / 负缓存）。
// fastest 策略的实时测速属 IO 行为，由转发管线在调用本方法前显式执行
//（测速只更新测速缓存，非路由状态）；本方法按测速缓存排序。

// EndpointScheduleDecision 单个端点的调度决策记录
type EndpointScheduleDecision struct {
	Name        string    `json:"name"`
	Decision    string    `json:"decision"` // candidate / skipped
	Reason      string    `json:"reason"`
	AvailableAt time.Time `json:"available_at,omitempty"` // skipped 时的预计可用时间（零值=未知）
}

const (
	endpointDecisionCandidate = "candidate"
	endpointDecisionSkipped   = "skipped"
)

// EndpointScheduleSnapshot 一次调度的完整决策快照（§4.5 观测）
type EndpointScheduleSnapshot struct {
	CapturedAt time.Time                  `json:"captured_at"`
	Decisions  []EndpointScheduleDecision `json:"decisions"`
}

// EarliestAvailableAt 返回被跳过端点中最早的可用时间（零值=无已知恢复时间）。
// 供空候选 / 全部失败时计算 503 Retry-After。
func (s *EndpointScheduleSnapshot) EarliestAvailableAt() time.Time {
	if s == nil {
		return time.Time{}
	}
	var earliest time.Time
	for _, decision := range s.Decisions {
		if decision.Decision != endpointDecisionSkipped || decision.AvailableAt.IsZero() {
			continue
		}
		if earliest.IsZero() || decision.AvailableAt.Before(earliest) {
			earliest = decision.AvailableAt
		}
	}
	return earliest
}

// EndpointScheduleResult 调度结果：候选序 + CAS 迁移所需的 revision 快照
type EndpointScheduleResult struct {
	Candidates                []*Endpoint
	ActiveEndpointAtSelection string
	ActiveRevision            int64
	RouteOverrideRevision     int64
	Snapshot                  *EndpointScheduleSnapshot
}

// PrepareRouteCandidates 计算本次请求的候选端点序（§4.2）。
// 候选序：[activeEndpoint（若可路由）] + 其余 failover_enabled 端点按策略排序；
// 过滤：冷却 / tripped / PausedUntil / 负缓存；skipped 记录 availableAt（四来源）。
// manual_fixed 只考虑目标端点；Failover.Enabled=false 时只返回单候选且禁用请求内换候选。
func (m *Manager) PrepareRouteCandidates(ctx context.Context, profile RouteRequestProfile) EndpointScheduleResult {
	cfg := m.getConfigSnapshot()
	override := m.routeOverride.Snapshot()
	activeName, activeRevision := m.GetActiveEndpointSelection()

	m.endpointsMu.RLock()
	snapshot := make([]*Endpoint, len(m.endpoints))
	copy(snapshot, m.endpoints)
	m.endpointsMu.RUnlock()

	result := EndpointScheduleResult{
		ActiveEndpointAtSelection: activeName,
		ActiveRevision:            activeRevision,
		RouteOverrideRevision:     override.Revision,
		Snapshot:                  &EndpointScheduleSnapshot{CapturedAt: time.Now()},
	}

	record := func(name, decision, reason string, availableAt time.Time) {
		result.Snapshot.Decisions = append(result.Snapshot.Decisions, EndpointScheduleDecision{
			Name:        name,
			Decision:    decision,
			Reason:      reason,
			AvailableAt: availableAt,
		})
	}

	// manual_fixed：只考虑目标端点，不可路由时不静默转移
	if override.Mode == RouteModeManualFixed && override.EndpointName != "" {
		target := findEndpointInSnapshot(snapshot, override.EndpointName)
		if target == nil {
			record(override.EndpointName, endpointDecisionSkipped, "manual_fixed_target_missing", time.Time{})
			return result
		}
		if ok, reason, availableAt := m.classifyEndpointRoutable(target, profile); !ok {
			record(target.Config.Name, endpointDecisionSkipped, "manual_fixed_"+reason, availableAt)
			return result
		}
		record(target.Config.Name, endpointDecisionCandidate, "manual_fixed", time.Time{})
		result.Candidates = []*Endpoint{target}
		return result
	}

	// Failover.Enabled=false：单候选规则（reject 模式未触发拒绝时同样不得静默尝试备用端点）
	if !cfg.Failover.Enabled {
		if activeName == "" {
			return result
		}
		active := findEndpointInSnapshot(snapshot, activeName)
		if active == nil {
			record(activeName, endpointDecisionSkipped, "active_endpoint_missing", time.Time{})
			return result
		}
		if ok, reason, availableAt := m.classifyEndpointRoutable(active, profile); !ok {
			record(active.Config.Name, endpointDecisionSkipped, reason, availableAt)
			return result
		}
		record(active.Config.Name, endpointDecisionCandidate, "active_failover_disabled", time.Time{})
		result.Candidates = []*Endpoint{active}
		return result
	}

	// 常规候选序：[active] + 其余 failover_enabled 端点
	candidates := make([]*Endpoint, 0, len(snapshot))
	if activeName != "" {
		if active := findEndpointInSnapshot(snapshot, activeName); active != nil {
			if ok, reason, availableAt := m.classifyEndpointRoutable(active, profile); ok {
				candidates = append(candidates, active)
				record(active.Config.Name, endpointDecisionCandidate, "active", time.Time{})
			} else {
				record(active.Config.Name, endpointDecisionSkipped, reason, availableAt)
			}
		}
	}

	rest := make([]*Endpoint, 0, len(snapshot))
	for _, ep := range snapshot {
		if ep == nil || ep.Config.Name == activeName {
			continue
		}
		failoverEnabled := true
		if ep.Config.FailoverEnabled != nil {
			failoverEnabled = *ep.Config.FailoverEnabled
		}
		if !failoverEnabled {
			record(ep.Config.Name, endpointDecisionSkipped, "failover_disabled_endpoint", time.Time{})
			continue
		}
		if ok, reason, availableAt := m.classifyEndpointRoutable(ep, profile); !ok {
			record(ep.Config.Name, endpointDecisionSkipped, reason, availableAt)
			continue
		}
		rest = append(rest, ep)
	}

	rest = m.sortRestForSchedule(ctx, rest)
	for _, ep := range rest {
		record(ep.Config.Name, endpointDecisionCandidate, "failover_candidate", time.Time{})
	}
	candidates = append(candidates, rest...)

	// manual_preferred：目标端点提到候选序最前（保持其余相对顺序）
	result.Candidates = m.applyRouteOverrideOrder(candidates)
	return result
}

// classifyEndpointRoutable 判定端点是否可路由；不可路由时给出原因与预计可用时间。
// availableAt 四来源（§3.1 补充规则）：暂停到期、冷却到期、tripped 窗口到期（保守估计）、
// 负缓存条目 expiresAt。
func (m *Manager) classifyEndpointRoutable(ep *Endpoint, profile RouteRequestProfile) (bool, string, time.Time) {
	if ep == nil {
		return false, "endpoint_missing", time.Time{}
	}

	ep.mutex.RLock()
	pausedUntil := ep.Status.PausedUntil
	cooldownUntil := ep.Status.CooldownUntil
	ep.mutex.RUnlock()

	now := time.Now()
	if !pausedUntil.IsZero() && now.Before(pausedUntil) {
		return false, "paused", pausedUntil
	}
	if !cooldownUntil.IsZero() && now.Before(cooldownUntil) {
		return false, "cooldown", cooldownUntil
	}

	cfg := m.getConfigSnapshot()
	if cfg.FailureTracker.Enabled && m.failureTracker.ShouldTriggerAction(ep.Config.Name) {
		return false, "failure_threshold_tripped", m.failureTracker.TrippedUntil(ep.Config.Name)
	}

	if hit, failureClass, expiresAt := m.routeState.NegativeHitWithExpiry(ep.Config.Name, profile); hit {
		return false, "negative_cache_" + failureClass, expiresAt
	}

	return true, "", time.Time{}
}

// sortRestForSchedule 备选端点排序：fastest+实时测速时按测速结果，否则按策略排序
// （priority / fastest 缓存）。测速只更新测速缓存，非路由状态。
func (m *Manager) sortRestForSchedule(ctx context.Context, rest []*Endpoint) []*Endpoint {
	cfg := m.getConfigSnapshot()
	if cfg.Strategy.Type == "fastest" && cfg.Strategy.FastTestEnabled && len(rest) > 1 && m.fastTester != nil {
		results, _ := m.fastTester.TestEndpointsParallel(ctx, rest)
		if ordered := orderByFastTestResults(rest, results); len(ordered) == len(rest) {
			return ordered
		}
	}
	return m.sortHealthyEndpoints(rest, false)
}

// orderByFastTestResults 按实时测速结果排序：成功者按响应时间升序在前，
// 失败/未覆盖者保持原相对顺序在后。
func orderByFastTestResults(rest []*Endpoint, results []*FastTestResult) []*Endpoint {
	if len(results) == 0 {
		return nil
	}
	type timing struct {
		ep           *Endpoint
		responseTime time.Duration
	}
	successTimes := make(map[string]time.Duration, len(results))
	for _, result := range results {
		if result != nil && result.Success && result.Endpoint != nil {
			successTimes[result.Endpoint.Config.Name] = result.ResponseTime
		}
	}
	fast := make([]timing, 0, len(rest))
	var slow []*Endpoint
	for _, ep := range rest {
		if responseTime, ok := successTimes[ep.Config.Name]; ok {
			fast = append(fast, timing{ep: ep, responseTime: responseTime})
		} else {
			slow = append(slow, ep)
		}
	}
	sort.SliceStable(fast, func(i, j int) bool {
		if fast[i].responseTime == fast[j].responseTime {
			return fast[i].ep.Config.Priority < fast[j].ep.Config.Priority
		}
		return fast[i].responseTime < fast[j].responseTime
	})
	ordered := make([]*Endpoint, 0, len(rest))
	for _, item := range fast {
		ordered = append(ordered, item.ep)
	}
	return append(ordered, slow...)
}
