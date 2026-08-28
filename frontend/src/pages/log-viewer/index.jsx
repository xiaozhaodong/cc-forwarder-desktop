// LogsPage - 系统日志查看页面（主组件）
import { useState, useMemo, useRef, useEffect } from 'react';
import { useWailsLogs } from '@/hooks/useWailsLogs';
import { FileText, RefreshCw, AlertCircle } from 'lucide-react';
import { LogEntry, LogControls, LogFilters } from './components';

// 主页面组件
function LogsPage() {
  // 日志数据和操作
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
    isActive: true,
  });

  // 本地状态
  const [searchQuery, setSearchQuery] = useState('');
  const [levelFilter, setLevelFilter] = useState('ALL');
  const [autoScroll, setAutoScroll] = useState(true);
  const logsEndRef = useRef(null);
  const prevLogsLengthRef = useRef(0);
  const hasInitialScrollDoneRef = useRef(false);

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

  // 自动滚动逻辑
  useEffect(() => {
    if (!hasInitialScrollDoneRef.current && !loading && logs.length > 0 && logsEndRef.current) {
      hasInitialScrollDoneRef.current = true;
      prevLogsLengthRef.current = logs.length;
      // 首次加载完成后定位到底部，确保显示最新日志
      const rafId = requestAnimationFrame(() => {
        logsEndRef.current?.scrollIntoView({ behavior: 'auto', block: 'end' });
      });
      return () => cancelAnimationFrame(rafId);
    }

    const hasNewLogs = logs.length > prevLogsLengthRef.current;
    if (autoScroll && hasNewLogs && !loading && logsEndRef.current) {
      const rafId = requestAnimationFrame(() => {
        logsEndRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' });
      });
      prevLogsLengthRef.current = logs.length;
      return () => cancelAnimationFrame(rafId);
    }

    prevLogsLengthRef.current = logs.length;
  }, [logs.length, autoScroll, loading]);

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

  // 清空日志
  const handleClear = () => {
    clear();
    hasInitialScrollDoneRef.current = false;
    prevLogsLengthRef.current = 0;
  };

  return (
    <div
      className="flex flex-col bg-surface rounded-lg overflow-hidden border border-line shadow-sm"
      style={{ height: 'calc(100vh - 150px)' }}
    >
      {/* 页面头部 - 固定 */}
      <div className="flex-shrink-0 border-b border-line px-6 py-4">
        {/* 标题栏 */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-fg flex items-center gap-2">
              <FileText size={28} className="text-accent" />
              系统日志
            </h1>
            <p className="text-fg-muted text-sm mt-1">
              实时查看系统运行日志 · 共 {logs.length} 条
              {filteredLogs.length !== logs.length && ` · 筛选后 ${filteredLogs.length} 条`}
            </p>
          </div>

          <LogControls
            isStreaming={isStreaming}
            loading={loading}
            logsCount={filteredLogs.length}
            onToggleStream={isStreaming ? stop : start}
            onRefresh={refresh}
            onClear={handleClear}
            onExport={handleExport}
          />
        </div>

        {/* 过滤器栏 */}
        <LogFilters
          stats={stats}
          levelFilter={levelFilter}
          onLevelFilterChange={setLevelFilter}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          autoScroll={autoScroll}
          onAutoScrollChange={setAutoScroll}
          totalLogs={logs.length}
        />
      </div>

      {/* 日志内容区域 - 可滚动 */}
      <div className="flex-1 overflow-y-auto bg-surface">
        {loading && logs.length === 0 ? (
          <div className="flex items-center justify-center h-full text-fg-subtle">
            <RefreshCw size={24} className="animate-spin mr-2" />
            加载中...
          </div>
        ) : error ? (
          <div className="flex items-center justify-center h-full text-danger">
            <AlertCircle size={24} className="mr-2" />
            {error}
          </div>
        ) : filteredLogs.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-fg-subtle">
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
