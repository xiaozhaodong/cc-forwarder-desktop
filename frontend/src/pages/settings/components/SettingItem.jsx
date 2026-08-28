// ============================================
// SettingItem - 可编辑的设置项组件
// v5.1.0 (2025-12-08)
// ============================================

import { useState, useEffect } from 'react';
import { AlertTriangle } from 'lucide-react';
import { CustomSelect } from '../../../components/ui';

// 值类型到输入类型的映射
const VALUE_TYPE_CONFIG = {
  string: { type: 'text', step: null },
  int: { type: 'number', step: 1 },
  float: { type: 'number', step: 0.1 },
  bool: { type: 'checkbox', step: null },
  duration: { type: 'text', step: null, placeholder: '例如: 30s, 5m, 1h' },
  password: { type: 'password', step: null }
};

// 策略类型选项
const STRATEGY_OPTIONS = [
  { value: 'priority', label: 'priority (优先级)' },
  { value: 'fastest', label: 'fastest (最快响应)' }
];

// 失败处理动作选项
const FAILURE_ACTION_OPTIONS = [
  { value: 'failover', label: 'failover - 故障转移到其他端点' },
  { value: 'reject', label: 'reject - 直接拒绝返回错误' }
];

const SettingItem = ({
  setting,
  value,
  onChange,
  disabled = false
}) => {
  const [localValue, setLocalValue] = useState(value);

  // 同步外部 value 变化
  useEffect(() => {
    setLocalValue(value);
  }, [value]);

  // 检测值类型（优先使用 setting.value_type，然后尝试自动检测）
  const valueType = setting.value_type || (
    (value === 'true' || value === 'false' || value === true || value === false) ? 'bool' : 'string'
  );

  const config = VALUE_TYPE_CONFIG[valueType] || VALUE_TYPE_CONFIG.string;
  const inputPlaceholder = valueType === 'password' && setting.secret_configured
    ? '已配置，留空则保持不变'
    : config.placeholder;

  // 显示标签（优先使用 label，没有则使用 key）
  const displayLabel = setting.label || setting.key;

  // 处理值变更
  const handleChange = (e) => {
    let newValue;
    if (valueType === 'bool') {
      newValue = e.target.checked ? 'true' : 'false';
    } else {
      newValue = e.target.value;
    }
    setLocalValue(newValue);
    onChange(setting.category, setting.key, newValue);
  };

  // 布尔类型使用 Toggle 开关
  if (valueType === 'bool') {
    const isChecked = localValue === 'true' || localValue === true;
    return (
      <div className="flex justify-between items-center py-3 border-b border-line-soft last:border-0">
        <div className="flex-1 min-w-0 pr-4">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-fg-body">{displayLabel}</span>
            {setting.requires_restart && (
              <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-warn-soft text-warn">
                <AlertTriangle size={10} className="mr-0.5" />
                重启生效
              </span>
            )}
          </div>
          {setting.description && (
            <p className="text-xs text-fg-subtle mt-0.5">{setting.description}</p>
          )}
        </div>
        <label className="relative inline-flex items-center cursor-pointer">
          <input
            type="checkbox"
            checked={isChecked}
            onChange={handleChange}
            disabled={disabled}
            className="sr-only peer"
          />
          <div className={`
            w-11 h-6 bg-surface-emph rounded-full peer
            peer-checked:bg-indigo-600
            peer-focus:ring-2 peer-focus:ring-accent-soft
            after:content-[''] after:absolute after:top-0.5 after:left-[2px]
            after:bg-surface after:rounded-full after:h-5 after:w-5
            after:transition-all after:shadow-sm
            peer-checked:after:translate-x-full
            ${disabled ? 'opacity-50 cursor-not-allowed' : ''}
          `}></div>
        </label>
      </div>
    );
  }

  // 策略类型使用下拉选择
  if (setting.key === 'type' && setting.category === 'strategy') {
    const handleStrategyChange = (newValue) => {
      setLocalValue(newValue);
      onChange(setting.category, setting.key, newValue);
    };

    return (
      <div className="flex justify-between items-center py-3 border-b border-line-soft last:border-0">
        <div className="flex-1 min-w-0 pr-4">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-fg-body">{displayLabel}</span>
            {setting.requires_restart && (
              <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-warn-soft text-warn">
                <AlertTriangle size={10} className="mr-0.5" />
                重启生效
              </span>
            )}
          </div>
          {setting.description && (
            <p className="text-xs text-fg-subtle mt-0.5">{setting.description}</p>
          )}
        </div>
        <CustomSelect
          options={STRATEGY_OPTIONS}
          value={localValue}
          onChange={handleStrategyChange}
          disabled={disabled}
          size="md"
        />
      </div>
    );
  }

  // 失败处理动作使用下拉选择
  if (setting.key === 'failure_action' && setting.category === 'request') {
    const handleActionChange = (newValue) => {
      setLocalValue(newValue);
      onChange(setting.category, setting.key, newValue);
    };

    return (
      <div className="flex justify-between items-center py-3 border-b border-line-soft last:border-0">
        <div className="flex-1 min-w-0 pr-4">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-fg-body">{displayLabel}</span>
            {setting.requires_restart && (
              <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-warn-soft text-warn">
                <AlertTriangle size={10} className="mr-0.5" />
                重启生效
              </span>
            )}
          </div>
          {setting.description && (
            <p className="text-xs text-fg-subtle mt-0.5">{setting.description}</p>
          )}
        </div>
        <CustomSelect
          options={FAILURE_ACTION_OPTIONS}
          value={localValue}
          onChange={handleActionChange}
          disabled={disabled}
          size="md"
        />
      </div>
    );
  }

  // 其他类型使用文本/数字输入框
  return (
    <div className="flex justify-between items-center py-3 border-b border-line-soft last:border-0">
      <div className="flex-1 min-w-0 pr-4">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-fg-body">{displayLabel}</span>
          {setting.requires_restart && (
            <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-warn-soft text-warn">
              <AlertTriangle size={10} className="mr-0.5" />
              重启生效
            </span>
          )}
        </div>
        {setting.description && (
          <p className="text-xs text-fg-subtle mt-0.5">{setting.description}</p>
        )}
      </div>
      <input
        type={config.type}
        autoComplete={valueType === 'password' ? 'new-password' : undefined}
        value={localValue}
        onChange={handleChange}
        disabled={disabled}
        step={setting.key === 'fixed_price_usd' ? 0.001 : config.step}
        placeholder={inputPlaceholder}
        className={`
          w-32 px-3 py-1.5 bg-surface-sub border border-line rounded-lg text-sm text-fg-body
          font-mono text-right
          focus:outline-none focus:ring-2 focus:ring-accent-ring focus:border-accent-line
          ${disabled ? 'opacity-50 cursor-not-allowed' : ''}
          ${setting.key === 'token' ? 'w-48' : ''}
          ${setting.key === 'endpoint_url' ? 'w-72 max-w-[58%]' : ''}
          ${setting.key === 'api_key' || setting.key === 'model' ? 'w-56 max-w-[50%]' : ''}
        `}
      />
    </div>
  );
};

export default SettingItem;
