import test from 'node:test';
import assert from 'node:assert/strict';

import {
  filterUpstreamOptionsByFamily,
  getRequestFamilyMeta,
  normalizeRequestSource,
  resolveRequestUpstream
} from './requestSource.js';

test('request source uses request_family and upstream_name as primary fields', () => {
  const request = normalizeRequestSource({ request_family: 'claude', upstream_name: 'claude-primary', endpoint_name: 'legacy' });
  assert.equal(request.requestFamily, 'claude');
  assert.equal(request.upstreamName, 'claude-primary');
  assert.equal(getRequestFamilyMeta(request.requestFamily).label, 'Claude');
});

test('unknown family remains stable as Other and upstream fallback is non-empty', () => {
  assert.equal(normalizeRequestSource({ request_family: 'mystery' }).requestFamily, 'other');
  assert.equal(resolveRequestUpstream({}), '未知上游');
});

test('upstream options follow selected request family', () => {
  const rows = [
    { request_family: 'claude', upstream_name: 'ep-a' },
    { request_family: 'codex', upstream_name: 'account-a' },
    { request_family: 'claude', upstream_name: 'ep-b' }
  ];
  assert.deepEqual(filterUpstreamOptionsByFamily(rows, 'claude'), ['ep-a', 'ep-b']);
  assert.deepEqual(filterUpstreamOptionsByFamily(rows, 'codex'), ['account-a']);
});

test('空上游按终态码归类：候选为空显示「无可用上游」而非「未知上游」', () => {
  assert.equal(
    resolveRequestUpstream({ failure_reason: 'no_endpoints_available: no routable endpoint candidates' }),
    '无可用上游'
  );
  assert.equal(resolveRequestUpstream({ failureReason: 'no_healthy' }), '无可用上游');
  assert.equal(resolveRequestUpstream({ failureReason: 'all_endpoints_failed: budget exhausted' }), '无可用上游');
  // 规则表与 lifecycle.js 共享，manual_fixed 历史空上游记录同样归类
  assert.equal(resolveRequestUpstream({ failureReason: 'manual_fixed_endpoint_disabled: ...' }), '无可用上游');
});

test('归类只锚定 code 前缀，detail 文本与其他终态不受影响', () => {
  // detail 里含 endpoint= 不得被误吞
  assert.equal(
    resolveRequestUpstream({ failure_reason: 'rate_limited: endpoint=xuanwulei status=429' }),
    '未知上游'
  );
  // 自由文本首词不可信，回退未知
  assert.equal(resolveRequestUpstream({ cancelReason: 'stream processing cancelled' }), '未知上游');
  // 有上游时终态码不参与判定
  assert.equal(
    resolveRequestUpstream({ upstream_name: 'xuanwulei', failure_reason: 'no_endpoints_available' }),
    'xuanwulei'
  );
});

// 上游筛选按 upstream_name 精确查库（filterTime.js 传 upstream_name 参数），
// 合成标签在库里对应空值，进选项就是必然返回 0 条的死选项。
test('合成上游标签不进筛选选项', () => {
  const rows = [
    { request_family: 'claude', upstream_name: 'xuanwulei' },
    { request_family: 'claude', upstream_name: '', failure_reason: 'no_endpoints_available: no routable endpoint candidates' },
    { request_family: 'claude', upstream_name: '', failure_reason: 'request_body_read_failed: Failed to read request body' }
  ];
  assert.deepEqual(filterUpstreamOptionsByFamily(rows, 'claude'), ['xuanwulei']);
  assert.deepEqual(filterUpstreamOptionsByFamily(rows, 'all'), ['xuanwulei']);
});
