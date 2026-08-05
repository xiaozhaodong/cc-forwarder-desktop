import { Activity, CheckCircle2, Clock3, Copy, Pencil, ShieldCheck, Snowflake, Trash2, XCircle } from 'lucide-react';
import { getEndpointLastCheckDisplayValue } from '../utils/lastCheckDisplay.js';
import { summarizeEndpointModelRewriteRules } from '../utils/modelRewrite.js';
import { summarizeEndpointAuthentication } from '../utils/endpointFormState.js';
import ActionButtons from './ActionButtons.jsx';
import ToggleSwitch from './ToggleSwitch.jsx';
import { useTimezone } from '@contexts/TimezoneContext.jsx';

const Health = ({ endpoint }) => {
  const neverChecked = endpoint.neverChecked || !endpoint.lastCheck;
  if (neverChecked) return <span className="inline-flex items-center gap-1 text-xs text-slate-400"><Clock3 size={13} />未检测</span>;
  if (endpoint.healthy) return <span className="inline-flex items-center gap-1 text-xs font-medium text-emerald-600"><CheckCircle2 size={13} />可达</span>;
  return <span className="inline-flex items-center gap-1 text-xs font-medium text-rose-600"><XCircle size={13} />不可达</span>;
};

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
        <Health endpoint={endpoint} />
        <div className="mt-1 text-[11px] text-slate-400">{lastCheck || '-'}</div>
        {endpoint.responseTimeMs > 0 && <div className="text-[11px] font-mono text-slate-400">{Math.round(endpoint.responseTimeMs)}ms</div>}
      </td>
      <td className="px-3 py-3 align-top">
        {(endpoint.inCooldown || endpoint.cooldownPersistPending) ? (
          <div className="max-w-[150px] text-xs text-amber-700" title={endpoint.cooldownReason}>
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
                ? '确认端点已恢复：解除冷却阻断并重置失败计数（是否参与调度仍取决于硬启用与调度资格）'
                : '上次解除未完成持久化，点击重试（否则重启后冷却可能恢复）'}
            >{endpoint.inCooldown ? '解除冷却' : '重试持久化清除'}</button>
          </div>
        ) : <span className="text-xs text-slate-300">—</span>}
      </td>
      <td className="px-3 py-3 align-top">{rewrite ? <span className="block max-w-[150px] truncate rounded-md border border-cyan-100 bg-cyan-50 px-2 py-1 text-[11px] text-cyan-700" title={rewrite.title}>{rewrite.label}</span> : <span className="text-xs text-slate-300">无</span>}</td>
      <td className="px-3 py-3 align-top">
        <ActionButtons endpointName={endpoint.name} routingState={routingState} onSetRouting={onSetRouting} disabled={busy || !endpoint.availabilityEnabled} />
        <div className="mt-2 flex items-center justify-end gap-1">
          <button type="button" onClick={() => navigator.clipboard.writeText(endpoint.url)} className="rounded-md p-1.5 text-slate-400 hover:bg-slate-100 hover:text-indigo-600" title="复制 URL"><Copy size={14} /></button>
          <button type="button" disabled={busy} onClick={() => onTest?.(endpoint.name)} className="rounded-md p-1.5 text-slate-400 hover:bg-slate-100 hover:text-indigo-600 disabled:opacity-50" title="测试连通性"><Activity size={14} /></button>
          <button type="button" disabled={busy} onClick={() => onEdit?.(endpoint)} className="rounded-md p-1.5 text-slate-400 hover:bg-slate-100 hover:text-indigo-600 disabled:opacity-50" title="编辑"><Pencil size={14} /></button>
          <button type="button" disabled={busy} onClick={() => onDelete?.(endpoint)} className="rounded-md p-1.5 text-slate-400 hover:bg-rose-50 hover:text-rose-600 disabled:opacity-50" title="删除"><Trash2 size={14} /></button>
        </div>
      </td>
    </tr>
  );
};

export default EndpointRow;
