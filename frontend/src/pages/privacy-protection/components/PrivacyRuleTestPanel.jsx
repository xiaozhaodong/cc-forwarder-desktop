// ============================================
// 隐私规则测试面板
// 2026-06-11 (v6.1 新增)
// 测试文本只保存在 React state，不写 localStorage、不落请求日志
// ============================================

import { useState } from 'react';
import { FlaskConical } from 'lucide-react';
import { Button, CustomSelect } from '@components/ui';
import PrivacyHitList from './PrivacyHitList.jsx';
import {
  PRIVACY_PATH_OPTIONS,
  PRIVACY_UPSTREAM_TYPE_OPTIONS
} from '../utils/privacyRules.js';

const PrivacyRuleTestPanel = ({ onTest, variant = 'card', showHeader = true }) => {
  const [text, setText] = useState('');
  const [path, setPath] = useState('/v1/messages');
  const [upstreamType, setUpstreamType] = useState('endpoint');
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState('');
  const isDrawer = variant === 'drawer';

  const handleTest = async () => {
    setTesting(true);
    setError('');
    try {
      setResult(await onTest({ text, path, upstream_type: upstreamType }));
    } catch (err) {
      setResult(null);
      setError(err.message || String(err));
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className={isDrawer ? 'space-y-4' : 'border border-line rounded-xl bg-surface p-4 space-y-3'}>
      {showHeader && (
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold text-fg-body flex items-center gap-1.5">
            <FlaskConical size={15} className="text-accent" />
            规则测试
          </h3>
          <span className="text-xs text-fg-subtle">测试文本不会被记录</span>
        </div>
      )}

      <div className={isDrawer ? 'grid grid-cols-1 gap-3 sm:grid-cols-[1fr_160px]' : 'flex gap-2'}>
        <CustomSelect
          options={PRIVACY_PATH_OPTIONS.map((p) => ({ value: p, label: p }))}
          value={path}
          onChange={setPath}
          className={isDrawer ? 'w-full [&>button]:h-11' : 'flex-1'}
        />
        <CustomSelect
          options={PRIVACY_UPSTREAM_TYPE_OPTIONS}
          value={upstreamType}
          onChange={setUpstreamType}
          className={isDrawer ? 'w-full [&>button]:h-11' : 'w-32'}
        />
      </div>

      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={isDrawer ? 9 : 6}
        spellCheck={false}
        placeholder="粘贴一段测试文本，例如包含 API Key 或手机号的内容"
        className="w-full px-3 py-2 border border-line rounded-lg text-sm font-mono break-all focus:outline-none focus:ring-2 focus:ring-accent-ring"
      />

      <Button
        onClick={handleTest}
        loading={testing}
        disabled={!text}
        className={isDrawer ? 'h-10 w-full' : 'w-full'}
        size={isDrawer ? 'md' : 'sm'}
      >
        测试
      </Button>

      {error && <p className="text-xs text-danger break-all">{error}</p>}

      {result && (
        <div className="space-y-3">
          <div className="flex items-center gap-3 text-xs text-fg-muted">
            <span>原文 {result.original_length} 字符</span>
            <span>命中 {result.hit_count} 处</span>
            <span>耗时 {result.scan_duration_ms.toFixed(2)} ms</span>
          </div>

          <div>
            <div className="text-xs font-medium text-fg-muted mb-1">替换后文本</div>
            <pre className={`text-xs bg-surface-sub border border-line-soft rounded-lg p-3 whitespace-pre-wrap break-all overflow-y-auto ${isDrawer ? 'max-h-72' : 'max-h-48'}`}>
              {result.replaced_text || '（空）'}
            </pre>
          </div>

          <div>
            <div className="text-xs font-medium text-fg-muted mb-1">命中规则</div>
            <PrivacyHitList hits={result.rule_hits} />
          </div>
        </div>
      )}
    </div>
  );
};

export default PrivacyRuleTestPanel;
