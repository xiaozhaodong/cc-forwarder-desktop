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
)

// UpstreamAccountRecord 上游账号记录
type UpstreamAccountRecord struct {
	ID            int64      `json:"id"`
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

// AccountPoolStore 账号池存储接口
type AccountPoolStore interface {
	CreateAccount(ctx context.Context, record *UpstreamAccountRecord) (*UpstreamAccountRecord, error)
	UpdateAccount(ctx context.Context, record *UpstreamAccountRecord) error
	DeleteAccount(ctx context.Context, id int64) error
	GetAccount(ctx context.Context, id int64) (*UpstreamAccountRecord, error)
	ListAccounts(ctx context.Context, includeDisabled bool) ([]*UpstreamAccountRecord, error)
	ListSchedulableAccounts(ctx context.Context, now time.Time) ([]*UpstreamAccountRecord, error)
	FindAccountByFingerprint(ctx context.Context, fingerprint string) (*UpstreamAccountRecord, error)
	ToggleAccount(ctx context.Context, id int64, enabled bool) error
	MarkAccountSuccess(ctx context.Context, id int64, successAt time.Time) error
	MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error
	MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldownUntil time.Time) error
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

// CreateAccount 创建账号
func (s *SQLiteAccountPoolStore) CreateAccount(ctx context.Context, record *UpstreamAccountRecord) (*UpstreamAccountRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizeAccountRecord(record)
	query := `
		INSERT INTO upstream_accounts (
			provider_type, account_name, credential_raw, base_url,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error, fingerprint
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := s.getQuerier().ExecContext(ctx, query,
		record.ProviderType, record.AccountName, record.CredentialRaw, record.BaseURL,
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
		SET provider_type = ?, account_name = ?, credential_raw = ?, base_url = ?,
			priority = ?, enabled = ?, state = ?, cooldown_until = ?, fail_count = ?, last_success_at = ?, last_error = ?, fingerprint = ?
		WHERE id = ?
	`
	res, err := s.getQuerier().ExecContext(ctx, query,
		record.ProviderType, record.AccountName, record.CredentialRaw, record.BaseURL,
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
		SELECT id, provider_type, account_name, credential_raw, base_url,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error, fingerprint, created_at, updated_at
		FROM upstream_accounts
		WHERE id = ?
	`
	return scanAccountRow(s.getQuerier().QueryRowContext(ctx, query, id))
}

// ListAccounts 列出账号
func (s *SQLiteAccountPoolStore) ListAccounts(ctx context.Context, includeDisabled bool) ([]*UpstreamAccountRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, provider_type, account_name, credential_raw, base_url,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error, fingerprint, created_at, updated_at
		FROM upstream_accounts
	`
	if !includeDisabled {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY priority ASC, id ASC`

	rows, err := s.getQuerier().QueryContext(ctx, query)
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

// ListSchedulableAccounts 查询可调度账号
func (s *SQLiteAccountPoolStore) ListSchedulableAccounts(ctx context.Context, now time.Time) ([]*UpstreamAccountRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, provider_type, account_name, credential_raw, base_url,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error, fingerprint, created_at, updated_at
		FROM upstream_accounts
		WHERE enabled = 1
			AND state != 'disabled_auth'
			AND (cooldown_until IS NULL OR cooldown_until <= ?)
		ORDER BY priority ASC, id ASC
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
		SELECT id, provider_type, account_name, credential_raw, base_url,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error, fingerprint, created_at, updated_at
		FROM upstream_accounts
		WHERE fingerprint = ?
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

func scanAccountRow(scanner rowScanner) (*UpstreamAccountRecord, error) {
	var (
		rec              UpstreamAccountRecord
		enabled          int
		cooldownUntilStr sql.NullString
		lastSuccessAtStr sql.NullString
		createdAtStr     string
		updatedAtStr     string
	)
	if err := scanner.Scan(
		&rec.ID, &rec.ProviderType, &rec.AccountName, &rec.CredentialRaw, &rec.BaseURL,
		&rec.Priority, &enabled, &rec.State, &cooldownUntilStr, &rec.FailCount, &lastSuccessAtStr, &rec.LastError, &rec.Fingerprint, &createdAtStr, &updatedAtStr,
	); err != nil {
		return nil, err
	}
	rec.Enabled = enabled == 1
	rec.CooldownUntil = parseNullableTime(cooldownUntilStr)
	rec.LastSuccessAt = parseNullableTime(lastSuccessAtStr)
	rec.CreatedAt = parseDBTime(createdAtStr)
	rec.UpdatedAt = parseDBTime(updatedAtStr)
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
