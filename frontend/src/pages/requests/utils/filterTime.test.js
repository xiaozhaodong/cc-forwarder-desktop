import test from 'node:test';
import assert from 'node:assert/strict';

import { buildQueryParamsFromFilters } from './filterTime.js';

test('buildQueryParamsFromFilters normalizes supported date-time precisions without producing malformed values', () => {
  assert.deepEqual(buildQueryParamsFromFilters({
    startDate: '2026-08-04',
    endDate: '2026-08-05T00:00'
  }), {
    start_date: '2026-08-04T00:00:00',
    end_date: '2026-08-05T00:00:00'
  });

  assert.deepEqual(buildQueryParamsFromFilters({
    startDate: '2026-08-04T01:02:03',
    endDate: '2026-08-05T01:02:03+08:00'
  }), {
    start_date: '2026-08-04T01:02:03',
    end_date: '2026-08-05T01:02:03+08:00'
  });
});
