package endpoint

import (
	"sort"
	"time"
)

// 兼容层（方案 §6）：组体系退役后由端点运行态合成组视图（组名=端点名）。
// IsActive = (name == activeEndpoint)；冷却/暂停映射端点状态。

// GroupCompatView 兼容层组视图
type GroupCompatView struct {
	Name           string
	Channel        string
	Priority       int
	IsActive       bool
	ManuallyPaused bool
	InCooldown     bool
	CooldownRemain time.Duration
	Healthy        bool
	NeverChecked   bool
}

// GetGroupCompatViews 从端点运行态合成所有组视图
func (m *Manager) GetGroupCompatViews() []GroupCompatView {
	activeName, _ := m.GetActiveEndpointSelection()

	m.endpointsMu.RLock()
	snapshot := make([]*Endpoint, len(m.endpoints))
	copy(snapshot, m.endpoints)
	m.endpointsMu.RUnlock()

	now := time.Now()
	views := make([]GroupCompatView, 0, len(snapshot))
	for _, ep := range snapshot {
		if ep == nil {
			continue
		}
		ep.mutex.RLock()
		cooldownUntil := ep.Status.CooldownUntil
		pausedUntil := ep.Status.PausedUntil
		healthy := ep.Status.Healthy
		neverChecked := ep.Status.NeverChecked
		ep.mutex.RUnlock()

		view := GroupCompatView{
			Name:           ep.Config.Name,
			Channel:        ep.Config.Channel,
			Priority:       ep.Config.Priority,
			IsActive:       ep.Config.Name == activeName,
			ManuallyPaused: !pausedUntil.IsZero() && now.Before(pausedUntil),
			Healthy:        healthy,
			NeverChecked:   neverChecked,
		}
		if !cooldownUntil.IsZero() && now.Before(cooldownUntil) {
			view.InCooldown = true
			view.CooldownRemain = cooldownUntil.Sub(now)
		}
		views = append(views, view)
	}
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Priority == views[j].Priority {
			return views[i].Name < views[j].Name
		}
		return views[i].Priority < views[j].Priority
	})
	return views
}

// GetActiveGroupName 兼容层：当前激活组名（= activeEndpoint，可能为空）
func (m *Manager) GetActiveGroupName() string {
	name, _ := m.GetActiveEndpointSelection()
	return name
}

// GetGroupDetails 保留旧事件载荷结构，数据全部从端点运行态合成。
func (m *Manager) GetGroupDetails() map[string]interface{} {
	views := m.GetGroupCompatViews()
	groups := make([]map[string]interface{}, 0, len(views))
	activeCount := 0

	for _, view := range views {
		if view.IsActive {
			activeCount++
		}

		healthyCount := 0
		unhealthyCount := 0
		uncheckedCount := 0
		switch {
		case view.NeverChecked:
			uncheckedCount = 1
		case view.Healthy:
			healthyCount = 1
		default:
			unhealthyCount = 1
		}

		status := "可用"
		statusColor := "secondary"
		switch {
		case view.IsActive:
			status = "活跃"
			statusColor = "success"
		case view.ManuallyPaused:
			status = "手动暂停"
			statusColor = "warning"
		case view.InCooldown:
			status = "冷却中"
			statusColor = "danger"
		}

		groups = append(groups, map[string]interface{}{
			"name":                   view.Name,
			"priority":               view.Priority,
			"is_active":              view.IsActive,
			"status":                 status,
			"status_color":           statusColor,
			"total_endpoints":        1,
			"healthy_endpoints":      healthyCount,
			"unhealthy_endpoints":    unhealthyCount,
			"unchecked_endpoints":    uncheckedCount,
			"manually_paused":        view.ManuallyPaused,
			"in_cooldown":            view.InCooldown,
			"cooldown_remaining":     view.CooldownRemain.Round(time.Second).String(),
			"can_activate":           !view.IsActive && !view.InCooldown,
			"can_pause":              !view.ManuallyPaused,
			"can_resume":             view.ManuallyPaused,
			"forced_activation":      false,
			"forced_activation_time": "",
			"activation_type":        "normal",
			"can_force_activate":     !view.IsActive && !view.InCooldown,
		})
	}

	return map[string]interface{}{
		"groups":        groups,
		"total_groups":  len(groups),
		"active_groups": activeCount,
	}
}
