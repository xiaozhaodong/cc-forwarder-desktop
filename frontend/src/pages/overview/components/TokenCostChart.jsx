// ============================================
// Token 成本图组件
// 2025-11-28
// ============================================

import { useState, useEffect, useCallback, useRef } from 'react';
import { RefreshCw, DollarSign } from 'lucide-react';
import {
  ComposedChart,
  Bar,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer
} from 'recharts';
import { fetchEndpointCostsData } from '@utils/api.js';
import useChartTheme from '@hooks/useChartTheme.js';

// 格式化 Token 数量
const formatTokens = (value) => {
  if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
  if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
  return value.toString();
};

// 自定义 Tooltip
const CustomTooltip = ({ active, payload, label }) => {
  if (!active || !payload || !payload.length) return null;

  return (
    <div className="bg-surface p-3 rounded-lg shadow-lg border border-line-soft text-sm">
      <p className="font-medium text-fg mb-2">{label}</p>
      {payload.map((entry, index) => (
        <div key={index} className="flex items-center justify-between gap-4">
          <span className="text-fg-muted">{entry.name}:</span>
          <span className="font-mono font-medium" style={{ color: entry.color }}>
            {entry.dataKey === 'tokens' ? formatTokens(entry.value) : `$${entry.value.toFixed(2)}`}
          </span>
        </div>
      ))}
    </div>
  );
};

const TokenCostChart = () => {
  const chart = useChartTheme();
  const [chartData, setChartData] = useState([]);
  const [loading, setLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const refreshIntervalRef = useRef(null);

  // 加载数据
  const loadData = useCallback(async (showRefreshing = false) => {
    if (showRefreshing) {
      setIsRefreshing(true);
    }
    try {
      const data = await fetchEndpointCostsData();
      setChartData(data);
    } catch (error) {
      console.error('加载端点成本数据失败:', error);
    } finally {
      setLoading(false);
      setIsRefreshing(false);
    }
  }, []);

  // 初始加载
  useEffect(() => {
    loadData();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // 定时刷新（每 60 秒）
  useEffect(() => {
    refreshIntervalRef.current = setInterval(() => {
      loadData(false);
    }, 60000);

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
      if (chart_type === 'endpoint_costs' || chart_type === 'endpointCosts') {
        if (Array.isArray(data)) {
          setChartData(data);
        } else if (data?.labels && data?.datasets) {
          // 如果是 Chart.js 格式，转换为 Recharts 格式
          const tokensData = data.datasets.find(d => d.label?.includes('Token'))?.data || [];
          const costData = data.datasets.find(d => d.label?.includes('成本') || d.label?.includes('Cost'))?.data || [];
          const converted = data.labels.map((name, i) => ({
            name,
            tokens: tokensData[i] || 0,
            cost: costData[i] || 0
          }));
          setChartData(converted);
        }
        console.log('📊 [SSE] 端点成本图已更新');
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

  return (
    <div className="bg-surface p-6 rounded-2xl border border-line/60 shadow-sm">
      <div className="flex justify-between items-start mb-1">
        <div className="flex items-center space-x-2">
          <div className="p-1.5 bg-danger-soft text-danger-solid rounded-md">
            <DollarSign size={16} />
          </div>
          <h3 className="font-semibold text-fg">当日上游 Token 成本</h3>
        </div>
        <button
          onClick={handleRefresh}
          disabled={isRefreshing}
          className="p-1.5 text-fg-subtle hover:text-fg-muted hover:bg-surface-mut rounded-md transition-colors disabled:opacity-50"
          title="刷新数据"
        >
          <RefreshCw size={14} className={isRefreshing ? 'animate-spin' : ''} />
        </button>
      </div>
      <p className="text-xs text-fg-muted mb-4">按请求类型与真实上游汇总 Token 使用量和预估成本</p>

      <div className="h-[280px] w-full">
        {loading ? (
          <div className="h-full flex items-center justify-center text-fg-subtle">
            <RefreshCw size={20} className="animate-spin mr-2" />
            加载中...
          </div>
        ) : chartData.length === 0 ? (
          <div className="h-full flex items-center justify-center text-fg-subtle text-sm">
            暂无数据
          </div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <ComposedChart data={chartData} margin={{ top: 10, right: 50, left: 0, bottom: 5 }}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} stroke={chart.grid} />
              <XAxis
                dataKey="name"
                axisLine={false}
                tickLine={false}
                tick={{ fill: chart.axisStrong, fontSize: 12 }}
                interval={0}
              />
              <YAxis
                yAxisId="left"
                axisLine={false}
                tickLine={false}
                tick={{ fill: chart.axis, fontSize: 11 }}
                tickFormatter={formatTokens}
                width={50}
              />
              <YAxis
                yAxisId="right"
                orientation="right"
                axisLine={false}
                tickLine={false}
                tick={{ fill: chart.series.cost, fontSize: 11 }}
                tickFormatter={(v) => `$${v.toFixed(2)}`}
                width={55}
              />
              <Tooltip content={<CustomTooltip />} cursor={{ fill: chart.cursor }} />
              <Bar
                yAxisId="left"
                dataKey="tokens"
                fill={chart.series.tokens}
                barSize={32}
                radius={[4, 4, 0, 0]}
                name="Token 量"
              />
              <Line
                yAxisId="right"
                type="monotone"
                dataKey="cost"
                stroke={chart.series.cost}
                strokeWidth={2}
                dot={{ r: 4, fill: chart.series.cost, strokeWidth: 0 }}
                activeDot={{ r: 6, fill: chart.series.cost, strokeWidth: 2, stroke: chart.dotStroke }}
                name="成本 (USD)"
              />
            </ComposedChart>
          </ResponsiveContainer>
        )}
      </div>

      {/* 图例：色块用 inline style 引色板，保证与柱/线严格同色 */}
      {!loading && chartData.length > 0 && (
        <div className="flex justify-center space-x-6 mt-4 pt-3 border-t border-line-soft">
          <div className="flex items-center text-xs text-fg-muted">
            <span className="w-3 h-3 rounded mr-2" style={{ backgroundColor: chart.series.tokens }}></span>
            Token 使用量
          </div>
          <div className="flex items-center text-xs text-fg-muted">
            <span className="w-3 h-3 rounded-full mr-2" style={{ backgroundColor: chart.series.cost }}></span>
            成本 (USD)
          </div>
        </div>
      )}
    </div>
  );
};

export default TokenCostChart;
