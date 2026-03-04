package tracking

import (
	"context"
	"database/sql"
	"embed"
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
		// Windows: %APPDATA%\CC-Forwarder\data\usage.db
		// macOS: ~/Library/Application Support/CC-Forwarder/data/usage.db
		// Linux: ~/.local/share/cc-forwarder/data/usage.db
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
		checkColumn string
		alterSQL    string
		description string
	}{
		{
			checkColumn: "cache_creation_5m_tokens",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN cache_creation_5m_tokens INTEGER DEFAULT 0",
			description: "5分钟缓存创建tokens字段",
		},
		{
			checkColumn: "cache_creation_1h_tokens",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN cache_creation_1h_tokens INTEGER DEFAULT 0",
			description: "1小时缓存创建tokens字段",
		},
		{
			checkColumn: "cache_creation_5m_cost_usd",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN cache_creation_5m_cost_usd REAL DEFAULT 0",
			description: "5分钟缓存创建成本字段",
		},
		{
			checkColumn: "cache_creation_1h_cost_usd",
			alterSQL:    "ALTER TABLE request_logs ADD COLUMN cache_creation_1h_cost_usd REAL DEFAULT 0",
			description: "1小时缓存创建成本字段",
		},
	}

	for _, m := range migrations {
		exists, err := s.columnExists(ctx, "request_logs", m.checkColumn)
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
// Windows: %APPDATA%\CC-Forwarder
// macOS: ~/Library/Application Support/CC-Forwarder
// Linux: ~/.local/share/cc-forwarder
func getSQLiteAppDataDir() string {
	var baseDir string

	switch runtime.GOOS {
	case "windows":
		baseDir = os.Getenv("APPDATA")
		if baseDir == "" {
			baseDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(baseDir, "CC-Forwarder")

	case "darwin":
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, "Library", "Application Support", "CC-Forwarder")

	case "linux":
		homeDir, _ := os.UserHomeDir()
		xdgDataHome := os.Getenv("XDG_DATA_HOME")
		if xdgDataHome != "" {
			return filepath.Join(xdgDataHome, "cc-forwarder")
		}
		return filepath.Join(homeDir, ".local", "share", "cc-forwarder")

	default:
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, ".cc-forwarder")
	}
}
