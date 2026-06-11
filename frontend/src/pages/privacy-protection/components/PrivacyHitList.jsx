// ============================================
// 隐私规则测试命中列表
// 2026-06-11 (v6.1 新增)
// ============================================

const PrivacyHitList = ({ hits = [] }) => {
  if (!hits.length) {
    return <p className="text-xs text-slate-400">无命中规则</p>;
  }

  return (
    <div className="border border-slate-200 rounded-lg overflow-hidden">
      <table className="w-full text-xs">
        <thead>
          <tr className="bg-slate-50 text-left text-slate-500">
            <th className="px-3 py-1.5 font-medium">规则</th>
            <th className="px-3 py-1.5 font-medium w-16">动作</th>
            <th className="px-3 py-1.5 font-medium w-16 text-right">命中数</th>
          </tr>
        </thead>
        <tbody>
          {hits.map((hit) => (
            <tr key={hit.rule_id} className="border-t border-slate-100">
              <td className="px-3 py-1.5 text-slate-700 break-all">{hit.rule_name}</td>
              <td className="px-3 py-1.5 text-slate-500">{hit.action === 'detect' ? '仅检测' : '脱敏'}</td>
              <td className="px-3 py-1.5 text-right tabular-nums text-slate-700">{hit.count}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default PrivacyHitList;
