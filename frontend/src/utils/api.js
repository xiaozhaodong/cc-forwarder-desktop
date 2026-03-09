// ============================================
// API 服务层
// 2025-11-28 (Updated 2025-12-04 for Wails support)
// ============================================

import { API_ENDPOINTS, ERROR_MESSAGES } from './constants.js';
import * as WailsApi from './wailsApi.js';

// 检测是否在 Wails 环境中运行
export const isWailsEnvironment = WailsApi.isWailsEnvironment;

// 请求超时配置
const DEFAULT_TIMEOUT = 30000;

// 带超时的 fetch 包装器
const fetchWithTimeout = async (url, options = {}, timeout = DEFAULT_TIMEOUT) => {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeout);

  try {
    const response = await fetch(url, {
      ...options,
      signal: controller.signal,
      headers: {
        'Content-Type': 'application/json',
        ...options.headers
      }
    });

    clearTimeout(timeoutId);

    if (!response.ok) {
      let errorMessage = ERROR_MESSAGES.SERVER_ERROR;
      try {
        const errorData = await response.json();
        errorMessage = errorData.message || errorData.error || errorMessage;
      } catch {
        errorMessage = `HTTP ${response.status}: ${response.statusText}`;
      }
      throw new Error(errorMessage);
    }

    const contentType = response.headers.get('content-type');
    if (contentType && contentType.includes('application/json')) {
      return await response.json();
    }
    return await response.text();
  } catch (error) {
    clearTimeout(timeoutId);
    if (error.name === 'AbortError') {
      throw new Error(ERROR_MESSAGES.REQUEST_TIMEOUT);
    }
    if (error instanceof TypeError && error.message.includes('fetch')) {
      throw new Error(ERROR_MESSAGES.NETWORK_ERROR);
    }
    throw error;
  }
};

// ============================================
// 系统状态 API
// ============================================

export const fetchStatus = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getSystemStatus();
  }
  return await fetchWithTimeout(API_ENDPOINTS.STATUS);
};

export const fetchConnections = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getUsageSummary();
  }
  return await fetchWithTimeout(API_ENDPOINTS.CONNECTIONS);
};

// ============================================
// 端点管理 API
// ============================================

export const fetchEndpoints = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getEndpoints();
  }

  const data = await fetchWithTimeout(API_ENDPOINTS.ENDPOINTS);
  // 返回完整结构，包含 total, healthy 等统计信息
  // API 返回格式: { endpoints: [...], total: N, healthy: N }
  if (data.endpoints) {
    return {
      endpoints: data.endpoints,
      total: data.total ?? data.endpoints.length,
      healthy: data.healthy ?? data.endpoints.filter(e => e.status === 'healthy').length
    };
  }
  // 如果 API 直接返回数组
  const endpoints = Array.isArray(data) ? data : [];
  return {
    endpoints,
    total: endpoints.length,
    healthy: endpoints.filter(e => e.status === 'healthy').length
  };
};

export const checkEndpointHealth = async (endpointName) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.triggerHealthCheck(endpointName);
  }
  return await fetchWithTimeout(`${API_ENDPOINTS.ENDPOINT_HEALTH}/${endpointName}`, {
    method: 'POST'
  });
};

export const checkAllEndpointsHealth = async () => {
  // Wails 环境：使用后端批量健康检查 API
  if (isWailsEnvironment()) {
    const result = await WailsApi.batchHealthCheckAll();
    return {
      success: result.success,
      message: result.message || '批量健康检测完成',
      total: result.total,
      healthy_count: result.healthy_count,
      unhealthy_count: result.unhealthy_count
    };
  }
  return await fetchWithTimeout(API_ENDPOINTS.ENDPOINT_HEALTH, {
    method: 'POST'
  });
};

/**
 * 更新端点优先级
 * @param {string} endpointName - 端点名称
 * @param {number} priority - 新优先级
 */
export const updateEndpointPriority = async (endpointName, priority) => {
  if (!endpointName) throw new Error('端点名称不能为空');
  if (priority < 1) throw new Error('优先级必须大于等于1');

  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    await WailsApi.setEndpointPriority(endpointName, parseInt(priority));
    return { success: true, message: `端点 ${endpointName} 优先级已更新为 ${priority}` };
  }

  const data = await fetchWithTimeout(`/api/v1/endpoints/${encodeURIComponent(endpointName)}/priority`, {
    method: 'POST',
    body: JSON.stringify({ priority: parseInt(priority) })
  });

  if (!data.success) {
    throw new Error(data.error || '优先级更新失败');
  }

  return data;
};

/**
 * 获取 Keys 概览数据
 * 返回每个端点的 tokens 列表，用于级联选择器
 */
export const fetchKeysOverview = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getKeysOverview();
  }
  const data = await fetchWithTimeout(API_ENDPOINTS.KEYS_OVERVIEW);
  // API 返回格式: { endpoints: [{ endpoint: "name", tokens: [...] }, ...] }
  return data;
};

/**
 * 切换端点的 Token
 * @param {string} endpointName - 端点名称
 * @param {string} keyType - 'token' 或 'api_key'
 * @param {number} index - Token 索引
 */
export const switchKey = async (endpointName, keyType, index) => {
  if (!endpointName) throw new Error('端点名称不能为空');
  if (keyType !== 'token' && keyType !== 'api_key') throw new Error('无效的 Key 类型');
  if (typeof index !== 'number' || index < 0) throw new Error('无效的 Key 索引');

  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.switchKey(endpointName, keyType, index);
  }

  const apiPath = keyType === 'token'
    ? `/api/v1/endpoints/${encodeURIComponent(endpointName)}/keys/token`
    : `/api/v1/endpoints/${encodeURIComponent(endpointName)}/keys/api-key`;

  const data = await fetchWithTimeout(apiPath, {
    method: 'POST',
    body: JSON.stringify({ index })
  });

  if (!data.success) {
    throw new Error(data.error || 'Key 切换失败');
  }

  return data;
};

// ============================================
// 组管理 API
// ============================================

export const fetchGroups = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getGroups();
  }

  const data = await fetchWithTimeout(API_ENDPOINTS.GROUPS);
  // 返回完整结构，包含 groups 数组和 active_group 信息
  // API 返回格式: { groups: [...], active_group: "xxx" } 或直接数组
  const groups = data.groups || (Array.isArray(data) ? data : []);
  const activeGroup = groups.find(g => g.is_active);
  return {
    groups,
    active_group: activeGroup?.name || data.active_group || null,
    total_suspended_requests: data.total_suspended_requests || 0
  };
};

export const activateGroup = async (groupName) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.activateGroup(groupName);
  }
  const url = API_ENDPOINTS.GROUP_ACTIVATE.replace('{name}', groupName);
  return await fetchWithTimeout(url, { method: 'POST' });
};

export const pauseGroup = async (groupName) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.pauseGroup(groupName);
  }
  const url = API_ENDPOINTS.GROUP_PAUSE.replace('{name}', groupName);
  return await fetchWithTimeout(url, { method: 'POST' });
};

// ============================================
// 使用统计 API
// ============================================

export const fetchUsageStats = async (params = {}) => {
  const queryInput = {
    ...params,
    source_view: params.source_view || params.sourceView || 'all'
  };

  // Wails 环境使用新的 GetUsageStats 绑定（与 HTTP API 格式一致）
  // 传递完整筛选参数
  if (isWailsEnvironment()) {
    return await WailsApi.getUsageStats(queryInput);
  }

  const queryParams = new URLSearchParams();
  Object.entries(queryInput).forEach(([key, value]) => {
    if (value !== null && value !== undefined && value !== '') {
      queryParams.append(key, value.toString());
    }
  });

  const url = queryParams.toString()
    ? `${API_ENDPOINTS.USAGE_STATS}?${queryParams.toString()}`
    : API_ENDPOINTS.USAGE_STATS;

  return await fetchWithTimeout(url);
};

export const fetchRequests = async (params = {}) => {
  const queryInput = {
    ...params,
    source_view: params.source_view || params.sourceView || 'all'
  };

  // 标准化请求数据（提取到外部，Wails和HTTP环境共用）
  const normalizeRequest = (request) => ({
    ...request,
    upstreamType: request.upstream_type || request.upstreamType || 'endpoint',
    upstreamName: request.upstream_name || request.upstreamName || '',
    upstreamSourceName: request.upstream_source_name || request.upstreamSourceName || '',
    upstreamId: request.upstream_id || request.upstreamId || null,
    requestId: request.request_id || request.requestId || request.id,
    id: request.request_id || request.requestId || request.id,
    timestamp: request.start_time || request.timestamp,
    model: request.model_name || request.model || 'unknown',
    channel: request.channel || '',
    endpoint: request.endpoint_name || request.endpoint || request.upstream_name || request.upstreamName || 'unknown',
    group: request.group_name || request.group || request.upstream_source_name || request.upstreamSourceName || 'default',
    duration: request.duration_ms || request.duration || 0,
    firstTokenMs: request.first_token_ms ?? request.firstTokenMs ?? null,
    inputTokens: request.input_tokens || request.inputTokens || 0,
    outputTokens: request.output_tokens || request.outputTokens || 0,
    cacheCreationTokens: request.cache_creation_tokens || request.cacheCreationTokens || 0,
    cacheCreation5mTokens: request.cache_creation_5m_tokens || request.cacheCreation5mTokens || 0, // v5.0.1+
    cacheCreation1hTokens: request.cache_creation_1h_tokens || request.cacheCreation1hTokens || 0, // v5.0.1+
    cacheReadTokens: request.cache_read_tokens || request.cacheReadTokens || 0,
    cost: request.total_cost_usd || request.cost || 0,
    isStreaming: request.is_streaming || request.isStreaming || false,
    statusCode: request.status_code || request.statusCode
  });

  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    // 直接传递所有参数给 wailsApi
    const data = await WailsApi.getRequests(queryInput);

    // 对 Wails 数据也进行规范化处理
    const requests = data.requests || [];
    const normalizedRequests = Array.isArray(requests) ? requests.map(normalizeRequest) : [];

    return {
      requests: normalizedRequests,
      total: data.total || normalizedRequests.length,
      page: data.page || 1,
      pageSize: data.pageSize || parseInt(queryInput.limit || queryInput.pageSize || 50),
      totalPages: data.totalPages || Math.ceil((data.total || 0) / (data.pageSize || parseInt(queryInput.limit || queryInput.pageSize || 50)))
    };
  }

  const queryParams = new URLSearchParams();
  Object.entries(queryInput).forEach(([key, value]) => {
    if (value !== null && value !== undefined && value !== '') {
      queryParams.append(key, value.toString());
    }
  });

  // 确保固定排序参数
  if (!queryParams.has('sort_by')) {
    queryParams.set('sort_by', 'start_time');
  }
  if (!queryParams.has('sort_order')) {
    queryParams.set('sort_order', 'desc');
  }

  const url = queryParams.toString()
    ? `${API_ENDPOINTS.USAGE_REQUESTS}?${queryParams.toString()}`
    : API_ENDPOINTS.USAGE_REQUESTS;

  const data = await fetchWithTimeout(url);

  const requests = data.requests || data.data || data || [];
  const normalizedRequests = Array.isArray(requests) ? requests.map(normalizeRequest) : [];

  return {
    requests: normalizedRequests,
    total: data.total || normalizedRequests.length,
    page: data.page || 1,
    pageSize: data.pageSize || data.limit || 50,
    totalPages: data.totalPages || Math.ceil((data.total || 0) / (data.pageSize || data.limit || 50))
  };
};

/**
 * 获取模型列表（用于筛选器）
 * v5.0: 从 model_pricing 表获取，而不是从使用记录
 * @returns {Promise<Array>} - 模型名称数组
 */
export const fetchModels = async () => {
  // Wails 环境：从 model_pricing 表获取
  if (isWailsEnvironment()) {
    try {
      const pricings = await WailsApi.getModelPricings();
      // 返回模型名称数组，保持向后兼容
      return (pricings || []).map(p => ({
        model_name: p.modelName,
        name: p.modelName,
        display_name: p.displayName || p.modelName
      }));
    } catch (err) {
      console.warn('获取模型定价失败，降级到使用记录:', err);
    }
  }

  // HTTP 环境或 Wails 降级：从使用记录获取
  const data = await fetchWithTimeout(API_ENDPOINTS.USAGE_MODELS);
  if (data.success && data.data) return data.data;
  if (data.models) return data.models;
  if (Array.isArray(data)) return data;
  return [];
};

// ============================================
// 配置 API
// ============================================

export const fetchConfig = async () => {
  return await fetchWithTimeout(API_ENDPOINTS.CONFIG);
};

// ============================================
// 图表数据 API
// ============================================

/**
 * 将 Chart.js 格式转换为 Recharts 格式
 * Chart.js: { labels: [...], datasets: [{data: [...]}, ...] }
 * Recharts: [{ time: '...', total: N, success: N, fail: N }, ...]
 */
const transformChartJsToRecharts = (chartJsData, keyMapping) => {
  if (!chartJsData?.labels || !chartJsData?.datasets) {
    return [];
  }

  const { labels, datasets } = chartJsData;
  return labels.map((label, index) => {
    const point = { time: label };
    datasets.forEach((dataset, datasetIndex) => {
      const key = keyMapping[datasetIndex] || `value${datasetIndex}`;
      point[key] = dataset.data?.[index] ?? 0;
    });
    return point;
  });
};

/**
 * 获取请求趋势数据
 * @param {number} minutes - 时间范围（分钟），默认 30
 */
export const fetchRequestTrendData = async (minutes = 30) => {
  try {
    // Wails 环境使用绑定
    if (isWailsEnvironment()) {
      return await WailsApi.getRequestTrendChart(minutes);
    }

    const data = await fetchWithTimeout(`/api/v1/chart/request-trends?minutes=${minutes}`);
    // 转换为 Recharts 格式: total, success, fail
    return transformChartJsToRecharts(data, ['total', 'success', 'fail']);
  } catch (error) {
    console.error('获取请求趋势数据失败:', error);
    return [];
  }
};

/**
 * 获取响应时间数据
 * @param {number} minutes - 时间范围（分钟），默认 30
 */
export const fetchResponseTimeData = async (minutes = 30) => {
  try {
    // Wails 环境使用绑定
    if (isWailsEnvironment()) {
      return await WailsApi.getResponseTimeChart(minutes);
    }

    const data = await fetchWithTimeout(`/api/v1/chart/response-times?minutes=${minutes}`);
    // 转换为 Recharts 格式: avg, min, max
    return transformChartJsToRecharts(data, ['avg', 'min', 'max']);
  } catch (error) {
    console.error('获取响应时间数据失败:', error);
    return [];
  }
};

/**
 * 获取 Token 使用数据
 */
export const fetchTokenUsageData = async () => {
  try {
    // Wails 环境使用绑定
    if (isWailsEnvironment()) {
      return await WailsApi.getTokenUsage();
    }

    const data = await fetchWithTimeout('/api/v1/tokens/usage');
    const current = data.current || data;
    return {
      input: current.input_tokens || 0,
      output: current.output_tokens || 0,
      cacheCreation: current.cache_creation_tokens || 0,
      cacheRead: current.cache_read_tokens || 0
    };
  } catch (error) {
    console.error('获取 Token 使用数据失败:', error);
    return { input: 0, output: 0, cacheCreation: 0, cacheRead: 0 };
  }
};

/**
 * 获取端点健康状态数据
 */
export const fetchEndpointHealthData = async () => {
  try {
    // Wails 环境使用绑定
    if (isWailsEnvironment()) {
      return await WailsApi.getEndpointHealthChart();
    }

    const data = await fetchWithTimeout('/api/v1/chart/endpoint-health');
    // 返回 { healthy: N, unhealthy: N } 或原始 Chart.js 格式
    if (data.labels && data.datasets) {
      const [healthy, unhealthy] = data.datasets[0]?.data || [0, 0];
      return { healthy, unhealthy };
    }
    return data;
  } catch (error) {
    console.error('获取端点健康状态数据失败:', error);
    return { healthy: 0, unhealthy: 0 };
  }
};

/**
 * 获取端点成本数据
 */
export const fetchEndpointCostsData = async () => {
  try {
    // Wails 环境使用绑定
    if (isWailsEnvironment()) {
      return await WailsApi.getEndpointCosts();
    }

    const data = await fetchWithTimeout('/api/v1/chart/endpoint-costs');
    // 转换为 Recharts 格式: { name, tokens, cost }
    if (data.labels && data.datasets) {
      // 查找 Token 数据集（label 包含 "Token" 或 "token"）
      const tokensDataset = data.datasets.find(d =>
        d.label?.toLowerCase().includes('token')
      );
      // 查找成本数据集（label 包含 "成本" 或 "Cost" 或 "USD"）
      const costDataset = data.datasets.find(d =>
        d.label?.includes('成本') || d.label?.toLowerCase().includes('cost') || d.label?.includes('USD')
      );

      const tokensData = tokensDataset?.data || [];
      const costData = costDataset?.data || [];

      return data.labels.map((name, i) => ({
        name,
        tokens: tokensData[i] || 0,
        cost: costData[i] || 0
      }));
    }
    return [];
  } catch (error) {
    console.error('获取端点成本数据失败:', error);
    return [];
  }
};

/**
 * 获取连接活动数据
 * @param {number} minutes - 时间范围（分钟），默认 60
 */
export const fetchConnectionActivityData = async (minutes = 60) => {
  try {
    // Wails 环境使用绑定
    if (isWailsEnvironment()) {
      return await WailsApi.getConnectionActivityChart(minutes);
    }

    const data = await fetchWithTimeout(`/api/v1/chart/connection-activity?minutes=${minutes}`);
    return transformChartJsToRecharts(data, ['connections']);
  } catch (error) {
    console.error('获取连接活动数据失败:', error);
    return [];
  }
};

// ============================================
// 导出工具函数
// ============================================

export const formatUptime = (seconds) => {
  if (typeof seconds !== 'number' || seconds <= 0) return seconds;

  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  let result = '';
  if (hours > 0) result += `${hours}h `;
  if (minutes > 0) result += `${minutes}m `;
  if (secs > 0 || result === '') result += `${secs}s`;

  return result.trim();
};

export const formatNumber = (num) => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
  return num.toString();
};

export const formatCost = (cost) => {
  if (cost >= 1) return `$${cost.toFixed(2)}`;
  if (cost >= 0.01) return `$${cost.toFixed(3)}`;
  return `$${cost.toFixed(4)}`;
};

export const formatDuration = (ms) => {
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
  return `${ms}ms`;
};

// 格式化时间戳
// 后端返回的时间已经是配置时区的时间（如北京时间），格式：2025-12-04 17:18:48
// 直接格式化显示，不做时区转换
export const formatTimestamp = (timestamp) => {
  if (!timestamp) return 'N/A';

  // 如果是纯时间格式（无T、无时区），直接格式化显示
  // 格式：2025-12-04 17:18:48 → 2025/12/4 17:18:48
  if (typeof timestamp === 'string' && !timestamp.includes('T') && !timestamp.includes('+') && !timestamp.includes('Z')) {
    // 解析 "2025-12-04 17:18:48" 格式
    const match = timestamp.match(/^(\d{4})-(\d{2})-(\d{2})\s+(\d{2}):(\d{2}):(\d{2})/);
    if (match) {
      const [, year, month, day, hours, minutes, seconds] = match;
      // 直接显示，不转换时区
      return `${year}/${parseInt(month)}/${parseInt(day)} ${hours}:${minutes}:${seconds}`;
    }
  }

  // 带时区的格式（RFC3339），用 Date 解析
  const date = new Date(timestamp);
  if (isNaN(date.getTime())) return 'Invalid Date';

  const year = date.getFullYear();
  const month = date.getMonth() + 1;
  const day = date.getDate();
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');
  const seconds = String(date.getSeconds()).padStart(2, '0');

  return `${year}/${month}/${day} ${hours}:${minutes}:${seconds}`;
};

// ============================================
// v5.0+ 端点存储 API (SQLite)
// ============================================

/**
 * 获取端点存储状态
 * @returns {Promise<Object>} - {enabled, storageType, totalCount, enabledCount}
 */
export const fetchEndpointStorageStatus = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getEndpointStorageStatus();
  }
  // HTTP 环境暂不支持
  return { enabled: false, storageType: 'yaml', totalCount: 0, enabledCount: 0 };
};

/**
 * 获取所有端点记录（SQLite 存储）
 * @returns {Promise<Array>} - 端点记录数组
 */
export const fetchEndpointRecords = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getEndpointRecords();
  }
  // HTTP 环境暂不支持
  return [];
};

/**
 * 创建端点记录
 * @param {Object} input - 端点配置
 * @returns {Promise<Object>}
 */
export const createEndpoint = async (input) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.createEndpointRecord(input);
  }
  throw new Error('HTTP 环境暂不支持端点存储功能');
};

/**
 * 更新端点配置
 * @param {string} name - 端点名称
 * @param {Object} input - 更新的配置
 * @returns {Promise<Object>}
 */
export const updateEndpoint = async (name, input) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.updateEndpointRecord(name, input);
  }
  throw new Error('HTTP 环境暂不支持端点存储功能');
};

/**
 * 删除端点
 * @param {string} name - 端点名称
 * @returns {Promise<Object>}
 */
export const deleteEndpoint = async (name) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.deleteEndpointRecord(name);
  }
  throw new Error('HTTP 环境暂不支持端点存储功能');
};

/**
 * 切换端点启用状态
 * @param {string} name - 端点名称
 * @param {boolean} enabled - 是否启用
 * @returns {Promise<Object>}
 */
export const toggleEndpoint = async (name, enabled) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.toggleEndpointRecord(name, enabled);
  }
  throw new Error('HTTP 环境暂不支持端点存储功能');
};

/**
 * 获取所有渠道
 * @returns {Promise<Array>} - 渠道列表
 */
export const fetchChannels = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getChannels();
  }
  return [];
};

// ============================================
// v6.0+ 账号池 API
// ============================================

const normalizeUpstreamAccountList = (data) => {
  const list = Array.isArray(data)
    ? data
    : (data?.accounts || data?.upstream_accounts || data?.data || []);

  return (Array.isArray(list) ? list : []).map(WailsApi.normalizeUpstreamAccount);
};

const normalizeEntityIdForUrl = (id) => encodeURIComponent(String(id));

export const fetchUpstreamAccounts = async () => {
  if (isWailsEnvironment()) {
    return await WailsApi.getUpstreamAccounts();
  }

  const data = await fetchWithTimeout('/api/v1/upstream-accounts');
  return normalizeUpstreamAccountList(data);
};

export const fetchLatestAccountScheduleSnapshot = async () => {
  if (isWailsEnvironment()) {
    return await WailsApi.getLatestAccountScheduleSnapshot();
  }

  return {
    unsupported: true,
    has_snapshot: false,
    hasSnapshot: false,
    candidates: [],
    message: '仅桌面版支持最近一次调度结果面板'
  };
};

export const createUpstreamAccount = async (input) => {
  const payload = WailsApi.buildUpstreamAccountPayload(input);

  if (isWailsEnvironment()) {
    return await WailsApi.createUpstreamAccount(payload);
  }

  return await fetchWithTimeout('/api/v1/upstream-accounts', {
    method: 'POST',
    body: JSON.stringify(payload)
  });
};

export const updateUpstreamAccount = async (id, input) => {
  const payload = WailsApi.buildUpstreamAccountPayload(input);

  if (isWailsEnvironment()) {
    return await WailsApi.updateUpstreamAccount(id, payload);
  }

  return await fetchWithTimeout(`/api/v1/upstream-accounts/${normalizeEntityIdForUrl(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload)
  });
};

export const fetchUpstreamAccountCredential = async (id) => {
  if (isWailsEnvironment()) {
    return await WailsApi.getUpstreamAccountCredential(id);
  }

  return await fetchWithTimeout(`/api/v1/upstream-accounts/${normalizeEntityIdForUrl(id)}/credential`);
};

export const deleteUpstreamAccount = async (id) => {
  if (isWailsEnvironment()) {
    return await WailsApi.deleteUpstreamAccount(id);
  }

  return await fetchWithTimeout(`/api/v1/upstream-accounts/${normalizeEntityIdForUrl(id)}`, {
    method: 'DELETE'
  });
};

export const moveUpstreamAccountToTier = async (id, targetTier) => {
  if (isWailsEnvironment()) {
    return await WailsApi.moveUpstreamAccountToTier(id, targetTier);
  }

  return await fetchWithTimeout(`/api/v1/upstream-accounts/${normalizeEntityIdForUrl(id)}/tier`, {
    method: 'POST',
    body: JSON.stringify({ target_tier: targetTier || 'primary' })
  });
};

export const toggleUpstreamAccount = async (id, enabled) => {
  if (isWailsEnvironment()) {
    return await WailsApi.toggleUpstreamAccount(id, enabled);
  }

  return await fetchWithTimeout(`/api/v1/upstream-accounts/${normalizeEntityIdForUrl(id)}/toggle`, {
    method: 'POST',
    body: JSON.stringify({ enabled: enabled !== false })
  });
};

export const testUpstreamAccount = async (id) => {
  if (isWailsEnvironment()) {
    return await WailsApi.testUpstreamAccount(id);
  }

  try {
    return await fetchWithTimeout(`/api/v1/upstream-accounts/${normalizeEntityIdForUrl(id)}/test`, {
      method: 'POST'
    });
  } catch (error) {
    const errorText = error?.message || String(error || '');
    if (/404|not found/i.test(errorText)) {
      return {
        success: false,
        unsupported: true,
        message: '当前后端版本暂未提供 TestUpstreamAccount'
      };
    }
    throw error;
  }
};

export const refreshUpstreamAccountProfile = async (id) => {
  if (isWailsEnvironment()) {
    return await WailsApi.refreshUpstreamAccountProfile(id);
  }

  try {
    return await fetchWithTimeout(`/api/v1/upstream-accounts/${normalizeEntityIdForUrl(id)}/profile/refresh`, {
      method: 'POST'
    });
  } catch (error) {
    const errorText = error?.message || String(error || '');
    if (/404|not found/i.test(errorText)) {
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
  if (isWailsEnvironment()) {
    return await WailsApi.generateChatGPTOAuthLink();
  }
  return {
    success: false,
    unsupported: true,
    message: '仅桌面版支持此功能'
  };
};

export const exchangeChatGPTOAuthCallback = async (sessionId, callbackUrl) => {
  if (isWailsEnvironment()) {
    return await WailsApi.exchangeChatGPTOAuthCallback(sessionId, callbackUrl);
  }
  return {
    success: false,
    unsupported: true,
    message: '仅桌面版支持此功能'
  };
};

// ============================================
// v5.1+ 系统设置 API (SQLite)
// ============================================

/**
 * 获取设置分类列表
 * @returns {Promise<Array>} - 分类信息数组
 */
export const fetchSettingCategories = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getSettingCategories();
  }
  // HTTP 环境
  const data = await fetchWithTimeout('/api/v1/settings/categories');
  return data.categories || data || [];
};

/**
 * 获取所有设置
 * @returns {Promise<Array>} - 设置记录数组
 */
export const fetchAllSettings = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getAllSettings();
  }
  // HTTP 环境
  const data = await fetchWithTimeout('/api/v1/settings');
  return data.settings || data || [];
};

/**
 * 获取指定分类的设置
 * @param {string} category - 分类名称
 * @returns {Promise<Array>} - 设置记录数组
 */
export const fetchSettingsByCategory = async (category) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getSettingsByCategory(category);
  }
  // HTTP 环境
  const data = await fetchWithTimeout(`/api/v1/settings/${encodeURIComponent(category)}`);
  return data.settings || data || [];
};

/**
 * 批量更新设置
 * @param {Array} settings - [{category, key, value}, ...]
 * @returns {Promise<Object>}
 */
export const updateSettings = async (settings) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.batchUpdateSettings({ settings });
  }
  // HTTP 环境
  return await fetchWithTimeout('/api/v1/settings', {
    method: 'PUT',
    body: JSON.stringify({ settings })
  });
};

/**
 * 重置分类设置为默认值
 * @param {string} category - 分类名称
 * @returns {Promise<Object>}
 */
export const resetSettings = async (category) => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.resetCategorySettings(category);
  }
  // HTTP 环境
  return await fetchWithTimeout(`/api/v1/settings/${encodeURIComponent(category)}/reset`, {
    method: 'POST'
  });
};

/**
 * 获取端口信息
 * @returns {Promise<Object>} - {preferred_port, actual_port, was_occupied}
 */
export const fetchPortInfo = async () => {
  // Wails 环境使用绑定
  if (isWailsEnvironment()) {
    return await WailsApi.getPortInfo();
  }
  // HTTP 环境
  const data = await fetchWithTimeout('/api/v1/settings/port');
  return data;
};
