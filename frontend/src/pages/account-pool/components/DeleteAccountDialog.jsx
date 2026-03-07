// ============================================
// 账号池删除确认弹窗
// 2026-03-07
// ============================================

import { resolveAccountId } from '../utils.js';
import { Modal } from './shared.jsx';

const DeleteAccountDialog = ({ deleteTarget, busyKey = '', onClose, onSubmit }) => {
  const deleteId = deleteTarget ? resolveAccountId(deleteTarget) : null;
  const submitting = deleteTarget ? busyKey === `account-delete-${deleteId}` : false;

  return (
    <Modal
      open={Boolean(deleteTarget)}
      title="确认删除账号"
      submitText="确认删除"
      submitVariant="danger"
      onClose={onClose}
      onSubmit={onSubmit}
      submitting={submitting}
    >
      <div className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700">
        此操作不可撤销。
      </div>
      <p className="text-sm text-slate-700">
        确认删除账号
        <span className="font-semibold mx-1">
          {`「${deleteTarget?.account_name || deleteTarget?.accountName || '-'}」`}
        </span>
        吗？
      </p>
    </Modal>
  );
};

export default DeleteAccountDialog;
