import { useState } from 'react';
import { AlertTriangle, Archive, Database, FileCog, RefreshCw, ShieldCheck } from 'lucide-react';
import { retryStartupMigration } from '@utils/wailsApi.js';

const Detail = ({ icon: Icon, label, value }) => (
  <div className="rounded-xl border border-slate-200 bg-white/80 p-4">
    <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.12em] text-slate-400"><Icon size={14} />{label}</div>
    <div className="mt-2 break-all font-mono text-xs leading-5 text-slate-700">{value || '未生成'}</div>
  </div>
);

const MigrationRecoveryPage = ({ status = {}, onStatusChange }) => {
  const [retrying, setRetrying] = useState(false);
  const [retryError, setRetryError] = useState('');

  const retry = async () => {
    setRetrying(true);
    setRetryError('');
    try {
      const next = await retryStartupMigration();
      onStatusChange?.(next);
    } catch (error) {
      setRetryError(error?.message || '重试失败');
    } finally {
      setRetrying(false);
    }
  };

  return (
    <div className="min-h-screen bg-[#f7f8fa] px-6 py-12 text-slate-900">
      <div className="pointer-events-none fixed inset-0 opacity-40" style={{ backgroundImage: 'radial-gradient(#cbd5e1 1px, transparent 1px)', backgroundSize: '24px 24px' }} />
      <main className="relative mx-auto max-w-4xl">
        <div className="overflow-hidden rounded-3xl border border-amber-200/80 bg-white shadow-[0_30px_90px_rgba(15,23,42,0.12)]">
          <div className="border-b border-amber-100 bg-gradient-to-r from-amber-50 via-white to-white px-8 py-7">
            <div className="flex items-start gap-4">
              <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-amber-100 text-amber-700"><AlertTriangle size={24} /></div>
              <div>
                <div className="text-xs font-semibold uppercase tracking-[0.16em] text-amber-600">安全恢复模式</div>
                <h1 className="mt-1 text-2xl font-bold">Claude 端点迁移未完成</h1>
                <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600">代理、后台检测和历史写入尚未启动，避免在数据库结构不一致时继续处理请求。请先查看备份与失败阶段，再执行幂等重试。</p>
              </div>
            </div>
          </div>

          <div className="space-y-6 px-8 py-7">
            <div className="rounded-2xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm leading-6 text-rose-800">
              <div className="font-semibold">失败阶段：{status.phase || '准备阶段'}</div>
              <div className="mt-1 break-words">{retryError || status.error || '未提供错误信息'}</div>
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <Detail icon={Database} label="运行数据库" value={status.databasePath} />
              <Detail icon={FileCog} label="配置文件" value={status.configPath} />
              <Detail icon={Archive} label="迁移备份" value={status.backupDir} />
              <Detail icon={ShieldCheck} label="完整性" value={`数据库 ${status.databaseIntegrity || '待验证'} · 备份 ${status.backupIntegrity || '待验证'}`} />
            </div>

            <div className="flex flex-col gap-3 rounded-2xl border border-slate-200 bg-slate-50/70 px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <div className="text-sm font-semibold text-slate-800">重试会从已提交阶段继续</div>
                <div className="mt-1 text-xs leading-5 text-slate-500">不会删除现有备份，也不会启动代理，直到 schema、配置与完整性验证全部完成。</div>
              </div>
              <button type="button" onClick={retry} disabled={retrying || status.retryAllowed === false} className="inline-flex shrink-0 items-center justify-center gap-2 rounded-xl bg-slate-900 px-4 py-2.5 text-sm font-semibold text-white shadow-lg shadow-slate-300 transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50">
                <RefreshCw size={16} className={retrying ? 'animate-spin' : ''} />{retrying ? '正在重试' : '重试迁移'}
              </button>
            </div>
          </div>
        </div>
      </main>
    </div>
  );
};

export default MigrationRecoveryPage;
