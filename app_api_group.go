// app_api_group.go - 组管理 API (Wails Bindings)
// 包含组状态查询、激活、暂停、恢复等功能
// 2025-12-06 10:49:09 v5.0: 激活端点时同步到数据库

package main

import (
	"fmt"
	"time"

	"cc-forwarder/internal/endpoint"
)

// ============================================================
// 组管理 API
// ============================================================

// GroupInfo 组信息
type GroupInfo struct {
	Name             string `json:"name"`
	Channel          string `json:"channel"` // v5.0: 渠道名称（从端点配置获取）
	Active           bool   `json:"active"`
	Paused           bool   `json:"paused"`
	Priority         int    `json:"priority"`
	EndpointCount    int    `json:"endpoint_count"`
	InCooldown       bool   `json:"in_cooldown"`
	CooldownRemainMs int64  `json:"cooldown_remain_ms"`
}

// GetGroups 获取所有组状态（v7 兼容层：组名=端点名，自端点运行态合成）
func (a *App) GetGroups() []GroupInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointManager == nil {
		return []GroupInfo{}
	}

	views := a.endpointManager.GetGroupCompatViews()
	result := make([]GroupInfo, 0, len(views))
	for _, view := range views {
		info := GroupInfo{
			Name:          view.Name,
			Channel:       view.Channel,
			Active:        view.IsActive,
			Paused:        view.ManuallyPaused,
			Priority:      view.Priority,
			EndpointCount: 1,
			InCooldown:    view.InCooldown,
		}
		if view.CooldownRemain > 0 {
			info.CooldownRemainMs = view.CooldownRemain.Milliseconds()
		}
		result = append(result, info)
	}

	return result
}

// ActivateGroup 激活指定组（端点），并记录为 Claude 手动优选路由。
func (a *App) ActivateGroup(name string) error {
	a.mu.RLock()
	manager := a.endpointManager
	a.mu.RUnlock()

	if manager == nil {
		return fmt.Errorf("端点管理器未初始化")
	}

	_, err := a.SetClaudeRoutingOverride(SetClaudeRoutingOverrideInput{
		Mode:            endpoint.RouteModeManualPreferred,
		EndpointName:    name,
		FallbackEnabled: true,
	})
	return err
}

// PauseGroup 暂停指定组
func (a *App) PauseGroup(name string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointManager == nil {
		return fmt.Errorf("端点管理器未初始化")
	}

	// 默认暂停 1 小时
	return a.endpointManager.ManualPauseGroup(name, time.Hour)
}

// ResumeGroup 恢复指定组
func (a *App) ResumeGroup(name string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointManager == nil {
		return fmt.Errorf("端点管理器未初始化")
	}

	return a.endpointManager.ManualResumeGroup(name)
}
