// ============================================
// Account Pool 调度组卡片（紧凑版）
// 2026-03-22
// ============================================

import { useState } from 'react';
import { ArrowUpCircle, ChevronDown, ChevronRight, ExternalLink, Pin, RotateCcw, Target } from 'lucide-react';
import { Button } from '@components/ui';
import Badge from './Badge.jsx';

const DEFAULT_VISIBLE = 10;

const GROUP_STYLE = {
  primary: {
    label: '主组',
    badgeClassName: 'bg-indigo-50 text-indigo-700 border-indigo-200',
    ringClassName: 'border-indigo-200/80',
    headerBg: 'bg-indigo-50/50'
  },
  backup: {
    label: '备组',
    badgeClassName: 'bg-cyan-50 text-cyan-700 border-cyan-200',
    ringClassName: 'border-cyan-200/80',
    headerBg: 'bg-cyan-50/50'
  },
  cold: {
    label: '冷备',
    badgeClassName: 'bg-violet-50 text-violet-700 border-violet-200',
    ringClassName: 'border-violet-200/80',
    headerBg: 'bg-violet-50/50'
  }
};

const STATE_TONE_CLASS = {
  rose: 'bg-rose-50 text-rose-700 border-rose-200',
  red: 'bg-rose-50 text-rose-700 border-rose-200',
  amber: 'bg-amber-50 text-amber-700 border-amber-200',
  emerald: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  green: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  slate: 'bg-slate-50 text-slate-600 border-slate-200'
};

const normalizeGroupKey = (key = '') => {
  const normalized = String(key || '').trim().toLowerCase();
  if (normalized === 'primary' || normalized.includes('main')) return 'primary';
  if (normalized === 'backup' || normalized.includes('secondary')) return 'backup';
  return 'cold';
};

const toAccountId = (account = {}) => account?.id ?? account?.accountId ?? account?.account_id;
const toActionAccount = (account = {}) => account?.rawAccount || account;

const ACTION_ICON_BY_KEY = {
  'swap-up': ArrowUpCircle,
  'swap-down': RotateCcw
};

const ACTION_VARIANT_BY_TONE = {
  primary: 'primary',
  secondary: 'secondary',
  ghost: 'ghost'
};

const quotaStrokeColor = (text, percent) => {
  if (!text || text === '-' || text === '无' || text === '未刷新') return '#cbd5e1';
  if (text === '无限额') return '#94a3b8';
  if (!Number.isFinite(percent)) return '#94a3b8';
  if (percent > 50) return '#34d399';
  if (percent > 20) return '#fbbf24';
  return '#f87171';
};

const quotaTextColor = (text, percent) => {
  if (!text || text === '-' || text === '无' || text === '未刷新') return 'text-slate-400';
  if (text === '无限额') return 'text-slate-500';
  if (!Number.isFinite(percent)) return 'text-slate-500';
  if (percent > 50) return 'text-emerald-600';
  if (percent > 20) return 'text-amber-600';
  return 'text-rose-600';
};

const RING_SIZE = 20;
const RING_STROKE = 2.5;
const RING_RADIUS = (RING_SIZE - RING_STROKE) / 2;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

const MiniQuotaRing = ({ label, text, percent }) => {
  const hasRing = Number.isFinite(percent);
  const fillPercent = hasRing ? Math.max(Math.min(percent, 100), 0) : 0;
  const dashOffset = RING_CIRCUMFERENCE - (fillPercent / 100) * RING_CIRCUMFERENCE;
  const stroke = quotaStrokeColor(text, percent);
  const isInfinite = text === '无限额';
  const isEmpty = !text || text === '-' || text === '无' || text === '未刷新';
  const tooltip = `${label}: ${text || '-'}`;

  return (
    <div className="relative shrink-0" style={{ width: RING_SIZE, height: RING_SIZE }} title={tooltip}>
      <svg width={RING_SIZE} height={RING_SIZE} className="-rotate-90">
        <circle cx={RING_SIZE / 2} cy={RING_SIZE / 2} r={RING_RADIUS} fill="none" stroke="#f1f5f9" strokeWidth={RING_STROKE} />
        {hasRing && (
          <circle cx={RING_SIZE / 2} cy={RING_SIZE / 2} r={RING_RADIUS} fill="none" stroke={stroke} strokeWidth={RING_STROKE}
            strokeDasharray={RING_CIRCUMFERENCE} strokeDashoffset={dashOffset} strokeLinecap="round" />
        )}
        {!hasRing && !isEmpty && (
          <circle cx={RING_SIZE / 2} cy={RING_SIZE / 2} r={RING_RADIUS} fill="none" stroke={stroke} strokeWidth={RING_STROKE}
            strokeDasharray={RING_CIRCUMFERENCE} strokeDashoffset={0} strokeLinecap="round" opacity={isInfinite ? 0.4 : 0.2} />
        )}
      </svg>
      <div className={`absolute inset-0 flex items-center justify-center text-[7px] font-bold leading-none ${quotaTextColor(text, percent)}`}>
        {isEmpty ? '-' : (hasRing ? Math.round(percent) : (isInfinite ? '∞' : ''))}
      </div>
    </div>
  );
};

const GroupBoardCard = ({
  group = {},
  onSwapGroup,
  onSetActiveAccount,
  onPinAccountSelection,
  onViewInInventory,
  busyKey = ''
}) => {
  const [expanded, setExpanded] = useState(false);
  const [showAll, setShowAll] = useState(false);

  const groupKey = normalizeGroupKey(group?.key);
  const style = GROUP_STYLE[groupKey];
  const accounts = Array.isArray(group?.accounts) ? group.accounts : [];
  const actions = Array.isArray(group?.actions) ? group.actions : [];
  const availableCount = accounts.filter((a) => a.isAvailable).length;
  const manualAccount = accounts.find((a) => a?.isActive) || null;
  const preferredAccount = accounts.find((a) => a?.isGroupPreferred) || null;
  const activeAccount = manualAccount || preferredAccount || accounts[0] || null;
  const hasManualPinnedAccount = accounts.some((a) => a?.isActive);
  const hasGroupPreferredAccount = accounts.some((a) => a?.isGroupPreferred);
  const activeAccountName = activeAccount?.account_name || activeAccount?.name || '暂无';

  const visibleAccounts = showAll ? accounts : accounts.slice(0, DEFAULT_VISIBLE);
  const hiddenCount = accounts.length - visibleAccounts.length;
  const groupActionBusy = Boolean(busyKey) && busyKey.startsWith(`group-swap-${groupKey}-`);

  const handleGroupAction = (action) => {
    if (!action || action.disabled) {
      return;
    }
    onSwapGroup?.(group, action);
  };

  return (
    <article className={`rounded-xl border ${style.ringClassName} bg-white shadow-sm overflow-hidden`}>
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className={`w-full flex items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-slate-50/80 ${style.headerBg}`}
      >
        <div className="flex items-center gap-2 min-w-0 flex-1">
          {expanded ? <ChevronDown size={14} className="text-slate-400 shrink-0" /> : <ChevronRight size={14} className="text-slate-400 shrink-0" />}
          <Badge text={group?.label || style.label} className={style.badgeClassName} />
          <span className="text-xs text-slate-500">{availableCount}/{accounts.length} 可用</span>
          <span className="text-slate-300">·</span>
          <span className="text-xs text-slate-700 font-medium truncate">
            {hasManualPinnedAccount ? '全局手动' : hasGroupPreferredAccount ? '本组首选' : '顺序首位'}: {activeAccountName}
          </span>
        </div>
        <span className="text-xs text-slate-400 shrink-0">{group?.healthSummary || ''}</span>
      </button>

      {expanded && (
        <div className="border-t border-slate-100">
          {accounts.length === 0 ? (
            <div className="px-4 py-4 text-sm text-slate-500">该组暂无账号。</div>
          ) : (
            <>
              <table className="w-full text-xs">
                <thead>
                  <tr className="bg-slate-50/60 text-left text-[11px] uppercase tracking-wider text-slate-400">
                    <th className="px-4 py-2 font-medium">名称</th>
                    <th className="px-3 py-2 font-medium">状态</th>
                    <th className="px-3 py-2 font-medium">额度</th>
                    <th className="px-3 py-2 font-medium text-right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {visibleAccounts.map((account, index) => {
                    const accountId = toAccountId(account);
                    const isActive = account.isActive;
                    const toneCls = STATE_TONE_CLASS[account.stateTone] || STATE_TONE_CLASS.slate;

                    return (
                      <tr key={String(accountId ?? `${groupKey}-${index}`)} className="border-t border-slate-50 hover:bg-slate-50/50">
                        <td className="px-4 py-2 whitespace-nowrap">
                          <div className="flex items-center gap-1.5">
                            <span className="font-medium text-slate-900">{account.name || `候选 ${index + 1}`}</span>
                            {isActive && <Badge text="全局手动" className="bg-indigo-50 text-indigo-700 border-indigo-200" />}
                            {!isActive && account?.isGroupPreferred && <Badge text="本组首选" className="bg-emerald-50 text-emerald-700 border-emerald-200" />}
                          </div>
                        </td>
                        <td className="px-3 py-2 whitespace-nowrap">
                          <Badge text={account.stateLabel || '-'} className={toneCls} />
                        </td>
                        <td className="px-3 py-2 whitespace-nowrap">
                          <div className="flex items-center gap-1.5">
                            <MiniQuotaRing label="5h" text={account.quota5hText} percent={account.quota5hPercent} />
                            <MiniQuotaRing label="d7" text={account.quota7dText} percent={account.quota7dPercent} />
                          </div>
                        </td>
                        <td className="px-3 py-2 whitespace-nowrap text-right">
                          <div className="flex items-center justify-end gap-1">
                            {!isActive && !account?.isGroupPreferred && typeof onSetActiveAccount === 'function' && (
                              <button
                                type="button"
                                title="设为本组首选"
                                disabled={busyKey === `group-active-${groupKey}-${accountId}` || groupActionBusy}
                                onClick={() => onSetActiveAccount?.(toActionAccount(account), group)}
                                className="p-1 text-slate-400 hover:text-emerald-600 hover:bg-emerald-50 rounded transition-colors disabled:opacity-40"
                              >
                                <Target size={13} />
                              </button>
                            )}
                            {!isActive && typeof onPinAccountSelection === 'function' && (
                              <button
                                type="button"
                                title="设为全局手动"
                                disabled={Boolean(busyKey)}
                                onClick={() => onPinAccountSelection?.(toActionAccount(account))}
                                className="p-1 text-slate-400 hover:text-indigo-600 hover:bg-indigo-50 rounded transition-colors disabled:opacity-40"
                              >
                                <Pin size={13} />
                              </button>
                            )}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>

              {hiddenCount > 0 && (
                <div className="border-t border-slate-50 px-4 py-2 text-center">
                  <button
                    type="button"
                    onClick={() => setShowAll(true)}
                    className="text-xs font-medium text-indigo-600 hover:text-indigo-700 transition-colors"
                  >
                    展开全部 {accounts.length} 个账号
                  </button>
                </div>
              )}

              {showAll && accounts.length > DEFAULT_VISIBLE && (
                <div className="border-t border-slate-50 px-4 py-2 text-center">
                  <button
                    type="button"
                    onClick={() => setShowAll(false)}
                    className="text-xs font-medium text-slate-500 hover:text-slate-700 transition-colors"
                  >
                    收起，只显示前 {DEFAULT_VISIBLE} 个
                  </button>
                </div>
              )}
            </>
          )}

          {actions.length > 0 && activeAccount ? (
            <div className="border-t border-slate-100 px-4 py-3">
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <div className="text-xs font-medium text-slate-600">当前组动作</div>
                  <div className="mt-0.5 text-[11px] text-slate-400">与相邻组整组交换，不是单账号加入</div>
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  {actions
                    .filter((action) => action.key === 'swap-up' || action.key === 'swap-down')
                    .map((action) => {
                      const Icon = ACTION_ICON_BY_KEY[action.key];
                      const variant = ACTION_VARIANT_BY_TONE[action.tone] || 'secondary';

                      return (
                        <Button
                          key={action.key}
                          type="button"
                          size="sm"
                          variant={variant}
                          disabled={Boolean(action.disabled) || groupActionBusy}
                          onClick={() => handleGroupAction(action)}
                          className="whitespace-nowrap"
                        >
                          {Icon ? <Icon size={14} className="mr-1" /> : null}
                          {action.label}
                        </Button>
                      );
                    })}
                </div>
              </div>
            </div>
          ) : null}

          {onViewInInventory && accounts.length > 0 && (
            <div className="border-t border-slate-100 px-4 py-2">
              <button
                type="button"
                onClick={() => onViewInInventory?.(groupKey)}
                className="flex items-center gap-1 text-xs font-medium text-slate-500 hover:text-indigo-600 transition-colors"
              >
                <ExternalLink size={12} />
                去账号资产查看本组全部账号
              </button>
            </div>
          )}
        </div>
      )}
    </article>
  );
};

export default GroupBoardCard;
