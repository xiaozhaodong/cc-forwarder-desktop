// ============================================
// Account Pool 共享表单组件
// 2026-03-07
// ============================================

import { Button } from '@components/ui';

const FormField = ({ label, required = false, children }) => (
  <label className="block">
    <div className="text-xs font-medium text-slate-600 mb-1.5">
      {label}
      {required && <span className="text-rose-500 ml-1">*</span>}
    </div>
    {children}
  </label>
);

const Modal = ({
  open,
  title,
  submitText,
  submitVariant = 'primary',
  cancelText = '取消',
  onClose,
  onSubmit,
  submitting,
  children
}) => {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center px-4">
      <div className="absolute inset-0 bg-slate-900/40" onClick={() => !submitting && onClose()} />
      <form
        onSubmit={onSubmit}
        className="relative w-full max-w-xl bg-white rounded-2xl border border-slate-200 shadow-2xl"
      >
        <div className="px-6 py-4 border-b border-slate-100 flex items-center justify-between">
          <h3 className="text-lg font-semibold text-slate-900">{title}</h3>
          <button
            type="button"
            className="text-slate-400 hover:text-slate-600 text-sm"
            onClick={onClose}
            disabled={submitting}
          >
            关闭
          </button>
        </div>

        <div className="px-6 py-5 space-y-4 max-h-[70vh] overflow-y-auto">
          {children}
        </div>

        <div className="px-6 py-4 border-t border-slate-100 flex items-center justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose} disabled={submitting}>
            {cancelText}
          </Button>
          <Button type="submit" variant={submitVariant} loading={submitting}>
            {submitText}
          </Button>
        </div>
      </form>
    </div>
  );
};

export { FormField, Modal };
