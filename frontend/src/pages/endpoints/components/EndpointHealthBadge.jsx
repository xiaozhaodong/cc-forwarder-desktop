import { CheckCircle2, Clock3, XCircle } from 'lucide-react';

// 端点连通性状态徽章:表格行与网格卡片共用
const EndpointHealthBadge = ({ endpoint }) => {
  const neverChecked = endpoint.neverChecked || !endpoint.lastCheck;
  if (neverChecked) return <span className="inline-flex items-center gap-1 text-xs text-fg-subtle"><Clock3 size={13} />未检测</span>;
  if (endpoint.healthy) return <span className="inline-flex items-center gap-1 text-xs font-medium text-success"><CheckCircle2 size={13} />可达</span>;
  return <span className="inline-flex items-center gap-1 text-xs font-medium text-danger"><XCircle size={13} />不可达</span>;
};

export default EndpointHealthBadge;
