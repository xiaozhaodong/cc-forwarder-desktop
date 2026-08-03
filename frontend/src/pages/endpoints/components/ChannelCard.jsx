// ============================================
// ChannelCard - 渠道卡片组件
// 2025-12-24
// ============================================

import React, { useState } from 'react';
import { ChevronDown, ChevronUp } from 'lucide-react';
import EndpointCard from './EndpointCard';

// 默认显示的端点数量
const DEFAULT_VISIBLE_COUNT = 2;

const ChannelCard = ({
  channel,
  endpoints,
  storageMode,
  onActivateGroup,
  onEdit,
  onDelete,
  onToggle,
  routingState,
  onSetRouting
}) => {
  const [expanded, setExpanded] = useState(false);

  const isSqliteMode = storageMode === 'sqlite';

  // 统计数据
  const totalCount = endpoints.length;
  const enabledCount = endpoints.filter(e => isSqliteMode ? e.enabled : e.group_is_active).length;
  const healthyCount = endpoints.filter(e => e.healthy).length;
  const checkedCount = endpoints.filter(e => e.never_checked !== true && e.neverChecked !== true).length;
  const cooldownCount = endpoints.filter(e => e.in_cooldown || e.inCooldown).length;

  // 是否有激活的端点
  const hasActiveEndpoint = enabledCount > 0;

  // 是否需要折叠
  const needsCollapse = totalCount > DEFAULT_VISIBLE_COUNT;
  const hiddenCount = totalCount - DEFAULT_VISIBLE_COUNT;

  // 显示的端点列表
  const visibleEndpoints = expanded ? endpoints : endpoints.slice(0, DEFAULT_VISIBLE_COUNT);

  return (
    <div
      className={`
        bg-white rounded-2xl border shadow-sm overflow-hidden transition-colors
        ${hasActiveEndpoint ? 'ring-1' : 'border-slate-200/60'}
      `}
      style={hasActiveEndpoint ? { borderColor: '#A3B3FF', '--tw-ring-color': '#A3B3FF33' } : {}}
    >
      {/* 渠道头部 */}
      <div className="px-4 py-3 border-b border-slate-100 bg-slate-50/50">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {/* 渠道名称 */}
            <h3 className="font-bold text-slate-800 text-sm">
              {channel}
            </h3>
            {/* 端点数量 */}
            <span className="text-[10px] text-slate-500 bg-slate-200/80 px-1.5 py-0.5 rounded-full">
              {totalCount}
            </span>
          </div>

          {/* 状态汇总 */}
          <div className="flex items-center gap-1.5 text-[10px]">
            <span className="text-indigo-600 bg-indigo-50 px-1.5 py-0.5 rounded border border-indigo-100">
              {enabledCount} 启用
            </span>
            <span className={`px-1.5 py-0.5 rounded border ${
              checkedCount > 0 && healthyCount === checkedCount
                ? 'text-indigo-600 bg-indigo-50 border-indigo-100'
              : healthyCount > 0
                  ? 'text-amber-600 bg-amber-50 border-amber-100'
                  : 'text-slate-400 bg-slate-50 border-slate-200'
            }`}>
              {checkedCount > 0 ? `${healthyCount}/${checkedCount} 可达` : `${totalCount} 未检测`}
            </span>
            {cooldownCount > 0 && (
              <span className="text-amber-600 bg-amber-50 px-1.5 py-0.5 rounded border border-amber-200">
                {cooldownCount} 冷却
              </span>
            )}
          </div>
        </div>
      </div>

      {/* 端点卡片区域 */}
      <div className="p-3">
        <div className="flex flex-col gap-2">
          {visibleEndpoints.map((endpoint, index) => (
            <EndpointCard
              key={endpoint.name || index}
              endpoint={endpoint}
              storageMode={storageMode}
              onActivateGroup={onActivateGroup}
              onEdit={onEdit}
              onDelete={onDelete}
              onToggle={onToggle}
              routingState={routingState}
              onSetRouting={onSetRouting}
            />
          ))}
        </div>

        {/* 展开/折叠按钮 */}
        {needsCollapse && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="w-full mt-2 py-1.5 flex items-center justify-center gap-1 text-[11px] text-slate-500 hover:text-slate-700 hover:bg-slate-50 rounded-lg transition-colors border border-dashed border-slate-200"
          >
            {expanded ? (
              <>
                <ChevronUp size={12} />
                收起
              </>
            ) : (
              <>
                <ChevronDown size={12} />
                展开 +{hiddenCount}
              </>
            )}
          </button>
        )}
      </div>
    </div>
  );
};

export default ChannelCard;
