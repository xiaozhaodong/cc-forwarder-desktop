import { getTodayTimeRange as getConfiguredTodayRange } from '../../../utils/timezone.js';
import { DEFAULT_FILTERS } from './constants.js';

export const getTodayTimeRange = (timezone, now = new Date()) => {
  const range = getConfiguredTodayRange(timezone, now);
  return { startDate: range.start_date, endDate: range.end_date };
};

export const createInitialFilters = (timezone, now = new Date()) => {
  const todayRange = getTodayTimeRange(timezone, now);
  return {
    ...DEFAULT_FILTERS,
    startDate: todayRange.startDate,
    endDate: todayRange.endDate
  };
};

const normalizeFilterDateTime = (value) => {
  const text = String(value || '').trim();
  if (/^\d{4}-\d{2}-\d{2}$/.test(text)) return `${text}T00:00:00`;
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(text)) return `${text}:00`;
  return text;
};

export const buildQueryParamsFromFilters = (filters = {}) => {
  const queryParams = {};

  if (filters.startDate) {
    queryParams.start_date = normalizeFilterDateTime(filters.startDate);
  }
  if (filters.endDate) {
    queryParams.end_date = normalizeFilterDateTime(filters.endDate);
  }

  if (filters.status && filters.status !== 'all') queryParams.status = filters.status;
  if (filters.model) queryParams.model = filters.model;
  if (filters.requestFamily && filters.requestFamily !== 'all') queryParams.request_family = filters.requestFamily;
  if (filters.upstreamName && filters.upstreamName !== 'all') queryParams.upstream_name = filters.upstreamName;

  return queryParams;
};
