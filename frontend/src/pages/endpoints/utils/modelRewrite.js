// ============================================
// Claude 端点模型改写纯函数
// ============================================

const CC_MODEL_REWRITE_PATHS = ['/v1/messages', '/v1/messages/count_tokens'];

const createEmptyEndpointModelRewriteRule = () => ({
  source: '',
  target: ''
});

const normalizeEndpointModelRewriteRule = (rule = {}) => ({
  source: String(rule.source ?? rule.from ?? '').trim(),
  target: String(rule.target ?? rule.to ?? '').trim()
});

const parseEndpointModelRewriteSettings = (raw = '') => {
  const fallback = {
    enabled: false,
    rules: [createEmptyEndpointModelRewriteRule()]
  };
  const text = String(raw || '').trim();
  if (!text) {
    return fallback;
  }

  try {
    const parsed = JSON.parse(text);
    const rules = (Array.isArray(parsed) ? parsed : [parsed])
      .filter((rule) => {
        if (!rule || typeof rule !== 'object') {
          return false;
        }
        const match = String(rule.match || 'exact').trim().toLowerCase();
        return match === 'exact';
      })
      .map(normalizeEndpointModelRewriteRule)
      .filter((rule) => rule.source && rule.target);

    return rules.length > 0
      ? { enabled: true, rules }
      : fallback;
  } catch {
    return fallback;
  }
};

const serializeEndpointModelRewriteRules = (value = []) => {
  if (typeof value === 'string') {
    return value.trim();
  }

  const rules = (Array.isArray(value) ? value : [])
    .map(normalizeEndpointModelRewriteRule)
    .filter((rule) => rule.source && rule.target);
  if (!rules.length) {
    return '';
  }

  return JSON.stringify(rules.map((rule) => ({
    paths: CC_MODEL_REWRITE_PATHS,
    match: 'exact',
    from: rule.source,
    to: rule.target
  })));
};

const summarizeEndpointModelRewriteRules = (raw = '') => {
  const settings = parseEndpointModelRewriteSettings(raw);
  if (!settings.enabled) {
    return null;
  }

  const mappings = settings.rules.map((rule) => `${rule.source} → ${rule.target}`);
  return {
    count: mappings.length,
    label: `${mappings[0]}${mappings.length > 1 ? ` +${mappings.length - 1}` : ''}`,
    title: mappings.join('\n')
  };
};

export {
  CC_MODEL_REWRITE_PATHS,
  createEmptyEndpointModelRewriteRule,
  parseEndpointModelRewriteSettings,
  serializeEndpointModelRewriteRules,
  summarizeEndpointModelRewriteRules
};
