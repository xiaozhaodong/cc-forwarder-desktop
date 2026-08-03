// ============================================
// RequestDetailModal - 请求详情模态框
// Command Palette 风格
// 2025-12-01
// ============================================

import { useState, useEffect, useRef } from 'react';
import {
  X,
  Copy,
  Check,
  Clock,
  Activity,
  DollarSign,
  Server,
  FileText,
  Waves,
  RefreshCw,
  Calendar,
  Database,
  AlertCircle,
  TrendingUp,
  TrendingDown,
  Zap
} from 'lucide-react';
import RequestStatusBadge from './RequestStatusBadge.jsx';
import ModelTag from './ModelTag.jsx';
import { formatCost, formatTimestamp } from '@utils/api.js';
import useModalLifecycle from '@hooks/useModalLifecycle.js';
import { copyTextToClipboard } from './clipboard.js';
import {
  calculateTokensPerSecond,
  formatOptionalTimingBadge,
  formatTimingBadge,
  formatTpsBadge,
  getTimingPillClassName,
  resolveCompletionMs
} from '../utils/timing.js';
import { getRequestFamilyMeta } from '../utils/requestSource.js';

/**
 * 信息行组件
 */
const InfoRow = ({ icon: Icon, label, value, copyable = false }) => {
  const [copied, setCopied] = useState(false);
  const resetTimerRef = useRef(null);

  useEffect(() => () => {
    if (resetTimerRef.current) {
      clearTimeout(resetTimerRef.current);
      resetTimerRef.current = null;
    }
  }, []);

  const handleCopy = async () => {
    if (copyable && typeof value === 'string' && value) {
      const didCopy = await copyTextToClipboard(value, label);
      if (!didCopy) {
        setCopied(false);
        return;
      }

      setCopied(true);
      if (resetTimerRef.current) {
        clearTimeout(resetTimerRef.current);
      }
      resetTimerRef.current = window.setTimeout(() => {
        setCopied(false);
        resetTimerRef.current = null;
      }, 2000);
    }
  };

  return (
    <div className="flex items-start justify-between py-3 border-b border-slate-100 last:border-b-0 group">
      <div className="flex items-center gap-2 text-slate-500 min-w-[120px]">
        <Icon className="w-4 h-4 flex-shrink-0" />
        <span className="text-sm font-medium">{label}</span>
      </div>
      <div className="flex items-center gap-2 flex-1 justify-end">
        <span className="text-sm font-mono text-slate-900 text-right">
          {value || '-'}
        </span>
        {copyable && (
          <button
            onClick={handleCopy}
            className="p-1 opacity-0 group-hover:opacity-100 hover:bg-slate-100 rounded transition-all"
            title="复制"
          >
            {copied ? (
              <Check className="w-3.5 h-3.5 text-emerald-600" />
            ) : (
              <Copy className="w-3.5 h-3.5 text-slate-400" />
            )}
          </button>
        )}
      </div>
    </div>
  );
};

/**
 * Token 指标卡片
 */
const TokenCard = ({ icon: Icon, label, value, colorClass }) => (
  <div className={`rounded-lg p-4 border ${colorClass.border} ${colorClass.bg}`}>
    <div className="flex items-center justify-between mb-2">
      <Icon className={`w-4 h-4 ${colorClass.icon}`} />
    </div>
    <div className={`text-2xl font-bold font-mono ${colorClass.text}`}>
      {(value || 0).toLocaleString()}
    </div>
    <div className="text-xs text-slate-500 mt-1">{label}</div>
  </div>
);

const formatRouteMode = (mode) => {
  switch (mode) {
    case 'manual_fixed':
      return '严格固定';
    case 'manual_preferred':
      return '手动优选';
    default:
      return '自动';
  }
};

const RequestTimingValue = ({ request }) => {
  const hasFirstToken = request.isStreaming && Number.isFinite(request.firstTokenMs);
  const completionMs = hasFirstToken
    ? resolveCompletionMs(request.completionMs, request.duration, request.firstTokenMs)
    : null;
  const tokensPerSecond = calculateTokensPerSecond(request.outputTokens, completionMs);

  return (
    <span className="inline-flex items-center justify-end gap-1.5 flex-wrap">
      <span className={`inline-flex items-center px-2 py-1 rounded text-xs font-mono font-medium border transition-all ${hasFirstToken ? getTimingPillClassName('first', request.firstTokenMs) : 'bg-slate-50 text-slate-400 border-slate-100'}`}>
        首响 {formatOptionalTimingBadge(hasFirstToken ? request.firstTokenMs : null)}
      </span>
      <span className="inline-flex items-center px-2 py-1 rounded text-xs font-mono font-medium border transition-all bg-slate-50 text-slate-600 border-slate-100">
        生成 {formatOptionalTimingBadge(completionMs)}
      </span>
      <span className={`inline-flex items-center px-2 py-1 rounded text-xs font-mono font-medium border transition-all ${getTimingPillClassName('duration', request.duration)}`}>
        总耗 {formatTimingBadge(request.duration)}
      </span>
      <span className="inline-flex items-center px-2 py-1 rounded text-xs font-mono font-medium border transition-all bg-slate-50 text-slate-600 border-slate-100">
        TPS {formatTpsBadge(tokensPerSecond)}
      </span>
    </span>
  );
};

/**
 * 请求详情模态框主组件
 */
const RequestDetailModal = ({ isOpen, onClose, request }) => {
  const [activeTab, setActiveTab] = useState('overview');
  const closeButtonRef = useRef(null);

  useModalLifecycle({ open: isOpen, onClose, initialFocusRef: closeButtonRef });

  if (!isOpen || !request) return null;

  // 计算 Token 总数
  const totalTokens = (request.inputTokens || 0) + (request.outputTokens || 0);
  const inputPercent = totalTokens > 0 ? Math.round((request.inputTokens / totalTokens) * 100) : 0;
  const outputPercent = totalTokens > 0 ? Math.round((request.outputTokens / totalTokens) * 100) : 0;

  // 请求类型
  const StreamIcon = request.isStreaming ? Waves : RefreshCw;
  const streamLabel = request.isStreaming ? '流式请求' : '常规请求';
  const streamColor = request.isStreaming ? 'text-blue-600 bg-blue-50' : 'text-slate-600 bg-slate-50';
  const family = getRequestFamilyMeta(request.requestFamily);

  return (
    <div className="fixed inset-0 z-[10000] flex items-start justify-center pt-[15vh] px-4">
      {/* 背景遮罩 */}
      <div
        className="fixed inset-0 bg-slate-900/20 backdrop-blur-sm animate-in fade-in duration-200"
        onClick={onClose}
      />

      {/* 模态框内容 - 固定高度，内容区域滚动 */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label="请求详情"
        className="relative w-full max-w-3xl max-h-[80vh] bg-white rounded-2xl shadow-2xl ring-1 ring-black/5 animate-in zoom-in-95 fade-in duration-200 flex flex-col overflow-hidden"
      >
        {/* 头部 - 固定不滚动 */}
        <div className="flex-shrink-0 px-6 py-4 border-b border-slate-100 bg-white rounded-t-2xl flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-indigo-50 rounded-lg">
              <FileText className="w-5 h-5 text-indigo-600" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-slate-900">请求详情</h2>
              <div className="flex items-center gap-2 mt-0.5">
                <span className="text-xs text-slate-400 font-mono">{request.requestId}</span>
                <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium ${streamColor}`}>
                  <StreamIcon className="w-3 h-3" />
                  {streamLabel}
                </span>
              </div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <kbd className="hidden sm:inline-block px-2 py-1 bg-slate-100 border border-slate-200 rounded text-xs text-slate-500">ESC</kbd>
            <button ref={closeButtonRef} aria-label="关闭详情" onClick={onClose} className="p-2 hover:bg-slate-100 rounded-lg transition-colors text-slate-400 hover:text-slate-600">
              <X className="w-5 h-5" />
            </button>
          </div>
        </div>

        {/* Tab 导航 - 固定不滚动 */}
        <div className="flex-shrink-0 px-6 pt-4 border-b border-slate-100 bg-slate-50/50">
          <div className="flex gap-1">
            {[
              { id: 'overview', label: '概览', icon: Activity },
              { id: 'tokens', label: 'Token 详情', icon: Database },
              { id: 'network', label: '网络信息', icon: Server }
            ].map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`flex items-center gap-2 px-4 py-2 rounded-t-lg text-sm font-medium transition-all ${
                  activeTab === tab.id
                    ? 'bg-white text-indigo-600 shadow-sm border-t border-x border-slate-200'
                    : 'text-slate-500 hover:text-slate-700 hover:bg-slate-100/50'
                }`}
              >
                <tab.icon className="w-4 h-4" />
                {tab.label}
              </button>
            ))}
          </div>
        </div>

        {/* 内容区域 - 自动填充剩余空间并滚动 */}
        <div className="flex-1 p-6 overflow-y-auto scrollbar-thin scrollbar-thumb-slate-200 min-h-0">
          {/* 概览 Tab */}
          {activeTab === 'overview' && (
            <div className="space-y-6">
              {/* 状态 & 成本卡片 */}
              <div className="grid grid-cols-2 gap-4">
                <div className="bg-gradient-to-br from-indigo-50 to-blue-50 rounded-xl p-4 border border-indigo-100">
                  <div className="flex items-center justify-between mb-3">
                    <span className="text-sm font-medium text-slate-600">请求状态</span>
                    <Activity className="w-4 h-4 text-indigo-500" />
                  </div>
                  <RequestStatusBadge status={request.status} />
                </div>

                <div className="bg-gradient-to-br from-orange-50 to-amber-50 rounded-xl p-4 border border-orange-100">
                  <div className="flex items-center justify-between mb-3">
                    <span className="text-sm font-medium text-slate-600">总成本</span>
                    <DollarSign className="w-4 h-4 text-orange-500" />
                  </div>
                  <div className="text-2xl font-bold text-orange-600 font-mono">
                    {formatCost(request.cost)}
                  </div>
                </div>
              </div>

              {/* 基本信息 */}
              <div className="bg-white rounded-xl border border-slate-200 overflow-hidden">
                <div className="px-4 py-3 bg-slate-50 border-b border-slate-100">
                  <h3 className="text-sm font-semibold text-slate-900">基本信息</h3>
                </div>
                <div className="p-4">
                  <InfoRow icon={FileText} label="请求 ID" value={request.requestId} copyable />
                  <InfoRow icon={Calendar} label="时间戳" value={formatTimestamp(request.timestamp)} />
                  <InfoRow icon={Clock} label="耗时指标" value={<RequestTimingValue request={request} />} />
                  <InfoRow icon={Activity} label="类型" value={family.label} />
                  <InfoRow icon={Server} label="上游" value={request.upstreamName || '未知上游'} />
                  <InfoRow icon={Activity} label="路由模式" value={formatRouteMode(request.routeMode)} />
                  {request.requestedEndpoint && (
                    <InfoRow icon={Server} label="手动目标" value={request.requestedEndpoint} />
                  )}
                  {request.effectiveEndpoint && request.effectiveEndpoint !== request.upstreamName && (
                    <InfoRow icon={Server} label="实际端点" value={request.effectiveEndpoint} />
                  )}
                  {request.fallbackReason && (
                    <InfoRow icon={AlertCircle} label="回退原因" value={request.fallbackReason} />
                  )}
                </div>
              </div>

              {/* 模型信息 */}
              <div className="bg-white rounded-xl border border-slate-200 overflow-hidden">
                <div className="px-4 py-3 bg-slate-50 border-b border-slate-100">
                  <h3 className="text-sm font-semibold text-slate-900">模型信息</h3>
                </div>
                <div className="p-4">
                  <div className="flex items-center justify-between py-3">
                    <span className="text-sm font-medium text-slate-500">模型名称</span>
                    <ModelTag model={request.model} />
                  </div>
                </div>
              </div>

              {/* 错误信息（如果有） */}
              {(['failed', 'error', 'cancelled', 'timeout'].includes(request.status) ||
                request.failure_reason || request.cancel_reason) && (
                <div className="bg-rose-50 rounded-xl border border-rose-200 overflow-hidden">
                  <div className="px-4 py-3 bg-rose-100/50 border-b border-rose-200">
                    <h3 className="text-sm font-semibold text-rose-900 flex items-center gap-2">
                      <AlertCircle className="w-4 h-4" />
                      错误信息
                    </h3>
                  </div>
                  <div className="p-4 space-y-2">
                    {request.failure_reason && (
                      <div className="text-sm">
                        <span className="font-medium text-slate-600">失败原因：</span>
                        <span className="text-rose-700">{request.failure_reason}</span>
                      </div>
                    )}
                    {request.cancel_reason && (
                      <div className="text-sm">
                        <span className="font-medium text-slate-600">取消原因：</span>
                        <span className="text-rose-700">{request.cancel_reason}</span>
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Token 详情 Tab */}
          {activeTab === 'tokens' && (
            <div className="space-y-6">
              {/* Token 卡片 - 基础 */}
              <div className="grid grid-cols-2 gap-4">
                <TokenCard
                  icon={TrendingUp}
                  label="输入 Token"
                  value={request.inputTokens || 0}
                  colorClass={{ bg: 'bg-blue-50', border: 'border-blue-200', text: 'text-blue-600', icon: 'text-blue-600' }}
                />
                <TokenCard
                  icon={TrendingDown}
                  label="输出 Token"
                  value={request.outputTokens || 0}
                  colorClass={{ bg: 'bg-emerald-50', border: 'border-emerald-200', text: 'text-emerald-600', icon: 'text-emerald-600' }}
                />
              </div>

              {/* Token 卡片 - 缓存 */}
              <div className="grid grid-cols-2 gap-4">
                <TokenCard
                  icon={Database}
                  label="缓存创建 (5分钟)"
                  value={request.cacheCreation5mTokens || 0}
                  colorClass={{ bg: 'bg-purple-50', border: 'border-purple-200', text: 'text-purple-600', icon: 'text-purple-600' }}
                />
                <TokenCard
                  icon={Database}
                  label="缓存创建 (1小时)"
                  value={request.cacheCreation1hTokens || 0}
                  colorClass={{ bg: 'bg-violet-50', border: 'border-violet-200', text: 'text-violet-600', icon: 'text-violet-600' }}
                />
                <TokenCard
                  icon={Zap}
                  label="缓存读取"
                  value={request.cacheReadTokens || 0}
                  colorClass={{ bg: 'bg-amber-50', border: 'border-amber-200', text: 'text-amber-600', icon: 'text-amber-600' }}
                />
                <TokenCard
                  icon={Database}
                  label="缓存创建 (总计)"
                  value={request.cacheCreationTokens || 0}
                  colorClass={{ bg: 'bg-slate-50', border: 'border-slate-200', text: 'text-slate-600', icon: 'text-slate-500' }}
                />
              </div>

              {/* Token 总计 */}
              <div className="bg-gradient-to-r from-indigo-50 to-blue-50 rounded-xl p-6 border border-indigo-100">
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className="p-2 bg-white rounded-lg shadow-sm">
                      <FileText className="w-5 h-5 text-indigo-600" />
                    </div>
                    <div>
                      <div className="text-sm font-medium text-slate-600">总 Token 消耗</div>
                      <div className="text-xs text-slate-400">Input + Output</div>
                    </div>
                  </div>
                  <div className="text-3xl font-bold text-indigo-600 font-mono">
                    {totalTokens.toLocaleString()}
                  </div>
                </div>

                {/* 分布条 */}
                <div className="space-y-2">
                  <div className="flex items-center justify-between text-xs text-slate-600">
                    <span>输入 {inputPercent}%</span>
                    <span>输出 {outputPercent}%</span>
                  </div>
                  <div className="h-2 bg-slate-200 rounded-full overflow-hidden flex">
                    <div className="bg-blue-500" style={{ width: `${inputPercent}%` }} />
                    <div className="bg-emerald-500" style={{ width: `${outputPercent}%` }} />
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* 网络信息 Tab */}
          {activeTab === 'network' && (
            <div className="bg-white rounded-xl border border-slate-200 overflow-hidden">
              <div className="px-4 py-3 bg-slate-50 border-b border-slate-100">
                <h3 className="text-sm font-semibold text-slate-900">网络详情</h3>
              </div>
              <div className="p-4">
                <InfoRow icon={Server} label="HTTP 状态码" value={request.http_status_code || request.httpStatusCode || '-'} />
                <InfoRow icon={RefreshCw} label="重试次数" value={`${request.retry_count || request.retryCount || 0} 次`} />
                <InfoRow icon={Activity} label="请求方法" value={request.method || 'POST'} />
                <InfoRow icon={FileText} label="请求路径" value={request.path || '/v1/messages'} copyable />
              </div>
            </div>
          )}
        </div>

        {/* 底部操作栏 - 固定不滚动 */}
        <div className="flex-shrink-0 px-6 py-4 border-t border-slate-100 bg-slate-50/80 backdrop-blur-sm rounded-b-2xl flex justify-between items-center">
          <div className="text-xs text-slate-500">
            Request ID: <span className="font-mono text-slate-700">{request.requestId}</span>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => copyTextToClipboard(request.requestId, '请求 ID')}
              className="px-3 py-1.5 text-xs font-medium text-slate-600 bg-white border border-slate-200 rounded-lg hover:bg-slate-50 transition-colors flex items-center gap-1.5"
            >
              <Copy className="w-3.5 h-3.5" /> 复制 ID
            </button>
            <button
              onClick={onClose}
              className="px-3 py-1.5 text-xs font-medium text-white bg-indigo-600 rounded-lg hover:bg-indigo-700 transition-colors"
            >
              关闭
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};

export default RequestDetailModal;
