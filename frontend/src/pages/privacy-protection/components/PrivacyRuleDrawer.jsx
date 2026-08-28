// ============================================
// 隐私规则编辑抽屉
// 2026-06-11 (v6.1 新增)
// ============================================

import { useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { Button, Input } from '@components/ui';
import useModalLifecycle from '@hooks/useModalLifecycle.js';
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
  const isBuiltin = form.match_type === 'builtin';
  const closeButtonRef = useRef(null);
  const handleRequestClose = () => {
    if (!saving) onClose();
  };

  useModalLifecycle({
    open,
    onClose: handleRequestClose,
    initialFocusRef: closeButtonRef
  });

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
      <div className="absolute inset-0 bg-overlay animate-fade-in" onClick={handleRequestClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={isBuiltin ? '编辑内置规则' : (form.id > 0 ? '编辑规则' : '新增规则')}
        className="relative w-full max-w-xl h-full bg-surface shadow-2xl flex flex-col"
      >
        <div className="flex items-center justify-between px-6 py-4 border-b border-line-soft">
          <h3 className="text-base font-semibold text-fg">
            {isBuiltin ? '编辑内置规则' : (form.id > 0 ? '编辑规则' : '新增规则')}
          </h3>
          <button
            type="button"
            ref={closeButtonRef}
            aria-label="关闭抽屉"
            onClick={handleRequestClose}
            className="p-1.5 text-fg-subtle hover:text-fg-body hover:bg-surface-mut rounded-lg"
          >
            <X size={18} />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="flex-1 min-h-0 flex flex-col">
          <div className="flex-1 overflow-y-auto px-6 py-4 space-y-5">
            {isBuiltin ? (
              <div className="rounded-lg border border-accent-line bg-accent-soft px-3 py-2">
                <div className="text-xs font-medium text-accent">内置规则</div>
                <div className="mt-1 text-sm font-semibold text-fg">{form.name}</div>
                {form.description && (
                  <div className="mt-1 text-xs leading-5 text-fg-muted">{form.description}</div>
                )}
                <div className="mt-1 text-xs text-fg-subtle">优先级 {form.priority}</div>
              </div>
            ) : (
              <>
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
                  <label className="text-sm font-medium text-fg-body mb-1.5">描述</label>
                  <textarea
                    value={form.description}
                    onChange={(e) => update({ description: e.target.value })}
                    rows={2}
                    className="w-full px-3 py-2 border border-line rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-accent-ring"
                  />
                </div>
              </>
            )}

            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-fg-body">启用</span>
              <button
                type="button"
                onClick={() => update({ enabled: !form.enabled })}
                className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                  form.enabled ? 'bg-indigo-600' : 'bg-line-strong'
                }`}
              >
                <span
                  className={`inline-block h-3.5 w-3.5 transform rounded-full bg-surface shadow transition-transform ${
                    form.enabled ? 'translate-x-[18px]' : 'translate-x-[3px]'
                  }`}
                />
              </button>
            </div>

            {isBuiltin ? (
              <div className="rounded-lg border border-line bg-surface-sub px-3 py-2">
                <div className="text-sm font-medium text-fg-body">内置规则 ID</div>
                <div className="mt-1 font-mono text-sm text-fg-body">{form.pattern}</div>
                <div className="mt-1 text-xs text-fg-subtle">内置规则的匹配器由代码提供，只能调整启用、动作、占位符和作用域。</div>
              </div>
            ) : (
              <PrivacyPatternEditor
                matchType={form.match_type}
                pattern={form.pattern}
                error={errors.pattern}
                onMatchTypeChange={(value) => update({ match_type: value })}
                onPatternChange={(value) => update({ pattern: value })}
              />
            )}

            <div className="grid grid-cols-2 gap-4">
              <div className="flex flex-col">
                <label className="text-sm font-medium text-fg-body mb-1.5">动作</label>
                <div className="inline-flex rounded-lg border border-line p-0.5 bg-surface-sub self-start">
                  {PRIVACY_ACTION_OPTIONS.map((opt) => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => update({ action: opt.value })}
                      className={`px-3 py-1 rounded-md text-xs font-medium transition-colors ${
                        form.action === opt.value
                          ? 'bg-surface text-accent shadow-sm'
                          : 'text-fg-muted hover:text-fg-body'
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

            <div className="border-t border-line-soft pt-4">
              <h4 className="text-sm font-semibold text-fg-body mb-3">作用域</h4>
              <PrivacyScopeEditor
                scope={form.scope}
                onChange={(scope) => update({ scope })}
                endpointOptions={endpointOptions}
                accountOptions={accountOptions}
              />
            </div>
          </div>

          <div className="px-6 py-4 border-t border-line-soft bg-surface-sub">
            {submitError && (
              <p className="text-xs text-danger mb-2 break-all">{submitError}</p>
            )}
            <div className="flex justify-end gap-2">
              <Button type="button" variant="secondary" onClick={handleRequestClose} disabled={saving}>
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
