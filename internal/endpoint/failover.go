// failover.go - 故障转移相关功能
// 包含请求级故障转移、冷却机制、端点切换等

package endpoint

import (
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// SetOnFailoverTriggered 设置故障转移回调
// 当请求失败触发故障转移时调用，用于同步数据库
func (m *Manager) SetOnFailoverTriggered(fn func(failedEndpoint, newEndpoint string)) {
	m.onFailoverTriggered = fn
}

// TriggerRequestFailover 触发请求级故障转移
// 当请求在某端点上失败达到重试上限时调用
// 返回: 新激活的端点名，如果没有可用端点则返回空字符串
func (m *Manager) TriggerRequestFailover(failedEndpointName string, reason string) (string, error) {
	cfg := m.getConfigSnapshot()

	// 🔧 [热更新修复] 检查故障转移开关
	if !cfg.Failover.Enabled {
		slog.Info(fmt.Sprintf("⏭️ [故障转移] 故障转移已禁用，跳过: %s", failedEndpointName))
		return "", fmt.Errorf("故障转移已禁用")
	}

	slog.Warn(fmt.Sprintf("🔄 [故障转移] 触发请求级故障转移: %s, 原因: %s", failedEndpointName, reason))

	// 1. 找到失败的端点并设置冷却
	failedEndpoint := m.GetEndpointByNameAny(failedEndpointName)
	if failedEndpoint == nil {
		return "", fmt.Errorf("端点 %s 不存在", failedEndpointName)
	}

	// 计算冷却时间
	cooldownDuration := cfg.Failover.DefaultCooldown
	if cooldownDuration == 0 {
		cooldownDuration = 10 * time.Minute // 默认 10 分钟
	}
	// 如果端点有自定义冷却时间，使用端点配置
	if failedEndpoint.Config.Cooldown != nil && *failedEndpoint.Config.Cooldown > 0 {
		cooldownDuration = *failedEndpoint.Config.Cooldown
	}

	// 设置冷却状态
	failedEndpoint.mutex.Lock()
	failedEndpoint.Status.CooldownUntil = time.Now().Add(cooldownDuration)
	failedEndpoint.Status.CooldownReason = reason
	failedEndpoint.mutex.Unlock()

	slog.Info(fmt.Sprintf("⏱️ [故障转移] 端点 %s 进入冷却，持续 %v", failedEndpointName, cooldownDuration))

	// 2. 停用失败端点的组
	if err := m.groupManager.DeactivateGroup(failedEndpointName); err != nil {
		slog.Warn(fmt.Sprintf("⚠️ [故障转移] 停用组失败: %v", err))
	}

	// 3. 选择下一个可用端点
	newEndpointName := m.selectNextFailoverEndpoint(failedEndpointName)
	if newEndpointName == "" {
		slog.Error("❌ [故障转移] 没有可用的故障转移端点")
		return "", fmt.Errorf("没有可用的故障转移端点")
	}

	// 4. 激活新端点
	if err := m.groupManager.ManualActivateGroup(newEndpointName); err != nil {
		slog.Error(fmt.Sprintf("❌ [故障转移] 激活新端点失败: %v", err))
		return "", fmt.Errorf("激活新端点失败: %w", err)
	}

	slog.Info(fmt.Sprintf("✅ [故障转移] 已切换到端点: %s", newEndpointName))

	// 5. 调用回调通知 App 层同步数据库
	if m.onFailoverTriggered != nil {
		go m.onFailoverTriggered(failedEndpointName, newEndpointName)
	}

	// 6. 触发前端刷新
	if m.onHealthCheckComplete != nil {
		go m.onHealthCheckComplete()
	}

	return newEndpointName, nil
}

// selectNextFailoverEndpoint 选择下一个故障转移端点
func (m *Manager) selectNextFailoverEndpoint(excludeEndpoint string) string {
	cfg := m.getConfigSnapshot()

	m.endpointsMu.RLock()
	snapshot := make([]*Endpoint, len(m.endpoints))
	copy(snapshot, m.endpoints)
	m.endpointsMu.RUnlock()

	// 按优先级排序
	sort.Slice(snapshot, func(i, j int) bool {
		return snapshot[i].Config.Priority < snapshot[j].Config.Priority
	})

	now := time.Now()
	for _, ep := range snapshot {
		// 跳过失败的端点
		if ep.Config.Name == excludeEndpoint {
			continue
		}

		// 检查是否参与故障转移
		failoverEnabled := true
		if ep.Config.FailoverEnabled != nil {
			failoverEnabled = *ep.Config.FailoverEnabled
		}
		if !failoverEnabled {
			continue
		}

		// 📊 [失败追踪] 达到阈值的端点不应作为故障转移目标
		if cfg.FailureTracker.Enabled && m.failureTracker.ShouldTriggerAction(ep.Config.Name) {
			slog.Debug(fmt.Sprintf("📊 [失败追踪] 跳过达到失败阈值的故障转移端点: %s", ep.Config.Name))
			continue
		}

		// 检查是否在冷却中
		ep.mutex.RLock()
		inCooldown := !ep.Status.CooldownUntil.IsZero() && now.Before(ep.Status.CooldownUntil)
		ep.mutex.RUnlock()

		if inCooldown {
			slog.Debug(fmt.Sprintf("⏭️ [故障转移] 跳过冷却中的端点: %s", ep.Config.Name))
			continue
		}

		return ep.Config.Name
	}

	return ""
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
