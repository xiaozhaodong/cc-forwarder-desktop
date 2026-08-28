// ============================================
// Account Pool 调度编排区块
// 2026-03-21
// ============================================

import { ArrowRight, GitBranch, ShieldAlert } from 'lucide-react';
import { Button } from '@components/ui';
import Badge from './Badge.jsx';
import GroupBoardCard from './GroupBoardCard.jsx';

const GROUP_ORDER = {
  primary: 0,
  backup: 1,
  cold: 2
};

const normalizeGroupKey = (key = '') => {
  const normalized = String(key || '').trim().toLowerCase();

  if (normalized === 'primary' || normalized.includes('main')) return 'primary';
  if (normalized === 'backup' || normalized.includes('secondary')) return 'backup';
  return 'cold';
};

const DEFAULT_GROUPS = [
  { key: 'primary', label: '主组', accounts: [], healthSummary: '主组当前没有候选账号', actions: [] },
  { key: 'backup', label: '备组', accounts: [], healthSummary: '备组等待承接降级流量', actions: [] },
  { key: 'cold', label: '冷备', accounts: [], healthSummary: '冷备保留为极端情况下的补位组', actions: [] }
];

const normalizeGroups = (groups = []) => {
  const mergedGroups = new Map(DEFAULT_GROUPS.map((group) => [group.key, group]));

  for (const group of Array.isArray(groups) ? groups : []) {
    const key = normalizeGroupKey(group?.key);
    mergedGroups.set(key, {
      ...mergedGroups.get(key),
      ...group,
      key,
      label: group?.label || mergedGroups.get(key)?.label || '调度组'
    });
  }

  return Array.from(mergedGroups.values()).sort((left, right) => GROUP_ORDER[left.key] - GROUP_ORDER[right.key]);
};

const AccountPoolSchedulerSection = ({
  scheduler = {},
  onSwapGroup,
  onSetActiveAccount,
  onPinAccountSelection,
  onViewInInventory,
  busyKey = '',
  onOpenSnapshot,
  embedded = false
}) => {
  const summary = scheduler?.summary || {};
  const latestSnapshot = scheduler?.latestSnapshot || {};
  const groups = normalizeGroups(scheduler?.groups);
  const degraded = Boolean(summary?.degraded || latestSnapshot?.degraded);

  const summaryCards = (
    <div className="grid gap-3 sm:grid-cols-3">
      <div className="rounded-xl border border-line bg-surface-sub px-4 py-3">
        <div className="text-xs text-fg-subtle">当前运行组</div>
        <div className="mt-1 text-sm font-semibold text-fg">{summary?.currentGroupLabel || '待确认'}</div>
      </div>
      <div className="rounded-xl border border-line bg-surface-sub px-4 py-3">
        <div className="text-xs text-fg-subtle">当前活跃账号</div>
        <div className="mt-1 text-sm font-semibold text-fg">{summary?.activeAccountName || '暂无'}</div>
      </div>
      <div className="rounded-xl border border-line bg-surface-sub px-4 py-3">
        <div className="text-xs text-fg-subtle">最新结论</div>
        <div className="mt-1 flex items-center gap-2 text-sm font-semibold text-fg">
          <span>{summary?.finalOutcomeLabel || '等待最近一次调度'}</span>
          <Badge
            text={degraded ? '已降级' : '未降级'}
            className={degraded
              ? 'tone-amber'
              : 'tone-emerald'}
          />
        </div>
      </div>
    </div>
  );

  const groupCards = (
    <div className={`grid gap-4 ${embedded ? '' : 'xl:grid-cols-3'}`}>
      {groups.map((group) => (
        <GroupBoardCard
          key={group.key}
          group={group}
          onSwapGroup={onSwapGroup}
          onSetActiveAccount={onSetActiveAccount}
          onPinAccountSelection={onPinAccountSelection}
          onViewInInventory={onViewInInventory}
          busyKey={busyKey}
        />
      ))}
    </div>
  );

  if (embedded) {
    return (
      <div className="space-y-4">
        {summaryCards}
        {groupCards}
      </div>
    );
  }

  return (
    <section className="rounded-2xl border border-line bg-surface shadow-sm overflow-hidden">
      <div className="border-b border-line-soft px-5 py-5">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div>
            <h2 className="text-xl font-semibold text-fg">调度编排</h2>
            <p className="mt-1 text-sm text-fg-muted">
              查看主组 / 备组 / 冷备的运行态与编排动作。
            </p>
          </div>
          {summaryCards}
        </div>
      </div>

      <div className="px-5 py-5">
        {groupCards}
      </div>

      <div className="border-t border-line-soft bg-surface-sub px-5 py-5">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex items-start gap-3">
            <div className="rounded-xl bg-surface p-2 text-accent shadow-sm ring-1 ring-line">
              {latestSnapshot?.finalError ? <ShieldAlert size={18} /> : <GitBranch size={18} />}
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-widest text-fg-subtle">最新调度摘要</div>
              <h3 className="mt-1 text-base font-semibold text-fg">最近一次调度</h3>
              <p className="mt-1 text-sm text-fg-muted">
                {latestSnapshot?.summary || '把完整调度结果收敛在底部，保留可观测性同时减少主视图占位。'}
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Badge text={latestSnapshot?.requestPath || '/v1/responses'} className="bg-surface text-fg-body border-line" />
            <Badge
              text={latestSnapshot?.selectedAccountName || '待命中账号'}
              className="tone-indigo"
            />
            <Badge
              text={Number.isFinite(latestSnapshot?.selectedPriority) ? `组内顺序 ${latestSnapshot.selectedPriority}` : '组内顺序待定'}
              className="tone-slate"
            />
          </div>
        </div>

        <div className="mt-4 flex items-center justify-end">
          <Button
            type="button"
            variant="secondary"
            className="justify-between"
            onClick={() => onOpenSnapshot?.(latestSnapshot)}
            disabled={!onOpenSnapshot}
          >
            查看完整候选链路
            <ArrowRight size={16} className="ml-2" />
          </Button>
        </div>
      </div>
    </section>
  );
};

export default AccountPoolSchedulerSection;
