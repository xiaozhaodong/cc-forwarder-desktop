// ============================================
// useFilters Hook - 筛选器状态管理
// 2025-11-28 17:02:47
// ============================================

import { useState, useCallback, useMemo } from 'react';
import { useTimezone } from '@/contexts/TimezoneContext.jsx';
import { buildQueryParamsFromFilters, createInitialFilters } from '../utils/filterTime.js';

export { buildQueryParamsFromFilters, createInitialFilters, getTodayTimeRange } from '../utils/filterTime.js';

/**
 * useFilters Hook - 管理筛选器状态和查询参数构建
 * @param {Object} initialFilters - 初始筛选器状态
 * @returns {Object}
 */
export const useFilters = (initialFilters = {}) => {
  const { timezone } = useTimezone();
  // 筛选器状态
  const [filters, setFilters] = useState(() => ({
    ...createInitialFilters(timezone),
    ...initialFilters
  }));

  // 更新单个筛选器
  const updateFilter = useCallback((key, value) => {
    setFilters(prev => ({ ...prev, [key]: value }));
  }, []);

  // 批量更新筛选器
  const updateFilters = useCallback((newFilters) => {
    setFilters(prev => ({ ...prev, ...newFilters }));
  }, []);

  // 重置筛选器
  const resetFilters = useCallback(() => {
    const nextFilters = createInitialFilters(timezone);
    setFilters(nextFilters);
    return nextFilters;
  }, [timezone]);

  // 构建 API 查询参数
  const buildQueryParams = useCallback(() => {
    return buildQueryParamsFromFilters(filters);
  }, [filters]);

  // 检查是否有活动筛选器
  const hasActiveFilters = useMemo(() => {
    const defaultFilters = createInitialFilters(timezone);
    return Object.entries(filters).some(([key, value]) => {
      if (key === 'startDate' || key === 'endDate') {
        return value && value !== '';
      }
      return value && value !== defaultFilters[key] && value !== 'all' && value !== '';
    });
  }, [filters, timezone]);

  // 活动筛选器数量
  const activeFiltersCount = useMemo(() => {
    const defaultFilters = createInitialFilters(timezone);
    return Object.entries(filters).filter(([key, value]) => {
      if (key === 'startDate' || key === 'endDate') {
        return value && value !== '';
      }
      return value && value !== defaultFilters[key] && value !== 'all' && value !== '';
    }).length;
  }, [filters, timezone]);

  return {
    filters,
    updateFilter,
    updateFilters,
    resetFilters,
    buildQueryParams,
    hasActiveFilters,
    activeFiltersCount
  };
};

export default useFilters;
