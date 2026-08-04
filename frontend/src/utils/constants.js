// ============================================
// API 常量定义
// 2025-11-28
// ============================================

// API 端点配置
export const API_ENDPOINTS = {
  // 状态相关
  STATUS: '/api/v1/status',
  CONNECTIONS: '/api/v1/connections',

  // 端点管理
  ENDPOINTS: '/api/v1/endpoints',
  ENDPOINT_HEALTH: '/api/v1/endpoints/health',

  // 使用统计
  USAGE_REQUESTS: '/api/v1/usage/requests',
  USAGE_STATS: '/api/v1/usage/stats',
  USAGE_MODELS: '/api/v1/usage/models',
  USAGE_EXPORT: '/api/v1/usage/export',

  // 实时流
  STREAM: '/api/v1/stream',

  // 配置仅通过已绑定的 Wails API 读取，不在代理端口暴露管理接口。
};

// 请求状态配置 (v3.5.0 状态机)
export const REQUEST_STATUS = {
  PENDING: 'pending',
  FORWARDING: 'forwarding',
  PROCESSING: 'processing',
  RETRY: 'retry',
  SUSPENDED: 'suspended',
  COMPLETED: 'completed',
  FAILED: 'failed',
  ERROR: 'error',
  CANCELLED: 'cancelled',
  TIMEOUT: 'timeout'
};

// 状态显示配置
export const STATUS_CONFIG = {
  [REQUEST_STATUS.PENDING]: {
    label: '等待中',
    icon: 'Clock',
    color: 'bg-blue-100 text-blue-700 border-blue-200'
  },
  [REQUEST_STATUS.FORWARDING]: {
    label: '转发中',
    icon: 'ArrowRightCircle',
    color: 'bg-blue-100 text-blue-700 border-blue-200'
  },
  [REQUEST_STATUS.PROCESSING]: {
    label: '处理中',
    icon: 'Settings',
    color: 'bg-orange-100 text-orange-700 border-orange-200'
  },
  [REQUEST_STATUS.RETRY]: {
    label: '重试中',
    icon: 'RotateCw',
    color: 'bg-amber-100 text-amber-700 border-amber-200'
  },
  [REQUEST_STATUS.SUSPENDED]: {
    label: '已挂起',
    icon: 'Pause',
    color: 'bg-slate-100 text-slate-700 border-slate-200'
  },
  [REQUEST_STATUS.COMPLETED]: {
    label: '已完成',
    icon: 'CheckCircle2',
    color: 'bg-emerald-100 text-emerald-700 border-emerald-200'
  },
  [REQUEST_STATUS.FAILED]: {
    label: '已失败',
    icon: 'XCircle',
    color: 'bg-rose-100 text-rose-700 border-rose-200'
  },
  [REQUEST_STATUS.ERROR]: {
    label: '已失败',
    icon: 'XCircle',
    color: 'bg-rose-100 text-rose-700 border-rose-200'
  },
  [REQUEST_STATUS.CANCELLED]: {
    label: '已取消',
    icon: 'Ban',
    color: 'bg-slate-100 text-slate-700 border-slate-200'
  },
  [REQUEST_STATUS.TIMEOUT]: {
    label: '已超时',
    icon: 'Timer',
    color: 'bg-slate-100 text-slate-700 border-slate-200'
  }
};

// 筛选器状态选项
export const FILTER_STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'pending', label: '等待中' },
  { value: 'forwarding', label: '转发中' },
  { value: 'processing', label: '处理中' },
  { value: 'retry', label: '重试中' },
  { value: 'suspended', label: '已挂起' },
  { value: 'completed', label: '已完成' },
  { value: 'failed', label: '已失败' },
  { value: 'cancelled', label: '已取消' }
];

// 分页配置
export const PAGINATION_CONFIG = {
  DEFAULT_PAGE_SIZE: 50,
  PAGE_SIZE_OPTIONS: [20, 50, 100, 200]
};

// 错误消息
export const ERROR_MESSAGES = {
  FETCH_FAILED: '获取数据失败',
  NETWORK_ERROR: '网络连接错误',
  REQUEST_TIMEOUT: '请求超时',
  SERVER_ERROR: '服务器错误'
};

// 获取状态配置
export const getStatusConfig = (status) => {
  return STATUS_CONFIG[status] || {
    label: status || '未知',
    icon: '❓',
    color: 'bg-slate-100 text-slate-700 border-slate-200'
  };
};
