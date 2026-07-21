// failover.go - 故障转移相关功能
// 包含请求级故障转移、冷却机制、端点切换等

package endpoint

import (
	"fmt"
	"log/slog"
	"time"
)

// SetOnFailoverTriggered 设置故障转移回调
// 当请求失败触发故障转移时调用，用于同步数据库
func (m *Manager) SetOnFailoverTriggered(fn func(failedEndpoint, newEndpoint string)) {
	m.onFailoverTriggered = fn
}

// IsEndpointInCooldown 检查端点是否在冷却中
func (m *Manager) IsEndpointInCooldown(name string) bool {
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return false
	}

	ep.mutex.RLock()
	defer ep.mutex.RUnlock()

	return !ep.Status.CooldownUntil.IsZero() && time.Now().Before(ep.Status.CooldownUntil)
}

// ClearEndpointCooldown 清除端点冷却状态（用于手动激活时）
func (m *Manager) ClearEndpointCooldown(name string) {
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return
	}

	ep.mutex.Lock()
	defer ep.mutex.Unlock()

	if !ep.Status.CooldownUntil.IsZero() {
		slog.Info(fmt.Sprintf("🔓 [冷却] 清除端点冷却: %s (原因: %s)", name, ep.Status.CooldownReason))
		ep.Status.CooldownUntil = time.Time{}
		ep.Status.CooldownReason = ""
	}
}

// GetEndpointCooldownInfo 获取端点冷却信息
func (m *Manager) GetEndpointCooldownInfo(name string) (inCooldown bool, until time.Time, reason string) {
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return false, time.Time{}, ""
	}

	ep.mutex.RLock()
	defer ep.mutex.RUnlock()

	now := time.Now()
	inCooldown = !ep.Status.CooldownUntil.IsZero() && now.Before(ep.Status.CooldownUntil)
	return inCooldown, ep.Status.CooldownUntil, ep.Status.CooldownReason
}

// SetEndpointCooldown 设置端点冷却（新转发管线的失败标记入口）
func (m *Manager) SetEndpointCooldown(name string, duration time.Duration, reason string) {
	if duration <= 0 {
		return
	}
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return
	}
	ep.mutex.Lock()
	ep.Status.CooldownUntil = time.Now().Add(duration)
	ep.Status.CooldownReason = reason
	ep.mutex.Unlock()
}

// PauseEndpoint 手动暂停端点至指定时刻（兼容层 ManualPauseGroup 语义）
func (m *Manager) PauseEndpoint(name string, until time.Time) bool {
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return false
	}
	ep.mutex.Lock()
	ep.Status.PausedUntil = until
	ep.mutex.Unlock()
	return true
}

// ResumeEndpoint 清除端点手动暂停状态
func (m *Manager) ResumeEndpoint(name string) bool {
	return m.PauseEndpoint(name, time.Time{})
}
