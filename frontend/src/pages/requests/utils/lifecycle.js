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

// 终态短标签归类规则（按优先级匹配错误码；`connection` 须排在 `timeout` 后，
// 保证 connection_timeout 归入「超时」；`privacy_scan` 须排在 `privacy` 前，区分
// 扫描引擎故障与规则命中拦截；stream 只匹配码首或 `_` 后，避免误吞 upstream 系码）。
const REASON_LABEL_RULES = [
  [/rate_limit/, '限流'],
  [/auth/, '鉴权失败'],
  [/privacy_scan/, '隐私扫描失败'],
  [/privacy/, '隐私拦截'],
  [/timeout/, '超时'],
  [/network|eof/, '网络错误'],
  [/no_healthy|no_endpoints|no_available_providers|pool_exhausted|pool_not_ready|pool_load_failed|all_endpoints_failed/, '无可用上游'],
  [/server_error/, '服务器错误'],
  [/client_error/, '上游拒绝'],
  [/invalid_upstream|upstream_error/, '上游错误'],
  [/write_error|read_error/, '读写错误'],
  [/parsing/, '解析错误'],
  [/empty_response/, '空响应'],
  [/(?:^|_)stream/, '流错误'],
  [/http_error/, 'HTTP 错误'],
  [/unknown/, '未知错误'],
  [/connection/, '网络错误'],
  [/rejected/, '冷却拦截'],
  [/cancel|disconnect/, '已取消']
];

// summarizeTerminalReason 从原始终态串提取短标签。
// 存储格式（tracker.RecordRequestFinalFailure）：`code: detail`，detail 常含 `status=NNN`。
// 归类失败时：结构化码（`code: …` 或整串即码）原样显示；自由文本返回 null 由调用方回退。
export const summarizeTerminalReason = (reason) => {
  const text = String(reason || '').trim();
  if (!text) return null;
  const codeMatch = text.match(/^([a-z][a-z0-9_]*)(:|\s|$)/i);
  if (!codeMatch) return null;
  // 自由文本（首词后跟空格）首词不可信，须先回退，否则会撞规则表被误归类：
  // 例如 cancelReason `stream processing cancelled` 首词命中 stream 规则会错标成「流错误」。
  const isStructuredCode = codeMatch[2] === ':' || codeMatch[2] === '';
  if (!isStructuredCode) return null;
  const code = codeMatch[1].toLowerCase();
  const statusMatch = text.match(/\b(?:upstream_)?status=(\d{3})\b/);
  const statusSuffix = statusMatch ? ` ${statusMatch[1]}` : '';
  const rule = REASON_LABEL_RULES.find(([pattern]) => pattern.test(code));
  return `${rule ? rule[1] : code}${statusSuffix}`;
};

// terminalSegment 终态标注段：failed/cancelled 时返回，否则 null。
// label 只放短标签，原始全文经 detail 供 tooltip 展示（完整错误另见详情页错误卡片）。
const terminalSegment = (request = {}, ms = 0) => {
  const status = String(request.status || '').toLowerCase();
  if (status === 'failed' || status === 'error') {
    const detail = request.failureReason || '';
    return { key: 'failure', label: summarizeTerminalReason(detail) || '失败', ms, detail };
  }
  if (status === 'cancelled') {
    const detail = request.cancelReason || '';
    return { key: 'cancelled', label: summarizeTerminalReason(detail) || '已取消', ms, detail };
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
