// ============================================
// useCountUp - 数值变化的平滑过渡
// 2026-08-28
// 统计指标在实时刷新下会频繁跳变，直接换数字看着像闪。
// rAF 驱动，从「当前显示值」滚到新值；被新值打断时从视觉当前位置续接，
// 不会回退到上一个终值再重滚。
// ============================================

import { useEffect, useRef, useState } from 'react';
import { prefersReducedMotion } from '@utils/motion.js';

const DEFAULT_DURATION_MS = 250;

// 变化幅度相对基数低于这个比例就直接落值，不滚。
// 请求页统计是 30d 口径，基数上万，一条新请求让总数 12847 -> 12848：
// 滚 250ms，而 Math.round 之后用户看到的是「12847 停约 200ms，然后跳 12848」——
// 动画没产生任何动效，只产生了延迟。总成本更糟，$12.3456 -> $12.3467
// 会让末几位在 250ms 里疯狂抖动，比直接换数字吵得多。
// 滚动只对「切筛选 / 换时间范围」这类大跳变有意义。
const MIN_RELATIVE_CHANGE = 0.02;

// easeOutCubic：起步快、收尾稳，数字滚动时最不容易看出跳变。
const easeOutCubic = (progress) => 1 - ((1 - progress) ** 3);

// 按相对幅度而不是绝对差值判定：成本（0.01 量级）与请求数（10^4 量级）
// 差四个数量级，统一的绝对阈值对其中一方必然是错的。
const isNegligibleChange = (from, to) => {
  const scale = Math.max(Math.abs(from), Math.abs(to));
  if (scale === 0) return true;
  return Math.abs(to - from) / scale < MIN_RELATIVE_CHANGE;
};

/**
 * @param {number|null|undefined} value 目标值；非有限数值时原样返回，不做动画
 * @param {{duration?: number}} options
 * @returns {number|null|undefined} 当前应显示的值
 */
export const useCountUp = (value, { duration = DEFAULT_DURATION_MS } = {}) => {
  const target = Number.isFinite(value) ? value : null;
  const [display, setDisplay] = useState(target ?? 0);
  const displayRef = useRef(target ?? 0);
  const fromRef = useRef(target ?? 0);
  const frameRef = useRef(null);
  const hasMountedRef = useRef(false);

  useEffect(() => {
    if (target === null) {
      return undefined;
    }

    const settle = () => {
      fromRef.current = target;
      displayRef.current = target;
      setDisplay(target);
    };

    // 首次挂载直接落终值，避免首屏所有指标一起滚动。
    if (!hasMountedRef.current) {
      hasMountedRef.current = true;
      settle();
      return undefined;
    }

    if (prefersReducedMotion() || duration <= 0 || isNegligibleChange(fromRef.current, target)) {
      settle();
      return undefined;
    }

    const from = fromRef.current;
    const startedAt = performance.now();

    const step = (now) => {
      const progress = Math.min((now - startedAt) / duration, 1);
      if (progress >= 1) {
        settle();
        frameRef.current = null;
        return;
      }
      const next = from + ((target - from) * easeOutCubic(progress));
      displayRef.current = next;
      setDisplay(next);
      frameRef.current = requestAnimationFrame(step);
    };

    frameRef.current = requestAnimationFrame(step);

    return () => {
      if (frameRef.current !== null) {
        cancelAnimationFrame(frameRef.current);
        frameRef.current = null;
      }
      // 下一轮从当前视觉位置续接，而不是从上一个终值重新滚。
      fromRef.current = displayRef.current;
    };
  }, [target, duration]);

  return target === null ? value : display;
};

export default useCountUp;
