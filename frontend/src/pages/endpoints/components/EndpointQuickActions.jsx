import { Activity, Copy, Pencil, Trash2 } from 'lucide-react';

// 端点快捷操作(复制 URL / 测试 / 编辑 / 删除):表格行与网格卡片共用
const EndpointQuickActions = ({ endpoint, busy = false, onEdit, onDelete, onTest, className = '' }) => (
  <div className={`flex items-center gap-1 ${className}`}>
    <button type="button" onClick={() => navigator.clipboard.writeText(endpoint.url)} className="rounded-md p-1.5 text-fg-subtle hover:bg-surface-mut hover:text-accent" title="复制 URL"><Copy size={14} /></button>
    <button type="button" disabled={busy} onClick={() => onTest?.(endpoint.name)} className="rounded-md p-1.5 text-fg-subtle hover:bg-surface-mut hover:text-accent disabled:opacity-50" title="测试连通性"><Activity size={14} /></button>
    <button type="button" disabled={busy} onClick={() => onEdit?.(endpoint)} className="rounded-md p-1.5 text-fg-subtle hover:bg-surface-mut hover:text-accent disabled:opacity-50" title="编辑"><Pencil size={14} /></button>
    <button type="button" disabled={busy} onClick={() => onDelete?.(endpoint)} className="rounded-md p-1.5 text-fg-subtle hover:bg-danger-soft hover:text-danger disabled:opacity-50" title="删除"><Trash2 size={14} /></button>
  </div>
);

export default EndpointQuickActions;
