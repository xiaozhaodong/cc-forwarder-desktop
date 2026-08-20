// ============================================
// LifecyclePanel - 请求生命周期面板（方案 F2）
// 2026-08-13
// 顶部分段耗时条 + 底部四格指标行（调度快照 / 计费输入 / 隐私扫描 / 重试）。
// 组件不持有调度器；拉取由 useRequestLifecycleDetail 负责。
// ============================================

import { useState } from 'react';
import { ChevronDown, ChevronUp, Shield, Zap } from 'lucide-react';
import { buildLifecycleSegments, lifecycleSegmentColors } from '../utils/lifecycle.js';
import {
  calculateTokensPerSecond,
  formatOptionalTimingBadge,
  formatTimingBadge,
  formatTpsBadge,
  resolveCompletionMs,
  resolveFirstResponseMs
} from '../utils/timing.js';

const EMPTY_CANDIDATES = [];

// 分段时间展示：<1s 用毫秒避免「0.0s」，≥1s 沿用秒格式。
const formatSegmentMs = (ms) => (ms < 1000 ? `${Math.round(ms)}ms` : formatTimingBadge(ms));

const LifecycleCell = ({ icon: Icon, iconClass, title, loading, children }) => (
  <div className="p-4 min-w-0">
    <div className="flex items-center gap-2 text-sm font-medium text-slate-500 mb-3">
      <Icon className={`w-4 h-4 ${iconClass}`} />
      <span>{title}</span>
    </div>
    {loading ? (
      <div className="space-y-2 animate-pulse">
        <div className="h-3 bg-slate-100 rounded w-3/4" />
        <div className="h-3 bg-slate-100 rounded w-1/2" />
      </div>
    ) : (
      children
    )}
  </div>
);

const DashText = () => <span className="text-sm text-slate-400">—</span>;

const EmptyCell = ({ icon: Icon, iconClass, title }) => (
  <LifecycleCell icon={Icon} iconClass={iconClass} title={title} loading={false}>
    <DashText />
  </LifecycleCell>
);

const LifecyclePanel = ({ request = {}, lifecycle = null, lifecycleLoading = false }) => {
  const [scheduleExpanded, setScheduleExpanded] = useState(false);
  const segments = buildLifecycleSegments(request);
  const totalMs = Math.max(request.duration || 0, 1);

  // 指标行（吸收原「耗时指标」行，含 TPS）。
  const firstResponseMs = resolveFirstResponseMs(request.firstTokenMs, request.duration, request.isStreaming);
  const completionMs = Number.isFinite(firstResponseMs)
    ? resolveCompletionMs(request.completionMs, request.duration, firstResponseMs, request.isStreaming)
    : null;
  const tokensPerSecond = calculateTokensPerSecond(request.outputTokens, completionMs);

  // 调度快照格
  const snapshot = lifecycle?.scheduleSnapshot || null;
  const isAccountRequest = request.upstreamType === 'account' || request.requestFamily === 'codex';
  const candidates = Array.isArray(snapshot?.candidates) ? snapshot.candidates : EMPTY_CANDIDATES;
  const selectedReason = snapshot ? snapshot.selected_tier_label || snapshot.selectedTierLabel || '' : '';
  const selectedName = snapshot ? snapshot.selected_account_name || snapshot.selectedAccountName || '' : '';

  const privacyScan = lifecycle?.privacyScan || null;
  const privacyActionLabel = {
    redact: '已脱敏',
    detect: '仅检测',
    blocked: '已拦截'
  }[privacyScan?.action];

  const runtimeOutcomeCount = candidates.filter((candidate) => {
    const outcome = candidate.runtime_outcome || candidate.runtimeOutcome || '';
    return outcome && outcome !== 'success';
  }).length;

  return (
    <div className="bg-white rounded-xl border border-slate-200 overflow-hidden">
      {/* 顶部分段耗时条 */}
      <div className="px-4 py-4 border-b border-slate-100 bg-slate-50/60">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-slate-900">生命周期</h3>
          <div className="inline-flex items-center gap-1.5 flex-wrap justify-end text-xs font-mono">
            {Number.isFinite(firstResponseMs) && (
              <span className="inline-flex items-center px-2 py-1 rounded bg-violet-50 text-violet-600 border border-violet-100">
                首字 {formatOptionalTimingBadge(firstResponseMs)}
              </span>
            )}
            {Number.isFinite(completionMs) && (
              <span className="inline-flex items-center px-2 py-1 rounded bg-emerald-50 text-emerald-600 border border-emerald-100">
                生成 {formatOptionalTimingBadge(completionMs)}
              </span>
            )}
            <span className="inline-flex items-center px-2 py-1 rounded bg-slate-50 text-slate-600 border border-slate-100">
              总耗 {formatTimingBadge(request.duration)}
            </span>
            {Number.isFinite(tokensPerSecond) && (
              <span className="inline-flex items-center px-2 py-1 rounded bg-slate-50 text-slate-600 border border-slate-100">
                TPS {formatTpsBadge(tokensPerSecond)}
              </span>
            )}
          </div>
        </div>

        <div className="flex items-stretch gap-1 h-7">
          {segments.map((segment) => {
            const pct = (segment.ms / totalMs) * 100;
            // 块宽按耗时比例伸展，但保证文字放得下（min-w-max）；
            // 占比过小的段只显示时间，名称见下方图例与 title。
            // 终态段 label 为短标签，原始错误全文经 detail 走 tooltip。
            const showLabel = pct >= 10;
            const tooltip = `${segment.label} ${formatSegmentMs(segment.ms)}${segment.detail ? `\n${segment.detail}` : ''}`;
            return (
              <div
                key={segment.key}
                className={`${lifecycleSegmentColors[segment.key] || 'bg-slate-200'} rounded-md flex items-center justify-center px-2 min-w-max transition-all`}
                style={{ flexGrow: Math.max(segment.ms, 1), flexBasis: 0 }}
                title={tooltip}
              >
                <span className="text-[11px] font-mono font-medium text-slate-700/90 whitespace-nowrap max-w-[220px] overflow-hidden text-ellipsis">
                  {showLabel ? `${segment.label} ${formatSegmentMs(segment.ms)}` : formatSegmentMs(segment.ms)}
                </span>
              </div>
            );
          })}
        </div>
        <div className="flex items-center flex-wrap gap-x-4 gap-y-1 mt-2 text-[11px] text-slate-500">
          {segments.map((segment) => (
            <span
              key={segment.key}
              className="inline-flex items-center gap-1.5 min-w-0"
              title={segment.detail || undefined}
            >
              <span className={`w-2.5 h-2.5 rounded-sm shrink-0 ${lifecycleSegmentColors[segment.key] || 'bg-slate-200'}`} />
              <span className="truncate max-w-[220px]">{segment.label}</span>
            </span>
          ))}
        </div>
      </div>

      {/* 底部四格指标行 */}
      <div className="grid grid-cols-2 divide-x divide-slate-100">
        {/* 调度快照 */}
        {lifecycleLoading ? (
          <LifecycleCell icon={Zap} iconClass="text-indigo-500" title="调度快照" loading>
            <span />
          </LifecycleCell>
        ) : isAccountRequest && snapshot ? (
          <LifecycleCell icon={Zap} iconClass="text-indigo-500" title="调度快照" loading={false}>
            <div className="text-sm">
              <div className="font-medium text-slate-900 truncate">
                {selectedName || '—'}
                {selectedReason ? <span className="text-slate-500 ml-1">· {selectedReason}</span> : null}
              </div>
              <button
                type="button"
                onClick={() => setScheduleExpanded((value) => !value)}
                className="mt-2 inline-flex items-center gap-1 text-xs text-indigo-600 hover:text-indigo-700"
              >
                {scheduleExpanded ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                候选决策（{candidates.length}）
              </button>
            </div>
            {scheduleExpanded && (
              <div className="mt-2 max-h-48 overflow-y-auto rounded-lg border border-slate-100 divide-y divide-slate-50">
                {candidates.length === 0 && (
                  <div className="px-2 py-1.5 text-xs text-slate-400">无候选</div>
                )}
                {candidates.map((candidate, index) => {
                  const outcome = candidate.runtime_outcome || candidate.runtimeOutcome || '';
                  return (
                    <div key={`${candidate.account_id || candidate.accountId || index}-${index}`} className="px-2 py-1.5 flex items-center justify-between gap-2">
                      <span className="text-xs text-slate-700 truncate">
                        {candidate.account_name || candidate.accountName || candidate.account_id || '—'}
                      </span>
                      <span className="text-[11px] text-slate-400 shrink-0">
                        {outcome || candidate.decision || candidate.reason || ''}
                      </span>
                    </div>
                  );
                })}
              </div>
            )}
          </LifecycleCell>
        ) : isAccountRequest ? (
          <EmptyCell icon={Zap} iconClass="text-indigo-500" title="调度快照" />
        ) : (
          <LifecycleCell icon={Zap} iconClass="text-indigo-500" title="调度快照" loading={false}>
            <div className="text-sm text-slate-700">
              <div>路由 {request.routeMode === 'manual_fixed' ? '严格固定' : request.routeMode === 'manual_preferred' ? '手动优选' : '自动'}</div>
              <div className="text-xs text-slate-500 mt-0.5">
                {request.effectiveEndpoint || request.upstreamName || '—'}
                {request.fallbackReason ? <span className="text-amber-600"> · 回退</span> : null}
              </div>
            </div>
          </LifecycleCell>
        )}

        {/* 计费输入 */}
        <LifecycleCell icon={Zap} iconClass="text-amber-500" title="计费输入" loading={false}>
          {isAccountRequest ? (
            <div className="font-mono text-lg text-slate-900">
              {Math.max((request.inputTokens || 0) - (request.cacheReadTokens || 0), 0).toLocaleString()}
              <span className="text-xs text-slate-400 ml-1">input − cache_read</span>
            </div>
          ) : (
            <div className="font-mono text-lg text-slate-900">
              {(request.inputTokens || 0).toLocaleString()}
              <span className="text-xs text-slate-400 ml-1">
                {request.cacheReadTokens ? `含缓存读 ${request.cacheReadTokens.toLocaleString()}` : '无缓存读'}
              </span>
            </div>
          )}
        </LifecycleCell>

        {/* 隐私扫描 */}
        {lifecycleLoading ? (
          <LifecycleCell icon={Shield} iconClass="text-emerald-500" title="隐私扫描" loading>
            <span />
          </LifecycleCell>
        ) : privacyScan && privacyActionLabel ? (
          <LifecycleCell icon={Shield} iconClass="text-emerald-500" title="隐私扫描" loading={false}>
            <div className="text-sm text-slate-700">
              <span className={privacyScan.action === 'blocked' ? 'text-rose-600' : 'text-emerald-600'}>
                {privacyActionLabel} · {privacyScan.hit_count ?? 0} 规则命中
              </span>
              {Number(privacyScan.scan_count) > 1 && (
                <span className="text-xs text-slate-400 ml-1">共扫描 {privacyScan.scan_count} 次</span>
              )}
              {privacyScan.blocked_code && (
                <div className="text-xs text-rose-500 mt-0.5">{privacyScan.blocked_code}</div>
              )}
            </div>
          </LifecycleCell>
        ) : (
          <EmptyCell icon={Shield} iconClass="text-emerald-500" title="隐私扫描" />
        )}

        {/* 重试 / 故障转移 */}
        <LifecycleCell icon={Zap} iconClass="text-rose-500" title="重试" loading={false}>
          <div className="text-sm text-slate-700">
            <span className="font-mono text-lg text-slate-900 mr-1">{request.retryCount ?? 0}</span>
            次
            {isAccountRequest && runtimeOutcomeCount > 0 && (
              <span className="text-xs text-slate-500 ml-1">换号 {runtimeOutcomeCount} 次</span>
            )}
            {!isAccountRequest && request.requestedEndpoint && request.effectiveEndpoint
              && request.requestedEndpoint !== request.effectiveEndpoint && (
              <span className="text-xs text-amber-600 ml-1">故障转移</span>
            )}
          </div>
        </LifecycleCell>
      </div>
    </div>
  );
};

export default LifecyclePanel;
