// ============================================
// EndpointCard - 端点卡片组件
// 2025-12-24
// ============================================

import React from 'react';
import {
  Globe,
  Key,
  Pencil,
  Trash2,
  ArrowRightLeft,
  Calculator,
  ShieldCheck,
  CheckCircle2,
  XCircle,
  Clock,
  Timer
} from 'lucide-react';
import { BrowserOpenURL } from '@wailsjs/runtime/runtime';
import { summarizeEndpointModelRewriteRules } from '../utils/modelRewrite.js';

// ============================================
// 连通性状态徽章
// ============================================

const HealthBadge = ({ healthy, neverChecked, inCooldown }) => {
  if (inCooldown) {
    return (
      <div className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-amber-50 text-amber-600 border border-amber-200">
        <Timer size={10} className="mr-1 animate-pulse" />
        冷却中
      </div>
    );
  }

  if (neverChecked) {
    return (
      <div className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-slate-50 text-slate-400 border border-slate-200">
        <Clock size={10} className="mr-1" />
        未检测
      </div>
    );
  }

  return healthy ? (
    <div className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-50 text-emerald-600 border border-emerald-100">
      <CheckCircle2 size={10} className="mr-1" />
      可达
    </div>
  ) : (
    <div className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-rose-50 text-rose-600 border border-rose-100">
      <XCircle size={10} className="mr-1" />
      不可达
    </div>
  );
};

// ============================================
// 延迟徽章
// ============================================

const LatencyBadge = ({ ms }) => {
  if (!ms || ms === 0) return <span className="text-slate-300 text-xs">-</span>;

  let colorClass = 'text-emerald-600 bg-emerald-50 border-emerald-100';
  if (ms > 500) colorClass = 'text-amber-600 bg-amber-50 border-amber-100';
  if (ms > 1000) colorClass = 'text-rose-600 bg-rose-50 border-rose-100';

  return (
    <span className={`font-mono text-xs font-medium px-1.5 py-0.5 rounded border ${colorClass}`}>
      {ms}ms
    </span>
  );
};

// ============================================
// 端点卡片组件
// ============================================

const EndpointCard = ({
  endpoint,
  storageMode,
  onActivateGroup,
  onEdit,
  onDelete,
  onToggle
}) => {
  if (!endpoint) return null;

  const isSqliteMode = storageMode === 'sqlite';
  const isActive = isSqliteMode ? endpoint.enabled : endpoint.group_is_active;
  const responseTime = endpoint.response_time || endpoint.responseTimeMs || 0;
  const isNeverChecked = endpoint.never_checked === true || endpoint.neverChecked === true || (!endpoint.lastCheck && !endpoint.last_check);
  const inCooldown = endpoint.in_cooldown || endpoint.inCooldown;
  const modelRewriteSummary = summarizeEndpointModelRewriteRules(endpoint.modelRewriteRules || endpoint.model_rewrite_rules || '');

  // 获取认证类型显示
  const getAuthType = () => {
    if (endpoint.token || endpoint.tokenMasked) return 'Token';
    if (endpoint.apiKey) return 'API Key';
    return null;
  };

  return (
    <div className={`
      bg-slate-50/80 rounded-lg border px-3 py-2.5 hover:bg-slate-50 transition-all
      ${isActive ? 'border-slate-200/80' : 'border-slate-200/60 opacity-70'}
    `}>
      {/* 单行布局：开关 | 名称+状态 | 信息标签 | 操作按钮 */}
      <div className="flex min-w-0 items-center gap-3">
        {/* 开关 */}
        <div
          className="cursor-pointer flex-shrink-0"
          onClick={() => {
            if (isSqliteMode) {
              onToggle?.(endpoint.name, !isActive);
            } else {
              onActivateGroup?.(endpoint.name, endpoint.group);
            }
          }}
        >
          {isActive ? (
            <div className="w-8 h-[18px] bg-indigo-500 rounded-full relative transition-colors shadow-inner">
              <div className="absolute right-0.5 top-[1px] w-4 h-4 bg-white rounded-full shadow-sm"></div>
            </div>
          ) : (
            <div className="w-8 h-[18px] bg-slate-300 rounded-full relative transition-colors shadow-inner">
              <div className="absolute left-0.5 top-[1px] w-4 h-4 bg-white rounded-full shadow-sm"></div>
            </div>
          )}
        </div>

        {/* 名称 + 连通性状态 */}
        <div className="flex items-center gap-2 min-w-0 flex-shrink-0">
          <span className="font-semibold text-slate-800 text-sm truncate max-w-[120px]" title={endpoint.name}>
            {endpoint.name}
          </span>
          <HealthBadge
            healthy={endpoint.healthy}
            neverChecked={isNeverChecked}
            inCooldown={inCooldown}
          />
        </div>

        {/* URL - 可伸缩区域 */}
        <div className="flex-1 min-w-0 hidden sm:block">
          <div className="flex items-center text-slate-400 text-xs" title={endpoint.url}>
            <button
              onClick={() => BrowserOpenURL(endpoint.url)}
              className="mr-1 flex-shrink-0 p-0.5 hover:bg-slate-200 hover:text-indigo-600 rounded transition-colors"
              title="打开网站"
            >
              <Globe size={10} />
            </button>
            <span className="truncate font-mono text-[11px]">{endpoint.url}</span>
          </div>
        </div>

        {/* 信息标签区 */}
        <div className="flex min-w-0 items-center gap-1.5 overflow-hidden">
          {/* 优先级 */}
          <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded border border-slate-200 bg-white text-[10px] font-bold text-slate-600" title="优先级">
            {endpoint.priority || 1}
          </span>

          {/* 延迟 */}
          <div className="shrink-0">
            <LatencyBadge ms={responseTime} />
          </div>

          {/* 倍率 */}
          {(endpoint.costMultiplier || 1) > 1.0 && (
            <span className="shrink-0 rounded border border-orange-100 bg-orange-50 px-1 py-0.5 font-mono text-[10px] font-medium text-orange-600">
              {endpoint.costMultiplier}x
            </span>
          )}

          {modelRewriteSummary && (
            <span
              className="min-w-0 max-w-[150px] flex-1 truncate rounded border border-cyan-100 bg-cyan-50 px-1.5 py-0.5 text-[10px] font-medium text-cyan-700"
              title={modelRewriteSummary.title}
            >
              {modelRewriteSummary.label}
            </span>
          )}

          {/* 故障转移 */}
          <div
            className={`shrink-0 rounded p-1 ${endpoint.failoverEnabled !== false ? 'bg-indigo-50 text-indigo-500' : 'bg-white text-slate-300 border border-slate-100'}`}
            title="故障转移"
          >
            <ArrowRightLeft size={11} />
          </div>

          {/* Token 计数 */}
          <div
            className={`shrink-0 rounded p-1 ${endpoint.supportsCountTokens ? 'bg-purple-50 text-purple-500' : 'bg-white text-slate-300 border border-slate-100'}`}
            title="Token 计数"
          >
            <Calculator size={11} />
          </div>

          {/* 认证 */}
          {getAuthType() && (
            <div className="shrink-0 rounded bg-amber-50 p-1 text-amber-500" title={`已配置 ${getAuthType()}`}>
              <ShieldCheck size={11} />
            </div>
          )}
        </div>

        {/* 操作按钮 */}
        <div className="ml-auto flex shrink-0 items-center gap-0.5">
          {/* 复制 Token */}
          {(endpoint.token || endpoint.apiKey) && (
            <button
              onClick={() => {
                const token = endpoint.token || endpoint.apiKey || '';
                navigator.clipboard.writeText(token);
              }}
              className="p-1.5 text-slate-400 hover:bg-slate-100 hover:text-amber-600 rounded transition-colors"
              title="复制 Token"
            >
              <Key size={14} />
            </button>
          )}
          {isSqliteMode && (
            <>
              <button
                onClick={() => onEdit?.(endpoint)}
                className="p-1.5 text-slate-400 hover:bg-slate-100 hover:text-indigo-600 rounded transition-colors"
                title="编辑"
              >
                <Pencil size={14} />
              </button>
              <button
                onClick={() => onDelete?.(endpoint)}
                className="p-1.5 text-slate-400 hover:bg-rose-50 hover:text-rose-600 rounded transition-colors"
                title="删除"
              >
                <Trash2 size={14} />
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
};

export default EndpointCard;
