// Package store 提供数据存储层实现
// 端点存储 - v5.0.0 新增 (2025-12-05)
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// EndpointRecord 表示数据库中的端点记录
type EndpointRecord struct {
	ID int64 `json:"id"`

	// 基本信息
	Channel string `json:"channel"` // 渠道标签（用于分组展示）
	Name    string `json:"name"`    // 端点唯一名称
	URL     string `json:"url"`     // 端点 URL

	// 认证配置
	Token   string            `json:"token,omitempty"`   // Bearer Token
	ApiKey  string            `json:"api_key,omitempty"` // API Key
	Headers map[string]string `json:"headers,omitempty"` // 自定义请求头

	// 路由配置
	Priority        int  `json:"priority"`         // 优先级（数字越小越高）
	FailoverEnabled bool `json:"failover_enabled"` // 是否参与故障转移
	CooldownSeconds *int `json:"cooldown_seconds"` // 冷却时间（秒，nil=使用全局配置）
	TimeoutSeconds  int  `json:"timeout_seconds"`  // 请求超时（秒）

	// 功能支持
	SupportsCountTokens bool   `json:"supports_count_tokens"` // 是否支持 count_tokens
	ModelRewriteRules   string `json:"model_rewrite_rules"`   // CC 端点模型兼容改写规则 JSON

	// 成本倍率
	CostMultiplier                float64 `json:"cost_multiplier"`
	InputCostMultiplier           float64 `json:"input_cost_multiplier"`
	OutputCostMultiplier          float64 `json:"output_cost_multiplier"`
	CacheCreationCostMultiplier   float64 `json:"cache_creation_cost_multiplier"`    // 5分钟缓存创建倍率（默认）
	CacheCreationCostMultiplier1h float64 `json:"cache_creation_cost_multiplier_1h"` // 1小时缓存创建倍率
	CacheReadCostMultiplier       float64 `json:"cache_read_cost_multiplier"`

	// 状态
	Enabled bool `json:"enabled"` // legacy active 标记（v8 起新版停止写入）
	// v8 硬启用（nil 视为 true；false=任何模式都不可使用）
	AvailabilityEnabled *bool `json:"availability_enabled,omitempty"`

	// 审计字段
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsAvailabilityEnabled 硬启用状态（nil 默认 true）
func (r *EndpointRecord) IsAvailabilityEnabled() bool {
	if r == nil || r.AvailabilityEnabled == nil {
		return true
	}
	return *r.AvailabilityEnabled
}

// EndpointStore 定义端点存储接口
type EndpointStore interface {
	// CRUD 操作
	Create(ctx context.Context, record *EndpointRecord) (*EndpointRecord, error)
	Get(ctx context.Context, name string) (*EndpointRecord, error)
	GetByID(ctx context.Context, id int64) (*EndpointRecord, error)
	List(ctx context.Context) ([]*EndpointRecord, error)
	Update(ctx context.Context, record *EndpointRecord) error
	Delete(ctx context.Context, name string) error

	// 批量操作
	BatchCreate(ctx context.Context, records []*EndpointRecord) error
	BatchDelete(ctx context.Context, names []string) error

	// 查询
	ListByChannel(ctx context.Context, channel string) ([]*EndpointRecord, error)
	ListEnabled(ctx context.Context) ([]*EndpointRecord, error)

	// 统计
	Count(ctx context.Context) (int, error)

	// ActivateExclusive 单事务完成「禁用其余端点 + 启用目标端点」。
	// 目标端点不存在时整笔回滚，不留下"全部禁用"的中间态。
	ActivateExclusive(ctx context.Context, name string) error

	// SetEnabled 更新单个端点的启用状态（legacy active 写路径，v8 起仅兼容期使用）
	SetEnabled(ctx context.Context, name string, enabled bool) error

	// SetAvailabilityEnabled 更新硬启用状态（v8）
	SetAvailabilityEnabled(ctx context.Context, name string, enabled bool) error

	// SetFailoverEnabled 更新“参与自动调度”状态（物理列仍为 failover_enabled）
	SetFailoverEnabled(ctx context.Context, name string, enabled bool) error

	// 事务支持
	WithTx(tx *sql.Tx) EndpointStore
}

// SQLiteEndpointStore 实现 EndpointStore 接口
type SQLiteEndpointStore struct {
	db *sql.DB
	tx *sql.Tx // 事务上下文（可选）
	mu sync.RWMutex
}

// NewSQLiteEndpointStore 创建新的 SQLite 端点存储
func NewSQLiteEndpointStore(db *sql.DB) *SQLiteEndpointStore {
	return &SQLiteEndpointStore{db: db}
}

// getQuerier 返回用于执行查询的对象（事务或数据库连接）
func (s *SQLiteEndpointStore) getQuerier() interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
} {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create 创建新端点
func (s *SQLiteEndpointStore) Create(ctx context.Context, record *EndpointRecord) (*EndpointRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 序列化 headers
	headersJSON, err := json.Marshal(record.Headers)
	if err != nil {
		return nil, fmt.Errorf("序列化 headers 失败: %w", err)
	}

	// 设置默认值
	if record.CostMultiplier == 0 {
		record.CostMultiplier = 1.0
	}
	if record.InputCostMultiplier == 0 {
		record.InputCostMultiplier = 1.0
	}
	if record.OutputCostMultiplier == 0 {
		record.OutputCostMultiplier = 1.0
	}
	if record.CacheCreationCostMultiplier == 0 {
		record.CacheCreationCostMultiplier = 1.0
	}
	if record.CacheCreationCostMultiplier1h == 0 {
		record.CacheCreationCostMultiplier1h = 1.0
	}
	if record.CacheReadCostMultiplier == 0 {
		record.CacheReadCostMultiplier = 1.0
	}
	if record.TimeoutSeconds == 0 {
		record.TimeoutSeconds = 300
	}

	query := `
		INSERT INTO endpoints (
			channel, name, url, token, api_key, headers,
			priority, failover_enabled, cooldown_seconds, timeout_seconds,
			supports_count_tokens, model_rewrite_rules,
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			enabled, availability_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := s.getQuerier().ExecContext(ctx, query,
		record.Channel, record.Name, record.URL, record.Token, record.ApiKey, string(headersJSON),
		record.Priority, boolToInt(record.FailoverEnabled), record.CooldownSeconds, record.TimeoutSeconds,
		boolToInt(record.SupportsCountTokens), record.ModelRewriteRules,
		record.CostMultiplier, record.InputCostMultiplier, record.OutputCostMultiplier,
		record.CacheCreationCostMultiplier, record.CacheCreationCostMultiplier1h, record.CacheReadCostMultiplier,
		boolToInt(record.Enabled), boolToInt(record.IsAvailabilityEnabled()),
	)
	if err != nil {
		return nil, fmt.Errorf("创建端点失败: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取插入 ID 失败: %w", err)
	}

	record.ID = id
	record.CreatedAt = time.Now()
	record.UpdatedAt = time.Now()

	return record, nil
}

// Get 根据名称获取端点
func (s *SQLiteEndpointStore) Get(ctx context.Context, name string) (*EndpointRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, channel, name, url, token, api_key, headers,
			priority, failover_enabled, cooldown_seconds, timeout_seconds,
			supports_count_tokens, model_rewrite_rules,
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			enabled, availability_enabled, created_at, updated_at
		FROM endpoints WHERE name = ?
	`

	return s.scanEndpoint(s.getQuerier().QueryRowContext(ctx, query, name))
}

// GetByID 根据 ID 获取端点
func (s *SQLiteEndpointStore) GetByID(ctx context.Context, id int64) (*EndpointRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, channel, name, url, token, api_key, headers,
			priority, failover_enabled, cooldown_seconds, timeout_seconds,
			supports_count_tokens, model_rewrite_rules,
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			enabled, availability_enabled, created_at, updated_at
		FROM endpoints WHERE id = ?
	`

	return s.scanEndpoint(s.getQuerier().QueryRowContext(ctx, query, id))
}

// List 获取所有端点
func (s *SQLiteEndpointStore) List(ctx context.Context) ([]*EndpointRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, channel, name, url, token, api_key, headers,
			priority, failover_enabled, cooldown_seconds, timeout_seconds,
			supports_count_tokens, model_rewrite_rules,
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			enabled, availability_enabled, created_at, updated_at
		FROM endpoints
		ORDER BY priority ASC, channel ASC, name ASC
	`

	return s.scanEndpoints(ctx, query)
}

// Update 更新端点
func (s *SQLiteEndpointStore) Update(ctx context.Context, record *EndpointRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 序列化 headers
	headersJSON, err := json.Marshal(record.Headers)
	if err != nil {
		return fmt.Errorf("序列化 headers 失败: %w", err)
	}

	query := `
		UPDATE endpoints SET
			channel = ?, url = ?, token = ?, api_key = ?, headers = ?,
			priority = ?, failover_enabled = ?, cooldown_seconds = ?, timeout_seconds = ?,
			supports_count_tokens = ?, model_rewrite_rules = ?,
			cost_multiplier = ?, input_cost_multiplier = ?, output_cost_multiplier = ?,
			cache_creation_cost_multiplier = ?, cache_creation_cost_multiplier_1h = ?, cache_read_cost_multiplier = ?,
			enabled = ?, availability_enabled = ?
		WHERE name = ?
	`

	result, err := s.getQuerier().ExecContext(ctx, query,
		record.Channel, record.URL, record.Token, record.ApiKey, string(headersJSON),
		record.Priority, boolToInt(record.FailoverEnabled), record.CooldownSeconds, record.TimeoutSeconds,
		boolToInt(record.SupportsCountTokens), record.ModelRewriteRules,
		record.CostMultiplier, record.InputCostMultiplier, record.OutputCostMultiplier,
		record.CacheCreationCostMultiplier, record.CacheCreationCostMultiplier1h, record.CacheReadCostMultiplier,
		boolToInt(record.Enabled), boolToInt(record.IsAvailabilityEnabled()),
		record.Name,
	)
	if err != nil {
		return fmt.Errorf("更新端点失败: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("端点不存在: %s", record.Name)
	}

	return nil
}

// Delete 删除端点
func (s *SQLiteEndpointStore) Delete(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM endpoints WHERE name = ?`

	result, err := s.getQuerier().ExecContext(ctx, query, name)
	if err != nil {
		return fmt.Errorf("删除端点失败: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("端点不存在: %s", name)
	}

	return nil
}

// ActivateExclusive 单事务完成「禁用其余端点 + 启用目标端点」。
// 任一步失败（含目标端点不存在）整笔回滚。
func (s *SQLiteEndpointStore) ActivateExclusive(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tx != nil {
		return activateExclusiveIn(ctx, s.tx, name)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启激活事务失败: %w", err)
	}
	if err := activateExclusiveIn(ctx, tx, name); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交激活事务失败: %w", err)
	}
	return nil
}

type execerContext interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func activateExclusiveIn(ctx context.Context, execer execerContext, name string) error {
	if _, err := execer.ExecContext(ctx, `UPDATE endpoints SET enabled = 0 WHERE name != ?`, name); err != nil {
		return fmt.Errorf("禁用其余端点失败: %w", err)
	}
	result, err := execer.ExecContext(ctx, `UPDATE endpoints SET enabled = 1 WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("启用目标端点失败: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取启用影响行数失败: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("端点不存在: %s", name)
	}
	return nil
}

// SetEnabled 更新单个端点的启用状态
func (s *SQLiteEndpointStore) SetEnabled(ctx context.Context, name string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.getQuerier().ExecContext(ctx, `UPDATE endpoints SET enabled = ? WHERE name = ?`, boolToInt(enabled), name)
	if err != nil {
		return fmt.Errorf("更新端点启用状态失败: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("端点不存在: %s", name)
	}
	return nil
}

// SetAvailabilityEnabled 更新硬启用状态（v8）
func (s *SQLiteEndpointStore) SetAvailabilityEnabled(ctx context.Context, name string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.getQuerier().ExecContext(ctx,
		`UPDATE endpoints SET availability_enabled = ? WHERE name = ?`, boolToInt(enabled), name)
	if err != nil {
		return fmt.Errorf("更新端点硬启用状态失败: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("端点不存在: %s", name)
	}
	return nil
}

// SetFailoverEnabled 更新“参与自动调度”状态（物理列 failover_enabled）
func (s *SQLiteEndpointStore) SetFailoverEnabled(ctx context.Context, name string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.getQuerier().ExecContext(ctx,
		`UPDATE endpoints SET failover_enabled = ? WHERE name = ?`, boolToInt(enabled), name)
	if err != nil {
		return fmt.Errorf("更新端点自动调度状态失败: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("端点不存在: %s", name)
	}
	return nil
}

// BatchCreate 批量创建端点
func (s *SQLiteEndpointStore) BatchCreate(ctx context.Context, records []*EndpointRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(records) == 0 {
		return nil
	}

	// 使用事务
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO endpoints (
			channel, name, url, token, api_key, headers,
			priority, failover_enabled, cooldown_seconds, timeout_seconds,
			supports_count_tokens, model_rewrite_rules,
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			enabled, availability_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	for _, record := range records {
		// 设置默认值
		if record.CostMultiplier == 0 {
			record.CostMultiplier = 1.0
		}
		if record.InputCostMultiplier == 0 {
			record.InputCostMultiplier = 1.0
		}
		if record.OutputCostMultiplier == 0 {
			record.OutputCostMultiplier = 1.0
		}
		if record.CacheCreationCostMultiplier == 0 {
			record.CacheCreationCostMultiplier = 1.0
		}
		if record.CacheCreationCostMultiplier1h == 0 {
			record.CacheCreationCostMultiplier1h = 1.0
		}
		if record.CacheReadCostMultiplier == 0 {
			record.CacheReadCostMultiplier = 1.0
		}
		if record.TimeoutSeconds == 0 {
			record.TimeoutSeconds = 300
		}

		headersJSON, err := json.Marshal(record.Headers)
		if err != nil {
			return fmt.Errorf("序列化 headers 失败: %w", err)
		}

		_, err = stmt.ExecContext(ctx,
			record.Channel, record.Name, record.URL, record.Token, record.ApiKey, string(headersJSON),
			record.Priority, boolToInt(record.FailoverEnabled), record.CooldownSeconds, record.TimeoutSeconds,
			boolToInt(record.SupportsCountTokens), record.ModelRewriteRules,
			record.CostMultiplier, record.InputCostMultiplier, record.OutputCostMultiplier,
			record.CacheCreationCostMultiplier, record.CacheCreationCostMultiplier1h, record.CacheReadCostMultiplier,
			boolToInt(record.Enabled), boolToInt(record.IsAvailabilityEnabled()),
		)
		if err != nil {
			return fmt.Errorf("插入端点 %s 失败: %w", record.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// BatchDelete 批量删除端点
func (s *SQLiteEndpointStore) BatchDelete(ctx context.Context, names []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(names) == 0 {
		return nil
	}

	placeholders := make([]string, len(names))
	args := make([]interface{}, len(names))
	for i, name := range names {
		placeholders[i] = "?"
		args[i] = name
	}

	query := fmt.Sprintf("DELETE FROM endpoints WHERE name IN (%s)", strings.Join(placeholders, ","))

	_, err := s.getQuerier().ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("批量删除端点失败: %w", err)
	}

	return nil
}

// ListByChannel 根据渠道获取端点
func (s *SQLiteEndpointStore) ListByChannel(ctx context.Context, channel string) ([]*EndpointRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, channel, name, url, token, api_key, headers,
			priority, failover_enabled, cooldown_seconds, timeout_seconds,
			supports_count_tokens, model_rewrite_rules,
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			enabled, availability_enabled, created_at, updated_at
		FROM endpoints
		WHERE channel = ?
		ORDER BY priority ASC, name ASC
	`

	return s.scanEndpointsWithArgs(ctx, query, channel)
}

// ListEnabled 获取所有启用的端点
func (s *SQLiteEndpointStore) ListEnabled(ctx context.Context) ([]*EndpointRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, channel, name, url, token, api_key, headers,
			priority, failover_enabled, cooldown_seconds, timeout_seconds,
			supports_count_tokens, model_rewrite_rules,
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			enabled, availability_enabled, created_at, updated_at
		FROM endpoints
		WHERE enabled = 1
		ORDER BY priority ASC, channel ASC, name ASC
	`

	return s.scanEndpoints(ctx, query)
}

// Count 获取端点总数
func (s *SQLiteEndpointStore) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.getQuerier().QueryRowContext(ctx, "SELECT COUNT(*) FROM endpoints").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("获取端点数量失败: %w", err)
	}

	return count, nil
}

// WithTx 返回使用事务的存储实例
func (s *SQLiteEndpointStore) WithTx(tx *sql.Tx) EndpointStore {
	return &SQLiteEndpointStore{
		db: s.db,
		tx: tx,
	}
}

// scanEndpoint 从单行扫描端点记录
func (s *SQLiteEndpointStore) scanEndpoint(row *sql.Row) (*EndpointRecord, error) {
	var record EndpointRecord
	var headersJSON string
	var cooldownSeconds sql.NullInt64
	var failoverEnabled, supportsCountTokens, enabled, availabilityEnabled int
	var createdAt, updatedAt string

	err := row.Scan(
		&record.ID, &record.Channel, &record.Name, &record.URL,
		&record.Token, &record.ApiKey, &headersJSON,
		&record.Priority, &failoverEnabled, &cooldownSeconds, &record.TimeoutSeconds,
		&supportsCountTokens, &record.ModelRewriteRules,
		&record.CostMultiplier, &record.InputCostMultiplier, &record.OutputCostMultiplier,
		&record.CacheCreationCostMultiplier, &record.CacheCreationCostMultiplier1h, &record.CacheReadCostMultiplier,
		&enabled, &availabilityEnabled, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("扫描端点记录失败: %w", err)
	}

	// 解析 headers
	if headersJSON != "" && headersJSON != "null" {
		if err := json.Unmarshal([]byte(headersJSON), &record.Headers); err != nil {
			// 忽略解析错误，保持 Headers 为 nil
		}
	}

	// 解析可空字段
	if cooldownSeconds.Valid {
		cd := int(cooldownSeconds.Int64)
		record.CooldownSeconds = &cd
	}

	// 转换布尔值
	record.FailoverEnabled = failoverEnabled == 1
	record.SupportsCountTokens = supportsCountTokens == 1
	record.Enabled = enabled == 1
	availValue := availabilityEnabled == 1
	record.AvailabilityEnabled = &availValue

	// 解析时间
	record.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999-07:00", createdAt)
	record.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05.999999-07:00", updatedAt)

	return &record, nil
}

// scanEndpoints 扫描多个端点记录
func (s *SQLiteEndpointStore) scanEndpoints(ctx context.Context, query string) ([]*EndpointRecord, error) {
	return s.scanEndpointsWithArgs(ctx, query)
}

// scanEndpointsWithArgs 使用参数扫描多个端点记录
func (s *SQLiteEndpointStore) scanEndpointsWithArgs(ctx context.Context, query string, args ...interface{}) ([]*EndpointRecord, error) {
	rows, err := s.getQuerier().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询端点失败: %w", err)
	}
	defer rows.Close()

	var records []*EndpointRecord
	for rows.Next() {
		var record EndpointRecord
		var headersJSON string
		var cooldownSeconds sql.NullInt64
		var failoverEnabled, supportsCountTokens, enabled, availabilityEnabled int
		var createdAt, updatedAt string

		err := rows.Scan(
			&record.ID, &record.Channel, &record.Name, &record.URL,
			&record.Token, &record.ApiKey, &headersJSON,
			&record.Priority, &failoverEnabled, &cooldownSeconds, &record.TimeoutSeconds,
			&supportsCountTokens, &record.ModelRewriteRules,
			&record.CostMultiplier, &record.InputCostMultiplier, &record.OutputCostMultiplier,
			&record.CacheCreationCostMultiplier, &record.CacheCreationCostMultiplier1h, &record.CacheReadCostMultiplier,
			&enabled, &availabilityEnabled, &createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("扫描端点记录失败: %w", err)
		}

		// 解析 headers
		if headersJSON != "" && headersJSON != "null" {
			if err := json.Unmarshal([]byte(headersJSON), &record.Headers); err != nil {
				// 忽略解析错误
			}
		}

		// 解析可空字段
		if cooldownSeconds.Valid {
			cd := int(cooldownSeconds.Int64)
			record.CooldownSeconds = &cd
		}

		// 转换布尔值
		record.FailoverEnabled = failoverEnabled == 1
		record.SupportsCountTokens = supportsCountTokens == 1
		record.Enabled = enabled == 1
		availValue := availabilityEnabled == 1
		record.AvailabilityEnabled = &availValue

		// 解析时间
		record.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999-07:00", createdAt)
		record.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05.999999-07:00", updatedAt)

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历端点记录失败: %w", err)
	}

	return records, nil
}

// boolToInt 将布尔值转换为整数
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
