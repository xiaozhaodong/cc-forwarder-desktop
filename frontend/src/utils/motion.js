// ============================================
// motion - 动效开关的统一判定
// 2026-08-29
// prefers-reduced-motion 有两个层次：CSS 侧的 @media 兜底，和 JS 侧
// 「干脆不下发动画」。后者更彻底 —— CSS 侧置 animation:none 会让
// animationend 永远不触发，靠该事件摘 class 的组件会永久留着 class
// 与内联 CSS 变量。能在 JS 侧拦掉的就别只靠 CSS 兜。
// ============================================

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)';

/**
 * 每次调用都重新查询，不缓存 MediaQueryList 的结果：
 * 用户可能在应用运行期间改系统设置，缓存会让改动直到重启才生效。
 */
export const prefersReducedMotion = () => (
  typeof window !== 'undefined'
  && typeof window.matchMedia === 'function'
  && window.matchMedia(REDUCED_MOTION_QUERY).matches
);

export default prefersReducedMotion;
