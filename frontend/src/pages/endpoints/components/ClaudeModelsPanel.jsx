import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertTriangle, Boxes, Plus, RefreshCw, Save, Trash2, X } from 'lucide-react';
import { createPortal } from 'react-dom';
import { Button } from '@components/ui';
import useModalLifecycle from '@hooks/useModalLifecycle.js';
import { getSettingsByCategory, isWailsEnvironment, updateSetting } from '@utils/wailsApi.js';

const CATEGORY = 'claude_models';
const CATALOG_KEY = 'catalog';
const MAX_MODELS = 1000;
const MAX_TEXT_LENGTH = 256;
const emptyCatalog = () => ({ enabled: true, models: [] });

// 行 key 必须与 model.id 解耦，否则输入 ID 时每个字符都会触发行重挂载导致失焦。
let modelUidSeed = 0;
const nextModelUid = () => {
  modelUidSeed += 1;
  return `m${modelUidSeed}`;
};

const normalizeForEditing = (value) => ({
  enabled: value?.enabled !== false,
  models: Array.isArray(value?.models)
    ? value.models.map((model) => ({
      uid: nextModelUid(),
      id: String(model?.id || ''),
      display_name: String(model?.display_name || ''),
      enabled: model?.enabled !== false
    }))
    : []
});

const serializeCatalog = (value) => JSON.stringify({
  enabled: value.enabled,
  models: value.models.map((model) => ({
    id: model.id,
    display_name: model.display_name,
    enabled: model.enabled
  }))
});

const normalizeForSave = (value) => {
  const models = value.models.map((model) => {
    const id = model.id.trim();
    return {
      id,
      display_name: model.display_name.trim() || id,
      enabled: Boolean(model.enabled)
    };
  });
  models.sort((left, right) => {
    if (left.enabled !== right.enabled) return left.enabled ? -1 : 1;
    return left.id.localeCompare(right.id);
  });
  return { enabled: Boolean(value.enabled), models };
};

const validateCatalog = (catalog) => {
  const rows = catalog.models.map(() => ({ id: false, name: false }));
  let message = catalog.models.length > MAX_MODELS ? `最多允许 ${MAX_MODELS} 个模型条目` : '';
  const fail = (index, field, text) => {
    rows[index][field] = true;
    if (!message && text) message = text;
  };
  const seen = new Map();
  catalog.models.forEach((model, index) => {
    const id = model.id.trim();
    const displayName = model.display_name.trim();
    if (!id) {
      // 新增的空行不标红，只在聚合提示里说明，避免刚添加就满屏红框。
      if (!message) message = `第 ${index + 1} 个模型缺少 ID`;
      return;
    }
    if ([...id].length > MAX_TEXT_LENGTH) fail(index, 'id', `模型 ${index + 1} 的 ID 超过 ${MAX_TEXT_LENGTH} 个字符`);
    if (/\s/u.test(id) || /[\p{Cc}\p{Cf}]/u.test(id)) fail(index, 'id', `模型 ID “${id}”不能包含空白或控制字符`);
    if (seen.has(id)) {
      fail(seen.get(id), 'id', '');
      fail(index, 'id', `模型 ID “${id}”重复`);
    } else {
      seen.set(id, index);
    }
    if ([...displayName].length > MAX_TEXT_LENGTH) fail(index, 'name', `模型 “${id}”的显示名称超过 ${MAX_TEXT_LENGTH} 个字符`);
    if (/[\p{Cc}\p{Cf}]/u.test(displayName)) fail(index, 'name', `模型 “${id}”的显示名称不能包含控制字符`);
  });
  return { message, rows };
};

const inputClass = (invalid, enabled) => [
  'w-full rounded-lg border px-3 py-2 text-sm outline-none transition focus:ring-2',
  invalid
    ? 'border-rose-300 focus:border-rose-400 focus:ring-rose-100'
    : 'border-slate-200 focus:border-indigo-400 focus:ring-indigo-100',
  enabled ? 'text-slate-800' : 'bg-slate-50 text-slate-400'
].join(' ');

const ROW_GRID = 'grid grid-cols-[2.75rem_1fr_1fr_2.25rem] items-center gap-3';

const Toggle = ({ checked, onChange, label }) => (
  <button
    type="button"
    role="switch"
    aria-checked={checked}
    aria-label={label}
    onClick={() => onChange(!checked)}
    className={`relative h-6 w-11 shrink-0 rounded-full transition-colors ${checked ? 'bg-indigo-600' : 'bg-slate-300'}`}
  >
    <span className={`absolute left-0.5 top-0.5 h-5 w-5 rounded-full bg-white shadow-sm transition-transform ${checked ? 'translate-x-5' : 'translate-x-0'}`} />
  </button>
);

const ClaudeModelsPanel = ({ open = false, onClose }) => {
  const closeButtonRef = useRef(null);
  const pendingFocusUidRef = useRef(null);
  const [catalog, setCatalog] = useState(emptyCatalog);
  const [baseline, setBaseline] = useState(() => serializeCatalog(emptyCatalog()));
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);

  const dirty = serializeCatalog(catalog) !== baseline;
  const validation = useMemo(() => validateCatalog(catalog), [catalog]);
  const validationError = validation.message;
  const enabledCount = catalog.models.filter((model) => model.enabled).length;

  const requestClose = () => {
    if (saving) return;
    if (dirty) {
      setConfirmDiscard(true);
      return;
    }
    onClose?.();
  };

  useModalLifecycle({ open, onClose: requestClose, initialFocusRef: closeButtonRef });

  const loadCatalog = async () => {
    setLoading(true);
    setError('');
    setSaved(false);
    setConfirmDiscard(false);
    try {
      if (!isWailsEnvironment()) throw new Error('当前页面未连接 AI Switchboard 桌面运行时');
      const settings = await getSettingsByCategory(CATEGORY);
      const record = settings.find((setting) => setting.key === CATALOG_KEY);
      const parsed = record?.value ? JSON.parse(record.value) : emptyCatalog();
      const next = normalizeForEditing(parsed);
      setCatalog(next);
      setBaseline(serializeCatalog(next));
    } catch (loadError) {
      setError(loadError?.message || '加载 Claude 模型目录失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (open) loadCatalog();
    // 面板只在打开边沿加载；编辑期间不因函数引用变化重置草稿。
  }, [open]);

  if (!open) return null;

  const updateModel = (index, patch) => {
    setSaved(false);
    setCatalog((current) => ({
      ...current,
      models: current.models.map((model, modelIndex) => (modelIndex === index ? { ...model, ...patch } : model))
    }));
  };

  const addModel = () => {
    setSaved(false);
    const uid = nextModelUid();
    pendingFocusUidRef.current = uid;
    setCatalog((current) => ({
      ...current,
      models: [...current.models, { uid, id: '', display_name: '', enabled: true }]
    }));
  };

  const removeModel = (index) => {
    setSaved(false);
    setCatalog((current) => ({
      ...current,
      models: current.models.filter((_, modelIndex) => modelIndex !== index)
    }));
  };

  const focusNewRowInput = (node, uid) => {
    if (node && pendingFocusUidRef.current === uid) {
      pendingFocusUidRef.current = null;
      node.focus();
      node.scrollIntoView({ block: 'nearest' });
    }
  };

  const saveCatalog = async () => {
    if (validationError) {
      setError(validationError);
      return;
    }
    setSaving(true);
    setError('');
    setSaved(false);
    try {
      const normalized = normalizeForSave(catalog);
      await updateSetting(CATEGORY, CATALOG_KEY, JSON.stringify(normalized));
      setCatalog(normalizeForEditing(normalized));
      setBaseline(serializeCatalog(normalized));
      setSaved(true);
      setConfirmDiscard(false);
    } catch (saveError) {
      setError(saveError?.message || '保存 Claude 模型目录失败');
    } finally {
      setSaving(false);
    }
  };

  const nav = document.querySelector('nav.sticky');
  const topOffset = nav ? nav.getBoundingClientRect().bottom : 0;

  return createPortal(
    <div className="fixed inset-0 z-[46] flex justify-end" style={{ top: topOffset }}>
      <button type="button" aria-label="关闭 Claude 模型目录" className="absolute inset-0 bg-slate-950/30 backdrop-blur-[2px]" onClick={requestClose} />
      <aside role="dialog" aria-modal="true" aria-label="Claude Gateway 模型目录" className="relative z-10 flex h-full w-full max-w-[760px] flex-col border-l border-slate-200 bg-white shadow-2xl shadow-slate-950/20 animate-in slide-in-from-right duration-300">
        <div className="border-b border-slate-100 bg-gradient-to-r from-indigo-50/80 via-white to-white px-6 py-5">
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-100 text-indigo-600 shadow-sm"><Boxes size={20} /></div>
              <div>
                <h2 className="text-lg font-semibold text-slate-900">Claude Gateway 模型目录</h2>
                <p className="mt-0.5 text-sm text-slate-500">维护 Claude Code `/model` 中的展示项，模型 ID 不限于 Anthropic 官方模型。</p>
              </div>
            </div>
            <button type="button" ref={closeButtonRef} aria-label="关闭抽屉" onClick={requestClose} className="rounded-lg border border-slate-200 bg-white p-2 text-slate-400 transition-colors hover:border-slate-300 hover:text-slate-700"><X size={16} /></button>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto px-6 py-5">
          <div className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-xs leading-5 text-amber-800">
            <div>这是独立的展示目录，不检测端点是否支持模型，也不改变模型改写、端点绑定或调度。</div>
            <details className="mt-1">
              <summary className="cursor-pointer select-none font-medium text-amber-700 transition-colors hover:text-amber-900">Gateway Discovery 补丁说明</summary>
              <p className="mt-1">客户端需应用完整的 Gateway Discovery 补丁；补丁会在配置 Base URL 与认证后自动发现模型，并仅让 <code className="rounded bg-amber-100 px-1">/v1/models</code> 绕过非必要流量限制，无需再设置 discovery 环境变量。</p>
            </details>
          </div>

          <div className="mt-4 flex items-center justify-between rounded-xl border border-slate-200 bg-slate-50/70 px-4 py-3">
            <div>
              <div className="text-sm font-medium text-slate-800">启用本地模型目录</div>
              <div className="mt-0.5 text-xs text-slate-500">关闭时 Claude `/v1/models` 返回空列表。</div>
            </div>
            <Toggle checked={catalog.enabled} label="启用本地 Claude 模型目录" onChange={(enabled) => { setSaved(false); setCatalog((current) => ({ ...current, enabled })); }} />
          </div>

          <div className="mt-5 flex items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold text-slate-800">展示模型</h3>
              <p className="mt-0.5 text-xs text-slate-400">共 {catalog.models.length} 项，启用 {enabledCount} 项；保存后按启用状态和 ID 排序。</p>
            </div>
            <Button variant="secondary" size="sm" icon={Plus} onClick={addModel} disabled={loading || saving || catalog.models.length >= MAX_MODELS}>添加模型</Button>
          </div>

          {loading ? (
            <div className="mt-4 flex items-center justify-center gap-2 rounded-xl border border-dashed border-slate-200 py-12 text-sm text-slate-400"><RefreshCw size={16} className="animate-spin" />加载目录...</div>
          ) : catalog.models.length === 0 ? (
            <button type="button" onClick={addModel} className="mt-4 w-full rounded-xl border border-dashed border-slate-200 bg-slate-50/50 px-4 py-10 text-center transition-colors hover:border-indigo-300 hover:bg-indigo-50/40">
              <Boxes size={28} className="mx-auto text-slate-300" />
              <div className="mt-2 text-sm font-medium text-slate-600">目录还是空的</div>
              <div className="mt-1 text-xs text-slate-400">添加 Gateway 返回给 Claude Code 的模型 ID。</div>
            </button>
          ) : (
            <div className="mt-4 overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <div className={`${ROW_GRID} border-b border-slate-100 bg-slate-50/70 px-4 py-2.5 text-xs font-medium text-slate-500`}>
                <span>启用</span>
                <span>模型 ID</span>
                <span>显示名称</span>
                <span aria-hidden="true" />
              </div>
              <div className="divide-y divide-slate-100">
                {catalog.models.map((model, index) => (
                  <div key={model.uid} className={`${ROW_GRID} px-4 py-2.5 transition-colors ${model.enabled ? '' : 'bg-slate-50/40'}`}>
                    <Toggle checked={model.enabled} label={`启用模型 ${model.id || index + 1}`} onChange={(enabled) => updateModel(index, { enabled })} />
                    <input
                      ref={(node) => focusNewRowInput(node, model.uid)}
                      value={model.id}
                      maxLength={MAX_TEXT_LENGTH}
                      spellCheck={false}
                      placeholder="例如 deepseek-v4-flash[1m]"
                      aria-label={`模型 ${index + 1} 的 ID`}
                      onChange={(event) => updateModel(index, { id: event.target.value })}
                      className={`${inputClass(validation.rows[index]?.id, model.enabled)} font-mono`}
                    />
                    <input
                      value={model.display_name}
                      maxLength={MAX_TEXT_LENGTH}
                      placeholder="留空时使用模型 ID"
                      aria-label={`模型 ${index + 1} 的显示名称`}
                      onChange={(event) => updateModel(index, { display_name: event.target.value })}
                      className={inputClass(validation.rows[index]?.name, model.enabled)}
                    />
                    <button type="button" aria-label={`删除模型 ${model.id || index + 1}`} onClick={() => removeModel(index)} className="rounded-lg p-2 text-slate-400 transition-colors hover:bg-rose-50 hover:text-rose-600"><Trash2 size={16} /></button>
                  </div>
                ))}
              </div>
            </div>
          )}

          {(error || validationError) && <div className="mt-4 flex items-start gap-2 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2.5 text-sm text-rose-700"><AlertTriangle size={16} className="mt-0.5 shrink-0" /><span>{error || validationError}</span></div>}
          {saved && !dirty && <div className="mt-4 rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2.5 text-sm text-emerald-700">模型目录已保存，新请求会立即读取最新目录。</div>}
        </div>

        <div className="border-t border-slate-100 bg-white px-6 py-4">
          {confirmDiscard ? (
            <div className="flex flex-col gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-start gap-2 text-sm text-amber-800"><AlertTriangle size={16} className="mt-0.5 shrink-0" /><span>有尚未保存的目录修改，确定放弃吗？</span></div>
              <div className="flex gap-2"><Button variant="ghost" size="sm" onClick={() => setConfirmDiscard(false)}>继续编辑</Button><Button variant="danger" size="sm" onClick={() => onClose?.()}>放弃修改</Button></div>
            </div>
          ) : (
            <div className="flex items-center justify-between gap-3">
              <div className="text-xs text-slate-400">{dirty ? '有未保存修改' : '目录已与本地设置同步'}</div>
              <div className="flex gap-2"><Button variant="ghost" onClick={requestClose}>关闭</Button><Button icon={Save} onClick={saveCatalog} loading={saving} disabled={loading || !dirty || Boolean(validationError)}>保存目录</Button></div>
            </div>
          )}
        </div>
      </aside>
    </div>,
    document.body
  );
};

export default ClaudeModelsPanel;
