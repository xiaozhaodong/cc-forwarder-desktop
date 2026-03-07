// ============================================
// Account Pool 通知状态 Hook
// 2026-03-07
// ============================================

import { useCallback, useEffect, useState } from 'react';

const useNotice = () => {
  const [notice, setNotice] = useState(null);

  useEffect(() => {
    if (!notice) return undefined;
    const timer = setTimeout(() => setNotice(null), 4000);
    return () => clearTimeout(timer);
  }, [notice]);

  const showNotice = useCallback((type, text) => {
    setNotice({ type, text });
  }, []);

  const closeNotice = useCallback(() => {
    setNotice(null);
  }, []);

  return {
    notice,
    showNotice,
    closeNotice
  };
};

export default useNotice;
