// ============================================
// EndpointRow - 端点表格行组件
// ============================================

import React from 'react';
import {
  Globe,
  Copy,
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
import { getEndpointLastCheckDisplayValue } from '../utils/lastCheckDisplay.js';
import { summarizeEndpointModelRewriteRules } from '../utils/modelRewrite.js';

// ============================================
// 连通性状态徽章
// ============================================

const HealthBadge = ({ healthy, neverChecked }) => {
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
// 冷却状态徽章
// ============================================

const CooldownBadge = ({ inCooldown, cooldownUntil, cooldownReason }) => {
  if (!inCooldown) return null;

  // 格式化剩余冷却时间
  const formatRemainingTime = (until) => {
    if (!until) return '';
    try {
      const endTime = new Date(until);
      const now = new Date();
      const diffMs = endTime - now;
      if (diffMs <= 0) return '即将恢复';
      const diffMins = Math.ceil(diffMs / 60000);
      if (diffMins < 60) return `${diffMins}分钟`;
      const diffHours = Math.floor(diffMins / 60);
      return `${diffHours}小时${diffMins % 60}分`;
    } catch {
      return '';
    }
  };

  return (
    <div
      className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-amber-50 text-amber-600 border border-amber-200 cursor-help"
      title={`冷却原因: ${cooldownReason || '请求失败'}\n恢复时间: ${cooldownUntil}`}
    >
      <Timer size={10} className="mr-1 animate-pulse" />
      冷却中 {formatRemainingTime(cooldownUntil)}
    </div>
  );
};

// ============================================
// 延迟指示器
// ============================================

const LatencyBadge = ({ ms }) => {
  if (!ms || ms === 0) return <span className="text-slate-300 text-xs">-</span>;

  let colorClass = 'text-emerald-600 bg-emerald-50 border-emerald-100';
  if (ms > 500) colorClass = 'text-amber-600 bg-amber-50 border-amber-100';
  if (ms > 1000) colorClass = 'text-rose-600 bg-rose-50 border-rose-100';

  return (
    <span className={`font-mono text-xs font-medium px-2 py-0.5 rounded border ${colorClass}`}>
      {ms}ms
    </span>
  );
};

// ============================================
// 端点表格行组件
// ============================================

const EndpointRow = ({
  endpoint,
  storageMode,
  onActivateGroup,
  onEdit,
  onDelete,
  onToggle,
  isGrouped = false
}) => {
  if (!endpoint) return null;

  // 格式化最后检查时间
  const formatLastCheck = (time) => {
    if (!time || time === '-') return '-';
    try {
      const date = new Date(time);
      return date.toLocaleString('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
      });
    } catch {
      return time;
    }
  };

  const isSqliteMode = storageMode === 'sqlite';
  // v8：开关表达硬启用（availability），不再表达 legacy active
  const isActive = isSqliteMode ? endpoint.availabilityEnabled !== false : endpoint.group_is_active;
  const responseTime = endpoint.response_time || endpoint.responseTimeMs || 0;
  const isNeverChecked = endpoint.never_checked === true || endpoint.neverChecked === true || (!endpoint.lastCheck && !endpoint.last_check);
  const modelRewriteSummary = summarizeEndpointModelRewriteRules(endpoint.modelRewriteRules || endpoint.model_rewrite_rules || '');

  // 获取认证类型显示
  const getAuthType = () => {
    if (endpoint.token || endpoint.tokenMasked) return 'Token';
    if (endpoint.apiKey) return 'API Key';
    return null;
  };

  return (
    <tr className={`transition-colors group ${isActive ? 'hover:bg-slate-50/50' : 'bg-slate-50/30 opacity-70'}`}>
      {/* 启用状态 Toggle */}
      <td className={`py-4 ${isGrouped ? 'px-6 pl-10' : 'px-6'}`}>
        <div className="flex items-center">
          {isGrouped && (
            <span className="text-slate-300 mr-2">└</span>
          )}
          <div
          className="cursor-pointer"
          onClick={() => {
            if (isSqliteMode) {
              onToggle?.(endpoint.name, !isActive);
            } else {
              onActivateGroup?.(endpoint.name, endpoint.group);
            }
          }}
        >
          {isActive ? (
            <div className="w-10 h-6 bg-emerald-500 rounded-full relative transition-colors shadow-inner">
              <div className="absolute right-1 top-1 w-4 h-4 bg-white rounded-full shadow-sm"></div>
            </div>
          ) : (
            <div className="w-10 h-6 bg-slate-200 rounded-full relative transition-colors shadow-inner">
              <div className="absolute left-1 top-1 w-4 h-4 bg-white rounded-full shadow-sm"></div>
            </div>
          )}
        </div>
        </div>
      </td>

      {/* 渠道 / 名称 / 连通性状态 */}
      <td className="px-6 py-4">
        <div className="flex flex-col space-y-1.5">
          <span className="font-bold text-slate-900 text-sm">{endpoint.name}</span>
          <div className="flex items-center space-x-2 flex-wrap gap-y-1">
            <span className="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium bg-blue-50 text-blue-600 border border-blue-100">
              {endpoint.channel || endpoint.group || '-'}
            </span>
            <HealthBadge healthy={endpoint.healthy} neverChecked={isNeverChecked} />
            <CooldownBadge
              inCooldown={endpoint.in_cooldown || endpoint.inCooldown}
              cooldownUntil={endpoint.cooldown_until || endpoint.cooldownUntil}
              cooldownReason={endpoint.cooldown_reason || endpoint.cooldownReason}
            />
          </div>
        </div>
      </td>

      {/* URL / 认证 */}
      <td className="px-6 py-4">
        <div className="flex flex-col space-y-1.5">
          <div className="flex items-center text-slate-500 max-w-[240px]" title={endpoint.url}>
            <Globe size={12} className="mr-1.5 text-slate-400 flex-shrink-0" />
            <span className="truncate text-xs font-mono">{endpoint.url}</span>
          </div>
          {getAuthType() && (
            <div className="flex items-center">
              <div className="flex items-center text-[10px] text-slate-400 bg-slate-100 px-1.5 py-0.5 rounded border border-slate-200">
                <ShieldCheck size={10} className="mr-1 text-amber-500" />
                已配置 {getAuthType()}
              </div>
            </div>
          )}
        </div>
      </td>

      {/* 优先级 */}
      <td className="px-6 py-4 text-center">
        <div className="inline-flex items-center justify-center w-8 h-8 rounded-full bg-slate-50 border border-slate-200 font-bold text-slate-600 text-xs">
          {endpoint.priority || 1}
        </div>
      </td>

      {/* 高级特性 */}
      <td className="px-6 py-4">
        <div className="flex items-center space-x-2">
          <div
            className={`p-1.5 rounded-md ${endpoint.failoverEnabled !== false ? 'bg-indigo-50 text-indigo-600' : 'bg-slate-100 text-slate-300'}`}
            title={endpoint.failoverEnabled !== false ? '参与自动调度' : '不参与自动调度（仍可手动优选或固定使用）'}
          >
            <ArrowRightLeft size={14} />
          </div>
          <div
            className={`p-1.5 rounded-md ${endpoint.supportsCountTokens ? 'bg-purple-50 text-purple-600' : 'bg-slate-100 text-slate-300'}`}
            title="支持 Token 计数"
          >
            <Calculator size={14} />
          </div>
          {modelRewriteSummary && (
            <span
              className="max-w-[220px] truncate rounded-md border border-cyan-100 bg-cyan-50 px-2 py-1 text-[10px] font-medium text-cyan-700"
              title={modelRewriteSummary.title}
            >
              {modelRewriteSummary.label}
            </span>
          )}
        </div>
      </td>

      {/* 响应延迟 */}
      <td className="px-6 py-4 text-center">
        <LatencyBadge ms={responseTime} />
      </td>

      {/* 倍率 */}
      <td className="px-6 py-4 text-center">
        <span className={`text-xs font-mono font-medium px-2 py-1 rounded ${
          (endpoint.costMultiplier || 1) > 1.0
            ? 'bg-orange-50 text-orange-600 border border-orange-100'
            : 'text-slate-500 bg-slate-50'
        }`}>
          {endpoint.costMultiplier || 1.0}x
        </span>
      </td>

      {/* 最近检测 */}
      <td className="px-6 py-4 text-slate-400 font-mono text-xs">
        {formatLastCheck(getEndpointLastCheckDisplayValue(endpoint))}
      </td>

      {/* 操作 */}
      <td className="px-6 py-4 text-right">
        <div className="flex items-center justify-end space-x-1">
          <button
            onClick={() => {
              navigator.clipboard.writeText(JSON.stringify(endpoint, null, 2));
            }}
            className="p-1.5 text-slate-400 hover:bg-slate-100 hover:text-indigo-600 rounded-md transition-colors"
            title="复制配置"
          >
            <Copy size={14} />
          </button>
          {isSqliteMode && (
            <>
              <button
                onClick={() => onEdit?.(endpoint)}
                className="p-1.5 text-slate-400 hover:bg-slate-100 hover:text-indigo-600 rounded-md transition-colors"
                title="编辑"
              >
                <Pencil size={14} />
              </button>
              <button
                onClick={() => onDelete?.(endpoint)}
                className="p-1.5 text-slate-400 hover:bg-rose-50 hover:text-rose-600 rounded-md transition-colors"
                title="删除"
              >
                <Trash2 size={14} />
              </button>
            </>
          )}
        </div>
      </td>
    </tr>
  );
};

export default EndpointRow;
