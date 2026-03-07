// ============================================
// Account Pool Badge
// 2026-03-07
// ============================================

const Badge = ({ text, className, title }) => (
  <span title={title} className={`inline-flex items-center px-2 py-0.5 text-xs rounded-full border ${className}`}>
    {text}
  </span>
);

export default Badge;
