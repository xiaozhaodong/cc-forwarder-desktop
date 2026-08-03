PRAGMA foreign_keys = ON;

-- Claude 端点旧表：包含待物理删除的 channel/enabled。
CREATE TABLE endpoints (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel TEXT NOT NULL,
    name TEXT UNIQUE NOT NULL,
    url TEXT NOT NULL,
    token TEXT,
    api_key TEXT,
    headers TEXT,
    priority INTEGER DEFAULT 1,
    failover_enabled INTEGER DEFAULT 1,
    cooldown_seconds INTEGER,
    timeout_seconds INTEGER DEFAULT 300,
    supports_count_tokens INTEGER DEFAULT 0,
    cost_multiplier REAL DEFAULT 1.0,
    input_cost_multiplier REAL DEFAULT 1.0,
    output_cost_multiplier REAL DEFAULT 1.0,
    cache_creation_cost_multiplier REAL DEFAULT 1.0,
    cache_creation_cost_multiplier_1h REAL DEFAULT 1.0,
    cache_read_cost_multiplier REAL DEFAULT 1.0,
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    model_rewrite_rules TEXT DEFAULT '',
    availability_enabled INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX idx_endpoints_channel ON endpoints(channel);
CREATE INDEX idx_endpoints_enabled ON endpoints(enabled);
CREATE INDEX idx_endpoints_priority ON endpoints(priority);
CREATE INDEX idx_endpoints_failover ON endpoints(failover_enabled);

CREATE TABLE endpoint_runtime_states (
    endpoint_id INTEGER NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('global', 'messages', 'count_tokens')),
    state TEXT NOT NULL DEFAULT 'active',
    cooldown_until DATETIME,
    cooldown_reason TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (endpoint_id, scope),
    FOREIGN KEY (endpoint_id) REFERENCES endpoints(id) ON DELETE CASCADE
);

-- 请求旧表：包含待物理删除的 channel/group_name，保留现有统一上游字段。
CREATE TABLE request_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT UNIQUE NOT NULL,
    client_ip TEXT,
    user_agent TEXT,
    method TEXT DEFAULT 'POST',
    path TEXT DEFAULT '/v1/messages',
    start_time DATETIME NOT NULL,
    end_time DATETIME,
    duration_ms INTEGER,
    channel TEXT DEFAULT '',
    endpoint_name TEXT,
    group_name TEXT,
    model_name TEXT,
    is_streaming BOOLEAN DEFAULT FALSE,
    status TEXT NOT NULL DEFAULT 'pending',
    http_status_code INTEGER,
    retry_count INTEGER DEFAULT 0,
    failure_reason TEXT,
    last_failure_reason TEXT,
    cancel_reason TEXT,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    cache_creation_tokens INTEGER DEFAULT 0,
    cache_creation_5m_tokens INTEGER DEFAULT 0,
    cache_creation_1h_tokens INTEGER DEFAULT 0,
    cache_read_tokens INTEGER DEFAULT 0,
    input_cost_usd REAL DEFAULT 0,
    output_cost_usd REAL DEFAULT 0,
    cache_creation_cost_usd REAL DEFAULT 0,
    cache_creation_5m_cost_usd REAL DEFAULT 0,
    cache_creation_1h_cost_usd REAL DEFAULT 0,
    cache_read_cost_usd REAL DEFAULT 0,
    total_cost_usd REAL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    upstream_type TEXT DEFAULT 'endpoint',
    upstream_source_name TEXT DEFAULT '',
    upstream_name TEXT DEFAULT '',
    upstream_id INTEGER,
    first_token_ms INTEGER,
    route_mode TEXT DEFAULT 'auto',
    requested_endpoint TEXT DEFAULT '',
    effective_endpoint TEXT DEFAULT '',
    fallback_reason TEXT DEFAULT '',
    route_decision_at DATETIME,
    completion_ms INTEGER
);

CREATE INDEX idx_request_logs_channel ON request_logs(channel);
CREATE INDEX idx_request_logs_group ON request_logs(group_name);
CREATE INDEX idx_request_logs_start_time ON request_logs(start_time);
CREATE INDEX idx_request_logs_upstream_type ON request_logs(upstream_type);
CREATE INDEX idx_request_logs_upstream_name ON request_logs(upstream_name);

CREATE TABLE usage_summary (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,
    model_name TEXT NOT NULL,
    endpoint_name TEXT NOT NULL,
    group_name TEXT,
    request_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    total_input_tokens INTEGER DEFAULT 0,
    total_output_tokens INTEGER DEFAULT 0,
    total_cache_creation_tokens INTEGER DEFAULT 0,
    total_cache_read_tokens INTEGER DEFAULT 0,
    total_cost_usd REAL DEFAULT 0,
    avg_duration_ms REAL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(date, model_name, endpoint_name, group_name)
);

CREATE TABLE app_settings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    value_type TEXT NOT NULL DEFAULT 'string',
    label TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(category, key)
);

CREATE TABLE privacy_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    enabled INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    priority INTEGER NOT NULL DEFAULT 100,
    match_type TEXT NOT NULL,
    pattern TEXT NOT NULL,
    placeholder TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    scope_json TEXT NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT 'custom',
    compile_error TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO endpoints (
    id, channel, name, url, token, api_key, headers, priority,
    failover_enabled, cooldown_seconds, timeout_seconds, supports_count_tokens,
    enabled, availability_enabled, model_rewrite_rules
) VALUES
    (7, 'coderelay', 'legacy-primary', 'https://legacy-primary.example.test',
     'synthetic-db-token', 'synthetic-db-api', '{"X-Fixture":"one"}', 1,
     1, 120, 45, 1, 1, 1, '[{"enabled":true,"match_type":"exact","from":"old","to":"new"}]'),
    (8, 'anyroute', 'legacy-disabled', 'https://anyrouter.top/path',
     '', '', '{}', 5, 0, NULL, 30, 0, 0, 0, '');

INSERT INTO endpoint_runtime_states (
    endpoint_id, scope, state, cooldown_until, cooldown_reason, revision
) VALUES
    (7, 'messages', 'cooldown', '2099-08-03T12:00:00+08:00', 'synthetic_fixture', 42);

INSERT INTO app_settings (category, key, value, value_type, label, description, display_order) VALUES
    ('claude_routing', 'mode', 'manual_fixed', 'string', 'route mode', '', 1),
    ('claude_routing', 'endpoint_name', 'legacy-primary', 'string', 'endpoint', '', 2),
    ('claude_routing', 'revision', '9', 'number', 'revision', '', 3);

INSERT INTO privacy_rules (
    enabled, name, description, priority, match_type, pattern, placeholder,
    action, scope_json, source
) VALUES
    (1, 'fixture-endpoint-scope', '', 100, 'literal', 'synthetic-secret',
     '[fixture]', 'redact', '{"endpoint_names":["legacy-primary"]}', 'custom');

INSERT INTO request_logs (
    request_id, path, start_time, end_time, duration_ms, channel, endpoint_name,
    group_name, model_name, status, input_tokens, output_tokens,
    cache_creation_tokens, cache_read_tokens, total_cost_usd,
    upstream_type, upstream_name, requested_endpoint, effective_endpoint
) VALUES
    ('req-fixture-claude', '/v1/messages', '2026-08-01T10:00:00+08:00',
     '2026-08-01T10:00:01+08:00', 1000, 'coderelay', 'legacy-primary', 'primary',
     'claude-fixture', 'completed', 100, 20, 5, 10, 0.001, 'endpoint',
     'legacy-primary', 'legacy-primary', 'legacy-primary'),
    ('req-fixture-codex', '/v1/responses', '2026-08-01T11:00:00+08:00',
     '2026-08-01T11:00:02+08:00', 2000, 'account-pool', 'codex-fixture', '',
     'gpt-fixture', 'completed', 200, 30, 0, 25, 0.002, 'account',
     'codex-fixture', '', ''),
    ('req-fixture-image', '/v1/images/generations', '2026-08-01T12:00:00+08:00',
     '2026-08-01T12:00:03+08:00', 3000, 'image', 'image-fixture', '',
     'gpt-image-fixture', 'completed', 0, 0, 0, 0, 0.04, 'endpoint',
     'image-fixture', '', ''),
    ('req-fixture-other', '/health', '2026-08-01T13:00:00+08:00',
     '2026-08-01T13:00:00+08:00', 10, '', '', '', '', 'completed',
     0, 0, 0, 0, 0, '', '', '', '');

INSERT INTO usage_summary (
    date, model_name, endpoint_name, group_name, request_count, success_count,
    error_count, total_input_tokens, total_output_tokens,
    total_cache_creation_tokens, total_cache_read_tokens, total_cost_usd,
    avg_duration_ms
) VALUES
    ('2026-08-01', 'legacy-summary', 'legacy-primary', 'primary', 4, 4, 0,
     300, 50, 5, 35, 0.043, 1502.5);
