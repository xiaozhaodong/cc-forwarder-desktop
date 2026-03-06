// Package store 提供账号池相关数据存储实现
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultAccountBaseURL = "https://api.openai.com"
	defaultAccountState   = "active"
	defaultAccountPrio    = 100
	defaultSourceSyncMode = "manual"
)

// SubscriptionSourceRecord 订阅源记录
type SubscriptionSourceRecord struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	URL        string     `json:"url"`
	Enabled    bool       `json:"enabled"`
	SyncMode   string     `json:"sync_mode"`
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	LastStatus string     `json:"last_status"`
	LastError  string     `json:"last_error"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// UpstreamAccountRecord 上游账号记录
type UpstreamAccountRecord struct {
	ID            int64      `json:"id"`
	SourceID      *int64     `json:"source_id,omitempty"`
	SourceName    string     `json:"source_name,omitempty"`
	ProviderType  string     `json:"provider_type"`
	AccountName   string     `json:"account_name"`
	CredentialRaw string     `json:"credential_raw"`
	BaseURL       string     `json:"base_url"`
	Priority      int        `json:"priority"`
	Enabled       bool       `json:"enabled"`
	State         string     `json:"state"`
	CooldownUntil *time.Time `json:"cooldown_until,omitempty"`
	FailCount     int        `json:"fail_count"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastError     string     `json:"last_error"`
	Fingerprint   string     `json:"fingerprint"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// SyncLogRecord 同步日志记录
type SyncLogRecord struct {
	ID            int64      `json:"id"`
	SourceID      int64      `json:"source_id"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Result        string     `json:"result"`
	AddedCount    int        `json:"added_count"`
	UpdatedCount  int        `json:"updated_count"`
	DisabledCount int        `json:"disabled_count"`
	ErrorSummary  string     `json:"error_summary"`
	CreatedAt     time.Time  `json:"created_at"`
}

// AccountPoolStore 账号池存储接口
type AccountPoolStore interface {
	CreateSource(ctx context.Context, record *SubscriptionSourceRecord) (*SubscriptionSourceRecord, error)
	UpdateSource(ctx context.Context, record *SubscriptionSourceRecord) error
	DeleteSource(ctx context.Context, id int64) error
	GetSource(ctx context.Context, id int64) (*SubscriptionSourceRecord, error)
	ListSources(ctx context.Context) ([]*SubscriptionSourceRecord, error)
	ToggleSource(ctx context.Context, id int64, enabled bool) error
	UpdateSourceSyncStatus(ctx context.Context, id int64, status, lastError string, syncAt time.Time) error

	CreateAccount(ctx context.Context, record *UpstreamAccountRecord) (*UpstreamAccountRecord, error)
	UpdateAccount(ctx context.Context, record *UpstreamAccountRecord) error
	DeleteAccount(ctx context.Context, id int64) error
	GetAccount(ctx context.Context, id int64) (*UpstreamAccountRecord, error)
	ListAccounts(ctx context.Context, includeDisabled bool) ([]*UpstreamAccountRecord, error)
	ListAccountsBySource(ctx context.Context, sourceID int64) ([]*UpstreamAccountRecord, error)
	ListSchedulableAccounts(ctx context.Context, now time.Time) ([]*UpstreamAccountRecord, error)
	FindAccountByFingerprint(ctx context.Context, fingerprint string) (*UpstreamAccountRecord, error)
	ToggleAccount(ctx context.Context, id int64, enabled bool) error
	MarkAccountSuccess(ctx context.Context, id int64, successAt time.Time) error
	MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error
	MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldownUntil time.Time) error
	DisableAccountsBySourceExcept(ctx context.Context, sourceID int64, keepFingerprints []string) (int, error)

	CreateSyncLog(ctx context.Context, record *SyncLogRecord) (*SyncLogRecord, error)
	FinishSyncLog(ctx context.Context, id int64, result string, added, updated, disabled int, errorSummary string, finishedAt time.Time) error
	ListRecentSyncLogs(ctx context.Context, sourceID int64, limit int) ([]*SyncLogRecord, error)
}

// SQLiteAccountPoolStore SQLite 账号池存储实现
type SQLiteAccountPoolStore struct {
	db *sql.DB
	tx *sql.Tx
	mu sync.RWMutex
}

// NewSQLiteAccountPoolStore 创建账号池存储
func NewSQLiteAccountPoolStore(db *sql.DB) *SQLiteAccountPoolStore {
	return &SQLiteAccountPoolStore{db: db}
}

func (s *SQLiteAccountPoolStore) getQuerier() interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
} {
	if s.tx != nil {
		return s.tx
	}
	return s.db
}

// CreateSource 创建订阅源
func (s *SQLiteAccountPoolStore) CreateSource(ctx context.Context, record *SubscriptionSourceRecord) (*SubscriptionSourceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(record.Name) == "" {
		return nil, fmt.Errorf("订阅源名称不能为空")
	}
	if strings.TrimSpace(record.URL) == "" {
		return nil, fmt.Errorf("订阅源 URL 不能为空")
	}
	if record.SyncMode == "" {
		record.SyncMode = defaultSourceSyncMode
	}

	query := `
		INSERT INTO subscription_sources (name, url, enabled, sync_mode, last_status, last_error)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	res, err := s.getQuerier().ExecContext(ctx, query,
		record.Name, record.URL, boolToInt(record.Enabled), record.SyncMode, record.LastStatus, record.LastError)
	if err != nil {
		return nil, fmt.Errorf("创建订阅源失败: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取订阅源 ID 失败: %w", err)
	}
	return s.getSourceByID(ctx, id)
}

// UpdateSource 更新订阅源
func (s *SQLiteAccountPoolStore) UpdateSource(ctx context.Context, record *SubscriptionSourceRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record.ID <= 0 {
		return fmt.Errorf("无效的订阅源 ID")
	}
	if strings.TrimSpace(record.Name) == "" {
		return fmt.Errorf("订阅源名称不能为空")
	}
	if strings.TrimSpace(record.URL) == "" {
		return fmt.Errorf("订阅源 URL 不能为空")
	}
	if record.SyncMode == "" {
		record.SyncMode = defaultSourceSyncMode
	}

	query := `
		UPDATE subscription_sources
		SET name = ?, url = ?, enabled = ?, sync_mode = ?
		WHERE id = ?
	`
	res, err := s.getQuerier().ExecContext(ctx, query,
		record.Name, record.URL, boolToInt(record.Enabled), record.SyncMode, record.ID)
	if err != nil {
		return fmt.Errorf("更新订阅源失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("订阅源不存在: %d", record.ID)
	}
	return nil
}

// DeleteSource 删除订阅源
func (s *SQLiteAccountPoolStore) DeleteSource(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.getQuerier().ExecContext(ctx, `DELETE FROM subscription_sources WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除订阅源失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("订阅源不存在: %d", id)
	}
	return nil
}

// GetSource 获取单个订阅源
func (s *SQLiteAccountPoolStore) GetSource(ctx context.Context, id int64) (*SubscriptionSourceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getSourceByID(ctx, id)
}

func (s *SQLiteAccountPoolStore) getSourceByID(ctx context.Context, id int64) (*SubscriptionSourceRecord, error) {
	query := `
		SELECT id, name, url, enabled, sync_mode, last_sync_at, last_status, last_error, created_at, updated_at
		FROM subscription_sources
		WHERE id = ?
	`
	return scanSourceRow(s.getQuerier().QueryRowContext(ctx, query, id))
}

// ListSources 列出所有订阅源
func (s *SQLiteAccountPoolStore) ListSources(ctx context.Context) ([]*SubscriptionSourceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, name, url, enabled, sync_mode, last_sync_at, last_status, last_error, created_at, updated_at
		FROM subscription_sources
		ORDER BY id DESC
	`
	rows, err := s.getQuerier().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("查询订阅源失败: %w", err)
	}
	defer rows.Close()

	var out []*SubscriptionSourceRecord
	for rows.Next() {
		rec, scanErr := scanSourceRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历订阅源失败: %w", err)
	}
	return out, nil
}

// ToggleSource 启停订阅源
func (s *SQLiteAccountPoolStore) ToggleSource(ctx context.Context, id int64, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.getQuerier().ExecContext(ctx,
		`UPDATE subscription_sources SET enabled = ? WHERE id = ?`, boolToInt(enabled), id)
	if err != nil {
		return fmt.Errorf("切换订阅源状态失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("订阅源不存在: %d", id)
	}
	return nil
}

// UpdateSourceSyncStatus 更新订阅源同步状态
func (s *SQLiteAccountPoolStore) UpdateSourceSyncStatus(ctx context.Context, id int64, status, lastError string, syncAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.getQuerier().ExecContext(ctx,
		`UPDATE subscription_sources SET last_sync_at = ?, last_status = ?, last_error = ? WHERE id = ?`,
		formatDBTime(syncAt), status, lastError, id)
	if err != nil {
		return fmt.Errorf("更新订阅源同步状态失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("订阅源不存在: %d", id)
	}
	return nil
}

// CreateAccount 创建账号
func (s *SQLiteAccountPoolStore) CreateAccount(ctx context.Context, record *UpstreamAccountRecord) (*UpstreamAccountRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizeAccountRecord(record)
	query := `
		INSERT INTO upstream_accounts (
			source_id, provider_type, account_name, credential_raw, base_url,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error, fingerprint
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := s.getQuerier().ExecContext(ctx, query,
		nullableInt64(record.SourceID), record.ProviderType, record.AccountName, record.CredentialRaw, record.BaseURL,
		record.Priority, boolToInt(record.Enabled), record.State, nullableTime(record.CooldownUntil), record.FailCount,
		nullableTime(record.LastSuccessAt), record.LastError, record.Fingerprint)
	if err != nil {
		return nil, fmt.Errorf("创建账号失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取账号 ID 失败: %w", err)
	}
	return s.getAccountByID(ctx, id)
}

// UpdateAccount 更新账号
func (s *SQLiteAccountPoolStore) UpdateAccount(ctx context.Context, record *UpstreamAccountRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record.ID <= 0 {
		return fmt.Errorf("无效的账号 ID")
	}
	normalizeAccountRecord(record)
	query := `
		UPDATE upstream_accounts
		SET source_id = ?, provider_type = ?, account_name = ?, credential_raw = ?, base_url = ?,
			priority = ?, enabled = ?, state = ?, cooldown_until = ?, fail_count = ?, last_success_at = ?, last_error = ?, fingerprint = ?
		WHERE id = ?
	`
	res, err := s.getQuerier().ExecContext(ctx, query,
		nullableInt64(record.SourceID), record.ProviderType, record.AccountName, record.CredentialRaw, record.BaseURL,
		record.Priority, boolToInt(record.Enabled), record.State, nullableTime(record.CooldownUntil), record.FailCount,
		nullableTime(record.LastSuccessAt), record.LastError, record.Fingerprint, record.ID)
	if err != nil {
		return fmt.Errorf("更新账号失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("账号不存在: %d", record.ID)
	}
	return nil
}

// DeleteAccount 删除账号
func (s *SQLiteAccountPoolStore) DeleteAccount(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.getQuerier().ExecContext(ctx, `DELETE FROM upstream_accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除账号失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("账号不存在: %d", id)
	}
	return nil
}

// GetAccount 获取单个账号
func (s *SQLiteAccountPoolStore) GetAccount(ctx context.Context, id int64) (*UpstreamAccountRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getAccountByID(ctx, id)
}

func (s *SQLiteAccountPoolStore) getAccountByID(ctx context.Context, id int64) (*UpstreamAccountRecord, error) {
	query := `
		SELECT a.id, a.source_id, COALESCE(s.name, ''), a.provider_type, a.account_name, a.credential_raw, a.base_url,
			a.priority, a.enabled, a.state, a.cooldown_until, a.fail_count, a.last_success_at, a.last_error, a.fingerprint, a.created_at, a.updated_at
		FROM upstream_accounts a
		LEFT JOIN subscription_sources s ON s.id = a.source_id
		WHERE a.id = ?
	`
	return scanAccountRow(s.getQuerier().QueryRowContext(ctx, query, id))
}

// ListAccounts 列出账号
func (s *SQLiteAccountPoolStore) ListAccounts(ctx context.Context, includeDisabled bool) ([]*UpstreamAccountRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT a.id, a.source_id, COALESCE(s.name, ''), a.provider_type, a.account_name, a.credential_raw, a.base_url,
			a.priority, a.enabled, a.state, a.cooldown_until, a.fail_count, a.last_success_at, a.last_error, a.fingerprint, a.created_at, a.updated_at
		FROM upstream_accounts a
		LEFT JOIN subscription_sources s ON s.id = a.source_id
	`
	args := make([]any, 0)
	if !includeDisabled {
		query += ` WHERE a.enabled = 1`
	}
	query += ` ORDER BY a.priority ASC, a.id ASC`

	rows, err := s.getQuerier().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询账号失败: %w", err)
	}
	defer rows.Close()

	var out []*UpstreamAccountRecord
	for rows.Next() {
		rec, scanErr := scanAccountRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历账号失败: %w", err)
	}
	return out, nil
}

// ListAccountsBySource 按订阅源查询账号
func (s *SQLiteAccountPoolStore) ListAccountsBySource(ctx context.Context, sourceID int64) ([]*UpstreamAccountRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT a.id, a.source_id, COALESCE(s.name, ''), a.provider_type, a.account_name, a.credential_raw, a.base_url,
			a.priority, a.enabled, a.state, a.cooldown_until, a.fail_count, a.last_success_at, a.last_error, a.fingerprint, a.created_at, a.updated_at
		FROM upstream_accounts a
		LEFT JOIN subscription_sources s ON s.id = a.source_id
		WHERE a.source_id = ?
		ORDER BY a.priority ASC, a.id ASC
	`
	rows, err := s.getQuerier().QueryContext(ctx, query, sourceID)
	if err != nil {
		return nil, fmt.Errorf("按订阅源查询账号失败: %w", err)
	}
	defer rows.Close()

	var out []*UpstreamAccountRecord
	for rows.Next() {
		rec, scanErr := scanAccountRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历账号失败: %w", err)
	}
	return out, nil
}

// ListSchedulableAccounts 查询可调度账号
func (s *SQLiteAccountPoolStore) ListSchedulableAccounts(ctx context.Context, now time.Time) ([]*UpstreamAccountRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT a.id, a.source_id, COALESCE(s.name, ''), a.provider_type, a.account_name, a.credential_raw, a.base_url,
			a.priority, a.enabled, a.state, a.cooldown_until, a.fail_count, a.last_success_at, a.last_error, a.fingerprint, a.created_at, a.updated_at
		FROM upstream_accounts a
		LEFT JOIN subscription_sources s ON s.id = a.source_id
		WHERE a.enabled = 1
			AND a.state != 'disabled_auth'
			AND (a.cooldown_until IS NULL OR a.cooldown_until <= ?)
			AND (a.source_id IS NULL OR COALESCE(s.enabled, 1) = 1)
		ORDER BY a.priority ASC, a.id ASC
	`
	rows, err := s.getQuerier().QueryContext(ctx, query, formatDBTime(now))
	if err != nil {
		return nil, fmt.Errorf("查询可调度账号失败: %w", err)
	}
	defer rows.Close()

	var out []*UpstreamAccountRecord
	for rows.Next() {
		rec, scanErr := scanAccountRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历可调度账号失败: %w", err)
	}
	return out, nil
}

// FindAccountByFingerprint 按指纹查询账号
func (s *SQLiteAccountPoolStore) FindAccountByFingerprint(ctx context.Context, fingerprint string) (*UpstreamAccountRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT a.id, a.source_id, COALESCE(s.name, ''), a.provider_type, a.account_name, a.credential_raw, a.base_url,
			a.priority, a.enabled, a.state, a.cooldown_until, a.fail_count, a.last_success_at, a.last_error, a.fingerprint, a.created_at, a.updated_at
		FROM upstream_accounts a
		LEFT JOIN subscription_sources s ON s.id = a.source_id
		WHERE a.fingerprint = ?
	`
	rec, err := scanAccountRow(s.getQuerier().QueryRowContext(ctx, query, fingerprint))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rec, nil
}

// ToggleAccount 启停账号
func (s *SQLiteAccountPoolStore) ToggleAccount(ctx context.Context, id int64, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		res sql.Result
		err error
	)
	if enabled {
		res, err = s.getQuerier().ExecContext(ctx,
			`UPDATE upstream_accounts SET enabled = 1, state = 'active', fail_count = 0, cooldown_until = NULL WHERE id = ?`,
			id)
	} else {
		res, err = s.getQuerier().ExecContext(ctx,
			`UPDATE upstream_accounts SET enabled = 0 WHERE id = ?`,
			id)
	}
	if err != nil {
		return fmt.Errorf("切换账号状态失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("获取影响行数失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("账号不存在: %d", id)
	}
	return nil
}

// MarkAccountSuccess 标记账号成功
func (s *SQLiteAccountPoolStore) MarkAccountSuccess(ctx context.Context, id int64, successAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.getQuerier().ExecContext(ctx,
		`UPDATE upstream_accounts SET fail_count = 0, state = 'active', cooldown_until = NULL, last_success_at = ?, last_error = '' WHERE id = ?`,
		formatDBTime(successAt), id)
	if err != nil {
		return fmt.Errorf("更新账号成功状态失败: %w", err)
	}
	return nil
}

// MarkAccountAuthFailed 标记账号鉴权失败
func (s *SQLiteAccountPoolStore) MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.getQuerier().ExecContext(ctx,
		`UPDATE upstream_accounts SET enabled = 0, state = 'disabled_auth', cooldown_until = NULL, last_error = ? WHERE id = ?`,
		reason, id)
	if err != nil {
		return fmt.Errorf("更新账号鉴权失败状态失败: %w", err)
	}
	return nil
}

// MarkAccountTransientFailure 标记账号瞬时失败（进入冷却）
func (s *SQLiteAccountPoolStore) MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldownUntil time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.getQuerier().ExecContext(ctx,
		`UPDATE upstream_accounts SET fail_count = fail_count + 1, state = 'cooldown', cooldown_until = ?, last_error = ? WHERE id = ?`,
		formatDBTime(cooldownUntil), reason, id)
	if err != nil {
		return fmt.Errorf("更新账号瞬时失败状态失败: %w", err)
	}
	return nil
}

// DisableAccountsBySourceExcept 禁用订阅源下未出现在保留列表中的账号
func (s *SQLiteAccountPoolStore) DisableAccountsBySourceExcept(ctx context.Context, sourceID int64, keepFingerprints []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var (
		res sql.Result
		err error
	)
	if len(keepFingerprints) == 0 {
		res, err = s.getQuerier().ExecContext(ctx,
			`UPDATE upstream_accounts SET enabled = 0, state = 'active', last_error = 'disabled_by_sync' WHERE source_id = ? AND enabled = 1`,
			sourceID)
	} else {
		holders := strings.TrimSuffix(strings.Repeat("?,", len(keepFingerprints)), ",")
		args := make([]any, 0, len(keepFingerprints)+1)
		args = append(args, sourceID)
		for _, fp := range keepFingerprints {
			args = append(args, fp)
		}
		query := fmt.Sprintf(
			`UPDATE upstream_accounts SET enabled = 0, state = 'active', last_error = 'disabled_by_sync' WHERE source_id = ? AND enabled = 1 AND fingerprint NOT IN (%s)`,
			holders,
		)
		res, err = s.getQuerier().ExecContext(ctx, query, args...)
	}
	if err != nil {
		return 0, fmt.Errorf("禁用缺失账号失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("获取影响行数失败: %w", err)
	}
	return int(affected), nil
}

// CreateSyncLog 创建同步日志
func (s *SQLiteAccountPoolStore) CreateSyncLog(ctx context.Context, record *SyncLogRecord) (*SyncLogRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record.SourceID <= 0 {
		return nil, fmt.Errorf("无效的订阅源 ID")
	}
	if record.StartedAt.IsZero() {
		record.StartedAt = time.Now()
	}
	query := `
		INSERT INTO sync_logs (source_id, started_at, result, added_count, updated_count, disabled_count, error_summary)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	res, err := s.getQuerier().ExecContext(ctx, query,
		record.SourceID, formatDBTime(record.StartedAt), record.Result, record.AddedCount, record.UpdatedCount, record.DisabledCount, record.ErrorSummary)
	if err != nil {
		return nil, fmt.Errorf("创建同步日志失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取同步日志 ID 失败: %w", err)
	}
	record.ID = id
	return record, nil
}

// FinishSyncLog 完成同步日志
func (s *SQLiteAccountPoolStore) FinishSyncLog(ctx context.Context, id int64, result string, added, updated, disabled int, errorSummary string, finishedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.getQuerier().ExecContext(ctx, `
		UPDATE sync_logs
		SET finished_at = ?, result = ?, added_count = ?, updated_count = ?, disabled_count = ?, error_summary = ?
		WHERE id = ?
	`, formatDBTime(finishedAt), result, added, updated, disabled, errorSummary, id)
	if err != nil {
		return fmt.Errorf("更新同步日志失败: %w", err)
	}
	return nil
}

// ListRecentSyncLogs 查询最近同步日志
func (s *SQLiteAccountPoolStore) ListRecentSyncLogs(ctx context.Context, sourceID int64, limit int) ([]*SyncLogRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, source_id, started_at, finished_at, result, added_count, updated_count, disabled_count, error_summary, created_at
		FROM sync_logs
		WHERE source_id = ?
		ORDER BY id DESC
		LIMIT ?
	`
	rows, err := s.getQuerier().QueryContext(ctx, query, sourceID, limit)
	if err != nil {
		return nil, fmt.Errorf("查询同步日志失败: %w", err)
	}
	defer rows.Close()

	var out []*SyncLogRecord
	for rows.Next() {
		rec, scanErr := scanSyncLogRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历同步日志失败: %w", err)
	}
	return out, nil
}

// GenerateAccountFingerprint 生成账号指纹（用于去重）
func GenerateAccountFingerprint(providerType, credentialRaw, baseURL string) string {
	text := strings.ToLower(strings.TrimSpace(providerType)) + "\n" +
		strings.TrimSpace(credentialRaw) + "\n" +
		normalizeBaseURL(baseURL)
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func normalizeAccountRecord(record *UpstreamAccountRecord) {
	if record.ProviderType == "" {
		record.ProviderType = "api_key"
	}
	if strings.TrimSpace(record.AccountName) == "" {
		record.AccountName = "unnamed-account"
	}
	if strings.TrimSpace(record.BaseURL) == "" {
		record.BaseURL = defaultAccountBaseURL
	}
	record.BaseURL = normalizeBaseURL(record.BaseURL)
	if record.Priority == 0 {
		record.Priority = defaultAccountPrio
	}
	if record.State == "" {
		record.State = defaultAccountState
	}
	if record.Fingerprint == "" {
		record.Fingerprint = GenerateAccountFingerprint(record.ProviderType, record.CredentialRaw, record.BaseURL)
	}
}

func normalizeBaseURL(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return defaultAccountBaseURL
	}
	return trimmed
}

func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTime(v *time.Time) any {
	if v == nil || v.IsZero() {
		return nil
	}
	return formatDBTime(*v)
}

func formatDBTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05.999999-07:00")
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSourceRow(scanner rowScanner) (*SubscriptionSourceRecord, error) {
	var (
		rec           SubscriptionSourceRecord
		enabled       int
		lastSyncAtStr sql.NullString
		createdAtStr  string
		updatedAtStr  string
	)
	if err := scanner.Scan(
		&rec.ID, &rec.Name, &rec.URL, &enabled, &rec.SyncMode, &lastSyncAtStr, &rec.LastStatus, &rec.LastError, &createdAtStr, &updatedAtStr,
	); err != nil {
		return nil, err
	}
	rec.Enabled = enabled == 1
	rec.LastSyncAt = parseNullableTime(lastSyncAtStr)
	rec.CreatedAt = parseDBTime(createdAtStr)
	rec.UpdatedAt = parseDBTime(updatedAtStr)
	return &rec, nil
}

func scanAccountRow(scanner rowScanner) (*UpstreamAccountRecord, error) {
	var (
		rec              UpstreamAccountRecord
		sourceID         sql.NullInt64
		enabled          int
		cooldownUntilStr sql.NullString
		lastSuccessAtStr sql.NullString
		createdAtStr     string
		updatedAtStr     string
	)
	if err := scanner.Scan(
		&rec.ID, &sourceID, &rec.SourceName, &rec.ProviderType, &rec.AccountName, &rec.CredentialRaw, &rec.BaseURL,
		&rec.Priority, &enabled, &rec.State, &cooldownUntilStr, &rec.FailCount, &lastSuccessAtStr, &rec.LastError, &rec.Fingerprint, &createdAtStr, &updatedAtStr,
	); err != nil {
		return nil, err
	}
	if sourceID.Valid {
		srcID := sourceID.Int64
		rec.SourceID = &srcID
	}
	rec.Enabled = enabled == 1
	rec.CooldownUntil = parseNullableTime(cooldownUntilStr)
	rec.LastSuccessAt = parseNullableTime(lastSuccessAtStr)
	rec.CreatedAt = parseDBTime(createdAtStr)
	rec.UpdatedAt = parseDBTime(updatedAtStr)
	return &rec, nil
}

func scanSyncLogRow(scanner rowScanner) (*SyncLogRecord, error) {
	var (
		rec          SyncLogRecord
		finishedStr  sql.NullString
		startedAtStr string
		createdAtStr string
	)
	if err := scanner.Scan(
		&rec.ID, &rec.SourceID, &startedAtStr, &finishedStr, &rec.Result, &rec.AddedCount, &rec.UpdatedCount, &rec.DisabledCount, &rec.ErrorSummary, &createdAtStr,
	); err != nil {
		return nil, err
	}
	rec.StartedAt = parseDBTime(startedAtStr)
	rec.FinishedAt = parseNullableTime(finishedStr)
	rec.CreatedAt = parseDBTime(createdAtStr)
	return &rec, nil
}

func parseNullableTime(v sql.NullString) *time.Time {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	t := parseDBTime(v.String)
	return &t
}

func parseDBTime(text string) time.Time {
	candidates := []string{
		"2006-01-02 15:04:05.999999-07:00",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	}
	for _, layout := range candidates {
		if t, err := time.Parse(layout, text); err == nil {
			return t
		}
	}
	return time.Time{}
}
