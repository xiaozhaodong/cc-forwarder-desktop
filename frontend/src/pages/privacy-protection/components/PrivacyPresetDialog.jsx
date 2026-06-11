// ============================================
// 隐私预设导入弹窗
// 2026-06-11 (v6.1 新增)
// ============================================

import { useState } from 'react';
import { PackagePlus, X } from 'lucide-react';
import { Button } from '@components/ui';

const PrivacyPresetDialog = ({ open, presets = [], onImport, onClose }) => {
  const [importingId, setImportingId] = useState('');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  if (!open) return null;

  const handleImport = async (preset) => {
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
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-md mx-4">
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
            <div
              key={preset.id}
              className="flex items-center justify-between gap-3 border border-slate-200 rounded-xl px-4 py-3"
            >
              <div className="min-w-0">
                <div className="text-sm font-medium text-slate-800">{preset.name}</div>
                <div className="text-xs text-slate-400 break-all">
                  {preset.description} · 共 {preset.rule_count} 条
                </div>
              </div>
              <Button
                size="sm"
                variant="secondary"
                loading={importingId === preset.id}
                disabled={Boolean(importingId)}
                onClick={() => handleImport(preset)}
              >
                导入
              </Button>
            </div>
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
      </div>
    </div>
  );
};

export default PrivacyPresetDialog;
