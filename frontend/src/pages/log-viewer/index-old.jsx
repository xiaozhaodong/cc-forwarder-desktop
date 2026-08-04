// LogsPage - 系统日志查看页面
// 提供实时日志流查看、过滤、搜索功能
import React, { useState, useMemo, useRef, useEffect } from 'react';
import { useWailsLogs } from '@/hooks/useWailsLogs';
import { useTimezone } from '@contexts/TimezoneContext.jsx';
import {
  FileText,
  Pause,
  Play,
  Trash2,
  RefreshCw,
  Search,
  Filter,
  Download,
  AlertCircle,
  Info,
  AlertTriangle,
  XCircle,
  CheckCircle,
} from 'lucide-react';

// 自定义Hook：检测组件是否挂载和可见
function useComponentVisibility() {
  const [isVisible, setIsVisible] = useState(true);

  useEffect(() => {
    // 组件挂载时设为可见
    setIsVisible(true);

    // 组件卸载时设为不可见
    return () => {
      setIsVisible(false);
    };
  }, []);

  return isVisible;
}

// 日志级别配置
const LOG_LEVELS = {
  DEBUG: { color: 'text-slate-500', bg: 'bg-slate-50', icon: Info },
  INFO: { color: 'text-blue-600', bg: 'bg-blue-50', icon: CheckCircle },
  WARN: { color: 'text-amber-600', bg: 'bg-amber-50', icon: AlertTriangle },
  ERROR: { color: 'text-red-600', bg: 'bg-red-50', icon: XCircle },
};

// 单条日志组件
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

      {/* 附加属性 */}
      {log.attrs && Object.keys(log.attrs).length > 0 && (
        <div className="text-slate-400 text-[10px] whitespace-nowrap flex-shrink-0">
          {Object.entries(log.attrs).slice(0, 2).map(([key, value]) => (
            <span key={key} className="mr-2">
              {key}={value}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

// 主页面组件
function LogsPage() {
  // 检测组件可见性
  const isPageVisible = useComponentVisibility();

  const {
    logs,
    loading,
    error,
    isStreaming,
    start,
    stop,
    clear,
    refresh,
  } = useWailsLogs({
    maxLogs: 1000,
    autoStart: true,
    isActive: isPageVisible, // 传递可见性状态
  });

  const [searchQuery, setSearchQuery] = useState('');
  const [levelFilter, setLevelFilter] = useState('ALL');
  const [autoScroll, setAutoScroll] = useState(true);
  const [isInitialLoad, setIsInitialLoad] = useState(true); // 标记初次加载
  const logsEndRef = useRef(null);
  const logsContainerRef = useRef(null);
  const prevLogsLengthRef = useRef(0); // 记录上次日志数量

  // 过滤日志
  const filteredLogs = useMemo(() => {
    let result = logs;

    // 日志级别过滤
    if (levelFilter !== 'ALL') {
      result = result.filter(log => log.level === levelFilter);
    }

    // 搜索过滤
    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      result = result.filter(log =>
        log.message.toLowerCase().includes(query) ||
        (log.attrs && JSON.stringify(log.attrs).toLowerCase().includes(query))
      );
    }

    return result;
  }, [logs, levelFilter, searchQuery]);

  // 自动滚动到底部（只在接收到新日志且启用自动滚动时触发）
  useEffect(() => {
    // 初次加载完成后，标记为非初次加载
    if (isInitialLoad && !loading && logs.length > 0) {
      setIsInitialLoad(false);
      prevLogsLengthRef.current = logs.length;
      return; // 初次加载不滚动，避免闪烁
    }

    // 只在日志数量增加时滚动（表示接收到新日志）
    const hasNewLogs = logs.length > prevLogsLengthRef.current;
    if (autoScroll && hasNewLogs && !loading && logsEndRef.current) {
      // 延迟滚动，避免闪烁
      const timer = setTimeout(() => {
        logsEndRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' });
      }, 50);
      prevLogsLengthRef.current = logs.length;
      return () => clearTimeout(timer);
    }

    prevLogsLengthRef.current = logs.length;
  }, [logs.length, autoScroll, loading, isInitialLoad]);

  // 导出日志
  const handleExport = () => {
    const content = filteredLogs
      .map(log => `[${log.timestamp}] [${log.level}] ${log.message}`)
      .join('\n');
    const blob = new Blob([content], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `logs-${new Date().toISOString()}.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };

  // 统计信息
  const stats = useMemo(() => {
    const counts = { DEBUG: 0, INFO: 0, WARN: 0, ERROR: 0 };
    logs.forEach(log => {
      if (counts[log.level] !== undefined) {
        counts[log.level]++;
      }
    });
    return counts;
  }, [logs]);

  // 清空日志时重置初始加载状态
  const handleClear = () => {
    clear();
    setIsInitialLoad(true);
    prevLogsLengthRef.current = 0;
  };

  return (
    <div className="flex flex-col bg-white rounded-lg overflow-hidden border border-slate-200 shadow-sm" style={{ height: 'calc(100vh - 150px)' }}>
      {/* 页面头部 - 固定不滚动 */}
      <div className="flex-shrink-0 border-b border-slate-200 px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-slate-800 flex items-center gap-2">
              <FileText size={28} className="text-indigo-600" />
              系统日志
            </h1>
            <p className="text-slate-500 text-sm mt-1">
              实时查看系统运行日志 · 共 {logs.length} 条
              {filteredLogs.length !== logs.length && ` · 筛选后 ${filteredLogs.length} 条`}
            </p>
          </div>

          <div className="flex items-center gap-2">
            {/* 实时状态指示器 */}
            <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium bg-slate-100 text-slate-700">
              <span className={`w-2 h-2 rounded-full ${
                isStreaming ? 'bg-emerald-400 animate-pulse' : 'bg-slate-300'
              }`} />
              {isStreaming ? '实时' : '已暂停'}
            </div>

            {/* 控制按钮 */}
            <button
              onClick={isStreaming ? stop : start}
              className={`p-2 rounded-lg transition-all ${
                isStreaming
                  ? 'bg-amber-100 text-amber-700 hover:bg-amber-200'
                  : 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200'
              }`}
              title={isStreaming ? '暂停' : '开始'}
            >
              {isStreaming ? <Pause size={16} /> : <Play size={16} />}
            </button>

            <button
              onClick={refresh}
              disabled={loading}
              className="p-2 rounded-lg bg-slate-100 text-slate-700 hover:bg-slate-200 transition-all disabled:opacity-50"
              title="刷新"
            >
              <RefreshCw size={16} className={loading ? 'animate-spin' : ''} />
            </button>

            <button
              onClick={handleClear}
              className="p-2 rounded-lg bg-slate-100 text-slate-700 hover:bg-slate-200 transition-all"
              title="清空"
            >
              <Trash2 size={16} />
            </button>

            <button
              onClick={handleExport}
              disabled={filteredLogs.length === 0}
              className="p-2 rounded-lg bg-slate-100 text-slate-700 hover:bg-slate-200 transition-all disabled:opacity-50"
              title="导出"
            >
              <Download size={16} />
            </button>
          </div>
        </div>

        {/* 统计和过滤栏 */}
        <div className="flex items-center gap-4 mt-4">
          {/* 日志级别统计 */}
          <div className="flex items-center gap-2">
            {Object.entries(stats).map(([level, count]) => {
              const config = LOG_LEVELS[level];
              return (
                <button
                  key={level}
                  onClick={() => setLevelFilter(levelFilter === level ? 'ALL' : level)}
                  className={`px-3 py-1 rounded-lg text-xs font-medium transition-all ${
                    levelFilter === level
                      ? `${config.bg} ${config.color} ring-2 ring-offset-1 ring-${config.color.split('-')[1]}-400`
                      : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                  }`}
                >
                  {level} ({count})
                </button>
              );
            })}
            {levelFilter !== 'ALL' && (
              <button
                onClick={() => setLevelFilter('ALL')}
                className="px-3 py-1 rounded-lg text-xs font-medium bg-slate-100 text-slate-600 hover:bg-slate-200"
              >
                全部 ({logs.length})
              </button>
            )}
          </div>

          {/* 搜索框 */}
          <div className="flex-1 max-w-md">
            <div className="relative">
              <Search size={18} className="absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-400 pointer-events-none" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="搜索日志内容..."
                className="w-full pl-11 pr-4 py-2.5 text-sm text-slate-900 bg-white border border-slate-300 rounded-lg placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 shadow-sm"
              />
            </div>
          </div>

          {/* 自动滚动开关 */}
          <label className="flex items-center gap-2 text-sm text-slate-600 cursor-pointer">
            <input
              type="checkbox"
              checked={autoScroll}
              onChange={(e) => setAutoScroll(e.target.checked)}
              className="w-4 h-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
            />
            自动滚动
          </label>
        </div>
      </div>

      {/* 日志内容区域 */}
      <div
        ref={logsContainerRef}
        className="flex-1 overflow-y-auto bg-white"
      >
        {loading && logs.length === 0 ? (
          <div className="flex items-center justify-center h-full text-slate-400">
            <RefreshCw size={24} className="animate-spin mr-2" />
            加载中...
          </div>
        ) : error ? (
          <div className="flex items-center justify-center h-full text-red-500">
            <AlertCircle size={24} className="mr-2" />
            {error}
          </div>
        ) : filteredLogs.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-slate-400">
            <FileText size={48} className="mb-2 opacity-50" />
            <p className="text-lg">暂无日志</p>
            {searchQuery || levelFilter !== 'ALL' ? (
              <p className="text-sm mt-1">尝试调整搜索条件或过滤器</p>
            ) : null}
          </div>
        ) : (
          <>
            {filteredLogs.map((log, index) => (
              <LogEntry
                key={`${log.timestamp}-${index}`}
                log={log}
                searchQuery={searchQuery}
              />
            ))}
            <div ref={logsEndRef} className="h-4" />
          </>
        )}
      </div>
    </div>
  );
}

export default LogsPage;
