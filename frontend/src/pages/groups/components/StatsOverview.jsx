// ============================================
// StatsOverview - 组统计概览组件
// 2025-12-01
// ============================================

import { Layers, Zap, Server, Clock } from 'lucide-react';

const StatsOverview = ({ groups = [] }) => {
  // 计算统计数据
  const activeGroup = groups.find(g => g.is_active);
  const totalEndpoints = groups.reduce((sum, g) => sum + (g.total_endpoints || 0), 0);
  const healthyEndpoints = groups.reduce((sum, g) => sum + (g.healthy_endpoints || 0), 0);
  const uncheckedEndpoints = groups.reduce((sum, g) => sum + (g.unchecked_endpoints || 0), 0);
  const checkedEndpoints = Math.max(totalEndpoints - uncheckedEndpoints, 0);
  const cooldownGroups = groups.filter(g => g.in_cooldown).length;

  const stats = [
    {
      label: '总组数',
      value: groups.length,
      icon: Layers,
      highlight: false,
      color: 'text-indigo-600',
      iconBg: 'bg-indigo-50 text-indigo-600'
    },
    {
      label: '活跃组',
      value: activeGroup?.name || '-',
      icon: Zap,
      highlight: true,
      color: 'text-indigo-600',
      iconBg: 'bg-indigo-50 text-indigo-600'
    },
    {
      label: '最近可达',
      value: checkedEndpoints > 0 ? `${healthyEndpoints}/${checkedEndpoints}` : '未检测',
      icon: Server,
      status: 'good',
      color: 'text-emerald-600',
      iconBg: 'bg-emerald-50 text-emerald-600'
    },
    {
      label: '冷却组数',
      value: cooldownGroups,
      icon: Clock,
      highlight: false,
      color: 'text-amber-600',
      iconBg: 'bg-amber-50 text-amber-600'
    }
  ];

  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
      {stats.map((stat, idx) => (
        <div
          key={idx}
          className={`bg-white p-5 rounded-xl border border-gray-100 shadow-sm flex items-center justify-between group hover:border-gray-300 transition-all ${
            stat.highlight ? 'ring-1 ring-indigo-50 border-indigo-100' : ''
          }`}
        >
          <div>
            <div className="text-xs font-medium text-gray-400 uppercase tracking-wider mb-1">
              {stat.label}
            </div>
            <div className={`text-2xl font-bold ${stat.highlight ? 'text-indigo-600 font-mono' : stat.color}`}>
              {stat.value}
            </div>
          </div>
          <div className={`p-3 rounded-xl ${stat.iconBg} transition-colors`}>
            <stat.icon className="w-5 h-5" />
          </div>
        </div>
      ))}
    </div>
  );
};

export default StatsOverview;
