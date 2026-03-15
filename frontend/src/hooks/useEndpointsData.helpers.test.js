import test from 'node:test';
import assert from 'node:assert/strict';

import { applyEndpointHealthCheckResult, calculateEndpointStats } from './useEndpointsData.helpers.js';

test('applyEndpointHealthCheckResult recalculates stats from updated endpoints', () => {
  const endpoints = [
    { name: 'a', healthy: false, never_checked: false, response_time: 0 },
    { name: 'b', healthy: true, never_checked: false, response_time: 12 }
  ];

  const updatedEndpoints = applyEndpointHealthCheckResult(endpoints, 'a', {
    healthy: true,
    response_time: 45
  }, '2026-03-15T10:00:00.000Z');

  const stats = calculateEndpointStats(updatedEndpoints);

  assert.equal(updatedEndpoints[0].healthy, true);
  assert.equal(updatedEndpoints[0].response_time, 45);
  assert.equal(stats.healthy, 2);
  assert.equal(stats.unhealthy, 0);
  assert.equal(stats.healthPercentage, '100.0');
});
