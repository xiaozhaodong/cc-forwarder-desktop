// ============================================
// Account Pool 统计卡片
// 2026-03-07
// ============================================

const StatsCards = ({ accountCount = 0, activeAccountCount = 0, authFailedCount = 0 }) => (
  <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
    <div className="bg-surface rounded-xl border border-line p-4 shadow-sm">
      <div className="text-2xl font-bold text-fg">{accountCount}</div>
      <div className="text-sm text-fg-muted">账号总数</div>
    </div>
    <div className="bg-surface rounded-xl border border-success-line/70 p-4 shadow-sm">
      <div className="text-2xl font-bold text-success">{activeAccountCount}</div>
      <div className="text-sm text-success">可用账号</div>
    </div>
    <div className="bg-surface rounded-xl border border-danger-line/70 p-4 shadow-sm">
      <div className="text-2xl font-bold text-danger">{authFailedCount}</div>
      <div className="text-sm text-danger">鉴权失效</div>
    </div>
  </div>
);

export default StatsCards;
