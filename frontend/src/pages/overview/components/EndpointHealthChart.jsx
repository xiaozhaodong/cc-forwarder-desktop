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
import useChartTheme from '@hooks/useChartTheme.js';

// 连通性状态配置（颜色来自 useChartTheme 的 series，随主题切换）
const HEALTH_CONFIG = {
  healthy: { name: '最近可达', icon: CheckCircle2 },
  unhealthy: { name: '最近不可达', icon: XCircle }
};

const EndpointHealthChart = () => {
  const chart = useChartTheme();
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
    { name: HEALTH_CONFIG.healthy.name, value: healthData.healthy, color: chart.series.healthy },
    { name: HEALTH_CONFIG.unhealthy.name, value: healthData.unhealthy, color: chart.series.unhealthy }
  ];

  // 确定最近连通性状态的显示样式
  const getHealthStatus = () => {
    if (total === 0) return { text: '无数据', tone: 'tone-slate' };
    if (healthPercent >= 90) return { text: '优秀', tone: 'tone-emerald' };
    if (healthPercent >= 70) return { text: '良好', tone: 'tone-amber' };
    return { text: '警告', tone: 'tone-rose' };
  };

  const status = getHealthStatus();

  return (
    <div className="bg-surface p-6 rounded-2xl border border-line/60 shadow-sm flex flex-col h-full">
      <div className="flex justify-between items-start mb-1">
        <div className="flex items-center space-x-2">
          <div className="p-1.5 bg-success-soft text-success-solid rounded-md">
            <Activity size={16} />
          </div>
          <h3 className="font-semibold text-fg">端点连通性概览</h3>
        </div>
        <div className="flex items-center space-x-2">
          <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${status.tone}`}>
            {healthPercent}% {status.text}
          </span>
          <button
            onClick={handleRefresh}
            disabled={isRefreshing}
            className="p-1.5 text-fg-subtle hover:text-fg-muted hover:bg-surface-mut rounded-md transition-colors disabled:opacity-50"
            title="刷新数据"
          >
            <RefreshCw size={14} className={isRefreshing ? 'animate-spin' : ''} />
          </button>
        </div>
      </div>
      <p className="text-xs text-fg-muted mb-4">基于最近一次检测结果的连通性概览</p>

      <div className="flex-1 min-h-[180px] flex items-center justify-center relative">
        {loading ? (
          <div className="flex items-center text-fg-subtle">
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
                className={healthPercent >= 70 ? 'text-success-solid' : 'text-danger-solid'}
              />
              <span className="text-2xl font-bold text-fg mt-1">
                {healthData.healthy}/{total}
              </span>
              <span className="text-xs text-fg-subtle">最近可达</span>
            </div>
          </>
        )}
      </div>

      {/* 图例和详情：色块用 inline style 引色板，保证与扇区严格同色 */}
      {!loading && (
        <div className="grid grid-cols-2 gap-4 mt-2 pt-3 border-t border-line-soft">
          <div className="flex items-center justify-between">
            <div className="flex items-center text-xs text-fg-body">
              <span className="w-2.5 h-2.5 rounded-full mr-2" style={{ backgroundColor: chart.series.healthy }} />
              最近可达
            </div>
            <span className="font-mono text-sm font-semibold text-success">
              {healthData.healthy}
            </span>
          </div>
          <div className="flex items-center justify-between">
            <div className="flex items-center text-xs text-fg-body">
              <span className="w-2.5 h-2.5 rounded-full mr-2" style={{ backgroundColor: chart.series.unhealthy }} />
              最近不可达
            </div>
            <span className="font-mono text-sm font-semibold text-danger">
              {healthData.unhealthy}
            </span>
          </div>
        </div>
      )}
    </div>
  );
};

export default EndpointHealthChart;
