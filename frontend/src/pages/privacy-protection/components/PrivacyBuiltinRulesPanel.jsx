import { Pencil, ShieldCheck } from 'lucide-react';
import { EmptyState } from '@components/ui';
import { summarizeScope } from '../utils/privacyRules.js';

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

const actionLabel = (action) => (action === 'detect' ? '仅检测' : '脱敏');

const PrivacyBuiltinRulesPanel = ({ rules = [], busy, onToggle, onEdit }) => {
  if (!rules.length) {
    return (
      <EmptyState
        icon={ShieldCheck}
        title="暂无内置规则"
        description="启动后会自动补齐邮箱、手机号、身份证和银行卡内置规则"
      />
    );
  }

  return (
    <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white">
      <table className="w-full text-sm">
        <thead>
          <tr className="bg-slate-50 text-left text-xs text-slate-500">
            <th className="w-12 px-3 py-2 font-medium">启用</th>
            <th className="px-3 py-2 font-medium">内置规则</th>
            <th className="w-20 px-3 py-2 font-medium">动作</th>
            <th className="w-28 px-3 py-2 font-medium">占位符</th>
            <th className="px-3 py-2 font-medium">作用域</th>
            <th className="w-16 px-3 py-2 text-right font-medium">操作</th>
          </tr>
        </thead>
        <tbody>
          {rules.map((rule) => (
            <tr key={rule.id} className="border-t border-slate-100 hover:bg-slate-50/60">
              <td className="px-3 py-2">
                <Toggle checked={rule.enabled} disabled={busy} onChange={(enabled) => onToggle(rule, enabled)} />
              </td>
              <td className="px-3 py-2">
                <div className="font-medium text-slate-800">{rule.name}</div>
                <div className="mt-0.5 font-mono text-xs text-slate-400">{rule.pattern}</div>
                {rule.description && <div className="mt-0.5 text-xs text-slate-400">{rule.description}</div>}
                {rule.compile_error && <div className="mt-1 break-all text-xs text-rose-500">{rule.compile_error}</div>}
              </td>
              <td className="px-3 py-2 text-slate-600">{actionLabel(rule.action)}</td>
              <td className="break-all px-3 py-2 text-slate-600">{rule.action === 'redact' ? rule.placeholder : '-'}</td>
              <td className="px-3 py-2 text-xs text-slate-500">{summarizeScope(rule.scope)}</td>
              <td className="px-3 py-2 text-right">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => onEdit(rule)}
                  className="rounded-lg p-1.5 text-slate-400 hover:bg-indigo-50 hover:text-indigo-600"
                  title="编辑"
                >
                  <Pencil size={14} />
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default PrivacyBuiltinRulesPanel;
