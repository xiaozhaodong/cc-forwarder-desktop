import test from 'node:test';
import assert from 'node:assert/strict';

import {
  buildReorderPayload,
  duplicateRuleForm,
  filterPrivacyRules,
  formatScanBytes,
  moveRuleInList,
  summarizeScope,
  validatePrivacyRuleForm
} from './privacyRules.js';

test('formatScanBytes renders human readable sizes', () => {
  assert.equal(formatScanBytes(4194304), '4 MB');
  assert.equal(formatScanBytes(1536), '1.5 KB');
  assert.equal(formatScanBytes(512), '512 B');
  assert.equal(formatScanBytes(0), '-');
  assert.equal(formatScanBytes('oops'), '-');
});

test('summarizeScope shows 全部请求 for empty scope and joins dimensions', () => {
  assert.equal(summarizeScope({}), '全部请求');
  assert.equal(summarizeScope({ paths: [], upstream_types: [] }), '全部请求');

  const summary = summarizeScope({
    paths: ['/v1/messages', '/v1/responses'],
    upstream_types: ['account'],
    account_ids: [7]
  });
  assert.match(summary, /路径 2/);
  assert.match(summary, /Codex 账号/);
  assert.match(summary, /账号 1/);
});

test('filterPrivacyRules applies keyword and facet filters', () => {
  const rules = [
    { id: 1, name: 'OpenAI Key', description: '', pattern: 'sk-.+', enabled: true, match_type: 'regex', action: 'redact', source: 'preset' },
    { id: 2, name: '项目代号', description: '内部', pattern: 'Phoenix', enabled: false, match_type: 'literal', action: 'detect', source: 'custom' }
  ];

  assert.equal(filterPrivacyRules(rules, { keyword: 'phoenix' }).length, 1);
  assert.equal(filterPrivacyRules(rules, { keyword: 'phoenix' })[0].id, 2);
  assert.equal(filterPrivacyRules(rules, { enabled: 'enabled' }).length, 1);
  assert.equal(filterPrivacyRules(rules, { enabled: 'disabled' })[0].id, 2);
  assert.equal(filterPrivacyRules(rules, { matchType: 'regex' })[0].id, 1);
  assert.equal(filterPrivacyRules(rules, { action: 'detect' })[0].id, 2);
  assert.equal(filterPrivacyRules(rules, { source: 'preset' })[0].id, 1);
  assert.equal(filterPrivacyRules(rules, {}).length, 2);
});

test('validatePrivacyRuleForm enforces required fields', () => {
  const valid = validatePrivacyRuleForm({
    name: 'A', pattern: 'x', action: 'redact', placeholder: '[x]', priority: 100
  });
  assert.deepEqual(valid, {});

  const errors = validatePrivacyRuleForm({
    name: ' ', pattern: '', action: 'redact', placeholder: '', priority: -1
  });
  assert.ok(errors.name);
  assert.ok(errors.pattern);
  assert.ok(errors.placeholder);
  assert.ok(errors.priority);

  // detect 动作不强制占位符
  const detect = validatePrivacyRuleForm({
    name: 'B', pattern: 'y', action: 'detect', placeholder: '', priority: 0
  });
  assert.equal(detect.placeholder, undefined);
});

test('duplicateRuleForm clears id and appends 副本', () => {
  const copy = duplicateRuleForm({
    id: 9, name: 'OpenAI Key', pattern: 'sk-.+', source: 'preset',
    scope: { paths: ['/v1/messages'] }
  });
  assert.equal(copy.id, 0);
  assert.equal(copy.name, 'OpenAI Key 副本');
  assert.equal(copy.source, 'custom');
  assert.deepEqual(copy.scope.paths, ['/v1/messages']);
  assert.deepEqual(copy.scope.endpoint_names, []);
});

test('moveRuleInList swaps neighbors and keeps bounds', () => {
  const rules = [{ id: 1 }, { id: 2 }, { id: 3 }];
  assert.deepEqual(moveRuleInList(rules, 2, -1).map((r) => r.id), [2, 1, 3]);
  assert.deepEqual(moveRuleInList(rules, 3, 1).map((r) => r.id), [1, 2, 3]);
  assert.deepEqual(moveRuleInList(rules, 1, -1).map((r) => r.id), [1, 2, 3]);
  assert.deepEqual(moveRuleInList(rules, 99, 1).map((r) => r.id), [1, 2, 3]);
});

test('buildReorderPayload assigns step-10 priorities in order', () => {
  const payload = buildReorderPayload([{ id: 5 }, { id: 3 }, { id: 8 }]);
  assert.deepEqual(payload, [
    { id: 5, priority: 10 },
    { id: 3, priority: 20 },
    { id: 8, priority: 30 }
  ]);
});
