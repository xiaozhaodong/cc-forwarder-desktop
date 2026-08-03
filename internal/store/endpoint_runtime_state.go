// Package store - v8 端点运行态存储（收敛方案 §7.2）
// 只保存达到阈值后的持久化 cooldown（global auth/quota 与 messages），
// 软失败事件列表不落库；count_tokens scope 预留，第一版不写入。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// 端点运行态 scope 常量
const (
	EndpointRuntimeScopeGlobal      = "global"
	EndpointRuntimeScopeMessages    = "messages"
	EndpointRuntimeScopeCountTokens = "count_tokens"
)

// EndpointRuntimeStateRecord 端点运行态记录
type EndpointRuntimeStateRecord struct {
	EndpointID     int64      `json:"endpoint_id"`
	Scope          string     `json:"scope"`
	State          string     `json:"state"`
	CooldownUntil  *time.Time `json:"cooldown_until,omitempty"`
	CooldownReason string     `json:"cooldown_reason"`
	Revision       int64      `json:"revision"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// EndpointRuntimeStateStore 端点运行态存储接口
type EndpointRuntimeStateStore interface {
	// Upsert 写入运行态；仅当 revision 更高时覆盖（§14.4：旧任务丢弃）
	Upsert(ctx context.Context, record *EndpointRuntimeStateRecord) error
	// ListActiveCooldowns 列出仍在生效的 cooldown（cooldown_until > now）
	ListActiveCooldowns(ctx context.Context, now time.Time) ([]*EndpointRuntimeStateRecord, error)
	// Clear 清除指定端点 scope 的运行态
	Clear(ctx context.Context, endpointID int64, scope string) error
	// DeleteByEndpoint 删除端点全部运行态。当前连接已启用外键（DSN _foreign_keys=1），
	// 端点删除时由 ON DELETE CASCADE 级联清理；本方法仅作为级联不可用场景的显式兜底。
	DeleteByEndpoint(ctx context.Context, endpointID int64) error
	// MaxRevision 返回全表最大 revision（空表为 0），用于启动播种 revision 发号器
	MaxRevision(ctx context.Context) (int64, error)
}

// SQLiteEndpointRuntimeStateStore SQLite 实现
type SQLiteEndpointRuntimeStateStore struct {
	db *sql.DB
}

// NewSQLiteEndpointRuntimeStateStore 创建运行态存储
func NewSQLiteEndpointRuntimeStateStore(db *sql.DB) *SQLiteEndpointRuntimeStateStore {
	return &SQLiteEndpointRuntimeStateStore{db: db}
}

// Upsert 写入运行态；已有更高或相同 revision 时静默丢弃旧任务
func (s *SQLiteEndpointRuntimeStateStore) Upsert(ctx context.Context, record *EndpointRuntimeStateRecord) error {
	if record == nil {
		return fmt.Errorf("endpoint runtime state record is nil")
	}
	var cooldownUntil interface{}
	if record.CooldownUntil != nil {
		cooldownUntil = record.CooldownUntil.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO endpoint_runtime_states (endpoint_id, scope, state, cooldown_until, cooldown_reason, revision, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(endpoint_id, scope) DO UPDATE SET
			state = excluded.state,
			cooldown_until = excluded.cooldown_until,
			cooldown_reason = excluded.cooldown_reason,
			revision = excluded.revision,
			updated_at = CURRENT_TIMESTAMP
		WHERE excluded.revision > endpoint_runtime_states.revision
	`, record.EndpointID, record.Scope, record.State, cooldownUntil, record.CooldownReason, record.Revision)
	if err != nil {
		return fmt.Errorf("写入端点运行态失败: %w", err)
	}
	return nil
}

// ListActiveCooldowns 列出仍在生效的 cooldown
func (s *SQLiteEndpointRuntimeStateStore) ListActiveCooldowns(ctx context.Context, now time.Time) ([]*EndpointRuntimeStateRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT endpoint_id, scope, state, cooldown_until, cooldown_reason, revision
		FROM endpoint_runtime_states
		WHERE cooldown_until IS NOT NULL AND cooldown_until > ?
	`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, fmt.Errorf("查询端点运行态失败: %w", err)
	}
	defer rows.Close()

	var records []*EndpointRuntimeStateRecord
	for rows.Next() {
		var record EndpointRuntimeStateRecord
		var cooldownUntil sql.NullString
		if err := rows.Scan(&record.EndpointID, &record.Scope, &record.State,
			&cooldownUntil, &record.CooldownReason, &record.Revision); err != nil {
			return nil, fmt.Errorf("扫描端点运行态失败: %w", err)
		}
		if cooldownUntil.Valid {
			if parsed, err := time.Parse(time.RFC3339Nano, cooldownUntil.String); err == nil {
				record.CooldownUntil = &parsed
			}
		}
		records = append(records, &record)
	}
	return records, rows.Err()
}

// Clear 清除指定端点 scope 的运行态
func (s *SQLiteEndpointRuntimeStateStore) Clear(ctx context.Context, endpointID int64, scope string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM endpoint_runtime_states WHERE endpoint_id = ? AND scope = ?`, endpointID, scope)
	if err != nil {
		return fmt.Errorf("清除端点运行态失败: %w", err)
	}
	return nil
}

// DeleteByEndpoint 删除端点全部运行态
func (s *SQLiteEndpointRuntimeStateStore) DeleteByEndpoint(ctx context.Context, endpointID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM endpoint_runtime_states WHERE endpoint_id = ?`, endpointID)
	if err != nil {
		return fmt.Errorf("删除端点运行态失败: %w", err)
	}
	return nil
}

// MaxRevision 返回全表最大 revision（空表为 0）
func (s *SQLiteEndpointRuntimeStateStore) MaxRevision(ctx context.Context) (int64, error) {
	var max sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(revision) FROM endpoint_runtime_states`).Scan(&max); err != nil {
		return 0, fmt.Errorf("查询端点运行态最大 revision 失败: %w", err)
	}
	return max.Int64, nil
}
