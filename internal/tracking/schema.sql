-- Usage Tracking Database Schema
-- 使用跟踪系统数据库结构
-- 创建时间: 2025-09-04

-- 请求记录主表
CREATE TABLE IF NOT EXISTS request_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT UNIQUE NOT NULL,        -- req-xxxxxxxx
    
    -- 请求基本信息
    client_ip TEXT,                         -- 客户端IP
    user_agent TEXT,                        -- 客户端User-Agent
    method TEXT DEFAULT 'POST',             -- HTTP方法
    path TEXT DEFAULT '/v1/messages',       -- 请求路径
    
    -- 时间信息
    start_time DATETIME NOT NULL,           -- 请求开始时间
    end_time DATETIME,                      -- 请求完成时间
    duration_ms INTEGER,                    -- 总耗时(毫秒)
    first_token_ms INTEGER,                 -- 首个可见文本输出耗时(毫秒，仅流式请求)
    
    -- 转发信息
    channel TEXT DEFAULT '',                -- 渠道标签（来自端点配置）
    endpoint_name TEXT,                     -- 使用的端点名称
    group_name TEXT,                        -- 所属组名
    model_name TEXT,                        -- Claude模型名称
    upstream_type TEXT DEFAULT 'endpoint',  -- 上游来源类型: endpoint/account
    upstream_source_name TEXT DEFAULT '',   -- 上游来源名称（账号链路用订阅源）
    upstream_name TEXT DEFAULT '',          -- 上游名称（账号名或端点名）
    upstream_id INTEGER,                    -- 上游ID（账号ID，可空）
    is_streaming BOOLEAN DEFAULT FALSE,     -- 是否为流式请求

    -- Claude 端点路由诊断信息 (V1.1)
    route_mode TEXT DEFAULT 'auto',          -- auto/manual_preferred/manual_fixed
    requested_endpoint TEXT DEFAULT '',      -- 用户手动指定的端点
    effective_endpoint TEXT DEFAULT '',      -- 本次实际选中的端点
    fallback_reason TEXT DEFAULT '',         -- manual_preferred fallback 或 manual_fixed 阻塞原因
    route_decision_at DATETIME,              -- 路由决策时间
    
    -- 状态信息 (v3.5.0更新: 生命周期状态与错误原因分离 - 2025-09-28)
    status TEXT NOT NULL DEFAULT 'pending', -- 生命周期状态: pending/forwarding/processing/retry/suspended/completed/failed/cancelled
    http_status_code INTEGER,               -- HTTP状态码
    retry_count INTEGER DEFAULT 0,          -- 重试次数

    -- 失败信息 (状态机重构新增字段 v3.5.0 - 2025-09-28)
    failure_reason TEXT,                    -- 当前失败原因类型: rate_limited/server_error/network_error/timeout/empty_response/invalid_response
    last_failure_reason TEXT,               -- 最后一次失败的详细错误信息

    -- 取消信息 (状态机重构新增字段 v3.5.0 - 2025-09-28)
    cancel_reason TEXT,                     -- 取消原因 (取消时间使用end_time字段)
    
    -- Token统计
    input_tokens INTEGER DEFAULT 0,        -- 输入token数
    output_tokens INTEGER DEFAULT 0,       -- 输出token数
    cache_creation_tokens INTEGER DEFAULT 0, -- 缓存创建token数（总计，向后兼容）
    cache_creation_5m_tokens INTEGER DEFAULT 0, -- 5分钟缓存创建token数 (v5.0.1+)
    cache_creation_1h_tokens INTEGER DEFAULT 0, -- 1小时缓存创建token数 (v5.0.1+)
    cache_read_tokens INTEGER DEFAULT 0,   -- 缓存读取token数

    -- 成本计算（包含缓存）
    input_cost_usd REAL DEFAULT 0,         -- 输入token成本
    output_cost_usd REAL DEFAULT 0,        -- 输出token成本
    cache_creation_cost_usd REAL DEFAULT 0, -- 缓存创建成本（总计，向后兼容）
    cache_creation_5m_cost_usd REAL DEFAULT 0, -- 5分钟缓存创建成本 (v5.0.1+)
    cache_creation_1h_cost_usd REAL DEFAULT 0, -- 1小时缓存创建成本 (v5.0.1+)
    cache_read_cost_usd REAL DEFAULT 0,    -- 缓存读取成本
    total_cost_usd REAL DEFAULT 0,         -- 总成本
    
    -- 审计字段（统一使用带时区格式，微秒精度）
    created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
    updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
);

-- 索引优化
CREATE INDEX IF NOT EXISTS idx_request_logs_request_id ON request_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_start_time ON request_logs(start_time);
CREATE INDEX IF NOT EXISTS idx_request_logs_status ON request_logs(status);
CREATE INDEX IF NOT EXISTS idx_request_logs_model ON request_logs(model_name);
CREATE INDEX IF NOT EXISTS idx_request_logs_channel ON request_logs(channel);
CREATE INDEX IF NOT EXISTS idx_request_logs_endpoint ON request_logs(endpoint_name);
CREATE INDEX IF NOT EXISTS idx_request_logs_group ON request_logs(group_name);
CREATE INDEX IF NOT EXISTS idx_request_logs_failure_reason ON request_logs(failure_reason);

-- 使用统计汇总表 (可选，用于快速查询)
CREATE TABLE IF NOT EXISTS usage_summary (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,                    -- YYYY-MM-DD
    model_name TEXT NOT NULL,
    endpoint_name TEXT NOT NULL,
    group_name TEXT,
    
    request_count INTEGER DEFAULT 0,       -- 请求总数
    success_count INTEGER DEFAULT 0,       -- 成功请求数
    error_count INTEGER DEFAULT 0,         -- 失败请求数
    
    total_input_tokens INTEGER DEFAULT 0,
    total_output_tokens INTEGER DEFAULT 0,
    total_cache_creation_tokens INTEGER DEFAULT 0,
    total_cache_read_tokens INTEGER DEFAULT 0,
    total_cost_usd REAL DEFAULT 0,
    
    avg_duration_ms REAL DEFAULT 0,        -- 平均响应时间
    
    created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
    updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
    
    UNIQUE(date, model_name, endpoint_name, group_name)
);

-- 汇总表索引
CREATE INDEX IF NOT EXISTS idx_usage_summary_date ON usage_summary(date);
CREATE INDEX IF NOT EXISTS idx_usage_summary_model ON usage_summary(model_name);
CREATE INDEX IF NOT EXISTS idx_usage_summary_endpoint ON usage_summary(endpoint_name);
CREATE INDEX IF NOT EXISTS idx_usage_summary_group ON usage_summary(group_name);

-- 触发器：自动更新 updated_at 时间戳（统一使用带时区格式，微秒精度）
CREATE TRIGGER IF NOT EXISTS update_request_logs_timestamp
    AFTER UPDATE ON request_logs
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE request_logs SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_usage_summary_timestamp
    AFTER UPDATE ON usage_summary
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE usage_summary SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

-- ============================================================================
-- 端点配置表 (v5.0.0 新增 - 2025-12-05)
-- 将端点配置从 config.yaml 迁移到 SQLite，支持动态增删改查
-- ============================================================================
CREATE TABLE IF NOT EXISTS endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,

    -- ========== 基本信息 ==========
    channel TEXT NOT NULL,                          -- 渠道标签（用于分组展示）
    name TEXT UNIQUE NOT NULL,                      -- 端点唯一名称
    url TEXT NOT NULL,                              -- 端点 URL

    -- ========== 认证配置 ==========
    token TEXT,                                     -- Bearer Token
    api_key TEXT,                                   -- API Key (备用)
    headers TEXT,                                   -- 自定义请求头 (JSON格式)

    -- ========== 路由配置 ==========
    priority INTEGER DEFAULT 1,                     -- 优先级（数字越小越高）
    failover_enabled INTEGER DEFAULT 1,             -- 是否参与故障转移 (1=是, 0=否)
    cooldown_seconds INTEGER,                       -- 冷却时间（秒，NULL=使用全局配置）
    timeout_seconds INTEGER DEFAULT 300,            -- 请求超时（秒）

    -- ========== 功能支持 ==========
    supports_count_tokens INTEGER DEFAULT 0,        -- 是否支持 count_tokens 端点

    -- ========== 成本倍率 ==========
    cost_multiplier REAL DEFAULT 1.0,               -- 总成本倍率
    input_cost_multiplier REAL DEFAULT 1.0,         -- 输入成本倍率
    output_cost_multiplier REAL DEFAULT 1.0,        -- 输出成本倍率
    cache_creation_cost_multiplier REAL DEFAULT 1.0,-- 5分钟缓存创建成本倍率（默认）
    cache_creation_cost_multiplier_1h REAL DEFAULT 1.0, -- 1小时缓存创建成本倍率
    cache_read_cost_multiplier REAL DEFAULT 1.0,    -- 缓存读取成本倍率

    -- ========== 状态 ==========
    enabled INTEGER DEFAULT 1,                      -- 是否启用 (1=启用, 0=禁用)

    -- ========== 审计字段 ==========
    created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
    updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
);

-- 端点表索引
CREATE INDEX IF NOT EXISTS idx_endpoints_channel ON endpoints(channel);
CREATE INDEX IF NOT EXISTS idx_endpoints_priority ON endpoints(priority);
CREATE INDEX IF NOT EXISTS idx_endpoints_enabled ON endpoints(enabled);
CREATE INDEX IF NOT EXISTS idx_endpoints_failover ON endpoints(failover_enabled);

-- 端点表触发器：自动更新 updated_at
CREATE TRIGGER IF NOT EXISTS update_endpoints_timestamp
    AFTER UPDATE ON endpoints
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE endpoints SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

-- ============================================================================
-- 模型定价表 (v5.0.0 新增 - 2025-12-06, 更新于 2025-12-06 支持 5m/1h 缓存)
-- 将模型定价从 config.yaml 迁移到 SQLite，支持动态管理
-- ============================================================================
CREATE TABLE IF NOT EXISTS model_pricing (
    id INTEGER PRIMARY KEY AUTOINCREMENT,

    -- ========== 模型信息 ==========
    model_name TEXT UNIQUE NOT NULL,                -- 模型名称（如 claude-sonnet-4-20250514）

    -- ========== 定价信息 (USD per 1M tokens) ==========
    input_price REAL NOT NULL DEFAULT 3.0,          -- 输入价格
    output_price REAL NOT NULL DEFAULT 15.0,        -- 输出价格
    cache_creation_price_5m REAL DEFAULT 3.75,      -- 5分钟缓存创建价格 (通常为 input * 1.25)
    cache_creation_price_1h REAL DEFAULT 6.0,       -- 1小时缓存创建价格 (通常为 input * 2.0)
    cache_read_price REAL DEFAULT 0.30,             -- 缓存读取价格 (通常为 input * 0.1)

    -- ========== 模型元信息 ==========
    display_name TEXT,                              -- 显示名称（用于前端展示）
    description TEXT,                               -- 模型描述
    is_default INTEGER DEFAULT 0,                   -- 是否为默认定价 (1=是)

    -- ========== 审计字段 ==========
    created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
    updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
);

-- 模型定价表索引
CREATE INDEX IF NOT EXISTS idx_model_pricing_model_name ON model_pricing(model_name);
CREATE INDEX IF NOT EXISTS idx_model_pricing_is_default ON model_pricing(is_default);

-- 模型定价表触发器：自动更新 updated_at
CREATE TRIGGER IF NOT EXISTS update_model_pricing_timestamp
    AFTER UPDATE ON model_pricing
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE model_pricing SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

-- ============================================================================
-- 系统设置表 (v5.1.0 新增 - 2025-12-08)
-- 将运行时可调配置从 config.yaml 迁移到 SQLite，支持动态管理和热更新
-- ============================================================================
CREATE TABLE IF NOT EXISTS settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,

    -- ========== 设置标识 ==========
    category TEXT NOT NULL,                         -- 分类: strategy, retry, health, failover, request, streaming, auth, token_counting, retention, hot_pool, server
    key TEXT NOT NULL,                              -- 配置键

    -- ========== 设置值 ==========
    value TEXT NOT NULL,                            -- 值 (JSON格式支持复杂类型)
    value_type TEXT DEFAULT 'string',               -- 类型: string, int, float, bool, duration, json

    -- ========== 显示信息 ==========
    label TEXT,                                     -- 显示名称
    description TEXT,                               -- 配置说明
    display_order INTEGER DEFAULT 0,                -- 显示顺序（同分类内排序）

    -- ========== 元信息 ==========
    requires_restart INTEGER DEFAULT 0,             -- 是否需要重启生效 (1=需要, 0=立即生效)

    -- ========== 审计字段 ==========
    created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
    updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),

    UNIQUE(category, key)
);

-- 设置表索引
CREATE INDEX IF NOT EXISTS idx_settings_category ON settings(category);
CREATE INDEX IF NOT EXISTS idx_settings_category_key ON settings(category, key);

-- 设置表触发器：自动更新 updated_at
CREATE TRIGGER IF NOT EXISTS update_settings_timestamp
    AFTER UPDATE ON settings
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE settings SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

-- ============================================================================
-- 上游账号表
-- 账号池中的可调度账号（纯手动维护，无订阅源概念）
-- ============================================================================
CREATE TABLE IF NOT EXISTS upstream_accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,

    provider_type TEXT NOT NULL DEFAULT 'api_key', -- api_key / session_cookie
    account_name TEXT NOT NULL,                     -- 账号展示名
    credential_raw TEXT NOT NULL,                   -- 凭据（首版明文）
    base_url TEXT NOT NULL DEFAULT 'https://api.openai.com', -- 上游基础URL
    model_rewrite_rules TEXT DEFAULT '',            -- Codex 模型兼容改写规则 JSON

    -- ========== 成本倍率 ==========
    cost_multiplier REAL DEFAULT 1.0,               -- 总成本倍率
    input_cost_multiplier REAL DEFAULT 1.0,         -- 输入成本倍率
    output_cost_multiplier REAL DEFAULT 1.0,        -- 输出成本倍率
    cache_creation_cost_multiplier REAL DEFAULT 1.0,-- 5分钟缓存创建成本倍率
    cache_creation_cost_multiplier_1h REAL DEFAULT 1.0, -- 1小时缓存创建成本倍率
    cache_read_cost_multiplier REAL DEFAULT 1.0,    -- 缓存读取成本倍率

    -- ========== 调度 ==========
    group_key TEXT DEFAULT '',                     -- 显式组别 primary/backup/cold
    priority INTEGER DEFAULT 100,                   -- 优先级（数字越小越高）
    enabled INTEGER DEFAULT 1,                      -- 是否可参与调度
    state TEXT DEFAULT 'active',                    -- active/cooldown/disabled_auth
    cooldown_until DATETIME,                        -- 冷却截止时间
    fail_count INTEGER DEFAULT 0,                   -- 失败次数

    -- ========== 状态 ==========
    last_success_at DATETIME,                       -- 最近成功时间
    last_error TEXT DEFAULT '',                     -- 最近错误
    plan_type TEXT DEFAULT '',                      -- 账号类型画像
    chatgpt_account_id TEXT DEFAULT '',            -- ChatGPT 账号ID
    chatgpt_user_id TEXT DEFAULT '',               -- ChatGPT 用户ID
    organization_id TEXT DEFAULT '',               -- 组织ID
    quota_5h_used_percent REAL,                    -- 5小时窗口已用百分比
    quota_5h_reset_at DATETIME,                    -- 5小时窗口重置时间
    quota_weekly_used_percent REAL,                -- 周窗口已用百分比
    quota_weekly_reset_at DATETIME,                -- 周窗口重置时间
    quota_status TEXT DEFAULT '',                  -- quota 状态
    quota_refreshed_at DATETIME,                   -- 最近 quota 刷新时间
    fingerprint TEXT UNIQUE NOT NULL,               -- 去重指纹

    -- ========== 审计字段 ==========
    created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
    updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
);

CREATE INDEX IF NOT EXISTS idx_upstream_accounts_priority ON upstream_accounts(priority);
CREATE INDEX IF NOT EXISTS idx_upstream_accounts_enabled ON upstream_accounts(enabled);
CREATE INDEX IF NOT EXISTS idx_upstream_accounts_state ON upstream_accounts(state);
CREATE INDEX IF NOT EXISTS idx_upstream_accounts_cooldown_until ON upstream_accounts(cooldown_until);

CREATE TRIGGER IF NOT EXISTS update_upstream_accounts_timestamp
    AFTER UPDATE ON upstream_accounts
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE upstream_accounts SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

-- ============================================================================
-- 隐私保护 (v5.3.0 新增 - 2026-06-11)
-- 出站请求隐私规则与全局设置；mode 是全局模式唯一真值来源
-- ============================================================================
CREATE TABLE IF NOT EXISTS privacy_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    mode TEXT NOT NULL DEFAULT 'disabled',          -- disabled / detect / redact
    scan_max_bytes INTEGER NOT NULL DEFAULT 4194304,-- 单请求累计扫描文本字节上限
    over_limit_action TEXT NOT NULL DEFAULT 'scan_prefix', -- scan_prefix / fail_closed
    on_error TEXT NOT NULL DEFAULT 'fail_open',     -- fail_open / fail_closed
    updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
);

INSERT OR IGNORE INTO privacy_settings (id) VALUES (1);

CREATE TABLE IF NOT EXISTS privacy_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    priority INTEGER NOT NULL DEFAULT 100,          -- 数字越小优先级越高
    match_type TEXT NOT NULL,                       -- literal / regex / builtin
    pattern TEXT NOT NULL,
    placeholder TEXT NOT NULL DEFAULT '[已脱敏]',
    action TEXT NOT NULL DEFAULT 'redact',          -- detect / redact
    scope_json TEXT NOT NULL DEFAULT '{}',          -- 作用域 JSON
    source TEXT NOT NULL DEFAULT 'custom',          -- custom / preset / builtin
    compile_error TEXT DEFAULT '',                  -- 历史规则编译失败信息
    created_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00'),
    updated_at DATETIME DEFAULT (strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00')
);

CREATE INDEX IF NOT EXISTS idx_privacy_rules_enabled_priority
ON privacy_rules(enabled, priority);

INSERT INTO privacy_rules (
    enabled, name, description, priority, match_type, pattern, placeholder, action, scope_json, source
)
SELECT TRUE, '中国身份证号', '18 位格式、合法出生日期与校验位', 20, 'builtin', 'builtin:cn_id_card', '[身份证]', 'redact', '{}', 'builtin'
WHERE NOT EXISTS (
    SELECT 1 FROM privacy_rules WHERE source = 'builtin' AND pattern = 'builtin:cn_id_card'
);

INSERT INTO privacy_rules (
    enabled, name, description, priority, match_type, pattern, placeholder, action, scope_json, source
)
SELECT TRUE, '银行卡号', '13-19 位数字并通过 Luhn 校验', 30, 'builtin', 'builtin:bank_card_luhn', '[银行卡]', 'redact', '{}', 'builtin'
WHERE NOT EXISTS (
    SELECT 1 FROM privacy_rules WHERE source = 'builtin' AND pattern = 'builtin:bank_card_luhn'
);

INSERT INTO privacy_rules (
    enabled, name, description, priority, match_type, pattern, placeholder, action, scope_json, source
)
SELECT TRUE, '中国手机号', '中国大陆手机号，支持 +86 前缀', 40, 'builtin', 'builtin:cn_mobile', '[手机号]', 'redact', '{}', 'builtin'
WHERE NOT EXISTS (
    SELECT 1 FROM privacy_rules WHERE source = 'builtin' AND pattern = 'builtin:cn_mobile'
);

INSERT INTO privacy_rules (
    enabled, name, description, priority, match_type, pattern, placeholder, action, scope_json, source
)
SELECT TRUE, '邮箱地址', '基础邮箱格式，排除 git/ssh/scp 命令上下文', 50, 'builtin', 'builtin:email', '[邮箱]', 'redact', '{}', 'builtin'
WHERE NOT EXISTS (
    SELECT 1 FROM privacy_rules WHERE source = 'builtin' AND pattern = 'builtin:email'
);

CREATE TABLE IF NOT EXISTS privacy_exact_secrets (
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
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_privacy_exact_secrets_value_hash
ON privacy_exact_secrets(value_hash);

CREATE TRIGGER IF NOT EXISTS update_privacy_settings_timestamp
    AFTER UPDATE ON privacy_settings
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE privacy_settings SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_privacy_rules_timestamp
    AFTER UPDATE ON privacy_rules
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE privacy_rules SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_privacy_exact_secrets_timestamp
    AFTER UPDATE ON privacy_exact_secrets
    FOR EACH ROW
    WHEN NEW.updated_at = OLD.updated_at
BEGIN
    UPDATE privacy_exact_secrets SET updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now', 'localtime') || '+08:00' WHERE id = NEW.id;
END;
