// ============================================
// 端点最近连通性状态图组件
// 2025-11-28
// ============================================

import { useState, useEffect, useCallback, useRef } from 'react';
import { RefreshCw, Activity, CheckCircle2, XCircle } from 'lucide-react';
import {
  PieChart,
  Pie,
  Cell,
  ResponsiveContainer
} from 'recharts';
import { fetchEndpointHealthData } from '@utils/api.js';

// 连通性状态配置
const HEALTH_CONFIG = {
  healthy: { name: '最近可达', color: '#10b981', icon: CheckCircle2 },
  unhealthy: { name: '最近不可达', color: '#ef4444', icon: XCircle }
};

const EndpointHealthChart = () => {
  const [healthData, setHealthData] = useState({ healthy: 0, unhealthy: 0 });
  const [loading, setLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const refreshIntervalRef = useRef(null);

  // 加载数据
  const loadData = useCallback(async (showRefreshing = false) => {
    if (showRefreshing) {
      setIsRefreshing(true);
    }
    try {
      const data = await fetchEndpointHealthData();
      setHealthData({
        healthy: data.healthy || 0,
        unhealthy: data.unhealthy || 0
      });
    } catch (error) {
      console.error('加载端点连通性数据失败:', error);
    } finally {
      setLoading(false);
      setIsRefreshing(false);
    }
  }, []);

  // 初始加载
  useEffect(() => {
    loadData();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // 定时刷新最近连通性概览
  useEffect(() => {
    refreshIntervalRef.current = setInterval(() => {
      loadData(false);
    }, 30000);

    return () => {
      if (refreshIntervalRef.current) {
        clearInterval(refreshIntervalRef.current);
      }
    };
  }, [loadData]);

  // 监听 SSE 图表更新事件
  useEffect(() => {
    const handleChartUpdate = (event) => {
      const { chart_type, data } = event.detail || {};
      if (chart_type === 'endpoint_health' || chart_type === 'endpointHealth') {
        if (data) {
          // 处理不同格式的数据
          if (data.healthy !== undefined) {
            setHealthData({
              healthy: data.healthy || 0,
              unhealthy: data.unhealthy || 0
            });
          } else if (data.labels && data.datasets) {
            // Chart.js 格式
            const [healthy, unhealthy] = data.datasets[0]?.data || [0, 0];
            setHealthData({ healthy, unhealthy });
          }
          console.log('📊 [SSE] 端点连通性图已更新');
        }
      }
    };

    document.addEventListener('chartUpdate', handleChartUpdate);
    return () => {
      document.removeEventListener('chartUpdate', handleChartUpdate);
    };
  }, []);

  // 手动刷新
  const handleRefresh = () => {
    loadData(true);
  };

  // 计算统计数据
  const total = healthData.healthy + healthData.unhealthy;
  const healthPercent = total > 0 ? Math.round((healthData.healthy / total) * 100) : 0;

  // 图表数据（半圆仪表盘）
  const chartData = [
    { name: '最近可达', value: healthData.healthy, color: HEALTH_CONFIG.healthy.color },
    { name: '最近不可达', value: healthData.unhealthy, color: HEALTH_CONFIG.unhealthy.color }
  ];

  // 确定最近连通性状态的显示样式
  const getHealthStatus = () => {
    if (total === 0) return { text: '无数据', color: 'text-slate-400', bg: 'bg-slate-50' };
    if (healthPercent >= 90) return { text: '优秀', color: 'text-emerald-600', bg: 'bg-emerald-50' };
    if (healthPercent >= 70) return { text: '良好', color: 'text-amber-600', bg: 'bg-amber-50' };
    return { text: '警告', color: 'text-rose-600', bg: 'bg-rose-50' };
  };

  const status = getHealthStatus();

  return (
    <div className="bg-white p-6 rounded-2xl border border-slate-200/60 shadow-sm flex flex-col h-full">
      <div className="flex justify-between items-start mb-1">
        <div className="flex items-center space-x-2">
          <div className="p-1.5 bg-emerald-50 text-emerald-500 rounded-md">
            <Activity size={16} />
          </div>
          <h3 className="font-semibold text-slate-900">端点连通性概览</h3>
        </div>
        <div className="flex items-center space-x-2">
          <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${status.bg} ${status.color}`}>
            {healthPercent}% {status.text}
          </span>
          <button
            onClick={handleRefresh}
            disabled={isRefreshing}
            className="p-1.5 text-slate-400 hover:text-slate-600 hover:bg-slate-100 rounded-md transition-colors disabled:opacity-50"
            title="刷新数据"
          >
            <RefreshCw size={14} className={isRefreshing ? 'animate-spin' : ''} />
          </button>
        </div>
      </div>
      <p className="text-xs text-slate-500 mb-4">基于最近一次检测结果的连通性概览</p>

      <div className="flex-1 min-h-[180px] flex items-center justify-center relative">
        {loading ? (
          <div className="flex items-center text-slate-400">
            <RefreshCw size={20} className="animate-spin mr-2" />
            加载中...
          </div>
        ) : (
          <>
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={chartData}
                  startAngle={180}
                  endAngle={0}
                  innerRadius={55}
                  outerRadius={80}
                  paddingAngle={2}
                  dataKey="value"
                  animationBegin={0}
                  animationDuration={500}
                >
                  {chartData.map((entry, index) => (
                    <Cell
                      key={`cell-${index}`}
                      fill={entry.color}
                      stroke="none"
                    />
                  ))}
                </Pie>
              </PieChart>
            </ResponsiveContainer>
            <div className="absolute inset-0 top-8 flex flex-col items-center justify-center pointer-events-none">
              <CheckCircle2
                size={28}
                className={healthPercent >= 70 ? 'text-emerald-500' : 'text-rose-500'}
              />
              <span className="text-2xl font-bold text-slate-900 mt-1">
                {healthData.healthy}/{total}
              </span>
              <span className="text-xs text-slate-400">最近可达</span>
            </div>
          </>
        )}
      </div>

      {/* 图例和详情 */}
      {!loading && (
        <div className="grid grid-cols-2 gap-4 mt-2 pt-3 border-t border-slate-100">
          <div className="flex items-center justify-between">
            <div className="flex items-center text-xs text-slate-600">
              <span className="w-2.5 h-2.5 rounded-full bg-emerald-500 mr-2" />
              最近可达
            </div>
            <span className="font-mono text-sm font-semibold text-emerald-600">
              {healthData.healthy}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <div className="flex items-center text-xs text-slate-600">
              <span className="w-2.5 h-2.5 rounded-full bg-rose-500 mr-2" />
              最近不可达
            </div>
            <span className="font-mono text-sm font-semibold text-rose-600">
              {healthData.unhealthy}
            </span>
          </div>
        </div>
      )}
    </div>
  );
};

export default EndpointHealthChart;
