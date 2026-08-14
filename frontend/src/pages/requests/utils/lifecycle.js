// ============================================
// lifecycle - 生命周期分段推导纯函数（方案 F2）
// 2026-08-13
// 输入：normalizeRequest 后的权威行数据（effective）。
// 时间契约：timestamp / routeDecisionAt 均为固定微秒精度 UTC ISO（…Z）绝对时刻，
// 直接相减即真实毫秒差；解析失败降级为 null，不制造假段。
// ============================================

import { parseTimestamp } from '../../../utils/timezone.js';

// 收尾残差超过该阈值才单独显示「收尾」微段，否则并入末段。
export const TAIL_THRESHOLD_MS = 80;

const toNullableMs = (value) => {
  if (value === null || value === undefined) return null;
  const ms = Number(value);
  return Number.isFinite(ms) ? Math.max(ms, 0) : null;
};

// parseTimestampOrNull 复用严格绝对时刻解析；失败返回 null。
export const parseTimestampOrNull = (value) => {
  try {
    return parseTimestamp(value);
  } catch {
    return null;
  }
};

// resolveRawQueueMs 排队段原始值：routeDecisionAt - startTime（毫秒）。
export const resolveRawQueueMs = (request = {}) => {
  const routeAt = parseTimestampOrNull(request.routeDecisionAt);
  const startAt = parseTimestampOrNull(request.timestamp);
  if (!routeAt || !startAt) return null;
  return Math.max(0, routeAt.getTime() - startAt.getTime());
};

// resolveQueueMs 排队段：与 upstreamWriteMs 同时存在时钳制到上游写偏移，禁止重叠。
export const resolveQueueMs = (request = {}) => {
  const rawQueueMs = resolveRawQueueMs(request);
  const upstreamWriteMs = toNullableMs(request.upstreamWriteMs);
  if (rawQueueMs === null) return null;
  if (upstreamWriteMs !== null) return Math.min(rawQueueMs, upstreamWriteMs);
  return rawQueueMs;
};

// resolveConnectMs 连接段：upstreamWriteMs - 排队段。
export const resolveConnectMs = (request = {}) => {
  const upstreamWriteMs = toNullableMs(request.upstreamWriteMs);
  if (upstreamWriteMs === null) return null;
  const queueMs = resolveQueueMs(request);
  return Math.max(0, upstreamWriteMs - (queueMs ?? 0));
};

// terminalSegment 终态标注段：failed/cancelled 时返回，否则 null。
const terminalSegment = (request = {}, ms = 0) => {
  const status = String(request.status || '').toLowerCase();
  if (status === 'failed' || status === 'error') {
    return { key: 'failure', label: request.failureReason || '失败', ms };
  }
  if (status === 'cancelled') {
    return { key: 'cancelled', label: request.cancelReason || '已取消', ms };
  }
  return null;
};

// buildLifecycleSegments 分段推导（毫秒，全部防御 NULL 与非单调输入）。
// 降级规则（自旧向新）：
//   1. 全无 timing → 单段「总耗时」
//   2. 只有 first/completion（旧数据）→ 前置 / 首字 / 流式输出
//   3. 有 upstreamWriteMs 无 routeDecisionAt → 准备（排队+连接）/ 首字 / 流式输出
//   4. 全量 → 排队 · 连接 · 首字 · 流式输出（· 收尾）
//   5. failed/cancelled → 已知段照画，末尾补红/灰标注段
export const buildLifecycleSegments = (request = {}) => {
  const duration = toNullableMs(request.duration) ?? 0;
  const firstMs = toNullableMs(request.firstTokenMs);
  const completionMs = toNullableMs(request.completionMs);
  const isStreaming = request.isStreaming !== false;
  const queueMs = resolveQueueMs(request);
  const connectMs = resolveConnectMs(request);

  const hasTiming = firstMs !== null || completionMs !== null || queueMs !== null || connectMs !== null;

  // 降级规则 1：全无 timing。
  if (!hasTiming) {
    const terminal = terminalSegment(request, duration);
    return terminal ? [terminal] : [{ key: 'total', label: '总耗时', ms: duration }];
  }

  const segments = [];

  if (queueMs !== null && queueMs > 0) {
    segments.push({ key: 'queue', label: '排队', ms: queueMs });
  }
  if (connectMs !== null && connectMs > 0) {
    // 降级规则 3：无 routeDecisionAt 时排队与连接不可分，合并为「准备」段。
    segments.push(queueMs === null
      ? { key: 'connect', label: '准备（排队+连接）', ms: connectMs }
      : { key: 'connect', label: '连接', ms: connectMs });
  }
  // 降级规则 2：无排队/连接信息时，用总耗时扣除首字与流式推算「前置」。
  if (queueMs === null && connectMs === null && firstMs !== null) {
    const preMs = Math.max(0, duration - ((firstMs ?? 0) + (completionMs ?? 0)));
    if (preMs > 0) {
      segments.push({ key: 'pre', label: '前置', ms: preMs });
    }
  }

  if (firstMs !== null) {
    segments.push({ key: 'first', label: isStreaming ? '首字' : '响应', ms: firstMs });
  }
  // 非流式 completionMs=0，不出流式段。
  if (isStreaming && completionMs !== null && completionMs > 0) {
    segments.push({ key: 'stream', label: '流式输出', ms: completionMs });
  }

  const knownSum = segments.reduce((sum, seg) => sum + seg.ms, 0);

  // 降级规则 5：终态标注段吸收残差。
  const terminal = terminalSegment(request, 0);
  if (terminal) {
    terminal.ms = Math.max(0, duration - knownSum);
    segments.push(terminal);
    return segments;
  }

  // 收尾残差：> 80ms 单独显示，否则并入末段。
  const tailMs = Math.max(0, duration - knownSum);
  if (tailMs > TAIL_THRESHOLD_MS) {
    segments.push({ key: 'tail', label: '收尾', ms: tailMs });
  } else if (tailMs > 0 && segments.length > 0) {
    segments[segments.length - 1].ms += tailMs;
  }

  return segments;
};

// lifecycleSegmentColors 分段配色（白底弹窗，沿用现有浅色语系）。
export const lifecycleSegmentColors = {
  queue: 'bg-slate-200',
  connect: 'bg-indigo-200',
  pre: 'bg-slate-200',
  first: 'bg-violet-300',
  stream: 'bg-emerald-300',
  tail: 'bg-slate-200',
  total: 'bg-slate-200',
  failure: 'bg-rose-300',
  cancelled: 'bg-slate-300'
};
