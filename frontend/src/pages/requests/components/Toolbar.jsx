// ============================================
// Toolbar - 页面工具栏
// 2025-12-06 10:40:38 v4.0: 简化端点切换（移除 Token 级联）
// ============================================

import { Filter, Settings2, ChevronDown } from 'lucide-react';
import { TIME_RANGE_OPTIONS } from '../utils/constants.js';
import ViewConfigPanel from './ViewConfigPanel.jsx';
import AutoRefreshControl from './AutoRefreshControl.jsx';
import ActiveGroupSwitcher from './ActiveGroupSwitcher.jsx';

/**
 * Toolbar - 请求追踪页面工具栏
 * @param {Object} props
 * @param {string} props.activeTimeRange - 当前激活的时间范围
 * @param {Function} props.onTimeRangeChange - 时间范围变更回调
 * @param {boolean} props.isFilterOpen - 筛选面板是否打开
 * @param {Function} props.onFilterToggle - 筛选面板切换回调
 * @param {boolean} props.isViewConfigOpen - 列配置面板是否打开
 * @param {Function} props.onViewConfigToggle - 列配置面板切换回调
 * @param {Function} props.onRefresh - 刷新回调
 * @param {Array} props.columns - 所有列配置
 * @param {Array} props.visibleColumns - 当前可见的列ID数组
 * @param {Function} props.onToggleColumn - 切换列显示回调
 * @param {Function} props.onResetColumns - 重置列配置回调
 * @param {Object} props.autoRefresh - 自动刷新状态和控制
 * @param {Array} props.groups - 所有端点列表（v4.0: 一个端点=一个组）
 * @param {string} props.activeGroup - 当前活跃端点名称
 * @param {Function} props.onGroupSwitch - 端点切换回调 (endpointName) => void
 */
const Toolbar = ({
  activeTimeRange = 'today',
  onTimeRangeChange,
  isFilterOpen = false,
  onFilterToggle,
  isViewConfigOpen = false,
  onViewConfigToggle,
  onRefresh,
  columns = [],
  visibleColumns = [],
  onToggleColumn,
  onResetColumns,
  autoRefresh = null,
  groups = [],
  activeGroup = '',
  onGroupSwitch
}) => {
  return (
    <div className="flex flex-wrap items-center gap-3">
      {/* 端点快捷切换器 */}
      <ActiveGroupSwitcher
        groups={groups}
        activeGroup={activeGroup}
        onSwitch={onGroupSwitch}
      />

      <div className="h-6 w-px bg-gray-300 mx-1 hidden sm:block"></div>

      {/* 快捷时间范围选择 */}
      <div className="hidden sm:flex items-center bg-white border border-gray-200 rounded-lg p-1 shadow-sm">
        {TIME_RANGE_OPTIONS.map((range) => (
          <button
            key={range.value}
            onClick={() => onTimeRangeChange?.(range.value)}
            className={`px-3 py-1.5 text-xs font-medium rounded-md transition-all ${
              activeTimeRange === range.value
                ? 'bg-indigo-50 text-indigo-700 shadow-sm'
                : 'text-gray-500 hover:text-gray-900 hover:bg-gray-50'
            }`}
          >
            {range.label}
          </button>
        ))}
      </div>

      <div className="h-6 w-px bg-gray-300 mx-1 hidden sm:block"></div>

      {/* 筛选按钮 */}
      <button
        onClick={onFilterToggle}
        className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-all shadow-sm border ${
          isFilterOpen
            ? 'bg-indigo-50 text-indigo-700 border-indigo-200 ring-2 ring-indigo-100'
            : 'bg-white text-gray-700 border-gray-200 hover:border-indigo-300 hover:text-indigo-600'
        }`}
      >
        <Filter className="w-4 h-4" /> 筛选
        <ChevronDown
          className={`w-3.5 h-3.5 transition-transform ${isFilterOpen ? 'rotate-180' : ''}`}
        />
      </button>

      <div className="h-6 w-px bg-gray-300 mx-1 hidden sm:block"></div>

      {/* 列配置按钮 */}
      <div className="relative">
        <button
          onClick={onViewConfigToggle}
          className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-all shadow-sm border ${
            isViewConfigOpen
              ? 'bg-indigo-50 text-indigo-700 border-indigo-200 ring-2 ring-indigo-100'
              : 'bg-white text-gray-600 border-gray-200 hover:bg-gray-50 hover:text-indigo-600 hover:border-indigo-200'
          }`}
          title="自定义显示列"
        >
          <Settings2 className="w-4 h-4" />
          <span className="hidden sm:inline font-medium">显示</span>
        </button>

        {/* 列配置面板 */}
        <ViewConfigPanel
          isOpen={isViewConfigOpen}
          onClose={() => onViewConfigToggle?.(false)}
          columns={columns}
          visibleColumns={visibleColumns}
          onToggleColumn={onToggleColumn}
          onReset={onResetColumns}
        />
      </div>

      {/* 自动刷新控制（包含手动刷新） */}
      {autoRefresh && (
        <AutoRefreshControl
          isEnabled={autoRefresh.isEnabled}
          interval={autoRefresh.interval}
          onIntervalChange={autoRefresh.changeInterval}
          onManualRefresh={onRefresh}
        />
      )}
    </div>
  );
};

export default Toolbar;
