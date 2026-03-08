// ============================================
// Account Pool 手动主备纯函数
// 2026-03-07
// ============================================

import { DEFAULT_BASE_URL, MANUAL_FAILOVER_TIER_PRESETS } from './constants.js';
import { resolveAccountId } from './accountPool.js';

const normalizePriorityValue = (value) => {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : null;
};

const buildManualFailoverTierSummary = (accounts = []) => {
  const counts = new Map();

  accounts.forEach((account) => {
    const priority = normalizePriorityValue(account?.priority ?? account?.Priority);
    if (!Number.isFinite(priority)) {
      return;
    }
    counts.set(priority, (counts.get(priority) || 0) + 1);
  });

  return Array.from(counts.entries())
    .sort((left, right) => left[0] - right[0])
    .map(([priority, count], index) => {
      const preset = MANUAL_FAILOVER_TIER_PRESETS[index];
      return {
        priority,
        count,
        order: index + 1,
        label: preset?.label || `第 ${index + 1} 层`,
        className: preset?.className || 'bg-slate-100 text-slate-700 border-slate-200',
        description: preset?.description || '更低优先级的手动兜底层'
      };
    });
};

const compareAccountsByManualPriority = (left = {}, right = {}) => {
  const leftPriority = normalizePriorityValue(left?.priority ?? left?.Priority);
  const rightPriority = normalizePriorityValue(right?.priority ?? right?.Priority);

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

const buildManualFailoverTierGroups = (accounts = []) => {
  const sorted = [...accounts].sort(compareAccountsByManualPriority);
  const groups = [];

  sorted.forEach((account) => {
    const priority = normalizePriorityValue(account?.priority ?? account?.Priority);
    const lastGroup = groups[groups.length - 1];

    if (lastGroup && lastGroup.priority === priority) {
      lastGroup.accounts.push(account);
      return;
    }

    groups.push({
      priority,
      accounts: [account]
    });
  });

  return groups;
};

const buildManualFailoverPriorityPlan = ({ accounts = [], targetAccountId, targetTierIndex }) => {
  const tiers = buildManualFailoverTierGroups(accounts);
  if (!tiers.length || targetAccountId === null || targetAccountId === undefined) {
    return [];
  }

  const remainingTiers = [];
  let targetAccount = null;

  tiers.forEach((tier) => {
    const nextAccounts = [];
    tier.accounts.forEach((account) => {
      const accountId = resolveAccountId(account);
      if (targetAccount === null && accountId === targetAccountId) {
        targetAccount = account;
        return;
      }
      nextAccounts.push(account);
    });
    if (nextAccounts.length > 0) {
      remainingTiers.push({ priority: tier.priority, accounts: nextAccounts });
    }
  });

  if (!targetAccount) {
    return [];
  }

  const insertIndex = Math.max(0, Math.min(targetTierIndex, remainingTiers.length));
  remainingTiers.splice(insertIndex, 0, { priority: null, accounts: [targetAccount] });

  return remainingTiers.flatMap((tier, index) => {
    const nextPriority = (index + 1) * 10;
    return tier.accounts
      .filter((account) => normalizePriorityValue(account?.priority ?? account?.Priority) !== nextPriority)
      .map((account) => ({ account, priority: nextPriority }));
  });
};

const buildAccountUpdatePayload = (account, priority) => ({
  provider_type: String(account?.provider_type ?? account?.providerType ?? '').trim(),
  account_name: String(account?.account_name ?? account?.accountName ?? '').trim(),
  credential_raw: String(account?.credential_raw ?? account?.credentialRaw ?? '').trim(),
  base_url: String(account?.base_url ?? account?.baseURL ?? DEFAULT_BASE_URL).trim() || DEFAULT_BASE_URL,
  costMultiplier: String(account?.cost_multiplier ?? account?.costMultiplier ?? 1.0),
  inputCostMultiplier: String(account?.input_cost_multiplier ?? account?.inputCostMultiplier ?? 1.0),
  outputCostMultiplier: String(account?.output_cost_multiplier ?? account?.outputCostMultiplier ?? 1.0),
  cacheCreationCostMultiplier: String(account?.cache_creation_cost_multiplier ?? account?.cacheCreationCostMultiplier ?? 1.0),
  cacheCreationCostMultiplier1h: String(account?.cache_creation_cost_multiplier_1h ?? account?.cacheCreationCostMultiplier1h ?? 1.0),
  cacheReadCostMultiplier: String(account?.cache_read_cost_multiplier ?? account?.cacheReadCostMultiplier ?? 1.0),
  priority,
  enabled: account?.enabled !== false
});

export {
  buildAccountUpdatePayload,
  buildManualFailoverPriorityPlan,
  buildManualFailoverTierSummary,
  normalizePriorityValue
};
