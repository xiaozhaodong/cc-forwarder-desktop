// lifecycle 分段推导单测（方案 F2 测试计划）
import test from 'node:test';
import assert from 'node:assert/strict';
import {
  buildLifecycleSegments,
  resolveConnectMs,
  resolveQueueMs,
  resolveRawQueueMs,
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

test('failed/cancelled：已知段照画，末尾补标注段', () => {
  const failed = buildLifecycleSegments({
    ...base,
    duration: 11000,
    status: 'failed',
    failureReason: 'upstream_500'
  });
  assert.equal(failed[failed.length - 1].key, 'failure');
  assert.equal(failed[failed.length - 1].label, 'upstream_500');

  const cancelled = buildLifecycleSegments({
    ...base,
    status: 'cancelled',
    cancelReason: 'client disconnected'
  });
  assert.equal(cancelled[cancelled.length - 1].key, 'cancelled');
});
