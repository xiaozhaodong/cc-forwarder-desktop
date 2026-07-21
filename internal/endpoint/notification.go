// notification.go - 通知相关功能
// 包含 EventBus 事件发布、组状态通知、Web 界面通知等

package endpoint

import (
	"fmt"
	"log/slog"
	"time"

	"cc-forwarder/internal/events"
	"cc-forwarder/internal/utils"
)

// SetEventBus 设置EventBus事件总线
func (m *Manager) SetEventBus(eventBus events.EventBus) {
	m.eventBus = eventBus
}

// notifyWebInterface 通过EventBus发布端点状态变化事件
func (m *Manager) notifyWebInterface(endpoint *Endpoint) {
	if m.eventBus == nil {
		return
	}

	endpoint.mutex.RLock()
	status := endpoint.Status
	endpoint.mutex.RUnlock()

	// 确定事件类型和优先级
	eventType := events.EventEndpointHealthy
	priority := events.PriorityHigh
	changeType := "status_changed"

	if !status.Healthy {
		eventType = events.EventEndpointUnhealthy
		priority = events.PriorityCritical
		changeType = "health_changed"
	}

	m.eventBus.Publish(events.Event{
		Type:     eventType,
		Source:   "endpoint_manager",
		Priority: priority,
		Data: map[string]interface{}{
			"endpoint":          endpoint.Config.Name,
			"healthy":           status.Healthy,
			"response_time":     utils.FormatResponseTime(status.ResponseTime),
			"last_check":        status.LastCheck.Format("2006-01-02 15:04:05"),
			"consecutive_fails": status.ConsecutiveFails,
			"change_type":       changeType,
		},
	})
}

// ManualActivateGroup 兼容层（v7）：组名=端点名，激活走 activeEndpoint 统一入口
// （有 writer 时等待 ACK，返回成功 ⇒ 已落库）+ 清该端点冷却。
func (m *Manager) ManualActivateGroup(groupName string) error {
	if err := m.ActivateEndpointManually(groupName); err != nil {
		return err
	}

	// 清除端点的冷却状态（用户手动激活时取消冷却）
	m.ClearEndpointCooldown(groupName)

	// Notify web interface about group change
	go m.notifyWebGroupChange("group_manually_activated", groupName)

	return nil
}

// ManualActivateGroupWithForce 兼容层（v7）：force 语义已随组体系退役，与普通激活等价
func (m *Manager) ManualActivateGroupWithForce(groupName string, force bool) error {
	if err := m.ActivateEndpointManually(groupName); err != nil {
		return err
	}

	// 清除端点的冷却状态（用户手动激活时取消冷却）
	m.ClearEndpointCooldown(groupName)

	// Notify web interface about group change
	if force {
		go m.notifyWebGroupChange("group_force_activated", groupName)
	} else {
		go m.notifyWebGroupChange("group_manually_activated", groupName)
	}

	return nil
}

// ManualPauseGroup 兼容层（v7）：映射为端点 PausedUntil（到期读取时自愈，无恢复 goroutine）
func (m *Manager) ManualPauseGroup(groupName string, duration time.Duration) error {
	if !m.PauseEndpoint(groupName, time.Now().Add(duration)) {
		return fmt.Errorf("端点 '%s' 不存在", groupName)
	}

	// Notify web interface about group change
	go m.notifyWebGroupChange("group_manually_paused", groupName)

	return nil
}

// ManualResumeGroup 兼容层（v7）：清除端点 PausedUntil
func (m *Manager) ManualResumeGroup(groupName string) error {
	if !m.ResumeEndpoint(groupName) {
		return fmt.Errorf("端点 '%s' 不存在", groupName)
	}

	// Notify web interface about group change
	go m.notifyWebGroupChange("group_manually_resumed", groupName)

	return nil
}

// notifyWebGroupChange notifies the web interface about group management changes
func (m *Manager) notifyWebGroupChange(eventType, groupName string) {
	// 检查EventBus是否可用
	if m.eventBus == nil {
		slog.Debug("[组管理] EventBus未设置，跳过组状态变化通知")
		return
	}

	// 获取组详细信息
	groupDetails := m.GetGroupDetails()

	// 构建事件数据
	data := map[string]interface{}{
		"event":     eventType,
		"group":     groupName,
		"timestamp": time.Now().Format("2006-01-02 15:04:05"),
		"details":   groupDetails,
	}

	// 使用EventBus发布组状态变化事件
	m.eventBus.Publish(events.Event{
		Type:      events.EventGroupStatusChanged,
		Source:    "endpoint_manager",
		Timestamp: time.Now(),
		Priority:  events.PriorityHigh,
		Data:      data,
	})

	slog.Debug(fmt.Sprintf("📢 [组管理] 发布组状态变化事件: %s (组: %s)", eventType, groupName))
}
