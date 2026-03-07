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
  unknown: 'Unknown'
};

const EMPTY_ACCOUNT_FORM = {
  account_name: '',
  auth_method: 'chatgpt_refresh_token',
  provider_type: 'chatgpt_refresh_token',
  priority: '1',
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
    label: '兜底组',
    className: 'bg-violet-50 text-violet-700 border-violet-200',
    description: '前两层都不可用时，再切到这一层'
  }
];

export {
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
