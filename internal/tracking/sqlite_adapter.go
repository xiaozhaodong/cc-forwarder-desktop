package tracking

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var sqliteSchemaFS embed.FS

// SQLiteAdapter SQLite数据库适配器实现（保持原有逻辑）
type SQLiteAdapter struct {
	config   DatabaseConfig
	db       *sql.DB
	logger   *slog.Logger
	location *time.Location // 配置的时区
}

// NewSQLiteAdapter 创建SQLite适配器实例
func NewSQLiteAdapter(config DatabaseConfig) (*SQLiteAdapter, error) {
	// 设置默认配置
	setDefaultConfig(&config)

	// 解析时区配置
	timezone := strings.TrimSpace(config.Timezone)
	if timezone == "" {
		timezone = "Asia/Shanghai" // 默认时区
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		// 如果时区解析失败，记录错误但不终止，使用系统本地时区
		location = time.Local
		slog.Warn("SQLite时区解析失败，使用系统本地时区",
			"configured_timezone", timezone,
			"error", err,
			"fallback_timezone", location.String())
	} else {
		slog.Info("SQLite时区配置成功", "timezone", timezone)
	}

	adapter := &SQLiteAdapter{
		config:   config,
		logger:   slog.Default(),
		location: location,
	}

	return adapter, nil
}

// Open 建立SQLite数据库连接
func (s *SQLiteAdapter) Open() error {
	dbPath := s.config.DatabasePath
	if dbPath == "" {
		// 使用跨平台用户目录作为默认路径
		// Windows: %APPDATA%\AI-Switchboard\data\usage.db
		// macOS: ~/Library/Application Support/AI-Switchboard/data/usage.db
		// Linux: ~/.local/share/ai-switchboard/data/usage.db
		dbPath = filepath.Join(getSQLiteAppDataDir(), "data", "usage.db")
		s.logger.Info("使用默认数据库路径", "path", dbPath)
	}

	s.logger.Info("正在连接SQLite数据库", "path", dbPath)

	// 确保数据库目录存在
	if dbPath != ":memory:" {
		dbDir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	// 构建SQLite连接字符串
	dsn := dbPath + "?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=10000&_foreign_keys=1&_busy_timeout=60000"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open SQLite database: %w", err)
	}

	// 设置连接池参数（SQLite建议少量连接）
	db.SetMaxOpenConns(1) // SQLite写操作需要单一连接
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping SQLite database: %w", err)
	}

	s.db = db

	// 诊断时区设置
	s.diagnoseTimezoneSettings()

	s.logger.Info("✅ SQLite数据库连接成功")

	return nil
}

// Close 关闭数据库连接
func (s *SQLiteAdapter) Close() error {
	if s.db != nil {
		s.logger.Info("正在关闭SQLite数据库连接")
		return s.db.Close()
	}
	return nil
}

// Ping 测试数据库连接
func (s *SQLiteAdapter) Ping(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("database not connected")
	}
	return s.db.PingContext(ctx)
}

// GetDB 获取数据库连接
func (s *SQLiteAdapter) GetDB() *sql.DB {
	return s.db
}

// GetReadDB 获取读数据库连接
func (s *SQLiteAdapter) GetReadDB() *sql.DB {
	return s.db
}

// GetWriteDB 获取写数据库连接
func (s *SQLiteAdapter) GetWriteDB() *sql.DB {
	return s.db
}

// BeginTx 开始事务
func (s *SQLiteAdapter) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	return s.db.BeginTx(ctx, opts)
}

// InitSchema 初始化SQLite数据库Schema
func (s *SQLiteAdapter) InitSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.logger.Info("正在初始化SQLite数据库Schema")

	// 读取并执行SQLite schema
	schema, err := sqliteSchemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema.sql: %w", err)
	}

	// SQLite可以直接执行整个schema
	if _, err := s.db.ExecContext(ctx, string(schema)); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	// v5.0.1+: 执行迁移添加新字段
	if err := s.migrateSchema(ctx); err != nil {
		return fmt.Errorf("failed to migrate schema: %w", err)
	}

	s.logger.Info("✅ SQLite数据库Schema初始化完成")
	return nil
}

// migrateSchema 执行数据库迁移（v5.0.1+: 添加 5m/1h 缓存字段）
func (s *SQLiteAdapter) migrateSchema(ctx context.Context) error {
	migrations := []struct {
		table       string
		checkColumn string
		alterSQL    string
		description string
	}{
		{
			table:       "request_logs",
			checkColumn: "first_token_ms",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN first_token_ms INTEGER",
			description: "上游首响耗时字段",
		},
		{
			table:       "request_logs",
			checkColumn: "completion_ms",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN completion_ms INTEGER",
			description: "流式首响后完成耗时字段",
		},
		{
			table:       "request_logs",
			checkColumn: "cache_creation_5m_tokens",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN cache_creation_5m_tokens INTEGER DEFAULT 0",
			description: "5分钟缓存创建tokens字段",
		},
		{
			table:       "request_logs",
			checkColumn: "cache_creation_1h_tokens",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN cache_creation_1h_tokens INTEGER DEFAULT 0",
			description: "1小时缓存创建tokens字段",
		},
		{
			table:       "request_logs",
			checkColumn: "cache_creation_5m_cost_usd",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN cache_creation_5m_cost_usd REAL DEFAULT 0",
			description: "5分钟缓存创建成本字段",
		},
		{
			table:       "request_logs",
			checkColumn: "cache_creation_1h_cost_usd",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN cache_creation_1h_cost_usd REAL DEFAULT 0",
			description: "1小时缓存创建成本字段",
		},
		{
			table:       "request_logs",
			checkColumn: "upstream_type",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN upstream_type TEXT DEFAULT 'endpoint'",
			description: "上游来源类型字段",
		},
		{
			table:       "request_logs",
			checkColumn: "upstream_source_name",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN upstream_source_name TEXT DEFAULT ''",
			description: "上游来源名称字段",
		},
		{
			table:       "request_logs",
			checkColumn: "upstream_name",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN upstream_name TEXT DEFAULT ''",
			description: "上游名称字段",
		},
		{
			table:       "request_logs",
			checkColumn: "upstream_id",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN upstream_id INTEGER",
			description: "上游ID字段",
		},
		{
			table:       "request_logs",
			checkColumn: "route_mode",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN route_mode TEXT DEFAULT 'auto'",
			description: "Claude路由模式字段",
		},
		{
			table:       "request_logs",
			checkColumn: "requested_endpoint",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN requested_endpoint TEXT DEFAULT ''",
			description: "Claude手动请求端点字段",
		},
		{
			table:       "request_logs",
			checkColumn: "effective_endpoint",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN effective_endpoint TEXT DEFAULT ''",
			description: "Claude实际路由端点字段",
		},
		{
			table:       "request_logs",
			checkColumn: "fallback_reason",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN fallback_reason TEXT DEFAULT ''",
			description: "Claude路由fallback原因字段",
		},
		{
			table:       "request_logs",
			checkColumn: "route_decision_at",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN route_decision_at DATETIME",
			description: "Claude路由决策时间字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "group_key",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN group_key TEXT DEFAULT ''",
			description: "账号显式组别字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "model_rewrite_rules",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN model_rewrite_rules TEXT DEFAULT ''",
			description: "账号模型兼容改写规则字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "cost_multiplier",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN cost_multiplier REAL DEFAULT 1.0",
			description: "账号总成本倍率字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "input_cost_multiplier",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN input_cost_multiplier REAL DEFAULT 1.0",
			description: "账号输入成本倍率字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "output_cost_multiplier",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN output_cost_multiplier REAL DEFAULT 1.0",
			description: "账号输出成本倍率字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "cache_creation_cost_multiplier",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN cache_creation_cost_multiplier REAL DEFAULT 1.0",
			description: "账号5分钟缓存创建倍率字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "cache_creation_cost_multiplier_1h",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN cache_creation_cost_multiplier_1h REAL DEFAULT 1.0",
			description: "账号1小时缓存创建倍率字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "cache_read_cost_multiplier",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN cache_read_cost_multiplier REAL DEFAULT 1.0",
			description: "账号缓存读取倍率字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "plan_type",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN plan_type TEXT DEFAULT ''",
			description: "账号类型画像字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "chatgpt_account_id",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN chatgpt_account_id TEXT DEFAULT ''",
			description: "ChatGPT 账号ID字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "chatgpt_user_id",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN chatgpt_user_id TEXT DEFAULT ''",
			description: "ChatGPT 用户ID字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "organization_id",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN organization_id TEXT DEFAULT ''",
			description: "组织ID字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "quota_5h_used_percent",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN quota_5h_used_percent REAL",
			description: "5小时窗口已用百分比字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "quota_5h_reset_at",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN quota_5h_reset_at DATETIME",
			description: "5小时窗口重置时间字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "quota_weekly_used_percent",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN quota_weekly_used_percent REAL",
			description: "周窗口已用百分比字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "quota_weekly_reset_at",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN quota_weekly_reset_at DATETIME",
			description: "周窗口重置时间字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "quota_status",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN quota_status TEXT DEFAULT ''",
			description: "quota 状态字段",
		},
		{
			table:       "upstream_accounts",
			checkColumn: "quota_refreshed_at",
			alterSQL:    "ALTER TABLE upstream_accounts ADD COLUMN quota_refreshed_at DATETIME",
			description: "quota 最近刷新时间字段",
		},
	}

	for _, m := range migrations {
		exists, err := s.columnExists(ctx, m.table, m.checkColumn)
		if err != nil {
			return fmt.Errorf("failed to check column %s: %w", m.checkColumn, err)
		}

		if !exists {
			s.logger.Info(fmt.Sprintf("🔧 [数据库迁移] 添加 %s", m.description))
			if _, err := s.db.ExecContext(ctx, m.alterSQL); err != nil {
				return fmt.Errorf("failed to add column %s: %w", m.checkColumn, err)
			}
			s.logger.Info(fmt.Sprintf("✅ [数据库迁移] %s 添加成功", m.description))
		}
	}

	// 索引迁移（幂等）
	indexSQLs := []string{
		"CREATE INDEX IF NOT EXISTS idx_request_logs_upstream_type ON request_logs(upstream_type)",
		"CREATE INDEX IF NOT EXISTS idx_request_logs_upstream_name ON request_logs(upstream_name)",
		"CREATE INDEX IF NOT EXISTS idx_upstream_accounts_group_key ON upstream_accounts(group_key)",
	}
	for _, sqlStmt := range indexSQLs {
		if _, err := s.db.ExecContext(ctx, sqlStmt); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	if err := s.backfillCompletionMs(ctx); err != nil {
		return err
	}
	if err := s.ensurePrivacyExactSecretsSchema(ctx); err != nil {
		return err
	}
	if err := s.migrateDeprecatedAccountPoolSchema(ctx); err != nil {
		return err
	}
	if err := s.migrateModelRewriteRulesToExact(ctx); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteAdapter) backfillCompletionMs(ctx context.Context) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE request_logs
		SET completion_ms = CASE
			WHEN duration_ms - first_token_ms < 0 THEN 0
			ELSE duration_ms - first_token_ms
		END
		WHERE completion_ms IS NULL
			AND duration_ms IS NOT NULL
			AND first_token_ms IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to backfill completion_ms: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err == nil && rowsAffected > 0 {
		s.logger.Info(fmt.Sprintf("✅ [数据库迁移] completion_ms 历史数据回填完成: %d 条", rowsAffected))
	}
	return nil
}

func (s *SQLiteAdapter) ensurePrivacyExactSecretsSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS privacy_exact_secrets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			name TEXT NOT NULL,
			secret_value TEXT NOT NULL,
			value_hash TEXT NOT NULL,
			placeholder TEXT NOT NULL DEFAULT '[敏感值]',
			category TEXT NOT NULL DEFAULT 'custom',
			source_type TEXT NOT NULL DEFAULT 'manual',
			source_ref TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
			updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_privacy_exact_secrets_value_hash
			ON privacy_exact_secrets(value_hash)`,
		`CREATE TRIGGER IF NOT EXISTS update_privacy_exact_secrets_timestamp
			AFTER UPDATE ON privacy_exact_secrets
			FOR EACH ROW
			WHEN NEW.updated_at = OLD.updated_at
		BEGIN
			UPDATE privacy_exact_secrets
			SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'
			WHERE id = NEW.id;
		END`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to ensure privacy exact secrets schema: %w", err)
		}
	}
	return nil
}

func (s *SQLiteAdapter) migrateModelRewriteRulesToExact(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, model_rewrite_rules
		FROM upstream_accounts
		WHERE TRIM(COALESCE(model_rewrite_rules, '')) != ''
	`)
	if err != nil {
		return fmt.Errorf("failed to read model rewrite rules: %w", err)
	}
	defer rows.Close()

	updates := make(map[int64]string)
	for rows.Next() {
		var (
			id    int64
			rules string
		)
		if err := rows.Scan(&id, &rules); err != nil {
			return fmt.Errorf("failed to scan model rewrite rules: %w", err)
		}
		if normalized, changed := normalizeModelRewriteRulesToExact(rules); changed {
			updates[id] = normalized
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate model rewrite rules: %w", err)
	}
	for id, rules := range updates {
		if _, err := s.db.ExecContext(ctx, `UPDATE upstream_accounts SET model_rewrite_rules = ? WHERE id = ?`, rules, id); err != nil {
			return fmt.Errorf("failed to migrate model rewrite rules to exact matching: %w", err)
		}
	}
	return nil
}

func normalizeModelRewriteRulesToExact(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw, false
	}

	var rules []map[string]any
	if err := json.Unmarshal([]byte(raw), &rules); err == nil {
		if !rewriteRuleMatchesToExact(rules) {
			return raw, false
		}
		encoded, err := json.Marshal(rules)
		if err != nil {
			return raw, false
		}
		return string(encoded), true
	}

	var single map[string]any
	if err := json.Unmarshal([]byte(raw), &single); err != nil {
		return raw, false
	}
	if !rewriteRuleMatchesToExact([]map[string]any{single}) {
		return raw, false
	}
	encoded, err := json.Marshal(single)
	if err != nil {
		return raw, false
	}
	return string(encoded), true
}

func rewriteRuleMatchesToExact(rules []map[string]any) bool {
	changed := false
	for _, rule := range rules {
		match, ok := rule["match"].(string)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(match), "prefix") {
			rule["match"] = "exact"
			changed = true
		}
	}
	return changed
}

func (s *SQLiteAdapter) migrateDeprecatedAccountPoolSchema(ctx context.Context) (retErr error) {
	hasLegacySourceID, err := s.columnExists(ctx, "upstream_accounts", "source_id")
	if err != nil {
		return fmt.Errorf("failed to inspect upstream_accounts schema: %w", err)
	}
	hasLegacyAccountLogs, err := s.tableExists(ctx, "account_request_logs")
	if err != nil {
		return fmt.Errorf("failed to inspect account_request_logs table: %w", err)
	}
	hasLegacySyncLogs, err := s.tableExists(ctx, "sync_logs")
	if err != nil {
		return fmt.Errorf("failed to inspect sync_logs table: %w", err)
	}
	hasLegacySources, err := s.tableExists(ctx, "subscription_sources")
	if err != nil {
		return fmt.Errorf("failed to inspect subscription_sources table: %w", err)
	}
	hasMirrorInsert, err := s.triggerExists(ctx, "mirror_account_logs_after_insert")
	if err != nil {
		return fmt.Errorf("failed to inspect mirror insert trigger: %w", err)
	}
	hasMirrorUpdate, err := s.triggerExists(ctx, "mirror_account_logs_after_update")
	if err != nil {
		return fmt.Errorf("failed to inspect mirror update trigger: %w", err)
	}
	hasMirrorDelete, err := s.triggerExists(ctx, "mirror_account_logs_after_delete")
	if err != nil {
		return fmt.Errorf("failed to inspect mirror delete trigger: %w", err)
	}

	legacyDetected := hasLegacySourceID || hasLegacyAccountLogs || hasLegacySyncLogs || hasLegacySources ||
		hasMirrorInsert || hasMirrorUpdate || hasMirrorDelete
	if !legacyDetected {
		return nil
	}

	foreignKeysDisabled := false
	if hasLegacySourceID {
		if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys=OFF"); err != nil {
			return fmt.Errorf("failed to disable foreign keys before upstream_accounts rebuild: %w", err)
		}
		foreignKeysDisabled = true
		defer func() {
			if !foreignKeysDisabled {
				return
			}
			if _, err := s.db.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil && retErr == nil {
				retErr = fmt.Errorf("failed to re-enable foreign keys after upstream_accounts rebuild: %w", err)
			}
		}()

		s.logger.Info("🔧 [数据库迁移] 重建 upstream_accounts，移除 source_id")
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin upstream_accounts rebuild transaction: %w", err)
		}
		defer tx.Rollback()

		rebuildSQLs := []string{
			`CREATE TABLE IF NOT EXISTS upstream_accounts_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				provider_type TEXT NOT NULL DEFAULT 'api_key',
				account_name TEXT NOT NULL,
				credential_raw TEXT NOT NULL,
				base_url TEXT NOT NULL DEFAULT 'https://api.openai.com',
				model_rewrite_rules TEXT DEFAULT '',
				cost_multiplier REAL DEFAULT 1.0,
				input_cost_multiplier REAL DEFAULT 1.0,
				output_cost_multiplier REAL DEFAULT 1.0,
				cache_creation_cost_multiplier REAL DEFAULT 1.0,
				cache_creation_cost_multiplier_1h REAL DEFAULT 1.0,
				cache_read_cost_multiplier REAL DEFAULT 1.0,
				group_key TEXT DEFAULT '',
				priority INTEGER DEFAULT 100,
				enabled INTEGER DEFAULT 1,
				state TEXT DEFAULT 'active',
				cooldown_until DATETIME,
				fail_count INTEGER DEFAULT 0,
				last_success_at DATETIME,
				last_error TEXT DEFAULT '',
				plan_type TEXT DEFAULT '',
				chatgpt_account_id TEXT DEFAULT '',
				chatgpt_user_id TEXT DEFAULT '',
				organization_id TEXT DEFAULT '',
				quota_5h_used_percent REAL,
				quota_5h_reset_at DATETIME,
				quota_weekly_used_percent REAL,
				quota_weekly_reset_at DATETIME,
				quota_status TEXT DEFAULT '',
				quota_refreshed_at DATETIME,
				fingerprint TEXT UNIQUE NOT NULL,
				created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
				updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
			)`,
			`INSERT INTO upstream_accounts_new (
				id, provider_type, account_name, credential_raw, base_url, model_rewrite_rules,
				cost_multiplier, input_cost_multiplier, output_cost_multiplier,
				cache_creation_cost_multiplier, cache_creation_cost_multiplier_1h, cache_read_cost_multiplier,
				group_key, priority, enabled, state, cooldown_until, fail_count,
				last_success_at, last_error, plan_type, chatgpt_account_id, chatgpt_user_id, organization_id,
				quota_5h_used_percent, quota_5h_reset_at, quota_weekly_used_percent, quota_weekly_reset_at,
				quota_status, quota_refreshed_at, fingerprint, created_at, updated_at
			)
			SELECT
				id, provider_type, account_name, credential_raw, base_url,
				'' AS model_rewrite_rules,
				1.0 AS cost_multiplier,
				1.0 AS input_cost_multiplier,
				1.0 AS output_cost_multiplier,
				1.0 AS cache_creation_cost_multiplier,
				1.0 AS cache_creation_cost_multiplier_1h,
				1.0 AS cache_read_cost_multiplier,
				'' AS group_key,
				priority, enabled, state, cooldown_until, fail_count,
				last_success_at, last_error,
				'' AS plan_type,
				'' AS chatgpt_account_id,
				'' AS chatgpt_user_id,
				'' AS organization_id,
				NULL AS quota_5h_used_percent,
				NULL AS quota_5h_reset_at,
				NULL AS quota_weekly_used_percent,
				NULL AS quota_weekly_reset_at,
				'' AS quota_status,
				NULL AS quota_refreshed_at,
				fingerprint, created_at, updated_at
			FROM upstream_accounts`,
			"DROP TABLE upstream_accounts",
			"ALTER TABLE upstream_accounts_new RENAME TO upstream_accounts",
			"CREATE INDEX IF NOT EXISTS idx_upstream_accounts_priority ON upstream_accounts(priority)",
			"CREATE INDEX IF NOT EXISTS idx_upstream_accounts_group_key ON upstream_accounts(group_key)",
			"CREATE INDEX IF NOT EXISTS idx_upstream_accounts_enabled ON upstream_accounts(enabled)",
			"CREATE INDEX IF NOT EXISTS idx_upstream_accounts_state ON upstream_accounts(state)",
			"CREATE INDEX IF NOT EXISTS idx_upstream_accounts_cooldown_until ON upstream_accounts(cooldown_until)",
			`CREATE TRIGGER IF NOT EXISTS update_upstream_accounts_timestamp
				AFTER UPDATE ON upstream_accounts
				FOR EACH ROW
				WHEN NEW.updated_at = OLD.updated_at
			BEGIN
				UPDATE upstream_accounts SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
			END`,
		}
		for _, stmt := range rebuildSQLs {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("failed to rebuild upstream_accounts: %w", err)
			}
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit upstream_accounts rebuild: %w", err)
		}
		s.logger.Info("✅ [数据库迁移] upstream_accounts 已移除 source_id")
	}

	cleanupSQLs := []string{
		"DROP TRIGGER IF EXISTS mirror_account_logs_after_insert",
		"DROP TRIGGER IF EXISTS mirror_account_logs_after_update",
		"DROP TRIGGER IF EXISTS mirror_account_logs_after_delete",
		"DROP TABLE IF EXISTS account_request_logs",
		"DROP TABLE IF EXISTS sync_logs",
		"DROP TABLE IF EXISTS subscription_sources",
	}
	for _, stmt := range cleanupSQLs {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to cleanup deprecated account pool schema: %w", err)
		}
	}

	return nil
}

// columnExists 检查表中是否存在指定列
func (s *SQLiteAdapter) columnExists(ctx context.Context, table, column string) (bool, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", table)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var dfltValue interface{}
		var pk int

		if err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}

		if name == column {
			return true, nil
		}
	}

	return false, nil
}

func (s *SQLiteAdapter) tableExists(ctx context.Context, table string) (bool, error) {
	return s.sqliteObjectExists(ctx, "table", table)
}

func (s *SQLiteAdapter) triggerExists(ctx context.Context, trigger string) (bool, error) {
	return s.sqliteObjectExists(ctx, "trigger", trigger)
}

func (s *SQLiteAdapter) sqliteObjectExists(ctx context.Context, objectType, name string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?",
		objectType, name,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// BuildInsertOrReplaceQuery 构建插入或更新查询（SQLite语法）
// 使用 INSERT ... ON CONFLICT DO UPDATE 来避免数据丢失
func (s *SQLiteAdapter) BuildInsertOrReplaceQuery(table string, columns []string, values []string) string {
	columnsStr := strings.Join(columns, ", ")
	valuesStr := strings.Join(values, ", ")

	// 构建INSERT部分
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, columnsStr, valuesStr)

	// 根据表结构推断冲突键
	containsColumn := func(name string) bool {
		for _, col := range columns {
			if col == name {
				return true
			}
		}
		return false
	}

	conflictColumns := make([]string, 0)
	switch table {
	case "request_logs":
		if containsColumn("request_id") {
			conflictColumns = append(conflictColumns, "request_id")
		}
	case "usage_summary":
		if containsColumn("date") && containsColumn("model_name") &&
			containsColumn("endpoint_name") && containsColumn("group_name") {
			conflictColumns = append(conflictColumns, "date", "model_name", "endpoint_name", "group_name")
		}
	default:
		if containsColumn("request_id") {
			conflictColumns = append(conflictColumns, "request_id")
		}
	}

	// 构建ON CONFLICT DO UPDATE部分，对start_time字段进行特殊处理
	// 对于request_logs表，主键冲突时更新提供的字段（除了request_id主键）
	var updatePairs []string
	for _, col := range columns {
		skipInUpdate := false
		for _, conflictCol := range conflictColumns {
			if col == conflictCol {
				skipInUpdate = true
				break
			}
		}
		if !skipInUpdate { // 跳过冲突键字段
			if col == "start_time" {
				// 对start_time使用COALESCE，只在原值为NULL时才更新
				updatePairs = append(updatePairs, fmt.Sprintf("%s = COALESCE(request_logs.%s, EXCLUDED.%s)", col, col, col))
			} else {
				updatePairs = append(updatePairs, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
			}
		}
	}

	if len(updatePairs) > 0 && len(conflictColumns) > 0 {
		query += " ON CONFLICT(" + strings.Join(conflictColumns, ", ") + ") DO UPDATE SET " + strings.Join(updatePairs, ", ")
	} else {
		// 如果没有可更新字段或没有冲突键，则使用IGNORE避免重复插入
		query = fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES (%s)", table, columnsStr, valuesStr)
	}

	return query
}

// BuildDateTimeNow 返回当前时间函数（支持微秒精度）
// SQLite没有时区支持，我们在Go层面生成正确时区的时间字符串
func (s *SQLiteAdapter) BuildDateTimeNow() string {
	// 获取当前配置时区的时间
	now := time.Now().In(s.location)

	// 格式化为SQLite兼容的datetime格式（微秒精度）
	return fmt.Sprintf("'%s'", now.Format("2006-01-02 15:04:05.000000"))
}

// BuildLimitOffset 构建分页查询
func (s *SQLiteAdapter) BuildLimitOffset(limit, offset int) string {
	if limit <= 0 {
		return ""
	}
	if offset <= 0 {
		return fmt.Sprintf(" LIMIT %d", limit)
	}
	return fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
}

// VacuumDatabase SQLite执行VACUUM操作
func (s *SQLiteAdapter) VacuumDatabase(ctx context.Context) error {
	s.logger.Info("正在执行SQLite VACUUM操作")

	_, err := s.db.ExecContext(ctx, "VACUUM")
	if err != nil {
		return fmt.Errorf("failed to vacuum SQLite database: %w", err)
	}

	s.logger.Info("✅ SQLite VACUUM操作完成")
	return nil
}

// GetDatabaseStats 获取SQLite数据库统计信息
func (s *SQLiteAdapter) GetDatabaseStats(ctx context.Context) (*DatabaseStats, error) {
	stats := &DatabaseStats{}

	// 获取请求记录总数
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM request_logs").Scan(&stats.TotalRequests)
	if err != nil {
		return nil, fmt.Errorf("failed to get total requests count: %w", err)
	}

	// 获取汇总记录总数
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_summary").Scan(&stats.TotalSummaries)
	if err != nil {
		return nil, fmt.Errorf("failed to get total summaries count: %w", err)
	}

	// 获取最早和最新的记录时间
	var earliestStr, latestStr sql.NullString
	err = s.db.QueryRowContext(ctx, "SELECT MIN(start_time), MAX(start_time) FROM request_logs").Scan(&earliestStr, &latestStr)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get record time range: %w", err)
	}

	if earliestStr.Valid {
		if t, err := time.Parse(time.RFC3339, earliestStr.String); err == nil {
			stats.EarliestRecord = &t
		}
	}
	if latestStr.Valid {
		if t, err := time.Parse(time.RFC3339, latestStr.String); err == nil {
			stats.LatestRecord = &t
		}
	}

	// 获取数据库文件大小（SQLite特有）
	var pageCount, pageSize int64
	err = s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	if err == nil {
		err = s.db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
		if err == nil {
			stats.DatabaseSize = pageCount * pageSize
		}
	}

	// 获取总成本
	err = s.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(total_cost_usd), 0) FROM request_logs WHERE total_cost_usd > 0").Scan(&stats.TotalCostUSD)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get total cost: %w", err)
	}

	return stats, nil
}

// GetConnectionStats 获取连接池统计信息
func (s *SQLiteAdapter) GetConnectionStats() ConnectionStats {
	if s.db == nil {
		return ConnectionStats{}
	}

	dbStats := s.db.Stats()
	return ConnectionStats{
		OpenConnections:  dbStats.OpenConnections,
		IdleConnections:  dbStats.Idle,
		InUseConnections: dbStats.InUse,
		WaitCount:        dbStats.WaitCount,
		WaitDuration:     dbStats.WaitDuration,
		MaxLifetime:      0, // SQLite不限制连接生命周期
	}
}

// GetDatabaseType 返回数据库类型标识
func (s *SQLiteAdapter) GetDatabaseType() string {
	return "sqlite"
}

// diagnoseTimezoneSettings 诊断SQLite时区设置，帮助调试时区不一致问题
func (s *SQLiteAdapter) diagnoseTimezoneSettings() {
	// SQLite时区诊断相对简单，因为我们在应用层处理时区
	goNow := time.Now()
	goInConfigTZ := time.Now().In(s.location)

	_, goOffset := goInConfigTZ.Zone()
	goOffsetHours := float64(goOffset) / 3600

	s.logger.Info("🔍 SQLite时区诊断信息",
		"configured_timezone", s.location.String(),
		"system_now", goNow.Format("2006-01-02 15:04:05 -07:00"),
		"configured_tz_now", goInConfigTZ.Format("2006-01-02 15:04:05 -07:00"),
		"configured_offset_hours", goOffsetHours,
		"builddatetimenow_output", s.BuildDateTimeNow())

	// 验证时区偏移是否符合预期
	if s.location.String() == "Asia/Shanghai" && goOffsetHours == 8.0 {
		s.logger.Info("✅ SQLite时区设置正确: 使用Asia/Shanghai时区 (+8小时)")
	} else if s.location == time.UTC {
		s.logger.Info("ℹ️  SQLite使用UTC时区")
	} else {
		s.logger.Info("ℹ️  SQLite使用自定义时区", "timezone", s.location.String(), "offset_hours", goOffsetHours)
	}
}

// getSQLiteAppDataDir 获取应用数据目录（跨平台）
// 复制自 internal/utils/appdir.go，避免循环依赖
// Windows: %APPDATA%\AI-Switchboard
// macOS: ~/Library/Application Support/AI-Switchboard
// Linux: ~/.local/share/ai-switchboard
func getSQLiteAppDataDir() string {
	var baseDir string

	switch runtime.GOOS {
	case "windows":
		baseDir = os.Getenv("APPDATA")
		if baseDir == "" {
			baseDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(baseDir, "AI-Switchboard")

	case "darwin":
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, "Library", "Application Support", "AI-Switchboard")

	case "linux":
		homeDir, _ := os.UserHomeDir()
		xdgDataHome := os.Getenv("XDG_DATA_HOME")
		if xdgDataHome != "" {
			return filepath.Join(xdgDataHome, "ai-switchboard")
		}
		return filepath.Join(homeDir, ".local", "share", "ai-switchboard")

	default:
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, ".ai-switchboard")
	}
}
