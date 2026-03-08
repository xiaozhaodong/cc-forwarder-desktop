// ============================================
// Account Pool 手动主备切换动作
// 2026-03-08
// ============================================

import { updateUpstreamAccount } from '@utils/api.js';
import { resolveAccountId } from './accountPool.js';
import { buildAccountUpdatePayload, buildManualFailoverPriorityPlan } from './manualFailover.js';

const isSameEntityId = (left, right) => String(left ?? '') === String(right ?? '');

const switchUpstreamAccountToTier = async ({ accounts = [], targetAccountId, targetTierIndex = 0 }) => {
  const targetAccount = accounts.find((account) => {
    const accountId = resolveAccountId(account);
    return accountId !== null && accountId !== undefined && isSameEntityId(accountId, targetAccountId);
  });

  if (!targetAccount) {
    throw new Error('目标账号不存在，无法切换');
  }

  const resolvedTargetAccountId = resolveAccountId(targetAccount);
  const changes = buildManualFailoverPriorityPlan({
    accounts,
    targetAccountId: resolvedTargetAccountId,
    targetTierIndex
  });

  if (changes.length === 0) {
    return {
      changed: false,
      changes: [],
      targetAccount,
      targetAccountId: resolvedTargetAccountId
    };
  }

  for (const change of changes) {
    const changeId = resolveAccountId(change.account);
    if (changeId === undefined || changeId === null || changeId === '') {
      throw new Error('存在缺少 ID 的账号，无法更新顺序');
    }

    await updateUpstreamAccount(changeId, buildAccountUpdatePayload(change.account, change.priority));
  }

  return {
    changed: true,
    changes,
    targetAccount,
    targetAccountId: resolvedTargetAccountId
  };
};

export {
  switchUpstreamAccountToTier
};
