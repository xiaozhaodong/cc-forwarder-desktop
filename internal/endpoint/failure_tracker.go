// Package endpoint - 通用失败追踪器
// 基于时间窗口追踪端点失败次数，不区分错误类型
package endpoint

import (
	"log/slog"
	"sync"
	"time"
)

// FailureTrackerConfig 失败追踪器配置
type FailureTrackerConfig struct {
	Enabled    bool          // 是否启用
	TimeWindow time.Duration // 时间窗口
	Threshold  int           // 失败次数阈值
}

// FailureTracker 通用失败追踪器
// 追踪端点在时间窗口内的失败次数，用于触发故障转移、挂起或拒绝请求
type FailureTracker struct {
	config FailureTrackerConfig
	mu     sync.RWMutex

	// endpointFailures 记录每个端点的失败时间列表
	// key: endpointName, value: 失败时间列表（最新的在后面）
	endpointFailures map[string][]time.Time
}

// NewFailureTracker 创建失败追踪器
func NewFailureTracker(enabled bool, timeWindow time.Duration, threshold int) *FailureTracker {
	return &FailureTracker{
		config: FailureTrackerConfig{
			Enabled:    enabled,
			TimeWindow: timeWindow,
			Threshold:  threshold,
		},
		endpointFailures: make(map[string][]time.Time),
	}
}

// RecordFailure 记录端点失败
// 返回当前窗口内的失败次数
func (t *FailureTracker) RecordFailure(endpointName string) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 如果追踪器禁用，不记录事件
	if !t.config.Enabled {
		return 0
	}

	now := time.Now()
	failures := t.endpointFailures[endpointName]

	// 添加当前失败事件
	failures = append(failures, now)
	t.endpointFailures[endpointName] = failures

	// 清理过期事件并返回当前计数
	return t.cleanAndCountLocked(endpointName, now)
}

// RecordSuccess 记录端点成功
// 成功后清空该端点的失败记录
func (t *FailureTracker) RecordSuccess(endpointName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.config.Enabled {
		return
	}

	// 清空失败记录
	delete(t.endpointFailures, endpointName)
	slog.Debug("[失败追踪] 端点成功，清空失败记录", "endpoint", endpointName)
}

// ShouldTriggerAction 检查是否应该触发动作
// 返回 true 表示当前窗口内失败次数达到阈值
func (t *FailureTracker) ShouldTriggerAction(endpointName string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.config.Enabled {
		return false
	}

	now := time.Now()
	count := t.countValidFailuresLocked(endpointName, now)

	return count >= t.config.Threshold
}

// GetStats 获取所有端点的失败统计
// 返回 map[endpointName]failureCount
func (t *FailureTracker) GetStats() map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := make(map[string]int)
	now := time.Now()

	for endpointName := range t.endpointFailures {
		count := t.countValidFailuresLocked(endpointName, now)
		if count > 0 {
			stats[endpointName] = count
		}
	}

	return stats
}

// ClearEndpoint 清空指定端点的失败记录
func (t *FailureTracker) ClearEndpoint(endpointName string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.endpointFailures, endpointName)
	slog.Info("[失败追踪] 清空端点失败记录", "endpoint", endpointName)
}

// UpdateConfig 热更新配置
func (t *FailureTracker) UpdateConfig(enabled bool, timeWindow time.Duration, threshold int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	oldConfig := t.config
	t.config = FailureTrackerConfig{
		Enabled:    enabled,
		TimeWindow: timeWindow,
		Threshold:  threshold,
	}

	// 如果从启用变为禁用，清空所有历史记录
	if oldConfig.Enabled && !enabled {
		t.endpointFailures = make(map[string][]time.Time)
		slog.Info("🗑️ [失败追踪] 追踪器禁用，已清空所有历史记录")
	}

	slog.Info("🔄 [失败追踪] 配置已更新",
		"enabled", enabled,
		"time_window", timeWindow,
		"threshold", threshold)
}

// CleanupExpiredEvents 清理所有端点的过期事件
// 由健康检查循环定期调用
func (t *FailureTracker) CleanupExpiredEvents() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.config.Enabled {
		return
	}

	now := time.Now()
	for endpointName := range t.endpointFailures {
		t.cleanAndCountLocked(endpointName, now)
	}
}

// cleanAndCountLocked 清理过期事件并返回有效失败次数
// 调用前必须持有写锁
func (t *FailureTracker) cleanAndCountLocked(endpointName string, now time.Time) int {
	failures := t.endpointFailures[endpointName]
	if len(failures) == 0 {
		return 0
	}

	// 计算窗口起始时间
	windowStart := now.Add(-t.config.TimeWindow)

	// 过滤掉窗口外的事件
	validFailures := []time.Time{}
	for _, failTime := range failures {
		if failTime.After(windowStart) {
			validFailures = append(validFailures, failTime)
		}
	}

	// 更新记录
	if len(validFailures) == 0 {
		delete(t.endpointFailures, endpointName)
	} else {
		t.endpointFailures[endpointName] = validFailures
	}

	return len(validFailures)
}

// countValidFailuresLocked 计算有效失败次数
// 调用前必须持有读锁
func (t *FailureTracker) countValidFailuresLocked(endpointName string, now time.Time) int {
	failures := t.endpointFailures[endpointName]
	if len(failures) == 0 {
		return 0
	}

	windowStart := now.Add(-t.config.TimeWindow)
	count := 0
	for _, failTime := range failures {
		if failTime.After(windowStart) {
			count++
		}
	}

	return count
}
