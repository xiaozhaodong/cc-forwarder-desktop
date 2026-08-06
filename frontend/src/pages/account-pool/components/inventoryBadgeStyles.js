// ============================================
// 账号资产 表格/网格 共用徽章配色映射
// 2026-08-06
// ============================================

const STATE_TONE_CLASS = {
  rose: 'bg-rose-50 text-rose-700 border-rose-200',
  red: 'bg-rose-50 text-rose-700 border-rose-200',
  amber: 'bg-amber-50 text-amber-700 border-amber-200',
  yellow: 'bg-amber-50 text-amber-700 border-amber-200',
  emerald: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  green: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  sky: 'bg-sky-50 text-sky-700 border-sky-200',
  blue: 'bg-sky-50 text-sky-700 border-sky-200',
  indigo: 'bg-indigo-50 text-indigo-700 border-indigo-200',
  slate: 'bg-slate-50 text-slate-600 border-slate-200'
};

const UNKNOWN_LABELS = ['unknown', 'Unknown', '未知', '-', ''];

const AUTH_BADGE_CLASS = {
  'OAuth': 'bg-teal-50 text-teal-700 border-teal-200',
  'oauth': 'bg-teal-50 text-teal-700 border-teal-200',
  'ChatGPT RT': 'bg-fuchsia-50 text-fuchsia-700 border-fuchsia-200',
  'chatgpt_rt': 'bg-fuchsia-50 text-fuchsia-700 border-fuchsia-200',
  'API Key': 'bg-indigo-50 text-indigo-700 border-indigo-200',
  'api_key': 'bg-indigo-50 text-indigo-700 border-indigo-200'
};

const PLAN_BADGE_CLASS = {
  'Plus': 'bg-amber-50 text-amber-700 border-amber-200',
  'plus': 'bg-amber-50 text-amber-700 border-amber-200',
  'Pro': 'bg-orange-50 text-orange-700 border-orange-200',
  'pro': 'bg-orange-50 text-orange-700 border-orange-200',
  'Free': 'bg-emerald-50 text-emerald-700 border-emerald-200',
  'free': 'bg-emerald-50 text-emerald-700 border-emerald-200',
  'Prepaid': 'bg-slate-100 text-slate-600 border-slate-200',
  'prepaid': 'bg-slate-100 text-slate-600 border-slate-200'
};

const GROUP_BADGE_CLASS = {
  '主组': 'bg-blue-100 text-blue-800 border-blue-300 font-semibold',
  '备组': 'bg-cyan-50 text-cyan-700 border-cyan-200',
  '冷备': 'bg-violet-50 text-violet-700 border-violet-200'
};

const STATE_DOT_CLASS = {
  emerald: 'bg-emerald-500',
  green: 'bg-emerald-500',
  amber: 'bg-amber-400',
  yellow: 'bg-amber-400',
  rose: 'bg-rose-500',
  red: 'bg-rose-500',
  sky: 'bg-sky-500',
  blue: 'bg-sky-500',
  indigo: 'bg-indigo-500',
  slate: 'bg-slate-300'
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
