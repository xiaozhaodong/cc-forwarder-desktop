// ============================================
// 账号资产 表格/网格 共用徽章配色映射
// 2026-08-06
// ============================================

const STATE_TONE_CLASS = {
  rose: 'tone-rose',
  red: 'tone-rose',
  amber: 'tone-amber',
  yellow: 'tone-amber',
  emerald: 'tone-emerald',
  green: 'tone-emerald',
  sky: 'tone-sky',
  blue: 'tone-sky',
  indigo: 'tone-indigo',
  slate: 'tone-slate'
};

const UNKNOWN_LABELS = ['unknown', 'Unknown', '未知', '-', ''];

const AUTH_BADGE_CLASS = {
  'OAuth': 'tone-teal',
  'oauth': 'tone-teal',
  'ChatGPT RT': 'tone-fuchsia',
  'chatgpt_rt': 'tone-fuchsia',
  'API Key': 'tone-indigo',
  'api_key': 'tone-indigo'
};

const PLAN_BADGE_CLASS = {
  'Plus': 'tone-amber',
  'plus': 'tone-amber',
  'Pro': 'tone-orange',
  'pro': 'tone-orange',
  'Free': 'tone-emerald',
  'free': 'tone-emerald',
  'Prepaid': 'tone-slate',
  'prepaid': 'tone-slate'
};

const GROUP_BADGE_CLASS = {
  '主组': 'tone-blue font-semibold',
  '备组': 'tone-cyan',
  '冷备': 'tone-violet'
};

// 饱和实色点在深浅两底上都成立，不参与 token 化；
// 只有中性点必须走 token —— 固定浅灰在暗色下会变成全场最亮的点，
// 让「未知」反而抢过正常状态。
const STATE_DOT_CLASS = {
  emerald: 'bg-emerald-500',
  green: 'bg-emerald-500',
  amber: 'bg-warn-solid',
  yellow: 'bg-warn-solid',
  rose: 'bg-rose-500',
  red: 'bg-rose-500',
  sky: 'bg-sky-500',
  blue: 'bg-sky-500',
  indigo: 'bg-indigo-500',
  slate: 'bg-fg-subtle'
};

const normalizePlanLabel = (label) => {
  if (!label || UNKNOWN_LABELS.includes(label)) return '-';
  return label;
};

const toBadgeToneClass = (tone) => STATE_TONE_CLASS[tone] || STATE_TONE_CLASS.slate;

const toStateDotClass = (tone) => STATE_DOT_CLASS[tone] || STATE_DOT_CLASS.slate;

export {
  AUTH_BADGE_CLASS,
  GROUP_BADGE_CLASS,
  PLAN_BADGE_CLASS,
  STATE_TONE_CLASS,
  normalizePlanLabel,
  toBadgeToneClass,
  toStateDotClass
};
