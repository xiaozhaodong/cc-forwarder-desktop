// ============================================
// DynamicConfigSection - 动态配置区块组件
// 用于显示未预设的配置section
// 2025-12-01
// ============================================

import { Settings } from 'lucide-react';
import ConfigSection from './ConfigSection.jsx';

const DynamicConfigSection = ({ sectionName, sectionData }) => {
  // 格式化纳秒为可读时间格式
  const formatDuration = (ns) => {
    const seconds = ns / 1000000000;
    if (seconds < 1) return `${Math.round(ns / 1000000)}ms`;
    if (seconds < 60) return `${Math.round(seconds)}s`;
    if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
    return `${Math.round(seconds / 3600)}h`;
  };

  // 判断是否为duration类型（纳秒数）
  const isDuration = (key, value) => {
    if (typeof value !== 'number') return false;
    const durationKeys = [
      'interval', 'timeout', 'delay', 'duration', 'ttl',
      'cooldown', 'heartbeat', 'idle', 'lifetime'
    ];
    return durationKeys.some(k => key.toLowerCase().includes(k));
  };

  // 判断值的类型
  const detectType = (key, value) => {
    if (typeof value === 'boolean') return 'boolean';
    if (isDuration(key, value)) return 'duration';
    if (typeof value === 'number') return 'number';
    if (typeof value === 'string' && /^\d+[smh]$/.test(value)) return 'duration';
    return 'text';
  };

  // 格式化section名称为标题
  const formatTitle = (name) => {
    // 将下划线转换为空格并首字母大写
    return name
      .split('_')
      .map(word => word.charAt(0).toUpperCase() + word.slice(1))
      .join(' ');
  };

  // 渲染嵌套对象
  const renderValue = (key, value) => {
    if (value === null || value === undefined) {
      return <span className="text-slate-400">-</span>;
    }

    if (typeof value === 'object' && !Array.isArray(value)) {
      // 嵌套对象，递归显示
      return (
        <div className="mt-2 space-y-2">
          {Object.entries(value).map(([nestedKey, val]) => (
            <div key={nestedKey} className="ml-4 flex justify-between items-start py-2 border-l-2 border-slate-200 pl-3">
              <span className="text-xs text-slate-500">{nestedKey}</span>
              {renderValue(nestedKey, val)}
            </div>
          ))}
        </div>
      );
    }

    if (Array.isArray(value)) {
      return (
        <span className="text-xs text-slate-600 bg-slate-100 px-2 py-1 rounded">
          Array ({value.length} items)
        </span>
      );
    }

    const type = detectType(key, value);
    if (type === 'boolean') {
      return (
        <span className={`px-2 py-0.5 rounded text-xs font-medium ${
          value ? 'bg-emerald-100 text-emerald-700' : 'bg-slate-100 text-slate-600'
        }`}>
          {value ? '启用' : '禁用'}
        </span>
      );
    }

    if (type === 'duration') {
      const displayValue = typeof value === 'number' ? formatDuration(value) : value;
      return <span className="font-mono text-xs text-indigo-600">{displayValue}</span>;
    }

    if (type === 'number') {
      return <span className="font-mono text-xs text-slate-700">{value}</span>;
    }

    return <span className="font-mono text-xs text-slate-700 break-all">{String(value)}</span>;
  };

  if (!sectionData || typeof sectionData !== 'object') {
    return null;
  }

  return (
    <ConfigSection title={formatTitle(sectionName)} icon={Settings}>
      <div className="space-y-1">
        {Object.entries(sectionData).map(([key, value]) => (
          <div key={key} className="py-2 border-b border-slate-100 last:border-0">
            <div className="flex justify-between items-start">
              <span className="text-sm text-slate-600">{key}</span>
              {renderValue(key, value)}
            </div>
          </div>
        ))}
      </div>
    </ConfigSection>
  );
};

export default DynamicConfigSection;
