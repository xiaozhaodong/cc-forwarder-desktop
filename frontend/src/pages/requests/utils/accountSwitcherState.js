import { isAccountSchedulable, resolveAccountId } from '../../account-pool/utils/accountPool.js';

const isRuntimeActiveSelection = (account = {}) => {
  return (account?.is_active_selection ?? account?.isActiveSelection) === true;
};

const resolveDisplayedActiveAccount = ({
  accounts = [],
  recentSelectedAccountId = null
} = {}) => {
  const list = Array.isArray(accounts) ? accounts : [];

  const runtimeActiveAccount = list.find(isRuntimeActiveSelection);
  if (runtimeActiveAccount) {
    return runtimeActiveAccount;
  }

  if (recentSelectedAccountId !== null && recentSelectedAccountId !== undefined) {
    const recentSelectedAccount = list.find((account) => {
      return String(resolveAccountId(account) ?? '') === String(recentSelectedAccountId);
    });
    if (recentSelectedAccount) {
      return recentSelectedAccount;
    }
  }

  return list.find(isAccountSchedulable) || list[0] || null;
};

export {
  isRuntimeActiveSelection,
  resolveDisplayedActiveAccount
};
