// ============================================
// useCountUp - 数值变化的平滑过渡
// 2026-08-28
// 统计指标在实时刷新下会频繁跳变，直接换数字看着像闪。
// rAF 驱动，从「当前显示值」滚到新值；被新值打断时从视觉当前位置续接，
// 不会回退到上一个终值再重滚。
// ============================================

import { useEffect, useRef, useState } from 'react';

const DEFAULT_DURATION_MS = 250;

const prefersReducedMotion = () => (
  typeof window !== 'undefined'
  && typeof window.matchMedia === 'function'
  && window.matchMedia('(prefers-reduced-motion: reduce)').matches
);

// easeOutCubic：起步快、收尾稳，数字滚动时最不容易看出跳变。
const easeOutCubic = (progress) => 1 - ((1 - progress) ** 3);

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

    if (prefersReducedMotion() || duration <= 0 || fromRef.current === target) {
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
