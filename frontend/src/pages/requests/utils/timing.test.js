import test from 'node:test';
import assert from 'node:assert/strict';

import {
  calculateGenerationMs,
  calculateTokensPerSecond,
  formatOptionalTimingBadge,
  formatTimingBadge,
  formatTpsBadge,
  getTimingPillClassName,
  resolveCompletionMs
} from './timing.js';

test('formatTimingBadge renders milliseconds as one-decimal seconds', () => {
  assert.equal(formatTimingBadge(12345), '12.3s');
  assert.equal(formatTimingBadge(-100), '0.0s');
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
