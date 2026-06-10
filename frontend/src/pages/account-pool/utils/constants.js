// ============================================
// Account Pool 常量与样式映射
// 2026-03-07
// ============================================

const DEFAULT_BASE_URL = 'https://api.openai.com';

const AUTH_METHOD_OPTIONS = [
  {
    value: 'chatgpt_refresh_token',
    label: 'ChatGPT RT',
    description: 'ChatGPT 账号授权（使用 Refresh Token / rt）'
  },
  {
    value: 'api_key',
    label: 'API Key',
    description: 'OpenAI Responses API Key'
  }
];

const ACCOUNT_GROUP_OPTIONS = [
  {
    value: 'primary',
    label: '主组',
    description: '默认优先尝试这组账号'
  },
  {
    value: 'backup',
    label: '备组',
    description: '主组不可用时，再尝试这组账号'
  },
  {
    value: 'cold',
    label: '冷备',
    description: '主组和备组都不可用时，最后尝试'
  }
];

const ACCOUNT_STATE_STYLE = {
  active: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  cooldown: 'bg-amber-50 text-amber-700 border-amber-200',
  disabled_auth: 'bg-rose-50 text-rose-700 border-rose-200',
  disabled: 'bg-slate-100 text-slate-600 border-slate-200'
};

const QUOTA_STATUS_STYLE = {
  ok: 'bg-sky-50 text-sky-700 border-sky-200',
  unavailable: 'bg-slate-100 text-slate-600 border-slate-200',
  exhausted: 'bg-amber-50 text-amber-700 border-amber-200',
  workspace_deactivated: 'bg-rose-50 text-rose-700 border-rose-200',
  auth_invalid: 'bg-rose-50 text-rose-700 border-rose-200',
  pending: 'bg-slate-100 text-slate-600 border-slate-200'
};

const SCHEDULE_OUTCOME_STYLE = {
  pending: 'bg-slate-100 text-slate-600 border-slate-200',
  success: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  auth_failed: 'bg-rose-50 text-rose-700 border-rose-200',
  transient_failure: 'bg-amber-50 text-amber-700 border-amber-200',
  passthrough_no_available_providers: 'bg-sky-50 text-sky-700 border-sky-200',
  passthrough_other_4xx: 'bg-slate-100 text-slate-700 border-slate-200',
  no_schedulable_accounts: 'bg-slate-100 text-slate-700 border-slate-200'
};

const SCHEDULE_DECISION_STYLE = {
  selected: 'bg-indigo-50 text-indigo-700 border-indigo-200',
  eligible: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  skipped: 'bg-slate-100 text-slate-600 border-slate-200'
};

const PLAN_TYPE_LABELS = {
  free: 'Free',
  plus: 'Plus',
  team: 'Team',
  enterprise: 'Enterprise',
  prepaid: 'Prepaid',
  unknown: 'Unknown'
};

const EMPTY_ACCOUNT_FORM = {
  account_name: '',
  auth_method: 'chatgpt_refresh_token',
  provider_type: 'chatgpt_refresh_token',
  group_key: 'primary',
  costMultiplier: '1.0',
  inputCostMultiplier: '1.0',
  outputCostMultiplier: '1.0',
  cacheCreationCostMultiplier: '1.0',
  cacheCreationCostMultiplier1h: '1.0',
  cacheReadCostMultiplier: '1.0',
  modelRewriteEnabled: false,
  modelRewriteRules: [
    { source: 'gpt-5.4', target: 'gpt-5.5' },
    { source: 'gpt-5.4-mini', target: 'gpt-5.5' }
  ],
  priority: '10',
  enabled: true,
  credential_raw: '',
  base_url: DEFAULT_BASE_URL
};

const MANUAL_FAILOVER_TIER_PRESETS = [
  {
    label: '主组',
    className: 'bg-indigo-50 text-indigo-700 border-indigo-200',
    description: '当前请求优先尝试这一层账号'
  },
  {
    label: '备组',
    className: 'bg-cyan-50 text-cyan-700 border-cyan-200',
    description: '主组全部失败后切到这一层'
  },
  {
    label: '冷备',
    className: 'bg-violet-50 text-violet-700 border-violet-200',
    description: '主组和备组都不可用时，再切到这一组'
  }
];

export {
  ACCOUNT_GROUP_OPTIONS,
  ACCOUNT_STATE_STYLE,
  AUTH_METHOD_OPTIONS,
  DEFAULT_BASE_URL,
  EMPTY_ACCOUNT_FORM,
  MANUAL_FAILOVER_TIER_PRESETS,
  PLAN_TYPE_LABELS,
  QUOTA_STATUS_STYLE,
  SCHEDULE_DECISION_STYLE,
  SCHEDULE_OUTCOME_STYLE
};
