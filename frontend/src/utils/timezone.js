const DEFAULT_LOCALE = 'zh-CN';

export const validateTimezone = (timezone) => {
  const value = String(timezone || '').trim();
  if (!value) throw new Error('后端未返回活动时区');
  try {
    new Intl.DateTimeFormat(DEFAULT_LOCALE, { timeZone: value }).format(new Date(0));
  } catch {
    throw new Error(`后端返回了无效时区：${value}`);
  }
  return value;
};

// 严格绝对时刻解析：仅接受有效 Date 或以 Z / ±hh:mm 结尾的时间点，
// 空串、无时区后缀、无效日期均拒绝，避免浏览器按本地时区解释无时区字符串。
export const parseTimestamp = (value) => {
  if (value instanceof Date) {
    if (!Number.isNaN(value.getTime())) return value;
    throw new Error('无效时间');
  }
  const raw = String(value || '').trim();
  if (!raw) throw new Error('时间为空');
  // 新 API 只接受带 offset 的时间点，避免浏览器把无时区字符串解释为系统本地时间。
  if (!/(?:Z|[+-]\d{2}:\d{2})$/i.test(raw)) {
    throw new Error(`后端时间缺少时区：${raw}`);
  }
  const parsed = new Date(raw);
  if (Number.isNaN(parsed.getTime())) throw new Error(`无效时间：${raw}`);
  return parsed;
};

const partsMap = (date, timezone, options = {}) => Object.fromEntries(
  new Intl.DateTimeFormat('en-CA', {
    timeZone: validateTimezone(timezone),
    hourCycle: 'h23',
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
    ...options
  }).formatToParts(date).filter((part) => part.type !== 'literal').map((part) => [part.type, part.value])
);

export const formatTimestamp = (value, timezone) => {
  if (!value) return '-';
  try {
    const parts = partsMap(parseTimestamp(value), timezone);
    return `${parts.year}/${Number(parts.month)}/${Number(parts.day)} ${parts.hour}:${parts.minute}:${parts.second}`;
  } catch {
    return '-';
  }
};

export const formatTimeOnly = (value, timezone) => {
  if (!value) return '-';
  try {
    const parts = partsMap(parseTimestamp(value), timezone);
    return `${parts.hour}:${parts.minute}:${parts.second}`;
  } catch {
    return '-';
  }
};

export const formatMonthDayTime = (value, timezone) => {
  if (!value) return '-';
  try {
    const parts = partsMap(parseTimestamp(value), timezone);
    return `${parts.month}-${parts.day} ${parts.hour}:${parts.minute}`;
  } catch {
    return '-';
  }
};

export const getZonedDateParts = (now, timezone) => {
  const parts = partsMap(parseTimestamp(now instanceof Date ? now : new Date(now)), timezone);
  return {
    year: Number(parts.year), month: Number(parts.month), day: Number(parts.day),
    hour: Number(parts.hour), minute: Number(parts.minute), second: Number(parts.second)
  };
};

const shiftCalendarDate = ({ year, month, day }, days) => {
  const cursor = new Date(Date.UTC(year, month - 1, day));
  cursor.setUTCDate(cursor.getUTCDate() + days);
  return { year: cursor.getUTCFullYear(), month: cursor.getUTCMonth() + 1, day: cursor.getUTCDate() };
};

const wallDate = ({ year, month, day }) => (
  `${String(year).padStart(4, '0')}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`
);

export const getTodayTimeRange = (timezone, now = new Date()) => {
  const today = getZonedDateParts(now, timezone);
  const tomorrow = shiftCalendarDate(today, 1);
  return { start_date: `${wallDate(today)}T00:00`, end_date: `${wallDate(tomorrow)}T00:00` };
};

export const getRecentDaysRange = (days, timezone, now = new Date()) => {
  const count = Math.max(1, Number.parseInt(days, 10) || 1);
  const today = getZonedDateParts(now, timezone);
  const start = shiftCalendarDate(today, -(count - 1));
  const end = shiftCalendarDate(today, 1);
  return { start_date: `${wallDate(start)}T00:00`, end_date: `${wallDate(end)}T00:00` };
};
