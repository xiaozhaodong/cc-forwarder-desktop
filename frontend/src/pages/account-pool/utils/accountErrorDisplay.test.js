import test from 'node:test';
import assert from 'node:assert/strict';

import { buildAccountErrorDisplay } from './accountErrorDisplay.js';

test('buildAccountErrorDisplay extracts nested JSON messages and request ids', () => {
  const display = buildAccountErrorDisplay(JSON.stringify({
    error: {
      code: '',
      message: '分组 Team 已被弃用 (request id: req-123)'
    }
  }));

  assert.equal(display?.category, 'config');
  assert.equal(display?.label, '配置异常');
  assert.equal(display?.tone, 'rose');
  assert.equal(display?.message, '分组 Team 已被弃用');
  assert.equal(display?.summary, '分组 Team 已被弃用');
  assert.equal(display?.requestId, 'req-123');
  assert.match(display?.raw || '', /"error"/);
});

test('buildAccountErrorDisplay categorizes quota, billing, capacity and authentication errors', () => {
  const cases = [
    ['用户额度不足，剩余额度：¥0.063836', 'quota', '额度不足', '剩余额度：¥0.063836'],
    ['预扣费额度失败，用户剩余额度：¥0.028898', 'billing', '扣费失败', '用户剩余额度：¥0.028898'],
    ['当前模型 gpt-5.6-sol 负载已经达到上限，请稍后重试', 'capacity', '上游繁忙', '当前模型 gpt-5.6-sol 负载已经达到上限，请稍后重试'],
    ['refresh token expired', 'auth', '认证异常', 'refresh token expired']
  ];

  cases.forEach(([raw, category, label, summary]) => {
    const display = buildAccountErrorDisplay(raw);
    assert.equal(display?.category, category);
    assert.equal(display?.label, label);
    assert.equal(display?.summary, summary);
  });
});

test('buildAccountErrorDisplay handles nested JSON strings and damaged JSON without exposing syntax on cards', () => {
  const nested = buildAccountErrorDisplay(JSON.stringify(JSON.stringify({
    error: { message: '用户额度不足，剩余额度：¥1.23' }
  })));
  assert.equal(nested?.summary, '剩余额度：¥1.23');

  const damaged = buildAccountErrorDisplay('{"error":{"message":"上游服务繁忙，请稍后重试"');
  assert.equal(damaged?.message, '上游服务繁忙，请稍后重试');
  assert.equal(damaged?.summary, '上游服务繁忙，请稍后重试');
  assert.doesNotMatch(damaged?.summary || '', /^[{[]/);
});

test('buildAccountErrorDisplay preserves plain text and returns null for empty input', () => {
  const display = buildAccountErrorDisplay('connection reset by peer');
  assert.equal(display?.category, 'unknown');
  assert.equal(display?.label, '最近异常');
  assert.equal(display?.summary, 'connection reset by peer');
  assert.equal(buildAccountErrorDisplay(''), null);
});
