// ============================================
// Overview 页面 - 监控面板
// 2025-11-28
// ============================================

import { LoadingSpinner, ErrorMessage } from '@components/ui';
import useOverviewData from '@hooks/useOverviewData.js';

// 子组件
import KPICardsGrid from './components/KPICardsGrid.jsx';
import ConnectionStats from './components/ConnectionStats.jsx';
import RequestTrendChart from './components/RequestTrendChart.jsx';
import TokenDistributionChart from './components/TokenDistributionChart.jsx';
import EndpointHealthChart from './components/EndpointHealthChart.jsx';
import TokenCostChart from './components/TokenCostChart.jsx';
import ResponseTimeChart from './components/ResponseTimeChart.jsx';
import ConnectionActivityChart from './components/ConnectionActivityChart.jsx';

// ============================================
// Overview 主页面
// ============================================

const OverviewPage = () => {
  const { data, refresh, isInitialized } = useOverviewData();

  if (data.error) {
    return (
      <ErrorMessage
        title="数据加载失败"
        message={data.error}
        onRetry={refresh}
      />
    );
  }

  if (data.loading && !isInitialized) {
    return <LoadingSpinner text="加载监控数据..." />;
  }

  return (
    <div className="animate-fade-in">
      {/* 页面标题 */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-fg">Dashboard</h1>
        <p className="text-fg-muted text-sm mt-1">高性能 API 请求转发器监控面板</p>
      </div>

      {/* KPI 卡片 */}
      <KPICardsGrid data={data} />

      {/* 图表区域 */}
      <div className="space-y-6">
        {/* 请求趋势 - 全宽 */}
        <RequestTrendChart />

        {/* 性能指标 - 响应时间 & 连接活动 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <ResponseTimeChart />
          <ConnectionActivityChart />
        </div>

        {/* Token 成本 - 全宽 */}
        <TokenCostChart />

        {/* 资源状态 - Token 分布 & 端点健康 */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <TokenDistributionChart />
          <EndpointHealthChart />
        </div>

        {/* 连接统计 */}
        <ConnectionStats connections={data.connections} />
      </div>
    </div>
  );
};

export default OverviewPage;
