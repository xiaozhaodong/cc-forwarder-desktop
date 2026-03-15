import test from 'node:test';
import assert from 'node:assert/strict';

import { getEndpointLastCheckDisplayValue } from './lastCheckDisplay.js';

test('getEndpointLastCheckDisplayValue ignores updatedAt when endpoint was never checked', () => {
  const endpoint = {
    never_checked: true,
    updatedAt: '2026-03-15T10:00:00Z'
  };

  assert.equal(getEndpointLastCheckDisplayValue(endpoint), '-');
});

test('getEndpointLastCheckDisplayValue prefers real lastCheck fields', () => {
  const endpoint = {
    lastCheck: '2026-03-15T10:10:00Z',
    updatedAt: '2026-03-15T10:00:00Z'
  };

  assert.equal(getEndpointLastCheckDisplayValue(endpoint), '2026-03-15T10:10:00Z');
});
