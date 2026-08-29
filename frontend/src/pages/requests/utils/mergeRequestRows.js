// ============================================
// mergeRequestRows - 请求行引用稳定化
// 2026-08-28
// 后端每次返回的都是全新对象，直接 setRequests 会让整页所有行的
// React.memo 失效（引用恒不等），一次 request:update 就重渲染 20 行 × 12 列。
// 这里按 requestId 比对渲染相关字段，未变化的行复用旧对象引用，
// 使「新到一条请求」只重渲染真正变化的那 1~2 行。
// ============================================

// 参与比对的字段 = 表格实际渲染用到的字段。
// ⚠️ 新增可见列时必须同步这里，否则该列的数据变化不会触发行重渲染。
// constants.js 的 TABLE_COLUMNS 与本表由 mergeRequestRows.test.js 的覆盖用例锁定。
export const RENDER_FIELDS = Object.freeze([
  'requestId',
  'timestamp',
  'status',
  'model',
  'requestFamily',
  'upstreamName',
  'duration',
  'firstTokenMs',
  'completionMs',
  'isStreaming',
  'inputTokens',
  'outputTokens',
  'cacheCreationTokens',
  'cacheReadTokens',
  'cost'
]);

// hasSameRenderData 只做渲染字段的浅比较；NaN 不参与（后端不会下发 NaN 数值）。
const hasSameRenderData = (left, right) => RENDER_FIELDS.every((field) => left[field] === right[field]);

/**
 * 合并新旧两页请求行，未变化的行沿用旧引用。
 *
 * @param {Array<Object>} prevRows 上一次渲染用的行（可为空）
 * @param {Array<Object>} nextRows 本次接口返回的行
 * @returns {Array<Object>} 与 nextRows 顺序、长度一致的新数组
 */
export const mergeRequestRows = (prevRows, nextRows) => {
  if (!Array.isArray(nextRows)) return [];
  if (!Array.isArray(prevRows) || prevRows.length === 0) return nextRows;

  const prevById = new Map();
  for (const row of prevRows) {
    // 同 ID 只认第一条，与列表渲染的 key 去重行为保持一致。
    if (row && row.requestId && !prevById.has(row.requestId)) {
      prevById.set(row.requestId, row);
    }
  }

  return nextRows.map((row) => {
    if (!row || !row.requestId) return row;
    const prev = prevById.get(row.requestId);
    return prev && hasSameRenderData(prev, row) ? prev : row;
  });
};

export default mergeRequestRows;
