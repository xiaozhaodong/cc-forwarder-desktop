// endpoint_crud.go - 动态端点管理功能
// 包含端点的增删改查操作（v5.0+ 新增）

package endpoint

import (
	"fmt"
	"log/slog"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/events"
)

// SyncEndpoints 从数据库同步端点（v5.0 Desktop 专用）
// 用于启动时从数据库加载端点，替换现有端点列表
func (m *Manager) SyncEndpoints(configs []config.EndpointConfig) {
	// 创建新端点列表
	endpoints := make([]*Endpoint, len(configs))
	for i, cfg := range configs {
		endpoints[i] = &Endpoint{
			Config: cfg,
			Status: EndpointStatus{
				Healthy:      false,
				NeverChecked: true,
			},
		}

		// 初始化 Key 管理状态
		tokenCount := len(cfg.Tokens)
		if tokenCount == 0 && cfg.Token != "" {
			tokenCount = 1
		}
		apiKeyCount := len(cfg.ApiKeys)
		if apiKeyCount == 0 && cfg.ApiKey != "" {
			apiKeyCount = 1
		}
		m.keyManager.InitEndpoint(cfg.Name, tokenCount, apiKeyCount)
	}

	// 使用写锁替换端点列表
	m.endpointsMu.Lock()
	m.endpoints = endpoints
	m.endpointsMu.Unlock()

	// 更新 GroupManager（创建组）
	m.groupManager.UpdateGroups(endpoints)

	slog.Info(fmt.Sprintf("🔄 [端点同步] 已同步 %d 个端点到管理器", len(configs)))
}

// AddEndpoint 动态添加端点（v5.0+ 新增）
// 线程安全地将新端点添加到管理器中
func (m *Manager) AddEndpoint(cfg config.EndpointConfig) error {
	// 验证端点名称唯一性
	m.endpointsMu.RLock()
	for _, ep := range m.endpoints {
		if ep.Config.Name == cfg.Name {
			m.endpointsMu.RUnlock()
			return fmt.Errorf("端点 '%s' 已存在", cfg.Name)
		}
	}
	m.endpointsMu.RUnlock()

	// 创建新端点
	endpoint := &Endpoint{
		Config: cfg,
		Status: EndpointStatus{
			Healthy:      false,
			NeverChecked: true,
		},
	}

	// 初始化 Key 管理状态
	tokenCount := len(cfg.Tokens)
	if tokenCount == 0 && cfg.Token != "" {
		tokenCount = 1
	}
	apiKeyCount := len(cfg.ApiKeys)
	if apiKeyCount == 0 && cfg.ApiKey != "" {
		apiKeyCount = 1
	}
	m.keyManager.InitEndpoint(cfg.Name, tokenCount, apiKeyCount)

	// 使用写锁添加端点
	m.endpointsMu.Lock()
	m.endpoints = append(m.endpoints, endpoint)
	m.endpointsMu.Unlock()

	// 更新 GroupManager
	m.endpointsMu.RLock()
	snapshot := make([]*Endpoint, len(m.endpoints))
	copy(snapshot, m.endpoints)
	m.endpointsMu.RUnlock()
	m.groupManager.UpdateGroups(snapshot)

	// 发布事件通知
	if m.eventBus != nil {
		m.eventBus.Publish(events.Event{
			Type:     "endpoint_added",
			Source:   "endpoint_manager",
			Priority: events.PriorityHigh,
			Data: map[string]interface{}{
				"name":      cfg.Name,
				"url":       cfg.URL,
				"priority":  cfg.Priority,
				"timestamp": time.Now().Format("2006-01-02 15:04:05"),
			},
		})
	}

	slog.Info(fmt.Sprintf("➕ [端点管理] 新增端点: %s (%s)", cfg.Name, cfg.URL))
	return nil
}

// RemoveEndpoint 动态移除端点（v5.0+ 新增）
// 线程安全地从管理器中移除端点
func (m *Manager) RemoveEndpoint(name string) error {
	m.endpointsMu.Lock()
	defer m.endpointsMu.Unlock()

	// 查找并移除端点
	index := -1
	for i, ep := range m.endpoints {
		if ep.Config.Name == name {
			index = i
			break
		}
	}

	if index == -1 {
		return fmt.Errorf("端点 '%s' 未找到", name)
	}

	// 移除端点（保持切片顺序）
	removedEndpoint := m.endpoints[index]
	m.endpoints = append(m.endpoints[:index], m.endpoints[index+1:]...)

	// 清理 KeyManager 状态
	m.keyManager.RemoveEndpoint(name)

	// 清理 FailureTracker 记录（避免内存泄漏）
	if m.failureTracker != nil {
		m.failureTracker.ClearEndpoint(name)
	}

	// 更新 GroupManager（在锁内创建快照）
	snapshot := make([]*Endpoint, len(m.endpoints))
	copy(snapshot, m.endpoints)

	// 在锁外更新 GroupManager
	go func() {
		m.groupManager.UpdateGroups(snapshot)
	}()

	// 发布事件通知
	if m.eventBus != nil {
		m.eventBus.Publish(events.Event{
			Type:     "endpoint_removed",
			Source:   "endpoint_manager",
			Priority: events.PriorityHigh,
			Data: map[string]interface{}{
				"name":      name,
				"url":       removedEndpoint.Config.URL,
				"timestamp": time.Now().Format("2006-01-02 15:04:05"),
			},
		})
	}

	slog.Info(fmt.Sprintf("➖ [端点管理] 移除端点: %s", name))
	return nil
}

// UpdateEndpointConfig 更新端点配置（v5.0+ 新增）
// 更新现有端点的配置（不包括名称）
func (m *Manager) UpdateEndpointConfig(name string, cfg config.EndpointConfig) error {
	m.endpointsMu.RLock()
	var targetEndpoint *Endpoint
	for _, ep := range m.endpoints {
		if ep.Config.Name == name {
			targetEndpoint = ep
			break
		}
	}
	m.endpointsMu.RUnlock()

	if targetEndpoint == nil {
		return fmt.Errorf("端点 '%s' 未找到", name)
	}

	// 保留原名称
	cfg.Name = name

	// 更新配置
	targetEndpoint.mutex.Lock()
	targetEndpoint.Config = cfg
	targetEndpoint.mutex.Unlock()

	// 更新 Key 管理状态
	tokenCount := len(cfg.Tokens)
	if tokenCount == 0 && cfg.Token != "" {
		tokenCount = 1
	}
	apiKeyCount := len(cfg.ApiKeys)
	if apiKeyCount == 0 && cfg.ApiKey != "" {
		apiKeyCount = 1
	}
	m.keyManager.UpdateEndpointKeyCount(name, tokenCount, apiKeyCount)

	// 更新 GroupManager
	m.endpointsMu.RLock()
	snapshot := make([]*Endpoint, len(m.endpoints))
	copy(snapshot, m.endpoints)
	m.endpointsMu.RUnlock()
	m.groupManager.UpdateGroups(snapshot)

	// 发布事件通知
	if m.eventBus != nil {
		m.eventBus.Publish(events.Event{
			Type:     "endpoint_updated",
			Source:   "endpoint_manager",
			Priority: events.PriorityHigh,
			Data: map[string]interface{}{
				"name":      name,
				"url":       cfg.URL,
				"priority":  cfg.Priority,
				"timestamp": time.Now().Format("2006-01-02 15:04:05"),
			},
		})
	}

	slog.Info(fmt.Sprintf("✏️ [端点管理] 更新端点配置: %s", name))
	return nil
}

// UpdateEndpointPriority updates the priority of an endpoint by name
// 注意: v5.0 SQLite 模式下，优先级应通过 UpdateEndpointConfig 更新
// 此函数保留用于 YAML 配置模式的向后兼容
func (m *Manager) UpdateEndpointPriority(name string, newPriority int) error {
	if newPriority < 1 {
		return fmt.Errorf("优先级必须大于等于1")
	}

	m.endpointsMu.RLock()
	// Find the endpoint
	var targetEndpoint *Endpoint
	for _, ep := range m.endpoints {
		if ep.Config.Name == name {
			targetEndpoint = ep
			break
		}
	}
	m.endpointsMu.RUnlock()

	if targetEndpoint == nil {
		return fmt.Errorf("端点 '%s' 未找到", name)
	}

	// Update the priority with lock
	targetEndpoint.mutex.Lock()
	targetEndpoint.Config.Priority = newPriority
	targetEndpoint.mutex.Unlock()

	slog.Info(fmt.Sprintf("🔄 端点优先级已更新: %s -> %d", name, newPriority))

	return nil
}
