// Package store 提供数据存储层实现
// 模型定价存储 - v5.0.0 新增 (2025-12-06)
package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// ModelPricingRecord 表示数据库中的模型定价记录
type ModelPricingRecord struct {
	ID int64 `json:"id"`

	// 模型信息
	ModelName string `json:"model_name"` // 模型名称（如 claude-sonnet-4-20250514）

	// 定价信息 (USD per 1M tokens)
	InputPrice           float64 `json:"input_price"`             // 输入价格
	OutputPrice          float64 `json:"output_price"`            // 输出价格
	CacheCreationPrice5m float64 `json:"cache_creation_price_5m"` // 5分钟缓存创建价格 (input * 1.25)
	CacheCreationPrice1h float64 `json:"cache_creation_price_1h"` // 1小时缓存创建价格 (input * 2.0)
	CacheReadPrice       float64 `json:"cache_read_price"`        // 缓存读取价格 (input * 0.1)

	// 模型元信息
	DisplayName string `json:"display_name,omitempty"` // 显示名称
	Description string `json:"description,omitempty"`  // 模型描述
	IsDefault   bool   `json:"is_default"`             // 是否为默认定价

	// 审计字段
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ModelPricingStore 定义模型定价存储接口
type ModelPricingStore interface {
	// CRUD 操作
	Create(ctx context.Context, record *ModelPricingRecord) (*ModelPricingRecord, error)
	Get(ctx context.Context, modelName string) (*ModelPricingRecord, error)
	GetByID(ctx context.Context, id int64) (*ModelPricingRecord, error)
	List(ctx context.Context) ([]*ModelPricingRecord, error)
	Update(ctx context.Context, record *ModelPricingRecord) error
	Delete(ctx context.Context, modelName string) error

	// 批量操作
	BatchCreate(ctx context.Context, records []*ModelPricingRecord) error
	BatchUpsert(ctx context.Context, records []*ModelPricingRecord) error

	// 查询
	GetDefault(ctx context.Context) (*ModelPricingRecord, error)
	SetDefault(ctx context.Context, modelName string) error

	// 统计
	Count(ctx context.Context) (int, error)

	// 事务支持
	WithTx(tx *sql.Tx) ModelPricingStore
}

// SQLiteModelPricingStore 实现 ModelPricingStore 接口
type SQLiteModelPricingStore struct {
	db *sql.DB
	tx *sql.Tx // 事务上下文（可选）
	mu sync.RWMutex
}

// NewSQLiteModelPricingStore 创建新的 SQLite 模型定价存储
func NewSQLiteModelPricingStore(db *sql.DB) *SQLiteModelPricingStore {
	return &SQLiteModelPricingStore{db: db}
}

// getQuerier 返回用于执行查询的对象（事务或数据库连接）
func (s *SQLiteModelPricingStore) getQuerier() interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
} {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// Create 创建新的模型定价
func (s *SQLiteModelPricingStore) Create(ctx context.Context, record *ModelPricingRecord) (*ModelPricingRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 设置默认值
	if record.InputPrice == 0 {
		record.InputPrice = 3.0
	}
	if record.OutputPrice == 0 {
		record.OutputPrice = 15.0
	}
	if record.CacheCreationPrice5m == 0 {
		record.CacheCreationPrice5m = record.InputPrice * 1.25
	}
	if record.CacheCreationPrice1h == 0 {
		record.CacheCreationPrice1h = record.InputPrice * 2.0
	}
	if record.CacheReadPrice == 0 {
		record.CacheReadPrice = record.InputPrice * 0.1
	}

	query := `
		INSERT INTO model_pricing (
			model_name, input_price, output_price,
			cache_creation_price_5m, cache_creation_price_1h, cache_read_price,
			display_name, description, is_default
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := s.getQuerier().ExecContext(ctx, query,
		record.ModelName, record.InputPrice, record.OutputPrice,
		record.CacheCreationPrice5m, record.CacheCreationPrice1h, record.CacheReadPrice,
		record.DisplayName, record.Description, boolToInt(record.IsDefault),
	)
	if err != nil {
		return nil, fmt.Errorf("创建模型定价失败: %w", err)
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

// Get 根据模型名称获取定价
func (s *SQLiteModelPricingStore) Get(ctx context.Context, modelName string) (*ModelPricingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, model_name, input_price, output_price,
			cache_creation_price_5m, cache_creation_price_1h, cache_read_price,
			display_name, description, is_default,
			created_at, updated_at
		FROM model_pricing WHERE model_name = ?
	`

	return s.scanModelPricing(s.getQuerier().QueryRowContext(ctx, query, modelName))
}

// GetByID 根据 ID 获取定价
func (s *SQLiteModelPricingStore) GetByID(ctx context.Context, id int64) (*ModelPricingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, model_name, input_price, output_price,
			cache_creation_price_5m, cache_creation_price_1h, cache_read_price,
			display_name, description, is_default,
			created_at, updated_at
		FROM model_pricing WHERE id = ?
	`

	return s.scanModelPricing(s.getQuerier().QueryRowContext(ctx, query, id))
}

// List 获取所有模型定价
func (s *SQLiteModelPricingStore) List(ctx context.Context) ([]*ModelPricingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, model_name, input_price, output_price,
			cache_creation_price_5m, cache_creation_price_1h, cache_read_price,
			display_name, description, is_default,
			created_at, updated_at
		FROM model_pricing
		ORDER BY is_default DESC, model_name ASC
	`

	return s.scanModelPricings(ctx, query)
}

// Update 更新模型定价
func (s *SQLiteModelPricingStore) Update(ctx context.Context, record *ModelPricingRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		UPDATE model_pricing SET
			input_price = ?, output_price = ?,
			cache_creation_price_5m = ?, cache_creation_price_1h = ?, cache_read_price = ?,
			display_name = ?, description = ?, is_default = ?
		WHERE model_name = ?
	`

	result, err := s.getQuerier().ExecContext(ctx, query,
		record.InputPrice, record.OutputPrice,
		record.CacheCreationPrice5m, record.CacheCreationPrice1h, record.CacheReadPrice,
		record.DisplayName, record.Description, boolToInt(record.IsDefault),
		record.ModelName,
	)
	if err != nil {
		return fmt.Errorf("更新模型定价失败: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("模型定价不存在: %s", record.ModelName)
	}

	return nil
}

// Delete 删除模型定价
func (s *SQLiteModelPricingStore) Delete(ctx context.Context, modelName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM model_pricing WHERE model_name = ?`

	result, err := s.getQuerier().ExecContext(ctx, query, modelName)
	if err != nil {
		return fmt.Errorf("删除模型定价失败: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("模型定价不存在: %s", modelName)
	}

	return nil
}

// BatchCreate 批量创建模型定价
func (s *SQLiteModelPricingStore) BatchCreate(ctx context.Context, records []*ModelPricingRecord) error {
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
		INSERT INTO model_pricing (
			model_name, input_price, output_price,
			cache_creation_price_5m, cache_creation_price_1h, cache_read_price,
			display_name, description, is_default
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	for _, record := range records {
		// 设置默认值
		if record.InputPrice == 0 {
			record.InputPrice = 3.0
		}
		if record.OutputPrice == 0 {
			record.OutputPrice = 15.0
		}
		if record.CacheCreationPrice5m == 0 {
			record.CacheCreationPrice5m = record.InputPrice * 1.25
		}
		if record.CacheCreationPrice1h == 0 {
			record.CacheCreationPrice1h = record.InputPrice * 2.0
		}
		if record.CacheReadPrice == 0 {
			record.CacheReadPrice = record.InputPrice * 0.1
		}

		_, err = stmt.ExecContext(ctx,
			record.ModelName, record.InputPrice, record.OutputPrice,
			record.CacheCreationPrice5m, record.CacheCreationPrice1h, record.CacheReadPrice,
			record.DisplayName, record.Description, boolToInt(record.IsDefault),
		)
		if err != nil {
			return fmt.Errorf("插入模型定价 %s 失败: %w", record.ModelName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// BatchUpsert 批量创建或更新模型定价
func (s *SQLiteModelPricingStore) BatchUpsert(ctx context.Context, records []*ModelPricingRecord) error {
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
		INSERT INTO model_pricing (
			model_name, input_price, output_price,
			cache_creation_price_5m, cache_creation_price_1h, cache_read_price,
			display_name, description, is_default
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(model_name) DO UPDATE SET
			input_price = excluded.input_price,
			output_price = excluded.output_price,
			cache_creation_price_5m = excluded.cache_creation_price_5m,
			cache_creation_price_1h = excluded.cache_creation_price_1h,
			cache_read_price = excluded.cache_read_price,
			display_name = excluded.display_name,
			description = excluded.description,
			is_default = excluded.is_default
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	for _, record := range records {
		// 设置默认值
		if record.InputPrice == 0 {
			record.InputPrice = 3.0
		}
		if record.OutputPrice == 0 {
			record.OutputPrice = 15.0
		}
		if record.CacheCreationPrice5m == 0 {
			record.CacheCreationPrice5m = record.InputPrice * 1.25
		}
		if record.CacheCreationPrice1h == 0 {
			record.CacheCreationPrice1h = record.InputPrice * 2.0
		}
		if record.CacheReadPrice == 0 {
			record.CacheReadPrice = record.InputPrice * 0.1
		}

		_, err = stmt.ExecContext(ctx,
			record.ModelName, record.InputPrice, record.OutputPrice,
			record.CacheCreationPrice5m, record.CacheCreationPrice1h, record.CacheReadPrice,
			record.DisplayName, record.Description, boolToInt(record.IsDefault),
		)
		if err != nil {
			return fmt.Errorf("upsert 模型定价 %s 失败: %w", record.ModelName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// GetDefault 获取默认定价
func (s *SQLiteModelPricingStore) GetDefault(ctx context.Context) (*ModelPricingRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, model_name, input_price, output_price,
			cache_creation_price_5m, cache_creation_price_1h, cache_read_price,
			display_name, description, is_default,
			created_at, updated_at
		FROM model_pricing WHERE is_default = 1 LIMIT 1
	`

	return s.scanModelPricing(s.getQuerier().QueryRowContext(ctx, query))
}

// SetDefault 设置默认定价（取消其他默认）
func (s *SQLiteModelPricingStore) SetDefault(ctx context.Context, modelName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 使用事务
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback()

	// 先清除所有默认标记
	_, err = tx.ExecContext(ctx, "UPDATE model_pricing SET is_default = 0")
	if err != nil {
		return fmt.Errorf("清除默认标记失败: %w", err)
	}

	// 设置新的默认
	result, err := tx.ExecContext(ctx, "UPDATE model_pricing SET is_default = 1 WHERE model_name = ?", modelName)
	if err != nil {
		return fmt.Errorf("设置默认定价失败: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("模型定价不存在: %s", modelName)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// Count 获取模型定价总数
func (s *SQLiteModelPricingStore) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.getQuerier().QueryRowContext(ctx, "SELECT COUNT(*) FROM model_pricing").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("获取模型定价数量失败: %w", err)
	}

	return count, nil
}

// WithTx 返回使用事务的存储实例
func (s *SQLiteModelPricingStore) WithTx(tx *sql.Tx) ModelPricingStore {
	return &SQLiteModelPricingStore{
		db: s.db,
		tx: tx,
	}
}

// scanModelPricing 从单行扫描模型定价记录
func (s *SQLiteModelPricingStore) scanModelPricing(row *sql.Row) (*ModelPricingRecord, error) {
	var record ModelPricingRecord
	var displayName, description sql.NullString
	var isDefault int
	var createdAt, updatedAt string

	err := row.Scan(
		&record.ID, &record.ModelName,
		&record.InputPrice, &record.OutputPrice,
		&record.CacheCreationPrice5m, &record.CacheCreationPrice1h, &record.CacheReadPrice,
		&displayName, &description, &isDefault,
		&createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("扫描模型定价记录失败: %w", err)
	}

	// 解析可空字段
	if displayName.Valid {
		record.DisplayName = displayName.String
	}
	if description.Valid {
		record.Description = description.String
	}

	// 转换布尔值
	record.IsDefault = isDefault == 1

	// 解析时间
	record.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999-07:00", createdAt)
	record.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05.999999-07:00", updatedAt)

	return &record, nil
}

// scanModelPricings 扫描多个模型定价记录
func (s *SQLiteModelPricingStore) scanModelPricings(ctx context.Context, query string, args ...interface{}) ([]*ModelPricingRecord, error) {
	rows, err := s.getQuerier().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询模型定价失败: %w", err)
	}
	defer rows.Close()

	var records []*ModelPricingRecord
	for rows.Next() {
		var record ModelPricingRecord
		var displayName, description sql.NullString
		var isDefault int
		var createdAt, updatedAt string

		err := rows.Scan(
			&record.ID, &record.ModelName,
			&record.InputPrice, &record.OutputPrice,
			&record.CacheCreationPrice5m, &record.CacheCreationPrice1h, &record.CacheReadPrice,
			&displayName, &description, &isDefault,
			&createdAt, &updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("扫描模型定价记录失败: %w", err)
		}

		// 解析可空字段
		if displayName.Valid {
			record.DisplayName = displayName.String
		}
		if description.Valid {
			record.Description = description.String
		}

		// 转换布尔值
		record.IsDefault = isDefault == 1

		// 解析时间
		record.CreatedAt, _ = time.Parse("2006-01-02 15:04:05.999999-07:00", createdAt)
		record.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05.999999-07:00", updatedAt)

		records = append(records, &record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历模型定价记录失败: %w", err)
	}

	return records, nil
}
