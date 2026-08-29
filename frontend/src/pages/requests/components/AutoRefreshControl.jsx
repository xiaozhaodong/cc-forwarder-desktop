// ============================================
// AutoRefreshControl - 实时刷新状态与手动刷新
// ============================================

import { LoaderCircle, Radio, RotateCcw, WifiOff } from 'lucide-react';

/**
 * @param {Object} props
 * @param {'connecting'|'live'|'fallback'} props.mode - 实时刷新状态
 * @param {number} props.fallbackInterval - 降级刷新间隔（秒）
 * @param {Function} props.onManualRefresh - 手动刷新回调
 * @param {boolean} props.refreshing - 手动刷新进行中
 */
const AutoRefreshControl = ({
  mode = 'connecting',
  fallbackInterval = 30,
  onManualRefresh,
  refreshing = false
}) => {
  const state = mode === 'live'
    ? {
        label: '实时',
        title: '请求变化将主动推送并自动更新',
        icon: <Radio className="w-4 h-4" />,
        className: 'tone-emerald'
      }
    : mode === 'fallback'
      ? {
          label: `降级 ${fallbackInterval}s`,
          title: `实时事件不可用或连续刷新失败，当前每 ${fallbackInterval} 秒自动校准一次`,
          icon: <WifiOff className="w-4 h-4" />,
          className: 'tone-amber'
        }
      : {
          label: '连接中',
          title: '正在建立请求实时事件订阅',
          icon: <LoaderCircle className="w-4 h-4 animate-spin" />,
          className: 'tone-indigo'
        };

  return (
    <div className="flex shrink-0 items-center">
      <div
        className={`flex items-center gap-1.5 xl:gap-2 px-2.5 xl:px-3 h-9 rounded-l-lg text-sm font-medium border ${state.className}`}
        title={state.title}
      >
        {state.icon}
        <span>{state.label}</span>
      </div>

      <button
        onClick={() => onManualRefresh?.()}
        disabled={refreshing}
        className="h-9 w-9 flex items-center justify-center border rounded-r-lg border-line bg-surface text-fg-muted hover:text-accent hover:bg-surface-sub -ml-px transition-colors disabled:cursor-not-allowed disabled:hover:text-fg-muted disabled:hover:bg-surface"
        title={refreshing ? '正在刷新…' : '立即刷新'}
      >
        <RotateCcw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
      </button>
    </div>
  );
};

export default AutoRefreshControl;
