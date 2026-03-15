// Package service 提供业务逻辑层实现
// 端点服务 - v5.0.0 新增 (2025-12-05)
package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/endpoint"
	"cc-forwarder/internal/store"
	"cc-forwarder/internal/utils"
)

// EndpointService 端点管理业务服务
// 连接 EndpointStore（数据持久化）和 EndpointManager（运行时管理）
type EndpointService struct {
	store   store.EndpointStore
	manager *endpoint.Manager
	config  *config.Config
}

// NewEndpointService 创建端点服务实例
func NewEndpointService(
	store store.EndpointStore,
	manager *endpoint.Manager,
	cfg *config.Config,
) *EndpointService {
	return &EndpointService{
		store:   store,
		manager: manager,
		config:  cfg,
	}
}

// CreateEndpoint 创建新端点
// 1. 保存到数据库
// 2. 添加到运行时管理器
func (s *EndpointService) CreateEndpoint(ctx context.Context, record *store.EndpointRecord) (*store.EndpointRecord, error) {
	// 验证必填字段
	if err := s.validateRecord(record); err != nil {
		return nil, err
	}

	// 检查名称唯一性（运行时）
	if existing := s.manager.GetEndpointByNameAny(record.Name); existing != nil {
		return nil, fmt.Errorf("端点 '%s' 已存在", record.Name)
	}

	// 保存到数据库
	created, err := s.store.Create(ctx, record)
	if err != nil {
		return nil, fmt.Errorf("保存端点到数据库失败: %w", err)
	}

	// 转换为配置并添加到管理器
	cfg := s.recordToConfig(created)
	if err := s.manager.AddEndpoint(cfg); err != nil {
		// 回滚数据库操作
		_ = s.store.Delete(ctx, record.Name)
		return nil, fmt.Errorf("添加端点到管理器失败: %w", err)
	}

	slog.Info(fmt.Sprintf("✅ [EndpointService] 创建端点成功: %s", record.Name))
	return created, nil
}

// GetEndpoint 获取端点详情
func (s *EndpointService) GetEndpoint(ctx context.Context, name string) (*store.EndpointRecord, error) {
	record, err := s.store.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("获取端点失败: %w", err)
	}
	return record, nil
}

// ListEndpoints 列出所有端点
func (s *EndpointService) ListEndpoints(ctx context.Context) ([]*store.EndpointRecord, error) {
	records, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("列出端点失败: %w", err)
	}
	return records, nil
}

// ListEndpointsByChannel 按渠道列出端点
func (s *EndpointService) ListEndpointsByChannel(ctx context.Context, channel string) ([]*store.EndpointRecord, error) {
	records, err := s.store.ListByChannel(ctx, channel)
	if err != nil {
		return nil, fmt.Errorf("按渠道列出端点失败: %w", err)
	}
	return records, nil
}

// UpdateEndpoint 更新端点配置
// 1. 更新数据库
// 2. 更新运行时管理器
func (s *EndpointService) UpdateEndpoint(ctx context.Context, record *store.EndpointRecord) error {
	// 验证端点存在
	existing, err := s.store.Get(ctx, record.Name)
	if err != nil {
		return fmt.Errorf("查询端点失败: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("端点 '%s' 不存在", record.Name)
	}

	// 验证必填字段
	if err := s.validateRecord(record); err != nil {
		return err
	}

	// 更新数据库
	if err := s.store.Update(ctx, record); err != nil {
		return fmt.Errorf("更新数据库失败: %w", err)
	}

	// 更新运行时管理器
	cfg := s.recordToConfig(record)
	if err := s.manager.UpdateEndpointConfig(record.Name, cfg); err != nil {
		slog.Warn(fmt.Sprintf("⚠️ [EndpointService] 更新运行时管理器失败: %v", err))
		// 不回滚数据库，下次重启会同步
	}

	slog.Info(fmt.Sprintf("✅ [EndpointService] 更新端点成功: %s", record.Name))
	return nil
}

// DeleteEndpoint 删除端点
// 1. 从运行时管理器移除
// 2. 从数据库删除
func (s *EndpointService) DeleteEndpoint(ctx context.Context, name string) error {
	// 验证端点存在
	existing, err := s.store.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("查询端点失败: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("端点 '%s' 不存在", name)
	}

	// 先从运行时管理器移除
	if err := s.manager.RemoveEndpoint(name); err != nil {
		slog.Warn(fmt.Sprintf("⚠️ [EndpointService] 从管理器移除端点失败: %v", err))
		// 继续删除数据库记录
	}

	// 从数据库删除
	if err := s.store.Delete(ctx, name); err != nil {
		return fmt.Errorf("从数据库删除失败: %w", err)
	}

	slog.Info(fmt.Sprintf("✅ [EndpointService] 删除端点成功: %s", name))
	return nil
}

// ToggleEndpoint 切换端点启用状态
func (s *EndpointService) ToggleEndpoint(ctx context.Context, name string, enabled bool) error {
	record, err := s.store.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("获取端点失败: %w", err)
	}
	if record == nil {
		return fmt.Errorf("端点 '%s' 不存在", name)
	}

	record.Enabled = enabled
	return s.UpdateEndpoint(ctx, record)
}

// DisableAllEndpoints 禁用所有端点
// v5.0: 用于实现互斥激活（激活一个端点前先禁用所有）
func (s *EndpointService) DisableAllEndpoints(ctx context.Context) error {
	records, err := s.store.List(ctx)
	if err != nil {
		return fmt.Errorf("获取端点列表失败: %w", err)
	}

	// 批量更新所有端点为 enabled=false
	for _, record := range records {
		if record.Enabled {
			record.Enabled = false
			if err := s.store.Update(ctx, record); err != nil {
				slog.Warn(fmt.Sprintf("⚠️ [EndpointService] 禁用端点失败: %s - %v", record.Name, err))
			}
		}
	}

	slog.Info("🔄 [EndpointService] 已禁用所有端点（准备激活新端点）")
	return nil
}

// ImportFromYAML 从 YAML 配置导入端点
// clearExisting: 是否清除现有端点
func (s *EndpointService) ImportFromYAML(ctx context.Context, endpoints []config.EndpointConfig, clearExisting bool) (int, error) {
	if clearExisting {
		// 清除现有端点
		existing, err := s.store.List(ctx)
		if err != nil {
			return 0, fmt.Errorf("获取现有端点失败: %w", err)
		}

		names := make([]string, len(existing))
		for i, ep := range existing {
			names[i] = ep.Name
		}

		if len(names) > 0 {
			if err := s.store.BatchDelete(ctx, names); err != nil {
				return 0, fmt.Errorf("清除现有端点失败: %w", err)
			}
		}
	}

	// 转换并导入
	records := make([]*store.EndpointRecord, 0, len(endpoints))
	for _, ep := range endpoints {
		record := s.configToRecord(ep)
		records = append(records, record)
	}

	if err := s.store.BatchCreate(ctx, records); err != nil {
		return 0, fmt.Errorf("批量导入失败: %w", err)
	}

	// 重新加载到管理器
	// 注意：这里简化处理，实际可能需要更精细的同步
	for _, ep := range endpoints {
		_ = s.manager.AddEndpoint(ep)
	}

	slog.Info(fmt.Sprintf("✅ [EndpointService] 从 YAML 导入 %d 个端点", len(records)))
	return len(records), nil
}

// SyncFromDatabase 从数据库同步端点到管理器
// v5.0 Desktop: 加载所有端点参与健康检查，并同步 enabled=true 的端点到组激活状态
func (s *EndpointService) SyncFromDatabase(ctx context.Context) error {
	// 获取所有端点（包括 enabled=false 的）
	records, err := s.store.List(ctx)
	if err != nil {
		return fmt.Errorf("获取端点列表失败: %w", err)
	}

	slog.Info(fmt.Sprintf("🔄 [EndpointService] 从数据库同步 %d 个端点", len(records)))

	// 转换为配置数组
	endpoints := make([]config.EndpointConfig, len(records))
	var enabledEndpointName string
	for i, record := range records {
		endpoints[i] = s.recordToConfig(record)
		// 记录 enabled=true 的端点
		if record.Enabled {
			enabledEndpointName = record.Name
		}
	}

	// 使用专门的同步方法（不走 UpdateConfig）
	s.manager.SyncEndpoints(endpoints)

	// 同步 enabled=true 的端点到组激活状态
	if enabledEndpointName != "" {
		if err := s.manager.ManualActivateGroup(enabledEndpointName); err != nil {
			slog.Warn(fmt.Sprintf("⚠️ [EndpointService] 激活组失败: %s - %v", enabledEndpointName, err))
		} else {
			slog.Info(fmt.Sprintf("✅ [EndpointService] 已激活端点: %s", enabledEndpointName))
		}
	}

	return nil
}

// GetEndpointWithHealth 获取端点详情（包含健康状态）
func (s *EndpointService) GetEndpointWithHealth(ctx context.Context, name string) (map[string]interface{}, error) {
	record, err := s.store.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("获取端点失败: %w", err)
	}
	if record == nil {
		return nil, fmt.Errorf("端点 '%s' 不存在", name)
	}

	// 获取健康状态
	status := s.manager.GetEndpointStatus(name)

	result := map[string]interface{}{
		"id":                                record.ID,
		"channel":                           record.Channel,
		"name":                              record.Name,
		"url":                               record.URL,
		"token_masked":                      maskToken(record.Token),
		"priority":                          record.Priority,
		"failover_enabled":                  record.FailoverEnabled,
		"timeout_seconds":                   record.TimeoutSeconds,
		"cost_multiplier":                   record.CostMultiplier,
		"cache_creation_cost_multiplier_1h": record.CacheCreationCostMultiplier1h,
		"enabled":                           record.Enabled,
		"created_at":                        record.CreatedAt,
		"updated_at":                        record.UpdatedAt,
		"health": map[string]interface{}{
			"healthy":          status.Healthy,
			"never_checked":    status.NeverChecked,
			"last_check":       "",
			"response_time_ms": status.ResponseTime.Milliseconds(),
		},
	}

	if !status.LastCheck.IsZero() {
		health := result["health"].(map[string]interface{})
		health["last_check"] = status.LastCheck.Format("2006-01-02 15:04:05")
	}

	return result, nil
}

// validateRecord 验证端点记录
func (s *EndpointService) validateRecord(record *store.EndpointRecord) error {
	if record.Name == "" {
		return fmt.Errorf("端点名称不能为空")
	}
	if record.URL == "" {
		return fmt.Errorf("端点 URL 不能为空")
	}
	if record.Channel == "" {
		return fmt.Errorf("端点渠道不能为空")
	}
	return nil
}

// recordToConfig 将数据库记录转换为配置对象
func (s *EndpointService) recordToConfig(record *store.EndpointRecord) config.EndpointConfig {
	cfg := config.EndpointConfig{
		Name:                record.Name,
		URL:                 record.URL,
		Channel:             record.Channel,
		Priority:            record.Priority,
		Token:               record.Token,
		ApiKey:              record.ApiKey,
		Headers:             record.Headers,
		Timeout:             time.Duration(record.TimeoutSeconds) * time.Second,
		SupportsCountTokens: record.SupportsCountTokens,
	}

	// v5.0: 设置 Enabled（是否作为代理端点）
	enabled := record.Enabled
	cfg.Enabled = &enabled

	// 设置 FailoverEnabled
	fe := record.FailoverEnabled
	cfg.FailoverEnabled = &fe

	// 设置 Cooldown
	if record.CooldownSeconds != nil {
		cd := time.Duration(*record.CooldownSeconds) * time.Second
		cfg.Cooldown = &cd
	}

	return cfg
}

// configToRecord 将配置对象转换为数据库记录
func (s *EndpointService) configToRecord(cfg config.EndpointConfig) *store.EndpointRecord {
	record := &store.EndpointRecord{
		Channel:             cfg.Name, // 默认使用名称作为渠道
		Name:                cfg.Name,
		URL:                 cfg.URL,
		Token:               cfg.Token,
		ApiKey:              cfg.ApiKey,
		Headers:             cfg.Headers,
		Priority:            cfg.Priority,
		FailoverEnabled:     true, // 默认参与故障转移
		TimeoutSeconds:      int(cfg.Timeout.Seconds()),
		SupportsCountTokens: cfg.SupportsCountTokens,
		CostMultiplier:      1.0,
		Enabled:             true,
	}

	if cfg.FailoverEnabled != nil {
		record.FailoverEnabled = *cfg.FailoverEnabled
	}

	if cfg.Cooldown != nil {
		cd := int(cfg.Cooldown.Seconds())
		record.CooldownSeconds = &cd
	}

	if record.TimeoutSeconds == 0 {
		record.TimeoutSeconds = 300 // 默认 5 分钟
	}

	return record
}

// maskToken 脱敏 Token
func maskToken(token string) string {
	return utils.MaskToken(token)
}
