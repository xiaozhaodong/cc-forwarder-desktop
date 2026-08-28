// ============================================
// Token 分布图组件
// 2025-11-28
// ============================================

import { useState, useEffect, useCallback, useRef } from 'react';
import { RefreshCw, PieChart as PieChartIcon } from 'lucide-react';
import {
  PieChart,
  Pie,
  Cell,
  Tooltip,
  ResponsiveContainer
} from 'recharts';
import { fetchTokenUsageData } from '@utils/api.js';
import useChartTheme from '@hooks/useChartTheme.js';

// Token 类型配置（颜色来自 useChartTheme 的 series，随主题切换）
const TOKEN_CONFIG = [
  { key: 'input', name: '输入 Token' },
  { key: 'output', name: '输出 Token' },
  { key: 'cacheCreation', name: '缓存创建' },
  { key: 'cacheRead', name: '缓存读取' }
];

// 格式化 Token 数量
const formatTokens = (value) => {
  if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
  if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
  return value.toString();
};

// 自定义 Tooltip
const CustomTooltip = ({ active, payload }) => {
  if (!active || !payload || !payload.length) return null;

  const data = payload[0].payload;
  return (
    <div className="bg-surface p-3 rounded-lg shadow-lg border border-line-soft text-sm">
      <div className="flex items-center gap-2 mb-1">
        <span
          className="w-3 h-3 rounded-full"
          style={{ backgroundColor: data.color }}
        />
        <span className="font-medium text-fg">{data.name}</span>
      </div>
      <div className="text-fg-body">
        <span className="font-mono">{formatTokens(data.value)}</span>
        <span className="text-fg-subtle ml-2">({data.percent.toFixed(1)}%)</span>
      </div>
    </div>
  );
};

const TokenDistributionChart = () => {
  const chart = useChartTheme();
  const [tokenData, setTokenData] = useState({ input: 0, output: 0, cacheCreation: 0, cacheRead: 0 });
  const [loading, setLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const refreshIntervalRef = useRef(null);

  // 加载数据
  const loadData = useCallback(async (showRefreshing = false) => {
    if (showRefreshing) {
      setIsRefreshing(true);
    }
    try {
      const data = await fetchTokenUsageData();
      setTokenData(data);
    } catch (error) {
      console.error('加载 Token 分布数据失败:', error);
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
      if (chart_type === 'token_distribution' || chart_type === 'tokenDistribution' || chart_type === 'token_usage') {
        if (data) {
          setTokenData({
            input: data.input || data.input_tokens || 0,
            output: data.output || data.output_tokens || 0,
            cacheCreation: data.cacheCreation || data.cache_creation_tokens || 0,
            cacheRead: data.cacheRead || data.cache_read_tokens || 0
          });
          console.log('📊 [SSE] Token 分布图已更新');
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

  // 计算总量和百分比
  const total = tokenData.input + tokenData.output + tokenData.cacheCreation + tokenData.cacheRead;

  // 转换为图表数据
  const chartData = TOKEN_CONFIG.map(config => ({
    ...config,
    color: chart.series[config.key],
    value: tokenData[config.key] || 0,
    percent: total > 0 ? ((tokenData[config.key] || 0) / total) * 100 : 0
  })).filter(item => item.value > 0); // 过滤掉零值

  // 如果没有数据，显示占位
  const hasData = total > 0;

  return (
    <div className="bg-surface p-6 rounded-2xl border border-line/60 shadow-sm flex flex-col h-full">
      <div className="flex justify-between items-start mb-1">
        <div className="flex items-center space-x-2">
          <div className="p-1.5 bg-accent-soft text-accent-ring rounded-md">
            <PieChartIcon size={16} />
          </div>
          <h3 className="font-semibold text-fg">Token 使用分布</h3>
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
      <p className="text-xs text-fg-muted mb-4">各类 Token 消耗占比</p>

      <div className="flex-1 min-h-[180px] flex items-center justify-center relative overflow-visible">
        {loading ? (
          <div className="flex items-center text-fg-subtle">
            <RefreshCw size={20} className="animate-spin mr-2" />
            加载中...
          </div>
        ) : !hasData ? (
          <div className="text-fg-subtle text-sm">暂无数据</div>
        ) : (
          <>
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={chartData}
                  innerRadius={50}
                  outerRadius={75}
                  paddingAngle={3}
                  dataKey="value"
                  animationBegin={0}
                  animationDuration={500}
                >
                  {chartData.map((entry, index) => (
                    <Cell
                      key={`cell-${index}`}
                      fill={entry.color}
                      stroke="none"
                      className="transition-opacity hover:opacity-80"
                    />
                  ))}
                </Pie>
                <Tooltip
                  content={<CustomTooltip />}
                  wrapperStyle={{ zIndex: 1000, pointerEvents: 'none' }}
                />
              </PieChart>
            </ResponsiveContainer>
            <div className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none">
              <span className="text-2xl font-bold text-fg-body">{formatTokens(total)}</span>
              <span className="text-xs text-fg-subtle">总计</span>
            </div>
          </>
        )}
      </div>

      {/* 图例 */}
      {!loading && hasData && (
        <div className="grid grid-cols-2 gap-x-4 gap-y-2 mt-4 pt-3 border-t border-line-soft">
          {TOKEN_CONFIG.map((config) => {
            const value = tokenData[config.key] || 0;
            const percent = total > 0 ? (value / total) * 100 : 0;
            return (
              <div key={config.key} className="flex items-center justify-between text-xs">
                <div className="flex items-center text-fg-body">
                  <span
                    className="w-2.5 h-2.5 rounded-full mr-2 flex-shrink-0"
                    style={{ backgroundColor: chart.series[config.key] }}
                  />
                  <span className="truncate">{config.name}</span>
                </div>
                <span className="font-mono text-fg-muted ml-2">
                  {percent > 0 ? `${percent.toFixed(0)}%` : '-'}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};

export default TokenDistributionChart;
