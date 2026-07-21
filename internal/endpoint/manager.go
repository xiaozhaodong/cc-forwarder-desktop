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
	"sync"
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
	NeverChecked     bool      // 表示从未被检测过
	CooldownUntil    time.Time // 请求失败冷却截止时间
	CooldownReason   string    // 冷却原因（如 "HTTP 503"）
	PausedUntil      time.Time // 手动暂停截止时间（零值=未暂停；到期读取时自愈）
}

// Endpoint represents an endpoint with its configuration and status
type Endpoint struct {
	Config config.EndpointConfig
	Status EndpointStatus
	mutex  sync.RWMutex
}

// RLock 锁定端点状态读锁（供外部安全读取状态）
func (e *Endpoint) RLock() {
	e.mutex.RLock()
}

// RUnlock 解锁端点状态读锁
func (e *Endpoint) RUnlock() {
	e.mutex.RUnlock()
}

// IsInCooldown 检查端点是否处于冷却状态
func (e *Endpoint) IsInCooldown() bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return !e.Status.CooldownUntil.IsZero() && time.Now().Before(e.Status.CooldownUntil)
}

// IsPaused 检查端点是否处于手动暂停状态（PausedUntil 到期自动视为恢复）
func (e *Endpoint) IsPaused() bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return !e.Status.PausedUntil.IsZero() && time.Now().Before(e.Status.PausedUntil)
}

// Manager manages endpoints and their health status
type Manager struct {
	endpoints      []*Endpoint
	endpointsMu    sync.RWMutex // v5.0+: 保护 endpoints 切片的并发访问
	configMu       sync.RWMutex
	config         *config.Config
	client         *http.Client
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	fastTester     *FastTester
	keyManager     *KeyManager     // 管理多 API Key 状态
	failureTracker *FailureTracker // 失败追踪器，用于检测端点持续故障
	routeOverride  *RouteOverride
	routeState     *RouteState
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
		ctx:           ctx,
		cancel:        cancel,
		fastTester:    NewFastTester(cfg),
		keyManager:    NewKeyManager(), // 初始化 Key 管理器
		routeOverride: NewRouteOverride(),
		routeState:    NewRouteState(),
		failureTracker: NewFailureTracker(
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
	if m.failureTracker != nil {
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

	// 🔧 [热更新] 同步更新失败追踪器配置
	if m.failureTracker != nil {
		m.failureTracker.UpdateConfig(cfg.FailureTracker.Enabled, cfg.FailureTracker.TimeWindow, cfg.FailureTracker.Threshold)
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

// RecordFailure 记录端点失败，返回当前窗口内失败次数
func (m *Manager) RecordFailure(endpointName string) int {
	return m.failureTracker.RecordFailure(endpointName)
}

// RecordSuccess 记录端点成功，清空失败记录
func (m *Manager) RecordSuccess(endpointName string) {
	m.failureTracker.RecordSuccess(endpointName)
}

// GetFailureStats 获取失败统计信息
func (m *Manager) GetFailureStats() map[string]int {
	return m.failureTracker.GetStats()
}

// ShouldTriggerFailureAction 检查指定端点是否达到失败阈值
// 用于健康检查回退逻辑中过滤端点
func (m *Manager) ShouldTriggerFailureAction(endpointName string) bool {
	return m.failureTracker.ShouldTriggerAction(endpointName)
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
			m.failureTracker.CleanupExpiredEvents()
		}
	}
}

// IsEndpointRoutable reports whether an endpoint can currently participate in routing.
// 调度仅依赖真实请求失败追踪和冷却状态，不依赖后台健康轮询。
func (m *Manager) IsEndpointRoutable(ep *Endpoint) bool {
	if ep == nil {
		return false
	}

	cfg := m.getConfigSnapshot()
	if cfg.FailureTracker.Enabled && m.failureTracker.ShouldTriggerAction(ep.Config.Name) {
		return false
	}

	return !ep.IsInCooldown()
}

// UpdateFailureTrackerConfig 热更新失败追踪器配置
func (m *Manager) UpdateFailureTrackerConfig(enabled bool, timeWindow time.Duration, threshold int) {
	m.failureTracker.UpdateConfig(enabled, timeWindow, threshold)
}

// ShouldRejectRequest 检查是否应该拒绝请求
// 当 FailureTracker 配置为 "reject" 模式且当前 active 端点达到失败阈值时返回 true
// 返回: (shouldReject, rejectedEndpointName)
func (m *Manager) ShouldRejectRequest() (bool, string) {
	cfg := m.getConfigSnapshot()

	// 未启用失败追踪或不是 reject 模式，不拒绝
	if !cfg.FailureTracker.Enabled || cfg.FailureTracker.Action != "reject" {
		return false, ""
	}

	// v7：活跃端点 = activeEndpoint（组体系已退役）
	activeName, _ := m.GetActiveEndpointSelection()
	if activeName == "" {
		return false, ""
	}
	if m.failureTracker.ShouldTriggerAction(activeName) {
		return true, activeName
	}

	return false, ""
}
