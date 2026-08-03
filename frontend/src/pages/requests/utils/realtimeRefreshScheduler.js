const defaultSetTimer = (callback, delay) => globalThis.setTimeout(callback, delay);
const defaultClearTimer = (timer) => globalThis.clearTimeout(timer);

/**
 * 创建实时刷新调度器。
 *
 * - 安静 250ms 后执行尾部刷新。
 * - 持续高频事件最多等待 1 秒，避免 trailing debounce 被饿死。
 * - 刷新进行中只记录 dirty，完成后再调度一次，避免并发查询。
 */
export const createRealtimeRefreshScheduler = ({
  refresh,
  debounceMs = 250,
  maxWaitMs = 1000,
  setTimer = defaultSetTimer,
  clearTimer = defaultClearTimer
}) => {
  let debounceTimer = null;
  let maxWaitTimer = null;
  let inFlight = false;
  let dirty = false;
  let stopped = false;

  const clearScheduledTimers = () => {
    if (debounceTimer !== null) {
      clearTimer(debounceTimer);
      debounceTimer = null;
    }
    if (maxWaitTimer !== null) {
      clearTimer(maxWaitTimer);
      maxWaitTimer = null;
    }
  };

  let armTimers;

  const flush = async () => {
    clearScheduledTimers();
    if (stopped || inFlight || !dirty) {
      return;
    }

    dirty = false;
    inFlight = true;
    try {
      await refresh();
    } finally {
      inFlight = false;
      if (dirty && !stopped) {
        armTimers();
      }
    }
  };

  armTimers = () => {
    if (stopped || inFlight || !dirty) {
      return;
    }

    if (maxWaitTimer === null) {
      maxWaitTimer = setTimer(() => {
        void flush();
      }, maxWaitMs);
    }

    if (debounceTimer !== null) {
      clearTimer(debounceTimer);
    }
    debounceTimer = setTimer(() => {
      void flush();
    }, debounceMs);
  };

  return {
    schedule() {
      if (stopped) return;
      dirty = true;
      armTimers();
    },

    triggerNow() {
      if (stopped) return Promise.resolve();
      dirty = true;
      return flush();
    },

    cancel() {
      stopped = true;
      dirty = false;
      clearScheduledTimers();
    }
  };
};

export default createRealtimeRefreshScheduler;
