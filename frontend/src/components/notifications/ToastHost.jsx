import { createPortal } from 'react-dom';
import {
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  Info,
  ShieldAlert,
  X
} from 'lucide-react';

const VARIANT_STYLES = {
  info: {
    shell: 'border-slate-200/80 bg-white/95 text-slate-900',
    accent: 'bg-slate-500',
    iconShell: 'bg-slate-100 text-slate-600',
    Icon: Info
  },
  success: {
    shell: 'border-emerald-200/80 bg-emerald-50/95 text-emerald-950',
    accent: 'bg-emerald-500',
    iconShell: 'bg-emerald-100 text-emerald-700',
    Icon: CheckCircle2
  },
  warning: {
    shell: 'border-amber-200/90 bg-amber-50/95 text-amber-950',
    accent: 'bg-amber-500',
    iconShell: 'bg-amber-100 text-amber-700',
    Icon: AlertTriangle
  },
  error: {
    shell: 'border-rose-200/90 bg-rose-50/95 text-rose-950',
    accent: 'bg-rose-500',
    iconShell: 'bg-rose-100 text-rose-700',
    Icon: ShieldAlert
  }
};

const laneLabel = (lane) => (lane === 'codex' ? 'Codex 账号' : 'CC 端点');

const ToastHost = ({ toasts = [], pendingCount = 0, onDismiss }) => {
  if (typeof document === 'undefined' || (toasts.length === 0 && pendingCount === 0)) return null;

  return createPortal(
    <div
      className="pointer-events-none fixed right-4 top-24 z-[120] flex w-[min(100vw-2rem,26rem)] flex-col gap-3"
      aria-live="polite"
      aria-label="全局通知"
    >
      <style>{`
        @keyframes switchboard-toast-in {
          from { opacity: 0; transform: translate3d(18px, -8px, 0) scale(.98); }
          to { opacity: 1; transform: translate3d(0, 0, 0) scale(1); }
        }
        @media (prefers-reduced-motion: reduce) {
          .switchboard-toast-motion { animation: none !important; }
        }
      `}</style>
      {toasts.map((toast) => {
        const variant = VARIANT_STYLES[toast.level] || VARIANT_STYLES.info;
        const Icon = variant.Icon;
        return (
          <article
            key={toast.id}
            data-testid="global-toast"
            role={toast.level === 'error' || toast.kind === 'failover' ? 'alert' : 'status'}
            className={`switchboard-toast-motion pointer-events-auto relative overflow-hidden rounded-2xl border shadow-[0_18px_50px_rgba(15,23,42,0.16)] backdrop-blur-xl ${variant.shell}`}
            style={{ animation: 'switchboard-toast-in 280ms cubic-bezier(.22,1,.36,1)' }}
          >
            <div className={`absolute inset-y-0 left-0 w-1 ${variant.accent}`} />
            <div className="flex gap-3 px-4 py-3.5 pl-5">
              <div className={`mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-xl ${variant.iconShell}`}>
                <Icon size={17} strokeWidth={2.2} />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-start gap-2">
                  <p className="min-w-0 flex-1 text-sm font-semibold leading-5">{toast.title}</p>
                  <button
                    type="button"
                    className="-mr-1 -mt-1 rounded-lg p-1 text-current/45 transition-colors hover:bg-black/5 hover:text-current/80"
                    onClick={() => onDismiss?.(toast.id)}
                    aria-label="关闭通知"
                  >
                    <X size={15} />
                  </button>
                </div>
                {toast.kind === 'failover' && toast.from && toast.to && (
                  <div className="mt-1.5 flex min-w-0 items-center gap-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-current/60">
                    <span className="shrink-0 rounded-md bg-black/5 px-1.5 py-0.5 normal-case tracking-normal">
                      {laneLabel(toast.lane)}
                    </span>
                    <span className="min-w-0 truncate" title={toast.from}>{toast.from}</span>
                    <ArrowRight size={12} className="shrink-0 opacity-60" />
                    <span className="min-w-0 truncate" title={toast.to}>{toast.to}</span>
                  </div>
                )}
                <p className="mt-1.5 break-words text-xs leading-5 text-current/75">{toast.message}</p>
              </div>
            </div>
          </article>
        );
      })}
      {pendingCount > 0 && (
        <div
          className="pointer-events-auto self-end rounded-full border border-slate-200/80 bg-white/90 px-3 py-1.5 text-[11px] font-medium text-slate-600 shadow-sm backdrop-blur-xl"
          role="status"
        >
          还有 {pendingCount} 条通知排队，将按发生顺序显示
        </div>
      )}
    </div>,
    document.body
  );
};

export default ToastHost;
