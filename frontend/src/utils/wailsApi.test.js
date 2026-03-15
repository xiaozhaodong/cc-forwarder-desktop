import test from 'node:test';
import assert from 'node:assert/strict';

import { mapEndpointRecord } from './wailsApi.js';

test('mapEndpointRecord preserves cacheCreationCostMultiplier1h from detail payload', () => {
  const result = mapEndpointRecord({
    id: 1,
    name: 'ep-cache-1h',
    cache_creation_cost_multiplier_1h: 1.75
  });

  assert.equal(result.cacheCreationCostMultiplier1h, 1.75);
});
