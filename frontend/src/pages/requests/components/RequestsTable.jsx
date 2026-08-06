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

const RequestStreamIcon = ({ request }) => {
  const StreamIcon = request.isStreaming ? Waves : RefreshCw;
  const streamTitle = request.isStreaming ? '流式请求' : '常规请求';
  const iconColor = request.isStreaming ? 'text-blue-500' : 'text-slate-400';

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
      className="pointer-events-none fixed z-[10001] -translate-x-1/2 rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs text-slate-600 shadow-lg shadow-slate-900/10 ring-1 ring-black/5"
      style={style}
    >
      <div className="grid grid-cols-[auto_auto] gap-x-3 gap-y-1">
        {items.map(item => (
          <React.Fragment key={item.label}>
            <span className="text-slate-400">{item.label}</span>
            <span className="text-right font-mono font-semibold text-slate-700">{item.value}</span>
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
        <div className="flex items-center gap-1.5 text-blue-600 font-mono text-xs group-hover:text-indigo-600 transition-colors">
          <RequestStreamIcon request={request} />
          <span className="truncate">{request.requestId}</span>
        </div>
      );
    case 'timestamp':
      return <span className="text-xs text-gray-400">{formatTimestamp(request.timestamp)}</span>;
    case 'status':
      return <RequestStatusBadge status={request.status} />;
    case 'model':
      return <ModelTag model={request.model} compact />;
    case 'requestFamily': {
      const family = getRequestFamilyMeta(request.requestFamily);
      return <span className={`inline-flex rounded-md border px-2 py-1 text-[11px] font-semibold ${family.className}`}>{family.label}</span>;
    }
    case 'upstreamName':
      return <span className="block max-w-[190px] truncate text-xs text-gray-600" title={request.upstreamName}>{request.upstreamName || '未知上游'}</span>;
    case 'duration':
      return <RequestTimingCell request={request} />;
    case 'inputTokens':
      return <span className="text-gray-700 text-right font-mono text-xs">{request.inputTokens}</span>;
    case 'outputTokens':
      return <span className="text-gray-700 text-right font-mono text-xs">{request.outputTokens}</span>;
    case 'cacheCreationTokens':
      return <span className="text-gray-500 text-right font-mono text-xs">{request.cacheCreationTokens}</span>;
    case 'cacheReadTokens':
      return <span className="text-gray-500 text-right font-mono text-xs">{request.cacheReadTokens}</span>;
    case 'cost':
      return <span className="text-right font-mono text-orange-500 font-medium text-xs">{formatCost(request.cost)}</span>;
    default:
      return null;
  }
};

/**
 * RequestRow - 单行请求数据（支持单击复制、双击查看详情）
 */
const RequestRow = ({ request, visibleColumns, onCopyId, onDoubleClick, formatTimestamp }) => {
  const clickCountRef = React.useRef(0);
  const clickTimerRef = React.useRef(null);

  React.useEffect(() => () => {
    if (clickTimerRef.current) {
      clearTimeout(clickTimerRef.current);
      clickTimerRef.current = null;
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

  return (
    <tr
      className="hover:bg-gray-50/80 transition-colors group cursor-pointer"
      onClick={handleRowClick}
    >
      {visibleColumns.map(colId => (
        <td
          key={colId}
          className={`px-3 py-3 ${colId === 'cost' || colId.includes('Tokens') ? 'text-right' : ''}`}
        >
          {renderCell(colId, request, formatTimestamp)}
        </td>
      ))}
    </tr>
  );
};

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
 */
const RequestsTable = ({
  requests = [],
  loading = false,
  pagination = { page: 1, pageSize: 10, total: 0 },
  onPageChange,
  onPageSizeChange,
  visibleColumns = [],
  columnConfigs = [],
  onRowDoubleClick
}) => {
  const { formatTimestamp } = useTimezone();
  // 复制请求 ID
  const handleCopyId = async (id) => {
    await copyTextToClipboard(id, '请求 ID');
  };

  // 获取可见的列配置
  const visibleColumnConfigs = columnConfigs.filter(col => visibleColumns.includes(col.id));

  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
      {/* 表头 */}
      <div className="px-4 py-4 border-b border-gray-100 flex justify-between items-center bg-gray-50/30">
        <div className="flex items-center gap-2">
          <h3 className="font-semibold text-gray-800">请求明细</h3>
          <span className="px-2 py-0.5 bg-gray-100 text-gray-500 text-xs rounded-full font-medium">
            共 {pagination.total} 条
          </span>
        </div>
        <span className="text-xs text-gray-400">单击复制 ID · 双击查看详情</span>
      </div>

      {/* 表格 */}
      {loading ? (
        <LoadingSpinner text="加载请求数据..." />
      ) : (
        <div className="overflow-x-auto min-h-[300px]">
          <table className="w-full text-left text-sm whitespace-nowrap">
            <thead className="bg-gray-50/50 text-gray-500 border-b border-gray-100">
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
            <tbody className="divide-y divide-gray-100">
              {requests.length === 0 ? (
                <tr>
                  <td colSpan={visibleColumnConfigs.length} className="px-5 py-12 text-center text-slate-500">
                    暂无请求数据
                  </td>
                </tr>
              ) : (
                requests.map((req) => (
                  <RequestRow
                    key={req.requestId}
                    request={req}
                    visibleColumns={visibleColumns}
                    onCopyId={handleCopyId}
                    onDoubleClick={onRowDoubleClick}
                    formatTimestamp={formatTimestamp}
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
