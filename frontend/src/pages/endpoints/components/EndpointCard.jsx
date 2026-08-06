import { ShieldCheck } from 'lucide-react';
import { getEndpointLastCheckDisplayValue } from '../utils/lastCheckDisplay.js';
import { summarizeEndpointModelRewriteRules } from '../utils/modelRewrite.js';
import { summarizeEndpointAuthentication } from '../utils/endpointFormState.js';
import ActionButtons from './ActionButtons.jsx';
import EndpointCooldownInfo from './EndpointCooldownInfo.jsx';
import EndpointHealthBadge from './EndpointHealthBadge.jsx';
import EndpointQuickActions from './EndpointQuickActions.jsx';
import ToggleSwitch from './ToggleSwitch.jsx';
import { useTimezone } from '@contexts/TimezoneContext.jsx';

// 头部名称前的状态点:冷却 > 不可达 > 可达 > 未检测
const statusDotClass = (endpoint) => {
  if (endpoint.inCooldown || endpoint.cooldownPersistPending) return 'bg-amber-400';
  if (endpoint.neverChecked || !endpoint.lastCheck) return 'bg-slate-300';
  return endpoint.healthy ? 'bg-emerald-500' : 'bg-rose-500';
};

const EndpointCard = ({ endpoint, routingState, busy, onEdit, onDelete, onTest, onAvailabilityChange, onAutoScheduleChange, onSetRouting, onClearCooldown }) => {
  const { formatTimestamp } = useTimezone();
  const rewrite = summarizeEndpointModelRewriteRules(endpoint.modelRewriteRules || '');
  const lastCheckRaw = getEndpointLastCheckDisplayValue(endpoint);
  const lastCheck = lastCheckRaw === '-' ? '-' : formatTimestamp(lastCheckRaw);

  const routingMode = routingState?.mode || 'auto';
  const routingTarget = routingState?.endpointName || routingState?.endpoint_name || '';
  const routingSelected = routingMode !== 'auto' && routingTarget === endpoint.name;

  const disabled = !endpoint.availabilityEnabled;

  return (
    <div
      className={`flex flex-col rounded-2xl border p-4 shadow-sm transition ${
        disabled
          ? 'border-slate-200 bg-slate-50/70 text-slate-400'
          : routingSelected
            ? 'border-indigo-300 bg-white ring-1 ring-indigo-200'
            : 'border-slate-200 bg-white hover:border-slate-300'
      }`}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className={`h-2 w-2 shrink-0 rounded-full ${statusDotClass(endpoint)}`} />
          <span className="truncate font-semibold text-slate-900" title={endpoint.name}>{endpoint.name}</span>
        </div>
        <ActionButtons endpointName={endpoint.name} routingState={routingState} onSetRouting={onSetRouting} disabled={busy || disabled} />
      </div>
      <div className="mt-1 truncate font-mono text-[11px] text-slate-400" title={endpoint.url}>{endpoint.url}</div>

      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        <span className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-slate-50 px-2 py-1 text-xs text-slate-600"><ShieldCheck size={13} className="text-amber-500" />{summarizeEndpointAuthentication(endpoint)}</span>
        <span className="inline-flex items-center rounded-full bg-slate-100 px-2 py-1 font-mono text-xs font-semibold text-slate-700">P{endpoint.priority ?? 1}</span>
        {rewrite && <span className="max-w-[160px] truncate rounded-md border border-cyan-100 bg-cyan-50 px-2 py-1 text-[11px] text-cyan-700" title={rewrite.title}>{rewrite.label}</span>}
      </div>

      <div className="mt-3 space-y-1.5 text-xs">
        <div className="flex h-5 flex-wrap items-center gap-x-2 gap-y-1">
          <EndpointHealthBadge endpoint={endpoint} />
          <span className="text-[11px] text-slate-400">{lastCheck || '-'}</span>
          {endpoint.responseTimeMs > 0 && <span className="font-mono text-[11px] text-slate-400">{Math.round(endpoint.responseTimeMs)}ms</span>}
        </div>
        <div className="flex h-5 items-center">
          <EndpointCooldownInfo endpoint={endpoint} busy={busy} onClearCooldown={onClearCooldown} compact />
        </div>
      </div>

      <div className="mt-4 flex items-center justify-between gap-2 border-t border-slate-100 pt-3">
        <div className="flex items-center gap-4">
          <label className="flex items-center gap-1.5" title="硬启用">
            <ToggleSwitch enabled={endpoint.availabilityEnabled} disabled={busy} onChange={() => onAvailabilityChange?.(endpoint.name, !endpoint.availabilityEnabled)} title="硬启用" />
            <span className="text-xs text-slate-500">启用</span>
          </label>
          <label className="flex items-center gap-1.5" title="参与自动调度">
            <ToggleSwitch enabled={endpoint.failoverEnabled} disabled={busy} onChange={() => onAutoScheduleChange?.(endpoint.name, !endpoint.failoverEnabled)} title="参与自动调度" />
            <span className="text-xs text-slate-500">调度</span>
          </label>
        </div>
        <EndpointQuickActions endpoint={endpoint} busy={busy} onEdit={onEdit} onDelete={onDelete} onTest={onTest} />
      </div>
    </div>
  );
};

export default EndpointCard;
