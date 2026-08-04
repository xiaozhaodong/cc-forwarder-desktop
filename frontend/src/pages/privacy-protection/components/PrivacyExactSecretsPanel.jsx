import { useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { KeyRound, Pencil, Plus, Search, Trash2, Upload, X } from 'lucide-react';
import { Button, CustomSelect, EmptyState, Input } from '@components/ui';
import useModalLifecycle from '@hooks/useModalLifecycle.js';
import {
  PRIVACY_EXACT_SECRET_CATEGORY_OPTIONS,
  createEmptyExactSecretForm,
  exactSecretCategoryLabel,
  exactSecretMinLength,
  exactSecretToForm,
  filterPrivacyExactSecrets,
  validateExactSecretForm
} from '../utils/privacyRules.js';
import {
  ClearExactSecretsDialog,
  DeleteExactSecretDialog
} from './PrivacyExactSecretActionDialogs.jsx';

const EXACT_SECRET_STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'enabled', label: '已启用' },
  { value: 'disabled', label: '已禁用' }
];

const EXACT_SECRET_CATEGORY_FILTER_OPTIONS = [
  { value: '', label: '全部分类' },
  ...PRIVACY_EXACT_SECRET_CATEGORY_OPTIONS
];

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

const placeholderForCategory = (category) => {
  if (category === 'api_key') return '[API密钥]';
  if (category === 'token') return '[Token]';
  if (category === 'password') return '[密码]';
  return '[敏感值]';
};

const ExactSecretDialog = ({ open, secret, saving, onSave, onClose }) => {
  const [form, setForm] = useState(() => exactSecretToForm(secret || createEmptyExactSecretForm()));
  const [errors, setErrors] = useState({});
  const [submitError, setSubmitError] = useState('');
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

  const handleCategoryChange = (category) => {
    update({
      category,
      placeholder: form.placeholder && form.placeholder !== placeholderForCategory(form.category)
        ? form.placeholder
        : placeholderForCategory(category)
    });
  };

  const handleSubmit = async (event) => {
    event.preventDefault();
    const nextErrors = validateExactSecretForm(form, { requireSecretValue: !(form.id > 0) });
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) return;
    setSubmitError('');
    try {
      await onSave(form);
      onClose();
    } catch (err) {
      const message = err.message || String(err);
      setSubmitError(message.includes('privacy_exact_secret_exists') ? '该敏感值已存在' : message);
    }
  };

  return createPortal(
    <div className="fixed inset-0 z-[60] flex items-start justify-center px-4 pt-[15vh]">
      <div className="absolute inset-0 bg-slate-900/40" />
      <form
        onSubmit={handleSubmit}
        role="dialog"
        aria-modal="true"
        aria-label={form.id > 0 ? '编辑本地敏感值' : '新增本地敏感值'}
        className="relative flex max-h-[75vh] w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl"
      >
        <div className="flex items-center justify-between border-b border-slate-100 px-6 py-4">
          <h3 className="text-lg font-semibold text-slate-900">
            {form.id > 0 ? '编辑本地敏感值' : '新增本地敏感值'}
          </h3>
          <button
            type="button"
            ref={closeButtonRef}
            onClick={handleRequestClose}
            disabled={saving}
            className="text-sm text-slate-400 hover:text-slate-600"
          >
            关闭
          </button>
        </div>

        <div className="flex-1 space-y-4 overflow-y-auto px-6 py-5">
          <Input
            label="名称 *"
            value={form.name}
            error={errors.name}
            onChange={(e) => update({ name: e.target.value })}
            placeholder="例如 生产 OpenAI Key"
          />
          <div className="grid grid-cols-2 gap-4">
            <div className="flex flex-col">
              <label className="mb-1.5 text-sm font-medium text-slate-700">分类</label>
              <CustomSelect
                options={PRIVACY_EXACT_SECRET_CATEGORY_OPTIONS}
                value={form.category}
                onChange={handleCategoryChange}
                size="md"
                className="w-full"
              />
            </div>
            <Input
              label="占位符 *"
              value={form.placeholder}
              error={errors.placeholder}
              onChange={(e) => update({ placeholder: e.target.value })}
            />
          </div>
          <Input
            label={form.id > 0 ? '替换敏感值' : '敏感值 *'}
            type="password"
            value={form.secret_value}
            error={errors.secret_value}
            onChange={(e) => update({ secret_value: e.target.value })}
            placeholder={form.id > 0 ? '留空则只更新名称/分类/占位符' : '粘贴完整敏感值'}
          />
          <div className="flex items-center justify-between rounded-lg border border-slate-200 px-3 py-2">
            <div>
              <div className="text-sm font-medium text-slate-700">启用</div>
              <div className="text-xs text-slate-400">关闭后保留记录但不参与出站扫描</div>
            </div>
            <Toggle checked={form.enabled} disabled={saving} onChange={(enabled) => update({ enabled })} />
          </div>
          <div className="flex flex-col">
            <label className="mb-1.5 text-sm font-medium text-slate-700">描述</label>
            <textarea
              value={form.description}
              onChange={(e) => update({ description: e.target.value })}
              rows={3}
              className="w-full rounded-lg border border-slate-200 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>
          {form.id > 0 && (
            <p className="text-xs text-slate-400">
              当前值不会回显；列表只展示掩码和长度。
            </p>
          )}
        </div>

        <div className="border-t border-slate-100 bg-white px-6 py-4">
          {submitError && <p className="mb-2 break-all text-xs text-rose-500">{submitError}</p>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={handleRequestClose} disabled={saving}>
              取消
            </Button>
            <Button type="submit" loading={saving}>
              保存并生效
            </Button>
          </div>
        </div>
      </form>
    </div>,
    document.body
  );
};

const ImportCandidatesDialog = ({ open, loading, candidates, onLoad, onImport, onClose }) => {
  const [importingKey, setImportingKey] = useState('');
  const [error, setError] = useState('');
  const closeButtonRef = useRef(null);
  const handleRequestClose = () => {
    if (!importingKey) onClose();
  };

  useModalLifecycle({
    open,
    onClose: handleRequestClose,
    initialFocusRef: closeButtonRef
  });

  if (!open) return null;

  const handleImport = async (candidate) => {
    const key = `${candidate.source_type}:${candidate.source_ref}`;
    setImportingKey(key);
    setError('');
    try {
      await onImport({
        source_type: candidate.source_type,
        source_ref: candidate.source_ref,
        name: candidate.name,
        category: candidate.category,
        placeholder: placeholderForCategory(candidate.category)
      });
    } catch (err) {
      const message = err.message || String(err);
      setError(message.includes('privacy_exact_secret_exists') ? '该敏感值已存在' : message);
    } finally {
      setImportingKey('');
    }
  };

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 pt-[12vh] animate-fade-in">
      <div
        role="dialog"
        aria-modal="true"
        aria-label="导入候选"
        className="mx-4 w-full max-w-2xl overflow-hidden rounded-2xl bg-white shadow-xl"
      >
        <div className="flex items-center justify-between border-b border-slate-100 px-5 py-4">
          <h3 className="flex items-center gap-2 text-base font-semibold text-slate-800">
            <Upload size={17} className="text-indigo-500" />
            导入候选
          </h3>
          <button
            type="button"
            ref={closeButtonRef}
            aria-label="关闭弹窗"
            onClick={handleRequestClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 hover:text-slate-700"
          >
            <X size={18} />
          </button>
        </div>
        <div className="max-h-[60vh] overflow-y-auto px-5 py-4">
          <div className="mb-3 flex justify-end">
            <Button type="button" size="sm" variant="secondary" onClick={onLoad} loading={loading}>
              刷新候选
            </Button>
          </div>
          {error && <p className="mb-3 break-all text-xs text-rose-500">{error}</p>}
          <div className="space-y-2">
            {candidates.map((candidate) => {
              const key = `${candidate.source_type}:${candidate.source_ref}`;
              return (
                <div key={key} className="flex items-center gap-3 rounded-lg border border-slate-200 px-3 py-2">
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium text-slate-800">{candidate.name}</div>
                    <div className="mt-0.5 text-xs text-slate-400">
                      {exactSecretCategoryLabel(candidate.category)} · {candidate.masked_value} · {candidate.value_length} 字符
                    </div>
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    variant={candidate.already_exists ? 'secondary' : 'primary'}
                    loading={importingKey === key}
                    disabled={candidate.already_exists || Boolean(importingKey)}
                    onClick={() => handleImport(candidate)}
                  >
                    {candidate.already_exists ? '已存在' : '导入'}
                  </Button>
                </div>
              );
            })}
          </div>
          {!loading && candidates.length === 0 && (
            <p className="py-6 text-center text-sm text-slate-400">暂无可导入候选</p>
          )}
        </div>
      </div>
    </div>,
    document.body
  );
};

const ManualImportDialog = ({ open, onImport, onClose }) => {
  const [form, setForm] = useState({
    namePrefix: '手动敏感值',
    category: 'api_key',
    placeholder: placeholderForCategory('api_key'),
    description: '',
    values: ''
  });
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState('');
  const closeButtonRef = useRef(null);
  const handleRequestClose = () => {
    if (!importing) onClose();
  };

  useModalLifecycle({
    open,
    onClose: handleRequestClose,
    initialFocusRef: closeButtonRef
  });

  if (!open) return null;

  const update = (patch) => {
    setForm((prev) => ({ ...prev, ...patch }));
    setError('');
  };

  const handleCategoryChange = (category) => {
    update({
      category,
      placeholder: form.placeholder && form.placeholder !== placeholderForCategory(form.category)
        ? form.placeholder
        : placeholderForCategory(category)
    });
  };

  const lines = () => Array.from(new Set(
    String(form.values || '')
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter(Boolean)
  ));

  const handleSubmit = async (event) => {
    event.preventDefault();
    const values = lines();
    if (values.length === 0) {
      setError('请粘贴至少一行敏感值');
      return;
    }
    const minLength = exactSecretMinLength(form.category);
    const tooShort = values.find((value) => value.length < minLength);
    if (tooShort) {
      setError(`当前分类至少 ${minLength} 个字符，请检查过短条目`);
      return;
    }
    if (!String(form.placeholder || '').trim()) {
      setError('占位符不能为空');
      return;
    }

    const baseName = String(form.namePrefix || '').trim() || '手动敏感值';
    setImporting(true);
    setError('');
    let imported = 0;
    let skipped = 0;
    const failures = [];
    for (const [index, value] of values.entries()) {
      try {
        await onImport({
          source_type: 'manual',
          source_ref: '',
          name: values.length === 1 ? baseName : `${baseName} ${index + 1}`,
          category: form.category,
          placeholder: String(form.placeholder || '').trim(),
          description: String(form.description || '').trim(),
          secret_value: value
        });
        imported += 1;
      } catch (err) {
        const message = err.message || String(err);
        if (message.includes('privacy_exact_secret_exists')) {
          skipped += 1;
        } else {
          failures.push(`#${index + 1}: ${message}`);
        }
      }
    }
    setImporting(false);

    if (failures.length > 0 || skipped > 0) {
      const parts = [];
      if (imported > 0) parts.push(`已导入 ${imported} 条`);
      if (skipped > 0) parts.push(`跳过 ${skipped} 条重复`);
      if (failures.length > 0) parts.push(`失败 ${failures.length} 条：${failures.slice(0, 2).join('；')}`);
      setError(parts.join('，'));
      return;
    }

    setForm((prev) => ({ ...prev, values: '' }));
    onClose();
  };

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 pt-[10vh] animate-fade-in">
      <form
        onSubmit={handleSubmit}
        role="dialog"
        aria-modal="true"
        aria-label="粘贴导入"
        className="mx-4 flex max-h-[80vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl bg-white shadow-xl"
      >
        <div className="flex items-center justify-between border-b border-slate-100 px-5 py-4">
          <h3 className="flex items-center gap-2 text-base font-semibold text-slate-800">
            <Upload size={17} className="text-indigo-500" />
            粘贴导入
          </h3>
          <button
            type="button"
            ref={closeButtonRef}
            aria-label="关闭弹窗"
            onClick={handleRequestClose}
            className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-100 hover:text-slate-700"
          >
            <X size={18} />
          </button>
        </div>

        <div className="flex-1 space-y-4 overflow-y-auto px-5 py-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Input
              label="名称前缀"
              value={form.namePrefix}
              onChange={(e) => update({ namePrefix: e.target.value })}
            />
            <div className="flex flex-col">
              <label className="mb-1.5 text-sm font-medium text-slate-700">分类</label>
              <CustomSelect
                options={PRIVACY_EXACT_SECRET_CATEGORY_OPTIONS}
                value={form.category}
                onChange={handleCategoryChange}
                size="md"
                className="w-full"
              />
            </div>
            <Input
              label="占位符 *"
              value={form.placeholder}
              onChange={(e) => update({ placeholder: e.target.value })}
            />
          </div>
          <div className="flex flex-col">
            <label className="mb-1.5 text-sm font-medium text-slate-700">敏感值列表 *</label>
            <textarea
              value={form.values}
              onChange={(e) => update({ values: e.target.value })}
              rows={9}
              placeholder="每行一个完整敏感值，空行会自动跳过"
              className="w-full rounded-lg border border-slate-200 px-3 py-2 font-mono text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
            <div className="mt-1 text-xs text-slate-400">导入前会逐行 TrimSpace，并按完整字符串精确匹配。</div>
          </div>
          <div className="flex flex-col">
            <label className="mb-1.5 text-sm font-medium text-slate-700">描述</label>
            <textarea
              value={form.description}
              onChange={(e) => update({ description: e.target.value })}
              rows={2}
              className="w-full rounded-lg border border-slate-200 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>
        </div>

        <div className="border-t border-slate-100 bg-slate-50/70 px-5 py-4">
          {error && <p className="mb-2 break-all text-xs text-rose-500">{error}</p>}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={handleRequestClose} disabled={importing}>
              取消
            </Button>
            <Button type="submit" loading={importing}>
              导入并生效
            </Button>
          </div>
        </div>
      </form>
    </div>,
    document.body
  );
};

const PrivacyExactSecretsPanel = ({
  secrets = [],
  busy,
  onSave,
  onDelete,
  onClear,
  onLoadCandidates,
  onImportCandidate
}) => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogSecret, setDialogSecret] = useState(null);
  const [dialogKey, setDialogKey] = useState(0);
  const [importOpen, setImportOpen] = useState(false);
  const [manualImportOpen, setManualImportOpen] = useState(false);
  const [candidateLoading, setCandidateLoading] = useState(false);
  const [candidates, setCandidates] = useState([]);
  const [filters, setFilters] = useState({});
  const [panelError, setPanelError] = useState('');
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  const [clearOpen, setClearOpen] = useState(false);
  const [clearBusy, setClearBusy] = useState(false);
  const filteredSecrets = useMemo(() => filterPrivacyExactSecrets(secrets, filters), [secrets, filters]);
  const actionBusy = busy || deleteBusy || clearBusy;

  const updateFilters = (patch) => {
    setFilters((prev) => ({ ...prev, ...patch }));
  };

  const openDialog = (secret) => {
    setDialogSecret(secret);
    setDialogKey((key) => key + 1);
    setDialogOpen(true);
  };

  const handleToggle = async (secret, enabled) => {
    setPanelError('');
    try {
      await onSave({ ...secret, enabled, secret_value: '' });
    } catch (err) {
      setPanelError(err.message || String(err));
    }
  };

  const handleDelete = (secret) => {
    setPanelError('');
    setDeleteTarget(secret);
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    setDeleteBusy(true);
    setPanelError('');
    try {
      await onDelete(deleteTarget.id);
      setDeleteTarget(null);
    } catch (err) {
      setPanelError(err.message || String(err));
    } finally {
      setDeleteBusy(false);
    }
  };

  const confirmClear = async (confirmText) => {
    setClearBusy(true);
    setPanelError('');
    try {
      await onClear(confirmText);
      setClearOpen(false);
    } catch (err) {
      setPanelError(err.message || String(err));
    } finally {
      setClearBusy(false);
    }
  };

  const loadCandidates = async () => {
    setCandidateLoading(true);
    setPanelError('');
    try {
      setCandidates(await onLoadCandidates());
    } catch (err) {
      setPanelError(err.message || String(err));
    } finally {
      setCandidateLoading(false);
    }
  };

  const openImport = () => {
    setImportOpen(true);
    loadCandidates();
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" icon={Plus} onClick={() => openDialog(createEmptyExactSecretForm())} disabled={actionBusy}>
          新增敏感值
        </Button>
        <Button size="sm" variant="secondary" icon={Upload} onClick={openImport} disabled={actionBusy}>
          从账号/端点导入
        </Button>
        <Button size="sm" variant="secondary" icon={Upload} onClick={() => setManualImportOpen(true)} disabled={actionBusy}>
          粘贴导入
        </Button>
        <Button size="sm" variant="secondary" icon={Trash2} onClick={() => setClearOpen(true)} disabled={actionBusy || secrets.length === 0}>
          清空
        </Button>
        <span className="text-xs text-slate-400">完整敏感值不在列表中展示，也不会导出到规则 JSON。</span>
      </div>
      {panelError && <p className="break-all text-sm text-rose-500">{panelError}</p>}

      {secrets.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 rounded-xl border border-slate-200 bg-white px-3 py-2">
          <div className="relative min-w-[220px] flex-1">
            <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
            <input
              value={filters.keyword || ''}
              onChange={(e) => updateFilters({ keyword: e.target.value })}
              placeholder="搜索名称 / 描述 / 掩码 / 来源"
              className="w-full rounded-lg border border-slate-200 py-1.5 pl-8 pr-3 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>
          <CustomSelect
            options={EXACT_SECRET_CATEGORY_FILTER_OPTIONS}
            value={filters.category || ''}
            onChange={(category) => updateFilters({ category })}
            className="w-28"
          />
          <CustomSelect
            options={EXACT_SECRET_STATUS_OPTIONS}
            value={filters.enabled || ''}
            onChange={(enabled) => updateFilters({ enabled })}
            className="w-28"
          />
          <span className="ml-auto text-xs text-slate-400">
            {filteredSecrets.length} / {secrets.length} 条
          </span>
        </div>
      )}

      {secrets.length === 0 ? (
        <EmptyState
          icon={KeyRound}
          title="暂无本地敏感值"
          description="手动加入真实 key、token、密码或固定敏感文本后，才会按精确字符串替换"
        />
      ) : filteredSecrets.length === 0 ? (
        <EmptyState
          icon={Search}
          title="没有匹配的本地敏感值"
          description="调整搜索词、分类或状态筛选后再看"
        />
      ) : (
        <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-slate-50 text-left text-xs text-slate-500">
                <th className="w-12 px-3 py-2 font-medium">启用</th>
                <th className="px-3 py-2 font-medium">名称</th>
                <th className="w-24 px-3 py-2 font-medium">分类</th>
                <th className="px-3 py-2 font-medium">掩码值</th>
                <th className="w-28 px-3 py-2 font-medium">占位符</th>
                <th className="w-28 px-3 py-2 font-medium">来源</th>
                <th className="w-24 px-3 py-2 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {filteredSecrets.map((secret) => (
                <tr key={secret.id} className="border-t border-slate-100 hover:bg-slate-50/60">
                  <td className="px-3 py-2">
                    <Toggle checked={secret.enabled} disabled={actionBusy} onChange={(enabled) => handleToggle(secret, enabled)} />
                  </td>
                  <td className="px-3 py-2">
                    <div className="font-medium text-slate-800">{secret.name}</div>
                    {secret.description && <div className="line-clamp-1 text-xs text-slate-400">{secret.description}</div>}
                  </td>
                  <td className="px-3 py-2 text-slate-600">{exactSecretCategoryLabel(secret.category)}</td>
                  <td className="px-3 py-2 text-xs text-slate-500">
                    <span className="font-mono">{secret.masked_value}</span>
                    <span className="ml-2 text-slate-300">{secret.value_length} 字符</span>
                  </td>
                  <td className="break-all px-3 py-2 text-slate-600">{secret.placeholder}</td>
                  <td className="px-3 py-2 text-xs text-slate-400">{secret.source_type}</td>
                  <td className="px-3 py-2">
                    <div className="flex justify-end gap-1">
                      <button
                        type="button"
                        disabled={actionBusy}
                        onClick={() => openDialog(exactSecretToForm(secret))}
                        className="rounded-lg p-1.5 text-slate-400 hover:bg-indigo-50 hover:text-indigo-600"
                        title="编辑"
                      >
                        <Pencil size={14} />
                      </button>
                      <button
                        type="button"
                        disabled={actionBusy}
                        onClick={() => handleDelete(secret)}
                        className="rounded-lg p-1.5 text-slate-400 hover:bg-rose-50 hover:text-rose-600"
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
      )}

      {deleteTarget && (
        <DeleteExactSecretDialog
          secret={deleteTarget}
          loading={deleteBusy}
          onConfirm={confirmDelete}
          onClose={() => setDeleteTarget(null)}
        />
      )}
      {clearOpen && (
        <ClearExactSecretsDialog
          loading={clearBusy}
          onConfirm={confirmClear}
          onClose={() => setClearOpen(false)}
        />
      )}

      <ExactSecretDialog
        key={dialogKey}
        open={dialogOpen}
        secret={dialogSecret}
        saving={actionBusy}
        onSave={onSave}
        onClose={() => setDialogOpen(false)}
      />
      <ManualImportDialog
        open={manualImportOpen}
        onImport={onImportCandidate}
        onClose={() => setManualImportOpen(false)}
      />
      <ImportCandidatesDialog
        open={importOpen}
        loading={candidateLoading}
        candidates={candidates}
        onLoad={loadCandidates}
        onImport={async (input) => {
          await onImportCandidate(input);
          await loadCandidates();
        }}
        onClose={() => setImportOpen(false)}
      />
    </div>
  );
};

export default PrivacyExactSecretsPanel;
