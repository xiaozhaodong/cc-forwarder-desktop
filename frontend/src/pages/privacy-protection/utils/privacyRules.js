// ============================================
// 隐私保护页面纯工具函数
// 2026-06-11 (v6.1 新增)
// ============================================

export const PRIVACY_MODE_OPTIONS = [
  { value: 'disabled', label: '关闭' },
  { value: 'detect', label: '仅检测' },
  { value: 'redact', label: '脱敏转发' }
];

export const PRIVACY_MATCH_TYPE_OPTIONS = [
  { value: 'literal', label: '字面匹配' },
  { value: 'regex', label: '正则' }
];

export const PRIVACY_ACTION_OPTIONS = [
  { value: 'detect', label: '仅检测' },
  { value: 'redact', label: '脱敏' }
];

export const PRIVACY_PATH_OPTIONS = [
  '/v1/messages',
  '/v1/messages/count_tokens',
  '/v1/responses',
  '/v1/responses/compact'
];

export const PRIVACY_UPSTREAM_TYPE_OPTIONS = [
  { value: 'endpoint', label: 'CC 端点' },
  { value: 'account', label: 'Codex 账号' }
];

export const PRIVACY_PROVIDER_TYPE_OPTIONS = [
  { value: 'api_key', label: 'API Key' },
  { value: 'chatgpt_oauth', label: 'ChatGPT OAuth' }
];

export const PRIVACY_OVER_LIMIT_OPTIONS = [
  { value: 'scan_prefix', label: '截断扫描并提示' },
  { value: 'fail_closed', label: '超限拒绝' }
];

export const PRIVACY_ON_ERROR_OPTIONS = [
  { value: 'fail_open', label: '出错放行' },
  { value: 'fail_closed', label: '出错拒绝' }
];

export const DEFAULT_SCAN_MAX_BYTES = 4194304;

// 格式化扫描上限为可读字符串
export const formatScanBytes = (bytes) => {
  const value = Number(bytes);
  if (!Number.isFinite(value) || value <= 0) return '-';
  if (value >= 1024 * 1024) {
    const mb = value / (1024 * 1024);
    return `${Number.isInteger(mb) ? mb : mb.toFixed(1)} MB`;
  }
  if (value >= 1024) {
    const kb = value / 1024;
    return `${Number.isInteger(kb) ? kb : kb.toFixed(1)} KB`;
  }
  return `${value} B`;
};

// 作用域摘要：空维度=不限
export const summarizeScope = (scope = {}) => {
  const parts = [];
  if (Array.isArray(scope.paths) && scope.paths.length > 0) {
    parts.push(`路径 ${scope.paths.length}`);
  }
  if (Array.isArray(scope.upstream_types) && scope.upstream_types.length > 0) {
    const labels = scope.upstream_types.map((item) => {
      const found = PRIVACY_UPSTREAM_TYPE_OPTIONS.find((opt) => opt.value === item);
      return found ? found.label : item;
    });
    parts.push(labels.join('/'));
  }
  if (Array.isArray(scope.endpoint_names) && scope.endpoint_names.length > 0) {
    parts.push(`端点 ${scope.endpoint_names.length}`);
  }
  if (Array.isArray(scope.account_ids) && scope.account_ids.length > 0) {
    parts.push(`账号 ${scope.account_ids.length}`);
  }
  if (Array.isArray(scope.provider_types) && scope.provider_types.length > 0) {
    parts.push(`类型 ${scope.provider_types.length}`);
  }
  return parts.length > 0 ? parts.join(' · ') : '全部请求';
};

// 规则筛选：搜索（名称/描述/pattern）、启用状态、匹配类型、动作、来源
export const filterPrivacyRules = (rules = [], filters = {}) => {
  const keyword = String(filters.keyword ?? '').trim().toLowerCase();
  return rules.filter((rule) => {
    if (keyword) {
      const haystack = `${rule.name} ${rule.description} ${rule.pattern}`.toLowerCase();
      if (!haystack.includes(keyword)) return false;
    }
    if (filters.enabled === 'enabled' && !rule.enabled) return false;
    if (filters.enabled === 'disabled' && rule.enabled) return false;
    if (filters.matchType && rule.match_type !== filters.matchType) return false;
    if (filters.action && rule.action !== filters.action) return false;
    if (filters.source && rule.source !== filters.source) return false;
    return true;
  });
};

// 表单校验（regex 编译校验由后端兜底，这里做基础前端校验）
export const validatePrivacyRuleForm = (form = {}) => {
  const errors = {};
  if (!String(form.name ?? '').trim()) {
    errors.name = '规则名不能为空';
  }
  if (!String(form.pattern ?? '')) {
    errors.pattern = 'Pattern 不能为空';
  }
  if (form.action === 'redact' && !String(form.placeholder ?? '')) {
    errors.placeholder = '脱敏动作必须填写占位符';
  }
  const priority = Number(form.priority);
  if (!Number.isFinite(priority) || priority < 0) {
    errors.priority = '优先级必须是非负数字';
  }
  return errors;
};

export const createEmptyPrivacyRuleForm = () => ({
  id: 0,
  enabled: true,
  name: '',
  description: '',
  priority: 100,
  match_type: 'regex',
  pattern: '',
  placeholder: '[已脱敏]',
  action: 'redact',
  scope: {
    paths: [],
    upstream_types: [],
    endpoint_names: [],
    account_ids: [],
    provider_types: []
  }
});

export const ruleToForm = (rule = {}) => ({
  ...createEmptyPrivacyRuleForm(),
  ...rule,
  scope: {
    ...createEmptyPrivacyRuleForm().scope,
    ...(rule.scope || {})
  }
});

// 复制规则：名称追加“副本”，清空 id
export const duplicateRuleForm = (rule = {}) => ({
  ...ruleToForm(rule),
  id: 0,
  name: `${rule.name || ''} 副本`.trim(),
  source: 'custom'
});

// 上移/下移后重排优先级（步长 10），返回 ReorderPrivacyRules 载荷
export const buildReorderPayload = (rules = []) =>
  rules.map((rule, index) => ({
    id: rule.id,
    priority: (index + 1) * 10
  }));

// 在数组内移动规则（direction: -1 上移 / 1 下移），返回新数组；越界返回原数组
export const moveRuleInList = (rules = [], ruleId, direction) => {
  const index = rules.findIndex((rule) => rule.id === ruleId);
  if (index < 0) return rules;
  const target = index + direction;
  if (target < 0 || target >= rules.length) return rules;
  const next = [...rules];
  [next[index], next[target]] = [next[target], next[index]];
  return next;
};
