// ============================================
// 隐私规则测试抽屉
// 2026-06-12
// ============================================

import { useRef } from 'react';
import { createPortal } from 'react-dom';
import { FlaskConical, X } from 'lucide-react';
import useModalLifecycle from '@hooks/useModalLifecycle.js';
import PrivacyRuleTestPanel from './PrivacyRuleTestPanel.jsx';

const PrivacyRuleTestDrawer = ({ open, onClose, onTest }) => {
  const closeButtonRef = useRef(null);

  useModalLifecycle({ open, onClose, initialFocusRef: closeButtonRef });

  if (!open) return null;

  const nav = document.querySelector('nav.sticky');
  const topOffset = nav ? nav.getBoundingClientRect().bottom : 0;

  return createPortal(
    <div className="fixed inset-0 z-[46] flex justify-end" style={{ top: topOffset }}>
      <button
        type="button"
        aria-label="关闭规则测试"
        className="absolute inset-0 bg-overlay backdrop-blur-[2px] animate-fade-in"
        onClick={onClose}
      />

      <aside
        role="dialog"
        aria-modal="true"
        aria-label="规则测试"
        className="relative z-10 flex h-full w-full max-w-[620px] flex-col border-l border-line bg-surface shadow-2xl animate-slide-in-right"
      >
        <div className="border-b border-line-soft bg-gradient-to-r from-accent-soft via-surface to-surface px-6 py-5">
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-accent-soft text-accent shadow-sm">
                <FlaskConical size={20} />
              </div>
              <div>
                <h2 className="text-lg font-semibold text-fg">规则测试</h2>
                <p className="mt-0.5 text-sm text-fg-muted">测试文本不会被记录</p>
              </div>
            </div>
            <button
              type="button"
              ref={closeButtonRef}
              aria-label="关闭抽屉"
              onClick={onClose}
              className="rounded-lg border border-line bg-surface p-2 text-fg-subtle transition-colors hover:border-line-strong hover:text-fg-body"
            >
              <X size={16} />
            </button>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto px-6 py-5">
          <PrivacyRuleTestPanel onTest={onTest} variant="drawer" showHeader={false} />
        </div>
      </aside>
    </div>,
    document.body
  );
};

export default PrivacyRuleTestDrawer;
