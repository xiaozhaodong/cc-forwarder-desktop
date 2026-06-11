// ============================================
// 隐私规则 Pattern 编辑器
// 2026-06-11 (v6.1 新增)
// ============================================

import { PRIVACY_MATCH_TYPE_OPTIONS } from '../utils/privacyRules.js';

const PrivacyPatternEditor = ({ matchType, pattern, error, onMatchTypeChange, onPatternChange }) => (
  <div className="space-y-2">
    <div className="flex items-center justify-between">
      <span className="text-sm font-medium text-slate-700">匹配方式与 Pattern</span>
      <div className="inline-flex rounded-lg border border-slate-200 p-0.5 bg-slate-50">
        {PRIVACY_MATCH_TYPE_OPTIONS.map((opt) => (
          <button
            key={opt.value}
            type="button"
            onClick={() => onMatchTypeChange(opt.value)}
            className={`px-3 py-1 rounded-md text-xs font-medium transition-colors ${
              matchType === opt.value
                ? 'bg-white text-indigo-600 shadow-sm'
                : 'text-slate-500 hover:text-slate-700'
            }`}
          >
            {opt.label}
          </button>
        ))}
      </div>
    </div>
    <textarea
      value={pattern}
      onChange={(e) => onPatternChange(e.target.value)}
      rows={3}
      spellCheck={false}
      placeholder={matchType === 'regex'
        ? String.raw`Go RE2 正则，例如 sk-(?:proj-)?[A-Za-z0-9_-]{20,}`
        : '普通包含匹配，例如内部项目代号'}
      className={`w-full px-3 py-2 border rounded-lg text-sm font-mono break-all focus:outline-none focus:ring-2 focus:ring-indigo-500 ${
        error ? 'border-rose-300' : 'border-slate-200'
      }`}
    />
    {error && <span className="text-xs text-rose-500">{error}</span>}
    {matchType === 'regex' && (
      <p className="text-xs text-slate-400">
        使用 Go RE2 语法（不支持回溯/前瞻）。保存时由后端编译校验，失败不会落库。
      </p>
    )}
  </div>
);

export default PrivacyPatternEditor;
