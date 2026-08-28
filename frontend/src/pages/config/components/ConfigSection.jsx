// ============================================
// ConfigSection - 配置区块组件
// 2025-12-01
// ============================================

const ConfigSection = ({ title, icon: Icon, children }) => (
  <div className="bg-surface rounded-2xl border border-line shadow-sm overflow-hidden">
    <div className="px-6 py-4 bg-surface-sub border-b border-line-soft flex items-center">
      {Icon && <Icon size={18} className="text-accent mr-3" />}
      <h3 className="font-semibold text-fg">{title}</h3>
    </div>
    <div className="p-6">
      {children}
    </div>
  </div>
);

export default ConfigSection;
