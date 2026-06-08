import test from 'node:test';
import assert from 'node:assert/strict';

import { formatTimingBadge, getTimingPillClassName } from './timing.js';

test('formatTimingBadge renders milliseconds as one-decimal seconds', () => {
  assert.equal(formatTimingBadge(12345), '12.3s');
  assert.equal(formatTimingBadge(-100), '0.0s');
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
