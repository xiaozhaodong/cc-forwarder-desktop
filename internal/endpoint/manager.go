// manager.go - 端点管理器核心结构和基础功能
// 其他功能已拆分到独立文件：
// - health_check.go: 健康检查相关
// - endpoint_selection.go: 端点选择/路由
// - endpoint_crud.go: 动态端点管理
// - failover.go: 故障转移
// - key_switch.go: Key 切换
// - notification.go: 通知相关

package endpoint

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/events"
	"cc-forwarder/internal/transport"
)

var failureTrackerCleanupInterval = 30 * time.Second

// EndpointStatus represents the health status of an endpoint
type EndpointStatus struct {
	Healthy          bool
	LastCheck        time.Time
	ResponseTime     time.Duration
	ConsecutiveFails int
	NeverChecked     bool // 表示从未被检测过
	// 冷却运行态按 scope 分槽（§14.4），互不覆盖：
	// messages 槽仅阻断 /v1/messages；global 槽（auth/quota）阻断双 path
	CooldownUntil        time.Time // messages scope 冷却截止时间
	CooldownReason       string    // messages scope 冷却原因
	GlobalCooldownUntil  time.Time // global scope 冷却截止时间
	GlobalCooldownReason string    // global scope 冷却原因
	PausedUntil          time.Time // 手动暂停截止时间（零值=未暂停；到期读取时自愈）
}

// EffectiveCooldown 返回当前生效冷却中截止最晚的一条（展示与 messages path 判定口径）
func (s EndpointStatus) EffectiveCooldown(now time.Time) (until time.Time, reason string, active bool) {
	if !s.CooldownUntil.IsZero() && now.Before(s.CooldownUntil) {
		until, reason, active = s.CooldownUntil, s.CooldownReason, true
	}
	if !s.GlobalCooldownUntil.IsZero() && now.Before(s.GlobalCooldownUntil) && s.GlobalCooldownUntil.After(until) {
		until, reason, active = s.GlobalCooldownUntil, s.GlobalCooldownReason, true
	}
	return until, reason, active
}

// Endpoint represents an endpoint with its configuration and status
type Endpoint struct {
	Config config.EndpointConfig
	Status EndpointStatus
	mutex  sync.RWMutex
	// v8：配置修订号（publish 时递增，AttemptPlan CAS 依据）与在途 admission 计数
	configRevision int64
	admissions     atomic.Int64
}

// RLock 锁定端点状态读锁（供外部安全读取状态）
func (e *Endpoint) RLock() {
	e.mutex.RLock()
}

// RUnlock 解锁端点状态读锁
func (e *Endpoint) RUnlock() {
	e.mutex.RUnlock()
}

// IsInCooldown 检查端点是否处于冷却状态（任一 scope 生效即视为冷却，/v1/messages 口径）
func (e *Endpoint) IsInCooldown() bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	_, _, active := e.Status.EffectiveCooldown(time.Now())
	return active
}

// IsPaused 检查端点是否处于手动暂停状态（PausedUntil 到期自动视为恢复）
func (e *Endpoint) IsPaused() bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return !e.Status.PausedUntil.IsZero() && time.Now().Before(e.Status.PausedUntil)
}

// Manager manages endpoints and their health status
type Manager struct {
	endpoints           []*Endpoint
	endpointsMu         sync.RWMutex // v5.0+: 保护 endpoints 切片的并发访问
	endpointConfigMu    sync.RWMutex // v8：配置/revision/KeyManager 发布与 attempt 结算的 generation barrier
	configMu            sync.RWMutex
	config              *config.Config
	client              *http.Client
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	fastTester          *FastTester
	keyManager          *KeyManager         // 管理多 API Key 状态
	softFailures        *SoftFailureTracker // 分类软失败追踪器（§9.3）
	pendingGates        *pendingGateSet     // §7.6 停用中的安全阻断 gate
	autoRetention       *autoRetentionState // §8.4 Auto retained 运行态
	scopedCooldowns     *ScopedCooldowns    // §10 count_tokens 进程内 scoped cooldown（D17）
	routingNotReady     atomic.Bool         // §6.4：启动读取失败时 Claude 路由 not ready
	cooldownPersistMu   sync.RWMutex
	cooldownPersistHook func(name string, until time.Time, reason string, revision int64) // §14.4 冷却持久化钩子
	cooldownClearHook   func(name string, revision int64)                                 // 手动清冷却时写 tombstone 覆盖持久化记录
	routeDecisionMu     sync.RWMutex
	routeDecisionNotify func(RouteOverrideState) // last_effective_endpoint 变更通知（UI 事件桥接）
	routeOverride       *RouteOverride
	routeState          *RouteState
	scheduleSnapshots   *endpointScheduleSnapshotStore
	// EventBus for decoupled event publishing
	eventBus events.EventBus
	// 健康检查完成回调（用于推送 Wails 事件）
	onHealthCheckComplete func()
	// 故障转移回调（用于同步数据库）
	// 参数: failedEndpoint 失败的端点名, newEndpoint 新激活的端点名
	onFailoverTriggered func(failedEndpoint, newEndpoint string)

	// v7 重构：activeEndpoint 单一权威状态（Phase 3 新增，Phase 4 接线）
	// 所有变更统一走 active_state.go 的各档入口，持久化经 runtimeWriter。
	activeMu       sync.Mutex
	activeEndpoint string
	activeRevision int64
	runtimeWriter  *RuntimeWriter
}

// NewManager creates a new endpoint manager
func NewManager(cfg *config.Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	// Create transport with proxy support
	httpTransport, err := transport.CreateTransport(cfg)
	if err != nil {
		slog.Error(fmt.Sprintf("❌ Failed to create HTTP transport with proxy: %s", err.Error()))
		// Fall back to default transport
		httpTransport = &http.Transport{}
	}

	manager := &Manager{
		config: cfg,
		client: &http.Client{
			Timeout:   cfg.Health.Timeout,
			Transport: httpTransport,
		},
		ctx:               ctx,
		cancel:            cancel,
		fastTester:        NewFastTester(cfg),
		keyManager:        NewKeyManager(), // 初始化 Key 管理器
		routeOverride:     NewRouteOverride(),
		pendingGates:      newPendingGateSet(),
		autoRetention:     newAutoRetentionState(),
		scopedCooldowns:   NewScopedCooldowns(),
		routeState:        NewRouteState(),
		scheduleSnapshots: newEndpointScheduleSnapshotStore(),
		softFailures: NewSoftFailureTracker(
			cfg.FailureTracker.Enabled,
			cfg.FailureTracker.TimeWindow,
			cfg.FailureTracker.Threshold,
		),
	}

	// Initialize endpoints
	for _, endpointCfg := range cfg.Endpoints {
		endpoint := &Endpoint{
			Config: endpointCfg,
			Status: EndpointStatus{
				Healthy:      false,
				NeverChecked: true, // 标记为未检测，等待手动/批量连通性测试或真实请求结果
			},
			configRevision: NextEndpointConfigRevision(),
		}
		manager.endpoints = append(manager.endpoints, endpoint)

		// 初始化端点的 Key 状态
		tokenCount := len(endpointCfg.Tokens)
		if tokenCount == 0 && endpointCfg.Token != "" {
			tokenCount = 1 // 单 Token 算作 1 个
		}
		apiKeyCount := len(endpointCfg.ApiKeys)
		if apiKeyCount == 0 && endpointCfg.ApiKey != "" {
			apiKeyCount = 1 // 单 API Key 算作 1 个
		}
		manager.keyManager.InitEndpoint(endpointCfg.Name, tokenCount, apiKeyCount)
	}

	// Set manager reference in fast tester for dynamic token resolution
	manager.fastTester.SetManager(manager)

	return manager
}

// Start starts endpoint background routines.
// 2026-03: 后台健康轮询已停用，避免持续探测 /v1/models。
func (m *Manager) Start() {
	if m.softFailures != nil {
		m.wg.Add(1)
		go m.failureTrackerCleanupLoop()
	}

	slog.Debug("🛑 [端点管理] 后台连通性轮询已停用，仅保留失败追踪清理与请求级状态更新")
}

// Stop stops endpoint background routines.
func (m *Manager) Stop() {
	m.cancel()
	m.wg.Wait()
}

// UpdateConfig updates the manager configuration (hot-reload)
// v5.0 Desktop: 只更新配置参数，不重建端点（端点完全由数据库管理）
func (m *Manager) UpdateConfig(cfg *config.Config) {
	m.configMu.Lock()
	m.config = cfg
	m.configMu.Unlock()

	slog.Debug("🔄 [热更新] 更新配置参数完成，端点保持不变")

	// Update fast tester with new config
	if m.fastTester != nil {
		m.fastTester.UpdateConfig(cfg)
	}

	// 🔧 [热更新] 同步更新软失败追踪器配置
	if m.softFailures != nil {
		m.softFailures.UpdateConfig(cfg.FailureTracker.Enabled, cfg.FailureTracker.TimeWindow, cfg.FailureTracker.Threshold)
	}

	// Recreate transport with new proxy configuration
	if transport, err := transport.CreateTransport(cfg); err == nil {
		m.client = &http.Client{
			Transport: transport,
			Timeout:   cfg.Health.Timeout,
		}
	}
}

// GetTokenForEndpoint dynamically resolves the token for an endpoint
// If the endpoint has its own token, return it
// If not, find the first endpoint in the same group that has a token
// 支持多 Token 配置：优先使用 tokens 数组中当前激活的 Token
func (m *Manager) GetTokenForEndpoint(ep *Endpoint) string {
	// 1. 优先使用多 Tokens 配置（端点独立管理）
	if len(ep.Config.Tokens) > 0 {
		activeIndex := m.keyManager.GetActiveTokenIndex(ep.Config.Name)
		if activeIndex >= 0 && activeIndex < len(ep.Config.Tokens) {
			return ep.Config.Tokens[activeIndex].Value
		}
		return ep.Config.Tokens[0].Value // 回退到第一个
	}

	// 2. 使用单 Token 配置
	if ep.Config.Token != "" {
		return ep.Config.Token
	}

	// 3. 组内继承（仅对单 Token 保持原有行为，多 Token 不继承）
	groupName := ep.Config.Group
	if groupName == "" {
		groupName = "Default"
	}

	// v5.0+: 使用读锁遍历 endpoints
	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()

	// Search through all endpoints for the same group
	for _, endpoint := range m.endpoints {
		endpointGroup := endpoint.Config.Group
		if endpointGroup == "" {
			endpointGroup = "Default"
		}

		// If same group and has token (only single token inheritance)
		if endpointGroup == groupName && endpoint.Config.Token != "" {
			return endpoint.Config.Token
		}
	}

	// 4. No token found in the group
	return ""
}

// GetApiKeyForEndpoint dynamically resolves the API key for an endpoint
// If the endpoint has its own api-key, return it
// If not, find the first endpoint in the same group that has an api-key
// 支持多 API Key 配置：优先使用 api-keys 数组中当前激活的 API Key
func (m *Manager) GetApiKeyForEndpoint(ep *Endpoint) string {
	// 1. 优先使用多 ApiKeys 配置（端点独立管理）
	if len(ep.Config.ApiKeys) > 0 {
		activeIndex := m.keyManager.GetActiveApiKeyIndex(ep.Config.Name)
		if activeIndex >= 0 && activeIndex < len(ep.Config.ApiKeys) {
			return ep.Config.ApiKeys[activeIndex].Value
		}
		return ep.Config.ApiKeys[0].Value // 回退到第一个
	}

	// 2. 使用单 ApiKey 配置
	if ep.Config.ApiKey != "" {
		return ep.Config.ApiKey
	}

	// 3. 组内继承（仅对单 ApiKey 保持原有行为，多 ApiKey 不继承）
	groupName := ep.Config.Group
	if groupName == "" {
		groupName = "Default"
	}

	// v5.0+: 使用读锁遍历 endpoints
	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()

	// Search through all endpoints for the same group
	for _, endpoint := range m.endpoints {
		endpointGroup := endpoint.Config.Group
		if endpointGroup == "" {
			endpointGroup = "Default"
		}

		// If same group and has api-key (only single api-key inheritance)
		if endpointGroup == groupName && endpoint.Config.ApiKey != "" {
			return endpoint.Config.ApiKey
		}
	}

	// 4. No api-key found in the group
	return ""
}

// GetConfig returns the manager's configuration
func (m *Manager) GetConfig() *config.Config {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	return m.config
}

// getConfigSnapshot returns a consistent config pointer for internal use.
func (m *Manager) getConfigSnapshot() *config.Config {
	m.configMu.RLock()
	cfg := m.config
	m.configMu.RUnlock()
	return cfg
}

// RecordSoftFailure 记录一次分类软失败，返回窗口内计数与是否达到阈值（§9.3）
func (m *Manager) RecordSoftFailure(endpointName string, scope SoftFailureScope, category SoftFailureCategory) (int, bool) {
	return m.softFailures.Record(endpointName, scope, category)
}

// RecordSuccess FullSuccess 清空端点 messages scope 的全部软失败类别。
// count_tokens scope 由其 handler 自行清理（Phase 3 接入）。
func (m *Manager) RecordSuccess(endpointName string) {
	m.softFailures.ClearScope(endpointName, SoftFailureScopeMessages)
}

// RecordSuccessSince FullSuccess 仅清除 since（请求开始时刻）之前的 messages 软失败：
// 慢请求的成功不会抹掉请求进行期间新记录的失败证据（IfNoNewerFailure）。
func (m *Manager) RecordSuccessSince(endpointName string, since time.Time) {
	m.softFailures.ClearScopeBefore(endpointName, SoftFailureScopeMessages, since)
}

// ClearSoftFailureScope 清空指定 scope 的软失败类别与 scoped cooldown
func (m *Manager) ClearSoftFailureScope(endpointName string, scope SoftFailureScope) {
	m.softFailures.ClearScope(endpointName, scope)
	m.scopedCooldowns.Clear(endpointName, scope)
}

// SetScopedCooldown 写入进程内 scoped cooldown（count_tokens 专用，D17）
func (m *Manager) SetScopedCooldown(endpointName string, scope SoftFailureScope, duration time.Duration, reason string) {
	if duration <= 0 {
		return
	}
	m.scopedCooldowns.Set(endpointName, scope, time.Now().Add(duration), reason)
}

// ScopedCooldownActive 查询 scoped cooldown
func (m *Manager) ScopedCooldownActive(endpointName string, scope SoftFailureScope) (bool, time.Time, string) {
	return m.scopedCooldowns.Active(endpointName, scope)
}

// GetFailureStats 获取失败统计信息（按端点聚合，兼容旧展示口径）
func (m *Manager) GetFailureStats() map[string]int {
	return m.softFailures.Stats()
}

// GetSoftFailureCounts 返回端点在指定 scope 的分类软失败计数（快照解释用）
func (m *Manager) GetSoftFailureCounts(endpointName string, scope SoftFailureScope) map[SoftFailureCategory]int {
	return m.softFailures.CountsFor(endpointName, scope)
}

// SoftFailureThreshold 当前软失败阈值
func (m *Manager) SoftFailureThreshold() int {
	return m.softFailures.Threshold()
}

func (m *Manager) failureTrackerCleanupLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(failureTrackerCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.softFailures.CleanupExpiredEvents()
		}
	}
}

// IsEndpointRoutable reports whether an endpoint can currently participate in routing.
// 调度仅依赖真实请求失败追踪和冷却状态，不依赖后台健康轮询。
func (m *Manager) IsEndpointRoutable(ep *Endpoint) bool {
	if ep == nil {
		return false
	}

	// v8：软失败阈值触发即写入 cooldown，可路由性只看 cooldown（§9.3）
	return !ep.IsInCooldown()
}

// UpdateFailureTrackerConfig 热更新失败追踪器配置
func (m *Manager) UpdateFailureTrackerConfig(enabled bool, timeWindow time.Duration, threshold int) {
	m.softFailures.UpdateConfig(enabled, timeWindow, threshold)
}

// ShouldRejectRequest 检查是否应该拒绝请求（D15 冻结语义，§12）：
// action=reject 时，检查"第一逻辑候选"是否处于软失败阈值型 cooldown；
// tripped 即拒绝，不尝试备用端点。第一逻辑候选优先取 manual fixed/preferred
// 目标，否则按 hard 资格 + 自动调度资格取 priority 最小值端点。
func (m *Manager) ShouldRejectRequest() (bool, string) {
	cfg := m.getConfigSnapshot()
	if !cfg.FailureTracker.Enabled || cfg.FailureTracker.Action != "reject" {
		return false, ""
	}

	name := m.firstLogicalCandidateName()
	if name == "" {
		return false, ""
	}
	// 直接读 messages 槽：effective cooldown 取"截止最晚"者，更晚到期的 global
	// auth/quota 冷却会掩盖软失败 reason，导致 reject 误放行（§12）
	ep := m.GetEndpointByNameAny(name)
	if ep == nil {
		return false, ""
	}
	ep.mutex.RLock()
	until := ep.Status.CooldownUntil
	reason := ep.Status.CooldownReason
	ep.mutex.RUnlock()
	if !until.IsZero() && time.Now().Before(until) && strings.HasPrefix(reason, SoftFailureCooldownReasonPrefix) {
		return true, name
	}
	return false, ""
}

// firstLogicalCandidateName 计算第一逻辑候选（忽略健康/冷却状态）：
// manual fixed/preferred 目标优先（硬启用时）；否则按 hard 资格 + 自动调度资格
// 取 priority 最小值端点（v8：legacy active 已退役，不再参与判定）。
func (m *Manager) firstLogicalCandidateName() string {
	override := m.routeOverride.Snapshot()
	if override.Mode != RouteModeAuto && override.EndpointName != "" {
		if ep := m.GetEndpointByNameAny(override.EndpointName); ep != nil && m.EndpointHardEnabled(ep) {
			return override.EndpointName
		}
	}

	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()
	bestName := ""
	bestPriority := 0
	for _, ep := range m.endpoints {
		if ep == nil {
			continue
		}
		ep.mutex.RLock()
		name := ep.Config.Name
		priority := ep.Config.Priority
		hardEnabled := ep.Config.IsAvailabilityEnabled()
		autoScheduleEnabled := ep.Config.IsAutoScheduleEnabled()
		ep.mutex.RUnlock()
		if !hardEnabled || !autoScheduleEnabled {
			continue
		}
		if bestName == "" || priority < bestPriority {
			bestName = name
			bestPriority = priority
		}
	}
	return bestName
}
