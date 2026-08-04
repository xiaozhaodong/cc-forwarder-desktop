import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  batchHealthCheckAll,
  getEndpointRecords,
  isWailsEnvironment,
  setEndpointAutoSchedule,
  setEndpointAvailability,
  subscribeToEvent,
  triggerHealthCheck
} from '@utils/wailsApi.js';
import { useTimezone } from '@contexts/TimezoneContext.jsx';

const computeStats = (endpoints = []) => {
  const checked = endpoints.filter((endpoint) => !endpoint.neverChecked && endpoint.lastCheck);
  const healthy = checked.filter((endpoint) => endpoint.healthy).length;
  const unhealthy = checked.length - healthy;
  return {
    total: endpoints.length,
    healthy,
    unhealthy,
    unchecked: endpoints.length - checked.length,
    cooldown: endpoints.filter((endpoint) => endpoint.inCooldown).length,
    healthPercentage: checked.length > 0 ? ((healthy / checked.length) * 100).toFixed(1) : '0.0'
  };
};

const useEndpointsData = () => {
  const { formatTimeOnly } = useTimezone();
  const [endpoints, setEndpoints] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [lastUpdate, setLastUpdate] = useState('');

  const loadData = useCallback(async () => {
    try {
      const records = await getEndpointRecords();
      setEndpoints(records);
      setError(null);
      setLastUpdate(formatTimeOnly(new Date()));
      return records;
    } catch (loadError) {
      setError(loadError?.message || 'Claude 端点加载失败');
      throw loadError;
    } finally {
      setLoading(false);
    }
  }, [formatTimeOnly]);

  useEffect(() => {
    loadData().catch(() => {});
  }, [loadData]);

  useEffect(() => {
    if (!isWailsEnvironment()) return undefined;
    const unsubscribe = subscribeToEvent('endpoint:update', () => {
      loadData().catch(() => {});
    });
    return () => unsubscribe?.();
  }, [loadData]);

  const runAndRefresh = useCallback(async (operation) => {
    const result = await operation();
    await loadData();
    return result;
  }, [loadData]);

  const sortedEndpoints = useMemo(() => [...endpoints].sort((left, right) => (
    (left.priority ?? 1) - (right.priority ?? 1)
    || String(left.name || '').localeCompare(String(right.name || ''), 'zh-Hans-CN')
  )), [endpoints]);

  return {
    endpoints: sortedEndpoints,
    loading,
    error,
    lastUpdate,
    stats: computeStats(endpoints),
    refresh: loadData,
    testEndpoint: (name) => runAndRefresh(() => triggerHealthCheck(name)),
    testAllEndpoints: () => runAndRefresh(() => batchHealthCheckAll()),
    setAvailability: (name, enabled) => runAndRefresh(() => setEndpointAvailability(name, enabled)),
    setAutoSchedule: (name, enabled) => runAndRefresh(() => setEndpointAutoSchedule(name, enabled))
  };
};

export default useEndpointsData;
