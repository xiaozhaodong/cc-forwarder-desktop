// ============================================
// Wails API 适配层
// 2025-12-04
// 将 Wails Bindings 包装为与 HTTP API 兼容的接口
// ============================================

// 检测是否在 Wails 环境中运行
export const isWailsEnvironment = () => {
  return typeof window !== 'undefined' && window.go !== undefined;
};

// 动态导入 Wails 绑定（避免在非 Wails 环境中报错）
let WailsApp = null;
let WailsRuntime = null;

const loadWailsBindings = async () => {
  if (!isWailsEnvironment()) {
    return false;
  }

  try {
    WailsApp = await import('@wailsjs/go/main/App');
    WailsRuntime = await import('@wailsjs/runtime/runtime');
    return true;
  } catch (error) {
    console.warn('Failed to load Wails bindings:', error);
    return false;
  }
};

// 初始化 Wails 绑定
let wailsInitialized = false;
let wailsInitPromise = null;

export const initWails = async () => {
  if (wailsInitialized) return true;
  if (wailsInitPromise) return wailsInitPromise;

  wailsInitPromise = loadWailsBindings().then(result => {
    wailsInitialized = result;
    return result;
  });

  return wailsInitPromise;
};

const getWailsMethod = (methodName, { optional = false } = {}) => {
  const method = WailsApp?.[methodName];
  if (typeof method === 'function') {
    return method;
  }
  const runtimeMethod = typeof window !== 'undefined' ? window?.go?.main?.App?.[methodName] : null;
  if (typeof runtimeMethod === 'function') {
    return (...args) => runtimeMethod(...args);
  }
  if (optional) {
    return null;
  }
  throw new Error(`当前后端版本暂未提供 ${methodName}，请升级后端后重试`);
};

const parseEntityId = (id) => {
  if (id === null || id === undefined) return null;
  if (typeof id === 'number') {
    return Number.isFinite(id) ? id : null;
  }
  if (typeof id === 'string') {
    const trimmed = id.trim();
    if (!trimmed) return null;
    const numeric = Number(trimmed);
    return Number.isNaN(numeric) ? trimmed : numeric;
  }
  return null;
};

const normalizeEntityId = (id) => {
  const normalized = parseEntityId(id);
  return normalized === null ? id : normalized;
};

// ============================================
// Wails Events 适配
// ============================================

// 事件监听器映射
const eventListeners = new Map();

/**
 * 订阅 Wails 事件
 * @param {string} eventName - 事件名称
 * @param {Function} callback - 回调函数
 * @returns {Function} - 取消订阅函数
 */
export const subscribeToEvent = (eventName, callback) => {
  // 异步初始化后订阅
  let unsubscribeFunc = null;
  let isUnsubscribed = false;

  console.log(`📡 [subscribeToEvent] 开始订阅事件: ${eventName}`);

  initWails().then(() => {
    if (isUnsubscribed) {
      console.log(`📡 [subscribeToEvent] 事件 ${eventName} 已取消订阅，跳过注册`);
      return;
    }

    if (!WailsRuntime) {
      console.warn(`📡 [subscribeToEvent] Wails Runtime not loaded, 无法订阅 ${eventName}`);
      return;
    }

    console.log(`📡 [subscribeToEvent] 注册事件监听: ${eventName}`);
    unsubscribeFunc = WailsRuntime.EventsOn(eventName, (data) => {
      console.log(`📡 [subscribeToEvent] 收到事件 ${eventName}:`, data);
      callback(data);
    });

    // 存储监听器以便清理
    if (!eventListeners.has(eventName)) {
      eventListeners.set(eventName, []);
    }
    eventListeners.get(eventName).push({ callback, unsubscribe: unsubscribeFunc });
  });

  // 返回取消订阅函数
  return () => {
    isUnsubscribed = true;
    if (unsubscribeFunc && typeof unsubscribeFunc === 'function') {
      unsubscribeFunc();
    }
  };
};

/**
 * 取消订阅所有事件
 */
export const unsubscribeAll = () => {
  if (!WailsRuntime) return;

  eventListeners.forEach((listeners, eventName) => {
    listeners.forEach(({ unsubscribe }) => {
      if (typeof unsubscribe === 'function') {
        unsubscribe();
      }
    });
  });
  eventListeners.clear();
};

// ============================================
// 系统状态 API
// ============================================

export const getSystemStatus = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const status = await WailsApp.GetSystemStatus();

  // 转换为前端期望的格式
  return {
    status: status.proxy_running ? 'running' : 'stopped',
    version: status.version,
    uptime: status.uptime_seconds,
    start_time: status.start_time, // ISO8601 格式的启动时间
    proxy_running: status.proxy_running,
    proxy_port: status.proxy_port,
    proxy_host: status.proxy_host,
    active_group: status.active_group,
    config_path: status.config_path,
    auth_enabled: status.auth_enabled
  };
};

export const getConfig = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  return await WailsApp.GetConfig();
};

// ============================================
// 端点管理 API
// ============================================

export const getEndpoints = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const endpoints = await WailsApp.GetEndpoints();

  // 转换为前端期望的格式
  const formattedEndpoints = endpoints.map(ep => ({
    name: ep.name,
    url: ep.url,
    channel: ep.channel || '', // v5.0: 渠道标签
    group: ep.group,
    priority: ep.priority,
    group_priority: ep.group_priority,
    group_is_active: ep.group_is_active,
    healthy: ep.healthy,
    status: ep.healthy ? 'healthy' : 'unhealthy',
    last_check: ep.last_check,
    response_time: ep.response_time_ms,
    consecutive_fail: ep.consecutive_fail,
    never_checked: ep.never_checked === true || !ep.last_check
  }));

  const healthy = formattedEndpoints.filter(e => e.healthy).length;

  return {
    endpoints: formattedEndpoints,
    total: formattedEndpoints.length,
    healthy
  };
};

export const setEndpointPriority = async (endpointName, priority) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.SetEndpointPriority(endpointName, priority);
  return { success: true };
};

export const triggerHealthCheck = async (endpointName) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.TriggerHealthCheck(endpointName);
  return { success: true, healthy: true, message: '连通性测试已完成' };
};

export const batchHealthCheckAll = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const result = await WailsApp.BatchHealthCheckAll();
  return {
    success: result.success,
    message: result.message || '批量连通性测试完成',
    total: result.total,
    healthy_count: result.healthy_count,
    unhealthy_count: result.unhealthy_count
  };
};

// ============================================
// Key 管理 API
// ============================================

export const getKeysOverview = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  console.log('🔑 [Wails] 调用 GetKeysOverview...');
  const result = await WailsApp.GetKeysOverview();
  console.log('🔑 [Wails] GetKeysOverview 原始返回:', result);

  // 转换为前端期望的格式
  const endpoints = (result.endpoints || []).map(ep => ({
    endpoint: ep.endpoint,
    tokens: (ep.tokens || []).map(t => ({
      index: t.index,
      name: t.name || `Token ${t.index + 1}`,
      masked: t.value,  // 后端返回的是 value 字段（已脱敏）
      is_active: t.is_active
    })),
    api_keys: (ep.api_keys || []).map(k => ({
      index: k.index,
      name: k.name || `API Key ${k.index + 1}`,
      masked: k.value,  // 后端返回的是 value 字段（已脱敏）
      is_active: k.is_active
    })),
    current_token_index: ep.current_token_index,
    current_api_key_index: ep.current_api_key_index
  }));

  const formatted = {
    endpoints,
    total: endpoints.length,
    timestamp: result.timestamp
  };
  console.log('🔑 [Wails] GetKeysOverview 格式化后:', formatted);
  return formatted;
};

export const switchKey = async (endpointName, keyType, index) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const result = await WailsApp.SwitchKey(endpointName, keyType, index);
  return {
    success: result.success,
    message: result.message,
    endpoint: result.endpoint,
    key_type: result.key_type,
    new_index: result.new_index
  };
};

// ============================================
// 组管理 API
// ============================================

export const getGroups = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  // 获取组、端点和 Keys 信息
  const [groups, endpointsData, keysData] = await Promise.all([
    WailsApp.GetGroups(),
    WailsApp.GetEndpoints(),
    WailsApp.GetKeysOverview().catch(() => ({ endpoints: [] })) // 容错：获取 keys 失败不影响主流程
  ]);

  // v4.0: 一个端点 = 一个组，组名 = 端点名
  // 计算每个组的最近连通性统计
  const groupHealthMap = new Map();
  endpointsData.forEach(ep => {
    // v4.0: 使用端点名作为组名（因为一个端点就是一个组）
    const groupName = ep.name;
    if (!groupHealthMap.has(groupName)) {
      groupHealthMap.set(groupName, { total: 0, healthy: 0, unchecked: 0 });
    }
    const stats = groupHealthMap.get(groupName);
    stats.total++;
    if (ep.never_checked === true || !ep.last_check) {
      stats.unchecked++;
    } else if (ep.healthy) {
      stats.healthy++;
    }
  });

  // 构建端点名到 tokens 的映射
  const endpointTokensMap = new Map();
  (keysData.endpoints || []).forEach(ep => {
    const tokens = (ep.tokens || []).map(t => ({
      index: t.index,
      name: t.name || `Token ${t.index + 1}`,
      key: t.value, // 脱敏的 key 值
      is_active: t.is_active,
      endpoint: ep.endpoint, // 关联的端点名
      type: inferTokenType(t.name) // 推断 Token 类型
    }));
    endpointTokensMap.set(ep.endpoint, tokens);
  });

  // 推断 Token 类型的辅助函数
  function inferTokenType(name) {
    if (!name) return 'Std';
    const lowerName = name.toLowerCase();
    if (lowerName.includes('pro') || lowerName.includes('特价')) return 'Pro';
    if (lowerName.includes('ent') || lowerName.includes('主号')) return 'Ent';
    if (lowerName.includes('free') || lowerName.includes('测试')) return 'Free';
    return 'Std';
  }

  // 转换为前端期望的格式
  const formattedGroups = groups.map(g => {
    const healthStats = groupHealthMap.get(g.name) || { total: 0, healthy: 0, unchecked: 0 };
    // v4.0: 组名 = 端点名，从 endpointTokensMap 获取 tokens
    const tokens = endpointTokensMap.get(g.name) || [];

    return {
      name: g.name,
      channel: g.channel,  // v5.0: 渠道名称
      is_active: g.active,
      paused: g.paused,
      priority: g.priority,
      endpoint_count: g.endpoint_count,
      total_endpoints: healthStats.total,
      healthy_endpoints: healthStats.healthy,
      unhealthy_endpoints: healthStats.total - healthStats.healthy - healthStats.unchecked,
      unchecked_endpoints: healthStats.unchecked,
      in_cooldown: g.in_cooldown,
      cooldown_remain_ms: g.cooldown_remain_ms,
      tokens: tokens // 添加 tokens 数组
    };
  });

  const activeGroup = formattedGroups.find(g => g.is_active);

  return {
    groups: formattedGroups,
    active_group: activeGroup?.name || null,
    total_suspended_requests: 0
  };
};

export const activateGroup = async (groupName) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.ActivateGroup(groupName);
  return { success: true };
};

const normalizeClaudeRoutingState = (state = {}) => ({
  mode: state.mode || state.Mode || 'auto',
  endpoint_name: state.endpoint_name || state.endpointName || state.EndpointName || '',
  endpointName: state.endpoint_name || state.endpointName || state.EndpointName || '',
  set_by: state.set_by || state.setBy || state.SetBy || '',
  setAt: state.set_at || state.setAt || state.SetAt || '',
  set_at: state.set_at || state.setAt || state.SetAt || '',
  fallback_enabled: state.fallback_enabled ?? state.fallbackEnabled ?? state.FallbackEnabled ?? true,
  fallbackEnabled: state.fallback_enabled ?? state.fallbackEnabled ?? state.FallbackEnabled ?? true,
  revision: state.revision ?? state.Revision ?? 0,
  fallback_reason: state.fallback_reason || state.fallbackReason || state.FallbackReason || '',
  fallbackReason: state.fallback_reason || state.fallbackReason || state.FallbackReason || '',
  last_effective_endpoint: state.last_effective_endpoint || state.lastEffectiveEndpoint || state.LastEffectiveEndpoint || '',
  lastEffectiveEndpoint: state.last_effective_endpoint || state.lastEffectiveEndpoint || state.LastEffectiveEndpoint || '',
  last_decision_at: state.last_decision_at || state.lastDecisionAt || state.LastDecisionAt || '',
  lastDecisionAt: state.last_decision_at || state.lastDecisionAt || state.LastDecisionAt || '',
  current_active_endpoint: state.current_active_endpoint || state.currentActiveEndpoint || state.CurrentActiveEndpoint || '',
  currentActiveEndpoint: state.current_active_endpoint || state.currentActiveEndpoint || state.CurrentActiveEndpoint || '',
  available_endpoints: state.available_endpoints || state.availableEndpoints || state.AvailableEndpoints || [],
  availableEndpoints: state.available_endpoints || state.availableEndpoints || state.AvailableEndpoints || []
});

export const getClaudeRoutingState = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('GetClaudeRoutingState');
  const state = await method();
  return normalizeClaudeRoutingState(state);
};

export const setClaudeRoutingOverride = async ({ mode, endpointName, endpoint_name, fallbackEnabled, fallback_enabled } = {}) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('SetClaudeRoutingOverride');
  const state = await method({
    mode: mode || 'auto',
    endpoint_name: endpoint_name || endpointName || '',
    fallback_enabled: fallback_enabled ?? fallbackEnabled ?? false
  });
  return normalizeClaudeRoutingState(state);
};

export const clearClaudeRoutingOverride = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ClearClaudeRoutingOverride');
  const state = await method();
  return normalizeClaudeRoutingState(state);
};

export const clearNegativeHitCache = async (endpointName = '') => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ClearNegativeHitCache');
  await method(endpointName || '');
  return { success: true };
};

export const pauseGroup = async (groupName) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.PauseGroup(groupName);
  return { success: true };
};

export const resumeGroup = async (groupName) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.ResumeGroup(groupName);
  return { success: true };
};

// ============================================
// v6.0+ 账号池 API (Wails)
// ============================================

export const parseNullableNumber = (value) => {
  if (value === null || value === undefined || value === '') {
    return null;
  }
  const numeric = Number.parseFloat(value);
  return Number.isFinite(numeric) ? numeric : null;
};

const pickAccountValue = (account, keys, fallback = '') => {
  if (!account || typeof account !== 'object') {
    return fallback;
  }

  for (const key of keys) {
    const value = account[key];
    if (value !== undefined && value !== null && value !== '') {
      return value;
    }
  }

  return fallback;
};

const mirrorAliasedField = (target, snakeKey, camelKey, value) => {
  target[snakeKey] = value;
  target[camelKey] = value;
  return target;
};

export const normalizeUpstreamAccount = (account = {}) => {
  const normalized = {
    id: parseEntityId(pickAccountValue(account, ['id', 'ID', 'Id', 'account_id', 'accountId', 'AccountID'], null)),
    priority: Number.parseInt(pickAccountValue(account, ['priority', 'Priority'], 1), 10) || 1,
    enabled: pickAccountValue(account, ['enabled', 'Enabled'], true) !== false,
    state: pickAccountValue(account, ['state', 'State'], 'active'),
    fingerprint: pickAccountValue(account, ['fingerprint', 'Fingerprint'], '')
  };

  const credentialRaw = pickAccountValue(account, ['credential_raw', 'credentialRaw', 'CredentialRaw'], '');
  const hasCredential = pickAccountValue(account, ['has_credential', 'hasCredential', 'HasCredential'], false) === true || Boolean(credentialRaw);

  [
    ['provider_type', 'providerType', pickAccountValue(account, ['provider_type', 'providerType', 'ProviderType'], '')],
    ['account_name', 'accountName', pickAccountValue(account, ['account_name', 'accountName', 'AccountName'], '')],
    ['is_active_selection', 'isActiveSelection', pickAccountValue(account, ['is_active_selection', 'isActiveSelection', 'IsActiveSelection'], false) === true],
    ['is_group_preferred', 'isGroupPreferred', pickAccountValue(account, ['is_group_preferred', 'isGroupPreferred', 'IsGroupPreferred'], false) === true],
    ['credential_raw', 'credentialRaw', credentialRaw],
    ['credential_raw_masked', 'credentialRawMasked', pickAccountValue(account, ['credential_raw_masked', 'credentialRawMasked', 'CredentialRawMasked'], credentialRaw)],
    ['has_credential', 'hasCredential', hasCredential],
    ['base_url', 'baseURL', pickAccountValue(account, ['base_url', 'baseURL', 'BaseURL'], '')],
    ['model_rewrite_rules', 'modelRewriteRules', pickAccountValue(account, ['model_rewrite_rules', 'modelRewriteRules', 'ModelRewriteRules'], '')],
    ['enable_request_compression', 'enableRequestCompression', pickAccountValue(account, ['enable_request_compression', 'enableRequestCompression', 'EnableRequestCompression'], false) === true],
    ['cost_multiplier', 'costMultiplier', Number.parseFloat(pickAccountValue(account, ['cost_multiplier', 'costMultiplier', 'CostMultiplier'], 1.0)) || 1.0],
    ['input_cost_multiplier', 'inputCostMultiplier', Number.parseFloat(pickAccountValue(account, ['input_cost_multiplier', 'inputCostMultiplier', 'InputCostMultiplier'], 1.0)) || 1.0],
    ['output_cost_multiplier', 'outputCostMultiplier', Number.parseFloat(pickAccountValue(account, ['output_cost_multiplier', 'outputCostMultiplier', 'OutputCostMultiplier'], 1.0)) || 1.0],
    ['cache_creation_cost_multiplier', 'cacheCreationCostMultiplier', Number.parseFloat(pickAccountValue(account, ['cache_creation_cost_multiplier', 'cacheCreationCostMultiplier', 'CacheCreationCostMultiplier'], 1.0)) || 1.0],
    ['cache_creation_cost_multiplier_1h', 'cacheCreationCostMultiplier1h', Number.parseFloat(pickAccountValue(account, ['cache_creation_cost_multiplier_1h', 'cacheCreationCostMultiplier1h', 'CacheCreationCostMultiplier1h'], 1.0)) || 1.0],
    ['cache_read_cost_multiplier', 'cacheReadCostMultiplier', Number.parseFloat(pickAccountValue(account, ['cache_read_cost_multiplier', 'cacheReadCostMultiplier', 'CacheReadCostMultiplier'], 1.0)) || 1.0],
    ['group_key', 'groupKey', pickAccountValue(account, ['group_key', 'groupKey', 'GroupKey'], '')],
    ['cooldown_until', 'cooldownUntil', pickAccountValue(account, ['cooldown_until', 'cooldownUntil', 'CooldownUntil'], '')],
    ['fail_count', 'failCount', pickAccountValue(account, ['fail_count', 'failCount', 'FailCount'], 0)],
    ['last_success_at', 'lastSuccessAt', pickAccountValue(account, ['last_success_at', 'lastSuccessAt', 'LastSuccessAt'], '')],
    ['last_error', 'lastError', pickAccountValue(account, ['last_error', 'lastError', 'LastError'], '')],
    ['plan_type', 'planType', pickAccountValue(account, ['plan_type', 'planType', 'PlanType'], '')],
    ['chatgpt_account_id', 'chatgptAccountId', pickAccountValue(account, ['chatgpt_account_id', 'chatgptAccountId', 'ChatGPTAccountID'], '')],
    ['chatgpt_user_id', 'chatgptUserId', pickAccountValue(account, ['chatgpt_user_id', 'chatgptUserId', 'ChatGPTUserID'], '')],
    ['organization_id', 'organizationId', pickAccountValue(account, ['organization_id', 'organizationId', 'OrganizationID'], '')],
    ['quota_5h_used_percent', 'quota5hUsedPercent', parseNullableNumber(pickAccountValue(account, ['quota_5h_used_percent', 'quota5hUsedPercent', 'Quota5HUsedPercent'], null))],
    ['quota_5h_reset_at', 'quota5hResetAt', pickAccountValue(account, ['quota_5h_reset_at', 'quota5hResetAt', 'Quota5HResetAt'], '')],
    ['quota_weekly_used_percent', 'quotaWeeklyUsedPercent', parseNullableNumber(pickAccountValue(account, ['quota_weekly_used_percent', 'quotaWeeklyUsedPercent', 'QuotaWeeklyUsedPercent'], null))],
    ['quota_weekly_reset_at', 'quotaWeeklyResetAt', pickAccountValue(account, ['quota_weekly_reset_at', 'quotaWeeklyResetAt', 'QuotaWeeklyResetAt'], '')],
    ['quota_status', 'quotaStatus', pickAccountValue(account, ['quota_status', 'quotaStatus', 'QuotaStatus'], '')],
    ['quota_refreshed_at', 'quotaRefreshedAt', pickAccountValue(account, ['quota_refreshed_at', 'quotaRefreshedAt', 'QuotaRefreshedAt'], '')],
    ['created_at', 'createdAt', pickAccountValue(account, ['created_at', 'createdAt', 'CreatedAt'], '')],
    ['updated_at', 'updatedAt', pickAccountValue(account, ['updated_at', 'updatedAt', 'UpdatedAt'], '')]
  ].forEach(([snakeKey, camelKey, value]) => {
    mirrorAliasedField(normalized, snakeKey, camelKey, value);
  });

  return normalized;
};

const normalizeModelRewriteSource = (sourceModel = '') => {
  const trimmed = String(sourceModel || '').trim();
  if (!trimmed) {
    return '';
  }
  return trimmed.endsWith('*') ? trimmed.slice(0, -1).trim() : trimmed;
};

const serializeModelRewriteRules = (value) => {
  if (typeof value === 'string') {
    return value.trim();
  }
  const rules = Array.isArray(value) ? value : [];
  const normalized = rules
    .map((rule = {}) => ({
      paths: Array.isArray(rule.paths) && rule.paths.length > 0
        ? rule.paths
        : ['/v1/responses', '/v1/responses/compact'],
      match: String(rule.match || 'exact').trim() || 'exact',
      from: normalizeModelRewriteSource(rule.from ?? rule.source ?? ''),
      to: String(rule.to ?? rule.target ?? '').trim()
    }))
    .filter((rule) => rule.from && rule.to);
  return normalized.length > 0 ? JSON.stringify(normalized) : '';
};

export const buildUpstreamAccountPayload = (input = {}) => {
  const providerType = String(input.provider_type || input.providerType || '').trim().toLowerCase();
  const isAPIKeyAccount = providerType === 'api_key';
  const normalizeMultiplier = (value) => {
    const parsed = Number.parseFloat(value);
    return Number.isFinite(parsed) && parsed > 0 ? parsed : 1.0;
  };

  return {
    ...input,
    account_name: input.account_name || input.accountName || '',
    provider_type: input.provider_type || input.providerType || '',
    credential_raw: input.credential_raw || input.credentialRaw || '',
    base_url: input.base_url || input.baseURL || '',
    model_rewrite_rules: isAPIKeyAccount ? serializeModelRewriteRules(input.model_rewrite_rules ?? input.modelRewriteRules ?? '') : '',
    enable_request_compression: isAPIKeyAccount && Boolean(input.enable_request_compression ?? input.enableRequestCompression),
    cost_multiplier: isAPIKeyAccount ? normalizeMultiplier(input.cost_multiplier ?? input.costMultiplier) : 1.0,
    input_cost_multiplier: isAPIKeyAccount ? normalizeMultiplier(input.input_cost_multiplier ?? input.inputCostMultiplier) : 1.0,
    output_cost_multiplier: isAPIKeyAccount ? normalizeMultiplier(input.output_cost_multiplier ?? input.outputCostMultiplier) : 1.0,
    cache_creation_cost_multiplier: isAPIKeyAccount ? normalizeMultiplier(input.cache_creation_cost_multiplier ?? input.cacheCreationCostMultiplier) : 1.0,
    cache_creation_cost_multiplier_1h: isAPIKeyAccount ? normalizeMultiplier(input.cache_creation_cost_multiplier_1h ?? input.cacheCreationCostMultiplier1h) : 1.0,
    cache_read_cost_multiplier: isAPIKeyAccount ? normalizeMultiplier(input.cache_read_cost_multiplier ?? input.cacheReadCostMultiplier) : 1.0,
    group_key: input.group_key || input.groupKey || '',
    priority: Number.parseInt(input.priority, 10) || 1,
    enabled: input.enabled !== false
  };
};

export const normalizeAccountScheduleCandidateDecision = (candidate = {}) => ({
  account_id: parseEntityId(candidate.account_id ?? candidate.accountId ?? candidate.AccountID ?? null),
  accountId: parseEntityId(candidate.account_id ?? candidate.accountId ?? candidate.AccountID ?? null),
  account_name: candidate.account_name || candidate.accountName || candidate.AccountName || '',
  accountName: candidate.account_name || candidate.accountName || candidate.AccountName || '',
  provider_type: candidate.provider_type || candidate.providerType || candidate.ProviderType || '',
  providerType: candidate.provider_type || candidate.providerType || candidate.ProviderType || '',
  priority: Number.parseInt(candidate.priority ?? candidate.Priority, 10) || 0,
  tier_index: Number.parseInt(candidate.tier_index ?? candidate.tierIndex ?? candidate.TierIndex, 10) || 0,
  tierIndex: Number.parseInt(candidate.tier_index ?? candidate.tierIndex ?? candidate.TierIndex, 10) || 0,
  tier_label: candidate.tier_label || candidate.tierLabel || candidate.TierLabel || '',
  tierLabel: candidate.tier_label || candidate.tierLabel || candidate.TierLabel || '',
  quota_status: candidate.quota_status || candidate.quotaStatus || candidate.QuotaStatus || '',
  quotaStatus: candidate.quota_status || candidate.quotaStatus || candidate.QuotaStatus || '',
  effective_quota_remaining: parseNullableNumber(candidate.effective_quota_remaining ?? candidate.effectiveQuotaRemaining ?? candidate.EffectiveQuotaRemaining ?? null),
  effectiveQuotaRemaining: parseNullableNumber(candidate.effective_quota_remaining ?? candidate.effectiveQuotaRemaining ?? candidate.EffectiveQuotaRemaining ?? null),
  fail_count: Number.parseInt(candidate.fail_count ?? candidate.failCount ?? candidate.FailCount, 10) || 0,
  failCount: Number.parseInt(candidate.fail_count ?? candidate.failCount ?? candidate.FailCount, 10) || 0,
  last_success_at: candidate.last_success_at || candidate.lastSuccessAt || candidate.LastSuccessAt || '',
  lastSuccessAt: candidate.last_success_at || candidate.lastSuccessAt || candidate.LastSuccessAt || '',
  decision: candidate.decision || candidate.Decision || '',
  reason: candidate.reason || candidate.Reason || '',
  reason_detail: candidate.reason_detail || candidate.reasonDetail || candidate.ReasonDetail || '',
  reasonDetail: candidate.reason_detail || candidate.reasonDetail || candidate.ReasonDetail || '',
  runtime_outcome: candidate.runtime_outcome || candidate.runtimeOutcome || candidate.RuntimeOutcome || '',
  runtimeOutcome: candidate.runtime_outcome || candidate.runtimeOutcome || candidate.RuntimeOutcome || '',
  runtime_error: candidate.runtime_error || candidate.runtimeError || candidate.RuntimeError || '',
  runtimeError: candidate.runtime_error || candidate.runtimeError || candidate.RuntimeError || ''
});

export const normalizeLatestAccountScheduleSnapshot = (snapshot = {}) => {
  const hasSnapshot = (snapshot.has_snapshot ?? snapshot.hasSnapshot ?? snapshot.HasSnapshot) === true;
  const candidates = Array.isArray(snapshot.candidates || snapshot.Candidates)
    ? (snapshot.candidates || snapshot.Candidates).map(normalizeAccountScheduleCandidateDecision)
    : [];

  return {
    unsupported: snapshot.unsupported === true,
    message: snapshot.message || '',
    has_snapshot: hasSnapshot,
    hasSnapshot,
    request_id: snapshot.request_id || snapshot.requestId || snapshot.RequestID || '',
    requestId: snapshot.request_id || snapshot.requestId || snapshot.RequestID || '',
    captured_at: snapshot.captured_at || snapshot.capturedAt || snapshot.CapturedAt || '',
    capturedAt: snapshot.captured_at || snapshot.capturedAt || snapshot.CapturedAt || '',
    updated_at: snapshot.updated_at || snapshot.updatedAt || snapshot.UpdatedAt || '',
    updatedAt: snapshot.updated_at || snapshot.updatedAt || snapshot.UpdatedAt || '',
    request_path: snapshot.request_path || snapshot.requestPath || snapshot.RequestPath || '',
    requestPath: snapshot.request_path || snapshot.requestPath || snapshot.RequestPath || '',
    selected_priority: Number.parseInt(snapshot.selected_priority ?? snapshot.selectedPriority ?? snapshot.SelectedPriority, 10) || 0,
    selectedPriority: Number.parseInt(snapshot.selected_priority ?? snapshot.selectedPriority ?? snapshot.SelectedPriority, 10) || 0,
    selected_tier_index: Number.parseInt(snapshot.selected_tier_index ?? snapshot.selectedTierIndex ?? snapshot.SelectedTierIndex, 10) || 0,
    selectedTierIndex: Number.parseInt(snapshot.selected_tier_index ?? snapshot.selectedTierIndex ?? snapshot.SelectedTierIndex, 10) || 0,
    selected_tier_label: snapshot.selected_tier_label || snapshot.selectedTierLabel || snapshot.SelectedTierLabel || '',
    selectedTierLabel: snapshot.selected_tier_label || snapshot.selectedTierLabel || snapshot.SelectedTierLabel || '',
    degraded_to_lower_priority: (snapshot.degraded_to_lower_priority ?? snapshot.degradedToLowerPriority ?? snapshot.DegradedToLowerPriority) === true,
    degradedToLowerPriority: (snapshot.degraded_to_lower_priority ?? snapshot.degradedToLowerPriority ?? snapshot.DegradedToLowerPriority) === true,
    selected_account_id: parseEntityId(snapshot.selected_account_id ?? snapshot.selectedAccountId ?? snapshot.SelectedAccountID ?? null),
    selectedAccountId: parseEntityId(snapshot.selected_account_id ?? snapshot.selectedAccountId ?? snapshot.SelectedAccountID ?? null),
    selected_account_name: snapshot.selected_account_name || snapshot.selectedAccountName || snapshot.SelectedAccountName || '',
    selectedAccountName: snapshot.selected_account_name || snapshot.selectedAccountName || snapshot.SelectedAccountName || '',
    final_outcome: snapshot.final_outcome || snapshot.finalOutcome || snapshot.FinalOutcome || '',
    finalOutcome: snapshot.final_outcome || snapshot.finalOutcome || snapshot.FinalOutcome || '',
    final_error: snapshot.final_error || snapshot.finalError || snapshot.FinalError || '',
    finalError: snapshot.final_error || snapshot.finalError || snapshot.FinalError || '',
    summary: snapshot.summary || snapshot.Summary || '',
    candidates
  };
};

export const getUpstreamAccounts = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('GetUpstreamAccounts');
  const result = await method();
  const list = Array.isArray(result) ? result : (result?.accounts || result?.data || []);
  return (list || []).map(normalizeUpstreamAccount);
};

export const createUpstreamAccount = async (input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('CreateUpstreamAccount');
  const result = await method(buildUpstreamAccountPayload(input));
  return result ?? { success: true };
};

export const updateUpstreamAccount = async (id, input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('UpdateUpstreamAccount');
  const result = await method(normalizeEntityId(id), buildUpstreamAccountPayload(input));
  return result ?? { success: true };
};

export const getUpstreamAccountCredential = async (id) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('GetUpstreamAccountCredential', { optional: true });
  if (!method) {
    return {
      unsupported: true,
      id: normalizeEntityId(id),
      credential_raw: '',
      credentialRaw: '',
      credential_raw_masked: '',
      credentialRawMasked: '',
      has_credential: false,
      hasCredential: false
    };
  }

  const result = await method(normalizeEntityId(id));
  return {
    id: parseEntityId(result?.id ?? result?.ID ?? normalizeEntityId(id)),
    credential_raw: result?.credential_raw || result?.credentialRaw || '',
    credentialRaw: result?.credential_raw || result?.credentialRaw || '',
    credential_raw_masked: result?.credential_raw_masked || result?.credentialRawMasked || '',
    credentialRawMasked: result?.credential_raw_masked || result?.credentialRawMasked || '',
    has_credential: (result?.has_credential ?? result?.hasCredential) === true,
    hasCredential: (result?.has_credential ?? result?.hasCredential) === true
  };
};

export const deleteUpstreamAccount = async (id) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('DeleteUpstreamAccount');
  const result = await method(normalizeEntityId(id));
  return result ?? { success: true };
};

export const moveUpstreamAccountToTier = async (id, targetTier) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('MoveUpstreamAccountToTier', { optional: true });
  if (!method) {
    return {
      success: false,
      unsupported: true,
      changed: false,
      message: '当前后端版本暂未提供 MoveUpstreamAccountToTier'
    };
  }

  const result = await method(normalizeEntityId(id), String(targetTier || 'primary'));
  return result ?? { success: true, changed: true };
};

export const swapUpstreamAccountGroups = async (sourceGroup, targetGroup) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('SwapUpstreamAccountGroups', { optional: true });
  if (!method) {
    return {
      success: false,
      unsupported: true,
      changed: false,
      message: '当前后端版本暂未提供 SwapUpstreamAccountGroups'
    };
  }

  const result = await method(String(sourceGroup || ''), String(targetGroup || ''));
  return result ?? { success: true, changed: true };
};

export const setGroupActiveAccount = async (groupKey, id) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('SetGroupActiveAccount', { optional: true });
  if (!method) {
    return {
      success: false,
      unsupported: true,
      changed: false,
      message: '当前后端版本暂未提供 SetGroupActiveAccount'
    };
  }

  const result = await method(String(groupKey || ''), normalizeEntityId(id));
  return result ?? { success: true, changed: true };
};

export const pinUpstreamAccountSelection = async (id) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('PinUpstreamAccountSelection', { optional: true });
  if (!method) {
    return {
      success: false,
      unsupported: true,
      changed: false,
      message: '当前后端版本暂未提供 PinUpstreamAccountSelection'
    };
  }

  const result = await method(normalizeEntityId(id));
  return result ?? { success: true, changed: true };
};

export const enableAutomaticAccountSelection = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('EnableAutomaticAccountSelection', { optional: true });
  if (!method) {
    return {
      success: false,
      unsupported: true,
      changed: false,
      message: '当前后端版本暂未提供 EnableAutomaticAccountSelection'
    };
  }

  const result = await method();
  return result ?? { success: true, changed: true };
};

export const toggleUpstreamAccount = async (id, enabled) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ToggleUpstreamAccount');
  const result = await method(normalizeEntityId(id), enabled !== false);
  return result ?? { success: true };
};

export const testUpstreamAccount = async (id) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('TestUpstreamAccount', { optional: true });
  if (!method) {
    return {
      success: false,
      unsupported: true,
      message: '当前后端版本暂未提供 TestUpstreamAccount'
    };
  }

  try {
    const result = await method(normalizeEntityId(id));
    if (result && typeof result === 'object') {
      return result;
    }
    return { success: true, message: '连通性测试已触发' };
  } catch (error) {
    const errorText = error?.message || String(error || '');
    if (/not found|undefined|is not a function/i.test(errorText)) {
      return {
        success: false,
        unsupported: true,
        message: '当前后端版本暂未提供 TestUpstreamAccount'
      };
    }
    throw error;
  }
};

export const getLatestAccountScheduleSnapshot = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('GetLatestAccountScheduleSnapshot', { optional: true });
  if (!method) {
    return normalizeLatestAccountScheduleSnapshot({
      unsupported: true,
      has_snapshot: false,
      message: '当前后端版本暂未提供 GetLatestAccountScheduleSnapshot'
    });
  }

  try {
    const result = await method();
    return normalizeLatestAccountScheduleSnapshot(result || {});
  } catch (error) {
    const errorText = error?.message || String(error || '');
    if (/not found|undefined|is not a function/i.test(errorText)) {
      return normalizeLatestAccountScheduleSnapshot({
        unsupported: true,
        has_snapshot: false,
        message: '当前后端版本暂未提供 GetLatestAccountScheduleSnapshot'
      });
    }
    throw error;
  }
};

export const refreshUpstreamAccountProfile = async (id) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('RefreshUpstreamAccountProfile', { optional: true });
  if (!method) {
    return {
      success: false,
      unsupported: true,
      message: '当前后端版本暂未提供 RefreshUpstreamAccountProfile'
    };
  }

  try {
    const result = await method(normalizeEntityId(id));
    if (result && typeof result === 'object') {
      return result;
    }
    return { success: true, message: '账号画像已刷新' };
  } catch (error) {
    const errorText = error?.message || String(error || '');
    if (/not found|undefined|is not a function/i.test(errorText)) {
      return {
        success: false,
        unsupported: true,
        message: '当前后端版本暂未提供 RefreshUpstreamAccountProfile'
      };
    }
    throw error;
  }
};

export const generateChatGPTOAuthLink = async () => {
  await initWails();
  const runtimeApp = typeof window !== 'undefined' ? window?.go?.main?.App : null;
  if (!WailsApp && !runtimeApp) throw new Error('Wails not available');

  const method = getWailsMethod('GenerateChatGPTOAuthLink', { optional: true });
  if (!method) {
    return {
      success: false,
      unsupported: true,
      message: '当前后端版本暂未提供 GenerateChatGPTOAuthLink'
    };
  }

  const result = await method();
  return {
    success: true,
    ...(result || {})
  };
};

export const exchangeChatGPTOAuthCallback = async (sessionId, callbackUrl) => {
  await initWails();
  const runtimeApp = typeof window !== 'undefined' ? window?.go?.main?.App : null;
  if (!WailsApp && !runtimeApp) throw new Error('Wails not available');

  const method = getWailsMethod('ExchangeChatGPTOAuthCallback', { optional: true });
  if (!method) {
    return {
      success: false,
      unsupported: true,
      message: '当前后端版本暂未提供 ExchangeChatGPTOAuthCallback'
    };
  }

  const result = await method({
    session_id: sessionId || '',
    callback_url: callbackUrl || ''
  });

  if (result && typeof result === 'object') {
    return result;
  }

  return {
    success: false,
    message: '提取 RT 失败：无响应数据'
  };
};

// ============================================
// 使用统计 API
// ============================================

/**
 * 获取使用统计（与 HTTP API 格式一致）
 * @param {Object} params - 查询参数
 * @param {string} params.period - 时间周期: "1h", "1d", "7d", "30d", "90d"
 * @param {string} params.start_date - 开始时间（优先于 period）
 * @param {string} params.end_date - 结束时间（优先于 period）
 * @param {string} params.status - 状态筛选
 * @param {string} params.model - 模型筛选
 * @param {string} params.endpoint - 端点筛选
 * @param {string} params.group - 组筛选
 * @returns {Promise<Object>} - 统计数据
 */
export const getUsageStats = async (params = {}) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  // 构建查询参数对象
  // 注意：前端 buildQueryParams() 返回 start_date/end_date（带下划线）
  const queryParams = {
    period: params.period || '30d',
    start_date: params.start_date || '',
    end_date: params.end_date || '',
    status: params.status || '',
    model: params.model || '',
    channel: params.channel || '',
    endpoint: params.endpoint || '',
    group: params.group || '',
    source_view: params.source_view || params.sourceView || 'all'
  };

  console.log('📊 [Wails] GetUsageStats 参数:', queryParams);
  const data = await WailsApp.GetUsageStats(queryParams);

  // 返回与 HTTP API 一致的格式
  return {
    period: data.period || queryParams.period,
    total_requests: data.total_requests || 0,
    success_rate: data.success_rate || 0,
    avg_duration_ms: data.avg_duration_ms || 0,
    total_cost_usd: data.total_cost_usd || 0,
    total_tokens: data.total_tokens || 0,
    failed_requests: data.failed_requests || 0
  };
};

export const getUsageSummary = async (startTime = '', endTime = '') => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const summary = await WailsApp.GetUsageSummary(startTime, endTime);

  return {
    total_requests: summary.total_requests || 0,
    all_time_total_requests: summary.all_time_total_requests || 0, // 全部历史请求数
    today_requests: summary.today_requests || 0,            // 今日请求数
    successful_requests: summary.success_requests || 0,
    failed_requests: summary.failed_requests || 0,
    total_input_tokens: summary.total_input_tokens || 0,
    total_output_tokens: summary.total_output_tokens || 0,
    total_cost: summary.total_cost || 0,
    today_cost: summary.today_cost || 0,                    // 今日成本
    all_time_total_cost: summary.all_time_total_cost || 0,  // 全部历史成本
    today_tokens: summary.today_tokens || 0,                // 今日 tokens
    all_time_total_tokens: summary.all_time_total_tokens || 0,  // 全部历史 tokens
    total_tokens: (summary.total_input_tokens || 0) + (summary.total_output_tokens || 0)
  };
};

export const getRequests = async (params = {}) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  // 构建 Wails 绑定参数对象
  // 注意：前端 buildQueryParams() 返回 start_date/end_date（带下划线）
  const queryParams = {
    page: parseInt(params.page || 1),
    page_size: parseInt(params.limit || params.pageSize || 50),
    start_date: params.start_date || params.start_time || params.startDate || '',
    end_date: params.end_date || params.end_time || params.endDate || '',
    status: params.status || '',
    model: params.model || '',
    channel: params.channel || '',
    endpoint: params.endpoint || '',
    group: params.group || '',
    source_view: params.source_view || params.sourceView || 'all'
  };

  console.log('🔍 [Wails] GetRequests 参数:', queryParams);
  const result = await WailsApp.GetRequests(queryParams);
  console.log('🔍 [Wails] GetRequests 返回:', result);

  // 转换请求记录格式
  const requests = (result.requests || []).map(r => ({
    request_id: r.request_id,
    id: r.request_id,
    timestamp: r.timestamp,
    start_time: r.timestamp,
    channel: r.channel || '',
    upstream_type: r.upstream_type || 'endpoint',
    upstream_name: r.upstream_name || '',
    upstream_source_name: r.upstream_source_name || '',
    upstream_id: r.upstream_id || null,
    endpoint_name: r.endpoint || r.upstream_name || '',
    endpoint: r.endpoint || r.upstream_name || '',
    group_name: r.group || r.upstream_source_name || '',
    group: r.group || r.upstream_source_name || '',
    model_name: r.model,
    model: r.model,
    status: r.status,
    status_code: r.http_status,
    http_status_code: r.http_status,  // 添加 http_status_code 映射
    retry_count: r.retry_count || 0,  // 添加重试次数
    input_tokens: r.input_tokens,
    output_tokens: r.output_tokens,
    cache_creation_tokens: r.cache_creation_tokens,
    cache_creation_5m_tokens: r.cache_creation_5m_tokens,  // v5.0.1+
    cache_creation_1h_tokens: r.cache_creation_1h_tokens,  // v5.0.1+
    cache_read_tokens: r.cache_read_tokens,
    duration_ms: r.response_time,
    duration: r.response_time,
    first_token_ms: r.first_token_ms ?? null,
    completion_ms: r.completion_ms ?? null,
    is_streaming: r.is_streaming,
    total_cost_usd: r.cost,
    cost: r.cost,
    route_mode: r.route_mode || r.routeMode || 'auto',
    routeMode: r.route_mode || r.routeMode || 'auto',
    requested_endpoint: r.requested_endpoint || r.requestedEndpoint || '',
    requestedEndpoint: r.requested_endpoint || r.requestedEndpoint || '',
    effective_endpoint: r.effective_endpoint || r.effectiveEndpoint || '',
    effectiveEndpoint: r.effective_endpoint || r.effectiveEndpoint || '',
    fallback_reason: r.fallback_reason || r.fallbackReason || '',
    fallbackReason: r.fallback_reason || r.fallbackReason || '',
    route_decision_at: r.route_decision_at || r.routeDecisionAt || '',
    routeDecisionAt: r.route_decision_at || r.routeDecisionAt || '',
    // 错误信息字段
    failure_reason: r.failure_reason || '',
    cancel_reason: r.cancel_reason || ''
  }));

  return {
    requests,
    total: result.total,
    page: result.page,
    pageSize: result.page_size,
    totalPages: Math.ceil(result.total / result.page_size)
  };
};

// ============================================
// 代理信息
// ============================================

export const getProxyURL = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  return await WailsApp.GetProxyURL();
};

export const isProxyRunning = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  return await WailsApp.IsProxyRunning();
};

// ============================================
// 图表数据 API
// ============================================

/**
 * 获取请求趋势图表数据
 * @param {number} minutes - 时间范围（分钟）
 * @returns {Promise<Array>} - 图表数据点数组 [{time, total, success, fail}]
 */
export const getRequestTrendChart = async (minutes = 30) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  console.log('📊 [Wails] 调用 GetRequestTrendChart, minutes:', minutes);
  const data = await WailsApp.GetRequestTrendChart(minutes);
  console.log('📊 [Wails] GetRequestTrendChart 返回:', data ? `${data.length} 个数据点` : '无数据', data);
  return data || [];
};

/**
 * 获取响应时间图表数据
 * @param {number} minutes - 时间范围（分钟）
 * @returns {Promise<Array>} - 图表数据点数组 [{time, avg, min, max}]
 */
export const getResponseTimeChart = async (minutes = 30) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const data = await WailsApp.GetResponseTimeChart(minutes);
  return data || [];
};

/**
 * 获取连接活动图表数据
 * @param {number} minutes - 时间范围（分钟）
 * @returns {Promise<Array>} - 图表数据点数组 [{time, value}]
 */
export const getConnectionActivityChart = async (minutes = 60) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const data = await WailsApp.GetConnectionActivityChart(minutes);
  // 转换为前端期望的格式 (connections 字段)
  return (data || []).map(point => ({
    time: point.time,
    connections: point.value
  }));
};

// ============================================
// Token 使用统计 API
// ============================================

/**
 * 获取 Token 使用统计（运行时内存数据）
 * @returns {Promise<Object>} - Token 使用数据 {input, output, cacheCreation, cacheRead}
 */
export const getTokenUsage = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const data = await WailsApp.GetTokenUsage();
  return {
    input: data.input_tokens || 0,
    output: data.output_tokens || 0,
    cacheCreation: data.cache_creation_tokens || 0,
    cacheRead: data.cache_read_tokens || 0,
    total: data.total_tokens || 0
  };
};

// ============================================
// 端点健康状态图表 API
// ============================================

/**
 * 获取端点健康状态数据
 * @returns {Promise<Object>} - 健康状态数据 {healthy, unhealthy, total}
 */
export const getEndpointHealthChart = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const data = await WailsApp.GetEndpointHealthChart();
  return {
    healthy: data.healthy || 0,
    unhealthy: data.unhealthy || 0,
    total: data.total || 0
  };
};

// ============================================
// 端点成本图表 API
// ============================================

/**
 * 获取当日端点成本数据
 * @returns {Promise<Array>} - 端点成本数据 [{name, tokens, cost}]
 */
export const getEndpointCosts = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const data = await WailsApp.GetEndpointCosts();
  // 数据已经是 [{name, tokens, cost}] 格式
  return data || [];
};

// ============================================
// v5.0+ 端点存储管理 API (SQLite)
// ============================================

/**
 * 获取端点存储状态
 * @returns {Promise<Object>} - {enabled, storage_type, total_count, enabled_count}
 */
export const getEndpointStorageStatus = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const status = await WailsApp.GetEndpointStorageStatus();
  return {
    enabled: status.enabled || false,
    storageType: status.storage_type || 'yaml',
    totalCount: status.total_count || 0,
    enabledCount: status.enabled_count || 0
  };
};

export const mapEndpointRecord = (record = {}) => ({
  id: record.id,
  channel: record.channel,
  name: record.name,
  url: record.url,
  token: record.token,
  apiKey: record.api_key,
  tokenMasked: record.token_masked,
  apiKeyMasked: record.api_key_masked,
  headers: record.headers || {},
  priority: record.priority,
  failoverEnabled: record.failover_enabled,
  cooldownSeconds: record.cooldown_seconds,
  timeoutSeconds: record.timeout_seconds,
  supportsCountTokens: record.supports_count_tokens,
  modelRewriteRules: record.model_rewrite_rules || '',
  costMultiplier: record.cost_multiplier,
  inputCostMultiplier: record.input_cost_multiplier,
  outputCostMultiplier: record.output_cost_multiplier,
  cacheCreationCostMultiplier: record.cache_creation_cost_multiplier,
  cacheCreationCostMultiplier1h: record.cache_creation_cost_multiplier_1h,
  cacheReadCostMultiplier: record.cache_read_cost_multiplier,
  enabled: record.enabled,
  createdAt: record.created_at,
  updatedAt: record.updated_at,
  healthy: record.healthy,
  never_checked: record.never_checked === true,
  neverChecked: record.never_checked === true,
  lastCheck: record.last_check,
  responseTimeMs: record.response_time_ms,
  in_cooldown: record.in_cooldown,
  inCooldown: record.in_cooldown,
  cooldown_until: record.cooldown_until,
  cooldownUntil: record.cooldown_until,
  cooldown_reason: record.cooldown_reason,
  cooldownReason: record.cooldown_reason
});

export const buildEndpointRecordPayload = (input = {}, fallbackName = '') => ({
  channel: input.channel || '',
  name: input.name || fallbackName,
  url: input.url || '',
  token: input.token || '',
  api_key: input.apiKey || '',
  headers: input.headers || {},
  priority: parseInt(input.priority) || 1,
  failover_enabled: input.failoverEnabled !== false,
  cooldown_seconds: input.cooldownSeconds ? parseInt(input.cooldownSeconds) : null,
  timeout_seconds: parseInt(input.timeoutSeconds) || 300,
  supports_count_tokens: input.supportsCountTokens || false,
  model_rewrite_rules: String(input.modelRewriteRules || '').trim(),
  cost_multiplier: parseFloat(input.costMultiplier) || 1.0,
  input_cost_multiplier: parseFloat(input.inputCostMultiplier) || 1.0,
  output_cost_multiplier: parseFloat(input.outputCostMultiplier) || 1.0,
  cache_creation_cost_multiplier: parseFloat(input.cacheCreationCostMultiplier) || 1.0,
  cache_creation_cost_multiplier_1h: parseFloat(input.cacheCreationCostMultiplier1h) || 1.0,
  cache_read_cost_multiplier: parseFloat(input.cacheReadCostMultiplier) || 1.0
});

/**
 * 获取所有端点记录（SQLite 存储）
 * @returns {Promise<Array>} - 端点记录数组
 */
export const getEndpointRecords = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const records = await WailsApp.GetEndpointRecords();
  return (records || []).map(mapEndpointRecord);
};

/**
 * 获取单个端点记录
 * @param {string} name - 端点名称
 * @returns {Promise<Object>} - 端点记录
 */
export const getEndpointRecord = async (name) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const r = await WailsApp.GetEndpointRecord(name);
  return mapEndpointRecord(r);
};

/**
 * 创建新端点
 * @param {Object} input - 端点配置
 * @returns {Promise<void>}
 */
export const createEndpointRecord = async (input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const record = buildEndpointRecordPayload(input);

  await WailsApp.CreateEndpointRecord(record);
  return { success: true };
};

/**
 * 更新端点配置
 * @param {string} name - 端点名称
 * @param {Object} input - 更新的配置
 * @returns {Promise<void>}
 */
export const updateEndpointRecord = async (name, input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const record = buildEndpointRecordPayload(input, name);

  await WailsApp.UpdateEndpointRecord(name, record);
  return { success: true };
};

/**
 * 删除端点
 * @param {string} name - 端点名称
 * @returns {Promise<void>}
 */
export const deleteEndpointRecord = async (name) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.DeleteEndpointRecord(name);
  return { success: true };
};

/**
 * 切换端点启用状态
 * @param {string} name - 端点名称
 * @param {boolean} enabled - 是否启用
 * @returns {Promise<void>}
 */
export const toggleEndpointRecord = async (name, enabled) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.ToggleEndpointRecord(name, enabled);
  return { success: true };
};

/**
 * 获取所有渠道
 * @returns {Promise<Array>} - 渠道列表 [{name, endpointCount}]
 */
export const getChannels = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const channels = await WailsApp.GetChannels();
  return (channels || []).map(c => ({
    name: c.name,
    endpointCount: c.endpoint_count
  }));
};

/**
 * 按渠道获取端点
 * @param {string} channel - 渠道名称
 * @returns {Promise<Array>} - 端点记录数组
 */
export const getEndpointsByChannel = async (channel) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const records = await WailsApp.GetEndpointsByChannel(channel);
  return (records || []).map(r => ({
    id: r.id,
    channel: r.channel,
    name: r.name,
    url: r.url,
    tokenMasked: r.token_masked,
    priority: r.priority,
    failoverEnabled: r.failover_enabled,
    enabled: r.enabled,
    healthy: r.healthy,
    responseTimeMs: r.response_time_ms
  }));
};

// ============================================
// v5.0+ 模型定价管理 API (SQLite)
// ============================================

/**
 * 获取模型定价存储状态
 * @returns {Promise<Object>} - {enabled, totalCount, hasDefault}
 */
export const getModelPricingStorageStatus = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const status = await WailsApp.GetModelPricingStorageStatus();
  return {
    enabled: status.enabled || false,
    totalCount: status.total_count || 0,
    hasDefault: status.has_default || false
  };
};

/**
 * 获取所有模型定价
 * @returns {Promise<Array>} - 模型定价数组
 */
export const getModelPricings = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const records = await WailsApp.GetModelPricings();
  return (records || []).map(r => ({
    id: r.id,
    modelName: r.model_name,
    displayName: r.display_name,
    description: r.description,
    inputPrice: r.input_price,
    outputPrice: r.output_price,
    cacheCreationPrice5m: r.cache_creation_price_5m,
    cacheCreationPrice1h: r.cache_creation_price_1h,
    cacheReadPrice: r.cache_read_price,
    isDefault: r.is_default,
    createdAt: r.created_at,
    updatedAt: r.updated_at
  }));
};

/**
 * 获取单个模型定价
 * @param {string} modelName - 模型名称
 * @returns {Promise<Object>} - 模型定价记录
 */
export const getModelPricing = async (modelName) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const r = await WailsApp.GetModelPricing(modelName);
  return {
    id: r.id,
    modelName: r.model_name,
    displayName: r.display_name,
    description: r.description,
    inputPrice: r.input_price,
    outputPrice: r.output_price,
    cacheCreationPrice5m: r.cache_creation_price_5m,
    cacheCreationPrice1h: r.cache_creation_price_1h,
    cacheReadPrice: r.cache_read_price,
    isDefault: r.is_default,
    createdAt: r.created_at,
    updatedAt: r.updated_at
  };
};

/**
 * 创建模型定价
 * @param {Object} input - 定价配置
 * @returns {Promise<Object>} - {success: true}
 */
export const createModelPricing = async (input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  // 转换为后端期望的格式
  const record = {
    model_name: input.modelName || '',
    display_name: input.displayName || '',
    description: input.description || '',
    input_price: parseFloat(input.inputPrice) || 3.0,
    output_price: parseFloat(input.outputPrice) || 15.0,
    cache_creation_price_5m: parseFloat(input.cacheCreationPrice5m) || 0,
    cache_creation_price_1h: parseFloat(input.cacheCreationPrice1h) || 0,
    cache_read_price: parseFloat(input.cacheReadPrice) || 0,
    is_default: input.isDefault || false
  };

  await WailsApp.CreateModelPricing(record);
  return { success: true };
};

/**
 * 更新模型定价
 * @param {string} modelName - 模型名称
 * @param {Object} input - 更新的配置
 * @returns {Promise<Object>} - {success: true}
 */
export const updateModelPricing = async (modelName, input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  // 转换为后端期望的格式
  const record = {
    model_name: input.modelName || modelName,
    display_name: input.displayName || '',
    description: input.description || '',
    input_price: parseFloat(input.inputPrice) || 3.0,
    output_price: parseFloat(input.outputPrice) || 15.0,
    cache_creation_price_5m: parseFloat(input.cacheCreationPrice5m) || 0,
    cache_creation_price_1h: parseFloat(input.cacheCreationPrice1h) || 0,
    cache_read_price: parseFloat(input.cacheReadPrice) || 0,
    is_default: input.isDefault || false
  };

  await WailsApp.UpdateModelPricing(modelName, record);
  return { success: true };
};

/**
 * 删除模型定价
 * @param {string} modelName - 模型名称
 * @returns {Promise<Object>} - {success: true}
 */
export const deleteModelPricing = async (modelName) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.DeleteModelPricing(modelName);
  return { success: true };
};

/**
 * 设置默认模型定价
 * @param {string} modelName - 模型名称
 * @returns {Promise<Object>} - {success: true}
 */
export const setDefaultModelPricing = async (modelName) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.SetDefaultModelPricing(modelName);
  return { success: true };
};

// ============================================
// v5.1+ 系统设置管理 API (SQLite)
// ============================================

/**
 * 获取设置存储状态
 * @returns {Promise<Object>} - {enabled, totalCount, categoryCount, isInitialized}
 */
export const getSettingsStorageStatus = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const status = await WailsApp.GetSettingsStorageStatus();
  return {
    enabled: status.enabled || false,
    totalCount: status.total_count || 0,
    categoryCount: status.category_count || 0,
    isInitialized: status.is_initialized || false
  };
};

/**
 * 获取所有设置分类
 * @returns {Promise<Array>} - 分类信息数组
 */
export const getSettingCategories = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const categories = await WailsApp.GetSettingCategories();
  return (categories || []).map(c => ({
    name: c.name,
    label: c.label,
    description: c.description,
    icon: c.icon,
    order: c.order
  }));
};

/**
 * 获取所有设置
 * @returns {Promise<Array>} - 设置数组
 */
export const getAllSettings = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const settings = await WailsApp.GetAllSettings();
  return (settings || []).map(s => ({
    id: s.id,
    category: s.category,
    key: s.key,
    value: s.value,
    value_type: s.value_type,
    label: s.label,
    description: s.description,
    display_order: s.display_order,
    requires_restart: s.requires_restart,
    secret_configured: s.secret_configured || false,
    created_at: s.created_at,
    updated_at: s.updated_at
  }));
};

/**
 * 获取指定分类的设置
 * @param {string} category - 分类名称
 * @returns {Promise<Array>} - 设置数组
 */
export const getSettingsByCategory = async (category) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const settings = await WailsApp.GetSettingsByCategory(category);
  return (settings || []).map(s => ({
    id: s.id,
    category: s.category,
    key: s.key,
    value: s.value,
    value_type: s.value_type,
    label: s.label,
    description: s.description,
    display_order: s.display_order,
    requires_restart: s.requires_restart,
    secret_configured: s.secret_configured || false,
    created_at: s.created_at,
    updated_at: s.updated_at
  }));
};

/**
 * 获取单个设置
 * @param {string} category - 分类名称
 * @param {string} key - 设置键
 * @returns {Promise<Object>} - 设置记录
 */
export const getSetting = async (category, key) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const s = await WailsApp.GetSetting(category, key);
  return {
    id: s.id,
    category: s.category,
    key: s.key,
    value: s.value,
    value_type: s.value_type,
    label: s.label,
    description: s.description,
    display_order: s.display_order,
    requires_restart: s.requires_restart,
    secret_configured: s.secret_configured || false,
    created_at: s.created_at,
    updated_at: s.updated_at
  };
};

/**
 * 更新单个设置
 * @param {string} category - 分类名称
 * @param {string} key - 设置键
 * @param {string} value - 新值
 * @returns {Promise<Object>} - {success: true}
 */
export const updateSetting = async (category, key, value) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.UpdateSetting({
    category,
    key,
    value
  });
  return { success: true };
};

/**
 * 批量更新设置
 * @param {Object|Array} input - 设置对象 {settings: [...]} 或设置数组 [{category, key, value}]
 * @returns {Promise<Object>} - {success: true}
 */
export const batchUpdateSettings = async (input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  // 兼容两种调用方式：对象 {settings: [...]} 或数组 [...]
  const settingsArray = Array.isArray(input) ? input : (input.settings || []);

  await WailsApp.BatchUpdateSettings({
    settings: settingsArray.map(s => ({
      category: s.category,
      key: s.key,
      value: s.value
    }))
  });
  return { success: true };
};

/**
 * 重置分类设置为默认值
 * @param {string} category - 分类名称
 * @returns {Promise<Object>} - {success: true}
 */
export const resetCategorySettings = async (category) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.ResetCategorySettings(category);
  return { success: true };
};

const normalizeCodexModel = (model = {}) => ({
  id: model.id || model.ID || '',
  object: model.object || model.Object || 'model',
  owned_by: model.owned_by || model.ownedBy || model.OwnedBy || 'openai',
  source: model.source || model.Source || 'custom',
  enabled: model.enabled !== undefined ? Boolean(model.enabled) : Boolean(model.Enabled),
  deprecated: model.deprecated !== undefined ? Boolean(model.deprecated) : Boolean(model.Deprecated),
  display_name: model.display_name || model.displayName || model.DisplayName || model.id || model.ID || '',
  description: model.description || model.Description || ''
});

const normalizeCodexModelCatalog = (catalog = {}) => {
  const models = Array.isArray(catalog.models || catalog.Models)
    ? (catalog.models || catalog.Models).map(normalizeCodexModel)
    : [];
  return {
    enabled: catalog.enabled !== undefined ? Boolean(catalog.enabled) : Boolean(catalog.Enabled),
    mode: catalog.mode || catalog.Mode || 'local',
    models,
    effective_count: catalog.effective_count ?? catalog.effectiveCount ?? catalog.EffectiveCount ?? models.filter(model => model.enabled).length
  };
};

export const getCodexModelCatalog = async () => {
  await initWails();
  const method = getWailsMethod('GetCodexModelCatalog');
  return normalizeCodexModelCatalog(await method());
};

export const saveCodexModelCatalog = async (catalog) => {
  await initWails();
  const method = getWailsMethod('SaveCodexModelCatalog');
  return normalizeCodexModelCatalog(await method({
    enabled: catalog?.enabled === true,
    mode: catalog?.mode || 'local',
    models: (catalog?.models || []).map(normalizeCodexModel)
  }));
};

export const mergeDefaultCodexModelCatalog = async () => {
  await initWails();
  const method = getWailsMethod('MergeDefaultCodexModelCatalog');
  return normalizeCodexModelCatalog(await method());
};

export const resetCodexModelCatalog = async () => {
  await initWails();
  const method = getWailsMethod('ResetCodexModelCatalog');
  return normalizeCodexModelCatalog(await method());
};

/**
 * 获取端口信息
 * @returns {Promise<Object>} - {preferred_port, actual_port, is_default, was_occupied}
 */
export const getPortInfo = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const info = await WailsApp.GetPortInfo();
  return {
    preferred_port: info.preferred_port,
    actual_port: info.actual_port,
    is_default: info.is_default,
    was_occupied: info.was_occupied
  };
};

/**
 * 更新首选端口（需要重启生效）
 * @param {number} port - 新端口号
 * @returns {Promise<Object>} - {success: true}
 */
export const updatePreferredPort = async (port) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  await WailsApp.UpdatePreferredPort(port);
  return { success: true };
};

/**
 * 检查端口是否可用
 * @param {number} port - 端口号
 * @returns {Promise<boolean>} - 是否可用
 */
export const checkPortAvailable = async (port) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  return await WailsApp.CheckPortAvailable(port);
};

// ============================================
// 隐私保护 (v6.1+)
// ============================================

const PRIVACY_SCOPE_KEYS = ['paths', 'upstream_types', 'endpoint_names', 'account_ids', 'provider_types'];

// 解析 scope JSON 为带默认空数组的对象（容错：非法 JSON 返回全空）
export const parsePrivacyScope = (scopeJson) => {
  const scope = {};
  for (const key of PRIVACY_SCOPE_KEYS) {
    scope[key] = [];
  }
  if (!scopeJson || typeof scopeJson !== 'string') {
    return scope;
  }
  try {
    const parsed = JSON.parse(scopeJson);
    if (parsed && typeof parsed === 'object') {
      for (const key of PRIVACY_SCOPE_KEYS) {
        if (Array.isArray(parsed[key])) {
          scope[key] = parsed[key];
        }
      }
    }
  } catch {
    // 非法 scope JSON 按不限处理，由后端在保存时校验
  }
  return scope;
};

// 将 UI scope 对象编码为 scope_json（空维度省略，保持后端语义“空=不限”）
export const encodePrivacyScope = (scope = {}) => {
  const payload = {};
  for (const key of PRIVACY_SCOPE_KEYS) {
    const values = scope[key];
    if (Array.isArray(values) && values.length > 0) {
      payload[key] = key === 'account_ids'
        ? values.map((v) => Number(v)).filter((v) => Number.isFinite(v))
        : values.map((v) => String(v).trim()).filter(Boolean);
    }
  }
  return JSON.stringify(payload);
};

export const normalizePrivacySettings = (settings = {}) => ({
  mode: settings.mode ?? settings.Mode ?? 'disabled',
  scan_max_bytes: Number(settings.scan_max_bytes ?? settings.ScanMaxBytes ?? 4194304),
  over_limit_action: settings.over_limit_action ?? settings.OverLimitAction ?? 'scan_prefix',
  on_error: settings.on_error ?? settings.OnError ?? 'fail_open',
  version: Number(settings.version ?? settings.Version ?? 0),
  status: settings.status ?? settings.Status ?? 'ok',
  compile_error: settings.compile_error ?? settings.CompileError ?? '',
  enabled_rules: Number(settings.enabled_rules ?? settings.EnabledRules ?? 0),
  updated_at: settings.updated_at ?? settings.UpdatedAt ?? ''
});

export const normalizePrivacyRule = (rule = {}) => {
  const scopeJson = rule.scope_json ?? rule.ScopeJSON ?? '{}';
  return {
    id: Number(rule.id ?? rule.ID ?? 0),
    enabled: Boolean(rule.enabled ?? rule.Enabled ?? false),
    name: rule.name ?? rule.Name ?? '',
    description: rule.description ?? rule.Description ?? '',
    priority: Number(rule.priority ?? rule.Priority ?? 100),
    match_type: rule.match_type ?? rule.MatchType ?? 'regex',
    pattern: rule.pattern ?? rule.Pattern ?? '',
    placeholder: rule.placeholder ?? rule.Placeholder ?? '',
    action: rule.action ?? rule.Action ?? 'redact',
    scope_json: scopeJson,
    scope: parsePrivacyScope(scopeJson),
    source: rule.source ?? rule.Source ?? 'custom',
    compile_error: rule.compile_error ?? rule.CompileError ?? '',
    created_at: rule.created_at ?? rule.CreatedAt ?? '',
    updated_at: rule.updated_at ?? rule.UpdatedAt ?? ''
  };
};

export const normalizePrivacyExactSecret = (secret = {}) => ({
  id: Number(secret.id ?? secret.ID ?? 0),
  enabled: Boolean(secret.enabled ?? secret.Enabled ?? false),
  name: secret.name ?? secret.Name ?? '',
  category: secret.category ?? secret.Category ?? 'custom',
  placeholder: secret.placeholder ?? secret.Placeholder ?? '',
  source_type: secret.source_type ?? secret.SourceType ?? 'manual',
  source_ref: secret.source_ref ?? secret.SourceRef ?? '',
  description: secret.description ?? secret.Description ?? '',
  masked_value: secret.masked_value ?? secret.MaskedValue ?? '',
  value_length: Number(secret.value_length ?? secret.ValueLength ?? 0),
  value_hash_short: secret.value_hash_short ?? secret.ValueHashShort ?? '',
  created_at: secret.created_at ?? secret.CreatedAt ?? '',
  updated_at: secret.updated_at ?? secret.UpdatedAt ?? ''
});

export const normalizePrivacyImportCandidate = (candidate = {}) => ({
  source_type: candidate.source_type ?? candidate.SourceType ?? '',
  source_ref: candidate.source_ref ?? candidate.SourceRef ?? '',
  name: candidate.name ?? candidate.Name ?? '',
  category: candidate.category ?? candidate.Category ?? 'custom',
  masked_value: candidate.masked_value ?? candidate.MaskedValue ?? '',
  value_length: Number(candidate.value_length ?? candidate.ValueLength ?? 0),
  value_hash_short: candidate.value_hash_short ?? candidate.ValueHashShort ?? '',
  already_exists: Boolean(candidate.already_exists ?? candidate.AlreadyExists ?? false)
});

// 将 UI 规则对象转换为后端 PrivacyRuleInput
export const buildPrivacyRulePayload = (input = {}) => ({
  enabled: Boolean(input.enabled),
  name: String(input.name ?? '').trim(),
  description: String(input.description ?? '').trim(),
  priority: Number.isFinite(Number(input.priority)) ? Number(input.priority) : 100,
  match_type: String(input.match_type ?? input.matchType ?? 'regex').toLowerCase(),
  pattern: String(input.pattern ?? ''),
  placeholder: String(input.placeholder ?? ''),
  action: String(input.action ?? 'redact').toLowerCase(),
  scope_json: typeof input.scope_json === 'string' && input.scope_json
    ? input.scope_json
    : encodePrivacyScope(input.scope)
});

export const buildPrivacyExactSecretPayload = (input = {}) => ({
  enabled: input.enabled !== false,
  name: String(input.name ?? '').trim(),
  secret_value: String(input.secret_value ?? input.secretValue ?? ''),
  category: String(input.category ?? 'custom').trim(),
  placeholder: String(input.placeholder ?? ''),
  source_type: String(input.source_type ?? input.sourceType ?? 'manual').trim(),
  source_ref: String(input.source_ref ?? input.sourceRef ?? '').trim(),
  description: String(input.description ?? '').trim()
});

export const getPrivacySettings = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('GetPrivacySettings');
  return normalizePrivacySettings(await method());
};

export const updatePrivacySettings = async (input = {}) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('UpdatePrivacySettings');
  const result = await method({
    mode: String(input.mode ?? 'disabled'),
    scan_max_bytes: Number(input.scan_max_bytes ?? input.scanMaxBytes ?? 4194304),
    over_limit_action: String(input.over_limit_action ?? input.overLimitAction ?? 'scan_prefix'),
    on_error: String(input.on_error ?? input.onError ?? 'fail_open')
  });
  return normalizePrivacySettings(result);
};

export const getPrivacyRules = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ListPrivacyRules');
  const result = await method();
  return (Array.isArray(result) ? result : []).map(normalizePrivacyRule);
};

export const listPrivacyExactSecrets = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ListPrivacyExactSecrets');
  const result = await method();
  return (Array.isArray(result) ? result : []).map(normalizePrivacyExactSecret);
};

export const createPrivacyExactSecret = async (input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('CreatePrivacyExactSecret');
  return normalizePrivacyExactSecret(await method(buildPrivacyExactSecretPayload(input)));
};

export const updatePrivacyExactSecret = async (id, input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('UpdatePrivacyExactSecret');
  return normalizePrivacyExactSecret(await method(normalizeEntityId(id), buildPrivacyExactSecretPayload(input)));
};

export const deletePrivacyExactSecret = async (id) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('DeletePrivacyExactSecret');
  await method(normalizeEntityId(id));
  return { success: true };
};

export const clearPrivacyExactSecrets = async (confirmText) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ClearPrivacyExactSecrets');
  await method(String(confirmText ?? ''));
  return { success: true };
};

export const listPrivacySecretImportCandidates = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ListPrivacySecretImportCandidates');
  const result = await method();
  return (Array.isArray(result) ? result : []).map(normalizePrivacyImportCandidate);
};

export const importPrivacySecretCandidate = async (input = {}) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ImportPrivacySecretCandidate');
  const result = await method({
    source_type: String(input.source_type ?? '').trim(),
    source_ref: String(input.source_ref ?? '').trim(),
    name: String(input.name ?? '').trim(),
    category: String(input.category ?? '').trim(),
    placeholder: String(input.placeholder ?? ''),
    description: String(input.description ?? '').trim(),
    secret_value: String(input.secret_value ?? '')
  });
  return normalizePrivacyExactSecret(result);
};

export const createPrivacyRule = async (input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('CreatePrivacyRule');
  return normalizePrivacyRule(await method(buildPrivacyRulePayload(input)));
};

export const updatePrivacyRule = async (id, input) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('UpdatePrivacyRule');
  return normalizePrivacyRule(await method(normalizeEntityId(id), buildPrivacyRulePayload(input)));
};

export const deletePrivacyRule = async (id) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('DeletePrivacyRule');
  await method(normalizeEntityId(id));
  return { success: true };
};

export const reorderPrivacyRules = async (orders = []) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ReorderPrivacyRules');
  await method(orders.map((item) => ({
    id: Number(item.id),
    priority: Number(item.priority)
  })));
  return { success: true };
};

// 测试面板：文本只在内存中处理，不写 localStorage、不落日志
export const testPrivacyRules = async (input = {}) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('TestPrivacyRules');
  const result = await method({
    text: String(input.text ?? ''),
    path: String(input.path ?? ''),
    upstream_type: String(input.upstream_type ?? input.upstreamType ?? ''),
    endpoint_name: String(input.endpoint_name ?? input.endpointName ?? ''),
    account_id: Number(input.account_id ?? input.accountId ?? 0),
    provider_type: String(input.provider_type ?? input.providerType ?? '')
  });
  return {
    original_length: Number(result?.original_length ?? 0),
    hit_count: Number(result?.hit_count ?? 0),
    changed: Boolean(result?.changed),
    replaced_text: result?.replaced_text ?? '',
    rule_hits: Array.isArray(result?.rule_hits) ? result.rule_hits : [],
    scan_duration_ms: Number(result?.scan_duration_ms ?? 0)
  };
};

export const listPrivacyPresets = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ListPrivacyPresets');
  const result = await method();
  return Array.isArray(result) ? result : [];
};

export const importPrivacyPreset = async (presetId) => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ImportPrivacyPreset');
  const result = await method(String(presetId));
  return (Array.isArray(result) ? result : []).map(normalizePrivacyRule);
};

export const exportPrivacyRules = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('ExportPrivacyRules');
  return await method();
};

export const getPrivacyRuntimeStats = async () => {
  await initWails();
  if (!WailsApp) throw new Error('Wails not available');

  const method = getWailsMethod('GetPrivacyRuntimeStats');
  const result = await method();
  return {
    scan_count: Number(result?.scan_count ?? 0),
    hit_count: Number(result?.hit_count ?? 0),
    blocked_count: Number(result?.blocked_count ?? 0),
    truncated_count: Number(result?.truncated_count ?? 0),
    rule_stats: Array.isArray(result?.rule_stats) ? result.rule_stats : []
  };
};
