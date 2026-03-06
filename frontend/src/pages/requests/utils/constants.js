// ============================================
// 请求追踪页面 - 常量定义
// 2025-11-28 17:02:47
// ============================================

// 状态选项配置 (v3.5.0 状态机)
export const STATUS_OPTIONS = {
  all: '全部状态',
  pending: '等待中',
  forwarding: '转发中',
  processing: '处理中',
  retry: '重试中',
  suspended: '已挂起',
  completed: '已完成',
  failed: '已失败',
  cancelled: '已取消'
};

// 转换为 Select 组件需要的格式
export const STATUS_SELECT_OPTIONS = Object.entries(STATUS_OPTIONS).map(([value, label]) => ({
  value,
  label
}));

// 分页配置
export const PAGINATION_CONFIG = {
  DEFAULT_PAGE_SIZE: 10,
  PAGE_SIZE_OPTIONS: [10, 20, 50, 100]
};

// 默认筛选器状态（v4.0: 移除group筛选）
export const DEFAULT_FILTERS = {
  status: 'all',
  model: '',
  channel: 'all',
  endpoint: 'all',
  startDate: '',
  endDate: ''
};

// 表格列定义
export const TABLE_COLUMNS = [
  { id: 'requestId', label: '请求ID', alwaysVisible: true, width: 'auto' },
  { id: 'timestamp', label: '时间', alwaysVisible: true, width: 'auto' },
  { id: 'status', label: '状态', alwaysVisible: false, width: 'auto' },
  { id: 'model', label: '模型', alwaysVisible: false, width: 'auto' },
  { id: 'channel', label: '渠道', alwaysVisible: false, width: 'auto' },
  { id: 'endpoint', label: '端点', alwaysVisible: false, width: 'auto' },
  { id: 'duration', label: '耗时', alwaysVisible: false, width: 'auto' },
  { id: 'inputTokens', label: '输入', alwaysVisible: false, width: 'auto', align: 'right' },
  { id: 'outputTokens', label: '输出', alwaysVisible: false, width: 'auto', align: 'right' },
  { id: 'cacheCreationTokens', label: '缓存创建', alwaysVisible: false, width: 'auto', align: 'right' },
  { id: 'cacheReadTokens', label: '缓存读取', alwaysVisible: false, width: 'auto', align: 'right' },
  { id: 'cost', label: '成本', alwaysVisible: false, width: 'auto', align: 'right' }
];

// 默认可见的列（v4.0: 添加缓存创建/读取列）
export const DEFAULT_VISIBLE_COLUMNS = [
  'requestId', 'timestamp', 'status', 'model', 'channel', 'endpoint',
  'duration', 'inputTokens', 'outputTokens', 'cacheCreationTokens', 'cacheReadTokens', 'cost'
];

// 时间范围快捷选项
export const TIME_RANGE_OPTIONS = [
  { value: 'today', label: '今天' },
  { value: '7days', label: '7天' },
  { value: '30days', label: '30天' }
];
