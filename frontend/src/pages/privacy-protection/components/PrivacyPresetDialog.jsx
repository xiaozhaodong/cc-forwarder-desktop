// ============================================
// 隐私预设导入弹窗
// 2026-06-11 (v6.1 新增)
// ============================================

import { useRef, useState } from 'react';
import { PackagePlus, X } from 'lucide-react';
import { Button } from '@components/ui';
import useModalLifecycle from '@hooks/useModalLifecycle.js';

const PrivacyPresetDialog = ({ open, presets = [], onImport, onClose }) => {
  const [selectedId, setSelectedId] = useState('');
  const [importingId, setImportingId] = useState('');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const closeButtonRef = useRef(null);
  const handleRequestClose = () => {
    if (!importingId) onClose();
  };

  useModalLifecycle({
    open,
    onClose: handleRequestClose,
    initialFocusRef: closeButtonRef
  });

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
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[14vh] bg-overlay animate-fade-in">
      <div
        role="dialog"
        aria-modal="true"
        aria-label="导入预设规则集"
        className="bg-surface rounded-2xl shadow-xl w-full max-w-lg mx-4 overflow-hidden"
      >
        <div className="flex items-center justify-between px-5 py-4 border-b border-line-soft">
          <h3 className="text-base font-semibold text-fg flex items-center gap-2">
            <PackagePlus size={17} className="text-accent" />
            导入预设规则集
          </h3>
          <button
            type="button"
            ref={closeButtonRef}
            aria-label="关闭弹窗"
            onClick={handleRequestClose}
            className="p-1.5 text-fg-subtle hover:text-fg-body hover:bg-surface-mut rounded-lg"
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
                  ? 'border-accent-line bg-accent-soft shadow-sm'
                  : 'border-line bg-surface hover:border-line-strong hover:bg-surface-sub'
              } ${importingId ? 'cursor-not-allowed opacity-70' : 'cursor-pointer'}`}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-sm font-semibold text-fg">{preset.name}</div>
                  <div className="mt-1 text-xs leading-5 text-fg-subtle">
                    {preset.description} · 共 {preset.rule_count} 条
                  </div>
                </div>
                <div className="text-xs text-fg-subtle break-all">
                  <span
                    className={`mt-1 block h-3 w-3 rounded-full border ${
                      selectedPreset?.id === preset.id
                        ? 'border-accent-line bg-indigo-500 shadow-[0_0_0_3px_rgba(99,102,241,0.12)]'
                        : 'border-line-strong bg-surface'
                    }`}
                  />
                </div>
              </div>
            </button>
          ))}
          {presets.length === 0 && (
            <p className="text-sm text-fg-subtle text-center py-4">暂无可用预设</p>
          )}

          <p className="text-xs text-fg-subtle">
            同名自定义规则不会覆盖；同名预设规则会同步到最新版。导入后立即编译热生效。
          </p>
          {message && <p className="text-xs text-success">{message}</p>}
          {error && <p className="text-xs text-danger break-all">{error}</p>}
        </div>
        <div className="flex items-center justify-end gap-2 border-t border-line-soft bg-surface-sub px-5 py-4">
          <Button type="button" variant="ghost" onClick={handleRequestClose} disabled={Boolean(importingId)}>
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
