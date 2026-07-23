// ============================================
// 主 App 组件 - Dashboard 入口
// 2025-11-28 (Updated 2025-12-12)
// ============================================

import { useState, Suspense, lazy, useCallback } from 'react';
import Header from '@components/layout/Header.jsx';
import ToastHost from '@components/notifications/ToastHost.jsx';
import { LoadingSpinner } from '@components/ui';
import useSSE from '@hooks/useSSE.js';
import useGlobalToasts from '@hooks/useGlobalToasts.js';

// 懒加载页面组件
const OverviewPage = lazy(() => import('@pages/overview/index.jsx'));
const EndpointsPage = lazy(() => import('@pages/endpoints/index.jsx'));
// v4.0: 组管理页面已移除，配置简化后不再需要独立的组管理功能
// const GroupsPage = lazy(() => import('@pages/groups/index.jsx'));
const RequestsPage = lazy(() => import('@pages/requests/index.jsx'));
const PricingPage = lazy(() => import('@pages/pricing/index.jsx'));
// v5.1: 配置页面改为设置页面（可编辑）
const SettingsPage = lazy(() => import('@pages/settings/index.jsx'));
// 保留只读配置页面用于调试
const ConfigPage = lazy(() => import('@pages/config/index.jsx'));
// v5.1: 系统日志页面
const LogsPage = lazy(() => import('@pages/log-viewer/index.jsx'));
// v6.0: 账号池页面
const AccountPoolPage = lazy(() => import('@pages/account-pool/index.jsx'));
// v6.1: 隐私保护页面
const PrivacyProtectionPage = lazy(() => import('@pages/privacy-protection/index.jsx'));

// ============================================
// App 组件
// ============================================

function App() {
  const [activeTab, setActiveTab] = useState('overview');
  const { toasts, pendingCount, showToast, dismissToast } = useGlobalToasts();
  // v5.1+: 代理网关真实状态（来自后端 system:status 事件）
  const [proxyStatus, setProxyStatus] = useState({
    running: false,
    port: 0,
    host: '127.0.0.1'
  });

  // 处理 system:status 事件
  const handleStatusUpdate = useCallback((data) => {
    const payload = data?.data && typeof data.data === 'object' ? data.data : data;
    if (payload?.proxy_running !== undefined) {
      setProxyStatus({
        running: payload.proxy_running,
        port: payload.proxy_port || 0,
        host: payload.proxy_host || '127.0.0.1'
      });
    }
  }, []);

  const handleRealtimeEvent = useCallback((data, eventType) => {
    if (eventType === 'notification' || data?.event === 'notification') {
      showToast(data);
      return;
    }
    // 兼容命名 SSE 事件与默认 message 事件；非状态数据会被 handleStatusUpdate 忽略。
    handleStatusUpdate(data);
  }, [handleStatusUpdate, showToast]);

  // SSE 连接状态（用于全局状态指示）
  const { connectionStatus } = useSSE(handleRealtimeEvent, { events: 'status,notification' });

  // 渲染当前页面
  const renderPage = () => {
    const pages = {
      overview: <OverviewPage />,
      endpoints: <EndpointsPage />,
      // v4.0: 组管理页面已移除
      // groups: <GroupsPage />,
      requests: <RequestsPage />,
      // v6.1: 隐私保护页面
      'privacy-protection': <PrivacyProtectionPage />,
      pricing: <PricingPage />,
      'account-pool': <AccountPoolPage />,
      // v5.1: 设置页面（可编辑）
      settings: <SettingsPage />,
      // 保留只读配置页面
      config: <ConfigPage />,
      // v5.1: 系统日志页面
      logs: <LogsPage />
    };

    return pages[activeTab] || <OverviewPage />;
  };

  return (
    <div className="h-screen overflow-hidden bg-[#FAFAFA] font-sans text-slate-900 flex flex-col">
      {/* 背景纹理 */}
      <div
        className="fixed inset-0 pointer-events-none opacity-[0.4]"
        style={{
          backgroundImage: 'radial-gradient(#cbd5e1 1px, transparent 1px)',
          backgroundSize: '24px 24px'
        }}
      />

      {/* 顶部导航 */}
      <Header
        activeTab={activeTab}
        onTabChange={setActiveTab}
        connectionStatus={connectionStatus}
        proxyStatus={proxyStatus}
      />

      {/* 应用窗口内全局 Toast；不会调用 macOS 通知中心。 */}
      <ToastHost toasts={toasts} pendingCount={pendingCount} onDismiss={dismissToast} />

      {/* 主内容区 */}
      <main id="app-scroll-container" className="flex-1 min-h-0 overflow-y-auto overscroll-none relative z-10">
        <div className="max-w-7xl mx-auto px-6 pt-8 pb-20">
          <Suspense fallback={<LoadingSpinner text="加载页面..." />}>
            {renderPage()}
          </Suspense>
        </div>
      </main>
    </div>
  );
}

export default App;
