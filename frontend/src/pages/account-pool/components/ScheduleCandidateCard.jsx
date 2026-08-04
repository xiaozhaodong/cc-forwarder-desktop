// ============================================
// 最近一次调度候选账号卡片
// 2026-03-07
// ============================================

import { useTimezone } from '@contexts/TimezoneContext.jsx';
import Badge from './Badge.jsx';
import {
  MANUAL_FAILOVER_TIER_PRESETS,
  QUOTA_STATUS_STYLE,
  SCHEDULE_DECISION_STYLE,
  SCHEDULE_OUTCOME_STYLE,
  normalizePriorityValue,
  resolveAccountId,
  toAccountAuthLabel,
  toQuotaStatusLabel,
  toScheduleDecisionLabel,
  toScheduleOutcomeLabel,
  toScheduleReasonLabel
} from '../utils.js';

const ScheduleCandidateCard = ({ candidate, index }) => {
  const { formatTimestamp } = useTimezone();
  const candidateId = resolveAccountId(candidate) ?? `${candidate.account_name || candidate.accountName || 'candidate'}-${index}`;
  const decision = String(candidate.decision || '').trim().toLowerCase();
  const decisionClass = SCHEDULE_DECISION_STYLE[decision] || SCHEDULE_DECISION_STYLE.skipped;
  const runtimeOutcome = String(candidate.runtime_outcome || candidate.runtimeOutcome || '').trim().toLowerCase();
  const runtimeOutcomeClass = SCHEDULE_OUTCOME_STYLE[runtimeOutcome] || SCHEDULE_OUTCOME_STYLE.pending;
  const quotaStatus = String(candidate.quota_status || candidate.quotaStatus || '').trim().toLowerCase() || 'pending';
  const quotaClass = QUOTA_STATUS_STYLE[quotaStatus] || QUOTA_STATUS_STYLE.pending;
  const candidatePriority = normalizePriorityValue(candidate.priority ?? candidate.Priority);
  const candidateTierIndex = normalizePriorityValue(candidate.tier_index ?? candidate.tierIndex);
  const candidateTierClass = MANUAL_FAILOVER_TIER_PRESETS[(candidateTierIndex || 1) - 1]?.className || 'bg-slate-100 text-slate-700 border-slate-200';
  const effectiveQuotaRemaining = Number.parseFloat(candidate.effective_quota_remaining ?? candidate.effectiveQuotaRemaining);
  const failCount = Number.parseInt(candidate.fail_count ?? candidate.failCount, 10) || 0;
  const lastSuccessAt = candidate.last_success_at || candidate.lastSuccessAt || '';
  const runtimeError = candidate.runtime_error || candidate.runtimeError || '';

  return (
    <div
      key={String(candidateId)}
      className={`rounded-xl border px-4 py-4 shadow-sm ${decision === 'selected' ? 'border-indigo-200 bg-indigo-50/40' : 'border-slate-200 bg-white'}`}
    >
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs font-medium text-slate-400">#{index + 1}</span>
            <span className="text-sm font-semibold text-slate-900">{candidate.account_name || candidate.accountName || '-'}</span>
            <Badge
              text={toAccountAuthLabel(candidate.provider_type || candidate.providerType || '')}
              className="bg-indigo-50 text-indigo-600 border-indigo-100"
            />
            <Badge
              text={`${candidate.tier_label || candidate.tierLabel || '未分组'}${Number.isFinite(candidatePriority) ? ` · 组内顺序 ${candidatePriority}` : ''}`}
              className={candidateTierClass}
            />
            <Badge text={toScheduleDecisionLabel(decision)} className={decisionClass} />
            {runtimeOutcome && (
              <Badge text={toScheduleOutcomeLabel(runtimeOutcome)} className={runtimeOutcomeClass} />
            )}
          </div>

          <div className="mt-2 text-sm font-medium text-slate-800">
            {toScheduleReasonLabel(candidate.reason || '')}
          </div>
          <div className="mt-1 text-xs leading-5 text-slate-500">
            {candidate.reason_detail || candidate.reasonDetail || '调度器未返回更详细的解释。'}
          </div>
        </div>

        <div className="flex flex-wrap gap-2 lg:max-w-[320px] lg:justify-end">
          <Badge text={toQuotaStatusLabel(quotaStatus)} className={quotaClass} />
          {Number.isFinite(effectiveQuotaRemaining) && (
            <Badge
              text={`剩余额度 ${effectiveQuotaRemaining.toFixed(0)}%`}
              className="bg-white text-slate-600 border-slate-200"
            />
          )}
          <Badge text={`失败 ${failCount}`} className="bg-white text-slate-600 border-slate-200" />
          {lastSuccessAt && (
            <Badge
              text={`最近成功 ${formatTimestamp(lastSuccessAt)}`}
              className="bg-white text-emerald-700 border-emerald-200"
            />
          )}
        </div>
      </div>

      {runtimeError && (
        <div className="mt-3 rounded-lg border border-amber-100 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-700">
          运行时错误：{runtimeError}
        </div>
      )}
    </div>
  );
};

export default ScheduleCandidateCard;
