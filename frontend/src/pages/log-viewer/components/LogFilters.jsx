// LogFilters.jsx - 日志过滤器组件
import { Search } from 'lucide-react';
import { LOG_LEVELS } from '../constants';

/**
 * 日志过滤器栏
 * @param {Object} stats 日志级别统计 { DEBUG: 10, INFO: 20, ... }
 * @param {string} levelFilter 当前级别过滤
 * @param {Function} onLevelFilterChange 级别过滤变化
 * @param {string} searchQuery 搜索关键词
 * @param {Function} onSearchChange 搜索变化
 * @param {boolean} autoScroll 自动滚动开关
 * @param {Function} onAutoScrollChange 自动滚动变化
 * @param {number} totalLogs 总日志数
 */
function LogFilters({
  stats,
  levelFilter,
  onLevelFilterChange,
  searchQuery,
  onSearchChange,
  autoScroll,
  onAutoScrollChange,
  totalLogs,
}) {
  return (
    <div className="flex items-center gap-4 mt-4">
      {/* 日志级别统计 */}
      <div className="flex items-center gap-2">
        {Object.entries(stats).map(([level, count]) => {
          const config = LOG_LEVELS[level];
          return (
            <button
              key={level}
              onClick={() => onLevelFilterChange(levelFilter === level ? 'ALL' : level)}
              className={`px-3 py-1 rounded-lg text-xs font-medium transition-all ${
                levelFilter === level
                  ? `${config.bg} ${config.color} ring-2 ring-offset-1 ring-${config.color.split('-')[1]}-400`
                  : 'bg-surface-mut text-fg-body hover:bg-surface-emph'
              }`}
            >
              {level} ({count})
            </button>
          );
        })}
        {levelFilter !== 'ALL' && (
          <button
            onClick={() => onLevelFilterChange('ALL')}
            className="px-3 py-1 rounded-lg text-xs font-medium bg-surface-mut text-fg-body hover:bg-surface-emph"
          >
            全部 ({totalLogs})
          </button>
        )}
      </div>

      {/* 搜索框 */}
      <div className="flex-1 max-w-md">
        <div className="relative">
          <Search size={18} className="absolute left-3.5 top-1/2 -translate-y-1/2 text-fg-subtle pointer-events-none" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder="搜索日志内容..."
            className="w-full pl-11 pr-4 py-2.5 text-sm text-fg bg-surface border border-line-strong rounded-lg placeholder:text-fg-subtle focus:outline-none focus:ring-2 focus:ring-accent-ring focus:border-accent-line shadow-sm"
          />
        </div>
      </div>

      {/* 自动滚动开关 */}
      <label className="flex items-center gap-2 text-sm text-fg-body cursor-pointer">
        <input
          type="checkbox"
          checked={autoScroll}
          onChange={(e) => onAutoScrollChange(e.target.checked)}
          className="app-checkbox"
        />
        自动滚动
      </label>
    </div>
  );
}

export default LogFilters;
