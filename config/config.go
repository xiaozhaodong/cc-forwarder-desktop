package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server              ServerConfig               `yaml:"server"`
	Strategy            StrategyConfig             `yaml:"strategy"`
	Retry               RetryConfig                `yaml:"retry"`
	Health              HealthConfig               `yaml:"health"`
	Logging             LoggingConfig              `yaml:"logging"`
	Streaming           StreamingConfig            `yaml:"streaming"`
	Group               GroupConfig                `yaml:"group"`                    // Group configuration (DEPRECATED: use Failover instead)
	Failover            FailoverConfig             `yaml:"failover"`                 // Failover configuration (v4.0+)
	FailureTracker      FailureTrackerConfig       `yaml:"failure_tracker"`          // Failure tracker configuration (v5.2.6+)
	RequestSuspend      LegacyRequestSuspendConfig `yaml:"request_suspend" json:"-"` // 已废弃：仅 YAML 解析兼容（挂起体系已删除）
	UsageTracking       UsageTrackingConfig        `yaml:"usage_tracking"`           // Usage tracking configuration
	TokenCounting       TokenCountingConfig        `yaml:"token_counting"`           // Token counting configuration
	EndpointsStorage    EndpointsStorageConfig     `yaml:"endpoints_storage"`        // Endpoints storage configuration (v5.0+)
	AccountPool         AccountPoolConfig          `yaml:"account_pool"`             // Account pool routing configuration (v6.0+)
	Proxy               ProxyConfig                `yaml:"proxy"`
	Auth                AuthConfig                 `yaml:"auth"`
	TUI                 TUIConfig                  `yaml:"tui"`                    // TUI configuration (DEPRECATED: TUI has been removed)
	GlobalTimeout       time.Duration              `yaml:"global_timeout"`         // Global timeout for non-streaming requests
	RequestBodyMaxBytes int64                      `yaml:"request_body_max_bytes"` // Optional max request body size in bytes, 0 = unlimited
	Timezone            string                     `yaml:"timezone"`               // Global timezone setting for all components
	Endpoints           []EndpointConfig           `yaml:"endpoints"`

	// Runtime priority override (not serialized to YAML)
	PrimaryEndpoint string `yaml:"-"` // Primary endpoint name from command line
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type StrategyConfig struct {
	Type             string        `yaml:"type"`                // "priority" or "fastest"
	FastTestEnabled  bool          `yaml:"fast_test_enabled"`   // Enable pre-request fast testing
	FastTestCacheTTL time.Duration `yaml:"fast_test_cache_ttl"` // Cache TTL for fast test results
	FastTestTimeout  time.Duration `yaml:"fast_test_timeout"`   // Timeout for individual fast tests
	FastTestPath     string        `yaml:"fast_test_path"`      // Path for fast testing (default: health path)
}

type RetryConfig struct {
	MaxAttempts int           `yaml:"max_attempts"`
	BaseDelay   time.Duration `yaml:"base_delay"`
	MaxDelay    time.Duration `yaml:"max_delay"`
	Multiplier  float64       `yaml:"multiplier"`
}

type HealthConfig struct {
	CheckInterval time.Duration `yaml:"check_interval"`
	Timeout       time.Duration `yaml:"timeout"`
	HealthPath    string        `yaml:"health_path"`
}

type LoggingConfig struct {
	Level                string           `yaml:"level"`
	Format               string           `yaml:"format"`                 // "json" or "text"
	FileEnabled          bool             `yaml:"file_enabled"`           // Enable file logging
	FilePath             string           `yaml:"file_path"`              // Log file path
	MaxFileSize          string           `yaml:"max_file_size"`          // Max file size (e.g., "100MB")
	MaxFiles             int              `yaml:"max_files"`              // Max number of rotated files to keep
	CompressRotated      bool             `yaml:"compress_rotated"`       // Compress rotated log files
	DisableResponseLimit bool             `yaml:"disable_response_limit"` // Disable response content output limit when file logging is enabled
	TokenDebug           TokenDebugConfig `yaml:"token_debug"`            // Token debug configuration
}

// TokenDebugConfig Token调试配置
type TokenDebugConfig struct {
	Enabled         bool   `yaml:"enabled"`           // 是否启用Token调试功能，默认: true
	SavePath        string `yaml:"save_path"`         // 调试文件保存目录，默认: logs
	MaxFiles        int    `yaml:"max_files"`         // 最大保留调试文件数量，默认: 50 (0=不限制)
	AutoCleanupDays int    `yaml:"auto_cleanup_days"` // 自动清理N天前的调试文件，默认: 7 (0=不清理)
}

type StreamingConfig struct {
	HeartbeatInterval     time.Duration `yaml:"heartbeat_interval"`
	ReadTimeout           time.Duration `yaml:"read_timeout"`
	MaxIdleTime           time.Duration `yaml:"max_idle_time"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"` // 响应头超时时间，默认: 60s
	// EOFRetryHintRaw 流式错误时发送可重试中断消息（触发客户端自动重试）。
	// 新配置键；nil 表示未配置，回退旧键 request_suspend.eof_retry_hint。
	EOFRetryHintRaw *bool `yaml:"eof_retry_hint" json:"-"`
	// EOFRetryHint 生效值：新键 ?? 旧键 ?? false（解析后由 resolveEOFRetryHint 计算）
	EOFRetryHint bool `yaml:"-" json:"eof_retry_hint"`
}

// GroupConfig (DEPRECATED in v4.0: use FailoverConfig instead)
// Kept for backward compatibility with v3.x configurations
type GroupConfig struct {
	Cooldown                time.Duration `yaml:"cooldown"`                   // Cooldown duration for groups when all endpoints fail
	AutoSwitchBetweenGroups bool          `yaml:"auto_switch_between_groups"` // Whether to automatically switch between groups, default: true
}

// FailoverConfig v4.0 故障转移配置
type FailoverConfig struct {
	Enabled         bool          `yaml:"enabled"`          // 启用故障转移，默认: true
	DefaultCooldown time.Duration `yaml:"default_cooldown"` // 默认冷却时间，默认: 10m
}

// FailureTrackerConfig v5.2.6 失败追踪器配置
// 追踪端点失败次数，用于触发故障转移或拒绝请求
type FailureTrackerConfig struct {
	Enabled    bool          `yaml:"enabled"`     // 启用失败追踪，默认: false
	TimeWindow time.Duration `yaml:"time_window"` // 时间窗口，默认: 5m
	Threshold  int           `yaml:"threshold"`   // 失败次数阈值，默认: 3
	Action     string        `yaml:"action"`      // 触发动作: failover|reject，默认: failover
}

// LegacyRequestSuspendConfig 仅用于 YAML 解析兼容（挂起体系已随 v7 重构删除）。
// 旧配置文件不报错；json:"-" 使其不出现在 Wails/JSON 输出中。
// eof_retry_hint 旧键仍参与 EOFRetryHint 生效值回退计算。
type LegacyRequestSuspendConfig struct {
	Enabled              bool          `yaml:"enabled" json:"-"`
	Timeout              time.Duration `yaml:"timeout" json:"-"`
	MaxSuspendedRequests int           `yaml:"max_suspended_requests" json:"-"`
	EOFRetryHint         *bool         `yaml:"eof_retry_hint" json:"-"`
}

// ModelPricing 模型定价配置
// Deprecated: v5.0+ 定价配置已迁移到 SQLite model_pricing 表，通过前端「定价」页面管理
// 保留此结构体仅为向后兼容，不再使用
type ModelPricing struct {
	Input         float64 `yaml:"input"`          // per 1M tokens
	Output        float64 `yaml:"output"`         // per 1M tokens
	CacheCreation float64 `yaml:"cache_creation"` // per 1M tokens (缓存创建)
	CacheRead     float64 `yaml:"cache_read"`     // per 1M tokens (缓存读取)
}

type UsageTrackingConfig struct {
	Enabled bool `yaml:"enabled"` // Enable usage tracking, default: false

	// 向后兼容：保留原有的 database_path 配置
	DatabasePath string `yaml:"database_path"` // SQLite database file path, default: data/usage.db

	// 新增：数据库配置（可选，优先级高于 database_path）
	Database *DatabaseBackendConfig `yaml:"database,omitempty"` // Database configuration (optional)

	BufferSize      int           `yaml:"buffer_size"`      // Event buffer size, default: 1000
	BatchSize       int           `yaml:"batch_size"`       // Batch write size, default: 100
	FlushInterval   time.Duration `yaml:"flush_interval"`   // Force flush interval, default: 30s
	MaxRetry        int           `yaml:"max_retry"`        // Max retry count for write failures, default: 3
	RetentionDays   int           `yaml:"retention_days"`   // Data retention days (0=permanent), default: 90
	CleanupInterval time.Duration `yaml:"cleanup_interval"` // Cleanup task execution interval, default: 24h

	// Deprecated: v5.0+ 以下定价配置已废弃，迁移到 SQLite model_pricing 表
	// 通过前端「定价」页面管理，这些字段仅保留用于向后兼容解析
	ModelPricing   map[string]ModelPricing `yaml:"model_pricing,omitempty"`   // [废弃] Model pricing configuration
	DefaultPricing ModelPricing            `yaml:"default_pricing,omitempty"` // [废弃] Default pricing for unknown models
}

// DatabaseBackendConfig 数据库后端配置
// v4.1.0: 简化为仅支持 SQLite，热池架构下无需外部数据库依赖
type DatabaseBackendConfig struct {
	Type string `yaml:"type"` // 仅支持 "sqlite"（保留用于向后兼容）

	// SQLite配置
	Path string `yaml:"path,omitempty"` // SQLite文件路径

	// 时区配置（用于时间格式化）
	Timezone string `yaml:"timezone,omitempty"`

	// ===== 以下字段已废弃，保留用于配置文件向后兼容（解析不报错）=====
	// 这些字段不再生效，请从配置文件中删除
	Host            string        `yaml:"host,omitempty"`               // DEPRECATED: MySQL not supported in v4.1+
	Port            int           `yaml:"port,omitempty"`               // DEPRECATED: MySQL not supported in v4.1+
	Database        string        `yaml:"database,omitempty"`           // DEPRECATED: MySQL not supported in v4.1+
	Username        string        `yaml:"username,omitempty"`           // DEPRECATED: MySQL not supported in v4.1+
	Password        string        `yaml:"password,omitempty"`           // DEPRECATED: MySQL not supported in v4.1+
	MaxOpenConns    int           `yaml:"max_open_conns,omitempty"`     // DEPRECATED: MySQL not supported in v4.1+
	MaxIdleConns    int           `yaml:"max_idle_conns,omitempty"`     // DEPRECATED: MySQL not supported in v4.1+
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime,omitempty"`  // DEPRECATED: MySQL not supported in v4.1+
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time,omitempty"` // DEPRECATED: MySQL not supported in v4.1+
	Charset         string        `yaml:"charset,omitempty"`            // DEPRECATED: MySQL not supported in v4.1+
}

type ProxyConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Type     string `yaml:"type"`     // "http", "https", "socks5"
	URL      string `yaml:"url"`      // Complete proxy URL
	Host     string `yaml:"host"`     // Proxy host
	Port     int    `yaml:"port"`     // Proxy port
	Username string `yaml:"username"` // Optional auth username
	Password string `yaml:"password"` // Optional auth password
}

type AuthConfig struct {
	Enabled bool   `yaml:"enabled"`         // Enable authentication, default: false
	Token   string `yaml:"token,omitempty"` // Bearer token for authentication
}

// TUIConfig is DEPRECATED - TUI has been removed in v4.0
// Kept for backward compatibility with old configuration files
type TUIConfig struct {
	Enabled           bool          `yaml:"enabled"`             // DEPRECATED: TUI has been removed
	UpdateInterval    time.Duration `yaml:"update_interval"`     // DEPRECATED: TUI has been removed
	SavePriorityEdits bool          `yaml:"save_priority_edits"` // DEPRECATED: TUI has been removed
}

// TokenCountingConfig Token计数配置
type TokenCountingConfig struct {
	Enabled         bool    `yaml:"enabled"`          // 启用count_tokens支持
	EstimationRatio float64 `yaml:"estimation_ratio"` // Token估算比例 (1 token ≈ N 字符)
}

// EndpointsStorageConfig 端点存储配置 (v5.0+)
// 支持从 YAML 文件或 SQLite 数据库加载端点配置
type EndpointsStorageConfig struct {
	Type string `yaml:"type"` // 存储类型: "yaml" | "sqlite"，默认 "yaml"
}

// AccountPoolConfig 账号池路由配置
// v6.0+: 当 enabled=true 时，/v1/responses 走 Codex 账号池链路，与 Claude endpoint 链路分离
type AccountPoolConfig struct {
	Enabled       bool                           `yaml:"enabled"`        // 是否启用账号池路由，默认: false
	FailurePolicy AccountPoolFailurePolicyConfig `yaml:"failure_policy"` // 账号池失败策略配置
}

// AccountPoolFailurePolicyConfig 账号池失败策略配置
// 用于控制 Codex 账号池中的软失败窗口、冷却时间和本地特例标记。
type AccountPoolFailurePolicyConfig struct {
	SoftFailureWindow               time.Duration                     `yaml:"soft_failure_window"`                 // 429/5xx 软失败统计窗口，默认: 5m
	SoftFailureThreshold            int                               `yaml:"soft_failure_threshold"`              // 软失败触发阈值，默认: 3
	RespectRetryAfter               *bool                             `yaml:"respect_retry_after"`                 // 429 时是否优先使用上游 Retry-After，默认: true
	LocalNoAvailableProvidersMarker string                            `yaml:"local_no_available_providers_marker"` // 本地 no_available_providers 透传标记，默认: no_available_providers::ccf_local
	Cooldowns                       AccountPoolFailureCooldownsConfig `yaml:"cooldowns"`                           // 各类失败的冷却时间配置
}

// AccountPoolFailureCooldownsConfig 账号池失败冷却时间配置
// 不同错误类型使用不同冷却时间，避免坏账号反复命中或瞬时抖动过度切换。
type AccountPoolFailureCooldownsConfig struct {
	ConnectionFailure time.Duration `yaml:"connection_failure"` // 连接失败 / 连接超时 / DNS / TLS 的冷却时间，默认: 90s
	ProcessingFailure time.Duration `yaml:"processing_failure"` // 响应处理异常的冷却时间，默认: 60s
	RateLimitDefault  time.Duration `yaml:"rate_limit_default"` // 429 无 Retry-After 时的默认冷却时间，默认: 180s
	ServerError       time.Duration `yaml:"server_error"`       // 普通 5xx 的冷却时间，默认: 120s
}

type EndpointConfig struct {
	Name                string            `yaml:"name"`
	URL                 string            `yaml:"url"`
	Channel             string            `yaml:"channel,omitempty"` // v5.0: 渠道标签（用于分组展示）
	Priority            int               `yaml:"priority"`
	Group               string            `yaml:"group,omitempty"`            // DEPRECATED in v4.0
	GroupPriority       int               `yaml:"group-priority,omitempty"`   // DEPRECATED in v4.0: use Priority instead
	FailoverEnabled     *bool             `yaml:"failover_enabled,omitempty"` // v4.0: 是否参与故障转移，默认: true
	Cooldown            *time.Duration    `yaml:"cooldown,omitempty"`         // v4.0: 端点级冷却时间（可选），默认使用全局配置
	Token               string            `yaml:"token,omitempty"`            // 单 Token 配置（向后兼容）
	ApiKey              string            `yaml:"api-key,omitempty"`          // 单 API Key 配置（向后兼容）
	Tokens              []TokenConfig     `yaml:"tokens,omitempty"`           // 多 Token 配置（新功能）
	ApiKeys             []ApiKeyConfig    `yaml:"api-keys,omitempty"`         // 多 API Key 配置（新功能）
	Timeout             time.Duration     `yaml:"timeout"`
	Headers             map[string]string `yaml:"headers,omitempty"`
	SupportsCountTokens bool              `yaml:"supports_count_tokens,omitempty"` // 是否支持count_tokens端点
	Enabled             *bool             `yaml:"enabled,omitempty"`               // v5.0: 是否激活为代理端点（SQLite模式），默认: true
}

// TokenConfig Token 配置项，用于多 Token 切换功能
type TokenConfig struct {
	Name  string `yaml:"name"`  // Key 标识名称（用于 UI 显示）
	Value string `yaml:"value"` // Token 值
}

// ApiKeyConfig API Key 配置项，用于多 API Key 切换功能
type ApiKeyConfig struct {
	Name  string `yaml:"name"`  // Key 标识名称（用于 UI 显示）
	Value string `yaml:"value"` // API Key 值
}

// LoadConfig loads configuration from file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Check if auto_switch_between_groups is explicitly set in YAML
	hasAutoSwitchConfig := strings.Contains(string(data), "auto_switch_between_groups")

	// Check if v4.0 failover config is present
	hasFailoverConfig := strings.Contains(string(data), "failover:")

	// Check if v5.2.6 failure_tracker config is present
	hasFailureTrackerConfig := strings.Contains(string(data), "failure_tracker:")

	// Check if account_pool.enabled is explicitly set in YAML
	hasAccountPoolEnabledConfig := hasYAMLNestedKey(data, "account_pool", "enabled")

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	config.setDefaults()

	// Handle auto_switch_between_groups default for backward compatibility
	if !hasAutoSwitchConfig {
		config.Group.AutoSwitchBetweenGroups = true // Default to auto mode for backward compatibility
	}

	// v3.x → v4.0 配置映射（向后兼容）
	if !hasFailoverConfig {
		// 没有v4.0配置，尝试从v3.x group配置映射
		if config.Group.Cooldown > 0 {
			config.Failover.DefaultCooldown = config.Group.Cooldown
		}
		config.Failover.Enabled = config.Group.AutoSwitchBetweenGroups
	}

	// v5.2.6+: 失败追踪器默认启用
	if !hasFailureTrackerConfig {
		config.FailureTracker.Enabled = true
	}

	// v6.0+: 账号池路由默认保持关闭，必须显式启用后才接管 Codex /v1/responses
	if !hasAccountPoolEnabledConfig {
		config.AccountPool.Enabled = false
	}

	// Validate configuration
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

func hasYAMLNestedKey(data []byte, parentKey, childKey string) bool {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}
	parent, ok := raw[parentKey]
	if !ok || parent == nil {
		return false
	}
	switch m := parent.(type) {
	case map[string]any:
		_, exists := m[childKey]
		return exists
	case map[any]any:
		for k := range m {
			if key, ok := k.(string); ok && key == childKey {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// setDefaults sets default values for configuration
func (c *Config) setDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "localhost"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Strategy.Type == "" {
		c.Strategy.Type = "priority"
	}
	// Set fast test defaults
	if c.Strategy.FastTestCacheTTL == 0 {
		c.Strategy.FastTestCacheTTL = 3 * time.Second // Default 3 seconds cache
	}
	if c.Strategy.FastTestTimeout == 0 {
		c.Strategy.FastTestTimeout = 1 * time.Second // Default 1 second timeout for fast tests
	}
	if c.Strategy.FastTestPath == "" {
		c.Strategy.FastTestPath = c.Health.HealthPath // Default to health path
	}
	if c.Retry.MaxAttempts == 0 {
		c.Retry.MaxAttempts = 3
	}
	if c.Retry.BaseDelay == 0 {
		c.Retry.BaseDelay = time.Second
	}
	if c.Retry.MaxDelay == 0 {
		c.Retry.MaxDelay = 30 * time.Second
	}
	if c.Retry.Multiplier == 0 {
		c.Retry.Multiplier = 2.0
	}
	if c.Health.CheckInterval == 0 {
		c.Health.CheckInterval = 30 * time.Second
	}
	if c.Health.Timeout == 0 {
		c.Health.Timeout = 5 * time.Second
	}
	if c.Health.HealthPath == "" {
		c.Health.HealthPath = ""
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "text"
	}
	// Set file logging defaults
	if c.Logging.FileEnabled && c.Logging.FilePath == "" {
		c.Logging.FilePath = "logs/app.log"
	}
	if c.Logging.FileEnabled && c.Logging.MaxFileSize == "" {
		c.Logging.MaxFileSize = "100MB"
	}
	if c.Logging.FileEnabled && c.Logging.MaxFiles == 0 {
		c.Logging.MaxFiles = 10
	}

	// Set token debug defaults
	// Default: enabled in development, consider disabling in production
	if c.Logging.TokenDebug.SavePath == "" {
		c.Logging.TokenDebug.SavePath = "logs"
	}
	if c.Logging.TokenDebug.MaxFiles == 0 {
		c.Logging.TokenDebug.MaxFiles = 50
	}
	if c.Logging.TokenDebug.AutoCleanupDays == 0 {
		c.Logging.TokenDebug.AutoCleanupDays = 7
	}
	// Note: TokenDebug.Enabled has no default - defaults to false (zero value)
	if c.Streaming.HeartbeatInterval == 0 {
		c.Streaming.HeartbeatInterval = 30 * time.Second
	}
	if c.Streaming.ReadTimeout == 0 {
		c.Streaming.ReadTimeout = 10 * time.Second
	}
	if c.Streaming.MaxIdleTime == 0 {
		c.Streaming.MaxIdleTime = 120 * time.Second
	}
	if c.Streaming.ResponseHeaderTimeout == 0 {
		c.Streaming.ResponseHeaderTimeout = 60 * time.Second // 默认60秒，适合AI服务
	}

	// Set global timeout default
	if c.GlobalTimeout == 0 {
		c.GlobalTimeout = 300 * time.Second // Default 5 minutes for non-streaming requests
	}

	// Set global timezone default
	if c.Timezone == "" {
		c.Timezone = "Asia/Shanghai" // Default timezone for all components
	}

	// Set group defaults (DEPRECATED: kept for v3.x compatibility)
	if c.Group.Cooldown == 0 {
		c.Group.Cooldown = 600 * time.Second // Default 10 minutes cooldown for groups
	}

	// Set failover defaults (v4.0+)
	// Failover.Enabled defaults to true if not explicitly set
	// Note: We check for zero value and set default here
	if c.Failover.DefaultCooldown == 0 {
		c.Failover.DefaultCooldown = 600 * time.Second // Default 10 minutes cooldown
	}
	// Failover.Enabled has no explicit default here - will be set by LoadConfig compatibility logic

	// Set failure tracker defaults (v5.2.6+)
	if c.FailureTracker.TimeWindow == 0 {
		c.FailureTracker.TimeWindow = 5 * time.Minute // Default 5 minutes time window
	}
	if c.FailureTracker.Threshold == 0 {
		c.FailureTracker.Threshold = 3 // Default threshold: 3 failures
	}
	if c.FailureTracker.Action == "" {
		c.FailureTracker.Action = "failover" // Default action: failover
	}
	// v5.2.6+: FailureTracker.Enabled defaults to true for better fault tolerance
	// 用户可以通过设置页面禁用

	// Set account pool failure policy defaults (v6.0+)
	if c.AccountPool.FailurePolicy.SoftFailureWindow == 0 {
		c.AccountPool.FailurePolicy.SoftFailureWindow = 5 * time.Minute // Default soft failure window
	}
	if c.AccountPool.FailurePolicy.SoftFailureThreshold == 0 {
		c.AccountPool.FailurePolicy.SoftFailureThreshold = 3 // Default soft failure threshold
	}
	if c.AccountPool.FailurePolicy.RespectRetryAfter == nil {
		value := true
		c.AccountPool.FailurePolicy.RespectRetryAfter = &value
	}
	if c.AccountPool.FailurePolicy.LocalNoAvailableProvidersMarker == "" {
		c.AccountPool.FailurePolicy.LocalNoAvailableProvidersMarker = "no_available_providers::ccf_local"
	}
	if c.AccountPool.FailurePolicy.Cooldowns.ConnectionFailure == 0 {
		c.AccountPool.FailurePolicy.Cooldowns.ConnectionFailure = 90 * time.Second // Default connection failure cooldown
	}
	if c.AccountPool.FailurePolicy.Cooldowns.ProcessingFailure == 0 {
		c.AccountPool.FailurePolicy.Cooldowns.ProcessingFailure = 60 * time.Second // Default processing failure cooldown
	}
	if c.AccountPool.FailurePolicy.Cooldowns.RateLimitDefault == 0 {
		c.AccountPool.FailurePolicy.Cooldowns.RateLimitDefault = 180 * time.Second // Default rate limit cooldown
	}
	if c.AccountPool.FailurePolicy.Cooldowns.ServerError == 0 {
		c.AccountPool.FailurePolicy.Cooldowns.ServerError = 120 * time.Second // Default server error cooldown
	}

	// Set endpoints storage defaults (v5.0+)
	if c.EndpointsStorage.Type == "" {
		c.EndpointsStorage.Type = "yaml" // Default to YAML for backward compatibility
	}

	// EOFRetryHint 生效值：新键 streaming.eof_retry_hint ?? 旧键 request_suspend.eof_retry_hint ?? false
	c.resolveEOFRetryHint()

	// Set usage tracking defaults
	if c.UsageTracking.DatabasePath == "" {
		// 使用跨平台用户目录作为默认路径
		// Windows: %APPDATA%\AI-Switchboard\data\usage.db
		// macOS: ~/Library/Application Support/AI-Switchboard/data/usage.db
		// Linux: ~/.local/share/ai-switchboard/data/usage.db
		c.UsageTracking.DatabasePath = filepath.Join(getConfigAppDataDir(), "data", "usage.db")
	}
	if c.UsageTracking.BufferSize == 0 {
		c.UsageTracking.BufferSize = 1000 // Default buffer size
	}
	if c.UsageTracking.BatchSize == 0 {
		c.UsageTracking.BatchSize = 100 // Default batch size
	}
	if c.UsageTracking.FlushInterval == 0 {
		c.UsageTracking.FlushInterval = 30 * time.Second // Default flush interval
	}
	if c.UsageTracking.MaxRetry == 0 {
		c.UsageTracking.MaxRetry = 3 // Default max retry count
	}
	if c.UsageTracking.RetentionDays == 0 {
		c.UsageTracking.RetentionDays = 90 // Default retention 90 days
	}
	if c.UsageTracking.CleanupInterval == 0 {
		c.UsageTracking.CleanupInterval = 24 * time.Hour // Default cleanup interval
	}
	// v5.0+ 注意：model_pricing 和 default_pricing 已废弃
	// 定价配置现在从 SQLite model_pricing 表加载，通过前端「定价」页面管理
	// 这里不再设置默认值，保留字段仅为向后兼容解析旧配置文件

	// UsageTracking.Enabled defaults to false (zero value) for backward compatibility

	// TUI has been removed in v4.0 - no defaults needed

	// Set Token Counting defaults
	if c.TokenCounting.EstimationRatio == 0 {
		c.TokenCounting.EstimationRatio = 4.0 // Default: 1 token ≈ 4 characters
	}
	// TokenCounting.Enabled defaults to false (zero value) for backward compatibility

	// Set default timeouts for endpoints and handle parameter inheritance (except tokens)
	var defaultEndpoint *EndpointConfig
	if len(c.Endpoints) > 0 {
		defaultEndpoint = &c.Endpoints[0]
	}

	// Handle group inheritance - endpoints inherit group settings from previous endpoint
	var currentGroup string = "Default" // Default group name
	var currentGroupPriority int = 1    // Default group priority

	for i := range c.Endpoints {
		// v3.x → v4.0 兼容：group-priority 映射到 priority
		if c.Endpoints[i].Priority == 0 && c.Endpoints[i].GroupPriority > 0 {
			c.Endpoints[i].Priority = c.Endpoints[i].GroupPriority
		}

		// Handle group inheritance - check if this endpoint defines a new group
		if c.Endpoints[i].Group != "" {
			// Endpoint specifies a group, use it and update current group
			currentGroup = c.Endpoints[i].Group
			if c.Endpoints[i].GroupPriority != 0 {
				currentGroupPriority = c.Endpoints[i].GroupPriority
			}
		} else {
			// Endpoint doesn't specify group, inherit from previous
			c.Endpoints[i].Group = currentGroup
			c.Endpoints[i].GroupPriority = currentGroupPriority
		}

		// If GroupPriority is still 0 after inheritance, set default
		if c.Endpoints[i].GroupPriority == 0 {
			c.Endpoints[i].GroupPriority = currentGroupPriority
		}

		// Set default timeout if not specified
		if c.Endpoints[i].Timeout == 0 {
			if defaultEndpoint != nil && defaultEndpoint.Timeout != 0 {
				// Inherit timeout from first endpoint
				c.Endpoints[i].Timeout = defaultEndpoint.Timeout
			} else {
				// Use global timeout setting
				c.Endpoints[i].Timeout = c.GlobalTimeout
			}
		}

		// NOTE: We do NOT inherit tokens here - tokens will be resolved dynamically at runtime
		// This allows for proper group-based token switching when groups fail

		// Inherit api-key from first endpoint if not specified
		if c.Endpoints[i].ApiKey == "" && defaultEndpoint != nil && defaultEndpoint.ApiKey != "" {
			c.Endpoints[i].ApiKey = defaultEndpoint.ApiKey
		}

		// Inherit headers from first endpoint if not specified
		if len(c.Endpoints[i].Headers) == 0 && defaultEndpoint != nil && len(defaultEndpoint.Headers) > 0 {
			// Copy headers from first endpoint
			c.Endpoints[i].Headers = make(map[string]string)
			for key, value := range defaultEndpoint.Headers {
				c.Endpoints[i].Headers[key] = value
			}
		} else if len(c.Endpoints[i].Headers) > 0 && defaultEndpoint != nil && len(defaultEndpoint.Headers) > 0 {
			// Merge headers: inherit from first endpoint, but allow override
			mergedHeaders := make(map[string]string)

			// First, copy all headers from the first endpoint
			for key, value := range defaultEndpoint.Headers {
				mergedHeaders[key] = value
			}

			// Then, override with endpoint-specific headers
			for key, value := range c.Endpoints[i].Headers {
				mergedHeaders[key] = value
			}

			c.Endpoints[i].Headers = mergedHeaders
		}
	}
}

// ApplyPrimaryEndpoint applies primary endpoint override from command line
// Returns error if the specified endpoint is not found
func (c *Config) ApplyPrimaryEndpoint(logger *slog.Logger) error {
	if c.PrimaryEndpoint == "" {
		return nil
	}

	// Find the specified endpoint
	primaryIndex := c.findEndpointIndex(c.PrimaryEndpoint)
	if primaryIndex == -1 {
		// Create list of available endpoints for better error message
		var availableEndpoints []string
		for _, endpoint := range c.Endpoints {
			availableEndpoints = append(availableEndpoints, endpoint.Name)
		}

		err := fmt.Errorf("指定的主端点 '%s' 未找到，可用端点: %v", c.PrimaryEndpoint, availableEndpoints)
		if logger != nil {
			logger.Error(fmt.Sprintf("❌ 主端点设置失败 - 端点: %s, 可用端点: %v",
				c.PrimaryEndpoint, availableEndpoints))
		}
		return err
	}

	// Store original priority for logging
	originalPriority := c.Endpoints[primaryIndex].Priority

	// Set the primary endpoint to priority 1
	c.Endpoints[primaryIndex].Priority = 1

	// Adjust other endpoints' priorities to ensure they are lower than primary
	adjustedCount := 0
	for i := range c.Endpoints {
		if i != primaryIndex && c.Endpoints[i].Priority <= 1 {
			c.Endpoints[i].Priority = c.Endpoints[i].Priority + 2 // Use consistent increment
			adjustedCount++
		}
	}

	if logger != nil {
		logger.Info(fmt.Sprintf("✅ 主端点优先级设置成功 - 端点: %s, 原优先级: %d → 新优先级: %d, 调整了%d个其他端点",
			c.PrimaryEndpoint, originalPriority, 1, adjustedCount))
	}

	return nil
}

// findEndpointIndex finds the index of an endpoint by name
func (c *Config) findEndpointIndex(name string) int {
	for i, endpoint := range c.Endpoints {
		if endpoint.Name == name {
			return i
		}
	}
	return -1
}

// validate validates the configuration
func (c *Config) validate() error {
	// v5.0+: 当使用 SQLite 存储端点时，允许 YAML 中 endpoints 为空
	// 端点会从数据库加载，不再要求 YAML 必须配置
	if len(c.Endpoints) == 0 && c.EndpointsStorage.Type != "sqlite" {
		return fmt.Errorf("at least one endpoint must be configured (or set endpoints_storage.type: sqlite)")
	}

	if c.Strategy.Type != "priority" && c.Strategy.Type != "fastest" {
		return fmt.Errorf("strategy type must be 'priority' or 'fastest'")
	}

	// Validate proxy configuration
	if c.Proxy.Enabled {
		if c.Proxy.Type == "" {
			return fmt.Errorf("proxy type is required when proxy is enabled")
		}
		if c.Proxy.Type != "http" && c.Proxy.Type != "https" && c.Proxy.Type != "socks5" {
			return fmt.Errorf("proxy type must be 'http', 'https', or 'socks5'")
		}
		if c.Proxy.URL == "" && (c.Proxy.Host == "" || c.Proxy.Port == 0) {
			return fmt.Errorf("proxy URL or host:port must be specified when proxy is enabled")
		}
	}

	if c.RequestBodyMaxBytes < 0 {
		return fmt.Errorf("request body max bytes cannot be negative")
	}

	// Validate usage tracking configuration
	if c.UsageTracking.Enabled {
		if c.UsageTracking.DatabasePath == "" {
			return fmt.Errorf("database path is required when usage tracking is enabled")
		}
		if c.UsageTracking.BufferSize <= 0 {
			return fmt.Errorf("buffer size must be greater than 0 when usage tracking is enabled")
		}
		if c.UsageTracking.BatchSize <= 0 {
			return fmt.Errorf("batch size must be greater than 0 when usage tracking is enabled")
		}
		if c.UsageTracking.BatchSize > c.UsageTracking.BufferSize {
			return fmt.Errorf("batch size cannot be larger than buffer size")
		}
		if c.UsageTracking.FlushInterval <= 0 {
			return fmt.Errorf("flush interval must be greater than 0 when usage tracking is enabled")
		}
		if c.UsageTracking.MaxRetry <= 0 {
			return fmt.Errorf("max retry count must be greater than 0 when usage tracking is enabled")
		}
		if c.UsageTracking.RetentionDays < 0 {
			return fmt.Errorf("retention days cannot be negative")
		}
		if c.UsageTracking.CleanupInterval <= 0 && c.UsageTracking.RetentionDays > 0 {
			return fmt.Errorf("cleanup interval must be greater than 0 when retention is enabled")
		}
	}

	for i, endpoint := range c.Endpoints {
		if endpoint.Name == "" {
			return fmt.Errorf("endpoint %d: name is required", i)
		}
		if endpoint.URL == "" {
			return fmt.Errorf("endpoint %s: URL is required", endpoint.Name)
		}
		if endpoint.Priority < 0 {
			return fmt.Errorf("endpoint %s: priority must be non-negative", endpoint.Name)
		}
		// 验证 token 和 tokens 互斥
		if endpoint.Token != "" && len(endpoint.Tokens) > 0 {
			return fmt.Errorf("endpoint %s: 'token' 和 'tokens' 不能同时配置，请选择其一", endpoint.Name)
		}
		// 验证 api-key 和 api-keys 互斥
		if endpoint.ApiKey != "" && len(endpoint.ApiKeys) > 0 {
			return fmt.Errorf("endpoint %s: 'api-key' 和 'api-keys' 不能同时配置，请选择其一", endpoint.Name)
		}
		// 验证 tokens 配置中每个项必须有 value
		for j, token := range endpoint.Tokens {
			if token.Value == "" {
				return fmt.Errorf("endpoint %s: tokens[%d] 必须设置 value", endpoint.Name, j)
			}
		}
		// 验证 api-keys 配置中每个项必须有 value
		for j, apiKey := range endpoint.ApiKeys {
			if apiKey.Value == "" {
				return fmt.Errorf("endpoint %s: api-keys[%d] 必须设置 value", endpoint.Name, j)
			}
		}
	}

	return nil
}

// ConfigWatcher handles automatic configuration reloading
type ConfigWatcher struct {
	configPath    string
	config        *Config
	mutex         sync.RWMutex
	watcher       *fsnotify.Watcher
	logger        *slog.Logger
	callbacks     []func(*Config)
	lastModTime   time.Time
	debounceTimer *time.Timer
}

// NewConfigWatcher creates a new configuration watcher
func NewConfigWatcher(configPath string, logger *slog.Logger) (*ConfigWatcher, error) {
	// Load initial configuration
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load initial config: %w", err)
	}

	// Get initial modification time
	fileInfo, err := os.Stat(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Create file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	cw := &ConfigWatcher{
		configPath:  configPath,
		config:      config,
		watcher:     watcher,
		logger:      logger,
		callbacks:   make([]func(*Config), 0),
		lastModTime: fileInfo.ModTime(),
	}

	// Add config file to watcher
	if err := watcher.Add(configPath); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch config file: %w", err)
	}

	// Start watching in background
	go cw.watchLoop()

	return cw, nil
}

// GetConfig returns the current configuration (thread-safe)
func (cw *ConfigWatcher) GetConfig() *Config {
	cw.mutex.RLock()
	defer cw.mutex.RUnlock()
	return cw.config
}

// UpdateLogger updates the logger used by the config watcher
func (cw *ConfigWatcher) UpdateLogger(logger *slog.Logger) {
	cw.mutex.Lock()
	defer cw.mutex.Unlock()
	cw.logger = logger
}

// AddReloadCallback adds a callback function that will be called when config is reloaded
func (cw *ConfigWatcher) AddReloadCallback(callback func(*Config)) {
	cw.mutex.Lock()
	defer cw.mutex.Unlock()
	cw.callbacks = append(cw.callbacks, callback)
}

// watchLoop monitors the config file for changes
func (cw *ConfigWatcher) watchLoop() {
	for {
		select {
		case event, ok := <-cw.watcher.Events:
			if !ok {
				return
			}

			// Handle file write events
			if event.Has(fsnotify.Write) {
				// Check if file was actually modified by comparing modification time
				fileInfo, err := os.Stat(cw.configPath)
				if err != nil {
					cw.logger.Warn(fmt.Sprintf("⚠️ 无法获取配置文件信息: %v", err))
					continue
				}

				// Skip if modification time hasn't changed
				if !fileInfo.ModTime().After(cw.lastModTime) {
					continue
				}

				cw.lastModTime = fileInfo.ModTime()

				// Cancel any existing debounce timer
				if cw.debounceTimer != nil {
					cw.debounceTimer.Stop()
				}

				// Set up debounce timer to avoid multiple rapid reloads
				cw.debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
					cw.logger.Info(fmt.Sprintf("🔄 检测到配置文件变更，正在重新加载... - 文件: %s", event.Name))
					if err := cw.reloadConfig(); err != nil {
						cw.logger.Error(fmt.Sprintf("❌ 配置文件重新加载失败: %v", err))
					} else {
						cw.logger.Info("✅ 配置文件重新加载成功")
					}
				})
			}

			// Handle file rename/remove events (some editors rename files during save)
			if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				// Re-add the file to watcher in case it was recreated
				time.Sleep(100 * time.Millisecond) // Give time for the file to be recreated
				if _, err := os.Stat(cw.configPath); err == nil {
					cw.watcher.Add(cw.configPath)
					cw.logger.Info(fmt.Sprintf("🔄 重新监听配置文件: %s", cw.configPath))
				}
			}

		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return
			}
			cw.logger.Error(fmt.Sprintf("⚠️ 配置文件监听错误: %v", err))
		}
	}
}

// reloadConfig reloads the configuration from file
func (cw *ConfigWatcher) reloadConfig() error {
	newConfig, err := LoadConfig(cw.configPath)
	if err != nil {
		return err
	}

	cw.mutex.Lock()
	oldConfig := cw.config
	cw.config = newConfig
	callbacks := make([]func(*Config), len(cw.callbacks))
	copy(callbacks, cw.callbacks)
	cw.mutex.Unlock()

	// Call all registered callbacks
	for _, callback := range callbacks {
		callback(newConfig)
	}

	// Log configuration changes
	cw.logConfigChanges(oldConfig, newConfig)

	return nil
}

// logConfigChanges logs the key differences between old and new configurations
func (cw *ConfigWatcher) logConfigChanges(oldConfig, newConfig *Config) {
	if len(oldConfig.Endpoints) != len(newConfig.Endpoints) {
		cw.logger.Info("📡 端点数量变更",
			"old_count", len(oldConfig.Endpoints),
			"new_count", len(newConfig.Endpoints))
	}

	if oldConfig.Server.Port != newConfig.Server.Port {
		cw.logger.Info("🌐 服务器端口变更",
			"old_port", oldConfig.Server.Port,
			"new_port", newConfig.Server.Port)
	}

	if oldConfig.Strategy.Type != newConfig.Strategy.Type {
		cw.logger.Info("🎯 策略类型变更",
			"old_strategy", oldConfig.Strategy.Type,
			"new_strategy", newConfig.Strategy.Type)
	}

	if oldConfig.Auth.Enabled != newConfig.Auth.Enabled {
		cw.logger.Info("🔐 鉴权状态变更",
			"old_enabled", oldConfig.Auth.Enabled,
			"new_enabled", newConfig.Auth.Enabled)
	}

	if oldConfig.UsageTracking.Enabled != newConfig.UsageTracking.Enabled {
		cw.logger.Info("📊 使用跟踪状态变更",
			"old_enabled", oldConfig.UsageTracking.Enabled,
			"new_enabled", newConfig.UsageTracking.Enabled)
	}

	if oldConfig.UsageTracking.RetentionDays != newConfig.UsageTracking.RetentionDays {
		cw.logger.Info("📊 使用跟踪数据保留天数变更",
			"old_retention", oldConfig.UsageTracking.RetentionDays,
			"new_retention", newConfig.UsageTracking.RetentionDays)
	}

	if oldConfig.Timezone != newConfig.Timezone {
		cw.logger.Info("🌍 全局时区配置变更",
			"old_timezone", oldConfig.Timezone,
			"new_timezone", newConfig.Timezone)
	}
}

// Close stops the configuration watcher
func (cw *ConfigWatcher) Close() error {
	// Cancel any pending debounce timer
	if cw.debounceTimer != nil {
		cw.debounceTimer.Stop()
	}
	return cw.watcher.Close()
}

// SaveConfig saves configuration to file
func SaveConfig(config *Config, path string) error {
	// Marshal config to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// SaveConfigWithComments saves configuration to file while preserving all comments
func SavePriorityConfigWithComments(config *Config, path string) error {
	// Read existing file to preserve comments
	yamlFile, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing config file: %w", err)
	}

	var rootNode yaml.Node
	if len(yamlFile) > 0 {
		// Decode existing YAML to preserve structure and comments
		if err := yaml.Unmarshal(yamlFile, &rootNode); err != nil {
			return fmt.Errorf("failed to decode existing YAML: %w", err)
		}
	} else {
		// Create new YAML structure if file doesn't exist
		rootNode = yaml.Node{}
		if err := rootNode.Encode(config); err != nil {
			return fmt.Errorf("failed to create new YAML structure: %w", err)
		}
	}

	// Update endpoint priorities in the YAML node tree
	if len(rootNode.Content) > 0 {
		mappingNode := rootNode.Content[0]

		// Find endpoints section
		for i := 0; i < len(mappingNode.Content); i += 2 {
			keyNode := mappingNode.Content[i]
			valueNode := mappingNode.Content[i+1]

			if keyNode.Value == "endpoints" {
				// Update each endpoint's priority
				for _, endpointNode := range valueNode.Content {
					var endpointName string
					var priorityNode *yaml.Node

					// Find name and priority nodes for this endpoint
					for j := 0; j < len(endpointNode.Content); j += 2 {
						fieldKey := endpointNode.Content[j]
						fieldValue := endpointNode.Content[j+1]

						if fieldKey.Value == "name" {
							endpointName = fieldValue.Value
						} else if fieldKey.Value == "priority" {
							priorityNode = fieldValue
						}
					}

					// Find the corresponding endpoint in config and update priority
					if endpointName != "" && priorityNode != nil {
						for _, endpoint := range config.Endpoints {
							if endpoint.Name == endpointName {
								priorityNode.Value = fmt.Sprintf("%d", endpoint.Priority)
								break
							}
						}
					}
				}
				break
			}
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Directly write to the original file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	// Encode with comments
	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2)
	if err := encoder.Encode(&rootNode); err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	return nil
}

// getConfigAppDataDir 获取应用数据目录（跨平台）
// 复制自 internal/utils/appdir.go，避免循环依赖
// Windows: %APPDATA%\AI-Switchboard
// macOS: ~/Library/Application Support/AI-Switchboard
// Linux: ~/.local/share/ai-switchboard
func getConfigAppDataDir() string {
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

// resolveEOFRetryHint 计算 EOFRetryHint 生效值（v7 D2 迁移）：
// 新键 streaming.eof_retry_hint 非 nil 优先；否则回退旧键 request_suspend.eof_retry_hint；均未配置为 false。
func (c *Config) resolveEOFRetryHint() {
	switch {
	case c.Streaming.EOFRetryHintRaw != nil:
		c.Streaming.EOFRetryHint = *c.Streaming.EOFRetryHintRaw
	case c.RequestSuspend.EOFRetryHint != nil:
		c.Streaming.EOFRetryHint = *c.RequestSuspend.EOFRetryHint
	default:
		c.Streaming.EOFRetryHint = false
	}
}
