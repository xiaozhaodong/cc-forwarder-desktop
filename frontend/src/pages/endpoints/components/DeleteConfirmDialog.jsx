// ============================================
// DeleteConfirmDialog - 删除确认对话框
// ============================================

import { useRef } from 'react';
import { createPortal } from 'react-dom';
import { AlertTriangle, Trash2 } from 'lucide-react';
import { Button } from '@components/ui';
import useModalLifecycle from '@hooks/useModalLifecycle.js';

const DeleteConfirmDialog = ({ endpoint, onConfirm, onCancel, loading }) => {
  // 挂载即打开（父组件条件渲染），卸载时由 hook 清理
  const dialogRef = useRef(null);
  useModalLifecycle({
    open: true,
    onClose: () => {
      if (!loading) onCancel();
    },
    initialFocusRef: dialogRef
  });

  return createPortal(
    <div className="fixed inset-0 z-[60] flex items-start justify-center px-4 pt-[20vh] animate-fade-in">
      <div className="absolute inset-0 bg-slate-900/40" />
      <div
        ref={dialogRef}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-label="确认删除端点"
        className="relative bg-white rounded-2xl border border-slate-200 shadow-2xl w-full max-w-md p-6 focus:outline-none"
      >
        <div className="flex items-center gap-3 mb-4">
          <div className="p-3 bg-rose-100 rounded-full">
            <AlertTriangle className="text-rose-600" size={24} />
          </div>
          <div>
            <h3 className="text-lg font-semibold text-slate-900">确认删除</h3>
            <p className="text-sm text-slate-500">此操作不可撤销</p>
          </div>
        </div>

        <p className="text-slate-700 mb-6">
          确定要删除端点 <span className="font-semibold">&quot;{endpoint?.name}&quot;</span> 吗？
          删除后将无法恢复。
        </p>

        <div className="flex justify-end gap-3">
          <Button variant="ghost" onClick={onCancel} disabled={loading}>
            取消
          </Button>
          <Button
            variant="danger"
            icon={Trash2}
            onClick={onConfirm}
            loading={loading}
          >
            确认删除
          </Button>
        </div>
      </div>
    </div>,
    document.body
  );
};

export default DeleteConfirmDialog;
