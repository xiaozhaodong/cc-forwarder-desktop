// ============================================
// 调度编排抽屉
// 2026-03-22
// ============================================

import { useRef } from 'react';
import { createPortal } from 'react-dom';
import { GitBranch, X } from 'lucide-react';
import { Button } from '@components/ui';
import useModalLifecycle from '@hooks/useModalLifecycle.js';
import Badge from './Badge.jsx';
import AccountPoolSchedulerSection from './AccountPoolSchedulerSection.jsx';
import LatestScheduleSnapshotCard from './LatestScheduleSnapshotCard.jsx';

const SchedulerDrawer = ({
  open = false,
  onClose,
  scheduler = {},
  latestScheduleSnapshot = {},
  snapshotUnsupported = false,
  onSwapGroup,
  onSetActiveAccount,
  onPinAccountSelection,
  onEnableAutoSelection,
  onViewInInventory,
  busyKey = ''
}) => {
  const closeButtonRef = useRef(null);

  useModalLifecycle({ open, onClose, initialFocusRef: closeButtonRef });

  if (!open) return null;

  const nav = document.querySelector('nav.sticky');
  const topOffset = nav ? nav.getBoundingClientRect().bottom : 0;
  const summary = scheduler?.summary || {};
  const degraded = Boolean(summary?.degraded);
  const isManualMode = summary?.selectionMode === 'manual';

  return createPortal(
    <div className="fixed inset-0 z-[45] flex justify-end" style={{ top: topOffset }}>
      <button
        type="button"
        aria-label="关闭调度编排"
        className="absolute inset-0 bg-overlay backdrop-blur-[2px]"
        onClick={onClose}
      />

      <aside
        role="dialog"
        aria-modal="true"
        aria-label="调度编排"
        className="relative z-10 flex h-full w-full max-w-[680px] flex-col border-l border-line bg-surface shadow-2xl animate-in slide-in-from-right duration-300"
      >
        <div className="border-b border-line-soft bg-gradient-to-r from-accent-soft via-surface to-surface px-6 py-5">
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-accent-soft text-accent shadow-sm">
                <GitBranch size={20} />
              </div>
              <div>
                <h2 className="text-lg font-semibold text-fg">调度编排</h2>
                <p className="mt-0.5 text-sm text-fg-muted">主组 / 备组 / 冷备的运行态与编排动作</p>
              </div>
            </div>
            <button
              type="button"
              ref={closeButtonRef}
              aria-label="关闭抽屉"
              onClick={onClose}
              className="rounded-lg border border-line bg-surface p-2 text-fg-subtle transition-colors hover:text-fg-body hover:border-line-strong"
            >
              <X size={16} />
            </button>
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-2">
            <Badge
              text={summary?.selectionModeLabel || 'Auto 模式'}
              className={isManualMode
                ? 'tone-indigo'
                : 'tone-slate'}
            />
            {isManualMode && summary?.pinnedAccountName ? (
              <Badge
                text={`全局手动：${summary.pinnedAccountName}`}
                className="bg-surface text-fg-body border-line"
              />
            ) : null}
            <Badge
              text={summary?.currentGroupLabel || '待确认'}
              className="bg-surface text-fg-body border-line"
            />
            <Badge
              text={`最近命中：${summary?.activeAccountName || '暂无'}`}
              className="tone-indigo"
            />
            <Badge
              text={degraded ? '已降级' : '未降级'}
              className={degraded
                ? 'tone-amber'
                : 'tone-emerald'}
            />
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant={isManualMode ? 'secondary' : 'primary'}
              size="sm"
              disabled={busyKey === 'scheduler-enable-auto' || !isManualMode}
              onClick={() => onEnableAutoSelection?.()}
            >
              启用编排
            </Button>
            <div className="text-xs text-fg-subtle">
              {isManualMode
                ? '当前为手动模式，可在下方账号行内重新指定手动账号，或切回按编排自动调度。'
                : '当前为 Auto 模式，可在下方任一账号行内指定手动账号。'}
            </div>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto">
          <div className="space-y-4 px-6 py-5">
            <AccountPoolSchedulerSection
              scheduler={scheduler}
              onSwapGroup={onSwapGroup}
              onSetActiveAccount={onSetActiveAccount}
              onPinAccountSelection={onPinAccountSelection}
              onViewInInventory={onViewInInventory}
              busyKey={busyKey}
              embedded
            />
          </div>

          <div className="border-t border-line-soft px-6 py-5">
            <LatestScheduleSnapshotCard
              snapshot={latestScheduleSnapshot}
              snapshotUnsupported={snapshotUnsupported}
            />
          </div>
        </div>
      </aside>
    </div>,
    document.body
  );
};

export default SchedulerDrawer;
