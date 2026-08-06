// ============================================
// 账号资产网格视图
// 与 AccountInventoryTable 相同的 props 契约与数据口径
// 2026-08-06
// ============================================

import AccountInventoryCard from './AccountInventoryCard.jsx';

const AccountInventoryGrid = ({
  rows = [],
  busyKey = '',
  selectedRowIds = [],
  allVisibleSelected = false,
  selectedCount = 0,
  onToggleAllRows,
  onToggleRow,
  onRowClick,
  onToggleAccount,
  onTestAccount,
  onRefreshAccountProfile,
  onEditAccount,
  onDeleteAccount
}) => {
  const selectedIdSet = new Set(selectedRowIds.map((item) => String(item)));

  return (
    <div>
      <div className="flex items-center gap-2 border-b border-slate-100 px-5 py-2.5">
        <input
          type="checkbox"
          checked={allVisibleSelected}
          aria-label="选择全部可见账号"
          onChange={() => onToggleAllRows?.()}
          className="h-4 w-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
        />
        <span className="text-xs text-slate-400">
          选择本页全部 {rows.length} 个账号{selectedCount > 0 ? `(已选 ${selectedCount} 个)` : ''}
        </span>
      </div>
      <div className="grid gap-4 p-5 sm:grid-cols-2 xl:grid-cols-3">
        {rows.map((row) => (
          <AccountInventoryCard
            key={String(row.id)}
            row={row}
            busyKey={busyKey}
            selected={selectedIdSet.has(String(row.id))}
            onToggleRow={onToggleRow}
            onRowClick={onRowClick}
            onToggleAccount={onToggleAccount}
            onTestAccount={onTestAccount}
            onRefreshAccountProfile={onRefreshAccountProfile}
            onEditAccount={onEditAccount}
            onDeleteAccount={onDeleteAccount}
          />
        ))}
      </div>
    </div>
  );
};

export default AccountInventoryGrid;
