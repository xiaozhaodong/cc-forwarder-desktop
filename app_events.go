// app_events.go - Wails 事件发射
// 将 Go 后端状态变化通知到前端

package main

import (
	"fmt"

	"cc-forwarder/internal/proxy"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 事件名称常量
const (
	EventSystemStatus   = "system:status"
	EventEndpointUpdate = "endpoint:update"
	EventGroupUpdate    = "group:update"
	EventUsageUpdate    = "usage:update"
	EventConfigReloaded = "config:reloaded"
	EventError          = "error"
	EventNotification   = "notification"
)

// emitSystemStatus 发送系统状态更新到前端
func (a *App) emitSystemStatus() {
	if a.ctx == nil {
		return
	}

	status := a.GetSystemStatus()
	runtime.EventsEmit(a.ctx, EventSystemStatus, status)
}

// emitEndpointUpdate 发送端点状态更新到前端
func (a *App) emitEndpointUpdate() {
	if a.ctx == nil {
		return
	}

	endpoints := a.GetEndpoints()
	// 包装为前端期望的格式 { endpoints: [...] }
	data := map[string]interface{}{
		"endpoints": endpoints,
	}

	if a.logger != nil {
		a.logger.Info("📡 [Wails Event] 推送端点更新", "count", len(endpoints))
	}

	runtime.EventsEmit(a.ctx, EventEndpointUpdate, data)
}

// emitGroupUpdate 发送组状态更新到前端
func (a *App) emitGroupUpdate() {
	if a.ctx == nil {
		return
	}

	groups := a.GetGroups()
	runtime.EventsEmit(a.ctx, EventGroupUpdate, groups)
}

// emitUsageUpdate 发送使用统计更新到前端
func (a *App) emitUsageUpdate() {
	if a.ctx == nil {
		return
	}

	summary, _ := a.GetUsageSummary("", "")
	runtime.EventsEmit(a.ctx, EventUsageUpdate, summary)
}

// emitNotification 发送通知到前端
func (a *App) emitNotification(level, title, message string) {
	a.emitNotificationData(map[string]interface{}{
		"level":   level, // "info", "warning", "error", "success"
		"title":   title,
		"message": message,
	})
}

// emitFailoverNotification 将请求级故障转移转换为窗口内全局 Toast 事件。
// 不调用任何 macOS/系统通知 API，避免在桌面之外产生额外打扰。
func (a *App) emitFailoverNotification(event proxy.FailoverEvent) {
	laneLabel := "CC 端点"
	if event.Lane == proxy.FailoverLaneCodex {
		laneLabel = "Codex 账号"
	}

	reason := event.ReasonLabel
	if reason == "" {
		reason = proxy.FailoverReasonLabel(event.ReasonCode)
	}
	if event.ReasonDetail != "" {
		reason = fmt.Sprintf("%s（%s）", reason, event.ReasonDetail)
	}

	a.emitNotificationData(map[string]interface{}{
		"level":         "warning",
		"title":         "发生故障转移",
		"message":       fmt.Sprintf("%s已从「%s」切换到「%s」。原因：%s", laneLabel, event.From, event.To, reason),
		"kind":          event.Kind,
		"lane":          event.Lane,
		"from":          event.From,
		"to":            event.To,
		"reason_code":   event.ReasonCode,
		"reason_label":  reason,
		"reason_detail": event.ReasonDetail,
		"request_id":    event.RequestID,
		"request_path":  event.RequestPath,
		"attempt":       event.Attempt,
	})
}

func (a *App) emitNotificationData(data map[string]interface{}) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, EventNotification, data)
}
