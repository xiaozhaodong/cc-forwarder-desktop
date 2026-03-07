// ============================================
// 账号池账号列表区域
// 2026-03-07
// ============================================

import { Info, Plus, Users } from 'lucide-react';
import { Button, EmptyState } from '@components/ui';
import AccountRow from './AccountRow.jsx';
import Badge from './Badge.jsx';

const AccountListSection = ({
  accounts = [],
  accountCount = 0,
  busyKey = '',
  priorityTierSummary = [],
  priorityTierMetaMap,
  onCreate,
  onEdit,
  onRefreshProfile,
  onTest,
  onDelete,
  onToggle,
  onMoveTier
}) => (
  <section className="bg-white rounded-2xl border border-slate-200/70 shadow-sm overflow-hidden">
    <div className="px-5 py-4 border-b border-slate-100 space-y-3">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-2.5">
            <Users size={18} className="text-indigo-600" />
            <h2 className="text-base font-semibold text-slate-900">账号列表</h2>
            <Badge text="轻量调度" className="bg-sky-50 text-sky-700 border-sky-200" />
          </div>

          <div className="flex items-center gap-2 text-xs text-slate-500">
            <Info size={13} className="shrink-0 text-sky-600" />
            <p className="min-w-0 leading-5">
              priority 越小越优先，相同 priority 视为同一层；系统会先选第一个有可用账号的层，再在层内按额度与健康度自动排序。
            </p>
          </div>
        </div>

        <Button icon={Plus} size="sm" onClick={onCreate}>
          新增账号
        </Button>
      </div>

      {priorityTierSummary.length > 0 && (
        <div className="flex flex-wrap gap-2 md:pl-7">
          {priorityTierSummary.map((tier) => (
            <Badge
              key={`tier-summary-${tier.priority}`}
              text={`${tier.label} · P${tier.priority}${tier.count > 1 ? ` · ${tier.count} 个账号` : ''}`}
              className={tier.className}
              title={tier.description}
            />
          ))}
        </div>
      )}
    </div>

    {accountCount === 0 ? (
      <EmptyState
        icon={Users}
        title="暂无账号"
        description="新增账号后可直接用于 /v1/responses 调度。"
        action={(
          <Button icon={Plus} size="sm" onClick={onCreate}>
            添加账号
          </Button>
        )}
      />
    ) : (
      <div className="divide-y divide-slate-100">
        {accounts.map((account) => (
          <AccountRow
            key={String(account.id ?? account.ID ?? account.account_id ?? account.account_name ?? account.accountName)}
            account={account}
            accountCount={accountCount}
            busyKey={busyKey}
            priorityTierMetaMap={priorityTierMetaMap}
            onEdit={onEdit}
            onRefreshProfile={onRefreshProfile}
            onTest={onTest}
            onDelete={onDelete}
            onToggle={onToggle}
            onMoveTier={onMoveTier}
          />
        ))}
      </div>
    )}
  </section>
);

export default AccountListSection;
