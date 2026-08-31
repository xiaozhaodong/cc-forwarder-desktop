import test from 'node:test';
import assert from 'node:assert/strict';

import {
  calculateGenerationMs,
  calculateTokensPerSecond,
  formatOptionalTimingBadge,
  formatTimingBadge,
  formatTpsBadge,
  getTimingPillClassName,
  resolveCompletionMs,
  resolveFirstResponseMs
} from './timing.js';

test('formatTimingBadge 三档：毫秒 / 两位小数 / 一位小数', () => {
  assert.equal(formatTimingBadge(340), '340ms', '1 秒内给毫秒整数，避免一屏的 0.3s');
  assert.equal(formatTimingBadge(999), '999ms');
  assert.equal(formatTimingBadge(1000), '1.00s');
  assert.equal(formatTimingBadge(5432), '5.43s');
  assert.equal(formatTimingBadge(12345), '12.3s', '10 秒后末位只剩噪声，收回一位');
  assert.equal(formatTimingBadge(-100), '0ms');
});

test('formatOptionalTimingBadge renders missing timing as placeholder', () => {
  assert.equal(formatOptionalTimingBadge(12345), '12.3s');
  assert.equal(formatOptionalTimingBadge(null), '-');
  assert.equal(formatOptionalTimingBadge(undefined), '-');
});

test('calculateGenerationMs derives generation duration from first response', () => {
  assert.equal(calculateGenerationMs(26000, 6000), 20000);
  assert.equal(calculateGenerationMs(6000, 26000), 0);
  assert.equal(calculateGenerationMs(26000, null), null);
});

test('resolveCompletionMs prefers recorded completion duration and falls back for old data', () => {
  assert.equal(resolveCompletionMs(14000, 26000, 6000), 14000);
  assert.equal(resolveCompletionMs(null, 26000, 6000), 20000);
  assert.equal(resolveCompletionMs(undefined, 26000, null), null);
  assert.equal(resolveCompletionMs(null, 26000, 26000, false), 0);
});

test('resolveFirstResponseMs uses total duration for historical non-streaming requests', () => {
  assert.equal(resolveFirstResponseMs(6100, 6300, false), 6100);
  assert.equal(resolveFirstResponseMs(null, 6300, false), 6300);
  assert.equal(resolveFirstResponseMs(null, 6300, true), null);
});

test('calculateTokensPerSecond uses generation duration as denominator', () => {
  assert.equal(calculateTokensPerSecond(400, 20000), 20);
  assert.equal(calculateTokensPerSecond(400, 0), null);
  assert.equal(calculateTokensPerSecond(null, 20000), null);
});

test('formatTpsBadge keeps compact table output', () => {
  assert.equal(formatTpsBadge(20), '20.0');
  assert.equal(formatTpsBadge(120.4), '120');
  assert.equal(formatTpsBadge(null), '-');
});

test('getTimingPillClassName uses relaxed first-token thresholds', () => {
  assert.match(getTimingPillClassName('first', 8000), /emerald/);
  assert.match(getTimingPillClassName('first', 8001), /amber/);
  assert.match(getTimingPillClassName('first', 15000), /amber/);
  assert.match(getTimingPillClassName('first', 15001), /rose/);
});

test('getTimingPillClassName uses relaxed duration thresholds', () => {
  assert.match(getTimingPillClassName('duration', 30000), /emerald/);
  assert.match(getTimingPillClassName('duration', 30001), /orange/);
  assert.match(getTimingPillClassName('duration', 60000), /orange/);
  assert.match(getTimingPillClassName('duration', 60001), /rose/);
});
