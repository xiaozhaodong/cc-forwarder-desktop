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

// ---- 手动解除冷却的持久化 pending-retry 闭环（§4.4）----

// cooldownPersistPendingEntry 以 endpoint ID + revision 管理待重试状态：
//   - 乱序完成防护：mark/clear 仅当本次 revision >= 已存 revision 才生效，
//     旧调用（低 revision）晚完成不得覆盖新调用终态；
//   - 删除重建防护：读取时要求 entry.endpointID 与当前端点记录 ID 一致，
//     同名重建端点不继承旧标记。
type cooldownPersistPendingEntry struct {
	endpointID int64
	revision   int64
	pending    bool
}

// markCooldownPersistPending 置位待重试标记；revision 低于已存值则忽略（乱序守卫）
func (a *App) markCooldownPersistPending(name string, endpointID, revision int64) {
	a.cooldownPendingMu.Lock()
	defer a.cooldownPendingMu.Unlock()
	existing, ok := a.cooldownPersistPending[name]
	if ok && existing.revision > revision {
		return
	}
	a.cooldownPersistPending[name] = cooldownPersistPendingEntry{
		endpointID: endpointID,
		revision:   revision,
		pending:    true,
	}
}

// clearCooldownPersistPending 清除待重试标记；revision 低于已存值则忽略（乱序守卫）。
// 终态 pending=false 但保留 revision 高水位：晚到的旧调用（低 revision）不得复活重试提示
func (a *App) clearCooldownPersistPending(name string, endpointID, revision int64) {
	a.cooldownPendingMu.Lock()
	defer a.cooldownPendingMu.Unlock()
	existing, ok := a.cooldownPersistPending[name]
	if ok && existing.revision > revision {
		return
	}
	a.cooldownPersistPending[name] = cooldownPersistPendingEntry{
		endpointID: endpointID,
		revision:   revision,
		pending:    false,
	}
}

// isCooldownPersistPending 读取待重试状态；endpointID 与当前端点记录不一致时视为无
// （同名重建端点不继承旧标记）
func (a *App) isCooldownPersistPending(name string, endpointID int64) bool {
	a.cooldownPendingMu.Lock()
	defer a.cooldownPendingMu.Unlock()
	entry, ok := a.cooldownPersistPending[name]
	if !ok {
		return false
	}
	return entry.pending && entry.endpointID == endpointID
}

// removeCooldownPersistPending 端点删除成功时按名清理孤儿 pending
func (a *App) removeCooldownPersistPending(name string) {
	a.cooldownPendingMu.Lock()
	defer a.cooldownPendingMu.Unlock()
	delete(a.cooldownPersistPending, name)
}

// ClearEndpointCooldown 手动解除端点冷却：推进故障 epoch、清内存冷却与软失败计数，
// 并同步事务化写入持久化 tombstone。仅当落库成功才报告成功；
// 落库失败置位 pending 标记并推送更新，前端保留稳定重试入口。
func (a *App) ClearEndpointCooldown(name string) error {
	a.mu.RLock()
	manager := a.endpointManager
	runtimeStore := a.endpointRuntimeStateStore
	endpointStore := a.endpointStore
	a.mu.RUnlock()
	if manager == nil || runtimeStore == nil || endpointStore == nil {
		return fmt.Errorf("端点管理器或运行态存储未初始化")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 先取 ID：读取失败 / 不存在时不触碰内存，完全无副作用
	record, err := endpointStore.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("读取端点 '%s' 失败: %w", name, err)
	}
	if record == nil {
		a.removeCooldownPersistPending(name) // 端点已删除：清理孤儿 pending
		return fmt.Errorf("端点 '%s' 不存在", name)
	}

	revision, found := manager.ResetEndpointFailureState(name)
	if !found {
		return fmt.Errorf("端点 '%s' 不存在", name)
	}
	if err := runtimeStore.ClearCooldownTombstones(ctx, record.ID, revision); err != nil {
		a.markCooldownPersistPending(name, record.ID, revision)
		a.emitEndpointUpdate() // 失败也推送：前端拿到 pending 态，保住重试入口
		return fmt.Errorf("冷却已在内存解除，但持久化清除失败（重启后可能恢复），请在端点行重试: %w", err)
	}
	a.clearCooldownPersistPending(name, record.ID, revision)
	a.emitEndpointUpdate()
	return nil
}
