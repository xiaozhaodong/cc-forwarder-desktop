// ============================================
// 隐私预设导入弹窗
// 2026-06-11 (v6.1 新增)
// ============================================

import { useState } from 'react';
import { PackagePlus, X } from 'lucide-react';
import { Button } from '@components/ui';

const PrivacyPresetDialog = ({ open, presets = [], onImport, onClose }) => {
  const [selectedId, setSelectedId] = useState('');
  const [importingId, setImportingId] = useState('');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  if (!open) return null;

  const selectedPreset = presets.find((preset) => preset.id === selectedId) || presets[0];

  const handleImport = async (preset) => {
    if (!preset) return;
    setImportingId(preset.id);
    setMessage('');
    setError('');
    try {
      const created = await onImport(preset.id);
      if (!created || created.length === 0) {
        setMessage(`「${preset.name}」中的预设规则已是最新`);
      } else {
        setMessage(`已导入或同步 ${created.length} 条规则并立即编译生效`);
      }
    } catch (err) {
      setError(err.message || String(err));
    } finally {
      setImportingId('');
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[14vh] bg-black/40 animate-fade-in">
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-lg mx-4 overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-slate-100">
          <h3 className="text-base font-semibold text-slate-800 flex items-center gap-2">
            <PackagePlus size={17} className="text-indigo-500" />
            导入预设规则集
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-slate-700 hover:bg-slate-100 rounded-lg"
          >
            <X size={18} />
          </button>
        </div>

        <div className="px-5 py-4 space-y-3">
          {presets.map((preset) => (
            <button
              key={preset.id}
              type="button"
              disabled={Boolean(importingId)}
              onClick={() => {
                setSelectedId(preset.id);
                setMessage('');
                setError('');
              }}
              className={`w-full text-left border rounded-xl px-4 py-3 transition-all ${
                selectedPreset?.id === preset.id
                  ? 'border-indigo-300 bg-indigo-50/60 shadow-sm'
                  : 'border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-50'
              } ${importingId ? 'cursor-not-allowed opacity-70' : 'cursor-pointer'}`}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-sm font-semibold text-slate-800">{preset.name}</div>
                  <div className="mt-1 text-xs leading-5 text-slate-400">
                    {preset.description} · 共 {preset.rule_count} 条
                  </div>
                </div>
                <div className="text-xs text-slate-400 break-all">
                  <span
                    className={`mt-1 block h-3 w-3 rounded-full border ${
                      selectedPreset?.id === preset.id
                        ? 'border-indigo-500 bg-indigo-500 shadow-[0_0_0_3px_rgba(99,102,241,0.12)]'
                        : 'border-slate-300 bg-white'
                    }`}
                  />
                </div>
              </div>
            </button>
          ))}
          {presets.length === 0 && (
            <p className="text-sm text-slate-400 text-center py-4">暂无可用预设</p>
          )}

          <p className="text-xs text-slate-400">
            同名自定义规则不会覆盖；同名预设规则会同步到最新版。导入后立即编译热生效。
          </p>
          {message && <p className="text-xs text-emerald-600">{message}</p>}
          {error && <p className="text-xs text-rose-500 break-all">{error}</p>}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-slate-100 bg-slate-50/70 px-5 py-4">
          <Button type="button" variant="ghost" onClick={onClose} disabled={Boolean(importingId)}>
            取消
          </Button>
          <Button
            type="button"
            icon={PackagePlus}
            loading={Boolean(importingId)}
            disabled={!selectedPreset || Boolean(importingId)}
            onClick={() => handleImport(selectedPreset)}
          >
            导入选中预设
          </Button>
        </div>
      </div>
    </div>
  );
};

export default PrivacyPresetDialog;
