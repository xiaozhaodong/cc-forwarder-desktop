// ============================================
// SettingsSection - 设置分类区块组件
// v5.1.0 (2025-12-08)
// ============================================

import { RotateCcw } from 'lucide-react';

const SettingsSection = ({
  title,
  icon: Icon,
  description,
  children,
  onReset,
  resetDisabled = false
}) => (
  <div className="bg-surface rounded-2xl border border-line shadow-sm overflow-hidden">
    <div className="px-6 py-4 bg-surface-sub border-b border-line-soft flex items-center justify-between">
      <div className="flex items-center">
        {Icon && (
          <div className="p-1.5 bg-accent-soft rounded-lg mr-3">
            <Icon size={16} className="text-accent" />
          </div>
        )}
        <div>
          <h3 className="font-semibold text-fg">{title}</h3>
          {description && (
            <p className="text-xs text-fg-subtle mt-0.5">{description}</p>
          )}
        </div>
      </div>
      {onReset && (
        <button
          onClick={onReset}
          disabled={resetDisabled}
          className={`
            inline-flex items-center px-2.5 py-1.5 text-xs font-medium
            text-fg-muted hover:text-fg-body hover:bg-surface-mut
            rounded-lg transition-colors
            ${resetDisabled ? 'opacity-50 cursor-not-allowed' : ''}
          `}
          title="重置为默认值"
        >
          <RotateCcw size={14} className="mr-1" />
          重置
        </button>
      )}
    </div>
    <div className="p-6">
      {children}
    </div>
  </div>
);

export default SettingsSection;
