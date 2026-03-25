// ============================================
// 最近一次调度结果面板
// 2026-03-07
// ============================================

import { useEffect, useMemo, useState } from 'react';
import { ChevronDown, ChevronUp, Info } from 'lucide-react';
import { formatTimestamp } from '@utils/api.js';
import Badge from './Badge.jsx';
import ScheduleCandidateCard from './ScheduleCandidateCard.jsx';
import {
  SCHEDULE_OUTCOME_STYLE,
  normalizePriorityValue,
  toScheduleOutcomeLabel
} from '../utils.js';

const DEFAULT_VISIBLE_CANDIDATES = 3;

const toDisplayTime = (value) => (value ? formatTimestamp(value) : '-');

const LatestScheduleSnapshotCard = ({ snapshot = {}, snapshotUnsupported = false }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const [showAllCandidates, setShowAllCandidates] = useState(false);

  const hasSnapshot = snapshot?.hasSnapshot === true || snapshot?.has_snapshot === true;
  const candidates = Array.isArray(snapshot?.candidates) ? snapshot.candidates : [];
  const outcome = String(snapshot?.finalOutcome || snapshot?.final_outcome || '').trim().toLowerCase() || (hasSnapshot ? 'pending' : '');
  const outcomeClass = SCHEDULE_OUTCOME_STYLE[outcome] || SCHEDULE_OUTCOME_STYLE.pending;
  const requestPath = snapshot?.requestPath || snapshot?.request_path || '/v1/responses';
  const selectedAccountName = snapshot?.selectedAccountName || snapshot?.selected_account_name || '-';
  const selectedTierLabel = snapshot?.selectedTierLabel || snapshot?.selected_tier_label || '-';
  const selectedPriority = normalizePriorityValue(snapshot?.selectedPriority ?? snapshot?.selected_priority);
  const capturedAt = snapshot?.capturedAt || snapshot?.captured_at || '';
  const updatedAt = snapshot?.updatedAt || snapshot?.updated_at || '';
  const finalError = snapshot?.finalError || snapshot?.final_error || '';
  const degraded = snapshot?.degradedToLowerPriority || snapshot?.degraded_to_lower_priority;
  const requestId = snapshot?.requestId || snapshot?.request_id || '';

  const isAbnormal = Boolean(degraded) || Boolean(finalError) || (Boolean(outcome) && !['success', 'pending'].includes(outcome));
  const snapshotIdentity = `${requestId}|${updatedAt}|${outcome}|${degraded ? '1' : '0'}|${finalError}`;

  useEffect(() => {
    setIsExpanded(isAbnormal);
    setShowAllCandidates(false);
  }, [snapshotIdentity, isAbnormal]);

  const visibleCandidates = useMemo(() => {
    if (showAllCandidates) {
      return candidates;
    }
    return candidates.slice(0, DEFAULT_VISIBLE_CANDIDATES);
  }, [candidates, showAllCandidates]);

  const hiddenCandidateCount = Math.max(candidates.length - visibleCandidates.length, 0);

  return (
    <section className="bg-white rounded-2xl border border-slate-200/70 shadow-sm overflow-hidden">
      <div className="px-5 py-4 border-b border-slate-100 flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <div className="flex items-start gap-2">
          <div className="mt-0.5 rounded-lg bg-indigo-50 p-1.5 text-indigo-600">
            <Info size={16} />
          </div>
          <div>
            <h2 className="text-base font-semibold text-slate-900">最近一次调度结果</h2>
            <p className="mt-0.5 text-xs text-slate-500">
              默认收起详情；出现失败、降级或错误时会自动展开，避免影响账号管理主视图。
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500">
          {!snapshotUnsupported && (
            <Badge text="5s 自动刷新" className="bg-slate-100 text-slate-600 border-slate-200" />
          )}
          {updatedAt && <span>更新于 {formatTimestamp(updatedAt)}</span>}
        </div>
      </div>

      {snapshotUnsupported ? (
        <div className="px-5 py-5">
          <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
            {snapshot?.message || '当前后端版本暂未提供最近一次调度结果接口。'}
          </div>
        </div>
      ) : !hasSnapshot ? (
        <div className="px-5 py-6">
          <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50/60 px-4 py-5">
            <div className="text-sm font-medium text-slate-900">暂无最近一次调度结果</div>
            <div className="mt-1 text-xs leading-5 text-slate-500">
              发起一次 `/v1/responses` 请求后，这里会显示命中账号、命中组别、组内顺序、是否降级以及候选账号的跳过原因。
            </div>
          </div>
        </div>
      ) : (
        <div className="px-5 py-4">
          <div className={`rounded-xl border px-4 py-3 ${isAbnormal ? 'border-amber-200 bg-amber-50/50' : 'border-slate-200 bg-slate-50/70'}`}>
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2.5 text-sm text-slate-700">
                  <Badge text={toScheduleOutcomeLabel(outcome)} className={outcomeClass} />
                  <span>
                    命中 <span className="font-semibold text-slate-900">{selectedAccountName}</span>
                  </span>
                  <span className="text-slate-500">
                    {selectedTierLabel}
                    {Number.isFinite(selectedPriority) ? ` · 组内顺序 ${selectedPriority}` : ''}
                  </span>
                  <Badge
                    text={degraded ? '已降级' : '未降级'}
                    className={degraded
                      ? 'bg-amber-50 text-amber-700 border-amber-200'
                      : 'bg-emerald-50 text-emerald-700 border-emerald-200'}
                  />
                  <span className="text-xs text-slate-500">{toDisplayTime(updatedAt || capturedAt)}</span>
                </div>

                {finalError && !isExpanded && (
                  <div className="mt-2 text-xs leading-5 text-amber-700 break-all">
                    最终错误：{finalError}
                  </div>
                )}
              </div>

              <div className="flex items-center gap-2 lg:shrink-0">
                <button
                  type="button"
                  onClick={() => setIsExpanded(prev => !prev)}
                  className="inline-flex items-center gap-1 rounded-md border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:border-indigo-200 hover:text-indigo-700 hover:bg-indigo-50"
                >
                  {isExpanded ? '收起详情' : '查看详情'}
                  {isExpanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                </button>
              </div>
            </div>
          </div>

          {isExpanded && (
            <div className="mt-3 space-y-4">
              <div className="rounded-xl border border-slate-200 bg-white p-4">
                <div className="text-sm font-semibold text-slate-900">
                  {snapshot?.summary || '已生成最近一次调度结果摘要'}
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-500">
                  <span>请求路径 {requestPath}</span>
                  {requestId ? (
                    <span title={requestId}>请求 ID {requestId.slice(0, 12)}...</span>
                  ) : null}
                  <span>最近捕获 {toDisplayTime(capturedAt)}</span>
                  <span>候选账号 {candidates.length}</span>
                </div>

                {finalError && (
                  <div className="mt-3 rounded-lg border border-amber-100 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-700">
                    最终错误：{finalError}
                  </div>
                )}
              </div>

              <div className="space-y-3">
                <div className="flex flex-col gap-1 md:flex-row md:items-center md:justify-between">
                  <div className="text-sm font-semibold text-slate-900">候选账号决策列表</div>
                  <div className="text-xs text-slate-500">
                    {hiddenCandidateCount > 0
                      ? `默认展示前 ${DEFAULT_VISIBLE_CANDIDATES} 个候选，避免面板过长`
                      : '按本次调度解释顺序展示'}
                  </div>
                </div>

                {candidates.length === 0 ? (
                  <div className="rounded-xl border border-dashed border-slate-200 bg-slate-50/60 px-4 py-4 text-xs text-slate-500">
                    当前快照没有候选账号决策明细。
                  </div>
                ) : (
                  <>
                    <div className="space-y-3">
                      {visibleCandidates.map((candidate, index) => (
                        <ScheduleCandidateCard
                          key={String(candidate.id ?? candidate.account_id ?? candidate.accountId ?? `${candidate.account_name || candidate.accountName || 'candidate'}-${index}`)}
                          candidate={candidate}
                          index={index}
                        />
                      ))}
                    </div>

                    {candidates.length > DEFAULT_VISIBLE_CANDIDATES && (
                      <div className="flex justify-center pt-1">
                        <button
                          type="button"
                          onClick={() => setShowAllCandidates(prev => !prev)}
                          className="inline-flex items-center gap-1 rounded-md border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:border-indigo-200 hover:text-indigo-700 hover:bg-indigo-50"
                        >
                          {showAllCandidates ? '收起候选列表' : `显示其余 ${hiddenCandidateCount} 个候选`}
                        </button>
                      </div>
                    )}
                  </>
                )}
              </div>
            </div>
          )}
        </div>
      )}
    </section>
  );
};

export default LatestScheduleSnapshotCard;
