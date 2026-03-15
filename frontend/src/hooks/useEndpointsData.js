// ============================================
// 端点数据管理 Hook
// 2025-11-28 (Updated 2025-12-04 for Wails support)
// ============================================

import { useState, useCallback, useEffect } from 'react';
import useSSE from './useSSE.js';
import { applyEndpointHealthCheckResult, calculateEndpointStats } from './useEndpointsData.helpers.js';
import {
  fetchEndpoints as apiFetchEndpoints,
  checkEndpointHealth as apiCheckEndpointHealth,
  checkAllEndpointsHealth as apiCheckAllEndpointsHealth,
  activateGroup as apiActivateGroup,
  updateEndpointPriority as apiUpdateEndpointPriority,
  fetchKeysOverview as apiFetchKeysOverview,
  switchKey as apiSwitchKey
} from '@utils/api.js';

/**
 * 端点数据管理 Hook
 *
 * 功能:
 * - 端点数据状态管理
 * - SSE 实时更新
 * - API 交互方法 (连通性测试、优先级更新、Key 切换等)
 */
const useEndpointsData = () => {
  const [data, setData] = useState({
    endpoints: [],
    total: 0,
    healthy: 0,
    unhealthy: 0,
    unchecked: 0,
    healthPercentage: 0,
    loading: false,
    error: null,
    lastUpdate: null
  });

  // Key 概览数据
  const [keysOverview, setKeysOverview] = useState(null);
  const [isInitialized, setIsInitialized] = useState(false);

  // 计算端点统计
  const calculateStats = calculateEndpointStats;

  // SSE 事件处理
  const handleSSEUpdate = useCallback((sseData, eventType) => {
    if (eventType !== 'endpoint') return;

    const actualData = sseData.data || sseData;

    try {
      setData(prevData => {
        const newData = { ...prevData };

        // 完整端点列表更新
        if (actualData.endpoints && Array.isArray(actualData.endpoints)) {
          newData.endpoints = actualData.endpoints;
          Object.assign(newData, calculateStats(actualData.endpoints));
        }
        // 单个端点更新
        else if (actualData.endpoint_name || actualData.name || actualData.endpoint) {
          const endpointName = actualData.endpoint_name || actualData.name || actualData.endpoint;
          newData.endpoints = newData.endpoints.map(ep =>
            ep.name === endpointName ? { ...ep, ...actualData } : ep
          );
          Object.assign(newData, calculateStats(newData.endpoints));
        }

        newData.lastUpdate = new Date().toLocaleTimeString();
        newData.error = null;
        return newData;
      });
    } catch (error) {
      console.error('❌ [端点SSE] 处理失败:', error);
    }
  }, [calculateStats]);

  // 初始化 SSE 连接
  const { connectionStatus } = useSSE(handleSSEUpdate, {
    events: 'endpoint'
  });

  // 加载端点数据
  const loadData = useCallback(async () => {
    try {
      if (!isInitialized) {
        setData(prev => ({ ...prev, loading: true, error: null }));
      }

      // 使用 API 适配层（自动检测 Wails 环境）
      const responseData = await apiFetchEndpoints();
      const endpoints = responseData.endpoints || [];
      const stats = calculateStats(endpoints);

      setData(prevData => ({
        ...prevData,
        endpoints,
        ...stats,
        lastUpdate: new Date().toLocaleTimeString(),
        loading: false,
        error: null
      }));

      setIsInitialized(true);
    } catch (error) {
      console.error('❌ 端点数据加载失败:', error);
      setData(prev => ({
        ...prev,
        loading: false,
        error: error.message || '端点数据加载失败'
      }));
    }
  }, [isInitialized, calculateStats]);

  // 加载 Key 概览数据
  const loadKeysOverview = useCallback(async () => {
    try {
      // 使用 API 适配层（自动检测 Wails 环境）
      const responseData = await apiFetchKeysOverview();
      setKeysOverview(responseData);
      return responseData;
    } catch (error) {
      console.error('❌ Key 概览加载失败:', error);
      return null;
    }
  }, []);

  // 更新端点优先级
  const updatePriority = useCallback(async (endpointName, newPriority) => {
    try {
      // 使用 API 适配层（自动检测 Wails 环境）
      const result = await apiUpdateEndpointPriority(endpointName, newPriority);

      // 本地更新状态
      setData(prevData => ({
        ...prevData,
        endpoints: prevData.endpoints.map(ep =>
          ep.name === endpointName ? { ...ep, priority: parseInt(newPriority) } : ep
        ),
        lastUpdate: new Date().toLocaleTimeString()
      }));

      setTimeout(() => loadData(), 500);
      return { success: true, message: result.message || `端点 ${endpointName} 优先级已更新为 ${newPriority}` };
    } catch (error) {
      console.error('❌ 优先级更新失败:', error);
      return { success: false, error: error.message };
    }
  }, [loadData]);

  // 执行连通性测试
  const performHealthCheck = useCallback(async (endpointName) => {
    try {
      if (!endpointName) throw new Error('端点名称不能为空');

      // 使用 API 适配层（自动检测 Wails 环境）
      const result = await apiCheckEndpointHealth(endpointName);
      const checkedAt = new Date().toISOString();

      // 本地更新状态
      setData(prevData => {
        const endpoints = applyEndpointHealthCheckResult(prevData.endpoints, endpointName, result, checkedAt);
        return {
          ...prevData,
          endpoints,
          ...calculateStats(endpoints),
          lastUpdate: new Date().toLocaleTimeString()
        };
      });

      setTimeout(() => loadData(), 500);
      return {
        success: true,
        healthy: result.healthy !== false,
        response_time: result.response_time,
        message: result.message || '连通性测试完成'
      };
    } catch (error) {
      console.error('❌ 连通性测试失败:', error);
      return { success: false, error: error.message };
    }
  }, [loadData, calculateStats]);

  // 批量连通性测试
  const performBatchHealthCheckAll = useCallback(async () => {
    try {
      // 使用 API 适配层（自动检测 Wails 环境）
      const result = await apiCheckAllEndpointsHealth();

      await loadData();
      return {
        success: true,
        message: result.message || '批量连通性测试完成',
        total: result.total,
        healthyCount: result.healthy_count,
        unhealthyCount: result.unhealthy_count
      };
    } catch (error) {
      console.error('❌ 批量连通性测试失败:', error);
      throw error;
    }
  }, [loadData]);

  // 激活端点（v4.0: 通过激活对应的组实现）
  const activateEndpointGroup = useCallback(async (endpointName, groupName) => {
    if (!groupName) {
      throw new Error('端点配置错误');
    }

    try {
      // 使用 API 适配层（自动检测 Wails 环境）
      const result = await apiActivateGroup(groupName);

      await loadData();
      return { success: true, message: result.message || '端点激活成功' };
    } catch (error) {
      console.error(`❌ 激活端点失败:`, error);
      throw error;
    }
  }, [loadData]);

  // 切换 Key
  const switchKey = useCallback(async (endpointName, keyType, index) => {
    try {
      // 使用 API 适配层（自动检测 Wails 环境）
      const result = await apiSwitchKey(endpointName, keyType, index);

      if (result.success) {
        await loadKeysOverview();
        return { success: true, message: result.message || 'Key 切换成功' };
      } else {
        throw new Error(result.error || 'Key 切换失败');
      }
    } catch (error) {
      console.error('❌ Key 切换失败:', error);
      throw error;
    }
  }, [loadKeysOverview]);

  // 搜索端点
  const searchEndpoints = useCallback((query) => {
    if (!query) return data.endpoints;
    const lowerQuery = query.toLowerCase();
    return data.endpoints.filter(ep =>
      ep.name.toLowerCase().includes(lowerQuery) ||
      ep.url.toLowerCase().includes(lowerQuery) ||
      (ep.group && ep.group.toLowerCase().includes(lowerQuery))
    );
  }, [data.endpoints]);

  // 按组分组
  const getEndpointsByGroup = useCallback(() => {
    const grouped = {};
    data.endpoints.forEach(ep => {
      const group = ep.group || 'default';
      if (!grouped[group]) grouped[group] = [];
      grouped[group].push(ep);
    });
    return grouped;
  }, [data.endpoints]);

  // 初始化
  useEffect(() => {
    loadData();
    loadKeysOverview();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // SSE 失败后定时刷新
  useEffect(() => {
    let interval = null;
    if (connectionStatus === 'failed' || connectionStatus === 'error') {
      interval = setInterval(loadData, 15000);
    }
    return () => {
      if (interval) clearInterval(interval);
    };
  }, [connectionStatus, loadData]);

  return {
    // 数据状态
    data,
    endpoints: data.endpoints,
    loading: data.loading,
    error: data.error,
    isInitialized,

    // 统计
    stats: {
      total: data.total,
      healthy: data.healthy,
      unhealthy: data.unhealthy,
      unchecked: data.unchecked,
      healthPercentage: data.healthPercentage
    },

    // 核心方法
    loadData,
    refresh: loadData,
    updatePriority,
    performHealthCheck,
    performBatchHealthCheckAll,
    activateEndpointGroup,

    // Key 管理
    keysOverview,
    loadKeysOverview,
    switchKey,

    // 查询方法
    searchEndpoints,
    getEndpointsByGroup,

    // 系统状态
    sseConnectionStatus: connectionStatus,
    lastUpdate: data.lastUpdate
  };
};

export default useEndpointsData;
