import { useState } from 'react';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Clock3,
  Route,
  SkipForward
} from 'lucide-react';
import { formatTimestamp } from '@utils/api.js';

const outcomeMeta = {
  pending: ['调度中', 'bg-blue-50 text-blue-700 border-blue-200'],
  success: ['成功', 'bg-emerald-50 text-emerald-700 border-emerald-200'],
  quality_incomplete: ['响应不完整', 'bg-amber-50 text-amber-700 border-amber-200'],
  failed_after_commit: ['响应阶段失败', 'bg-rose-50 text-rose-700 border-rose-200'],
  cancelled: ['已取消', 'bg-slate-100 text-slate-600 border-slate-200'],
  privacy_blocked: ['隐私策略拦截', 'bg-rose-50 text-rose-700 border-rose-200'],
  passthrough_error: ['上游失败', 'bg-rose-50 text-rose-700 border-rose-200'],
  passthrough_raw: ['原样透传', 'bg-amber-50 text-amber-700 border-amber-200'],
  no_candidates: ['无可用候选', 'bg-rose-50 text-rose-700 border-rose-200'],
  manual_fixed_blocked: ['固定端点不可用', 'bg-rose-50 text-rose-700 border-rose-200'],
  all_candidates_failed: ['候选全部失败', 'bg-rose-50 text-rose-700 border-rose-200'],
  rejected_by_failure_tracker: ['失败阈值拒绝', 'bg-rose-50 text-rose-700 border-rose-200']
};

const runtimeLabels = {
  attempting: '尝试中',
  try_next: '失败，尝试下一候选',
  success: '成功命中',
  quality_incomplete: '响应不完整',
  failed_after_commit: '响应阶段失败',
  cancelled: '已取消',
  privacy_blocked: '隐私策略拦截',
  passthrough_error: '上游失败',
  passthrough_raw: '原样透传'
};

const reasonLabels = {
  active: '当前激活端点',
  active_failover_disabled: '当前激活端点（故障转移关闭）',
  failover_candidate: '故障转移候选',
  manual_fixed: '手动固定目标',
  failover_disabled_endpoint: '端点未参与故障转移',
  active_endpoint_missing: '当前激活端点不存在',
  endpoint_missing: '端点不存在',
  paused: '手动暂停中',
  cooldown: '冷却中',
  failure_threshold_tripped: '失败阈值已触发',
  manual_fixed_target_missing: '手动固定目标不存在',
  manual_fixed_paused: '手动固定目标已暂停',
  manual_fixed_cooldown: '手动固定目标冷却中',
  manual_fixed_failure_threshold_tripped: '手动固定目标触发失败阈值'
};

const negativeCacheLabels = {
  model_unsupported: '模型不支持',
  schema_incompatible: '请求 schema 不兼容',
  payload_too_large: '请求体过大',
  count_tokens_unsupported: '不支持 count_tokens',
  client_cancel: '客户端取消'
};

const resolveReasonLabel = (reason) => {
  if (!reason) return '-';
  if (reasonLabels[reason]) return reasonLabels[reason];
  if (reason.startsWith('manual_fixed_')) {
    return `手动固定目标：${resolveReasonLabel(reason.slice('manual_fixed_'.length))}`;
  }
  if (reason.startsWith('negative_cache_')) {
    const failureClass = reason.slice('negative_cache_'.length);
    return `路由负缓存：${negativeCacheLabels[failureClass] || failureClass}`;
  }
  return reason;
};

const routeModeLabels = {
  auto: '自动调度',
  manual_preferred: '手动优选',
  manual_fixed: '手动固定'
};

const formatTime = (value) => (value ? formatTimestamp(value) : '-');

const EndpointScheduleSnapshotCard = ({ snapshot = {}, unsupported = false }) => {
  const outcome = snapshot.finalOutcome || (snapshot.hasSnapshot ? 'pending' : '');
  const decisions = Array.isArray(snapshot.decisions) ? snapshot.decisions : [];
  const isAbnormal = Boolean(outcome) && !['pending', 'success'].includes(outcome);
  const identity = `${snapshot.requestId}|${snapshot.updatedAt}|${outcome}|${snapshot.finalError}`;
  const [expansionState, setExpansionState] = useState({ identity: '', expanded: false });
  const expanded = expansionState.identity === identity ? expansionState.expanded : isAbnormal;
  const effectiveFailoverEnabled = snapshot.failoverEnabled
    && snapshot.routeFallbackEnabled !== false
    && snapshot.routeMode !== 'manual_fixed';

  const counts = decisions.reduce((result, decision) => {
    if (decision.decision === 'candidate') result.candidates += 1;
    if (decision.decision === 'skipped') result.skipped += 1;
    return result;
  }, { candidates: 0, skipped: 0 });

  const [outcomeLabel, outcomeClass] = outcomeMeta[outcome] || [outcome || '未知', 'bg-slate-100 text-slate-600 border-slate-200'];

  return (
    <section className="mb-6 overflow-hidden rounded-2xl border border-slate-200/70 bg-white shadow-sm">
      <div className="flex flex-col gap-3 border-b border-slate-100 px-5 py-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex items-start gap-3">
          <div className={`mt-0.5 rounded-xl p-2 ${isAbnormal ? 'bg-amber-50 text-amber-600' : 'bg-indigo-50 text-indigo-600'}`}>
            {isAbnormal ? <AlertTriangle size={18} /> : <Route size={18} />}
          </div>
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-base font-semibold text-slate-900">最近一次端点调度</h2>
              {!unsupported && <span className="rounded-full border border-slate-200 bg-slate-50 px-2 py-0.5 text-[10px] font-medium text-slate-500">5s 自动刷新</span>}
            </div>
            <p className="mt-1 text-xs text-slate-500">展示真实请求的候选筛选、跳过原因、实际命中端点与最终结果。</p>
          </div>
        </div>

        {snapshot.hasSnapshot && (
          <button
            type="button"
            onClick={() => setExpansionState({ identity, expanded: !expanded })}
            className="inline-flex items-center justify-center gap-1 rounded-lg border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:border-indigo-200 hover:bg-indigo-50 hover:text-indigo-700"
          >
            {expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
            {expanded ? '收起决策明细' : '查看决策明细'}
          </button>
        )}
      </div>

      {unsupported ? (
        <div className="px-5 py-5 text-sm text-slate-600">{snapshot.message || '当前运行版本暂不支持端点调度快照。'}</div>
      ) : !snapshot.hasSnapshot ? (
        <div className="px-5 py-6">
          <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50/70 px-4 py-4">
            <div className="text-sm font-medium text-slate-800">暂无端点调度记录</div>
            <div className="mt-1 text-xs leading-5 text-slate-500">发起一次 Claude `/v1/messages` 请求后，这里会立即显示本次请求为何选择或跳过每个端点。</div>
          </div>
        </div>
      ) : (
        <div className="px-5 py-4">
          <div className={`rounded-xl border px-4 py-3 ${isAbnormal ? 'border-amber-200 bg-amber-50/40' : 'border-slate-200 bg-slate-50/70'}`}>
            <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className={`rounded-full border px-2.5 py-1 text-xs font-semibold ${outcomeClass}`}>{outcomeLabel}</span>
                  <span className="truncate text-sm font-medium text-slate-800">{snapshot.summary || '端点调度已完成'}</span>
                </div>
                <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500">
                  <span>请求：<span className="font-mono text-slate-700">{snapshot.requestPath || '-'}</span></span>
                  <span>更新：{formatTime(snapshot.updatedAt || snapshot.capturedAt)}</span>
                  <span>候选 {counts.candidates} · 跳过 {counts.skipped}</span>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-x-6 gap-y-2 text-xs sm:grid-cols-4">
                <div><div className="text-slate-400">实际命中</div><div className="mt-0.5 max-w-36 truncate font-semibold text-slate-800" title={snapshot.selectedEndpoint}>{snapshot.selectedEndpoint || '-'}</div></div>
                <div><div className="text-slate-400">调度时激活</div><div className="mt-0.5 max-w-36 truncate font-medium text-slate-700" title={snapshot.activeEndpointAtSelection}>{snapshot.activeEndpointAtSelection || '-'}</div></div>
                <div><div className="text-slate-400">路由模式</div><div className="mt-0.5 font-medium text-slate-700">{routeModeLabels[snapshot.routeMode] || snapshot.routeMode || '自动调度'}</div></div>
                <div><div className="text-slate-400">请求内故障转移</div><div className={`mt-0.5 font-medium ${effectiveFailoverEnabled ? 'text-emerald-700' : 'text-slate-500'}`}>{effectiveFailoverEnabled ? '已启用' : snapshot.routeMode === 'manual_fixed' ? '固定模式关闭' : '已关闭'}</div></div>
              </div>
            </div>
          </div>

          {expanded && (
            <div className="mt-4 space-y-4">
              <div className="grid grid-cols-1 gap-3 text-xs md:grid-cols-3">
                <div className="rounded-lg border border-slate-100 bg-slate-50/60 px-3 py-2"><span className="text-slate-400">请求 ID</span><div className="mt-1 truncate font-mono text-slate-700" title={snapshot.requestId}>{snapshot.requestId || '-'}</div></div>
                <div className="rounded-lg border border-slate-100 bg-slate-50/60 px-3 py-2"><span className="text-slate-400">路由目标</span><div className="mt-1 truncate font-medium text-slate-700">{snapshot.routeEndpointName || '按自动策略选择'}</div></div>
                <div className="rounded-lg border border-slate-100 bg-slate-50/60 px-3 py-2"><span className="text-slate-400">捕获时间</span><div className="mt-1 font-medium text-slate-700">{formatTime(snapshot.capturedAt)}</div></div>
              </div>

              {snapshot.finalError && (
                <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">最终信息：{snapshot.finalError}</div>
              )}

              <div>
                <div className="mb-2 flex items-center justify-between">
                  <div className="text-sm font-semibold text-slate-900">候选与跳过决策</div>
                  <div className="text-xs text-slate-400">按调度解释顺序展示</div>
                </div>
                {decisions.length === 0 ? (
                  <div className="rounded-xl border border-dashed border-slate-200 px-4 py-4 text-xs text-slate-500">本次请求未生成端点候选。</div>
                ) : (
                  <div className="space-y-2">
                    {decisions.map((decision, index) => {
                      const candidate = decision.decision === 'candidate';
                      return (
                        <div key={`${decision.name}-${index}`} className="rounded-xl border border-slate-200/70 bg-white px-3 py-3">
                          <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                            <div className="flex min-w-0 items-center gap-2">
                              <div className={`rounded-lg p-1.5 ${candidate ? 'bg-indigo-50 text-indigo-600' : 'bg-slate-100 text-slate-500'}`}>
                                {candidate ? <Activity size={14} /> : <SkipForward size={14} />}
                              </div>
                              <div className="min-w-0">
                                <div className="flex flex-wrap items-center gap-2">
                                  <span className="truncate text-sm font-semibold text-slate-800">{decision.name}</span>
                                  <span className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${candidate ? 'bg-indigo-50 text-indigo-700' : 'bg-slate-100 text-slate-600'}`}>{candidate ? '候选' : '跳过'}</span>
                                  {decision.runtimeOutcome && (
                                    <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium ${decision.runtimeOutcome === 'success' ? 'bg-emerald-50 text-emerald-700' : decision.runtimeOutcome === 'attempting' ? 'bg-blue-50 text-blue-700' : 'bg-amber-50 text-amber-700'}`}>
                                      {decision.runtimeOutcome === 'success' ? <CheckCircle2 size={10} /> : <Clock3 size={10} />}
                                      {runtimeLabels[decision.runtimeOutcome] || decision.runtimeOutcome}
                                    </span>
                                  )}
                                </div>
                                <div className="mt-1 text-xs text-slate-500">{resolveReasonLabel(decision.reason)}</div>
                              </div>
                            </div>
                            {decision.availableAt && (
                              <div className="shrink-0 text-xs text-amber-700">预计恢复：{formatTime(decision.availableAt)}</div>
                            )}
                          </div>
                          {decision.runtimeError && <div className="mt-2 rounded-lg bg-rose-50 px-2.5 py-1.5 text-xs leading-5 text-rose-700">{decision.runtimeError}</div>}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </section>
  );
};

export default EndpointScheduleSnapshotCard;
