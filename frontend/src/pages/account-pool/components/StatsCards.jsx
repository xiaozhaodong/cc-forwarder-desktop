// ============================================
// Account Pool 统计卡片
// 2026-03-07
// ============================================

const StatsCards = ({ accountCount = 0, activeAccountCount = 0, authFailedCount = 0 }) => (
  <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
    <div className="bg-white rounded-xl border border-slate-200/60 p-4 shadow-sm">
      <div className="text-2xl font-bold text-slate-900">{accountCount}</div>
      <div className="text-sm text-slate-500">账号总数</div>
    </div>
    <div className="bg-white rounded-xl border border-emerald-200/70 p-4 shadow-sm">
      <div className="text-2xl font-bold text-emerald-700">{activeAccountCount}</div>
      <div className="text-sm text-emerald-600">可用账号</div>
    </div>
    <div className="bg-white rounded-xl border border-rose-200/70 p-4 shadow-sm">
      <div className="text-2xl font-bold text-rose-700">{authFailedCount}</div>
      <div className="text-sm text-rose-600">鉴权失效</div>
    </div>
  </div>
);

export default StatsCards;
