// ============================================
// 隐私规则测试命中列表
// 2026-06-11 (v6.1 新增)
// ============================================

import { sourceLabel } from '../utils/privacyRules.js';

const PrivacyHitList = ({ hits = [] }) => {
  if (!hits.length) {
    return <p className="text-xs text-fg-subtle">无命中规则</p>;
  }

  return (
    <div className="border border-line rounded-lg overflow-hidden">
      <table className="w-full text-xs">
        <thead>
          <tr className="bg-surface-sub text-left text-fg-muted">
            <th className="px-3 py-1.5 font-medium">规则</th>
            <th className="px-3 py-1.5 font-medium w-20">来源</th>
            <th className="px-3 py-1.5 font-medium w-16">动作</th>
            <th className="px-3 py-1.5 font-medium w-16 text-right">命中数</th>
          </tr>
        </thead>
        <tbody>
          {hits.map((hit) => (
            <tr key={hit.rule_id} className="border-t border-line-soft">
              <td className="px-3 py-1.5 text-fg-body break-all">{hit.rule_name}</td>
              <td className="px-3 py-1.5 text-fg-muted">{sourceLabel(hit.source)}</td>
              <td className="px-3 py-1.5 text-fg-muted">{hit.action === 'detect' ? '仅检测' : '脱敏'}</td>
              <td className="px-3 py-1.5 text-right tabular-nums text-fg-body">{hit.count}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default PrivacyHitList;
