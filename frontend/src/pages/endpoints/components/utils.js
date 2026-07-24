// ============================================
// Endpoints 页面工具函数
// ============================================

/**
 * 按渠道分组端点
 * @param {Array} endpoints - 端点列表
 * @returns {Array} 分组后的端点列表 [{ channel, endpoints }, ...]
 */
export const groupEndpointsByChannel = (endpoints) => {
  const groups = {};
  endpoints.forEach(ep => {
    const channel = ep.channel || ep.group || '未分组';
    if (!groups[channel]) groups[channel] = [];
    groups[channel].push(ep);
  });

  // 先对每组内的端点按优先级排序
  Object.keys(groups).forEach(channel => {
    groups[channel].sort((a, b) => (a.priority || 99) - (b.priority || 99));
  });

  // 按渠道内最高优先级（最小数字）排序
  return Object.entries(groups)
    .sort(([, epsA], [, epsB]) => {
      const minPriorityA = Math.min(...epsA.map(e => e.priority || 99));
      const minPriorityB = Math.min(...epsB.map(e => e.priority || 99));
      return minPriorityA - minPriorityB;
    })
    .map(([channel, eps]) => ({
      channel,
      endpoints: eps
    }));
};

// 端点调度快照结果标签（2026-07-24 随调度快照抽屉迁入）
const scheduleOutcomeMeta = {
  pending: ['调度中', 'bg-blue-50 text-blue-700 border-blue-200'],
  success: ['成功', 'bg-emerald-50 text-emerald-700 border-emerald-200'],
  quality_incomplete: ['响应不完整', 'bg-amber-50 text-amber-700 border-amber-200'],
  failed_after_commit: ['响应阶段失败', 'bg-rose-50 text-rose-700 border-rose-200'],
  cancelled: ['已取消', 'bg-slate-100 text-slate-600 border-slate-200'],
  privacy_blocked: ['隐私策略拦截', 'bg-rose-50 text-rose-700 border-rose-200'],
  passthrough_error: ['上游失败', 'bg-rose-50 text-rose-700 border-rose-200'],
  passthrough_raw: ['原样透传', 'bg-amber-50 text-amber-700 border-amber-200'],
  no_candidates: ['无可用候选', 'bg-rose-50 text-rose-700 border-rose-200'],
  manual_fixed_blocked: ['固定端点不可用', 'bg-rose-50 text-rose-700 border-rose-200'],
  all_candidates_failed: ['候选全部失败', 'bg-rose-50 text-rose-700 border-rose-200'],
  rejected_by_failure_tracker: ['失败阈值拒绝', 'bg-rose-50 text-rose-700 border-rose-200']
};

/**
 * 解析调度快照的最终结果展示信息
 * @param {Object} snapshot - 端点调度快照
 * @returns {{outcome: string, label: string, className: string, isAbnormal: boolean}}
 */
export const resolveSnapshotOutcome = (snapshot = {}) => {
  const outcome = snapshot.finalOutcome || (snapshot.hasSnapshot ? 'pending' : '');
  const [label, className] = scheduleOutcomeMeta[outcome]
    || [outcome || '未知', 'bg-slate-100 text-slate-600 border-slate-200'];
  return {
    outcome,
    label,
    className,
    isAbnormal: Boolean(outcome) && !['pending', 'success'].includes(outcome)
  };
};
