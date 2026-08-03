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
