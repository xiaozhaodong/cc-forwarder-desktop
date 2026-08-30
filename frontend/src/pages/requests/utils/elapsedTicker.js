// ============================================
// elapsedTicker - 进行中请求的「跑秒」时钟
// 2026-08-30
// 单个模块级定时器，所有跑秒 pill 共享。
// 订阅者归零时定时器彻底停掉 —— 没有进行中的请求时，
// 整张表必须是零定时器、零动画，这条不能被跑秒破坏。
// ============================================

// 显示精度是 0.1s（formatTimingBadge），刷新间隔必须比它细，
// 否则数字会跳格。100ms 是「看起来在连续跑」的下限。
export const TICK_INTERVAL_MS = 100;

/**
 * 计算当前段已耗时。
 *
 * @param {number} startedAt 请求开始的绝对毫秒时刻
 * @param {number} baseMs 当前段之前已经走完的毫秒数（首响位为 0）
 * @param {number} now 当前绝对毫秒时刻
 */
export const resolveElapsedMs = (startedAt, baseMs, now) => (
  // clamp >= 0：桌面端 start_time 与 Date.now() 同源于一个系统时钟，
  // 正常不会为负；但时间戳解析异常或系统时钟被改时不能显示负数。
  Math.max(now - startedAt - baseMs, 0)
);

const subscribers = new Set();
let timerId = 0;

const tick = () => {
  // 订阅者在回调里只写 textContent / className，不 setState。
  // 逐个 try 隔离：一个订阅者抛错不能让整张表的时钟停摆。
  for (const notify of subscribers) {
    try {
      notify();
    } catch (error) {
      console.error('跑秒回调异常:', error);
    }
  }
};

/**
 * 订阅共享时钟。
 * @param {() => void} notify 每 tick 调用一次
 * @returns {() => void} 取消订阅；最后一个订阅者退订时定时器被清掉
 */
export const subscribeElapsedTick = (notify) => {
  subscribers.add(notify);
  if (!timerId) {
    // 裸 setInterval 而非 window.setInterval：这个模块要能在 node 测试里跑。
    timerId = setInterval(tick, TICK_INTERVAL_MS);
  }

  return () => {
    subscribers.delete(notify);
    if (subscribers.size === 0 && timerId) {
      clearInterval(timerId);
      timerId = 0;
    }
  };
};

/** 仅供测试断言「没有订阅者时不留定时器」。 */
export const __getTickerState = () => ({ subscriberCount: subscribers.size, running: timerId !== 0 });
