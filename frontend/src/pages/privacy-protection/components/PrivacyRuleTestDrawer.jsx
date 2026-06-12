// ============================================
// 隐私规则测试抽屉
// 2026-06-12
// ============================================

import { useEffect } from 'react';
import { createPortal } from 'react-dom';
import { FlaskConical, X } from 'lucide-react';
import { lockAppScroll } from '@utils/scrollLock.js';
import PrivacyRuleTestPanel from './PrivacyRuleTestPanel.jsx';

const PrivacyRuleTestDrawer = ({ open, onClose, onTest }) => {
  useEffect(() => {
    if (!open) return undefined;
    return lockAppScroll();
  }, [open]);

  useEffect(() => {
    if (!open) return undefined;

    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        onClose?.();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [open, onClose]);

  if (!open) return null;

  const nav = document.querySelector('nav.sticky');
  const topOffset = nav ? nav.getBoundingClientRect().bottom : 0;

  return createPortal(
    <div className="fixed inset-0 z-[46] flex justify-end" style={{ top: topOffset }}>
      <button
        type="button"
        aria-label="关闭规则测试"
        className="absolute inset-0 bg-slate-950/30 backdrop-blur-[2px] animate-fade-in"
        onClick={onClose}
      />

      <aside className="relative z-10 flex h-full w-full max-w-[620px] flex-col border-l border-slate-200 bg-white shadow-2xl shadow-slate-950/20 animate-slide-in-right">
        <div className="border-b border-slate-100 bg-gradient-to-r from-indigo-50/80 via-white to-white px-6 py-5">
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-100 text-indigo-600 shadow-sm">
                <FlaskConical size={20} />
              </div>
              <div>
                <h2 className="text-lg font-semibold text-slate-900">规则测试</h2>
                <p className="mt-0.5 text-sm text-slate-500">测试文本不会被记录</p>
              </div>
            </div>
            <button
              type="button"
              aria-label="关闭抽屉"
              onClick={onClose}
              className="rounded-lg border border-slate-200 bg-white p-2 text-slate-400 transition-colors hover:border-slate-300 hover:text-slate-700"
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
