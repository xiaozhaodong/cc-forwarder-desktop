// LogEntry.jsx - 单条日志显示组件
import { LOG_LEVELS } from '../constants';
import { useTimezone } from '@contexts/TimezoneContext.jsx';

/**
 * 单条日志组件
 * @param {Object} log 日志对象
 * @param {string} searchQuery 搜索关键词
 */
function LogEntry({ log, searchQuery }) {
  const { formatTimeOnly } = useTimezone();
  const levelConfig = LOG_LEVELS[log.level] || LOG_LEVELS.INFO;
  const Icon = levelConfig.icon;

  // 高亮搜索关键词
  const highlightText = (text) => {
    if (!searchQuery) return text;
    const parts = text.split(new RegExp(`(${searchQuery})`, 'gi'));
    return parts.map((part, i) =>
      part.toLowerCase() === searchQuery.toLowerCase() ? (
        <mark key={i} className="bg-yellow-200 text-gray-900">{part}</mark>
      ) : (
        part
      )
    );
  };

  return (
    <div className="flex items-start gap-3 px-4 py-2 hover:bg-slate-50 border-b border-slate-100 font-mono text-xs">
      {/* 时间戳 */}
      <div className="text-slate-400 whitespace-nowrap w-32 flex-shrink-0">
        {formatTimeOnly(log.timestamp)}
      </div>

      {/* 日志级别 */}
      <div className={`flex items-center gap-1 ${levelConfig.color} whitespace-nowrap w-16 flex-shrink-0`}>
        <Icon size={12} />
        <span className="font-semibold">{log.level}</span>
      </div>

      {/* 日志消息 */}
      <div className="flex-1 text-slate-700 break-all">
        {highlightText(log.message)}
      </div>

      {/* 附加属性 - 显示前3个，超长值截断 */}
      {log.attrs && Object.keys(log.attrs).length > 0 && (
        <div className="text-slate-400 text-[10px] whitespace-nowrap flex-shrink-0 max-w-[400px] truncate">
          {Object.entries(log.attrs).slice(0, 3).map(([key, value]) => (
            <span key={key} className="mr-2">
              {key}={String(value).length > 40 ? String(value).slice(0, 40) + '...' : value}
            </span>
          ))}
          {Object.keys(log.attrs).length > 3 && (
            <span className="text-slate-300">+{Object.keys(log.attrs).length - 3}</span>
          )}
        </div>
      )}
    </div>
  );
}

export default LogEntry;
