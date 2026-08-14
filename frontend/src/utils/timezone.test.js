import test from 'node:test';
import assert from 'node:assert/strict';
import {
  formatTimestamp,
  getRecentDaysRange,
  getTodayTimeRange,
  parseTimestamp,
  validateTimezone
} from './timezone.js';

test('formatTimestamp ignores the host timezone and uses configured timezone', () => {
  assert.equal(formatTimestamp('2026-08-04T06:50:10.000000Z', 'Asia/Shanghai'), '2026/8/4 14:50:10');
  assert.equal(formatTimestamp('2026-08-04T06:50:10.000000Z', 'America/New_York'), '2026/8/4 02:50:10');
});

test('today range uses configured calendar date with a half-open next-day boundary', () => {
  const now = new Date('2026-03-08T05:30:00Z');
  assert.deepEqual(getTodayTimeRange('America/New_York', now), {
    start_date: '2026-03-08T00:00', end_date: '2026-03-09T00:00'
  });
});

test('recent range shifts calendar dates instead of subtracting fixed 24 hour durations', () => {
  const now = new Date('2026-11-01T17:00:00Z');
  assert.deepEqual(getRecentDaysRange(7, 'America/New_York', now), {
    start_date: '2026-10-26T00:00', end_date: '2026-11-02T00:00'
  });
});

test('invalid or missing timezone fails explicitly', () => {
  assert.throws(() => validateTimezone(''), /未返回活动时区/);
  assert.throws(() => validateTimezone('Mars\/Olympus'), /无效时区/);
});

test('parseTimestamp accepts absolute instants and rejects ambiguous strings', () => {
  assert.equal(parseTimestamp('2026-08-13T00:00:01.000000Z').getTime(), Date.parse('2026-08-13T00:00:01.000000Z'));
  assert.equal(parseTimestamp('2026-08-13T08:00:00+08:00').getTime(), Date.parse('2026-08-13T08:00:00+08:00'));

  assert.throws(() => parseTimestamp(''), /时间为空/);
  assert.throws(() => parseTimestamp('2026-08-13 00:00:01'), /缺少时区/);
  assert.throws(() => parseTimestamp('not-a-date'), /缺少时区/);
  assert.throws(() => parseTimestamp('2026-99-99T00:00:00Z'), /无效时间/);
});
