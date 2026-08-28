// ============================================
// 视图切换(表格 / 网格)分段控件
// 端点页与账号池共用,选择持久化在各页面自己的 localStorage key 里
// ============================================

import { LayoutGrid, LayoutList } from 'lucide-react';

const ViewModeSwitcher = ({ value = 'table', onChange, compact = false }) => {
  const options = [
    { key: 'table', label: '表格', icon: LayoutList },
    { key: 'grid', label: '网格', icon: LayoutGrid }
  ];
  return (
    <div role="group" aria-label="视图切换" className="inline-flex items-center gap-0.5 rounded-lg border border-line bg-surface-mut/80 p-0.5">
      {options.map(({ key, label, icon: Icon }) => (
        <button
          key={key}
          type="button"
          onClick={() => onChange?.(key)}
          aria-pressed={value === key}
          title={key === 'table' ? '表格视图' : '网格卡片视图'}
          className={`inline-flex items-center rounded-md text-xs font-medium transition ${
            compact ? 'h-7 w-7 justify-center' : 'gap-1 px-2.5 py-1'
          } ${
            value === key ? 'bg-surface text-accent shadow-sm' : 'text-fg-muted hover:text-fg-body'
          }`}
        >
          <Icon size={13} />
          <span className={compact ? 'sr-only' : ''}>{label}</span>
        </button>
      ))}
    </div>
  );
};

export default ViewModeSwitcher;
