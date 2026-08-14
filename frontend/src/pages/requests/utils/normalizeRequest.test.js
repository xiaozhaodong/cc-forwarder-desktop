// normalizeRequest 统一字段单测（方案 F1 测试计划）
import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeRequest } from './normalizeRequest.js';

test('Wails 列表形状：duration_ms / status_code', () => {
  const result = normalizeRequest({
    request_id: 'req-1',
    duration_ms: 120,
    status_code: 200,
    retry_count: 2,
    failure_reason: 'boom',
    cancel_reason: '',
    first_token_ms: 30,
    completion_ms: 50,
    route_decision_at: '2026-08-13T00:00:00.000000Z'
  });
  assert.equal(result.duration, 120);
  assert.equal(result.statusCode, 200);
  assert.equal(result.httpStatusCode, 200);
  assert.equal(result.retryCount, 2);
  assert.equal(result.failureReason, 'boom');
  assert.equal(result.cancelReason, '');
  assert.equal(result.firstTokenMs, 30);
  assert.equal(result.completionMs, 50);
});

test('Go RequestRecord 原始形状：response_time / http_status，无 duration_ms/status_code', () => {
  const result = normalizeRequest({
    request_id: 'req-2',
    response_time: 321,
    http_status: 503,
    http_status_code: 503,
    retry_count: 1
  });
  assert.equal(result.duration, 321);
  assert.equal(result.statusCode, 503);
  assert.equal(result.httpStatusCode, 503);
  assert.equal(result.retryCount, 1);
});

test('camelCase 输入与合法 0 保留', () => {
  const result = normalizeRequest({
    requestId: 'req-3',
    durationMs: 0,
    statusCode: 0,
    retryCount: 0,
    failureReason: '',
    firstTokenMs: 0,
    completionMs: 0
  });
  assert.equal(result.requestId, 'req-3');
  assert.equal(result.duration, 0);
  assert.equal(result.statusCode, 0);
  assert.equal(result.retryCount, 0);
  assert.equal(result.firstTokenMs, 0);
  assert.equal(result.completionMs, 0);
});

test('HTTP 列表形状与默认值', () => {
  const result = normalizeRequest({
    id: 'req-4',
    start_time: '2026-08-13T00:00:00.000000Z',
    model_name: 'gpt-5.4'
  });
  assert.equal(result.requestId, 'req-4');
  assert.equal(result.model, 'gpt-5.4');
  assert.equal(result.retryCount, 0);
  assert.equal(result.statusCode, null);
  assert.equal(result.routeMode, 'auto');
});

test('同时存在时 start_time 保持为权威请求时间', () => {
  const result = normalizeRequest({
    timestamp: '2026-08-13T00:00:01.000000Z',
    start_time: '2026-08-13T00:00:02.000000Z'
  });
  assert.equal(result.timestamp, '2026-08-13T00:00:02.000000Z');
});
