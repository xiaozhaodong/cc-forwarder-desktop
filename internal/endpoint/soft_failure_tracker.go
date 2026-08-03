// Package endpoint - 分类软失败追踪器（收敛方案 §9.3）
// 替换旧 FailureTracker：按 endpoint + pathScope + category 独立计数，
// 不同类别不互相凑数；达到阈值即视为 tripped，同时清除触发类别窗口
// （长期阻断由调用方写入 cooldown 表达，tracker 本身不承担阻断状态）。
package endpoint

import (
	"sync"
	"time"
)

// SoftFailureScope 软失败 path scope（§9.3：messages | count_tokens）
type SoftFailureScope string

const (
	SoftFailureScopeMessages    SoftFailureScope = "messages"
	SoftFailureScopeCountTokens SoftFailureScope = "count_tokens"
)

// SoftFailureCategory 软失败类别（§9.3）
type SoftFailureCategory string

const (
	SoftFailureCategoryConnection  SoftFailureCategory = "connection"
	SoftFailureCategoryTransport   SoftFailureCategory = "transport"
	SoftFailureCategoryRateLimit   SoftFailureCategory = "rate_limit"
	SoftFailureCategoryServerError SoftFailureCategory = "server_error"
)

// SoftFailureCooldownReasonPrefix 阈值触发 cooldown 的统一 reason 前缀，
// reject 判定与快照解释依赖该前缀识别"软失败阈值型冷却"。
const SoftFailureCooldownReasonPrefix = "soft_failure_"

// SoftFailureCooldownReason 组装阈值 cooldown 的 reason 字符串
func SoftFailureCooldownReason(category SoftFailureCategory) string {
	return SoftFailureCooldownReasonPrefix + string(category)
}

// FailureTrackerConfig 软失败窗口配置（复用 failure_tracker.* 配置键，§12）
type FailureTrackerConfig struct {
	Enabled    bool          // 是否启用
	TimeWindow time.Duration // 时间窗口
	Threshold  int           // 阈值
}

type softFailureKey struct {
	endpoint string
	scope    SoftFailureScope
	category SoftFailureCategory
}

// SoftFailureTracker 分类软失败追踪器（纯内存态，D13）
type SoftFailureTracker struct {
	mu     sync.Mutex
	config FailureTrackerConfig
	events map[softFailureKey][]time.Time
}

// NewSoftFailureTracker 创建分类软失败追踪器
func NewSoftFailureTracker(enabled bool, timeWindow time.Duration, threshold int) *SoftFailureTracker {
	return &SoftFailureTracker{
		config: FailureTrackerConfig{Enabled: enabled, TimeWindow: timeWindow, Threshold: threshold},
		events: make(map[softFailureKey][]time.Time),
	}
}

// Record 记录一次软失败，返回窗口内计数与是否达到阈值。
// 达到阈值时清除该类别窗口（§9.3 规则 4：避免 cooldown 到期立即再次触发）。
func (t *SoftFailureTracker) Record(endpointName string, scope SoftFailureScope, category SoftFailureCategory) (count int, tripped bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.config.Enabled {
		return 0, false
	}

	key := softFailureKey{endpoint: endpointName, scope: scope, category: category}
	now := time.Now()
	windowStart := now.Add(-t.config.TimeWindow)

	valid := make([]time.Time, 0, len(t.events[key])+1)
	for _, item := range t.events[key] {
		if item.After(windowStart) {
			valid = append(valid, item)
		}
	}
	valid = append(valid, now)

	if len(valid) >= t.config.Threshold {
		delete(t.events, key)
		return len(valid), true
	}
	t.events[key] = valid
	return len(valid), false
}

// ClearScope 清除端点在指定 scope 的全部类别（FullSuccess，§9.3 规则 2）
func (t *SoftFailureTracker) ClearScope(endpointName string, scope SoftFailureScope) {
	t.ClearScopeBefore(endpointName, scope, time.Time{})
}

// ClearScopeBefore 清除端点在指定 scope 中 cutoff 之前的软失败事件；
// cutoff 之后的事件是比本次成功更新的失败证据，必须保留（IfNoNewerFailure：
// 较早发起的慢请求成功不得清除请求进行期间新记录的失败）。cutoff 零值=全清。
func (t *SoftFailureTracker) ClearScopeBefore(endpointName string, scope SoftFailureScope, cutoff time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, events := range t.events {
		if key.endpoint != endpointName || key.scope != scope {
			continue
		}
		if cutoff.IsZero() {
			delete(t.events, key)
			continue
		}
		remaining := make([]time.Time, 0, len(events))
		for _, item := range events {
			if item.After(cutoff) {
				remaining = append(remaining, item)
			}
		}
		if len(remaining) == 0 {
			delete(t.events, key)
		} else {
			t.events[key] = remaining
		}
	}
}

// ClearEndpoint 清除端点全部软失败记录（端点删除/手动清理）
func (t *SoftFailureTracker) ClearEndpoint(endpointName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key := range t.events {
		if key.endpoint == endpointName {
			delete(t.events, key)
		}
	}
}

// CountsFor 返回端点在指定 scope 的各类别有效计数（快照/UI 解释用）
func (t *SoftFailureTracker) CountsFor(endpointName string, scope SoftFailureScope) map[SoftFailureCategory]int {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make(map[SoftFailureCategory]int)
	if !t.config.Enabled {
		return result
	}
	windowStart := time.Now().Add(-t.config.TimeWindow)
	for key, events := range t.events {
		if key.endpoint != endpointName || key.scope != scope {
			continue
		}
		count := 0
		for _, item := range events {
			if item.After(windowStart) {
				count++
			}
		}
		if count > 0 {
			result[key.category] = count
		}
	}
	return result
}

// Stats 按端点聚合全部 scope/类别的有效计数（兼容旧 GetFailureStats 展示口径）
func (t *SoftFailureTracker) Stats() map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := make(map[string]int)
	if !t.config.Enabled {
		return stats
	}
	windowStart := time.Now().Add(-t.config.TimeWindow)
	for key, events := range t.events {
		count := 0
		for _, item := range events {
			if item.After(windowStart) {
				count++
			}
		}
		if count > 0 {
			stats[key.endpoint] += count
		}
	}
	return stats
}

// Threshold 返回当前阈值（快照解释用）
func (t *SoftFailureTracker) Threshold() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.config.Threshold
}

// UpdateConfig 热更新配置；禁用时清空历史
func (t *SoftFailureTracker) UpdateConfig(enabled bool, timeWindow time.Duration, threshold int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	wasEnabled := t.config.Enabled
	t.config = FailureTrackerConfig{Enabled: enabled, TimeWindow: timeWindow, Threshold: threshold}
	if wasEnabled && !enabled {
		t.events = make(map[softFailureKey][]time.Time)
	}
}

// CleanupExpiredEvents 惰性清理过窗事件（由健康检查循环定期调用）
func (t *SoftFailureTracker) CleanupExpiredEvents() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.config.Enabled {
		return
	}
	windowStart := time.Now().Add(-t.config.TimeWindow)
	for key, events := range t.events {
		valid := events[:0]
		for _, item := range events {
			if item.After(windowStart) {
				valid = append(valid, item)
			}
		}
		if len(valid) == 0 {
			delete(t.events, key)
		} else {
			t.events[key] = valid
		}
	}
}

// ---- count_tokens 进程内 scoped cooldown（D17：第一版不落库，不冷却 /v1/messages）----

type scopedCooldownEntry struct {
	Until  time.Time
	Reason string
}

// ScopedCooldowns 按 endpoint+scope 的内存冷却表
type ScopedCooldowns struct {
	mu      sync.Mutex
	entries map[softFailureKey]scopedCooldownEntry
}

// NewScopedCooldowns 创建 scoped cooldown 表
func NewScopedCooldowns() *ScopedCooldowns {
	return &ScopedCooldowns{entries: make(map[softFailureKey]scopedCooldownEntry)}
}

// Set 写入 scoped cooldown
func (c *ScopedCooldowns) Set(endpointName string, scope SoftFailureScope, until time.Time, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[softFailureKey{endpoint: endpointName, scope: scope}] = scopedCooldownEntry{Until: until, Reason: reason}
}

// Active 查询仍生效的 scoped cooldown
func (c *ScopedCooldowns) Active(endpointName string, scope SoftFailureScope) (bool, time.Time, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := softFailureKey{endpoint: endpointName, scope: scope}
	entry, ok := c.entries[key]
	if !ok {
		return false, time.Time{}, ""
	}
	if !entry.Until.After(time.Now()) {
		delete(c.entries, key)
		return false, time.Time{}, ""
	}
	return true, entry.Until, entry.Reason
}

// Clear 清除端点在指定 scope 的冷却
func (c *ScopedCooldowns) Clear(endpointName string, scope SoftFailureScope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, softFailureKey{endpoint: endpointName, scope: scope})
}

// ClearEndpoint 清除端点在所有 scope 下的冷却（删除/同名重建隔离）。
func (c *ScopedCooldowns) ClearEndpoint(endpointName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if key.endpoint == endpointName {
			delete(c.entries, key)
		}
	}
}
