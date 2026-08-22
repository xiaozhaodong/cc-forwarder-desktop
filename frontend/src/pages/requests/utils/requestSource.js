import { NO_UPSTREAM_LABEL, isNoUpstreamReason } from './lifecycle.js';

export const REQUEST_FAMILY_META = {
  claude: { label: 'Claude', className: 'border-violet-200 bg-violet-50 text-violet-700' },
  codex: { label: 'Codex', className: 'border-sky-200 bg-sky-50 text-sky-700' },
  image: { label: 'Image', className: 'border-fuchsia-200 bg-fuchsia-50 text-fuchsia-700' },
  other: { label: 'Other', className: 'border-slate-200 bg-slate-50 text-slate-600' }
};

export const normalizeRequestFamily = (value) => {
  const normalized = String(value || '').trim().toLowerCase();
  return REQUEST_FAMILY_META[normalized] ? normalized : 'other';
};

export const getRequestFamilyMeta = (value) => REQUEST_FAMILY_META[normalizeRequestFamily(value)];

export const UNKNOWN_UPSTREAM_LABEL = '未知上游';

// 合成上游标签：不是真实上游名，DB 里对应的 upstream_name 为空。
// 上游筛选按 upstream_name 精确查库，放进选项必然返回 0 条，故排除出候选。
const SYNTHETIC_UPSTREAM_LABELS = new Set([UNKNOWN_UPSTREAM_LABEL, NO_UPSTREAM_LABEL]);

// 空上游归属：候选为空时后端没有端点可归属（endpoint_pipeline.go 的 no_endpoints_available 分支），
// 一律显示「未知上游」会被读成数据丢失，而实际是明确的「没有端点可路由」。
// 归类规则复用 lifecycle.js 的终态规则表，保证与生命周期分段条标签同源。
const resolveMissingUpstreamLabel = (request = {}) => (
  isNoUpstreamReason(request.failureReason || request.failure_reason)
    ? NO_UPSTREAM_LABEL
    : UNKNOWN_UPSTREAM_LABEL
);

export const resolveRequestUpstream = (request = {}) => (
  request.upstreamName
  || request.upstream_name
  || request.effectiveEndpoint
  || request.effective_endpoint
  || request.endpoint
  || request.endpoint_name
  || resolveMissingUpstreamLabel(request)
);

export const normalizeRequestSource = (request = {}) => ({
  ...request,
  requestFamily: normalizeRequestFamily(request.requestFamily || request.request_family),
  upstreamName: resolveRequestUpstream(request)
});

export const filterUpstreamOptionsByFamily = (requests = [], family = 'all') => {
  const selectedFamily = family === 'all' ? '' : normalizeRequestFamily(family);
  const names = new Set();
  for (const request of requests) {
    const normalized = normalizeRequestSource(request);
    if (selectedFamily && normalized.requestFamily !== selectedFamily) continue;
    if (normalized.upstreamName && !SYNTHETIC_UPSTREAM_LABELS.has(normalized.upstreamName)) {
      names.add(normalized.upstreamName);
    }
  }
  return [...names].sort((left, right) => left.localeCompare(right, 'zh-Hans-CN'));
};
