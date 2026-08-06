// ============================================
// 账号错误展示模型
// 将上游原始错误收敛为卡片摘要与详情信息
// ============================================

const MAX_JSON_DEPTH = 3;

const ERROR_CLASSIFIERS = [
  {
    category: 'billing',
    label: '扣费失败',
    tone: 'amber',
    pattern: /(?:预扣费|扣费失败|billing[^\n]{0,24}(?:fail|error))/i
  },
  {
    category: 'quota',
    label: '额度不足',
    tone: 'amber',
    pattern: /(?:额度不足|余额不足|剩余额度|insufficient[^\n]{0,24}(?:balance|credit)|quota[^\n]{0,24}(?:exhaust|limit))/i
  },
  {
    category: 'capacity',
    label: '上游繁忙',
    tone: 'amber',
    pattern: /(?:负载[^\n]{0,24}(?:上限|已满)|rate[ _-]?limit|too many requests|overload|上游繁忙|限流|\b429\b)/i
  },
  {
    category: 'auth',
    label: '认证异常',
    tone: 'rose',
    pattern: /(?:refresh token[^\n]{0,24}(?:expired|invalid)|token[^\n]{0,24}(?:expired|invalid)|unauthori[sz]ed|forbidden|鉴权|认证失败|凭据[^\n]{0,16}(?:失效|无效)|\b401\b|\b403\b)/i
  },
  {
    category: 'config',
    label: '配置异常',
    tone: 'rose',
    pattern: /(?:分组[^\n]{0,24}(?:弃用|不存在|无效)|配置[^\n]{0,16}(?:错误|无效)|deprecated|unsupported|not found)/i
  }
];

const normalizeWhitespace = (value = '') => String(value || '').replace(/\s+/g, ' ').trim();

const tryParseJSON = (value, depth = 0) => {
  if (depth >= MAX_JSON_DEPTH || typeof value !== 'string') {
    return { parsed: false, value };
  }

  const text = value.trim();
  if (!text || !['{', '[', '"'].includes(text[0])) {
    return { parsed: false, value };
  }

  try {
    const parsedValue = JSON.parse(text);
    if (typeof parsedValue === 'string') {
      const nested = tryParseJSON(parsedValue, depth + 1);
      return nested.parsed ? nested : { parsed: true, value: parsedValue };
    }
    return { parsed: true, value: parsedValue };
  } catch {
    return { parsed: false, value };
  }
};

const findMessage = (value, depth = 0) => {
  if (depth >= MAX_JSON_DEPTH || value == null) return '';

  if (typeof value === 'string') {
    const nested = tryParseJSON(value, depth);
    if (nested.parsed && nested.value !== value) {
      return findMessage(nested.value, depth + 1);
    }
    return normalizeWhitespace(value);
  }

  if (Array.isArray(value)) {
    for (const item of value) {
      const message = findMessage(item, depth + 1);
      if (message) return message;
    }
    return '';
  }

  if (typeof value !== 'object') return '';

  const candidates = [
    value.message,
    value.error?.message,
    value.error_description,
    value.detail,
    value.error,
    value.errors
  ];

  for (const candidate of candidates) {
    const message = findMessage(candidate, depth + 1);
    if (message) return message;
  }

  return '';
};

const decodeLooseJSONString = (value = '') => {
  try {
    return JSON.parse(`"${value}"`);
  } catch {
    return value.replace(/\\"/g, '"').replace(/\\n/g, ' ');
  }
};

const findLooseMessage = (raw = '') => {
  const match = raw.match(/["']message["']\s*:\s*["']((?:\\.|[^"'\\])*)/i);
  return match ? normalizeWhitespace(decodeLooseJSONString(match[1])) : '';
};

const extractRequestID = (value = '') => {
  const match = String(value || '').match(/request(?:[_\s-]?id)?\s*[:：=]\s*([a-z0-9._:-]+)/i);
  return match?.[1] || '';
};

const stripRequestID = (value = '') => normalizeWhitespace(
  String(value || '')
    .replace(/\s*\(?request(?:[_\s-]?id)?\s*[:：=]\s*[a-z0-9._:-]+\)?/gi, '')
    .replace(/[，,、;；:：\s-]+$/g, '')
);

const classifyError = (message, raw) => {
  const source = `${message}\n${raw}`;
  return ERROR_CLASSIFIERS.find((item) => item.pattern.test(source)) || {
    category: 'unknown',
    label: '最近异常',
    tone: 'rose'
  };
};

const removeRepeatedCategoryPrefix = (summary, category) => {
  if (category === 'billing') {
    return summary.replace(/^(?:预扣费额度失败|预扣费失败|扣费失败)[，,:：\s-]*/i, '');
  }
  if (category === 'quota') {
    return summary.replace(/^(?:用户)?(?:额度不足|余额不足)[，,:：\s-]*/i, '');
  }
  return summary;
};

const buildAccountErrorDisplay = (rawError = '') => {
  const raw = String(rawError || '').trim();
  if (!raw) return null;

  const parsed = tryParseJSON(raw);
  const parsedMessage = parsed.parsed ? findMessage(parsed.value) : '';
  const looseMessage = parsedMessage || findLooseMessage(raw);
  const looksLikeJSON = ['{', '[', '"'].includes(raw[0]);
  const extractedMessage = looseMessage || (looksLikeJSON ? '上游返回了无法解析的错误响应' : normalizeWhitespace(raw));
  const requestId = extractRequestID(extractedMessage) || extractRequestID(raw);
  const message = stripRequestID(extractedMessage) || extractedMessage;
  const classification = classifyError(extractedMessage, raw);
  const summary = removeRepeatedCategoryPrefix(message, classification.category)
    || classification.label;

  return {
    ...classification,
    message,
    summary,
    requestId,
    raw
  };
};

export {
  buildAccountErrorDisplay
};
