// ============================================
// 调度编排抽屉
// 2026-03-22
// ============================================

import { createPortal } from 'react-dom';
import { GitBranch, X } from 'lucide-react';
import { Button } from '@components/ui';
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
        className="absolute inset-0 bg-slate-950/30 backdrop-blur-[2px]"
        onClick={onClose}
      />

      <aside className="relative z-10 flex h-full w-full max-w-[680px] flex-col border-l border-slate-200 bg-white shadow-2xl shadow-slate-950/20 animate-in slide-in-from-right duration-300">
        <div className="border-b border-slate-100 bg-gradient-to-r from-indigo-50/80 via-white to-white px-6 py-5">
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-100 text-indigo-600 shadow-sm">
                <GitBranch size={20} />
              </div>
              <div>
                <h2 className="text-lg font-semibold text-slate-900">调度编排</h2>
                <p className="mt-0.5 text-sm text-slate-500">主组 / 备组 / 冷备的运行态与编排动作</p>
              </div>
            </div>
            <button
              type="button"
              aria-label="关闭抽屉"
              onClick={onClose}
              className="rounded-lg border border-slate-200 bg-white p-2 text-slate-400 transition-colors hover:text-slate-700 hover:border-slate-300"
            >
              <X size={16} />
            </button>
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-2">
            <Badge
              text={summary?.selectionModeLabel || 'Auto 模式'}
              className={isManualMode
                ? 'bg-indigo-50 text-indigo-700 border-indigo-200'
                : 'bg-slate-100 text-slate-600 border-slate-200'}
            />
            {isManualMode && summary?.pinnedAccountName ? (
              <Badge
                text={`全局手动：${summary.pinnedAccountName}`}
                className="bg-white text-slate-700 border-slate-200"
              />
            ) : null}
            <Badge
              text={summary?.currentGroupLabel || '待确认'}
              className="bg-white text-slate-700 border-slate-200"
            />
            <Badge
              text={`最近命中：${summary?.activeAccountName || '暂无'}`}
              className="bg-indigo-50 text-indigo-700 border-indigo-200"
            />
            <Badge
              text={degraded ? '已降级' : '未降级'}
              className={degraded
                ? 'bg-amber-50 text-amber-700 border-amber-200'
                : 'bg-emerald-50 text-emerald-700 border-emerald-200'}
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
            <div className="text-xs text-slate-400">
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

          <div className="border-t border-slate-100 px-6 py-5">
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
