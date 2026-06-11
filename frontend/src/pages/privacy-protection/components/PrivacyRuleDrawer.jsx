// ============================================
// 隐私规则编辑抽屉
// 2026-06-11 (v6.1 新增)
// ============================================

import { useState } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { Button, Input } from '@components/ui';
import PrivacyPatternEditor from './PrivacyPatternEditor.jsx';
import PrivacyScopeEditor from './PrivacyScopeEditor.jsx';
import {
  PRIVACY_ACTION_OPTIONS,
  ruleToForm,
  validatePrivacyRuleForm
} from '../utils/privacyRules.js';

// 注意：调用方通过 key 在每次打开时重挂载本组件，初始 state 直接取自 rule
const PrivacyRuleDrawer = ({
  open,
  rule,
  saving,
  onSave,
  onClose,
  endpointOptions,
  accountOptions
}) => {
  const [form, setForm] = useState(() => ruleToForm(rule || {}));
  const [errors, setErrors] = useState({});
  const [submitError, setSubmitError] = useState('');

  if (!open) return null;

  const update = (patch) => {
    setForm((prev) => ({ ...prev, ...patch }));
    setErrors((prev) => {
      const next = { ...prev };
      Object.keys(patch).forEach((key) => delete next[key]);
      return next;
    });
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    const nextErrors = validatePrivacyRuleForm(form);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) return;

    setSubmitError('');
    try {
      await onSave(form);
    } catch (err) {
      // 后端编译失败等错误在抽屉内提示，不丢失用户输入
      setSubmitError(err.message || String(err));
    }
  };

  const nav = document.querySelector('nav.sticky');
  const topOffset = nav ? nav.getBoundingClientRect().bottom : 0;

  return createPortal(
    <div className="fixed inset-0 z-[45] flex justify-end" style={{ top: topOffset }}>
      <div className="absolute inset-0 bg-black/30 animate-fade-in" onClick={onClose} />
      <div className="relative w-full max-w-xl h-full bg-white shadow-2xl flex flex-col">
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-100">
          <h3 className="text-base font-semibold text-slate-800">
            {form.id > 0 ? '编辑规则' : '新增规则'}
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-slate-700 hover:bg-slate-100 rounded-lg"
          >
            <X size={18} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="flex-1 min-h-0 flex flex-col">
          <div className="flex-1 overflow-y-auto px-6 py-4 space-y-5">
            <div className="grid grid-cols-2 gap-4">
              <Input
                label="规则名 *"
                value={form.name}
                error={errors.name}
                onChange={(e) => update({ name: e.target.value })}
                placeholder="例如 OpenAI API Key"
              />
              <Input
                label="优先级（越小越先匹配）"
                type="number"
                value={form.priority}
                error={errors.priority}
                onChange={(e) => update({ priority: e.target.value })}
              />
            </div>

            <div className="flex flex-col">
              <label className="text-sm font-medium text-slate-700 mb-1.5">描述</label>
              <textarea
                value={form.description}
                onChange={(e) => update({ description: e.target.value })}
                rows={2}
                className="w-full px-3 py-2 border border-slate-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>

            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-slate-700">启用</span>
              <button
                type="button"
                onClick={() => update({ enabled: !form.enabled })}
                className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                  form.enabled ? 'bg-indigo-600' : 'bg-slate-200'
                }`}
              >
                <span
                  className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${
                    form.enabled ? 'translate-x-[18px]' : 'translate-x-[3px]'
                  }`}
                />
              </button>
            </div>

            <PrivacyPatternEditor
              matchType={form.match_type}
              pattern={form.pattern}
              error={errors.pattern}
              onMatchTypeChange={(value) => update({ match_type: value })}
              onPatternChange={(value) => update({ pattern: value })}
            />

            <div className="grid grid-cols-2 gap-4">
              <div className="flex flex-col">
                <label className="text-sm font-medium text-slate-700 mb-1.5">动作</label>
                <div className="inline-flex rounded-lg border border-slate-200 p-0.5 bg-slate-50 self-start">
                  {PRIVACY_ACTION_OPTIONS.map((opt) => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => update({ action: opt.value })}
                      className={`px-3 py-1 rounded-md text-xs font-medium transition-colors ${
                        form.action === opt.value
                          ? 'bg-white text-indigo-600 shadow-sm'
                          : 'text-slate-500 hover:text-slate-700'
                      }`}
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
              </div>
              {form.action === 'redact' && (
                <Input
                  label="占位符 *"
                  value={form.placeholder}
                  error={errors.placeholder}
                  onChange={(e) => update({ placeholder: e.target.value })}
                  placeholder="[已脱敏]"
                />
              )}
            </div>

            <div className="border-t border-slate-100 pt-4">
              <h4 className="text-sm font-semibold text-slate-700 mb-3">作用域</h4>
              <PrivacyScopeEditor
                scope={form.scope}
                onChange={(scope) => update({ scope })}
                endpointOptions={endpointOptions}
                accountOptions={accountOptions}
              />
            </div>
          </div>

          <div className="px-6 py-4 border-t border-slate-100 bg-slate-50/60">
            {submitError && (
              <p className="text-xs text-rose-500 mb-2 break-all">{submitError}</p>
            )}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={onClose} disabled={saving}>
                取消
              </Button>
              <Button type="submit" loading={saving}>
                保存并生效
              </Button>
            </div>
          </div>
        </form>
      </div>
    </div>,
    document.body
  );
};

export default PrivacyRuleDrawer;
