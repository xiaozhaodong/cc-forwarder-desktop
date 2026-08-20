// lifecycle 分段推导单测（方案 F2 测试计划）
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  buildLifecycleSegments,
  resolveConnectMs,
  resolveQueueMs,
  resolveRawQueueMs,
  summarizeTerminalReason,
  TAIL_THRESHOLD_MS
} from './lifecycle.js';

const base = {
  duration: 10000,
  timestamp: '2026-08-13T00:00:00.000000Z',
  routeDecisionAt: '2026-08-13T00:00:02.000000Z',
  firstTokenMs: 4000,
  completionMs: 5000,
  isStreaming: true,
  status: 'completed'
};

test('全量：UTC 微秒 ISO 绝对时刻差 + 排队/连接拆分', () => {
  const request = {
    ...base,
    upstreamWriteMs: 2500 // 排队 2000 + 连接 500
  };
  const segments = buildLifecycleSegments(request);
  assert.deepEqual(
    segments.map((seg) => [seg.key, seg.ms]),
    [['queue', 2000], ['connect', 500], ['first', 4000], ['stream', 5000]]
  );
});

test('routeDecisionAt 晚于 upstreamWriteMs：排队钳制、连接为 0', () => {
  const request = {
    ...base,
    routeDecisionAt: '2026-08-13T00:00:05.000000Z',
    upstreamWriteMs: 1000
  };
  assert.equal(resolveQueueMs(request), 1000);
  assert.equal(resolveConnectMs(request), 0);
});

test('空串 / 无时区 / 无效 ISO：排队与连接降级为 null', () => {
  assert.equal(resolveRawQueueMs({ ...base, routeDecisionAt: '' }), null);
  assert.equal(resolveRawQueueMs({ ...base, routeDecisionAt: '2026-08-13 00:00:01' }), null);
  assert.equal(resolveRawQueueMs({ ...base, routeDecisionAt: 'not-a-date' }), null);
  assert.equal(resolveConnectMs({ ...base, upstreamWriteMs: null }), null);
});

test('降级 2：只有 first/completion（旧数据）→ 前置/首字/流式输出', () => {
  const segments = buildLifecycleSegments({
    duration: 10000,
    firstTokenMs: 3000,
    completionMs: 6000,
    isStreaming: true,
    status: 'completed'
  });
  assert.deepEqual(
    segments.map((seg) => [seg.key, seg.ms]),
    [['pre', 1000], ['first', 3000], ['stream', 6000]]
  );
});

test('降级 3：有 upstreamWriteMs 无 routeDecisionAt → 准备（排队+连接）', () => {
  const segments = buildLifecycleSegments({
    duration: 10000,
    upstreamWriteMs: 1500,
    firstTokenMs: 3000,
    completionMs: 5000,
    isStreaming: true,
    status: 'completed'
  });
  assert.deepEqual(segments[0], { key: 'connect', label: '准备（排队+连接）', ms: 1500 });
});

test('降级 1：全无 timing → 单段总耗时', () => {
  const segments = buildLifecycleSegments({ duration: 8000, status: 'completed', isStreaming: true });
  assert.deepEqual(segments, [{ key: 'total', label: '总耗时', ms: 8000 }]);
});

test('收尾残差 > 80ms 单独显示，否则并入末段', () => {
  const withTail = buildLifecycleSegments({
    ...base,
    duration: 20000 // 已知段和 11000，残差 9000 > 80
  });
  assert.equal(withTail[withTail.length - 1].key, 'tail');

  const merged = buildLifecycleSegments({
    ...base,
    duration: 11550, // 已知段和 11500，残差 50 <= 80，并入流式段
    upstreamWriteMs: 2500
  });
  const stream = merged.find((seg) => seg.key === 'stream');
  assert.equal(stream.ms, 5050);
  assert.equal(merged.some((seg) => seg.key === 'tail'), false);
  assert.ok(TAIL_THRESHOLD_MS > 50 && TAIL_THRESHOLD_MS < 9000);
});

test('非流式：completion=0 不出流式段，first 段命名「响应」', () => {
  const segments = buildLifecycleSegments({
    duration: 5000,
    firstTokenMs: 5000,
    completionMs: 0,
    isStreaming: false,
    status: 'completed'
  });
  assert.equal(segments.some((seg) => seg.key === 'stream'), false);
  const first = segments.find((seg) => seg.key === 'first');
  assert.equal(first.label, '响应');
});

test('failed/cancelled：已知段照画，末尾补标注段（短标签 + detail 全文）', () => {
  const rawReason = 'rate_limited: endpoint=xuanwulei status=429 content_type=text/event-stream body={"error":{"message":"Service Unavailable","type":"error"},"type":"error"}';
  const failed = buildLifecycleSegments({
    ...base,
    duration: 11000,
    status: 'failed',
    failureReason: rawReason
  });
  const failureSeg = failed[failed.length - 1];
  assert.equal(failureSeg.key, 'failure');
  assert.equal(failureSeg.label, '限流 429');
  assert.equal(failureSeg.detail, rawReason);

  const cancelled = buildLifecycleSegments({
    ...base,
    status: 'cancelled',
    cancelReason: 'client disconnected during streaming'
  });
  const cancelSeg = cancelled[cancelled.length - 1];
  assert.equal(cancelSeg.key, 'cancelled');
  assert.equal(cancelSeg.label, '已取消');
  assert.equal(cancelSeg.detail, 'client disconnected during streaming');
});

test('summarizeTerminalReason：错误码归类 + status 提取', () => {
  assert.equal(
    summarizeTerminalReason('rate_limited: endpoint=x status=429 body={"error":"..."}'),
    '限流 429'
  );
  assert.equal(summarizeTerminalReason('auth_error: token expired'), '鉴权失败');
  assert.equal(summarizeTerminalReason('privacy_blocked: rule=id_card'), '隐私拦截');
  assert.equal(summarizeTerminalReason('connection_timeout: dial tcp'), '超时');
  assert.equal(summarizeTerminalReason('network_error'), '网络错误');
  assert.equal(
    summarizeTerminalReason('upstream_no_available_providers: status=503 no providers'),
    '无可用上游 503'
  );
  assert.equal(summarizeTerminalReason('server_error: status=502'), '服务器错误 502');
  assert.equal(summarizeTerminalReason('upstream_client_error: status=400'), '上游拒绝 400');
  assert.equal(summarizeTerminalReason('incomplete_stream'), '流错误');
  assert.equal(summarizeTerminalReason('rejected_by_failure_tracker: cooldown'), '冷却拦截');
  assert.equal(summarizeTerminalReason('client_cancelled: closed'), '已取消');
});

test('summarizeTerminalReason：upstream 系码不被 stream 规则误吞', () => {
  assert.equal(
    summarizeTerminalReason('image_generation_upstream_error: upstream_status=502 reason=bad_gateway'),
    '上游错误 502'
  );
  assert.equal(
    summarizeTerminalReason('image_api_invalid_upstream_response: content_type=text/html'),
    '上游错误'
  );
  // stream 规则仍覆盖码首与 `_` 后的 stream。
  assert.equal(summarizeTerminalReason('stream_truncated'), '流错误');
  assert.equal(summarizeTerminalReason('account_stream_error: broken pipe'), '流错误');
});

test('summarizeTerminalReason：未识别结构化码原样显示，自由文本与空值回退 null', () => {
  // 结构化未识别码：显示码本身，避免整串全文。
  assert.equal(summarizeTerminalReason('some_new_code: detail here'), 'some_new_code');
  // 整串即码（无冒号无空格）视为结构化。
  assert.equal(summarizeTerminalReason('weird_code_123'), 'weird_code_123');
  // 自由英文句子：首词不可信，返回 null 由调用方回退「失败/已取消」。
  assert.equal(summarizeTerminalReason('something went wrong badly'), null);
  // 回归：自由文本首词即便撞上规则表也必须回退，不能被误归类。
  // endpoint_pipeline.go 的 cancelReason `stream processing cancelled` 曾被错标成「流错误」。
  assert.equal(summarizeTerminalReason('stream processing cancelled'), null);
  assert.equal(summarizeTerminalReason('rate_limit hit while writing'), null);
  assert.equal(summarizeTerminalReason(''), null);
  assert.equal(summarizeTerminalReason(null), null);
  assert.equal(summarizeTerminalReason(undefined), null);
});

test('summarizeTerminalReason：隐私扫描故障与规则命中拦截区分开', () => {
  assert.equal(summarizeTerminalReason('privacy_scan_failed: engine panic'), '隐私扫描失败');
  assert.equal(summarizeTerminalReason('privacy_blocked: rule=id_card'), '隐私拦截');
});

test('后端真实 cancelReason 均落到「已取消」段', () => {
  // 取值来源：endpoint_pipeline.go / account_pipeline.go / lifecycle_manager.go 的 CancelRequest 调用点。
  const reasons = [
    'stream processing cancelled',
    'account stream processing cancelled',
    'client disconnected',
    'context canceled'
  ];
  for (const cancelReason of reasons) {
    const segments = buildLifecycleSegments({ ...base, status: 'cancelled', cancelReason });
    const seg = segments[segments.length - 1];
    assert.equal(seg.key, 'cancelled', `${cancelReason} 段类型应为 cancelled`);
    assert.equal(seg.label, '已取消', `${cancelReason} 应标为「已取消」`);
    assert.equal(seg.detail, cancelReason);
  }
});
