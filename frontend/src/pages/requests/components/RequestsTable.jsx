// ============================================
// RequestsTable - 请求列表表格（支持动态列配置）
// 2025-12-01 10:31:09
// 更新：添加双击查看详情功能
// ============================================

import React from 'react';
import { Waves, RefreshCw } from 'lucide-react';
import { LoadingSpinner } from '@components/ui';
import { formatCost, formatTimestamp } from '@utils/api.js';
import RequestStatusBadge from './RequestStatusBadge.jsx';
import ModelTag from './ModelTag.jsx';
import Pagination from './Pagination.jsx';
import { copyTextToClipboard } from './clipboard.js';

const formatTimingBadge = (ms) => {
  const value = Number.isFinite(ms) ? Math.max(ms, 0) : 0;
  return `${(value / 1000).toFixed(1)}s`;
};

const getTimingPillClassName = (type, ms) => {
  if (type === 'first') {
    if (ms > 20000) return 'bg-rose-50 text-rose-600 border-rose-100';
    if (ms > 10000) return 'bg-amber-50 text-amber-600 border-amber-100';
    return 'bg-emerald-50 text-emerald-600 border-emerald-100';
  }

  if (ms > 40000) return 'bg-rose-50 text-rose-600 border-rose-100';
  if (ms > 20000) return 'bg-orange-50 text-orange-600 border-orange-100';
  return 'bg-emerald-50 text-emerald-600 border-emerald-100';
};

const RequestTimingCell = ({ request }) => {
  const hasFirstToken = request.isStreaming && Number.isFinite(request.firstTokenMs);

  return (
    <div className="flex items-center gap-1.5">
      {hasFirstToken && (
        <span className={`inline-flex items-center px-2 py-1 rounded text-xs font-mono font-medium border transition-all ${getTimingPillClassName('first', request.firstTokenMs)}`}>
          {formatTimingBadge(request.firstTokenMs)}
        </span>
      )}
      <span className={`inline-flex items-center px-2 py-1 rounded text-xs font-mono font-medium border transition-all ${getTimingPillClassName('duration', request.duration)}`}>
        {formatTimingBadge(request.duration)}
      </span>
    </div>
  );
};

/**
 * 渲染单元格内容
 */
const renderCell = (columnId, request) => {
  switch (columnId) {
    case 'requestId':
      const StreamIcon = request.isStreaming ? Waves : RefreshCw;
      const streamTitle = request.isStreaming ? '流式请求' : '常规请求';
      const iconColor = request.isStreaming ? 'text-blue-500' : 'text-slate-400';
      return (
        <div className="flex items-center gap-1.5 text-blue-600 font-mono text-xs group-hover:text-indigo-600 transition-colors">
          <StreamIcon className={`w-3 h-3 ${iconColor} flex-shrink-0`} title={streamTitle} />
          <span className="truncate">{request.requestId}</span>
        </div>
      );
    case 'timestamp':
      return <span className="text-xs text-gray-400">{formatTimestamp(request.timestamp)}</span>;
    case 'status':
      return <RequestStatusBadge status={request.status} />;
    case 'model':
      return <ModelTag model={request.model} />;
    case 'channel':
      return <span className="text-purple-600 text-xs font-medium">{request.channel || '-'}</span>;
    case 'endpoint':
      return <span className="text-gray-600 text-xs">{request.endpoint}</span>;
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
const RequestRow = ({ request, visibleColumns, onCopyId, onDoubleClick }) => {
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
          {renderCell(colId, request)}
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
