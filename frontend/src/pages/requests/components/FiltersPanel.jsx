import { useMemo } from 'react';
import { Calendar, Check, Filter, X } from 'lucide-react';
import { CustomSelect } from '@components/ui';
import { STATUS_SELECT_OPTIONS } from '../utils/constants.js';

const FAMILY_OPTIONS = [
  { value: '', label: '全部类型' },
  { value: 'claude', label: 'Claude' },
  { value: 'codex', label: 'Codex' },
  { value: 'image', label: 'Image' },
  { value: 'other', label: 'Other' }
];

const FiltersPanel = ({ isOpen, onClose, filters, updateFilter, onApply, onReset, models = [], upstreams = [] }) => {
  const modelOptions = useMemo(() => [
    { value: '', label: '所有模型' },
    ...models.map((model) => {
      const name = typeof model === 'string' ? model : (model.model_name || model.name || '');
      return { value: name, label: name };
    }).filter((option) => option.value)
  ], [models]);

  const upstreamOptions = useMemo(() => [
    { value: '', label: '所有上游' },
    ...upstreams.map((upstream) => ({ value: upstream, label: upstream }))
  ], [upstreams]);

  if (!isOpen) return null;
  return (
    <div className="absolute left-0 right-0 z-20 mt-3 rounded-xl border border-gray-200 bg-white p-5 shadow-xl ring-1 ring-black/5">
      <div className="mb-5 flex items-start justify-between">
        <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-900"><Filter className="h-4 w-4 text-indigo-500" />高级筛选</h3>
        <button type="button" onClick={onClose} className="text-gray-400 hover:text-gray-600" aria-label="关闭筛选"><X className="h-4 w-4" /></button>
      </div>
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <label className="min-w-[80px] text-sm font-medium text-gray-700">时间范围:</label>
          {['startDate', 'endDate'].map((field, index) => (
            <div key={field} className="flex items-center gap-2">
              {index === 1 && <span className="text-sm text-gray-400">至</span>}
              <div className="relative">
                <Calendar className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400" />
                <input type="datetime-local" value={filters[field]} onChange={(event) => updateFilter(field, event.target.value)} className="w-[200px] rounded-lg border border-gray-200 py-1.5 pl-9 pr-2 text-sm text-gray-600 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20" />
              </div>
            </div>
          ))}
        </div>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <CustomSelect options={STATUS_SELECT_OPTIONS} value={filters.status} onChange={(value) => updateFilter('status', value)} size="sm" placeholder="状态" />
          <CustomSelect options={modelOptions} value={filters.model} onChange={(value) => updateFilter('model', value)} size="sm" placeholder="模型" />
          <CustomSelect options={FAMILY_OPTIONS} value={filters.requestFamily === 'all' ? '' : filters.requestFamily} onChange={(value) => { updateFilter('requestFamily', value || 'all'); updateFilter('upstreamName', 'all'); }} size="sm" placeholder="类型" />
          <CustomSelect options={upstreamOptions} value={filters.upstreamName === 'all' ? '' : filters.upstreamName} onChange={(value) => updateFilter('upstreamName', value || 'all')} size="sm" placeholder="上游" />
        </div>
      </div>
      <div className="mt-4 flex items-center justify-between border-t border-gray-100 pt-4">
        <button type="button" onClick={onReset} className="text-sm text-gray-500 hover:text-gray-800">重置所有条件</button>
        <div className="flex gap-2"><button type="button" onClick={onClose} className="rounded-lg border border-gray-200 bg-white px-4 py-2 text-sm text-gray-600 hover:bg-gray-50">取消</button><button type="button" onClick={() => { onApply?.(); onClose?.(); }} className="flex items-center gap-2 rounded-lg bg-slate-900 px-4 py-2 text-sm text-white shadow-md hover:bg-slate-800"><Check className="h-3.5 w-3.5" />应用筛选</button></div>
      </div>
    </div>
  );
};

export default FiltersPanel;
