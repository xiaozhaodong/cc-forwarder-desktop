// app_api_group.go - 组管理 API (Wails Bindings)
// 包含组状态查询、激活、暂停、恢复等功能
// 2025-12-06 10:49:09 v5.0: 激活端点时同步到数据库

package main

import (
	"context"
	"fmt"
	"time"
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

// GetGroups 获取所有组状态
func (a *App) GetGroups() []GroupInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointManager == nil {
		return []GroupInfo{}
	}

	gm := a.endpointManager.GetGroupManager()
	if gm == nil {
		return []GroupInfo{}
	}

	groups := gm.GetAllGroups()
	result := make([]GroupInfo, 0, len(groups))

	for _, g := range groups {
		// 从第一个端点获取渠道名称
		channel := ""
		if len(g.Endpoints) > 0 {
			channel = g.Endpoints[0].Config.Channel
		}

		info := GroupInfo{
			Name:          g.Name,
			Channel:       channel,
			Active:        g.IsActive,
			Paused:        g.ManuallyPaused,
			Priority:      g.Priority,
			EndpointCount: len(g.Endpoints),
			InCooldown:    gm.IsGroupInCooldown(g.Name),
		}

		// 获取冷却剩余时间
		remaining := gm.GetGroupCooldownRemaining(g.Name)
		if remaining > 0 {
			info.CooldownRemainMs = remaining.Milliseconds()
		}

		result = append(result, info)
	}

	return result
}

// ActivateGroup 激活指定组（端点）
// v5.0: 同时更新数据库中的 enabled 状态
func (a *App) ActivateGroup(name string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointManager == nil {
		return fmt.Errorf("端点管理器未初始化")
	}

	// v5.0: 如果有 endpointService，同步到数据库
	if a.endpointService != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// 1. 先禁用所有端点
		if err := a.endpointService.DisableAllEndpoints(ctx); err != nil {
			a.logger.Warn("禁用所有端点失败", "error", err)
			// 继续执行，不阻断流程
		}

		// 2. 启用选中的端点
		if err := a.endpointService.ToggleEndpoint(ctx, name, true); err != nil {
			a.logger.Warn("启用端点失败", "endpoint", name, "error", err)
			// 继续执行内存激活
		} else {
			a.logger.Info("✅ 端点已同步到数据库", "endpoint", name, "enabled", true)
		}
	}

	// 3. 内存中激活组
	return a.endpointManager.ManualActivateGroup(name)
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
