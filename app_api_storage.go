// app_api_storage.go - v5.0+ 端点存储管理 API (Wails Bindings)
// 提供 SQLite 端点存储的增删改查功能

package main

import (
	"context"
	"fmt"
	"time"

	"cc-forwarder/config"
	"cc-forwarder/internal/store"
	"cc-forwarder/internal/utils"
)

// ============================================================
// v5.0+ 端点存储管理 API (SQLite)
// ============================================================
// 这些 API 仅在 endpoints_storage.type 为 "sqlite" 时可用
// 提供端点的增删改查功能

// EndpointRecordInfo 端点记录信息（给前端用的结构体）
type EndpointRecordInfo struct {
	ID                            int64             `json:"id"`
	Channel                       string            `json:"channel"`
	Name                          string            `json:"name"`
	URL                           string            `json:"url"`
	Token                         string            `json:"token"`        // v5.0: 本地桌面应用，直接返回原始 Token
	ApiKey                        string            `json:"api_key"`      // v5.0: 本地桌面应用，直接返回原始 ApiKey
	TokenMasked                   string            `json:"token_masked"` // 脱敏后的 Token（列表展示用）
	ApiKeyMasked                  string            `json:"api_key_masked"`
	Headers                       map[string]string `json:"headers"`
	Priority                      int               `json:"priority"`
	FailoverEnabled               bool              `json:"failover_enabled"`
	CooldownSeconds               *int              `json:"cooldown_seconds"`
	TimeoutSeconds                int               `json:"timeout_seconds"`
	SupportsCountTokens           bool              `json:"supports_count_tokens"`
	CostMultiplier                float64           `json:"cost_multiplier"`
	InputCostMultiplier           float64           `json:"input_cost_multiplier"`
	OutputCostMultiplier          float64           `json:"output_cost_multiplier"`
	CacheCreationCostMultiplier   float64           `json:"cache_creation_cost_multiplier"`
	CacheCreationCostMultiplier1h float64           `json:"cache_creation_cost_multiplier_1h"`
	CacheReadCostMultiplier       float64           `json:"cache_read_cost_multiplier"`
	Enabled                       bool              `json:"enabled"`
	CreatedAt                     string            `json:"created_at"`
	UpdatedAt                     string            `json:"updated_at"`
	// 运行时健康状态
	Healthy        bool    `json:"healthy"`
	NeverChecked   bool    `json:"never_checked"`
	LastCheck      string  `json:"last_check"` // 最近连通性测试时间
	ResponseTimeMs float64 `json:"response_time_ms"`
	// 冷却状态（请求级故障转移）
	InCooldown     bool   `json:"in_cooldown"`     // 是否处于冷却中
	CooldownUntil  string `json:"cooldown_until"`  // 冷却截止时间
	CooldownReason string `json:"cooldown_reason"` // 冷却原因
}

// CreateEndpointInput 创建端点的输入参数
type CreateEndpointInput struct {
	Channel                       string            `json:"channel"`
	Name                          string            `json:"name"`
	URL                           string            `json:"url"`
	Token                         string            `json:"token"`
	ApiKey                        string            `json:"api_key"`
	Headers                       map[string]string `json:"headers"`
	Priority                      int               `json:"priority"`
	FailoverEnabled               bool              `json:"failover_enabled"`
	CooldownSeconds               *int              `json:"cooldown_seconds"`
	TimeoutSeconds                int               `json:"timeout_seconds"`
	SupportsCountTokens           bool              `json:"supports_count_tokens"`
	CostMultiplier                float64           `json:"cost_multiplier"`
	InputCostMultiplier           float64           `json:"input_cost_multiplier"`
	OutputCostMultiplier          float64           `json:"output_cost_multiplier"`
	CacheCreationCostMultiplier   float64           `json:"cache_creation_cost_multiplier"`
	CacheCreationCostMultiplier1h float64           `json:"cache_creation_cost_multiplier_1h"`
	CacheReadCostMultiplier       float64           `json:"cache_read_cost_multiplier"`
}

// EndpointStorageStatus 端点存储状态
type EndpointStorageStatus struct {
	Enabled      bool   `json:"enabled"`       // 是否启用 SQLite 存储
	StorageType  string `json:"storage_type"`  // "yaml" 或 "sqlite"
	TotalCount   int    `json:"total_count"`   // 端点总数
	EnabledCount int    `json:"enabled_count"` // 已启用端点数
}

// GetEndpointStorageStatus 获取端点存储状态
func (a *App) GetEndpointStorageStatus() EndpointStorageStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()

	status := EndpointStorageStatus{
		StorageType: "yaml", // 默认
	}

	if a.config != nil {
		status.StorageType = a.config.EndpointsStorage.Type
	}

	// 如果使用 SQLite 存储且服务已初始化
	if a.endpointService != nil {
		status.Enabled = true

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		records, err := a.endpointService.ListEndpoints(ctx)
		if err == nil {
			status.TotalCount = len(records)
			for _, r := range records {
				if r.Enabled {
					status.EnabledCount++
				}
			}
		}
	}

	return status
}

// GetEndpointRecords 获取所有端点记录（SQLite 存储）
func (a *App) GetEndpointRecords() ([]EndpointRecordInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointService == nil {
		return nil, fmt.Errorf("端点存储服务未启用 (需要设置 endpoints_storage.type: sqlite)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	records, err := a.endpointService.ListEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取端点列表失败: %w", err)
	}

	result := make([]EndpointRecordInfo, 0, len(records))
	for _, r := range records {
		info := a.recordToInfo(r)

		// 获取运行时健康状态
		if a.endpointManager != nil {
			status := a.endpointManager.GetEndpointStatus(r.Name)
			info.Healthy = status.Healthy
			info.NeverChecked = status.NeverChecked
			info.ResponseTimeMs = float64(status.ResponseTime.Milliseconds())
			// 格式化最后检查时间
			if !status.LastCheck.IsZero() {
				info.LastCheck = status.LastCheck.Format("2006-01-02 15:04:05")
			}
			// 冷却状态
			if !status.CooldownUntil.IsZero() && status.CooldownUntil.After(time.Now()) {
				info.InCooldown = true
				info.CooldownUntil = status.CooldownUntil.Format("2006-01-02 15:04:05")
				info.CooldownReason = status.CooldownReason
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// GetEndpointRecord 获取单个端点详情
func (a *App) GetEndpointRecord(name string) (EndpointRecordInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointService == nil {
		return EndpointRecordInfo{}, fmt.Errorf("端点存储服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	detail, err := a.endpointService.GetEndpointWithHealth(ctx, name)
	if err != nil {
		return EndpointRecordInfo{}, err
	}

	// 转换为 EndpointRecordInfo
	info := EndpointRecordInfo{
		Name:    name,
		Enabled: true,
	}

	// 从 map 中提取字段
	if v, ok := detail["id"].(int64); ok {
		info.ID = v
	}
	if v, ok := detail["channel"].(string); ok {
		info.Channel = v
	}
	if v, ok := detail["url"].(string); ok {
		info.URL = v
	}
	if v, ok := detail["token_masked"].(string); ok {
		info.TokenMasked = v
	}
	if v, ok := detail["priority"].(int); ok {
		info.Priority = v
	}
	if v, ok := detail["failover_enabled"].(bool); ok {
		info.FailoverEnabled = v
	}
	if v, ok := detail["timeout_seconds"].(int); ok {
		info.TimeoutSeconds = v
	}
	if v, ok := detail["cost_multiplier"].(float64); ok {
		info.CostMultiplier = v
	}
	if v, ok := detail["cache_creation_cost_multiplier_1h"].(float64); ok {
		info.CacheCreationCostMultiplier1h = v
	}
	if v, ok := detail["enabled"].(bool); ok {
		info.Enabled = v
	}

	// 健康状态
	if health, ok := detail["health"].(map[string]interface{}); ok {
		if v, ok := health["healthy"].(bool); ok {
			info.Healthy = v
		}
		if v, ok := health["never_checked"].(bool); ok {
			info.NeverChecked = v
		}
		if v, ok := health["response_time_ms"].(int64); ok {
			info.ResponseTimeMs = float64(v)
		}
		if v, ok := health["last_check"].(string); ok {
			info.LastCheck = v
		}
	}

	return info, nil
}

// CreateEndpointRecord 创建新端点
func (a *App) CreateEndpointRecord(input CreateEndpointInput) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointService == nil {
		return fmt.Errorf("端点存储服务未启用")
	}

	// 设置默认值
	if input.Priority == 0 {
		input.Priority = 1
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = 300
	}
	if input.CostMultiplier == 0 {
		input.CostMultiplier = 1.0
	}

	record := &store.EndpointRecord{
		Channel:                       input.Channel,
		Name:                          input.Name,
		URL:                           input.URL,
		Token:                         input.Token,
		ApiKey:                        input.ApiKey,
		Headers:                       input.Headers,
		Priority:                      input.Priority,
		FailoverEnabled:               input.FailoverEnabled,
		CooldownSeconds:               input.CooldownSeconds,
		TimeoutSeconds:                input.TimeoutSeconds,
		SupportsCountTokens:           input.SupportsCountTokens,
		CostMultiplier:                input.CostMultiplier,
		InputCostMultiplier:           input.InputCostMultiplier,
		OutputCostMultiplier:          input.OutputCostMultiplier,
		CacheCreationCostMultiplier:   input.CacheCreationCostMultiplier,
		CacheCreationCostMultiplier1h: input.CacheCreationCostMultiplier1h,
		CacheReadCostMultiplier:       input.CacheReadCostMultiplier,
		Enabled:                       false, // v5.0: 新建端点默认不激活，需手动激活
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := a.endpointService.CreateEndpoint(ctx, record)
	if err != nil {
		return fmt.Errorf("创建端点失败: %w", err)
	}

	if a.logger != nil {
		a.logger.Info("✅ 端点已创建", "name", input.Name, "channel", input.Channel)
	}

	// v5.0: endpointService.CreateEndpoint 已经将端点添加到内存并触发健康检测
	// 不需要再次调用 AddEndpoint，否则会导致 "端点已存在" 错误

	// v5.0: 创建成功后，异步同步端点倍率到 UsageTracker
	go a.syncEndpointMultipliersToTracker(context.Background())

	return nil
}

// UpdateEndpointRecord 更新端点
func (a *App) UpdateEndpointRecord(name string, input CreateEndpointInput) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointService == nil {
		return fmt.Errorf("端点存储服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// v5.0: 从数据库获取当前记录，用于保留敏感字段
	existingRecord, err := a.endpointService.GetEndpoint(ctx, name)
	if err != nil {
		return fmt.Errorf("获取端点失败: %w", err)
	}

	// 处理 Token: 如果前端传空值，保留原有 Token（防止误删）
	token := input.Token
	if token == "" {
		token = existingRecord.Token
	}

	// 处理 ApiKey: 如果前端传空值，保留原有 ApiKey（防止误删）
	apiKey := input.ApiKey
	if apiKey == "" {
		apiKey = existingRecord.ApiKey
	}

	record := &store.EndpointRecord{
		Channel:                       input.Channel,
		Name:                          name, // 使用 URL 参数中的 name
		URL:                           input.URL,
		Token:                         token,  // 空值时保留原有值
		ApiKey:                        apiKey, // 空值时保留原有值
		Headers:                       input.Headers,
		Priority:                      input.Priority,
		FailoverEnabled:               input.FailoverEnabled,
		CooldownSeconds:               input.CooldownSeconds,
		TimeoutSeconds:                input.TimeoutSeconds,
		SupportsCountTokens:           input.SupportsCountTokens,
		CostMultiplier:                input.CostMultiplier,
		InputCostMultiplier:           input.InputCostMultiplier,
		OutputCostMultiplier:          input.OutputCostMultiplier,
		CacheCreationCostMultiplier:   input.CacheCreationCostMultiplier,
		CacheCreationCostMultiplier1h: input.CacheCreationCostMultiplier1h,
		CacheReadCostMultiplier:       input.CacheReadCostMultiplier,
		Enabled:                       existingRecord.Enabled, // 保持原有激活状态
	}

	if err := a.endpointService.UpdateEndpoint(ctx, record); err != nil {
		return fmt.Errorf("更新端点失败: %w", err)
	}

	if a.logger != nil {
		a.logger.Info("✅ 端点已更新", "name", name)
	}

	// v5.0: 同步更新内存中的端点配置（确保 Key 等配置立即生效）
	if a.endpointManager != nil {
		// 构建 failover_enabled 指针
		failoverEnabled := input.FailoverEnabled

		// 构建 config.EndpointConfig
		endpointCfg := config.EndpointConfig{
			Name:                name,
			URL:                 input.URL,
			Channel:             input.Channel,
			Priority:            input.Priority,
			FailoverEnabled:     &failoverEnabled,
			Token:               token,  // 使用处理后的值（空值时保留原有）
			ApiKey:              apiKey, // 使用处理后的值（空值时保留原有）
			Timeout:             time.Duration(input.TimeoutSeconds) * time.Second,
			Headers:             input.Headers,
			SupportsCountTokens: input.SupportsCountTokens,
		}

		// 更新内存中的端点配置
		if err := a.endpointManager.UpdateEndpointConfig(name, endpointCfg); err != nil {
			a.logger.Warn("⚠️ 同步端点配置到内存失败", "name", name, "error", err)
			// 不返回错误，数据库已更新成功
		} else {
			a.logger.Debug("✅ 端点配置已同步到内存", "name", name)
		}
	}

	// v5.0: 更新成功后，异步同步端点倍率到 UsageTracker
	// 确保成本计算使用最新的倍率配置
	go a.syncEndpointMultipliersToTracker(context.Background())

	return nil
}

// DeleteEndpointRecord 删除端点
func (a *App) DeleteEndpointRecord(name string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointService == nil {
		return fmt.Errorf("端点存储服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.endpointService.DeleteEndpoint(ctx, name); err != nil {
		return fmt.Errorf("删除端点失败: %w", err)
	}

	if a.logger != nil {
		a.logger.Info("✅ 端点已删除", "name", name)
	}

	// v5.0: 删除成功后，异步同步端点倍率到 UsageTracker
	go a.syncEndpointMultipliersToTracker(context.Background())

	return nil
}

// ToggleEndpointRecord 切换端点启用状态
// v5.0: enabled=true 时激活组，enabled=false 时不操作（组管理器会自动处理）
func (a *App) ToggleEndpointRecord(name string, enabled bool) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointService == nil {
		return fmt.Errorf("端点存储服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// v5.0: 激活端点时实现互斥
	if enabled {
		// 1. 数据库层互斥：先停用所有端点
		if err := a.endpointService.DisableAllEndpoints(ctx); err != nil {
			return fmt.Errorf("停用其他端点失败: %w", err)
		}

		// 2. 激活目标端点（数据库）
		if err := a.endpointService.ToggleEndpoint(ctx, name, true); err != nil {
			return fmt.Errorf("激活端点失败: %w", err)
		}

		// 3. 确保端点在内存中存在（处理新建端点的情况）
		if a.endpointManager != nil {
			// 从数据库获取完整的端点配置
			record, err := a.endpointService.GetEndpoint(ctx, name)
			if err != nil {
				return fmt.Errorf("获取端点配置失败: %w", err)
			}

			// 构建 config.EndpointConfig
			failoverEnabled := record.FailoverEnabled
			endpointCfg := config.EndpointConfig{
				Name:                record.Name,
				URL:                 record.URL,
				Channel:             record.Channel,
				Priority:            record.Priority,
				FailoverEnabled:     &failoverEnabled,
				Token:               record.Token,
				ApiKey:              record.ApiKey,
				Timeout:             time.Duration(record.TimeoutSeconds) * time.Second,
				Headers:             record.Headers,
				SupportsCountTokens: record.SupportsCountTokens,
			}

			// 尝试添加端点（如果已存在会更新配置）
			if err := a.endpointManager.AddEndpoint(endpointCfg); err != nil {
				// 端点已存在，尝试更新配置
				if updateErr := a.endpointManager.UpdateEndpointConfig(name, endpointCfg); updateErr != nil {
					a.logger.Warn("⚠️ 更新端点配置失败", "name", name, "error", updateErr)
				}
			} else {
				a.logger.Info("✅ 新端点已添加到内存", "name", name)
			}

			// 4. 激活对应的组
			if err := a.endpointManager.ManualActivateGroup(name); err != nil {
				return fmt.Errorf("激活组失败: %w", err)
			}
		}
	} else {
		// 停用端点：只更新数据库
		if err := a.endpointService.ToggleEndpoint(ctx, name, false); err != nil {
			return fmt.Errorf("切换端点状态失败: %w", err)
		}
	}

	status := "启用"
	if !enabled {
		status = "禁用"
	}

	if a.logger != nil {
		a.logger.Info("✅ 端点状态已切换", "name", name, "status", status)
	}

	return nil
}

// ChannelInfo 渠道信息
type ChannelInfo struct {
	Name          string `json:"name"`
	EndpointCount int    `json:"endpoint_count"`
}

// GetChannels 获取所有渠道
func (a *App) GetChannels() ([]ChannelInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointService == nil {
		return nil, fmt.Errorf("端点存储服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	records, err := a.endpointService.ListEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取端点列表失败: %w", err)
	}

	// 统计每个渠道的端点数
	channelMap := make(map[string]int)
	for _, r := range records {
		channelMap[r.Channel]++
	}

	result := make([]ChannelInfo, 0, len(channelMap))
	for name, count := range channelMap {
		result = append(result, ChannelInfo{
			Name:          name,
			EndpointCount: count,
		})
	}

	return result, nil
}

// GetEndpointsByChannel 按渠道获取端点
func (a *App) GetEndpointsByChannel(channel string) ([]EndpointRecordInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.endpointService == nil {
		return nil, fmt.Errorf("端点存储服务未启用")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	records, err := a.endpointService.ListEndpointsByChannel(ctx, channel)
	if err != nil {
		return nil, fmt.Errorf("获取渠道端点列表失败: %w", err)
	}

	result := make([]EndpointRecordInfo, 0, len(records))
	for _, r := range records {
		info := a.recordToInfo(r)

		// 获取运行时健康状态
		if a.endpointManager != nil {
			status := a.endpointManager.GetEndpointStatus(r.Name)
			info.Healthy = status.Healthy
			info.ResponseTimeMs = float64(status.ResponseTime.Milliseconds())
			// 格式化最后检查时间
			if !status.LastCheck.IsZero() {
				info.LastCheck = status.LastCheck.Format("2006-01-02 15:04:05")
			}
			// 冷却状态
			if !status.CooldownUntil.IsZero() && status.CooldownUntil.After(time.Now()) {
				info.InCooldown = true
				info.CooldownUntil = status.CooldownUntil.Format("2006-01-02 15:04:05")
				info.CooldownReason = status.CooldownReason
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// recordToInfo 将数据库记录转换为前端 Info 结构
func (a *App) recordToInfo(r *store.EndpointRecord) EndpointRecordInfo {
	info := EndpointRecordInfo{
		ID:                            r.ID,
		Channel:                       r.Channel,
		Name:                          r.Name,
		URL:                           r.URL,
		Token:                         r.Token,  // v5.0: 本地桌面应用，直接返回原始 Token
		ApiKey:                        r.ApiKey, // v5.0: 本地桌面应用，直接返回原始 ApiKey
		TokenMasked:                   maskToken(r.Token),
		ApiKeyMasked:                  maskToken(r.ApiKey),
		Headers:                       r.Headers,
		Priority:                      r.Priority,
		FailoverEnabled:               r.FailoverEnabled,
		CooldownSeconds:               r.CooldownSeconds,
		TimeoutSeconds:                r.TimeoutSeconds,
		SupportsCountTokens:           r.SupportsCountTokens,
		CostMultiplier:                r.CostMultiplier,
		InputCostMultiplier:           r.InputCostMultiplier,
		OutputCostMultiplier:          r.OutputCostMultiplier,
		CacheCreationCostMultiplier:   r.CacheCreationCostMultiplier,
		CacheCreationCostMultiplier1h: r.CacheCreationCostMultiplier1h,
		CacheReadCostMultiplier:       r.CacheReadCostMultiplier,
		Enabled:                       r.Enabled,
	}

	if !r.CreatedAt.IsZero() {
		info.CreatedAt = r.CreatedAt.Format("2006-01-02 15:04:05")
	}
	if !r.UpdatedAt.IsZero() {
		info.UpdatedAt = r.UpdatedAt.Format("2006-01-02 15:04:05")
	}

	return info
}

// maskToken Token 脱敏显示
func maskToken(token string) string {
	return utils.MaskToken(token)
}
