import test from 'node:test';
import assert from 'node:assert/strict';
import { createRealtimeRefreshScheduler } from './realtimeRefreshScheduler.js';

const createFakeClock = () => {
  let now = 0;
  let nextId = 1;
  const timers = new Map();

  const setTimer = (callback, delay) => {
    const id = nextId++;
    timers.set(id, { callback, at: now + delay });
    return id;
  };

  const clearTimer = (id) => {
    timers.delete(id);
  };

  const advance = (duration) => {
    const target = now + duration;
    while (true) {
      let selectedId = null;
      let selectedTimer = null;
      for (const [id, timer] of timers.entries()) {
        if (timer.at <= target && (!selectedTimer || timer.at < selectedTimer.at)) {
          selectedId = id;
          selectedTimer = timer;
        }
      }
      if (!selectedTimer) break;
      now = selectedTimer.at;
      timers.delete(selectedId);
      selectedTimer.callback();
    }
    now = target;
  };

  return { setTimer, clearTimer, advance };
};

test('realtime refresh scheduler flushes after the trailing debounce window', async () => {
  const clock = createFakeClock();
  let refreshCount = 0;
  const scheduler = createRealtimeRefreshScheduler({
    refresh: async () => { refreshCount += 1; },
    setTimer: clock.setTimer,
    clearTimer: clock.clearTimer
  });

  scheduler.schedule();
  clock.advance(249);
  assert.equal(refreshCount, 0);
  clock.advance(1);
  await Promise.resolve();
  assert.equal(refreshCount, 1);
});

test('realtime refresh scheduler cannot be starved by continuous events', async () => {
  const clock = createFakeClock();
  let refreshCount = 0;
  const scheduler = createRealtimeRefreshScheduler({
    refresh: async () => { refreshCount += 1; },
    setTimer: clock.setTimer,
    clearTimer: clock.clearTimer
  });

  scheduler.schedule();
  for (let i = 0; i < 9; i += 1) {
    clock.advance(100);
    scheduler.schedule();
  }
  assert.equal(refreshCount, 0);

  clock.advance(100);
  await Promise.resolve();
  assert.equal(refreshCount, 1);
});

test('realtime refresh scheduler keeps refreshes single-flight and replays dirty work', async () => {
  const clock = createFakeClock();
  let refreshCount = 0;
  let resolveFirstRefresh;
  const firstRefresh = new Promise((resolve) => {
    resolveFirstRefresh = resolve;
  });
  const scheduler = createRealtimeRefreshScheduler({
    refresh: async () => {
      refreshCount += 1;
      if (refreshCount === 1) {
        await firstRefresh;
      }
    },
    setTimer: clock.setTimer,
    clearTimer: clock.clearTimer
  });

  const firstRun = scheduler.triggerNow();
  assert.equal(refreshCount, 1);
  scheduler.schedule();
  clock.advance(1000);
  assert.equal(refreshCount, 1);

  resolveFirstRefresh();
  await firstRun;
  clock.advance(249);
  assert.equal(refreshCount, 1);
  clock.advance(1);
  await Promise.resolve();
  assert.equal(refreshCount, 2);
});
