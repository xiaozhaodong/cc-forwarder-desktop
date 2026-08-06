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

const EndpointRow = ({ endpoint, routingState, busy, onEdit, onDelete, onTest, onAvailabilityChange, onAutoScheduleChange, onSetRouting, onClearCooldown }) => {
  const { formatTimestamp } = useTimezone();
  const rewrite = summarizeEndpointModelRewriteRules(endpoint.modelRewriteRules || '');
  const lastCheckRaw = getEndpointLastCheckDisplayValue(endpoint);
  const lastCheck = lastCheckRaw === '-' ? '-' : formatTimestamp(lastCheckRaw);
  return (
    <tr className={`${endpoint.availabilityEnabled ? 'bg-white hover:bg-slate-50/70' : 'bg-slate-50/70 text-slate-400'} transition-colors`}>
      <td className="px-3 py-3 align-top">
        <div className="font-semibold text-slate-900">{endpoint.name}</div>
        <div className="mt-1 max-w-[200px] truncate font-mono text-[11px] text-slate-400" title={endpoint.url}>{endpoint.url}</div>
      </td>
      <td className="px-3 py-3 align-top">
        <div className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-slate-50 px-2 py-1 text-xs text-slate-600"><ShieldCheck size={13} className="text-amber-500" />{summarizeEndpointAuthentication(endpoint)}</div>
      </td>
      <td className="px-3 py-3 text-center align-top"><span className="inline-flex min-w-8 justify-center rounded-full bg-slate-100 px-2 py-1 font-mono text-xs font-semibold text-slate-700">{endpoint.priority ?? 1}</span></td>
      <td className="px-3 py-3 align-top">
        <div className="flex items-center gap-2"><ToggleSwitch enabled={endpoint.availabilityEnabled} disabled={busy} onChange={() => onAvailabilityChange?.(endpoint.name, !endpoint.availabilityEnabled)} title="硬启用" /><span className="text-xs text-slate-500">{endpoint.availabilityEnabled ? '启用' : '停用'}</span></div>
      </td>
      <td className="px-3 py-3 align-top">
        <div className="flex items-center gap-2"><ToggleSwitch enabled={endpoint.failoverEnabled} disabled={busy} onChange={() => onAutoScheduleChange?.(endpoint.name, !endpoint.failoverEnabled)} title="参与自动调度" /><span className="text-xs text-slate-500">{endpoint.failoverEnabled ? '参与' : '排除'}</span></div>
      </td>
      <td className="px-3 py-3 align-top">
        <EndpointHealthBadge endpoint={endpoint} />
        <div className="mt-1 text-[11px] text-slate-400">{lastCheck || '-'}</div>
        {endpoint.responseTimeMs > 0 && <div className="text-[11px] font-mono text-slate-400">{Math.round(endpoint.responseTimeMs)}ms</div>}
      </td>
      <td className="px-3 py-3 align-top">
        <EndpointCooldownInfo endpoint={endpoint} busy={busy} onClearCooldown={onClearCooldown} className="max-w-[150px]" emptyFallback={<span className="text-xs text-slate-300">—</span>} />
      </td>
      <td className="px-3 py-3 align-top">{rewrite ? <span className="block max-w-[150px] truncate rounded-md border border-cyan-100 bg-cyan-50 px-2 py-1 text-[11px] text-cyan-700" title={rewrite.title}>{rewrite.label}</span> : <span className="text-xs text-slate-300">无</span>}</td>
      <td className="px-3 py-3 align-top">
        <ActionButtons endpointName={endpoint.name} routingState={routingState} onSetRouting={onSetRouting} disabled={busy || !endpoint.availabilityEnabled} />
        <EndpointQuickActions endpoint={endpoint} busy={busy} onEdit={onEdit} onDelete={onDelete} onTest={onTest} className="mt-2 justify-end" />
      </td>
    </tr>
  );
};

export default EndpointRow;
