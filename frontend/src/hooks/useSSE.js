// ============================================
// SSE 实时数据 Hook
// 2025-11-28 (Updated 2025-12-04 for Wails support)
// ============================================

import { useState, useRef, useCallback, useEffect } from 'react';
import { API_ENDPOINTS } from '@utils/constants.js';
import { isWailsEnvironment } from '@utils/wailsApi.js';
import useWailsEvents, { WAILS_STATUS } from './useWailsEvents.js';

// 生成或获取客户端ID
const getOrCreateClientId = () => {
  let clientId = localStorage.getItem('dashboard_client_id');
  if (!clientId) {
    clientId = 'client_' + Math.random().toString(36).substring(2, 11);
    localStorage.setItem('dashboard_client_id', clientId);
  }
  return clientId;
};

// SSE 连接状态
export const SSE_STATUS = {
  DISCONNECTED: 'disconnected',
  CONNECTING: 'connecting',
  CONNECTED: 'connected',
  RECONNECTING: 'reconnecting',
  ERROR: 'error',
  FAILED: 'failed'
};

/**
 * SSE 实时数据 Hook
 * 在 Wails 环境中自动使用 Wails Events 替代 SSE
 * @param {Function} onDataUpdate - 数据更新回调函数 (data, eventType) => void
 * @param {Object} options - 配置选项
 */
const useSSE = (onDataUpdate, options = {}) => {
  const isWails = isWailsEnvironment();

  // 两个 hook 都无条件调用以满足 hooks 规则；
  // Wails 环境下通过 enabled=false 阻止 SSE 自动建连
  const wailsEvents = useWailsEvents(onDataUpdate, options);
  const sse = useSSEInternal(onDataUpdate, options, !isWails);

  // 如果是 Wails 环境，直接返回 Wails Events 结果
  if (isWails) {
    return {
      connectionStatus: wailsEvents.connectionStatus === WAILS_STATUS.CONNECTED
        ? SSE_STATUS.CONNECTED
        : wailsEvents.connectionStatus === WAILS_STATUS.ERROR
          ? SSE_STATUS.ERROR
          : SSE_STATUS.CONNECTING,
      reconnectAttempts: 0,
      connect: wailsEvents.connect,
      disconnect: wailsEvents.disconnect,
      reconnect: wailsEvents.reconnect,
      isConnected: wailsEvents.isConnected
    };
  }

  // 非 Wails 环境使用原有的 SSE 实现
  return sse;
};

/**
 * 内部 SSE 实现（仅在非 Wails 环境启用自动连接）
 */
const useSSEInternal = (onDataUpdate, options = {}, enabled = true) => {
  const {
    events = 'status,endpoint,group,connection,log,chart',
    maxReconnectAttempts = 5,
    reconnectDelay = 3000
  } = options;

  const [connectionStatus, setConnectionStatus] = useState(SSE_STATUS.DISCONNECTED);
  const [reconnectAttempts, setReconnectAttempts] = useState(0);
  const connectionRef = useRef(null);
  const reconnectTimerRef = useRef(null);
  const onDataUpdateRef = useRef(onDataUpdate);
  const reconnectAttemptsRef = useRef(0);

  // 保持回调引用最新
  useEffect(() => {
    onDataUpdateRef.current = onDataUpdate;
  }, [onDataUpdate]);

  // 处理重连 - 使用 ref 版本避免循环依赖
  const handleReconnectRef = useRef(null);

  // 连接 SSE
  const connect = useCallback(() => {
    if (connectionRef.current) {
      return;
    }

    const clientId = getOrCreateClientId();

    try {
      console.log('🔄 [SSE] 建立连接...');
      setConnectionStatus(SSE_STATUS.CONNECTING);

      connectionRef.current = new EventSource(
        `${API_ENDPOINTS.STREAM}?client_id=${clientId}&events=${events}`
      );

      connectionRef.current.onopen = () => {
        console.log('📡 [SSE] 连接已建立');
        setConnectionStatus(SSE_STATUS.CONNECTED);
        setReconnectAttempts(0);
        reconnectAttemptsRef.current = 0;
      };

      connectionRef.current.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (onDataUpdateRef.current) {
            onDataUpdateRef.current(data);
          }
        } catch (error) {
          console.error('❌ [SSE] 解析消息失败:', error);
        }
      };

      // 监听特定事件类型
      events.split(',').forEach(eventType => {
        connectionRef.current.addEventListener(eventType, (event) => {
          try {
            const data = JSON.parse(event.data);
            if (onDataUpdateRef.current) {
              onDataUpdateRef.current(data, eventType);
            }
          } catch (error) {
            console.error(`❌ [SSE] 解析${eventType}事件失败:`, error);
          }
        });
      });

      connectionRef.current.onerror = () => {
        console.error('❌ [SSE] 连接错误');
        setConnectionStatus(SSE_STATUS.ERROR);
        if (handleReconnectRef.current) {
          handleReconnectRef.current();
        }
      };

    } catch (error) {
      console.error('❌ [SSE] 创建连接失败:', error);
      setConnectionStatus(SSE_STATUS.ERROR);
      if (handleReconnectRef.current) {
        handleReconnectRef.current();
      }
    }
  }, [events]);

  // 处理重连
  const handleReconnect = useCallback(() => {
    if (connectionRef.current) {
      connectionRef.current.close();
      connectionRef.current = null;
    }

    reconnectAttemptsRef.current += 1;
    const newAttempts = reconnectAttemptsRef.current;
    setReconnectAttempts(newAttempts);

    if (newAttempts <= maxReconnectAttempts) {
      console.log(`🔄 [SSE] 准备重连 (${newAttempts}/${maxReconnectAttempts})`);
      setConnectionStatus(SSE_STATUS.RECONNECTING);

      reconnectTimerRef.current = setTimeout(() => {
        connect();
      }, reconnectDelay);
    } else {
      console.error('❌ [SSE] 重连次数已达上限');
      setConnectionStatus(SSE_STATUS.FAILED);
    }
  }, [connect, maxReconnectAttempts, reconnectDelay]);

  // 更新 ref
  useEffect(() => {
    handleReconnectRef.current = handleReconnect;
  }, [handleReconnect]);

  // 断开连接
  const disconnect = useCallback(() => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }

    if (connectionRef.current) {
      connectionRef.current.close();
      connectionRef.current = null;
    }

    setConnectionStatus(SSE_STATUS.DISCONNECTED);
    console.log('📡 [SSE] 连接已断开');
  }, []);

  // 手动重连
  const reconnect = useCallback(() => {
    disconnect();
    setReconnectAttempts(0);
    reconnectAttemptsRef.current = 0;
    connect();
  }, [connect, disconnect]);

  // 组件挂载时自动连接 - 使用延迟初始化避免 effect 同步 setState
  useEffect(() => {
    if (!enabled) return undefined;

    // 使用 setTimeout 延迟执行，避免 React 18 严格模式下的问题
    const timer = setTimeout(() => {
      connect();
    }, 0);

    return () => {
      clearTimeout(timer);
      disconnect();
    };
  }, [enabled]); // eslint-disable-line react-hooks/exhaustive-deps

  return {
    connectionStatus,
    reconnectAttempts,
    connect,
    disconnect,
    reconnect,
    isConnected: connectionStatus === SSE_STATUS.CONNECTED
  };
};

export default useSSE;
