// ============================================
// Account Pool 页面头部
// 2026-03-07
// ============================================

import { RefreshCw, ShieldCheck } from 'lucide-react';
import { Button } from '@components/ui';

const PageHeader = ({ loading = false, onRefresh }) => (
  <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
    <div className="flex items-center gap-3">
      <div className="p-2 bg-slate-900 rounded-lg text-white shadow-lg">
        <ShieldCheck className="w-5 h-5" />
      </div>
      <div>
        <h1 className="text-2xl font-bold text-slate-900">账号管理</h1>
        <p className="text-sm text-slate-500">管理上游账号池，支持 API Key 与 ChatGPT OAuth 两种授权方式</p>
      </div>
    </div>
    <Button icon={RefreshCw} variant="secondary" onClick={onRefresh} loading={loading}>
      刷新数据
    </Button>
  </div>
);

export default PageHeader;
