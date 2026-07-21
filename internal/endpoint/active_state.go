package endpoint

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
)

// activeEndpoint 变更统一入口（方案 §4.1 规则 3 / §4.3 / §4.6）。
// 五档变更：手动激活、端点页停用 active、启动恢复、成功驱动迁移、端点删除；
// 持久化统一经 RuntimeWriter，手动路径等待 ACK（返回成功 ⇒ 已落库）。

// SetRuntimeWriter 注入持久化 writer（非 SQLite 模式传 nil，仅内存生效）
func (m *Manager) SetRuntimeWriter(w *RuntimeWriter) {
	m.activeMu.Lock()
	defer m.activeMu.Unlock()
	m.runtimeWriter = w
}

// GetActiveEndpointSelection 返回当前 active 端点名与 revision
func (m *Manager) GetActiveEndpointSelection() (string, int64) {
	m.activeMu.Lock()
	defer m.activeMu.Unlock()
	return m.activeEndpoint, m.activeRevision
}

// IsCurrentActiveRevision 供 RuntimeWriter 过期判定
func (m *Manager) IsCurrentActiveRevision(revision int64) bool {
	m.activeMu.Lock()
	defer m.activeMu.Unlock()
	return m.activeRevision == revision
}

// ActivateEndpointManually 手动激活（无条件档）。
// 有 writer 时等待 ACK：返回 nil ⇒ 已落库；写库失败时回滚内存到 lastPersisted
// 并返回错误（v6 语义，强于旧实现的"仅 Warn 继续激活"）。
func (m *Manager) ActivateEndpointManually(name string) error {
	if m.GetEndpointByNameAny(name) == nil {
		return fmt.Errorf("端点 '%s' 不存在", name)
	}

	m.activeMu.Lock()
	m.activeEndpoint = name
	m.activeRevision++
	revision := m.activeRevision
	writer := m.runtimeWriter
	m.activeMu.Unlock()

	if writer == nil {
		return nil
	}
	err := writer.EnqueueManual(endpointTaskActivate, name, revision)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrSuperseded) {
		return fmt.Errorf("激活操作已被更新的变更取代: %w", err)
	}
	m.rollbackActiveTo(revision)
	return fmt.Errorf("激活端点 '%s' 持久化失败，已回滚: %w", name, err)
}

// DeactivateActiveEndpointManually 端点页停用当前 active 端点（v6 纳入）：
// 按 revision 顺序清空内存 active，并由 writer 协调该端点自身的 DB disable 写入。
func (m *Manager) DeactivateActiveEndpointManually(name string) error {
	m.activeMu.Lock()
	if m.activeEndpoint != name {
		m.activeMu.Unlock()
		return fmt.Errorf("端点 '%s' 不是当前激活端点", name)
	}
	m.activeEndpoint = ""
	m.activeRevision++
	revision := m.activeRevision
	writer := m.runtimeWriter
	m.activeMu.Unlock()

	if writer == nil {
		return nil
	}
	err := writer.EnqueueManual(endpointTaskDisable, name, revision)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrSuperseded) {
		return fmt.Errorf("停用操作已被更新的变更取代: %w", err)
	}
	m.rollbackActiveTo(revision)
	return fmt.Errorf("停用端点 '%s' 持久化失败，已回滚: %w", name, err)
}

// rollbackActiveTo 手动任务写库失败后的内存回滚（v6 修订）：
// 仅当 revision 仍等于失败操作的 revision 时回滚到 lastPersisted（最后确认落库状态，
// 不能回滚到"操作前内存值"——它可能是未落库的自动迁移中间态）；
// lastPersisted 未确立（degraded）时回滚为清空 active（保守基准）。
// 回滚产生新 revision（防止临时状态期间取得的调度快照仍通过 CAS），不入队。
func (m *Manager) rollbackActiveTo(failedRevision int64) {
	m.activeMu.Lock()
	defer m.activeMu.Unlock()
	if m.activeRevision != failedRevision {
		return // 已有更新 revision，不得覆盖新状态
	}
	lastName, _, established := m.runtimeWriter.LastPersisted()
	if !established {
		slog.Warn("端点激活基准未确立（degraded），回滚为清空 active")
		lastName = ""
	}
	m.activeEndpoint = lastName
	m.activeRevision++
}

// RestoreActiveEndpoint 启动恢复档：仅内存 restore，不入队、不写回刚读取的 DB（§4.6）
func (m *Manager) RestoreActiveEndpoint(name string) {
	m.activeMu.Lock()
	defer m.activeMu.Unlock()
	m.activeEndpoint = name
	m.activeRevision++
}

// TryMigrateActiveEndpoint 成功驱动迁移（CAS 档，§4.3）。
// 请求在候选 B 上以 FullSuccess 完成时调用；CAS 失败放弃迁移并返回 false。
func (m *Manager) TryMigrateActiveEndpoint(candidate string, expectedActiveRevision, expectedRouteOverrideRevision int64) bool {
	override := m.routeOverride.Snapshot()
	if override.Revision != expectedRouteOverrideRevision {
		return false
	}

	current, _ := m.GetActiveEndpointSelection()
	if allowed, reason := m.AllowSystemRouteSwitch(current, candidate, RouteCallerSystemFailoverRequest); !allowed {
		slog.Debug("成功驱动迁移被路由模式拒绝", "candidate", candidate, "reason", reason)
		return false
	}

	ep := m.GetEndpointByNameAny(candidate)
	if ep == nil || !m.IsEndpointRoutable(ep) || ep.IsPaused() {
		return false
	}

	m.activeMu.Lock()
	if m.activeRevision != expectedActiveRevision {
		m.activeMu.Unlock()
		slog.Debug("成功驱动迁移 CAS 失败，放弃迁移", "candidate", candidate,
			"expected_revision", expectedActiveRevision, "current_revision", m.activeRevision)
		return false
	}
	m.activeEndpoint = candidate
	m.activeRevision++
	revision := m.activeRevision
	writer := m.runtimeWriter
	m.activeMu.Unlock()

	if writer != nil {
		writer.EnqueueAuto(endpointTaskActivate, candidate, revision)
	}
	return true
}

// DeleteEndpointCoordinated 端点删除协调（v7）：删除经 writer 串行队列，
// DB 删除成功后才同步内存移除；DB 删除失败时内存不动、返回错误
// （替代旧实现"先删内存再删 DB、失败不恢复"的顺序）。
func (m *Manager) DeleteEndpointCoordinated(name string) error {
	m.activeMu.Lock()
	writer := m.runtimeWriter
	m.activeMu.Unlock()

	if writer != nil {
		if err := writer.EnqueueManual(endpointTaskDelete, name, 0); err != nil {
			return fmt.Errorf("删除端点 '%s' 持久化失败（内存未变更）: %w", name, err)
		}
	}

	m.activeMu.Lock()
	if m.activeEndpoint == name {
		m.activeEndpoint = ""
		m.activeRevision++
	}
	m.activeMu.Unlock()

	if err := m.RemoveEndpoint(name); err != nil {
		slog.Warn("从运行时管理器移除端点失败", "endpoint", name, "error", err)
	}
	return nil
}

// ReconcileActiveEndpointOnReload 配置热更新档：active 端点仍存在则保留，不存在则清空
func (m *Manager) ReconcileActiveEndpointOnReload() {
	current, _ := m.GetActiveEndpointSelection()
	if current == "" || m.GetEndpointByNameAny(current) != nil {
		return
	}
	m.activeMu.Lock()
	if m.activeEndpoint == current {
		m.activeEndpoint = ""
		m.activeRevision++
	}
	m.activeMu.Unlock()
}

// EnabledEndpointRecord 启动恢复输入（来自 DB 的 enabled 行）
type EnabledEndpointRecord struct {
	Name     string
	Priority int
	ID       int64
}

// RestoreActiveFromEnabled 启动恢复：按 §4.6 处理 0/1/N enabled 三态。
//   - 0 个：不设置 active，lastPersisted 初始化为空；
//   - 1 个：内存恢复该端点，lastPersisted 初始化为该端点；
//   - N 个（脏数据）：按确定性规则选 winner（priority 最小，并列取 ID 最小），
//     内存恢复 winner 后同步提交修复事务（仅 winner enabled）并等待 ACK；
//     事务成功才确立 lastPersisted；失败进入 degraded+retry（应用继续启动，
//     修复任务转入自动重试队列，lastPersisted 保持未确立）。
//
// 返回值 degraded=true 表示多 enabled 修复事务失败，当前处于 degraded 状态。
func (m *Manager) RestoreActiveFromEnabled(records []EnabledEndpointRecord) (active string, degraded bool) {
	m.activeMu.Lock()
	writer := m.runtimeWriter
	m.activeMu.Unlock()

	switch len(records) {
	case 0:
		m.activeMu.Lock()
		m.activeEndpoint = ""
		m.activeRevision++
		revision := m.activeRevision
		m.activeMu.Unlock()
		writer.InitializeLastPersisted("", revision)
		return "", false
	case 1:
		m.RestoreActiveEndpoint(records[0].Name)
		_, revision := m.GetActiveEndpointSelection()
		writer.InitializeLastPersisted(records[0].Name, revision)
		return records[0].Name, false
	}

	winner := pickEnabledWinner(records)
	slog.Warn("检测到多个 enabled 端点（脏数据），按确定性规则修复",
		"enabled_count", len(records), "winner", winner.Name)

	m.RestoreActiveEndpoint(winner.Name)
	_, revision := m.GetActiveEndpointSelection()

	if writer == nil {
		return winner.Name, false
	}

	// v7：同步提交修复事务，成功才确立 persisted 基准
	if err := writer.EnqueueManual(endpointTaskActivate, winner.Name, revision); err != nil {
		slog.Error("多 enabled 修复事务失败，进入 degraded 状态并转入重试",
			"winner", winner.Name, "error", err)
		if !errors.Is(err, ErrSuperseded) {
			writer.EnqueueAuto(endpointTaskActivate, winner.Name, revision)
		}
		return winner.Name, true
	}
	return winner.Name, false
}

func pickEnabledWinner(records []EnabledEndpointRecord) EnabledEndpointRecord {
	sorted := make([]EnabledEndpointRecord, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted[0]
}
