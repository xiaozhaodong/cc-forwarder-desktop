// ============================================
// KPI 卡片网格组件
// 2025-12-06 v4.0: 一个端点 = 一个组
// 2025-12-12 v5.1: 服务状态改为成本和 tokens 统计
// ============================================

import {
  DollarSign,
  Zap,
  Server,
  Network,
  Activity
} from 'lucide-react';
import { KPICard } from '@components/ui';

// 格式化成本显示
const formatCost = (cost) => {
  if (!cost || cost === 0) return '$0.00';
  if (cost >= 1) return `$${cost.toFixed(2)}`;
  if (cost >= 0.01) return `$${cost.toFixed(3)}`;
  return `$${cost.toFixed(4)}`;
};

// 格式化 tokens 显示
const formatTokens = (tokens) => {
  if (!tokens || tokens === 0) return '0';
  if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(2)}M`;
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}K`;
  return tokens.toString();
};

const KPICardsGrid = ({ data }) => {
  const { endpoints, connections } = data;
  const endpointHealthText = endpoints.total > 0 ? `${endpoints.healthy || 0} / ${endpoints.total}` : '无端点';

  // v5.1+: 成本和 tokens 数据
  const todayCost = formatCost(connections.today_cost);
  const totalCost = formatCost(connections.all_time_total_cost);
  const todayTokens = formatTokens(connections.today_tokens);
  const totalTokens = formatTokens(connections.all_time_total_tokens);

  return (
    <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-8 gap-4 mb-6">
      <KPICard
        title="今日成本"
        value={todayCost}
        icon={DollarSign}
        statusColor="tone-emerald"
      />
      <KPICard
        title="今日 Tokens"
        value={todayTokens}
        icon={Zap}
        statusColor="tone-amber"
      />
      <KPICard
        title="今日请求"
        value={connections.today_requests || 0}
        icon={Network}
        statusColor="tone-cyan"
      />
      <KPICard
        title="总成本"
        value={totalCost}
        icon={DollarSign}
        statusColor="tone-blue"
      />
      <KPICard
        title="总 Tokens"
        value={totalTokens}
        icon={Zap}
        statusColor="tone-indigo"
      />
      <KPICard
        title="总请求数"
        value={connections.all_time_total_requests || 0}
        icon={Network}
        statusColor="tone-purple"
      />
      <KPICard
        title="端点数量"
        value={endpoints.total || 0}
        icon={Server}
        statusColor="tone-violet"
      />
      <KPICard
        title="最近可达"
        value={endpointHealthText}
        tooltip="最近主动检测可达数 / Claude 端点总数"
        icon={Activity}
        statusColor={(endpoints.healthy || 0) > 0 ? 'tone-emerald' : 'tone-slate'}
      />
    </div>
  );
};

export default KPICardsGrid;
