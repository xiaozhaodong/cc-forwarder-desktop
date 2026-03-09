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

var accountDBTimeZone = time.FixedZone("UTC+8", 8*60*60)

// UpstreamAccountRecord 上游账号记录
type UpstreamAccountRecord struct {
	ID                            int64      `json:"id"`
	ProviderType                  string     `json:"provider_type"`
	AccountName                   string     `json:"account_name"`
	CredentialRaw                 string     `json:"credential_raw"`
	BaseURL                       string     `json:"base_url"`
	CostMultiplier                float64    `json:"cost_multiplier"`
	InputCostMultiplier           float64    `json:"input_cost_multiplier"`
	OutputCostMultiplier          float64    `json:"output_cost_multiplier"`
	CacheCreationCostMultiplier   float64    `json:"cache_creation_cost_multiplier"`
	CacheCreationCostMultiplier1h float64    `json:"cache_creation_cost_multiplier_1h"`
	CacheReadCostMultiplier       float64    `json:"cache_read_cost_multiplier"`
	Priority                      int        `json:"priority"`
	Enabled                       bool       `json:"enabled"`
	State                         string     `json:"state"`
	CooldownUntil                 *time.Time `json:"cooldown_until,omitempty"`
	FailCount                     int        `json:"fail_count"`
	LastSuccessAt                 *time.Time `json:"last_success_at,omitempty"`
	LastError                     string     `json:"last_error"`
	PlanType                      string     `json:"plan_type"`
	ChatGPTAccountID              string     `json:"chatgpt_account_id"`
	ChatGPTUserID                 string     `json:"chatgpt_user_id"`
	OrganizationID                string     `json:"organization_id"`
	Quota5HUsedPercent            *float64   `json:"quota_5h_used_percent,omitempty"`
	Quota5HResetAt                *time.Time `json:"quota_5h_reset_at,omitempty"`
	QuotaWeeklyUsedPercent        *float64   `json:"quota_weekly_used_percent,omitempty"`
	QuotaWeeklyResetAt            *time.Time `json:"quota_weekly_reset_at,omitempty"`
	QuotaStatus                   string     `json:"quota_status"`
	QuotaRefreshedAt              *time.Time `json:"quota_refreshed_at,omitempty"`
	Fingerprint                   string     `json:"fingerprint"`
	CreatedAt                     time.Time  `json:"created_at"`
	UpdatedAt                     time.Time  `json:"updated_at"`
}

// AccountPoolStore 账号池存储接口
type AccountPoolStore interface {
	CreateAccount(ctx context.Context, record *UpstreamAccountRecord) (*UpstreamAccountRecord, error)
	UpdateAccount(ctx context.Context, record *UpstreamAccountRecord) error
	UpdateAccountPriorities(ctx context.Context, updates map[int64]int) error
	DeleteAccount(ctx context.Context, id int64) error
	GetAccount(ctx context.Context, id int64) (*UpstreamAccountRecord, error)
	ListAccounts(ctx context.Context, includeDisabled bool) ([]*UpstreamAccountRecord, error)
	ListSchedulableAccounts(ctx context.Context, now time.Time) ([]*UpstreamAccountRecord, error)
	FindAccountByFingerprint(ctx context.Context, fingerprint string) (*UpstreamAccountRecord, error)
	ToggleAccount(ctx context.Context, id int64, enabled bool) error
	MarkAccountSuccess(ctx context.Context, id int64, successAt time.Time) error
	MarkAccountSuccessIfNoNewerFailure(ctx context.Context, id int64, successAt, attemptStartedAt time.Time) (bool, error)
	MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error
	MarkAccountAuthFailedWithProfile(ctx context.Context, record *UpstreamAccountRecord, reason string) error
	MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldownUntil time.Time) error
	UpdateAccountProfile(ctx context.Context, record *UpstreamAccountRecord) error
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
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error,
			plan_type, chatgpt_account_id, chatgpt_user_id, organization_id,
			quota_5h_used_percent, quota_5h_reset_at, quota_weekly_used_percent, quota_weekly_reset_at,
			quota_status, quota_refreshed_at, fingerprint
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	res, err := s.getQuerier().ExecContext(ctx, query,
		record.ProviderType, record.AccountName, record.CredentialRaw, record.BaseURL,
		record.CostMultiplier, record.InputCostMultiplier, record.OutputCostMultiplier,
		record.CacheCreationCostMultiplier, record.CacheCreationCostMultiplier1h, record.CacheReadCostMultiplier,
		record.Priority, boolToInt(record.Enabled), record.State, nullableTime(record.CooldownUntil), record.FailCount,
		nullableTime(record.LastSuccessAt), record.LastError,
		record.PlanType, record.ChatGPTAccountID, record.ChatGPTUserID, record.OrganizationID,
		nullableFloat(record.Quota5HUsedPercent), nullableTime(record.Quota5HResetAt),
		nullableFloat(record.QuotaWeeklyUsedPercent), nullableTime(record.QuotaWeeklyResetAt),
		record.QuotaStatus, nullableTime(record.QuotaRefreshedAt), record.Fingerprint)
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
			cost_multiplier = ?, input_cost_multiplier = ?, output_cost_multiplier = ?,
			cache_creation_cost_multiplier = ?, cache_creation_cost_multiplier_1h = ?, cache_read_cost_multiplier = ?,
			priority = ?, enabled = ?, state = ?, cooldown_until = ?, fail_count = ?, last_success_at = ?, last_error = ?,
			plan_type = ?, chatgpt_account_id = ?, chatgpt_user_id = ?, organization_id = ?,
			quota_5h_used_percent = ?, quota_5h_reset_at = ?, quota_weekly_used_percent = ?, quota_weekly_reset_at = ?,
			quota_status = ?, quota_refreshed_at = ?, fingerprint = ?
		WHERE id = ?
	`
	res, err := s.getQuerier().ExecContext(ctx, query,
		record.ProviderType, record.AccountName, record.CredentialRaw, record.BaseURL,
		record.CostMultiplier, record.InputCostMultiplier, record.OutputCostMultiplier,
		record.CacheCreationCostMultiplier, record.CacheCreationCostMultiplier1h, record.CacheReadCostMultiplier,
		record.Priority, boolToInt(record.Enabled), record.State, nullableTime(record.CooldownUntil), record.FailCount,
		nullableTime(record.LastSuccessAt), record.LastError,
		record.PlanType, record.ChatGPTAccountID, record.ChatGPTUserID, record.OrganizationID,
		nullableFloat(record.Quota5HUsedPercent), nullableTime(record.Quota5HResetAt),
		nullableFloat(record.QuotaWeeklyUsedPercent), nullableTime(record.QuotaWeeklyResetAt),
		record.QuotaStatus, nullableTime(record.QuotaRefreshedAt), record.Fingerprint, record.ID)
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

// UpdateAccountPriorities 批量更新账号优先级。
func (s *SQLiteAccountPoolStore) UpdateAccountPriorities(ctx context.Context, updates map[int64]int) error {
	if len(updates) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始账号优先级事务失败: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `UPDATE upstream_accounts SET priority = ? WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("准备账号优先级更新语句失败: %w", err)
	}
	defer stmt.Close()

	for id, priority := range updates {
		res, execErr := stmt.ExecContext(ctx, priority, id)
		if execErr != nil {
			return fmt.Errorf("更新账号优先级失败: %w", execErr)
		}
		affected, rowsErr := res.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("获取账号优先级更新影响行数失败: %w", rowsErr)
		}
		if affected == 0 {
			return fmt.Errorf("账号不存在: %d", id)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交账号优先级事务失败: %w", err)
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
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error,
			plan_type, chatgpt_account_id, chatgpt_user_id, organization_id,
			quota_5h_used_percent, quota_5h_reset_at, quota_weekly_used_percent, quota_weekly_reset_at,
			quota_status, quota_refreshed_at, fingerprint, created_at, updated_at
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
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error,
			plan_type, chatgpt_account_id, chatgpt_user_id, organization_id,
			quota_5h_used_percent, quota_5h_reset_at, quota_weekly_used_percent, quota_weekly_reset_at,
			quota_status, quota_refreshed_at, fingerprint, created_at, updated_at
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
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error,
			plan_type, chatgpt_account_id, chatgpt_user_id, organization_id,
			quota_5h_used_percent, quota_5h_reset_at, quota_weekly_used_percent, quota_weekly_reset_at,
			quota_status, quota_refreshed_at, fingerprint, created_at, updated_at
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
			cost_multiplier, input_cost_multiplier, output_cost_multiplier,
			cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
			priority, enabled, state, cooldown_until, fail_count, last_success_at, last_error,
			plan_type, chatgpt_account_id, chatgpt_user_id, organization_id,
			quota_5h_used_percent, quota_5h_reset_at, quota_weekly_used_percent, quota_weekly_reset_at,
			quota_status, quota_refreshed_at, fingerprint, created_at, updated_at
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
		`UPDATE upstream_accounts
		 SET fail_count = 0, state = 'active', cooldown_until = NULL, last_success_at = ?, last_error = '', updated_at = ?
		 WHERE id = ?`,
		formatDBTime(successAt), currentDBTime(), id)
	if err != nil {
		return fmt.Errorf("更新账号成功状态失败: %w", err)
	}
	return nil
}

func (s *SQLiteAccountPoolStore) MarkAccountSuccessIfNoNewerFailure(ctx context.Context, id int64, successAt, attemptStartedAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.getQuerier().ExecContext(ctx,
		`UPDATE upstream_accounts
		 SET fail_count = 0, state = 'active', cooldown_until = NULL, last_success_at = ?, last_error = '', updated_at = ?
		 WHERE id = ?
		   AND NOT (updated_at > ? AND state IN ('cooldown', 'disabled_auth'))`,
		formatDBTime(successAt), currentDBTime(), id, formatDBTime(attemptStartedAt))
	if err != nil {
		return false, fmt.Errorf("条件更新账号成功状态失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("获取影响行数失败: %w", err)
	}
	return affected > 0, nil
}

// MarkAccountAuthFailed 标记账号鉴权失败
func (s *SQLiteAccountPoolStore) MarkAccountAuthFailed(ctx context.Context, id int64, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.getQuerier().ExecContext(ctx,
		`UPDATE upstream_accounts
		 SET enabled = 0, state = 'disabled_auth', cooldown_until = NULL, last_error = ?, updated_at = ?
		 WHERE id = ?`,
		reason, currentDBTime(), id)
	if err != nil {
		return fmt.Errorf("更新账号鉴权失败状态失败: %w", err)
	}
	return nil
}

func (s *SQLiteAccountPoolStore) MarkAccountAuthFailedWithProfile(ctx context.Context, record *UpstreamAccountRecord, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record == nil || record.ID <= 0 {
		return fmt.Errorf("无效的账号记录")
	}
	record.PlanType = strings.TrimSpace(record.PlanType)
	record.ChatGPTAccountID = strings.TrimSpace(record.ChatGPTAccountID)
	record.ChatGPTUserID = strings.TrimSpace(record.ChatGPTUserID)
	record.OrganizationID = strings.TrimSpace(record.OrganizationID)
	record.QuotaStatus = strings.TrimSpace(record.QuotaStatus)
	record.Fingerprint = GenerateAccountFingerprint(record.ProviderType, record.CredentialRaw, record.BaseURL)

	res, err := s.getQuerier().ExecContext(ctx, `
		UPDATE upstream_accounts
		SET enabled = 0, state = 'disabled_auth', cooldown_until = NULL, last_error = ?, updated_at = ?,
			credential_raw = ?, plan_type = ?, chatgpt_account_id = ?, chatgpt_user_id = ?, organization_id = ?,
			quota_5h_used_percent = ?, quota_5h_reset_at = ?, quota_weekly_used_percent = ?, quota_weekly_reset_at = ?,
			quota_status = ?, quota_refreshed_at = ?, fingerprint = ?
		WHERE id = ?
	`,
		reason, currentDBTime(),
		record.CredentialRaw, record.PlanType, record.ChatGPTAccountID, record.ChatGPTUserID, record.OrganizationID,
		nullableFloat(record.Quota5HUsedPercent), nullableTime(record.Quota5HResetAt),
		nullableFloat(record.QuotaWeeklyUsedPercent), nullableTime(record.QuotaWeeklyResetAt),
		record.QuotaStatus, nullableTime(record.QuotaRefreshedAt), record.Fingerprint, record.ID,
	)
	if err != nil {
		return fmt.Errorf("更新账号鉴权失败与画像状态失败: %w", err)
	}
	affected, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		return fmt.Errorf("获取账号鉴权失败画像影响行数失败: %w", rowsErr)
	}
	if affected == 0 {
		return fmt.Errorf("账号不存在: %d", record.ID)
	}
	return nil
}

// MarkAccountTransientFailure 标记账号瞬时失败（进入冷却）
func (s *SQLiteAccountPoolStore) MarkAccountTransientFailure(ctx context.Context, id int64, reason string, cooldownUntil time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.getQuerier().ExecContext(ctx,
		`UPDATE upstream_accounts
		 SET fail_count = fail_count + 1, state = 'cooldown', cooldown_until = ?, last_error = ?, updated_at = ?
		 WHERE id = ?`,
		formatDBTime(cooldownUntil), reason, currentDBTime(), id)
	if err != nil {
		return fmt.Errorf("更新账号瞬时失败状态失败: %w", err)
	}
	return nil
}

// UpdateAccountProfile 更新账号画像与 quota 字段。
func (s *SQLiteAccountPoolStore) UpdateAccountProfile(ctx context.Context, record *UpstreamAccountRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record == nil || record.ID <= 0 {
		return fmt.Errorf("无效的账号记录")
	}
	record.PlanType = strings.TrimSpace(record.PlanType)
	record.ChatGPTAccountID = strings.TrimSpace(record.ChatGPTAccountID)
	record.ChatGPTUserID = strings.TrimSpace(record.ChatGPTUserID)
	record.OrganizationID = strings.TrimSpace(record.OrganizationID)
	record.QuotaStatus = strings.TrimSpace(record.QuotaStatus)
	record.Fingerprint = GenerateAccountFingerprint(record.ProviderType, record.CredentialRaw, record.BaseURL)

	res, err := s.getQuerier().ExecContext(ctx, `
		UPDATE upstream_accounts
		SET credential_raw = ?, plan_type = ?, chatgpt_account_id = ?, chatgpt_user_id = ?, organization_id = ?,
			quota_5h_used_percent = ?, quota_5h_reset_at = ?, quota_weekly_used_percent = ?, quota_weekly_reset_at = ?,
			quota_status = ?, quota_refreshed_at = ?, fingerprint = ?
		WHERE id = ?
	`,
		record.CredentialRaw, record.PlanType, record.ChatGPTAccountID, record.ChatGPTUserID, record.OrganizationID,
		nullableFloat(record.Quota5HUsedPercent), nullableTime(record.Quota5HResetAt),
		nullableFloat(record.QuotaWeeklyUsedPercent), nullableTime(record.QuotaWeeklyResetAt),
		record.QuotaStatus, nullableTime(record.QuotaRefreshedAt), record.Fingerprint, record.ID,
	)
	if err != nil {
		return fmt.Errorf("更新账号画像失败: %w", err)
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
	applyAccountCostMultiplierPolicy(record)
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

func normalizeMultiplierValue(value float64) float64 {
	if value <= 0 {
		return 1.0
	}
	return value
}

func applyAccountCostMultiplierPolicy(record *UpstreamAccountRecord) {
	if record == nil {
		return
	}

	if strings.TrimSpace(strings.ToLower(record.ProviderType)) != "api_key" {
		record.CostMultiplier = 1.0
		record.InputCostMultiplier = 1.0
		record.OutputCostMultiplier = 1.0
		record.CacheCreationCostMultiplier = 1.0
		record.CacheCreationCostMultiplier1h = 1.0
		record.CacheReadCostMultiplier = 1.0
		return
	}

	record.CostMultiplier = normalizeMultiplierValue(record.CostMultiplier)
	record.InputCostMultiplier = normalizeMultiplierValue(record.InputCostMultiplier)
	record.OutputCostMultiplier = normalizeMultiplierValue(record.OutputCostMultiplier)
	record.CacheCreationCostMultiplier = normalizeMultiplierValue(record.CacheCreationCostMultiplier)
	record.CacheCreationCostMultiplier1h = normalizeMultiplierValue(record.CacheCreationCostMultiplier1h)
	record.CacheReadCostMultiplier = normalizeMultiplierValue(record.CacheReadCostMultiplier)
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

func nullableFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func formatDBTime(t time.Time) string {
	return t.In(accountDBTimeZone).Format("2006-01-02 15:04:05.999999-07:00")
}

func currentDBTime() string {
	return formatDBTime(time.Now())
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAccountRow(scanner rowScanner) (*UpstreamAccountRecord, error) {
	var (
		rec                       UpstreamAccountRecord
		enabled                   int
		cooldownUntilStr          sql.NullString
		lastSuccessAtStr          sql.NullString
		quota5HUsedPercent        sql.NullFloat64
		quota5HResetAtStr         sql.NullString
		quotaWeeklyUsedPercent    sql.NullFloat64
		quotaWeeklyResetAtStr     sql.NullString
		quotaRefreshedAtStr       sql.NullString
		costMultiplier            sql.NullFloat64
		inputCostMultiplier       sql.NullFloat64
		outputCostMultiplier      sql.NullFloat64
		cacheCreationMultiplier   sql.NullFloat64
		cacheCreationMultiplier1h sql.NullFloat64
		cacheReadMultiplier       sql.NullFloat64
		createdAtStr              string
		updatedAtStr              string
	)
	if err := scanner.Scan(
		&rec.ID, &rec.ProviderType, &rec.AccountName, &rec.CredentialRaw, &rec.BaseURL,
		&costMultiplier, &inputCostMultiplier, &outputCostMultiplier,
		&cacheCreationMultiplier, &cacheCreationMultiplier1h, &cacheReadMultiplier,
		&rec.Priority, &enabled, &rec.State, &cooldownUntilStr, &rec.FailCount, &lastSuccessAtStr, &rec.LastError,
		&rec.PlanType, &rec.ChatGPTAccountID, &rec.ChatGPTUserID, &rec.OrganizationID,
		&quota5HUsedPercent, &quota5HResetAtStr, &quotaWeeklyUsedPercent, &quotaWeeklyResetAtStr,
		&rec.QuotaStatus, &quotaRefreshedAtStr, &rec.Fingerprint, &createdAtStr, &updatedAtStr,
	); err != nil {
		return nil, err
	}
	rec.Enabled = enabled == 1
	rec.CooldownUntil = parseNullableTime(cooldownUntilStr)
	rec.CostMultiplier = parseMultiplierFloat(costMultiplier)
	rec.InputCostMultiplier = parseMultiplierFloat(inputCostMultiplier)
	rec.OutputCostMultiplier = parseMultiplierFloat(outputCostMultiplier)
	rec.CacheCreationCostMultiplier = parseMultiplierFloat(cacheCreationMultiplier)
	rec.CacheCreationCostMultiplier1h = parseMultiplierFloat(cacheCreationMultiplier1h)
	rec.CacheReadCostMultiplier = parseMultiplierFloat(cacheReadMultiplier)
	rec.LastSuccessAt = parseNullableTime(lastSuccessAtStr)
	rec.Quota5HUsedPercent = parseNullableFloat(quota5HUsedPercent)
	rec.Quota5HResetAt = parseNullableTime(quota5HResetAtStr)
	rec.QuotaWeeklyUsedPercent = parseNullableFloat(quotaWeeklyUsedPercent)
	rec.QuotaWeeklyResetAt = parseNullableTime(quotaWeeklyResetAtStr)
	rec.QuotaRefreshedAt = parseNullableTime(quotaRefreshedAtStr)
	rec.CreatedAt = parseDBTime(createdAtStr)
	rec.UpdatedAt = parseDBTime(updatedAtStr)
	return &rec, nil
}

func parseMultiplierFloat(v sql.NullFloat64) float64 {
	if !v.Valid || v.Float64 <= 0 {
		return 1.0
	}
	return v.Float64
}

func parseNullableTime(v sql.NullString) *time.Time {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	t := parseDBTime(v.String)
	return &t
}

func parseNullableFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	value := v.Float64
	return &value
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
