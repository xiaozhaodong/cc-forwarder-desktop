package main

import (
	"context"
	"fmt"
	"time"
)

// v8 端点调度状态 API（收敛方案 §7.5）。
// 用户路由选择统一走 SetClaudeRoutingOverride / ClearClaudeRoutingOverride；
// 本文件只承担配置资格（硬启用 / 参与自动调度）的写入。

// SetEndpointAvailability 设置端点硬启用状态；false 时任何路由模式都不可使用（D7）
func (a *App) SetEndpointAvailability(name string, enabled bool) error {
	a.mu.RLock()
	service := a.endpointService
	a.mu.RUnlock()
	if service == nil {
		return fmt.Errorf("端点存储服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := service.SetEndpointAvailability(ctx, name, enabled); err != nil {
		return err
	}
	a.emitEndpointUpdate()
	return nil
}

// SetEndpointAutoSchedule 设置端点是否参与自动调度（D6）；
// 关闭后 Auto 模式不自动选择，但仍可手动优选或固定使用
func (a *App) SetEndpointAutoSchedule(name string, enabled bool) error {
	a.mu.RLock()
	service := a.endpointService
	a.mu.RUnlock()
	if service == nil {
		return fmt.Errorf("端点存储服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := service.SetEndpointAutoSchedule(ctx, name, enabled); err != nil {
		return err
	}
	a.emitEndpointUpdate()
	return nil
}
