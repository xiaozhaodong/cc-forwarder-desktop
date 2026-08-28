// ============================================
// 优先级编辑器组件
// 用于编辑端点优先级
// 2025-11-28
// ============================================

import { useState, useRef, useImperativeHandle, forwardRef } from 'react';
import { Minus, Plus } from 'lucide-react';

/**
 * 优先级编辑器组件
 * @param {number} priority - 当前优先级
 * @param {string} endpointName - 端点名称
 * @param {Function} onUpdate - 更新回调 (endpointName, newPriority) => Promise
 */
const PriorityEditor = forwardRef(({ priority = 1, endpointName, onUpdate }, ref) => {
  const [value, setValue] = useState(priority);
  const [isUpdating, setIsUpdating] = useState(false);
  const inputRef = useRef(null);

  // 暴露给父组件的方法
  useImperativeHandle(ref, () => ({
    executeUpdate: handleUpdate,
    isUpdating,
    getCurrentValue: () => value
  }));

  // 值变化是否需要更新
  const hasChanged = value !== priority;

  // 增加优先级
  const handleIncrease = () => {
    setValue(prev => prev + 1);
  };

  // 减少优先级
  const handleDecrease = () => {
    if (value > 1) {
      setValue(prev => prev - 1);
    }
  };

  // 手动输入
  const handleInputChange = (e) => {
    const newValue = parseInt(e.target.value) || 1;
    setValue(Math.max(1, newValue));
  };

  // 执行更新
  const handleUpdate = async () => {
    if (!hasChanged || isUpdating || !onUpdate) return;

    setIsUpdating(true);
    try {
      const result = await onUpdate(endpointName, value);
      if (result?.success) {
        // 更新成功，保持当前值
      } else {
        // 更新失败，回退值
        setValue(priority);
      }
    } catch (error) {
      console.error('优先级更新失败:', error);
      setValue(priority);
    } finally {
      setIsUpdating(false);
    }
  };

  // 键盘事件
  const handleKeyDown = (e) => {
    if (e.key === 'Enter') {
      handleUpdate();
    } else if (e.key === 'Escape') {
      setValue(priority);
    }
  };

  return (
    <div className="inline-flex items-center gap-0.5">
      {/* 减少按钮 */}
      <button
        onClick={handleDecrease}
        disabled={value <= 1 || isUpdating}
        className={`
          p-1 rounded transition-colors
          ${value <= 1 || isUpdating
            ? 'text-fg-subtle/60 cursor-not-allowed'
            : 'text-fg-subtle hover:text-fg-body hover:bg-surface-mut'}
        `}
        title="降低优先级"
      >
        <Minus size={12} />
      </button>

      {/* 输入框 */}
      <input
        ref={inputRef}
        type="number"
        min="1"
        value={value}
        onChange={handleInputChange}
        onKeyDown={handleKeyDown}
        onBlur={() => {
          if (hasChanged) handleUpdate();
        }}
        disabled={isUpdating}
        className={`
          w-10 h-7 text-center text-xs font-semibold rounded border transition-all
          ${hasChanged
            ? 'tone-amber border-warn-line'
            : 'border-line bg-surface-sub text-fg-body'}
          ${isUpdating ? 'opacity-50' : ''}
          focus:outline-none focus:ring-2 focus:ring-accent-soft focus:border-accent-ring
        `}
        title={hasChanged ? '有未保存的更改' : '当前优先级'}
      />

      {/* 增加按钮 */}
      <button
        onClick={handleIncrease}
        disabled={isUpdating}
        className={`
          p-1 rounded transition-colors
          ${isUpdating
            ? 'text-fg-subtle/60 cursor-not-allowed'
            : 'text-fg-subtle hover:text-fg-body hover:bg-surface-mut'}
        `}
        title="提高优先级"
      >
        <Plus size={12} />
      </button>

      {/* 更新指示器 */}
      {hasChanged && !isUpdating && (
        <span className="ml-1 w-1.5 h-1.5 rounded-full bg-warn-solid animate-pulse" title="有未保存的更改" />
      )}
    </div>
  );
});

PriorityEditor.displayName = 'PriorityEditor';

export default PriorityEditor;
