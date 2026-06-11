// ============================================
// 隐私规则筛选工具条
// 2026-06-11 (v6.1 新增)
// ============================================

import { Search } from 'lucide-react';
import { CustomSelect } from '@components/ui';
import {
  PRIVACY_ACTION_OPTIONS,
  PRIVACY_MATCH_TYPE_OPTIONS
} from '../utils/privacyRules.js';

const ENABLED_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'enabled', label: '已启用' },
  { value: 'disabled', label: '已禁用' }
];

const SOURCE_OPTIONS = [
  { value: '', label: '全部来源' },
  { value: 'custom', label: '自定义' },
  { value: 'preset', label: '预设' }
];

const PrivacyRulesToolbar = ({ filters, onChange, total, filtered }) => {
  const update = (patch) => onChange({ ...filters, ...patch });

  return (
    <div className="flex flex-wrap items-center gap-2 mb-3">
      <div className="relative flex-1 min-w-[180px]">
        <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
        <input
          value={filters.keyword || ''}
          onChange={(e) => update({ keyword: e.target.value })}
          placeholder="搜索规则名 / 描述 / pattern"
          className="w-full pl-8 pr-3 py-1.5 border border-slate-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
        />
      </div>
      <CustomSelect
        options={ENABLED_OPTIONS}
        value={filters.enabled || ''}
        onChange={(value) => update({ enabled: value })}
        className="w-28"
      />
      <CustomSelect
        options={[{ value: '', label: '全部类型' }, ...PRIVACY_MATCH_TYPE_OPTIONS]}
        value={filters.matchType || ''}
        onChange={(value) => update({ matchType: value })}
        className="w-28"
      />
      <CustomSelect
        options={[{ value: '', label: '全部动作' }, ...PRIVACY_ACTION_OPTIONS]}
        value={filters.action || ''}
        onChange={(value) => update({ action: value })}
        className="w-28"
      />
      <CustomSelect
        options={SOURCE_OPTIONS}
        value={filters.source || ''}
        onChange={(value) => update({ source: value })}
        className="w-28"
      />
      <span className="text-xs text-slate-400 ml-auto">
        {filtered} / {total} 条
      </span>
    </div>
  );
};

export default PrivacyRulesToolbar;
