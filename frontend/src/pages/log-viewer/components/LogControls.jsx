// LogControls.jsx - 日志控制按钮组件
import {
  Pause,
  Play,
  RefreshCw,
  Trash2,
  Download,
} from 'lucide-react';

/**
 * 日志控制按钮组
 * @param {boolean} isStreaming 是否正在流式传输
 * @param {boolean} loading 是否加载中
 * @param {number} logsCount 日志数量
 * @param {Function} onToggleStream 切换流状态
 * @param {Function} onRefresh 刷新
 * @param {Function} onClear 清空
 * @param {Function} onExport 导出
 */
function LogControls({
  isStreaming,
  loading,
  logsCount,
  onToggleStream,
  onRefresh,
  onClear,
  onExport,
}) {
  return (
    <div className="flex items-center gap-2">
      {/* 实时状态指示器 */}
      <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg text-xs font-medium bg-surface-mut text-fg-body">
        <span className={`w-2 h-2 rounded-full ${
          isStreaming ? 'bg-success-solid animate-pulse' : 'bg-surface-emph'
        }`} />
        {isStreaming ? '实时' : '已暂停'}
      </div>

      {/* 暂停/开始按钮 */}
      <button
        onClick={onToggleStream}
        className={`p-2 rounded-lg transition-all ${
          isStreaming
            ? 'bg-warn-soft text-warn hover:bg-warn-soft'
            : 'bg-success-soft text-success hover:bg-success-soft'
        }`}
        title={isStreaming ? '暂停' : '开始'}
      >
        {isStreaming ? <Pause size={16} /> : <Play size={16} />}
      </button>

      {/* 刷新按钮 */}
      <button
        onClick={onRefresh}
        disabled={loading}
        className="p-2 rounded-lg bg-surface-mut text-fg-body hover:bg-surface-emph transition-all disabled:opacity-50"
        title="刷新"
      >
        <RefreshCw size={16} className={loading ? 'animate-spin' : ''} />
      </button>

      {/* 清空按钮 */}
      <button
        onClick={onClear}
        className="p-2 rounded-lg bg-surface-mut text-fg-body hover:bg-surface-emph transition-all"
        title="清空"
      >
        <Trash2 size={16} />
      </button>

      {/* 导出按钮 */}
      <button
        onClick={onExport}
        disabled={logsCount === 0}
        className="p-2 rounded-lg bg-surface-mut text-fg-body hover:bg-surface-emph transition-all disabled:opacity-50"
        title="导出"
      >
        <Download size={16} />
      </button>
    </div>
  );
}

export default LogControls;
