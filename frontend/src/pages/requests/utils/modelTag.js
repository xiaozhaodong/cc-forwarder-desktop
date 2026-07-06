// 请求追踪模型标签颜色规则

const DEFAULT_MODEL_CLASSES = 'bg-slate-50 text-slate-600 border-slate-200';

const MODEL_COLOR_RULES = [
  {
    matches: (modelName) => modelName.includes('glm-5.2') || modelName.startsWith('glm-'),
    classes: 'bg-teal-50 text-teal-700 border-teal-200'
  },
  {
    matches: (modelName) => modelName.includes('fable-5') || modelName.includes('claude-fable-5'),
    classes: 'bg-rose-50 text-rose-700 border-rose-200'
  },
  {
    matches: (modelName) => modelName.includes('sonnet-5') || modelName.includes('claude-sonnet-5'),
    classes: 'bg-amber-50 text-amber-700 border-amber-200'
  },
  {
    matches: (modelName) => modelName.includes('sonnet-4') || modelName.includes('claude-sonnet-4'),
    classes: 'bg-orange-50 text-orange-700 border-orange-200'
  },
  {
    matches: (modelName) => modelName.includes('3-5-haiku') || modelName.includes('haiku'),
    classes: 'bg-green-50 text-green-800 border-green-200'
  },
  {
    matches: (modelName) => modelName.includes('3-5-sonnet') || (modelName.includes('sonnet') && modelName.includes('3.5')),
    classes: 'bg-blue-50 text-blue-700 border-blue-100'
  },
  {
    matches: (modelName) => modelName.includes('opus'),
    classes: 'bg-purple-50 text-purple-700 border-purple-200'
  },
  {
    matches: (modelName) => modelName.includes('gpt-'),
    classes: 'bg-indigo-50 text-indigo-700 border-indigo-200'
  }
];

/**
 * 根据模型名称获取对应的颜色类名
 */
export const getModelColorClasses = (modelName) => {
  if (!modelName || modelName === 'unknown' || modelName === '-') {
    return DEFAULT_MODEL_CLASSES;
  }

  const lowerName = String(modelName).trim().toLowerCase();
  if (!lowerName || lowerName === 'unknown' || lowerName === '-') {
    return DEFAULT_MODEL_CLASSES;
  }

  const matchedRule = MODEL_COLOR_RULES.find((rule) => rule.matches(lowerName));

  return matchedRule?.classes ?? DEFAULT_MODEL_CLASSES;
};
