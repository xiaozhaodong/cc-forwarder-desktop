// ============================================
// 连接统计详情组件
// 2025-11-28
// ============================================

import { StatDetailItem } from '@components/ui';
import { formatNumber } from '@utils/api.js';

const ConnectionStats = ({ connections }) => {
  const stats = [
    { label: '总请求数', value: formatNumber(connections.total_requests || 0), unit: '' },
    { label: '活跃连接', value: connections.active_connections || 0, unit: '' },
    { label: '成功请求', value: formatNumber(connections.successful_requests || 0), unit: '', valueColor: 'text-success' },
    { label: '失败请求', value: connections.failed_requests || 0, unit: '', valueColor: 'text-danger' },
    { label: '平均响应时间', value: connections.average_response_time || '0s', unit: '' },
    { label: 'TOKEN使用量', value: formatNumber(connections.total_tokens || 0), unit: '' }
  ];

  return (
    <div className="bg-surface rounded-2xl border border-line/60 shadow-sm mb-6 p-5">
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
        {stats.map((stat, index) => (
          <StatDetailItem
            key={index}
            label={stat.label}
            value={stat.value}
            unit={stat.unit}
            valueColor={stat.valueColor}
          />
        ))}
      </div>
    </div>
  );
};

export default ConnectionStats;
