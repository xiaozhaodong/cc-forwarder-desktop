// ============================================
// iOS 风格 Toggle Switch 组件
// 2025-11-28
// ============================================

import { useState } from 'react';

/**
 * iOS 风格滑动开关
 * @param {boolean} enabled - 是否开启
 * @param {boolean} disabled - 是否禁用
 * @param {Function} onChange - 状态变化回调
 * @param {string} title - 提示文字
 * @param {string} size - 尺寸 'sm' | 'md'
 */
const ToggleSwitch = ({
  enabled = false,
  disabled = false,
  onChange,
  title,
  size = 'sm'
}) => {
  const [loading, setLoading] = useState(false);

  const handleClick = async () => {
    if (disabled || loading) return;

    setLoading(true);
    try {
      await onChange?.();
    } catch (error) {
      console.error('Toggle 操作失败:', error);
    } finally {
      setLoading(false);
    }
  };

  // 尺寸配置
  const sizes = {
    sm: {
      track: 'w-9 h-5',
      thumb: 'w-4 h-4',
      translate: 'translate-x-4'
    },
    md: {
      track: 'w-11 h-6',
      thumb: 'w-5 h-5',
      translate: 'translate-x-5'
    }
  };

  const s = sizes[size] || sizes.sm;

  return (
    <button
      type="button"
      onClick={handleClick}
      disabled={disabled || loading}
      title={title}
      className={`
        relative inline-flex items-center shrink-0 rounded-full
        transition-colors duration-200 ease-in-out
        focus:outline-none focus-visible:ring-2 focus-visible:ring-success-solid focus-visible:ring-offset-2
        ${s.track}
        ${disabled
          ? 'bg-surface-mut cursor-not-allowed'
          : enabled
            ? 'bg-success-solid'
            : 'bg-surface-emph hover:bg-line-strong'}
        ${loading ? 'opacity-70' : ''}
      `}
      role="switch"
      aria-checked={enabled}
    >
      {/* 滑块 */}
      <span
        className={`
          pointer-events-none inline-block rounded-full bg-white shadow-lg
          ring-0 transition-transform duration-200 ease-in-out
          ${s.thumb}
          ${enabled ? s.translate : 'translate-x-0.5'}
          ${loading ? 'animate-pulse' : ''}
        `}
      />
    </button>
  );
};

export default ToggleSwitch;
