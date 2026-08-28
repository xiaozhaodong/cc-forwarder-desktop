import { useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { AlertTriangle, Trash2 } from 'lucide-react';
import { Button } from '@components/ui';
import useModalLifecycle from '@hooks/useModalLifecycle.js';

const CLEAR_CONFIRM_TEXT = '清空本地敏感值';

const DialogFrame = ({ title, loading, onClose, children }) => {
  const dialogRef = useRef(null);
  const handleRequestClose = () => {
    if (!loading) onClose();
  };

  useModalLifecycle({
    open: true,
    onClose: handleRequestClose,
    initialFocusRef: dialogRef
  });

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-start justify-center px-4 pt-[20vh]">
      <div className="absolute inset-0 bg-overlay" onClick={handleRequestClose} />
      <div
        ref={dialogRef}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="relative w-full max-w-md rounded-2xl border border-line bg-surface p-6 shadow-2xl focus:outline-none"
      >
        {children}
      </div>
    </div>,
    document.body
  );
};

export const DeleteExactSecretDialog = ({ secret, loading, onConfirm, onClose }) => (
  <DialogFrame title="确认删除本地敏感值" loading={loading} onClose={onClose}>
    <div className="mb-4 flex items-center gap-3">
      <div className="rounded-full bg-danger-soft p-3">
        <AlertTriangle className="text-danger" size={24} />
      </div>
      <div>
        <h3 className="text-lg font-semibold text-fg">确认删除</h3>
        <p className="text-sm text-fg-muted">删除后将立即停止匹配，且无法恢复</p>
      </div>
    </div>

    <p className="mb-6 text-sm text-fg-body">
      确定删除本地敏感值
      <span className="mx-1 font-semibold">「{secret?.name || '-'}」</span>
      吗？
    </p>

    <div className="flex justify-end gap-2">
      <Button type="button" variant="ghost" onClick={onClose} disabled={loading}>
        取消
      </Button>
      <Button type="button" variant="danger" icon={Trash2} onClick={onConfirm} loading={loading}>
        确认删除
      </Button>
    </div>
  </DialogFrame>
);

export const ClearExactSecretsDialog = ({ loading, onConfirm, onClose }) => {
  const [confirmText, setConfirmText] = useState('');
  const confirmed = confirmText.trim() === CLEAR_CONFIRM_TEXT;

  const handleSubmit = (event) => {
    event.preventDefault();
    if (confirmed && !loading) onConfirm(confirmText.trim());
  };

  return (
    <DialogFrame title="确认清空本地敏感值" loading={loading} onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="mb-4 flex items-center gap-3">
          <div className="rounded-full bg-danger-soft p-3">
            <AlertTriangle className="text-danger" size={24} />
          </div>
          <div>
            <h3 className="text-lg font-semibold text-fg">确认清空</h3>
            <p className="text-sm text-fg-muted">所有本地敏感值都将被永久删除</p>
          </div>
        </div>

        <label className="mb-6 block text-sm text-fg-body">
          输入 <span className="font-semibold text-danger">{CLEAR_CONFIRM_TEXT}</span> 以确认：
          <input
            autoFocus
            autoComplete="off"
            spellCheck={false}
            value={confirmText}
            onChange={(event) => setConfirmText(event.target.value)}
            className="mt-2 w-full rounded-lg border border-line px-3 py-2 text-sm focus:border-danger-line focus:outline-none focus:ring-2 focus:ring-danger-line"
          />
        </label>

        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose} disabled={loading}>
            取消
          </Button>
          <Button type="submit" variant="danger" icon={Trash2} loading={loading} disabled={!confirmed}>
            确认清空
          </Button>
        </div>
      </form>
    </DialogFrame>
  );
};
