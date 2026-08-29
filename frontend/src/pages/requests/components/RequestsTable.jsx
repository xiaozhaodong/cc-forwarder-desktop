// ============================================
// RequestsTable - 请求列表表格（支持动态列配置）
// 2025-12-01 10:31:09
// 更新：添加双击查看详情功能
// ============================================

import React from 'react';
import { createPortal } from 'react-dom';
import { Waves, RefreshCw } from 'lucide-react';
import { LoadingSpinner } from '@components/ui';
import { formatCost } from '@utils/api.js';
import { useTimezone } from '@contexts/TimezoneContext.jsx';
import useCountUp from '@hooks/useCountUp.js';
import RequestStatusBadge from './RequestStatusBadge.jsx';
import ModelTag from './ModelTag.jsx';
import Pagination from './Pagination.jsx';
import { copyTextToClipboard } from './clipboard.js';
import { getRequestFamilyMeta } from '../utils/requestSource.js';
import {
  calculateTokensPerSecond,
  formatOptionalTimingBadge,
  formatTimingBadge,
  formatTpsBadge,
  getTimingPillClassName,
  resolveCompletionMs,
  resolveFirstResponseMs
} from '../utils/timing.js';

// 「进行中」的状态集合，与 constants.js 的 STATUS_OPTIONS 状态机对齐。
// suspended（已挂起）不算进行中：它在等人工干预，动效会造成误导。
const IN_FLIGHT_STATUSES = new Set(['pending', 'forwarding', 'processing', 'retry']);

// 默认值提到模块级，避免每次渲染新建 Map 破坏子组件的 memo。
const EMPTY_ENTER_ANIMATIONS = new Map();

// 列序号走 CSS 变量驱动逐列接力的 animation-delay。
// 预生成并冻结，避免每个单元格每次渲染都新建 style 对象。
const COL_INDEX_STYLES = Array.from({ length: 32 }, (_, index) =>
  Object.freeze({ '--col-index': index })
);

const RequestStreamIcon = ({ request }) => {
  const StreamIcon = request.isStreaming ? Waves : RefreshCw;
  const streamTitle = request.isStreaming ? '流式请求' : '常规请求';
  const iconColor = request.isStreaming ? 'text-info' : 'text-fg-subtle';

  return <StreamIcon className={`w-3 h-3 ${iconColor} flex-shrink-0`} title={streamTitle} />;
};

const TimingTooltip = ({ anchorRect, items }) => {
  if (!anchorRect) {
    return null;
  }

  const style = {
    left: `${anchorRect.left + anchorRect.width / 2}px`,
    top: `${anchorRect.bottom + 6}px`
  };

  return createPortal(
    <div
      className="pointer-events-none fixed z-[10001] -translate-x-1/2 rounded-lg border border-line bg-surface px-3 py-2 text-xs text-fg-body shadow-lg ring-1 ring-hairline"
      style={style}
    >
      <div className="grid grid-cols-[auto_auto] gap-x-3 gap-y-1">
        {items.map(item => (
          <React.Fragment key={item.label}>
            <span className="text-fg-subtle">{item.label}</span>
            <span className="text-right font-mono font-semibold text-fg-body">{item.value}</span>
          </React.Fragment>
        ))}
      </div>
    </div>,
    document.body
  );
};

const RequestTimingCell = ({ request }) => {
  const [tooltipAnchor, setTooltipAnchor] = React.useState(null);
  const firstResponseMs = resolveFirstResponseMs(request.firstTokenMs, request.duration, request.isStreaming);
  const hasFirstResponse = Number.isFinite(firstResponseMs);
  const completionMs = hasFirstResponse
    ? resolveCompletionMs(request.completionMs, request.duration, firstResponseMs, request.isStreaming)
    : null;
  const tokensPerSecond = calculateTokensPerSecond(request.outputTokens, completionMs);
  const timingTooltipItems = [
    { label: '总耗', value: formatTimingBadge(request.duration) },
    { label: 'TPS', value: formatTpsBadge(tokensPerSecond) }
  ];

  const showTooltip = (event) => {
    setTooltipAnchor(event.currentTarget.getBoundingClientRect());
  };

  const hideTooltip = () => {
    setTooltipAnchor(null);
  };

  return (
    <div
      className="inline-flex items-center gap-1.5"
      onMouseEnter={showTooltip}
      onMouseMove={showTooltip}
      onMouseLeave={hideTooltip}
      onFocus={showTooltip}
      onBlur={hideTooltip}
    >
      {hasFirstResponse && (
        <span
          className={`inline-flex items-center px-2 py-1 rounded text-xs font-mono font-medium border transition-all ${getTimingPillClassName('first', firstResponseMs)}`}
        >
          {formatTimingBadge(firstResponseMs)}
        </span>
      )}
      <span
        className={`inline-flex items-center px-2 py-1 rounded text-xs font-mono font-medium border transition-all ${getTimingPillClassName('duration', completionMs ?? request.duration)}`}
      >
        {formatOptionalTimingBadge(completionMs)}
      </span>
      <TimingTooltip anchorRect={tooltipAnchor} items={timingTooltipItems} />
    </div>
  );
};

/**
 * 渲染单元格内容
 */
const renderCell = (columnId, request, formatTimestamp) => {
  switch (columnId) {
    case 'requestId':
      return (
        <div className="flex items-center gap-1.5 text-accent font-mono text-xs group-hover:text-accent-fg transition-colors">
          <RequestStreamIcon request={request} />
          <span className="truncate">{request.requestId}</span>
        </div>
      );
    case 'timestamp':
      return <span className="text-xs text-fg-subtle">{formatTimestamp(request.timestamp)}</span>;
    case 'status':
      return <RequestStatusBadge status={request.status} />;
    case 'model':
      return <ModelTag model={request.model} compact />;
    case 'requestFamily': {
      const family = getRequestFamilyMeta(request.requestFamily);
      return <span className={`inline-flex rounded-md border px-2 py-1 text-[11px] font-semibold ${family.className}`}>{family.label}</span>;
    }
    case 'upstreamName':
      return <span className="block max-w-[190px] truncate text-xs text-fg-body" title={request.upstreamName}>{request.upstreamName || '未知上游'}</span>;
    case 'duration':
      return <RequestTimingCell request={request} />;
    case 'inputTokens':
      return <span className="text-fg-body text-right font-mono text-xs">{request.inputTokens}</span>;
    case 'outputTokens':
      return <span className="text-fg-body text-right font-mono text-xs">{request.outputTokens}</span>;
    case 'cacheCreationTokens':
      return <span className="text-fg-muted text-right font-mono text-xs">{request.cacheCreationTokens}</span>;
    case 'cacheReadTokens':
      return <span className="text-fg-muted text-right font-mono text-xs">{request.cacheReadTokens}</span>;
    case 'cost':
      return <span className="text-right font-mono text-warn font-medium text-xs">{formatCost(request.cost)}</span>;
    default:
      return null;
  }
};

/**
 * RequestRow - 单行请求数据（支持单击复制、双击查看详情）
 *
 * enterAnimation 只在挂载那一刻取值并固化：React 按 requestId 做 key diff，
 * 「新的 requestId」等价于「新挂载的 RequestRow」。固化之后，
 * 后续任何 re-render 都不会重播入场动画。
 */
const RequestRow = ({ request, visibleColumns, onCopyId, onDoubleClick, formatTimestamp, enterAnimation }) => {
  const clickCountRef = React.useRef(0);
  const clickTimerRef = React.useRef(null);
  const [enterState, setEnterState] = React.useState(() => enterAnimation);

  React.useEffect(() => () => {
    if (clickTimerRef.current) {
      clearTimeout(clickTimerRef.current);
      clickTimerRef.current = null;
    }
  }, []);

  // 入场动画跑完就摘掉 class：它只在挂载那一次有意义，留在 DOM 上没有价值，
  // 且一旦将来有规则中断 animation（例如给 hover 加 animation:none），
  // 中断恢复时会把入场高亮从头重播一次，看着像又来了一条新请求。
  const handleAnimationEnd = React.useCallback((event) => {
    // 只认行级动画：逐列接力的 request-cell-enter 会从 td 冒泡上来，
    // 且它比行动画先结束（最晚 ~252ms vs 800ms），提前摘 class 会截断接力。
    // 扫光是 infinite，本来就不触发 end。
    if (event.animationName === 'request-row-enter-cascade' || event.animationName === 'request-row-enter-flat') {
      setEnterState(null);
    }
  }, []);

  const handleRowClick = () => {
    const nextCount = clickCountRef.current + 1;
    clickCountRef.current = nextCount;

    if (clickTimerRef.current) {
      clearTimeout(clickTimerRef.current);
      clickTimerRef.current = null;
    }

    if (nextCount === 1) {
      clickTimerRef.current = window.setTimeout(() => {
        onCopyId?.(request.requestId);
        clickCountRef.current = 0;
        clickTimerRef.current = null;
      }, 250);
      return;
    }

    if (nextCount === 2) {
      onDoubleClick?.(request);
      clickCountRef.current = 0;
    }
  };

  const rowAnimationClass = [
    enterState ? (enterState.withCascade ? 'request-row-enter-cascade' : 'request-row-enter-flat') : '',
    IN_FLIGHT_STATUSES.has(request.status) ? 'request-row-inflight' : ''
  ].filter(Boolean).join(' ');

  return (
    <tr
      className={`hover:bg-surface-sub transition-colors group cursor-pointer ${rowAnimationClass}`}
      // 走 CSS 变量而非行内 animation-delay：复合规则里有两条动画，
      // 行内 delay 会同时套到扫光上，把它的起跑点冲掉。
      style={enterState?.delayMs ? { '--enter-delay': `${enterState.delayMs}ms` } : undefined}
      onClick={handleRowClick}
      onAnimationEnd={handleAnimationEnd}
    >
      {visibleColumns.map((colId, colIndex) => (
        <td
          key={colId}
          className={`px-3 py-3 ${colId === 'cost' || colId.includes('Tokens') ? 'text-right' : ''}`}
          style={COL_INDEX_STYLES[colIndex] ?? COL_INDEX_STYLES[COL_INDEX_STYLES.length - 1]}
        >
          {renderCell(colId, request, formatTimestamp)}
        </td>
      ))}
    </tr>
  );
};

// memo 生效的前提是 index.jsx 用 mergeRequestRows 保持了未变化行的对象引用。
// enterAnimation 已在挂载时固化，比较它没有意义，故排除在外。
const MemoizedRequestRow = React.memo(RequestRow, (prev, next) => (
  prev.request === next.request
  && prev.visibleColumns === next.visibleColumns
  && prev.onCopyId === next.onCopyId
  && prev.onDoubleClick === next.onDoubleClick
  && prev.formatTimestamp === next.formatTimestamp
));

/**
 * RequestsTable - 请求列表表格组件（支持动态列）
 * @param {Object} props
 * @param {Array} props.requests - 请求列表
 * @param {boolean} props.loading - 加载状态
 * @param {Object} props.pagination - 分页信息
 * @param {Function} props.onPageChange - 页码变更回调
 * @param {Function} props.onPageSizeChange - 每页条数变更回调
 * @param {Array} props.visibleColumns - 可见列ID数组
 * @param {Array} props.columnConfigs - 列配置数组
 * @param {Function} props.onRowDoubleClick - 双击行回调
 * @param {Map<string, {delayMs:number, withCascade:boolean}>} props.enterAnimations
 *        本批次新增行的入场编排，由 index.jsx 在拿到新旧两批数据时算好传入
 */
const RequestsTable = ({
  requests = [],
  loading = false,
  refreshing = false,
  enterAnimations = EMPTY_ENTER_ANIMATIONS,
  pagination = { page: 1, pageSize: 10, total: 0 },
  onPageChange,
  onPageSizeChange,
  visibleColumns = [],
  columnConfigs = [],
  onRowDoubleClick
}) => {
  const { formatTimestamp } = useTimezone();
  const animatedTotal = useCountUp(pagination.total);

  // 引用必须稳定，否则 MemoizedRequestRow 的比较恒为 false。
  const handleCopyId = React.useCallback(async (id) => {
    await copyTextToClipboard(id, '请求 ID');
  }, []);

  const visibleColumnConfigs = React.useMemo(
    () => columnConfigs.filter(col => visibleColumns.includes(col.id)),
    [columnConfigs, visibleColumns]
  );

  return (
    <div className="bg-surface rounded-xl shadow-sm border border-line overflow-hidden">
      {/* 刷新进度条：占位高度恒定，表格不因刷新而位移 */}
      <div className="h-0.5 overflow-hidden">
        {refreshing && <div className="indeterminate-bar h-full w-1/3 bg-accent-ring/70" />}
      </div>

      {/* 表头 */}
      <div className="px-4 py-4 border-b border-line-soft flex justify-between items-center bg-surface-sub">
        <div className="flex items-center gap-2">
          <h3 className="font-semibold text-fg">请求明细</h3>
          <span className="px-2 py-0.5 bg-surface-mut text-fg-muted text-xs rounded-full font-medium tabular-nums">
            共 {Math.round(animatedTotal).toLocaleString()} 条
          </span>
        </div>
        <span className="text-xs text-fg-subtle">单击复制 ID · 双击查看详情</span>
      </div>

      {/* 表格 */}
      {loading ? (
        <LoadingSpinner text="加载请求数据..." />
      ) : (
        <div className="overflow-x-auto min-h-[300px]">
          <table className="w-full text-left text-sm whitespace-nowrap">
            <thead className="bg-surface-sub text-fg-muted border-b border-line-soft">
              <tr>
                {visibleColumnConfigs.map(col => (
                  <th
                    key={col.id}
                    className={`px-3 py-3 font-medium text-xs uppercase tracking-wider ${
                      col.align === 'right' ? 'text-right' : ''
                    }`}
                  >
                    {col.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {requests.length === 0 ? (
                <tr>
                  <td colSpan={visibleColumnConfigs.length} className="px-5 py-12 text-center text-fg-muted">
                    暂无请求数据
                  </td>
                </tr>
              ) : (
                requests.map((req) => (
                  <MemoizedRequestRow
                    key={req.requestId}
                    request={req}
                    visibleColumns={visibleColumns}
                    onCopyId={handleCopyId}
                    onDoubleClick={onRowDoubleClick}
                    formatTimestamp={formatTimestamp}
                    enterAnimation={enterAnimations.get(req.requestId) || null}
                  />
                ))
              )}
            </tbody>
          </table>
        </div>
      )}

      {/* 分页 */}
      <Pagination
        currentPage={pagination.page}
        pageSize={pagination.pageSize}
        total={pagination.total}
        onPageChange={onPageChange}
        onPageSizeChange={onPageSizeChange}
      />
    </div>
  );
};

export default RequestsTable;
