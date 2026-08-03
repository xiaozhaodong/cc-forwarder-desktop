// failover.go - 故障转移相关功能
// 包含请求级故障转移、冷却机制、端点切换等

package endpoint

import (
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

// Cooldown 作用域（§14.4：与 endpoint_runtime_states.scope 持久化口径一致）
const (
	CooldownScopeGlobal   = "global"   // auth/quota 型：阻断 /v1/messages 与 count_tokens
	CooldownScopeMessages = "messages" // 普通软失败：仅阻断 /v1/messages
)

// CooldownScopeForReason 由冷却原因推导作用域
func CooldownScopeForReason(reason string) string {
	if reason == "auth_rejected" || strings.HasPrefix(reason, "quota") {
		return CooldownScopeGlobal
	}
	return CooldownScopeMessages
}

// cooldownRevisionCounter 持久化 revision 的全局单调源。
// 不用裸 UnixNano（时钟回拨/同纳秒重复会破坏单调），也不用从 0 起步的纯计数器
// （重启后会小于库里旧 revision，新写入被 Upsert 全部丢弃）：
// 取「当前 UnixNano 与已发号取大再 CAS」，进程内严格单调；跨重启由
// 启动时 SeedCooldownRevision(MAX(revision)) 保证大于全部历史记录。
var cooldownRevisionCounter atomic.Int64

func nextCooldownRevision() int64 {
	for {
		current := cooldownRevisionCounter.Load()
		candidate := time.Now().UnixNano()
		if candidate <= current {
			candidate = current + 1
		}
		if cooldownRevisionCounter.CompareAndSwap(current, candidate) {
			return candidate
		}
	}
}

// SeedCooldownRevision 启动时用持久化的 MAX(revision) 播种发号器：
// 兜底时钟回拨或数据库迁移自更快时钟机器的场景，保证新发号严格大于全部历史记录
func SeedCooldownRevision(minRevision int64) {
	for {
		current := cooldownRevisionCounter.Load()
		if current >= minRevision {
			return
		}
		if cooldownRevisionCounter.CompareAndSwap(current, minRevision) {
			return
		}
	}
}

// SetOnFailoverTriggered 设置故障转移回调
// 当请求失败触发故障转移时调用，用于同步数据库
func (m *Manager) SetOnFailoverTriggered(fn func(failedEndpoint, newEndpoint string)) {
	m.onFailoverTriggered = fn
}

// IsEndpointInCooldown 检查端点是否在冷却中（任一 scope 生效即视为冷却）
func (m *Manager) IsEndpointInCooldown(name string) bool {
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return false
	}
	return ep.IsInCooldown()
}

// ClearEndpointCooldown 清除端点冷却状态（用于手动激活时，两个 scope 槽全部清除，
// 并以更高 revision 的 tombstone 覆盖持久化记录——物理 DELETE 会让 revision 保护失效，
// 晚执行的旧 persist 任务能把冷却重新写回）。revision 在锁内生成，与 Set 侧共用单调序列。
func (m *Manager) ClearEndpointCooldown(name string) {
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return
	}

	var revision int64
	ep.mutex.Lock()
	cleared := !ep.Status.CooldownUntil.IsZero() || !ep.Status.GlobalCooldownUntil.IsZero()
	if cleared {
		slog.Info(fmt.Sprintf("🔓 [冷却] 清除端点冷却: %s (messages: %s, global: %s)",
			name, ep.Status.CooldownReason, ep.Status.GlobalCooldownReason))
		ep.Status.CooldownUntil = time.Time{}
		ep.Status.CooldownReason = ""
		ep.Status.GlobalCooldownUntil = time.Time{}
		ep.Status.GlobalCooldownReason = ""
		revision = nextCooldownRevision()
	}
	ep.mutex.Unlock()

	if cleared {
		m.notifyCooldownClear(name, revision)
	}
}

// GetEndpointCooldownInfo 获取端点冷却信息（两个 scope 槽中取截止最晚的生效冷却）
func (m *Manager) GetEndpointCooldownInfo(name string) (inCooldown bool, until time.Time, reason string) {
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return false, time.Time{}, ""
	}

	ep.mutex.RLock()
	defer ep.mutex.RUnlock()

	until, reason, inCooldown = ep.Status.EffectiveCooldown(time.Now())
	return inCooldown, until, reason
}

// SetEndpointCooldown 设置端点冷却（新转发管线的失败标记入口）；
// scope 由 reason 推导并只写入对应槽：auth/quota 型写 global 槽（阻断两个 path），
// 其余写 messages 槽。同槽只延长不缩短：新截止不晚于现值时保持原冷却，也不触发持久化。
// 持久化 revision 在锁内同步生成，保证落库顺序与内存写入顺序一致（§14.4）。
func (m *Manager) SetEndpointCooldown(name string, duration time.Duration, reason string) {
	if duration <= 0 {
		return
	}
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return
	}
	until := time.Now().Add(duration)
	var revision int64
	extended := false
	ep.mutex.Lock()
	if CooldownScopeForReason(reason) == CooldownScopeGlobal {
		if until.After(ep.Status.GlobalCooldownUntil) {
			ep.Status.GlobalCooldownUntil = until
			ep.Status.GlobalCooldownReason = reason
			extended = true
		}
	} else if until.After(ep.Status.CooldownUntil) {
		ep.Status.CooldownUntil = until
		ep.Status.CooldownReason = reason
		extended = true
	}
	if extended {
		revision = nextCooldownRevision()
	}
	ep.mutex.Unlock()
	if !extended {
		return
	}

	// v8：进入冷却即清除 retained（§8.4 规则 3），并触发持久化钩子（§14.4）
	m.ClearAutoRetentionFor(name)
	m.notifyCooldownPersist(name, until, reason, revision)
}

// SetCooldownPersistHook 注入 cooldown 持久化回调（App 层桥接 endpoint_runtime_states）；
// revision 由 Set 侧在锁内生成，Upsert 按 revision 单调丢弃晚执行的旧任务
func (m *Manager) SetCooldownPersistHook(hook func(name string, until time.Time, reason string, revision int64)) {
	m.cooldownPersistMu.Lock()
	m.cooldownPersistHook = hook
	m.cooldownPersistMu.Unlock()
}

// SetCooldownClearHook 注入 cooldown 清除回调（手动激活等清冷却时以 tombstone 覆盖持久化记录）
func (m *Manager) SetCooldownClearHook(hook func(name string, revision int64)) {
	m.cooldownPersistMu.Lock()
	m.cooldownClearHook = hook
	m.cooldownPersistMu.Unlock()
}

func (m *Manager) notifyCooldownPersist(name string, until time.Time, reason string, revision int64) {
	m.cooldownPersistMu.RLock()
	hook := m.cooldownPersistHook
	m.cooldownPersistMu.RUnlock()
	if hook != nil {
		go hook(name, until, reason, revision)
	}
}

func (m *Manager) notifyCooldownClear(name string, revision int64) {
	m.cooldownPersistMu.RLock()
	hook := m.cooldownClearHook
	m.cooldownPersistMu.RUnlock()
	if hook != nil {
		go hook(name, revision)
	}
}

// RestoreEndpointCooldown 启动恢复持久化 cooldown（仅内存，不再回写、不清 retained）。
// 同端点的 global 与 messages 两行分别恢复到各自槽位，互不覆盖；
// 同槽位重复恢复时截止晚者优先。
func (m *Manager) RestoreEndpointCooldown(name, scope string, until time.Time, reason string) {
	ep := m.GetEndpointByNameAny(name)
	if ep == nil || !until.After(time.Now()) {
		return
	}
	ep.mutex.Lock()
	defer ep.mutex.Unlock()

	if scope == CooldownScopeGlobal {
		if until.After(ep.Status.GlobalCooldownUntil) {
			ep.Status.GlobalCooldownUntil = until
			ep.Status.GlobalCooldownReason = reason
		}
		return
	}
	if until.After(ep.Status.CooldownUntil) {
		ep.Status.CooldownUntil = until
		ep.Status.CooldownReason = reason
	}
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
