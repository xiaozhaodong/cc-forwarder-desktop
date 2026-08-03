import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, Check, LockKeyhole, RotateCcw, Route, Server } from 'lucide-react';

const ClaudeEndpointSwitcher = ({ endpoints = [], routingState = {}, onSwitch, onRestoreAuto, loading = false }) => {
  const [open, setOpen] = useState(false);
  const ref = useRef(null);
  const sorted = useMemo(() => [...endpoints].filter((endpoint) => endpoint.availabilityEnabled !== false).sort((left, right) => (left.priority ?? 1) - (right.priority ?? 1) || String(left.name).localeCompare(String(right.name))), [endpoints]);
  const mode = routingState.mode || 'auto';
  const target = routingState.endpointName || routingState.endpoint_name || '';
  const effective = routingState.lastEffectiveEndpoint || routingState.last_effective_endpoint || '';

  useEffect(() => {
    const close = (event) => { if (ref.current && !ref.current.contains(event.target)) setOpen(false); };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, []);

  if (sorted.length === 0) return <div className="flex items-center gap-2 rounded-lg border border-gray-200 bg-gray-50 px-3 py-1.5 text-sm text-gray-400"><AlertCircle className="h-3.5 w-3.5" />无可用 Claude 端点</div>;

  const label = mode === 'auto' ? `Auto${effective ? ` · ${effective}` : ''}` : target || '未选择';
  return (
    <div ref={ref} className="relative">
      <button type="button" disabled={loading} onClick={() => setOpen((value) => !value)} className="flex w-[190px] items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 shadow-sm hover:border-indigo-300 hover:text-indigo-600 disabled:opacity-50"><Server className="h-3.5 w-3.5 shrink-0 text-gray-400" /><span className="min-w-0 flex-1 truncate text-left">Claude: {label}</span><span className="text-[10px] text-slate-400">{mode === 'manual_fixed' ? '固定' : mode === 'manual_preferred' ? '优选' : '自动'}</span></button>
      {open && (
        <div className="absolute left-0 top-full z-50 mt-2 w-[310px] overflow-hidden rounded-xl border border-gray-100 bg-white shadow-xl ring-1 ring-black/5">
          <button type="button" onClick={async () => { await onRestoreAuto?.(); setOpen(false); }} className={`flex w-full items-center gap-2 border-b border-slate-100 px-4 py-3 text-left text-sm ${mode === 'auto' ? 'bg-indigo-50 text-indigo-700' : 'text-slate-600 hover:bg-slate-50'}`}><RotateCcw size={14} />自动调度{mode === 'auto' && <Check size={14} className="ml-auto" />}</button>
          <div className="max-h-[340px] overflow-y-auto p-2">
            {sorted.map((endpoint) => (
              <div key={endpoint.name} className="rounded-lg px-2 py-2 hover:bg-slate-50">
                <div className="flex items-center gap-2"><span className={`h-2 w-2 rounded-full ${endpoint.healthy ? 'bg-emerald-400' : endpoint.neverChecked ? 'bg-slate-300' : 'bg-rose-400'}`} /><span className="min-w-0 flex-1 truncate text-sm font-medium text-slate-700">{endpoint.name}</span><span className="font-mono text-[10px] text-slate-400">P{endpoint.priority}</span></div>
                <div className="mt-2 flex gap-1.5"><button type="button" onClick={async () => { await onSwitch?.(endpoint.name, 'manual_preferred'); setOpen(false); }} className={`inline-flex flex-1 items-center justify-center gap-1 rounded-md border px-2 py-1.5 text-[11px] ${mode === 'manual_preferred' && target === endpoint.name ? 'border-indigo-200 bg-indigo-50 text-indigo-700' : 'border-slate-200 text-slate-500'}`}><Route size={12} />优选</button><button type="button" onClick={async () => { await onSwitch?.(endpoint.name, 'manual_fixed'); setOpen(false); }} className={`inline-flex flex-1 items-center justify-center gap-1 rounded-md border px-2 py-1.5 text-[11px] ${mode === 'manual_fixed' && target === endpoint.name ? 'border-rose-200 bg-rose-50 text-rose-700' : 'border-slate-200 text-slate-500'}`}><LockKeyhole size={12} />固定</button></div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};

export default ClaudeEndpointSwitcher;
