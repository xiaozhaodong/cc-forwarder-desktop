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

export const resolveRequestUpstream = (request = {}) => (
  request.upstreamName
  || request.upstream_name
  || request.effectiveEndpoint
  || request.effective_endpoint
  || request.endpoint
  || request.endpoint_name
  || '未知上游'
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
    if (normalized.upstreamName) names.add(normalized.upstreamName);
  }
  return [...names].sort((left, right) => left.localeCompare(right, 'zh-Hans-CN'));
};
