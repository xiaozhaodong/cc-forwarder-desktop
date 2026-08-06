import { Snowflake } from 'lucide-react';
import { useTimezone } from '@contexts/TimezoneContext.jsx';

// 端点冷却状态与手动解除入口:表格行与网格卡片共用
// 无冷却状态时渲染 emptyFallback(表格传 “—” 占位,卡片传 null 配合固定高度容器隐藏)
// compact(网格卡片用):单行呈现,雪花按钮 inline-flex 无 mt,与连通性行同高,卡片高度不抖动
const EndpointCooldownInfo = ({ endpoint, busy = false, onClearCooldown, emptyFallback = null, className = '', compact = false }) => {
  const { formatTimestamp, formatMonthDayTime } = useTimezone();
  if (!endpoint.inCooldown && !endpoint.cooldownPersistPending) return emptyFallback;

  if (compact) {
    return (
      <div className={`flex items-center gap-1.5 truncate text-xs text-amber-700 ${className}`} title={endpoint.cooldownReason}>
        {endpoint.inCooldown ? (
          <>
            <span className="inline-flex shrink-0 items-center gap-1 font-medium"><Snowflake size={13} />冷却中</span>
            <span className="truncate text-[11px] text-amber-600">{endpoint.cooldownUntil ? formatMonthDayTime(endpoint.cooldownUntil) : (endpoint.cooldownReason || '-')}</span>
          </>
        ) : (
          <span className="inline-flex shrink-0 items-center gap-1 font-medium text-amber-600">解除未完全持久化</span>
        )}
        <button
          type="button"
          disabled={busy}
          onClick={() => onClearCooldown?.(endpoint.name)}
          className="inline-flex shrink-0 items-center rounded-md border border-amber-200 bg-amber-50 px-1.5 py-0.5 text-[11px] font-medium text-amber-700 hover:bg-amber-100 disabled:opacity-50"
          title={endpoint.inCooldown
            ? '确认端点已恢复:解除冷却阻断并重置失败计数(是否参与调度仍取决于硬启用与调度资格)'
            : '上次解除未完成持久化,点击重试(否则重启后冷却可能恢复)'}
        >{endpoint.inCooldown ? '解除冷却' : '重试持久化清除'}</button>
      </div>
    );
  }

  return (
    <div className={`text-xs text-amber-700 ${className}`} title={endpoint.cooldownReason}>
      {endpoint.inCooldown ? (
        <>
          <span className="inline-flex items-center gap-1 font-medium"><Snowflake size={13} />冷却中</span>
          <div className="mt-1 truncate text-[11px] text-amber-600">{endpoint.cooldownUntil ? formatTimestamp(endpoint.cooldownUntil) : (endpoint.cooldownReason || '-')}</div>
        </>
      ) : (
        <span className="inline-flex items-center gap-1 font-medium text-amber-600">解除未完全持久化</span>
      )}
      <button
        type="button"
        disabled={busy}
        onClick={() => onClearCooldown?.(endpoint.name)}
        className="mt-1 rounded-md border border-amber-200 bg-amber-50 px-1.5 py-0.5 text-[11px] font-medium text-amber-700 hover:bg-amber-100 disabled:opacity-50"
        title={endpoint.inCooldown
          ? '确认端点已恢复:解除冷却阻断并重置失败计数(是否参与调度仍取决于硬启用与调度资格)'
          : '上次解除未完成持久化,点击重试(否则重启后冷却可能恢复)'}
      >{endpoint.inCooldown ? '解除冷却' : '重试持久化清除'}</button>
    </div>
  );
};

export default EndpointCooldownInfo;
