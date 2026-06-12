// ============================================
// 隐私保护页面数据 Hook
// 2026-06-11 (v6.1 新增)
// ============================================

import { useCallback, useEffect, useState } from 'react';
import {
  fetchPrivacySettings,
  updatePrivacySettings,
  fetchPrivacyRules,
  fetchPrivacyExactSecrets,
  createPrivacyRule,
  updatePrivacyRule,
  deletePrivacyRule,
  createPrivacyExactSecret,
  updatePrivacyExactSecret,
  deletePrivacyExactSecret,
  clearPrivacyExactSecrets,
  fetchPrivacySecretImportCandidates,
  importPrivacySecretCandidate,
  reorderPrivacyRules,
  testPrivacyRules,
  fetchPrivacyPresets,
  importPrivacyPreset,
  fetchPrivacyRuntimeStats
} from '@utils/api.js';

const usePrivacyProtection = () => {
  const [settings, setSettings] = useState(null);
  const [rules, setRules] = useState([]);
  const [exactSecrets, setExactSecrets] = useState([]);
  const [presets, setPresets] = useState([]);
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const reloadAll = useCallback(async () => {
    setError(null);
    try {
      const [nextSettings, nextRules, nextExactSecrets] = await Promise.all([
        fetchPrivacySettings(),
        fetchPrivacyRules(),
        fetchPrivacyExactSecrets()
      ]);
      setSettings(nextSettings);
      setRules(nextRules);
      setExactSecrets(nextExactSecrets);
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  const reloadStats = useCallback(async () => {
    try {
      setStats(await fetchPrivacyRuntimeStats());
    } catch {
      // 统计为可选信息，失败时静默忽略
      setStats(null);
    }
  }, []);

  useEffect(() => {
    reloadAll();
    reloadStats();
    fetchPrivacyPresets()
      .then(setPresets)
      .catch(() => setPresets([]));
  }, [reloadAll, reloadStats]);

  // 保存全局设置（模式立即保存；其余字段由调用方收集后提交）
  const saveSettings = useCallback(async (input) => {
    const updated = await updatePrivacySettings(input);
    setSettings(updated);
    return updated;
  }, []);

  const saveRule = useCallback(async (form, payload) => {
    const saved = form.id > 0
      ? await updatePrivacyRule(form.id, payload)
      : await createPrivacyRule(payload);
    await reloadAll();
    return saved;
  }, [reloadAll]);

  const removeRule = useCallback(async (id) => {
    await deletePrivacyRule(id);
    await reloadAll();
  }, [reloadAll]);

  const saveExactSecret = useCallback(async (form) => {
    const saved = form.id > 0
      ? await updatePrivacyExactSecret(form.id, form)
      : await createPrivacyExactSecret(form);
    await reloadAll();
    return saved;
  }, [reloadAll]);

  const removeExactSecret = useCallback(async (id) => {
    await deletePrivacyExactSecret(id);
    await reloadAll();
  }, [reloadAll]);

  const clearExactSecrets = useCallback(async (confirmText) => {
    await clearPrivacyExactSecrets(confirmText);
    await reloadAll();
  }, [reloadAll]);

  const loadImportCandidates = useCallback(async () => fetchPrivacySecretImportCandidates(), []);

  const importSecretCandidate = useCallback(async (input) => {
    const imported = await importPrivacySecretCandidate(input);
    await reloadAll();
    return imported;
  }, [reloadAll]);

  // 启用开关：toggle 后立即保存并热生效
  const toggleRule = useCallback(async (rule, enabled) => {
    await updatePrivacyRule(rule.id, { ...rule, enabled });
    await reloadAll();
  }, [reloadAll]);

  const reorderRules = useCallback(async (orders) => {
    await reorderPrivacyRules(orders);
    await reloadAll();
  }, [reloadAll]);

  const importPreset = useCallback(async (presetId) => {
    const created = await importPrivacyPreset(presetId);
    await reloadAll();
    return created;
  }, [reloadAll]);

  // 测试文本只在调用栈中传递，不进入任何持久化状态
  const runTest = useCallback(async (input) => testPrivacyRules(input), []);

  return {
    settings,
    rules,
    exactSecrets,
    presets,
    stats,
    loading,
    error,
    reloadAll,
    reloadStats,
    saveSettings,
    saveRule,
    removeRule,
    saveExactSecret,
    removeExactSecret,
    clearExactSecrets,
    loadImportCandidates,
    importSecretCandidate,
    toggleRule,
    reorderRules,
    importPreset,
    runTest
  };
};

export default usePrivacyProtection;
