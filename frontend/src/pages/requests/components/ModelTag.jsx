// ============================================
// ModelTag - 模型标签组件（带颜色区分）
// 2025-12-01 09:30:08
// ============================================

import { getModelColorClasses } from '../utils/modelTag.js';

/**
 * ModelTag - 显示模型名称的标签组件
 * @param {Object} props
 * @param {string} props.model - 模型名称
 */
const ModelTag = ({ model }) => {
  const colorClasses = getModelColorClasses(model);
  const displayName = (!model || model === 'unknown') ? '-' : model;

  return (
    <span className={`px-2 py-1 rounded text-xs font-mono border transition-all ${colorClasses}`}>
      {displayName}
    </span>
  );
};

export default ModelTag;
