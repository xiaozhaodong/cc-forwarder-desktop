// Package endpoint - v8 端点配置运行态发布（收敛方案 §7.6 / §8.1）
// hard availability 与 auto schedule 的运行时发布、pending gate 与 config revision。
// 写入顺序统一为 persist-then-publish；本文件只承担 publish 侧。
package endpoint

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// endpointConfigRevisions 端点配置修订号（全局单调递增；AttemptPlan CAS 依据）
var endpointConfigRevision atomic.Int64

// NextEndpointConfigRevision 生成新的配置 revision
func NextEndpointConfigRevision() int64 {
	return endpointConfigRevision.Add(1)
}

// pendingAvailabilityGates 停用中的安全阻断 gate（§7.6 规则 1）
type pendingGateSet struct {
	mu    sync.RWMutex
	gates map[string]bool
}

func newPendingGateSet() *pendingGateSet {
	return &pendingGateSet{gates: make(map[string]bool)}
}

func (p *pendingGateSet) set(name string, pending bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if pending {
		p.gates[name] = true
	} else {
		delete(p.gates, name)
	}
}

func (p *pendingGateSet) isPending(name string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.gates[name]
}

// SetPendingAvailabilityGate 设置/清除端点的 pending disable gate
func (m *Manager) SetPendingAvailabilityGate(name string, pending bool) {
	m.ensurePendingGates()
	m.pendingGates.set(name, pending)
}

// HasPendingAvailabilityGate 查询端点是否处于 pending disable 阻断
func (m *Manager) HasPendingAvailabilityGate(name string) bool {
	if m == nil || m.pendingGates == nil {
		return false
	}
	return m.pendingGates.isPending(name)
}

func (m *Manager) ensurePendingGates() {
	m.endpointsMu.Lock()
	defer m.endpointsMu.Unlock()
	if m.pendingGates == nil {
		m.pendingGates = newPendingGateSet()
	}
}

// PublishEndpointAvailability 原子发布硬启用运行态（copy-on-write：新指针值 + 新 revision）
func (m *Manager) PublishEndpointAvailability(name string, enabled bool) error {
	m.endpointConfigMu.Lock()
	defer m.endpointConfigMu.Unlock()

	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return fmt.Errorf("端点 '%s' 不在运行时", name)
	}
	value := enabled
	ep.mutex.Lock()
	ep.Config.AvailabilityEnabled = &value
	ep.configRevision = NextEndpointConfigRevision()
	ep.mutex.Unlock()

	// 硬停用即刻清除 retained（§8.4 规则 3）
	if !enabled {
		m.ClearAutoRetentionFor(name)
	}
	return nil
}

// PublishEndpointAutoSchedule 原子发布自动调度运行态
func (m *Manager) PublishEndpointAutoSchedule(name string, enabled bool) error {
	m.endpointConfigMu.Lock()
	defer m.endpointConfigMu.Unlock()

	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return fmt.Errorf("端点 '%s' 不在运行时", name)
	}
	value := enabled
	ep.mutex.Lock()
	ep.Config.FailoverEnabled = &value
	ep.configRevision = NextEndpointConfigRevision()
	ep.mutex.Unlock()
	return nil
}

// SetClaudeRoutingReady 设置 Claude 路由是否开放（§6.4：启动读取失败保持 not ready）
func (m *Manager) SetClaudeRoutingReady(ready bool) {
	m.routingNotReady.Store(!ready)
}

// IsClaudeRoutingReady Claude 路由是否开放
func (m *Manager) IsClaudeRoutingReady() bool {
	return !m.routingNotReady.Load()
}

// EndpointHardEnabled 端点硬启用状态（运行时读取，nil 默认 true）
func (m *Manager) EndpointHardEnabled(ep *Endpoint) bool {
	if ep == nil {
		return false
	}
	ep.mutex.RLock()
	defer ep.mutex.RUnlock()
	return ep.Config.IsAvailabilityEnabled()
}
