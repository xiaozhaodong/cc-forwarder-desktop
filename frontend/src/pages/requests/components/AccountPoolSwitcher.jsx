// ============================================
// AccountPoolSwitcher - Codex 账号池快捷切换器
// 2026-03-08
// ============================================

import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, ArrowLeftRight, Check, Search, UserRound } from 'lucide-react';
import { canPinAccountSelection } from '../utils/accountSwitcherState.js';
import {
  normalizePriorityValue,
  resolveAccountId,
  toAccountAuthLabel,
  toPlanTypeLabel,
  toQuotaStatusLabel
} from '@pages/account-pool/utils.js';

const getAccountName = (account = {}) => account.account_name || account.accountName || '-';

const isSameAccountId = (left, right) => String(left ?? '') === String(right ?? '');

const isSwitchableAccount = (account = {}) => canPinAccountSelection(account);

const normalizeGroupKey = (value = '') => {
  const normalized = String(value || '').trim().toLowerCase();
  if (normalized === 'primary' || normalized === '主组') return 'primary';
  if (normalized === 'backup' || normalized === '备组') return 'backup';
  if (normalized === 'cold' || normalized === '冷备') return 'cold';
  return '';
};

const inferGroupKey = (account = {}) => {
  const explicitGroupKey = normalizeGroupKey(account.group_key || account.groupKey);
  if (explicitGroupKey) {
    return explicitGroupKey;
  }

  const priority = normalizePriorityValue(account.priority ?? account.Priority);
  if (!Number.isFinite(priority)) {
    return 'primary';
  }
  if (priority <= 10) {
    return 'primary';
  }
  if (priority <= 20) {
    return 'backup';
  }
  return 'cold';
};

const GROUP_META = {
  primary: {
    label: '主组',
    badgeClass: 'bg-indigo-50 text-indigo-600',
    rank: 0
  },
  backup: {
    label: '备组',
    badgeClass: 'bg-cyan-50 text-cyan-600',
    rank: 1
  },
  cold: {
    label: '冷备',
    badgeClass: 'bg-violet-50 text-violet-600',
    rank: 2
  }
};

const getGroupMeta = (account = {}) => {
  const groupKey = inferGroupKey(account);
  return {
    key: groupKey,
    ...(GROUP_META[groupKey] || GROUP_META.primary)
  };
};

const compareAccountsByGroupOrder = (left = {}, right = {}) => {
  const leftGroup = getGroupMeta(left);
  const rightGroup = getGroupMeta(right);

  if (leftGroup.rank !== rightGroup.rank) {
    return leftGroup.rank - rightGroup.rank;
  }

  const leftPriority = normalizePriorityValue(left.priority ?? left.Priority);
  const rightPriority = normalizePriorityValue(right.priority ?? right.Priority);
  if (Number.isFinite(leftPriority) && Number.isFinite(rightPriority) && leftPriority !== rightPriority) {
    return leftPriority - rightPriority;
  }
  if (Number.isFinite(leftPriority) && !Number.isFinite(rightPriority)) {
    return -1;
  }
  if (!Number.isFinite(leftPriority) && Number.isFinite(rightPriority)) {
    return 1;
  }

  const leftId = resolveAccountId(left);
  const rightId = resolveAccountId(right);
  if (typeof leftId === 'number' && typeof rightId === 'number' && leftId !== rightId) {
    return leftId - rightId;
  }
  return String(leftId ?? '').localeCompare(String(rightId ?? ''));
};

const getIndicatorClass = (account = {}) => {
  const state = String(account.state || '').trim().toLowerCase();
  if (account.enabled === false || state === 'disabled_auth') return 'bg-rose-400';
  if (state === 'cooldown') return 'bg-amber-400';
  return 'bg-emerald-400';
};

const AccountPoolSwitcher = ({
  accounts = [],
  activeAccount = null,
  recentSelectedAccountId = null,
  onSwitch,
  onSwitchAuto,
  loading = false
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const containerRef = useRef(null);

  const closePanel = () => {
    setIsOpen(false);
    setSearchTerm('');
  };

  const sortedAccounts = useMemo(() => {
    return [...accounts].sort(compareAccountsByGroupOrder);
  }, [accounts]);

  const activeAccountId = resolveAccountId(activeAccount);
  const isAutoMode = activeAccountId === null || activeAccountId === undefined;

  const filteredAccounts = useMemo(() => {
    const keyword = searchTerm.trim().toLowerCase();
    if (!keyword) {
      return sortedAccounts;
    }

    return sortedAccounts.filter((account) => {
      const accountName = getAccountName(account).toLowerCase();
      const authLabel = toAccountAuthLabel(account.provider_type || account.providerType || '').toLowerCase();
      const planLabel = toPlanTypeLabel(account.plan_type || account.planType || '').toLowerCase();
      const groupLabel = getGroupMeta(account).label.toLowerCase();
      return accountName.includes(keyword) || authLabel.includes(keyword) || planLabel.includes(keyword) || groupLabel.includes(keyword);
    });
  }, [searchTerm, sortedAccounts]);

  useEffect(() => {
    const handleClickOutside = (event) => {
      if (containerRef.current && !containerRef.current.contains(event.target)) {
        closePanel();
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  useEffect(() => {
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        closePanel();
      }
    };

    if (isOpen) {
      window.addEventListener('keydown', handleKeyDown);
    }

    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen]);

  const handleAccountSelect = async (account) => {
    const accountId = resolveAccountId(account);
    if (loading || accountId === null || accountId === undefined) {
      return;
    }

    if (isSameAccountId(accountId, activeAccountId)) {
      closePanel();
      return;
    }

    if (!isSwitchableAccount(account)) {
      return;
    }

    try {
      await onSwitch?.(account);
      closePanel();
    } catch (error) {
      console.error('切换账号失败:', error);
    }
  };

  const handleAutoSelect = async () => {
    if (loading || isAutoMode) {
      closePanel();
      return;
    }

    try {
      await onSwitchAuto?.();
      closePanel();
    } catch (error) {
      console.error('启用编排失败:', error);
    }
  };

  if (!sortedAccounts.length) {
    return (
      <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm text-gray-400 bg-gray-50 border border-gray-200">
        <AlertCircle className="w-3.5 h-3.5" />
        <span>无可用 Codex 账号</span>
      </div>
    );
  }

  const currentAccount = activeAccount;
  const activeIndicatorPingClass = isAutoMode ? 'bg-sky-400' : getIndicatorClass(currentAccount);
  const activeIndicatorDotClass = isAutoMode ? 'bg-sky-500' : getIndicatorClass(currentAccount);
  const activeGroupMeta = isAutoMode ? null : getGroupMeta(currentAccount);
  const currentTitle = isAutoMode ? '当前按编排自动调度' : '切换即固定该账号，直到其严格不可用';
  const currentLabel = isAutoMode ? 'Auto' : getAccountName(currentAccount);

  return (
    <div className="relative" ref={containerRef}>
      <button
        onClick={() => {
          if (isOpen) {
            closePanel();
            return;
          }
          setSearchTerm('');
          setIsOpen(true);
        }}
        disabled={loading}
        className={`group flex items-center justify-between gap-2 w-[170px] lg:w-[190px] xl:w-[220px] px-3 py-1.5 bg-white border rounded-lg text-sm font-medium transition-all shadow-sm ${
          isOpen
            ? 'border-indigo-300 ring-2 ring-indigo-100 text-indigo-700'
            : 'border-gray-200 text-gray-700 hover:border-indigo-300 hover:text-indigo-600'
        } ${loading ? 'opacity-60 cursor-wait' : ''}`}
        title={currentTitle}
      >
        <span className="relative flex h-2 w-2">
          <span className={`animate-ping absolute inline-flex h-full w-full rounded-full ${activeIndicatorPingClass} opacity-75`}></span>
          <span className={`relative inline-flex rounded-full h-2 w-2 ${activeIndicatorDotClass}`}></span>
        </span>

        <div className="flex items-center gap-1.5 min-w-0 flex-1">
          <UserRound className="w-3.5 h-3.5 text-gray-400 shrink-0" />
          <span className="hidden xl:inline font-semibold text-xs text-gray-500 shrink-0">Codex:</span>
          <span className="font-bold truncate">{currentLabel}</span>
          {activeGroupMeta?.label && (
            <span className={`hidden xl:inline text-[10px] px-1.5 py-0.5 rounded shrink-0 ${activeGroupMeta.badgeClass}`}>
              {activeGroupMeta.label}
            </span>
          )}
        </div>

        <ArrowLeftRight className={`w-3.5 h-3.5 ml-1 text-gray-400 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
      </button>

      {isOpen && (
        <div className="absolute top-full left-0 mt-2 w-[360px] bg-white rounded-xl shadow-xl border border-gray-100 ring-1 ring-black/5 z-50 overflow-hidden animate-in fade-in slide-in-from-top-2 duration-200">
          <div className="px-4 py-3 bg-gray-50 border-b border-gray-100">
            <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Codex 模式</div>
            <div className="text-[10px] text-gray-400 mt-0.5">可固定具体账号，也可切回 Auto 按编排调度</div>
          </div>

          <div className="px-3 py-2 border-b border-gray-100">
            <div className="flex items-center gap-2 px-3 py-2 rounded-lg border border-gray-200 bg-white focus-within:border-indigo-300 focus-within:ring-2 focus-within:ring-indigo-100 transition-all">
              <Search className="w-3.5 h-3.5 text-gray-400" />
              <input
                type="text"
                value={searchTerm}
                onChange={(event) => setSearchTerm(event.target.value)}
                placeholder="搜索账号名称 / 认证方式 / 套餐"
                className="w-full bg-transparent text-sm text-gray-700 placeholder:text-gray-400 outline-none"
              />
            </div>
          </div>

          <div className="p-2 max-h-[360px] overflow-y-auto">
            <button
              type="button"
              onClick={handleAutoSelect}
              disabled={loading}
              className={`mb-2 w-full flex items-start justify-between gap-3 px-3 py-2.5 rounded-lg text-left transition-colors ${
                isAutoMode
                  ? 'bg-indigo-50 text-indigo-700'
                  : 'hover:bg-gray-50 text-gray-700'
              } ${loading ? 'opacity-60 cursor-not-allowed' : ''}`}
            >
              <div className="flex items-start gap-3 min-w-0 flex-1">
                <span className={`w-2 h-2 rounded-full mt-1.5 shrink-0 ${isAutoMode ? 'bg-sky-500' : 'bg-sky-300'}`} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className={`font-medium truncate ${isAutoMode ? 'text-indigo-700' : 'text-gray-800'}`}>
                      Auto / 按编排
                    </span>
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 text-slate-500">全局模式</span>
                  </div>
                  <div className="mt-1 text-[10px] text-gray-500">
                    清除固定账号，恢复按当前编排规则自动选择
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2 shrink-0 pt-0.5">
                {isAutoMode && <Check className="w-4 h-4 text-indigo-600" />}
              </div>
            </button>

            {filteredAccounts.length === 0 ? (
              <div className="px-3 py-8 text-center text-sm text-gray-400">没有匹配的账号</div>
            ) : (
              filteredAccounts.map((account) => {
                const accountId = resolveAccountId(account);
                const isActive = isSameAccountId(accountId, activeAccountId);
                const isRecentSelected = recentSelectedAccountId !== null && recentSelectedAccountId !== undefined
                  && isSameAccountId(accountId, recentSelectedAccountId);
                const switchable = isSwitchableAccount(account);
                const priority = normalizePriorityValue(account.priority ?? account.Priority);
                const groupMeta = getGroupMeta(account);
                const accountName = getAccountName(account);
                const authLabel = toAccountAuthLabel(account.provider_type || account.providerType || '');
                const planLabel = toPlanTypeLabel(account.plan_type || account.planType || '');
                const quotaLabel = toQuotaStatusLabel(account.quota_status || account.quotaStatus || '');
                const indicatorClass = getIndicatorClass(account);

                return (
                  <button
                    key={String(accountId ?? accountName)}
                    onClick={() => handleAccountSelect(account)}
                    disabled={loading || !switchable}
                    className={`w-full flex items-start justify-between gap-3 px-3 py-2.5 rounded-lg text-left transition-colors ${
                      isActive
                        ? 'bg-indigo-50 text-indigo-700'
                        : 'hover:bg-gray-50 text-gray-700'
                    } ${(!switchable || loading) ? 'opacity-60 cursor-not-allowed' : ''}`}
                  >
                    <div className="flex items-start gap-3 min-w-0 flex-1">
                      <span className={`w-2 h-2 rounded-full mt-1.5 shrink-0 ${indicatorClass}`} />

                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className={`font-medium truncate ${isActive ? 'text-indigo-700' : 'text-gray-800'}`} title={accountName}>
                            {accountName}
                          </span>
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 text-slate-500">{authLabel}</span>
                          {planLabel && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-violet-50 text-violet-600">{planLabel}</span>
                          )}
                        </div>

                        <div className="mt-1 flex items-center gap-2 flex-wrap text-[10px] text-gray-500">
                          {groupMeta?.label && (
                            <span className={`px-1.5 py-0.5 rounded ${groupMeta.badgeClass}`}>{groupMeta.label}</span>
                          )}
                          {Number.isFinite(priority) && <span>组内顺序 {priority}</span>}
                          <span>{quotaLabel}</span>
                          {account.enabled === false && <span className="text-slate-400">已停用</span>}
                          {String(account.state || '').trim().toLowerCase() === 'disabled_auth' && <span className="text-rose-500">鉴权失效</span>}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 shrink-0 pt-0.5">
                      {isRecentSelected && !isActive && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-50 text-emerald-600">最近命中</span>
                      )}
                      {!switchable && !isActive && (
                        <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 text-slate-500">不可切</span>
                      )}
                      {isActive && <Check className="w-4 h-4 text-indigo-600" />}
                    </div>
                  </button>
                );
              })
            )}
          </div>
        </div>
      )}
    </div>
  );
};

export default AccountPoolSwitcher;
