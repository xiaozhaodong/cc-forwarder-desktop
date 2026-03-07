// ============================================
// 账号池账号行
// 2026-03-07
// ============================================

import { Edit3, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react';
import { formatTimestamp } from '@utils/api.js';
import Badge from './Badge.jsx';
import {
  ACCOUNT_STATE_STYLE,
  QUOTA_STATUS_STYLE,
  normalizePlanType,
  normalizePriorityValue,
  resolveAccountId,
  toAccountAuthLabel,
  toAccountStateLabel,
  toPlanTypeLabel,
  toQuotaProgressClass,
  toQuotaStatusLabel,
  toRemainingPercent
} from '../utils.js';

const toDisplayTime = (value) => (value ? formatTimestamp(value) : '-');

const formatQuotaResetTime = (value) => {
  if (!value) return '';

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }

  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');

  return `${month}-${day} ${hours}:${minutes}`;
};

const buildQuotaDisplayText = (remaining, resetAt) => {
  const remainingText = Number.isFinite(remaining) ? `${remaining.toFixed(0)}%` : '';
  const resetText = formatQuotaResetTime(resetAt);

  if (remainingText && resetText) {
    return `${remainingText} · ${resetText} 重置`;
  }
  if (remainingText) {
    return remainingText;
  }
  if (resetText) {
    return `${resetText} 重置`;
  }
  return '-';
};

const AccountRow = ({
  account,
  accountCount,
  busyKey,
  priorityTierMetaMap,
  onEdit,
  onRefreshProfile,
  onTest,
  onDelete,
  onToggle,
  onMoveTier
}) => {
  const accountId = resolveAccountId(account) ?? account.account_name ?? account.accountName;
  const accountName = account.account_name || account.accountName || '-';
  const state = account.state || 'active';
  const stateClass = ACCOUNT_STATE_STYLE[state] || 'bg-slate-100 text-slate-600 border-slate-200';
  const quotaStatus = String(account.quota_status || account.quotaStatus || '').trim().toLowerCase() || 'pending';
  const quotaStatusClass = QUOTA_STATUS_STYLE[quotaStatus] || QUOTA_STATUS_STYLE.pending;
  const planType = account.plan_type || account.planType || '';
  const normalizedPlanType = normalizePlanType(planType);
  const normalizedProviderType = String(account.provider_type || account.providerType || '').trim().toLowerCase();
  const isAPIKeyAccount = normalizedProviderType === 'api_key';
  const planTypeLabel = toPlanTypeLabel(planType);
  const priority = normalizePriorityValue(account.priority ?? account.Priority);
  const tierMeta = Number.isFinite(priority) ? priorityTierMetaMap.get(priority) : null;
  const canSetAsPrimary = accountCount > 1 && (!tierMeta || tierMeta.order !== 1 || tierMeta.count > 1);
  const canSetAsBackup = accountCount > 1 && (!tierMeta || tierMeta.order !== 2 || tierMeta.count > 1);
  const refreshedAt = account.quota_refreshed_at || account.quotaRefreshedAt;
  const rowBusy = busyKey.startsWith('account-') && busyKey.includes(String(accountId));

  const quota5hUsed = Number.parseFloat(account.quota_5h_used_percent ?? account.quota5hUsedPercent);
  const quotaWeeklyUsed = Number.parseFloat(account.quota_weekly_used_percent ?? account.quotaWeeklyUsedPercent);
  const quota5hRemaining = toRemainingPercent(quota5hUsed);
  const quotaWeeklyRemaining = toRemainingPercent(quotaWeeklyUsed);
  const quota5hResetAt = account.quota_5h_reset_at || account.quota5hResetAt;
  const quotaWeeklyResetAt = account.quota_weekly_reset_at || account.quotaWeeklyResetAt;
  const quota5hDisplayText = buildQuotaDisplayText(quota5hRemaining, quota5hResetAt);
  const quotaWeeklyDisplayText = buildQuotaDisplayText(quotaWeeklyRemaining, quotaWeeklyResetAt);
  const quota5hResetLabel = formatQuotaResetTime(quota5hResetAt);
  const quotaWeeklyResetLabel = formatQuotaResetTime(quotaWeeklyResetAt);

  return (
    <div className={`px-5 py-4 hover:bg-slate-50/60 transition-colors ${!account.enabled ? 'opacity-60' : ''}`}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <button
          type="button"
          role="switch"
          aria-checked={account.enabled}
          onClick={() => onToggle(account)}
          disabled={rowBusy}
          title={account.enabled ? '停用' : '启用'}
          className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400 focus-visible:ring-offset-2 ${
            account.enabled ? 'bg-indigo-500' : 'bg-slate-300'
          } ${rowBusy ? 'opacity-50 cursor-not-allowed' : ''}`}
        >
          <span
            aria-hidden="true"
            className={`pointer-events-none inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ${
              account.enabled ? 'translate-x-[18px]' : 'translate-x-[2px]'
            }`}
          />
        </button>

        <span className="min-w-0 max-w-full truncate text-sm font-semibold text-slate-900 sm:max-w-[220px]" title={accountName}>
          {accountName}
        </span>

        <Badge
          text={toAccountAuthLabel(account.provider_type || account.providerType || '')}
          className="bg-indigo-50 text-indigo-600 border-indigo-100"
        />

        {tierMeta && (
          <Badge
            text={tierMeta.label}
            className={tierMeta.className}
            title={`${tierMeta.description}${tierMeta.count > 1 ? `（当前同层共 ${tierMeta.count} 个账号）` : ''}`}
          />
        )}

        <Badge
          text={`优先级 ${Number.isFinite(priority) ? priority : '-'}`}
          className="bg-amber-50 text-amber-700 border-amber-100"
        />

        {tierMeta?.count > 1 && (
          <Badge
            text={`同层 ${tierMeta.count} 个`}
            className="bg-white text-slate-500 border-slate-200"
            title="相同 priority 的账号属于同一层，按手动主备规则依次切换"
          />
        )}

        {planTypeLabel && (
          <Badge text={planTypeLabel} className="bg-violet-50 text-violet-600 border-violet-100" />
        )}

        <div className="flex items-center gap-1.5 md:ml-auto">
          <Badge text={toQuotaStatusLabel(quotaStatus)} className={quotaStatusClass} />
          <Badge text={toAccountStateLabel(state)} className={stateClass} />
        </div>

        <div className="hidden h-5 w-px shrink-0 bg-slate-200 md:block" />

        <div className="flex items-center gap-1 shrink-0">
          {canSetAsPrimary ? (
            <button
              type="button"
              onClick={() => onMoveTier(account, 'primary')}
              disabled={rowBusy}
              className="inline-flex items-center rounded-md border border-indigo-200 bg-indigo-50 px-2 py-1 text-xs font-medium text-indigo-700 transition-colors hover:bg-indigo-100 disabled:cursor-not-allowed disabled:opacity-50"
              title="将当前账号提升为新的主组，其他账号顺延"
            >
              设为主组
            </button>
          ) : (
            <Badge text="当前主组" className="bg-indigo-50 text-indigo-700 border-indigo-200" />
          )}

          {canSetAsBackup && (
            <button
              type="button"
              onClick={() => onMoveTier(account, 'backup')}
              disabled={rowBusy}
              className="inline-flex items-center rounded-md border border-cyan-200 bg-cyan-50 px-2 py-1 text-xs font-medium text-cyan-700 transition-colors hover:bg-cyan-100 disabled:cursor-not-allowed disabled:opacity-50"
              title="将当前账号切到备组，主组仍优先，其他组顺延"
            >
              设为备组
            </button>
          )}

          {!canSetAsBackup && tierMeta?.order === 2 && tierMeta.count === 1 && (
            <Badge text="当前备组" className="bg-cyan-50 text-cyan-700 border-cyan-200" />
          )}
        </div>

        <div className="flex items-center gap-0.5 shrink-0">
          <button
            type="button"
            className="p-1.5 text-slate-400 hover:bg-indigo-50 hover:text-indigo-600 rounded-md transition-colors cursor-pointer"
            onClick={() => onEdit(account)}
            disabled={rowBusy}
            title="编辑"
          >
            <Edit3 size={14} />
          </button>
          <button
            type="button"
            className="p-1.5 text-slate-400 hover:bg-indigo-50 hover:text-indigo-600 rounded-md transition-colors cursor-pointer"
            onClick={() => onRefreshProfile(account)}
            disabled={rowBusy}
            title="刷新账号信息"
          >
            <RefreshCw size={14} />
          </button>
          <button
            type="button"
            className="p-1.5 text-slate-400 hover:bg-indigo-50 hover:text-indigo-600 rounded-md transition-colors cursor-pointer"
            onClick={() => onTest(account)}
            disabled={rowBusy}
            title="测试连通性"
          >
            <ShieldCheck size={14} />
          </button>
          <button
            type="button"
            className="p-1.5 text-slate-400 hover:bg-rose-50 hover:text-rose-500 rounded-md transition-colors cursor-pointer"
            onClick={() => onDelete(account)}
            disabled={rowBusy}
            title="删除"
          >
            <Trash2 size={14} />
          </button>
        </div>
      </div>

      <div className="mt-3 flex flex-col gap-3 md:ml-12 md:gap-4 lg:flex-row lg:items-center lg:gap-6">
        <div className="flex min-w-0 w-full items-center gap-2 lg:max-w-[360px]">
          <span className="text-[11px] text-slate-400 shrink-0 w-10">5h</span>
          {isAPIKeyAccount ? (
            <span className="text-[11px] text-slate-500">无限额</span>
          ) : normalizedPlanType === 'free' ? (
            <span className="text-[11px] text-slate-300">无额度</span>
          ) : (
            <>
              <div className="flex-1 h-1.5 bg-slate-100 rounded-full overflow-hidden">
                <div
                  className={`h-full rounded-full transition-all duration-500 ${toQuotaProgressClass(quota5hRemaining)}`}
                  style={{ width: `${Number.isFinite(quota5hRemaining) ? quota5hRemaining : 0}%` }}
                />
              </div>
              <span className="text-[11px] font-medium text-slate-600 shrink-0 text-right whitespace-nowrap" title={quota5hResetAt ? `重置 ${formatTimestamp(quota5hResetAt)}` : ''}>
                {Number.isFinite(quota5hRemaining) ? `${quota5hRemaining.toFixed(0)}%` : '-'}
                {quota5hResetLabel ? ` · ${quota5hResetLabel} 重置` : ''}
              </span>
            </>
          )}
        </div>

        <div className="flex min-w-0 w-full items-center gap-2 lg:max-w-[360px]">
          <span className="text-[11px] text-slate-400 shrink-0 w-10">d7</span>
          {isAPIKeyAccount ? (
            <span className="text-[11px] text-slate-500">无限额</span>
          ) : (
            <>
              <div className="flex-1 h-1.5 bg-slate-100 rounded-full overflow-hidden">
                <div
                  className={`h-full rounded-full transition-all duration-500 ${toQuotaProgressClass(quotaWeeklyRemaining)}`}
                  style={{ width: `${Number.isFinite(quotaWeeklyRemaining) ? quotaWeeklyRemaining : 0}%` }}
                />
              </div>
              <span className="text-[11px] font-medium text-slate-600 shrink-0 text-right whitespace-nowrap" title={quotaWeeklyResetAt ? `重置 ${formatTimestamp(quotaWeeklyResetAt)}` : ''}>
                {Number.isFinite(quotaWeeklyRemaining) ? `${quotaWeeklyRemaining.toFixed(0)}%` : '-'}
                {quotaWeeklyResetLabel ? ` · ${quotaWeeklyResetLabel} 重置` : ''}
              </span>
            </>
          )}
        </div>

        <div className="hidden flex-1 lg:block" />

        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-slate-400 lg:shrink-0">
          <span>刷新 {toDisplayTime(refreshedAt)}</span>
          {(account.last_success_at || account.lastSuccessAt) && (
            <span className="text-emerald-500">
              连通 {formatTimestamp(account.last_success_at || account.lastSuccessAt)}
            </span>
          )}
        </div>
      </div>
    </div>
  );
};

export default AccountRow;
