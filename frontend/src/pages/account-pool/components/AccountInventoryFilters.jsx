// ============================================
// 账号资产筛选条
// 2026-03-21
// ============================================

import { RotateCcw, Search, Sparkles, TestTube2, UserRoundCheck, Rows3 } from 'lucide-react';
import { Button, CustomSelect } from '@components/ui';

const BATCH_LABELS = {
  test: '批量测试',
  refresh: '批量刷新画像',
  enable: '批量启用',
  disable: '批量停用',
  primary: '移到主组',
  backup: '移到备组',
  cold: '移到冷备'
};

const findAction = (actions = [], ...keys) => actions.find((a) => keys.includes(a.key));
const findVariant = (action, key) => action?.variants?.find((v) => v.key === key);

const BatchActionButton = ({ icon: Icon, label, onClick, disabled, title, className = '' }) => (
  <button
    type="button"
    onClick={onClick}
    disabled={disabled}
    title={title}
    className={`inline-flex items-center rounded-lg border border-slate-200 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:border-slate-300 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40 ${className}`}
  >
    {Icon && <Icon size={13} className="mr-1.5" />}
    {label}
  </button>
);

const AccountInventoryFilters = ({
  searchTerm = '',
  onSearchTermChange,
  filterConfigs = [],
  filterValues = {},
  onFilterChange,
  onResetFilters,
  resultCount = 0,
  selectedCount = 0,
  batchActions = [],
  batchFeedback,
  onBatchAction
}) => {
  const testAction = findAction(batchActions, 'batch-test', 'test');
  const refreshAction = findAction(batchActions, 'batch-refresh-profile', 'refresh-profile');
  const toggleAction = findAction(batchActions, 'batch-toggle', 'toggle-enabled');
  const moveAction = findAction(batchActions, 'batch-move-tier', 'move-backup', 'move-cold-standby');
  const enableVariant = findVariant(toggleAction, 'enable');
  const disableVariant = findVariant(toggleAction, 'disable');
  const primaryVariant = findVariant(moveAction, 'primary');
  const backupVariant = findVariant(moveAction, 'backup');
  const coldVariant = findVariant(moveAction, 'cold');

  return (
    <div className="space-y-2.5 border-b border-slate-100 px-5 py-3">
      {/* 搜索 + 筛选条 + 统计（一行） */}
      <div className="flex items-center gap-2.5">
        <div className="relative w-64 shrink-0">
          <Search size={14} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-400" />
          <input
            id="account-inventory-search"
            type="search"
            aria-label="搜索账号"
            value={searchTerm}
            placeholder="搜索账号名 / 备注 / 标签"
            onChange={(event) => onSearchTermChange?.(event.target.value)}
            className="w-full rounded-lg border border-slate-200 bg-white py-1.5 pl-8 pr-3 text-xs text-slate-700 outline-none placeholder:text-slate-400 focus:border-indigo-300 focus:ring-2 focus:ring-indigo-100"
          />
        </div>

        <span className="h-5 w-px bg-slate-200" />

        <div className="flex flex-wrap items-center gap-2">
          {filterConfigs.map((config) => (
            <div key={config.key} className="inline-flex items-center gap-1.5">
              <span className="text-[11px] text-slate-400 whitespace-nowrap">{config.label}</span>
              <CustomSelect
                size="xs"
                options={config.options}
                value={filterValues[config.key]}
                onChange={(value) => onFilterChange?.(config.key, value)}
              />
            </div>
          ))}
        </div>

        <div className="ml-auto flex shrink-0 items-center gap-2 text-xs text-slate-400">
          <span className="tabular-nums">{resultCount} 个账号</span>
          <button
            type="button"
            onClick={onResetFilters}
            className="inline-flex items-center gap-1 rounded-md px-1.5 py-1 text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-600"
            title="重置筛选"
          >
            <RotateCcw size={12} />
          </button>
        </div>
      </div>

      {/* 批量操作 - 选中时显示 */}
      {selectedCount > 0 && (
        <div className="flex items-center gap-2 rounded-lg border border-indigo-100 bg-indigo-50/40 px-3 py-2">
          <span className="shrink-0 text-xs font-medium text-indigo-600">
            {selectedCount} 个已选
          </span>
          <span className="h-4 w-px bg-indigo-200" />
          <div className="flex flex-wrap items-center gap-1.5">
            <BatchActionButton
              icon={TestTube2}
              label={testAction?.label || BATCH_LABELS.test}
              onClick={() => onBatchAction?.(testAction || { key: 'batch-test', label: BATCH_LABELS.test })}
            />
            <BatchActionButton
              icon={Sparkles}
              label={refreshAction?.label || BATCH_LABELS.refresh}
              onClick={() => onBatchAction?.(refreshAction || { key: 'batch-refresh-profile', label: BATCH_LABELS.refresh })}
            />
            <BatchActionButton
              icon={UserRoundCheck}
              label={enableVariant?.label || BATCH_LABELS.enable}
              onClick={() => onBatchAction?.(toggleAction || { key: 'batch-toggle' }, enableVariant || { key: 'enable', label: BATCH_LABELS.enable })}
            />
            <BatchActionButton
              icon={Rows3}
              label={disableVariant?.label || BATCH_LABELS.disable}
              onClick={() => onBatchAction?.(toggleAction || { key: 'batch-toggle' }, disableVariant || { key: 'disable', label: BATCH_LABELS.disable })}
            />
            <span className="h-4 w-px bg-slate-200" />
            <BatchActionButton
              label={primaryVariant?.label || BATCH_LABELS.primary}
              onClick={() => onBatchAction?.(moveAction || { key: 'batch-move-tier' }, primaryVariant || { key: 'primary', label: BATCH_LABELS.primary })}
            />
            <BatchActionButton
              label={backupVariant?.label || BATCH_LABELS.backup}
              onClick={() => onBatchAction?.(moveAction || { key: 'batch-move-tier' }, backupVariant || { key: 'backup', label: BATCH_LABELS.backup })}
            />
            <BatchActionButton
              label={coldVariant?.label || BATCH_LABELS.cold}
              onClick={() => onBatchAction?.(moveAction || { key: 'batch-move-tier' }, coldVariant || { key: 'cold', label: BATCH_LABELS.cold })}
              disabled={Boolean(coldVariant?.disabled)}
              title={coldVariant?.soon ? '即将支持' : ''}
            />
          </div>
          {batchFeedback?.message && (
            <span className="ml-auto shrink-0 text-xs text-slate-500">{batchFeedback.message}</span>
          )}
        </div>
      )}
    </div>
  );
};

export default AccountInventoryFilters;
