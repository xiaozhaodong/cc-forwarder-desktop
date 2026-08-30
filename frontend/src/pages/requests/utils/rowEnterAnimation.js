// ============================================
// rowEnterAnimation - 新行入场动画编排
// 2026-08-29
// 一次刷新可能同时带来多条新请求。若每行都立刻整条播完，
// 视觉上是「一坨行一起闪」，比不做动画还糟。这里做一件事：
// stagger —— 按顺序逐行延迟，视觉上读作「一条条流进来」。
// ============================================

// 相邻两行的入场间隔。太小看不出错峰，太大整批入场拖沓。
export const STAGGER_STEP_MS = 28;

// 延迟累计的档位上限。不封顶的话，一次来 10 条时最后一条要等 280ms 才出现，
// 观感上像卡住了；封到 3 档意味着最长等 84ms。
//
// 封顶之后就不需要「批量降级」了：第 4 条之后的行全部共用同一个延迟，
// 一次来 8 条和一次来 20 条看起来完全一样。入场只是 200ms 的轻微位移，
// 多行同时播不会像旧版 800ms 高亮底那样连成一片。
export const MAX_STAGGER_STEPS = 3;

// 无新增行时统一返回这一个实例。每次 new Map() 都是新引用，会让调用方的
// state 每轮刷新（最快 250ms 一次）都变，白白推一次全表重渲染 —— 而
// 「无新增」恰恰是绝大多数刷新的实际情况。
// 只读：调用方拿到后不得 set。
export const NO_ROW_ANIMATIONS = new Map();

/**
 * 计算本批次新增行各自的入场动画参数。
 *
 * @param {Array<Object>} rows 当前这一页的行
 * @param {Set<string>|null} prevIds 上一批的 requestId 集合；null 表示首次渲染
 * @param {boolean} enabled 是否允许播入场动画（仅第 1 页的实时刷新为 true，
 *        判定见调用方：非首页的「新 ID」多半是被上一页挤下来的旧请求）
 * @returns {Map<string, {delayMs: number}>} 仅包含新增行
 */
export const buildRowEnterAnimations = (rows, prevIds, enabled) => {
  if (!enabled || !prevIds || !Array.isArray(rows)) {
    return NO_ROW_ANIMATIONS;
  }

  const newIds = [];
  for (const row of rows) {
    if (row?.requestId && !prevIds.has(row.requestId)) {
      newIds.push(row.requestId);
    }
  }
  if (newIds.length === 0) {
    return NO_ROW_ANIMATIONS;
  }

  const animations = new Map();
  newIds.forEach((requestId, index) => {
    animations.set(requestId, {
      delayMs: Math.min(index, MAX_STAGGER_STEPS) * STAGGER_STEP_MS
    });
  });

  return animations;
};

/** collectRequestIds 提取当前页的 requestId 集合，供下一轮 diff 用。 */
export const collectRequestIds = (rows) => {
  const ids = new Set();
  if (!Array.isArray(rows)) return ids;
  for (const row of rows) {
    if (row?.requestId) ids.add(row.requestId);
  }
  return ids;
};

export default buildRowEnterAnimations;
