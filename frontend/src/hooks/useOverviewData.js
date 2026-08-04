import { useCallback, useEffect, useState } from 'react';
import { fetchConnections, fetchEndpoints, fetchStatus, formatUptime } from '@utils/api.js';
import useSSE from './useSSE.js';
import { useTimezone } from '@contexts/TimezoneContext.jsx';

const useOverviewData = () => {
  const { formatTimeOnly } = useTimezone();
  const [data, setData] = useState({
    status: { status: 'running', uptime: '加载中...' },
    endpoints: { total: 0, healthy: 0, endpoints: [] },
    connections: {
      total_requests: 0,
      all_time_total_requests: 0,
      today_requests: 0,
      today_cost: 0,
      all_time_total_cost: 0,
      today_tokens: 0,
      all_time_total_tokens: 0
    },
    lastUpdate: null,
    loading: true,
    error: null
  });
  const [isInitialized, setIsInitialized] = useState(false);

  const loadData = useCallback(async () => {
    try {
      if (!isInitialized) setData((current) => ({ ...current, loading: true, error: null }));
      const [status, endpoints, connections] = await Promise.all([
        fetchStatus(),
        fetchEndpoints(),
        fetchConnections()
      ]);
      const formattedStatus = { ...status };
      if (formattedStatus.uptime !== undefined) formattedStatus.uptime = formatUptime(formattedStatus.uptime);
      setData((current) => ({
        status: { ...current.status, ...formattedStatus },
        endpoints: { ...current.endpoints, ...endpoints },
        connections: { ...current.connections, ...connections },
        lastUpdate: formatTimeOnly(new Date()),
        loading: false,
        error: null
      }));
      setIsInitialized(true);
    } catch (error) {
      setData((current) => ({ ...current, loading: false, error: error?.message || '数据加载失败' }));
    }
  }, [formatTimeOnly, isInitialized]);

  const handleRealtimeUpdate = useCallback((event, eventType) => {
    const payload = event?.data && typeof event.data === 'object' ? event.data : event;
    if (eventType === 'chart') {
      document.dispatchEvent(new CustomEvent('chartUpdate', {
        detail: { chart_type: event.chart_type || payload?.chart_type, data: event.data || payload }
      }));
      return;
    }
    if (eventType === 'endpoint' || eventType === 'connection' || eventType === 'status') {
      loadData();
    }
  }, [loadData]);

  const { connectionStatus, isConnected } = useSSE(handleRealtimeUpdate, { events: 'status,endpoint,connection,chart' });

  useEffect(() => {
    loadData();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (connectionStatus !== 'failed' && connectionStatus !== 'error') return undefined;
    const timer = window.setInterval(loadData, 10000);
    return () => window.clearInterval(timer);
  }, [connectionStatus, loadData]);

  return { data, loadData, refresh: loadData, isInitialized, connectionStatus, isConnected };
};

export default useOverviewData;
