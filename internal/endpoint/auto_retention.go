// Package endpoint - v8 Auto retained 运行态（收敛方案 §8.4）
// 自动模式在同一 priority 层内的成功粘性；纯运行态，不落库。
package endpoint

import (
	"sync"
	"time"
)

// endpointAutoRetention 同层级成功粘性记录
type endpointAutoRetention struct {
	EndpointName string
	Priority     int
	SelectedAt   time.Time
	LastSuccess  time.Time
}

type autoRetentionState struct {
	mu sync.Mutex
	// 按 priority 层保存；低层 retained 不阻挡高层恢复（§8.4 规则 6 由调度层保证）
	byTier map[int]*endpointAutoRetention
}

func newAutoRetentionState() *autoRetentionState {
	return &autoRetentionState{byTier: make(map[int]*endpointAutoRetention)}
}

// UpdateAutoRetention 为自动候选或 manual preferred 的自动 fallback 建立/刷新 retained。
// 用户显式 preferred/fixed 目标的成功不写 retained；路由意图已变化时放弃更新。
func (m *Manager) UpdateAutoRetention(endpointName string, priority int, selectionSource string, routeRevisionAtSelection int64) bool {
	override := m.routeOverride.Snapshot()
	if override.Revision != routeRevisionAtSelection {
		return false
	}
	switch selectionSource {
	case "auto_priority", "auto_retained", "fallback":
	default:
		return false
	}
	ep := m.GetEndpointByNameAny(endpointName)
	if ep == nil || !m.EndpointHardEnabled(ep) || !m.IsEndpointRoutable(ep) {
		return false
	}

	m.autoRetention.mu.Lock()
	defer m.autoRetention.mu.Unlock()
	now := time.Now()
	entry := m.autoRetention.byTier[priority]
	if entry == nil || entry.EndpointName != endpointName {
		entry = &endpointAutoRetention{EndpointName: endpointName, Priority: priority, SelectedAt: now}
	}
	entry.LastSuccess = now
	m.autoRetention.byTier[priority] = entry
	return true
}

// RetainedInTier 返回指定层级的 retained 端点名（无则空）
func (m *Manager) RetainedInTier(priority int) string {
	m.autoRetention.mu.Lock()
	defer m.autoRetention.mu.Unlock()
	if entry := m.autoRetention.byTier[priority]; entry != nil {
		return entry.EndpointName
	}
	return ""
}

// ClearAutoRetentionFor 清除指定端点的 retained（删除/硬停用/暂停/冷却/负缓存时，§8.4 规则 3）
func (m *Manager) ClearAutoRetentionFor(endpointName string) {
	m.autoRetention.mu.Lock()
	defer m.autoRetention.mu.Unlock()
	for tier, entry := range m.autoRetention.byTier {
		if entry != nil && entry.EndpointName == endpointName {
			delete(m.autoRetention.byTier, tier)
		}
	}
}
