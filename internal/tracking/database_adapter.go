package tracking

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// DatabaseAdapter 定义数据库操作接口
// v4.1.0: 简化为仅支持 SQLite，热池架构下无需外部数据库依赖
type DatabaseAdapter interface {
	// 基础连接管理
	Open() error
	Close() error
	Ping(ctx context.Context) error

	// 获取数据库连接
	GetDB() *sql.DB
	GetReadDB() *sql.DB
	GetWriteDB() *sql.DB

	// 事务支持
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)

	// 数据库初始化
	InitSchema() error

	// SQL语法适配
	BuildInsertOrReplaceQuery(table string, columns []string, values []string) string
	BuildUTCDateTimeNow() string
	BuildLimitOffset(limit, offset int) string

	// 数据库特定操作
	VacuumDatabase(ctx context.Context) error
	GetDatabaseStats(ctx context.Context) (*DatabaseStats, error)

	// 连接统计
	GetConnectionStats() ConnectionStats

	// 类型标识
	GetDatabaseType() string
}

// DatabaseConfig 数据库配置结构
// v4.1.0: 简化配置，仅保留 SQLite 相关字段
type DatabaseConfig struct {
	// 数据库类型（保留用于向后兼容，但只支持 "sqlite"）
	Type string `yaml:"type"` // 仅支持 "sqlite"

	// SQLite配置
	DatabasePath string `yaml:"database_path,omitempty"`

	// ===== 以下 MySQL 相关字段已废弃，保留用于配置文件向后兼容 =====
	// 注意：这些字段不再生效，配置这些字段会打印警告日志
	// Host     string `yaml:"host,omitempty"`     // DEPRECATED: MySQL not supported
	// Port     int    `yaml:"port,omitempty"`     // DEPRECATED: MySQL not supported
	// Database string `yaml:"database,omitempty"` // DEPRECATED: MySQL not supported
	// Username string `yaml:"username,omitempty"` // DEPRECATED: MySQL not supported
	// Password string `yaml:"password,omitempty"` // DEPRECATED: MySQL not supported
	// MaxOpenConns    int           `yaml:"max_open_conns,omitempty"`    // DEPRECATED
	// MaxIdleConns    int           `yaml:"max_idle_conns,omitempty"`    // DEPRECATED
	// ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime,omitempty"` // DEPRECATED
	// ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time,omitempty"` // DEPRECATED
	// Charset  string `yaml:"charset,omitempty"`  // DEPRECATED: MySQL not supported
}

// ConnectionStats 连接池统计信息
type ConnectionStats struct {
	OpenConnections  int           `json:"open_connections"`
	IdleConnections  int           `json:"idle_connections"`
	InUseConnections int           `json:"in_use_connections"`
	WaitCount        int64         `json:"wait_count"`
	WaitDuration     time.Duration `json:"wait_duration"`
	MaxLifetime      time.Duration `json:"max_lifetime"`
}

// NewDatabaseAdapter 数据库适配器工厂函数
// v4.1.0: 简化为仅创建 SQLite 适配器
func NewDatabaseAdapter(config DatabaseConfig) (DatabaseAdapter, error) {
	// 确定数据库类型
	dbType := getDatabaseType(config)

	switch dbType {
	case "sqlite", "":
		return NewSQLiteAdapter(config)
	case "mysql":
		// MySQL 支持已移除，返回错误提示
		slog.Warn("⚠️  MySQL 支持已在 v4.1.0 中移除，请使用 SQLite",
			"reason", "热池架构下无需外部数据库依赖",
			"suggestion", "删除数据库配置中的 type: mysql，使用默认 SQLite")
		return nil, fmt.Errorf("MySQL support removed in v4.1.0: use SQLite instead (set type: sqlite or remove type field)")
	default:
		return nil, fmt.Errorf("unsupported database type: %s (only 'sqlite' is supported)", dbType)
	}
}

// getDatabaseType 从配置推断数据库类型
func getDatabaseType(config DatabaseConfig) string {
	// 优先使用明确配置的类型
	if config.Type != "" {
		return config.Type
	}
	// 默认为 SQLite
	return "sqlite"
}

// setDefaultConfig 设置数据库配置默认值
// v4.1.0: 简化为仅处理 SQLite 配置
func setDefaultConfig(config *DatabaseConfig) {
	// SQLite 默认配置
	if config.DatabasePath == "" {
		config.DatabasePath = "data/usage.db"
	}
}
