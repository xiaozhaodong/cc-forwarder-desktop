import { LockKeyhole, Route, RotateCcw } from 'lucide-react';

const ActionButtons = ({ endpointName, routingState = {}, onSetRouting, disabled = false }) => {
  const mode = routingState.mode || 'auto';
  const target = routingState.endpointName || routingState.endpoint_name || '';
  const selected = target === endpointName;

  const buttonClass = (active, tone = 'indigo') => {
    const activeClass = tone === 'rose'
      ? 'border-rose-200 bg-rose-50 text-rose-700'
      : 'border-indigo-200 bg-indigo-50 text-indigo-700';
    return `inline-flex items-center gap-1 rounded-md border px-2 py-1 text-[11px] font-medium transition disabled:cursor-not-allowed disabled:opacity-50 ${active ? activeClass : 'border-slate-200 bg-white text-slate-500 hover:border-slate-300 hover:text-slate-700'}`;
  };

  return (
    <div className="flex flex-wrap items-center justify-end gap-1">
      <button type="button" disabled={disabled} onClick={() => onSetRouting?.('manual_preferred', endpointName)} className={buttonClass(selected && mode === 'manual_preferred')} title="请求优先使用此端点；失败时允许 fallback"><Route size={12} />优选</button>
      <button type="button" disabled={disabled} onClick={() => onSetRouting?.('manual_fixed', endpointName)} className={buttonClass(selected && mode === 'manual_fixed', 'rose')} title="严格固定此端点，不做 fallback"><LockKeyhole size={12} />固定</button>
      {selected && mode !== 'auto' && <button type="button" disabled={disabled} onClick={() => onSetRouting?.('auto', '')} className={buttonClass(false)} title="恢复自动调度"><RotateCcw size={12} />自动</button>}
    </div>
  );
};

export default ActionButtons;
