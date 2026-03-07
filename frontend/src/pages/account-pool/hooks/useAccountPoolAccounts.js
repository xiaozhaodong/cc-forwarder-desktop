// ============================================
// Account Pool 账号列表数据 Hook
// 2026-03-07
// ============================================

import { useCallback, useEffect, useState } from 'react';
import { fetchUpstreamAccounts } from '@utils/api.js';

const useAccountPoolAccounts = () => {
  const [accounts, setAccounts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const loadData = useCallback(async ({ silent = false } = {}) => {
    try {
      if (!silent) {
        setLoading(true);
      }
      setError('');

      const accountData = await fetchUpstreamAccounts();
      setAccounts(Array.isArray(accountData) ? accountData : []);
    } catch (err) {
      setError(err.message || '加载账号数据失败');
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  return {
    accounts,
    loading,
    error,
    loadData
  };
};

export default useAccountPoolAccounts;
