// ============================================
// 隐私规则表格
// 2026-06-11 (v6.1 新增)
// ============================================

import { ArrowDown, ArrowUp, Copy, Pencil, Trash2 } from 'lucide-react';
import { EmptyState } from '@components/ui';
import { ShieldOff } from 'lucide-react';
import { summarizeScope } from '../utils/privacyRules.js';

const matchTypeLabel = (matchType) => (matchType === 'literal' ? '字面' : '正则');
const actionLabel = (action) => (action === 'detect' ? '仅检测' : '脱敏');

const Toggle = ({ checked, onChange, disabled }) => (
  <button
    type="button"
    disabled={disabled}
    onClick={() => onChange(!checked)}
    className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors ${
      checked ? 'bg-indigo-600' : 'bg-slate-200'
    } ${disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
  >
    <span
      className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${
        checked ? 'translate-x-[18px]' : 'translate-x-[3px]'
      }`}
    />
  </button>
);

const CompileStatusDot = ({ compileError }) => (
  <span className="inline-flex items-center" title={compileError || '编译正常'}>
    <span
      className={`inline-block w-2 h-2 rounded-full ${
        compileError ? 'bg-rose-500' : 'bg-emerald-500'
      }`}
    />
  </span>
);

const PrivacyRulesTable = ({
  rules,
  busy,
  onToggle,
  onEdit,
  onDuplicate,
  onDelete,
  onMove,
  reorderEnabled
}) => {
  if (!rules.length) {
    return (
      <EmptyState
        icon={ShieldOff}
        title="暂无规则"
        description="点击右上角“新增规则”或“导入预设”开始配置出站隐私保护"
      />
    );
  }

  return (
    <div className="overflow-x-auto border border-slate-200 rounded-xl bg-white">
      <table className="w-full text-sm">
        <thead>
          <tr className="bg-slate-50 text-left text-xs text-slate-500">
            <th className="px-3 py-2 font-medium w-12">启用</th>
            <th className="px-3 py-2 font-medium w-20">优先级</th>
            <th className="px-3 py-2 font-medium">规则名</th>
            <th className="px-3 py-2 font-medium w-14">类型</th>
            <th className="px-3 py-2 font-medium w-16">动作</th>
            <th className="px-3 py-2 font-medium w-28">占位符</th>
            <th className="px-3 py-2 font-medium">作用域</th>
            <th className="px-3 py-2 font-medium w-12">状态</th>
            <th className="px-3 py-2 font-medium w-28 text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          {rules.map((rule, index) => (
            <tr key={rule.id} className="border-t border-slate-100 hover:bg-slate-50/60">
              <td className="px-3 py-2">
                <Toggle
                  checked={rule.enabled}
                  disabled={busy}
                  onChange={(enabled) => onToggle(rule, enabled)}
                />
              </td>
              <td className="px-3 py-2">
                <div className="flex items-center gap-1">
                  <span className="tabular-nums text-slate-600">{rule.priority}</span>
                  {reorderEnabled && (
                    <span className="flex flex-col">
                      <button
                        type="button"
                        disabled={busy || index === 0}
                        onClick={() => onMove(rule, -1)}
                        className="text-slate-300 hover:text-slate-600 disabled:opacity-30"
                        title="上移"
                      >
                        <ArrowUp size={12} />
                      </button>
                      <button
                        type="button"
                        disabled={busy || index === rules.length - 1}
                        onClick={() => onMove(rule, 1)}
                        className="text-slate-300 hover:text-slate-600 disabled:opacity-30"
                        title="下移"
                      >
                        <ArrowDown size={12} />
                      </button>
                    </span>
                  )}
                </div>
              </td>
              <td className="px-3 py-2">
                <div className="font-medium text-slate-800 break-all">{rule.name}</div>
                {rule.description && (
                  <div className="text-xs text-slate-400 break-all line-clamp-1">{rule.description}</div>
                )}
              </td>
              <td className="px-3 py-2 text-slate-600">{matchTypeLabel(rule.match_type)}</td>
              <td className="px-3 py-2">
                <span
                  className={`px-1.5 py-0.5 rounded text-xs ${
                    rule.action === 'detect'
                      ? 'bg-amber-50 text-amber-600'
                      : 'bg-indigo-50 text-indigo-600'
                  }`}
                >
                  {actionLabel(rule.action)}
                </span>
              </td>
              <td className="px-3 py-2 text-slate-600 break-all">{rule.action === 'redact' ? rule.placeholder : '-'}</td>
              <td className="px-3 py-2 text-xs text-slate-500">{summarizeScope(rule.scope)}</td>
              <td className="px-3 py-2">
                <CompileStatusDot compileError={rule.compile_error} />
              </td>
              <td className="px-3 py-2">
                <div className="flex items-center justify-end gap-1">
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => onEdit(rule)}
                    className="p-1.5 text-slate-400 hover:text-indigo-600 hover:bg-indigo-50 rounded-lg"
                    title="编辑"
                  >
                    <Pencil size={14} />
                  </button>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => onDuplicate(rule)}
                    className="p-1.5 text-slate-400 hover:text-indigo-600 hover:bg-indigo-50 rounded-lg"
                    title="复制"
                  >
                    <Copy size={14} />
                  </button>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => onDelete(rule)}
                    className="p-1.5 text-slate-400 hover:text-rose-600 hover:bg-rose-50 rounded-lg"
                    title="删除"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default PrivacyRulesTable;
