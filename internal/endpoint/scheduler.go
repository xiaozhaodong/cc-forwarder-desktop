package endpoint

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// 端点调度器（方案 §4.2）：纯读候选计算，无路由状态写副作用
//（不修改冷却 / 激活 / 失败追踪 / 负缓存）。
// fastest 策略的实时测速属 IO 行为，由转发管线在调用本方法前显式执行
//（测速只更新测速缓存，非路由状态）；本方法按测速缓存排序。

// EndpointScheduleDecision 单个端点的调度决策记录
type EndpointScheduleDecision struct {
	Name           string    `json:"name"`
	Decision       string    `json:"decision"` // candidate / skipped
	Reason         string    `json:"reason"`
	AvailableAt    time.Time `json:"available_at,omitempty"` // skipped 时的预计可用时间（零值=未知）
	RuntimeOutcome string    `json:"runtime_outcome,omitempty"`
	RuntimeError   string    `json:"runtime_error,omitempty"`
}

const (
	endpointDecisionCandidate = "candidate"
	endpointDecisionSkipped   = "skipped"
)

// 端点调度快照运行态与终态。字符串作为 Wails/UI 稳定契约。
const (
	EndpointScheduleRuntimeAttempting = "attempting"
	EndpointScheduleRuntimeTryNext    = "try_next"

	EndpointScheduleOutcomePending                  = "pending"
	EndpointScheduleOutcomeSuccess                  = "success"
	EndpointScheduleOutcomeQualityIncomplete        = "quality_incomplete"
	EndpointScheduleOutcomeFailedAfterCommit        = "failed_after_commit"
	EndpointScheduleOutcomeCancelled                = "cancelled"
	EndpointScheduleOutcomePrivacyBlocked           = "privacy_blocked"
	EndpointScheduleOutcomePassthroughError         = "passthrough_error"
	EndpointScheduleOutcomePassthroughRaw           = "passthrough_raw"
	EndpointScheduleOutcomeNoCandidates             = "no_candidates"
	EndpointScheduleOutcomeManualFixedBlocked       = "manual_fixed_blocked"
	EndpointScheduleOutcomeAllCandidatesFailed      = "all_candidates_failed"
	EndpointScheduleOutcomeRejectedByFailureTracker = "rejected_by_failure_tracker"
	endpointScheduleSnapshotPendingTTL              = 5 * time.Minute
)

// EndpointScheduleSnapshot 一次调度的完整决策快照（§4.5 观测）
type EndpointScheduleSnapshot struct {
	RequestID                 string                     `json:"request_id"`
	CapturedAt                time.Time                  `json:"captured_at"`
	UpdatedAt                 time.Time                  `json:"updated_at"`
	RequestPath               string                     `json:"request_path"`
	ActiveEndpointAtSelection string                     `json:"active_endpoint_at_selection"`
	SelectedEndpoint          string                     `json:"selected_endpoint"`
	RouteMode                 string                     `json:"route_mode"`
	RouteEndpointName         string                     `json:"route_endpoint_name"`
	RouteFallbackEnabled      bool                       `json:"route_fallback_enabled"`
	FailoverEnabled           bool                       `json:"failover_enabled"`
	FinalOutcome              string                     `json:"final_outcome"`
	FinalError                string                     `json:"final_error"`
	Summary                   string                     `json:"summary"`
	Decisions                 []EndpointScheduleDecision `json:"decisions"`
}

type endpointScheduleSnapshotEntry struct {
	sequence uint64
	snapshot *EndpointScheduleSnapshot
}

// endpointScheduleSnapshotStore 保存最近一次请求和并发中的请求草稿。
// sequence 保证较早请求稍后完成时不会覆盖更晚开始的 latest。
type endpointScheduleSnapshotStore struct {
	mu             sync.RWMutex
	nextSequence   uint64
	latestSequence uint64
	latest         *EndpointScheduleSnapshot
	pending        map[string]endpointScheduleSnapshotEntry
}

func newEndpointScheduleSnapshotStore() *endpointScheduleSnapshotStore {
	return &endpointScheduleSnapshotStore{
		pending: make(map[string]endpointScheduleSnapshotEntry),
	}
}

func (s *endpointScheduleSnapshotStore) saveDraft(requestID, requestPath string, snapshot *EndpointScheduleSnapshot) {
	if s == nil || snapshot == nil {
		return
	}

	clone := cloneEndpointScheduleSnapshot(snapshot)
	now := time.Now()
	if clone.CapturedAt.IsZero() {
		clone.CapturedAt = now
	}
	clone.RequestID = requestID
	clone.RequestPath = requestPath
	clone.UpdatedAt = now
	if clone.FinalOutcome == "" {
		clone.FinalOutcome = EndpointScheduleOutcomePending
	}
	clone.Summary = endpointScheduleSummary(clone)

	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := now.Add(-endpointScheduleSnapshotPendingTTL)
	for id, entry := range s.pending {
		if entry.snapshot == nil || entry.snapshot.CapturedAt.Before(cutoff) {
			delete(s.pending, id)
		}
	}

	s.nextSequence++
	entry := endpointScheduleSnapshotEntry{
		sequence: s.nextSequence,
		snapshot: cloneEndpointScheduleSnapshot(clone),
	}
	if requestID != "" {
		s.pending[requestID] = entry
	}
	s.latestSequence = entry.sequence
	s.latest = clone
}

func (s *endpointScheduleSnapshotStore) updateAttempt(requestID, endpointName, outcome, runtimeError string) {
	if s == nil || endpointName == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.pending[requestID]
	if requestID == "" {
		ok = s.latest != nil && s.latest.RequestID == ""
		entry = endpointScheduleSnapshotEntry{sequence: s.latestSequence, snapshot: cloneEndpointScheduleSnapshot(s.latest)}
	}
	if !ok || entry.snapshot == nil {
		return
	}

	snapshot := entry.snapshot
	snapshot.SelectedEndpoint = endpointName
	snapshot.UpdatedAt = time.Now()
	for idx := range snapshot.Decisions {
		decision := &snapshot.Decisions[idx]
		if decision.Name != endpointName || decision.Decision != endpointDecisionCandidate {
			continue
		}
		decision.RuntimeOutcome = outcome
		decision.RuntimeError = runtimeError
		break
	}
	snapshot.Summary = endpointScheduleSummary(snapshot)
	entry.snapshot = cloneEndpointScheduleSnapshot(snapshot)
	if requestID != "" {
		s.pending[requestID] = entry
	}
	if entry.sequence == s.latestSequence {
		s.latest = cloneEndpointScheduleSnapshot(snapshot)
	}
}

func (s *endpointScheduleSnapshotStore) complete(requestID, endpointName, outcome, finalError string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.pending[requestID]
	if requestID == "" {
		ok = s.latest != nil && s.latest.RequestID == ""
		entry = endpointScheduleSnapshotEntry{sequence: s.latestSequence, snapshot: cloneEndpointScheduleSnapshot(s.latest)}
	}
	if !ok || entry.snapshot == nil {
		return
	}
	if requestID != "" {
		delete(s.pending, requestID)
	}

	snapshot := entry.snapshot
	if endpointName != "" {
		snapshot.SelectedEndpoint = endpointName
	}
	if outcome != "" {
		snapshot.FinalOutcome = outcome
	}
	snapshot.FinalError = finalError
	snapshot.UpdatedAt = time.Now()
	if endpointName != "" {
		for idx := range snapshot.Decisions {
			decision := &snapshot.Decisions[idx]
			if decision.Name != endpointName || decision.Decision != endpointDecisionCandidate {
				continue
			}
			decision.RuntimeOutcome = outcome
			decision.RuntimeError = finalError
			break
		}
	}
	snapshot.Summary = endpointScheduleSummary(snapshot)

	if entry.sequence == s.latestSequence {
		s.latest = cloneEndpointScheduleSnapshot(snapshot)
	}
}

func (s *endpointScheduleSnapshotStore) getLatest() *EndpointScheduleSnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneEndpointScheduleSnapshot(s.latest)
}

func cloneEndpointScheduleSnapshot(snapshot *EndpointScheduleSnapshot) *EndpointScheduleSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	if len(snapshot.Decisions) > 0 {
		clone.Decisions = make([]EndpointScheduleDecision, len(snapshot.Decisions))
		copy(clone.Decisions, snapshot.Decisions)
	}
	return &clone
}

func endpointScheduleSummary(snapshot *EndpointScheduleSnapshot) string {
	if snapshot == nil {
		return ""
	}
	switch snapshot.FinalOutcome {
	case EndpointScheduleOutcomePending:
		if snapshot.SelectedEndpoint != "" {
			return fmt.Sprintf("正在尝试端点 %s", snapshot.SelectedEndpoint)
		}
		return "调度处理中"
	case EndpointScheduleOutcomeSuccess:
		return fmt.Sprintf("成功命中端点 %s", snapshot.SelectedEndpoint)
	case EndpointScheduleOutcomeQualityIncomplete:
		return fmt.Sprintf("端点 %s 返回了不完整响应", snapshot.SelectedEndpoint)
	case EndpointScheduleOutcomePassthroughRaw:
		return fmt.Sprintf("原样返回端点 %s 的客户端错误", snapshot.SelectedEndpoint)
	case EndpointScheduleOutcomeNoCandidates, EndpointScheduleOutcomeManualFixedBlocked:
		return "本次请求没有可路由端点"
	case EndpointScheduleOutcomeAllCandidatesFailed:
		return "全部候选端点均失败"
	case EndpointScheduleOutcomeCancelled:
		return "请求已取消"
	default:
		if snapshot.SelectedEndpoint != "" {
			return fmt.Sprintf("端点 %s 调度结束：%s", snapshot.SelectedEndpoint, snapshot.FinalOutcome)
		}
		return snapshot.FinalOutcome
	}
}

// BeginEndpointScheduleSnapshot 保存本次调度草稿。传入 nil 时用于记录调度前置拒绝。
func (m *Manager) BeginEndpointScheduleSnapshot(requestID, requestPath string, snapshot *EndpointScheduleSnapshot) {
	if m == nil || m.scheduleSnapshots == nil {
		return
	}
	if snapshot == nil {
		cfg := m.getConfigSnapshot()
		override := m.routeOverride.Snapshot()
		active, _ := m.GetActiveEndpointSelection()
		snapshot = &EndpointScheduleSnapshot{
			CapturedAt:                time.Now(),
			ActiveEndpointAtSelection: active,
			RouteMode:                 override.Mode,
			RouteEndpointName:         override.EndpointName,
			RouteFallbackEnabled:      override.FallbackEnabled,
			FailoverEnabled:           cfg.Failover.Enabled,
		}
	}
	m.scheduleSnapshots.saveDraft(requestID, requestPath, snapshot)
}

// RecordEndpointScheduleAttempt 更新候选端点的运行结果。
func (m *Manager) RecordEndpointScheduleAttempt(requestID, endpointName, outcome, runtimeError string) {
	if m == nil || m.scheduleSnapshots == nil {
		return
	}
	m.scheduleSnapshots.updateAttempt(requestID, endpointName, outcome, runtimeError)
}

// CompleteEndpointScheduleSnapshot 写入最终 Outcome。
func (m *Manager) CompleteEndpointScheduleSnapshot(requestID, endpointName, outcome, finalError string) {
	if m == nil || m.scheduleSnapshots == nil {
		return
	}
	m.scheduleSnapshots.complete(requestID, endpointName, outcome, finalError)
}

// GetLatestEndpointScheduleSnapshot 返回深拷贝，避免 UI 读取与管线更新产生竞态。
func (m *Manager) GetLatestEndpointScheduleSnapshot() *EndpointScheduleSnapshot {
	if m == nil || m.scheduleSnapshots == nil {
		return nil
	}
	return m.scheduleSnapshots.getLatest()
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
		Snapshot: &EndpointScheduleSnapshot{
			CapturedAt:                time.Now(),
			ActiveEndpointAtSelection: activeName,
			RouteMode:                 override.Mode,
			RouteEndpointName:         override.EndpointName,
			RouteFallbackEnabled:      override.FallbackEnabled,
			FailoverEnabled:           cfg.Failover.Enabled,
			FinalOutcome:              EndpointScheduleOutcomePending,
		},
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
	result.Snapshot.Decisions = orderEndpointScheduleDecisions(result.Snapshot.Decisions, result.Candidates)
	return result
}

// orderEndpointScheduleDecisions 让候选决策顺序与实际尝试顺序一致，
// 跳过项保持原解释顺序并排在候选项之后。
func orderEndpointScheduleDecisions(decisions []EndpointScheduleDecision, candidates []*Endpoint) []EndpointScheduleDecision {
	if len(decisions) == 0 || len(candidates) == 0 {
		return decisions
	}
	byName := make(map[string]EndpointScheduleDecision, len(decisions))
	for _, decision := range decisions {
		if decision.Decision == endpointDecisionCandidate {
			byName[decision.Name] = decision
		}
	}

	ordered := make([]EndpointScheduleDecision, 0, len(decisions))
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if decision, ok := byName[candidate.Config.Name]; ok {
			ordered = append(ordered, decision)
			delete(byName, candidate.Config.Name)
		}
	}
	for _, decision := range decisions {
		if decision.Decision != endpointDecisionCandidate {
			ordered = append(ordered, decision)
			continue
		}
		if _, ok := byName[decision.Name]; ok {
			ordered = append(ordered, decision)
			delete(byName, decision.Name)
		}
	}
	return ordered
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
