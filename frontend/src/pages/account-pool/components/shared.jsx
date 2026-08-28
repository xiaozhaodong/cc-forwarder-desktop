// ============================================
// Account Pool 共享表单组件
// 2026-03-07
// ============================================

import { useRef } from 'react';
import { Button } from '@components/ui';
import useModalLifecycle from '@hooks/useModalLifecycle.js';

const FormField = ({ label, required = false, children }) => (
  <label className="block">
    <div className="text-xs font-medium text-fg-body mb-1.5">
      {label}
      {required && <span className="text-danger ml-1">*</span>}
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
  const closeButtonRef = useRef(null);
  const handleRequestClose = () => {
    if (!submitting) onClose();
  };

  useModalLifecycle({
    open,
    onClose: handleRequestClose,
    initialFocusRef: closeButtonRef
  });

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center px-4">
      <div className="absolute inset-0 bg-overlay" onClick={handleRequestClose} />
      <form
        onSubmit={onSubmit}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="relative w-full max-w-xl bg-surface rounded-2xl border border-line shadow-2xl"
      >
        <div className="px-6 py-4 border-b border-line-soft flex items-center justify-between">
          <h3 className="text-lg font-semibold text-fg">{title}</h3>
          <button
            type="button"
            ref={closeButtonRef}
            className="text-fg-subtle hover:text-fg-body text-sm"
            onClick={handleRequestClose}
            disabled={submitting}
          >
            关闭
          </button>
        </div>

        <div className="px-6 py-5 space-y-4 max-h-[70vh] overflow-y-auto">
          {children}
        </div>

        <div className="px-6 py-4 border-t border-line-soft flex items-center justify-end gap-2">
          <Button type="button" variant="secondary" onClick={handleRequestClose} disabled={submitting}>
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
