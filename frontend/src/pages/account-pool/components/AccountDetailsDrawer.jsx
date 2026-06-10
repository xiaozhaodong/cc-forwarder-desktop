// ============================================
// 账号详情抽屉
// 2026-03-21
// ============================================

import { createPortal } from 'react-dom';
import { Activity, Gauge, Settings, Shield, X } from 'lucide-react';
import { Button } from '@components/ui';
import Badge from './Badge.jsx';
import { parseCodexModelRewriteSettings } from '../utils.js';

const TONE_CLASS = {
  rose: 'bg-rose-50 text-rose-700 border-rose-200',
  red: 'bg-rose-50 text-rose-700 border-rose-200',
  amber: 'bg-amber-50 text-amber-700 border-amber-200',
  yellow: 'bg-amber-50 text-amber-700 border-amber-200',
  emerald: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  green: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  sky: 'bg-sky-50 text-sky-700 border-sky-200',
  blue: 'bg-sky-50 text-sky-700 border-sky-200',
  indigo: 'bg-indigo-50 text-indigo-700 border-indigo-200',
  slate: 'bg-slate-50 text-slate-600 border-slate-200'
};

const SECTION_THEME = {
  '额度': { iconBg: 'bg-amber-50', iconColor: 'text-amber-600', border: 'border-amber-100' },
  '健康': { iconBg: 'bg-emerald-50', iconColor: 'text-emerald-600', border: 'border-emerald-100' },
  '路由配置': { iconBg: 'bg-sky-50', iconColor: 'text-sky-600', border: 'border-sky-100' },
  '动作': { iconBg: 'bg-indigo-50', iconColor: 'text-indigo-600', border: 'border-indigo-100' }
};

const quotaBarColor = (percent) => {
  if (!Number.isFinite(percent)) return 'bg-slate-200';
  if (percent > 50) return 'bg-emerald-400';
  if (percent > 20) return 'bg-amber-400';
  return 'bg-rose-400';
};

const quotaTextColor = (percent) => {
  if (!Number.isFinite(percent)) return 'text-slate-500';
  if (percent > 50) return 'text-emerald-600';
  if (percent > 20) return 'text-amber-600';
  return 'text-rose-600';
};

const QuotaBar = ({ label, text, percent }) => {
  const hasBar = Number.isFinite(percent);

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-sm">
        <span className="text-slate-400">{label}</span>
        <span className={`font-semibold ${quotaTextColor(percent)}`}>{text || '-'}</span>
      </div>
      {hasBar && (
        <div className="h-2 w-full overflow-hidden rounded-full bg-slate-100">
          <div
            className={`h-full rounded-full transition-all ${quotaBarColor(percent)}`}
            style={{ width: `${Math.max(Math.min(percent, 100), 0)}%` }}
          />
        </div>
      )}
    </div>
  );
};

const SectionCard = ({ icon: Icon, title, children }) => {
  const theme = SECTION_THEME[title] || { iconBg: 'bg-slate-100', iconColor: 'text-slate-500', border: 'border-slate-200' };

  return (
    <section className={`rounded-xl border ${theme.border} bg-white p-4 shadow-sm shadow-slate-950/5`}>
      <div className="mb-3 flex items-center gap-2">
        <div className={`flex h-8 w-8 items-center justify-center rounded-lg ${theme.iconBg} ${theme.iconColor}`}>
          <Icon size={16} />
        </div>
        <h3 className="text-sm font-semibold text-slate-900">{title}</h3>
      </div>
      <div className="space-y-3">{children}</div>
    </section>
  );
};

const DetailRow = ({ label, value }) => (
  <div className="flex items-start justify-between gap-4 text-sm">
    <span className="text-slate-400">{label}</span>
    <span className="text-right font-medium text-slate-700">{value || '-'}</span>
  </div>
);

const normalizeGroupKey = (value = '') => {
  const normalized = String(value || '').trim().toLowerCase();
  if (normalized === 'primary' || normalized === '主组') return 'primary';
  if (normalized === 'backup' || normalized === '备组') return 'backup';
  if (normalized === 'cold' || normalized === '冷备') return 'cold';
  return '';
};

const AccountDetailsDrawer = ({
  open = false,
  row = null,
  busyKey = '',
  onClose,
  onToggleAccount,
  onTestAccount,
  onRefreshAccountProfile,
  onMoveAccountToTier,
  onDeleteAccount,
  onEditAccount
}) => {
  if (!open || !row) {
    return null;
  }

  const detail = row.detail || {};
  const account = detail.rawAccount || detail.account || row;
  const currentGroupKey = normalizeGroupKey(detail.groupKey || row.groupKey || row.groupLabel);
  const enabled = detail.enabled ?? account.enabled ?? true;
  const modelRewriteSettings = parseCodexModelRewriteSettings(account.model_rewrite_rules || account.modelRewriteRules || '');
  const modelRewriteDetailText = modelRewriteSettings.enabled
    ? (modelRewriteSettings.rules || [])
      .map((rule) => `${rule.source} -> ${rule.target}`)
      .join(' / ')
    : '未启用';
  const actionBusy = Boolean(busyKey);
  const moveBusy = actionBusy;
  const groupActions = [
    { key: 'primary', label: '主组' },
    { key: 'backup', label: '备组' },
    { key: 'cold', label: '冷备' }
  ];

  const nav = document.querySelector('nav.sticky');
  const topOffset = nav ? nav.getBoundingClientRect().bottom : 0;

  return createPortal(
    <div className="fixed inset-0 z-[45] flex justify-end" style={{ top: topOffset }}>
      <button
        type="button"
        aria-label="关闭账号详情"
        className="absolute inset-0 bg-slate-950/20"
        onClick={onClose}
      />

      <aside className="relative z-10 flex h-full w-full max-w-[560px] flex-col border-l border-slate-200 bg-slate-50 shadow-2xl shadow-slate-950/20">
        <div className="flex items-start justify-between gap-4 border-b border-slate-200 bg-white px-5 py-4">
          <div className="min-w-0 space-y-2">
            <div className="text-xs font-semibold uppercase tracking-widest text-slate-400">账号详情</div>
            <div className="text-xl font-semibold text-slate-900">{row.name || '-'}</div>
            <div className="flex flex-wrap gap-2">
              <Badge text={row.authLabel || '-'} className="bg-indigo-50 text-indigo-700 border-indigo-200" />
              <Badge text={row.planLabel || '-'} className="bg-violet-50 text-violet-700 border-violet-200" />
              <Badge text={row.groupLabel || '-'} className="bg-sky-50 text-sky-700 border-sky-200" />
              <Badge text={row.stateLabel || '-'} className={TONE_CLASS[row.stateTone] || TONE_CLASS.slate} />
            </div>
          </div>

          <button
            type="button"
            aria-label="关闭抽屉"
            onClick={onClose}
            className="rounded-xl border border-slate-200 bg-white p-2 text-slate-400 transition-colors hover:text-slate-700"
          >
            <X size={18} />
          </button>
        </div>

        <div className="flex-1 space-y-4 overflow-y-auto px-5 py-5">
          <SectionCard icon={Gauge} title="额度">
            <QuotaBar label="5h 额度" text={row.quota5hText} percent={row.quota5hPercent} />
            <QuotaBar label="d7 额度" text={row.quota7dText} percent={row.quota7dPercent} />
            <DetailRow label="额度状态" value={detail.quotaStatusLabel || detail.quotaStatus || '-'} />
            <DetailRow label="最近刷新" value={row.refreshedAtText} />
            {detail.quota5hResetText ? <DetailRow label="5h 重置" value={detail.quota5hResetText} /> : null}
            {detail.quota7dResetText ? <DetailRow label="d7 重置" value={detail.quota7dResetText} /> : null}
            {!detail.quota5hResetText && !detail.quota7dResetText ? (
              <DetailRow label="下次重置" value={detail.nextResetText || detail.quotaResetText || '-'} />
            ) : null}
          </SectionCard>

          <SectionCard icon={Shield} title="健康">
            <DetailRow label="最近成功" value={row.lastSuccessText} />
            <DetailRow label="连通性" value={detail.healthLabel || detail.reachabilityLabel || '-'} />
            <DetailRow label="异常摘要" value={detail.riskLabel || detail.lastErrorText || '-'} />
            <DetailRow label="观察备注" value={detail.healthNote || detail.note || '-'} />
          </SectionCard>

          <SectionCard icon={Activity} title="路由配置">
            <DetailRow label="路由组别" value={row.groupLabel} />
            <DetailRow label="组内顺序" value={detail.priorityLabel || detail.priority || '-'} />
            <DetailRow label="组内角色" value={detail.groupOrderLabel || detail.groupOrder || '-'} />
            <DetailRow label="Base URL" value={detail.baseUrl || detail.baseURL || '-'} />
            <DetailRow
              label="模型兼容"
              value={modelRewriteDetailText}
            />
            <DetailRow label="路由备注" value={detail.routingNote || detail.routeNote || '-'} />
          </SectionCard>

          <SectionCard icon={Settings} title="动作">
            <div className="grid grid-cols-3 gap-2">
              <Button
                variant={enabled ? 'secondary' : 'primary'}
                size="sm"
                disabled={actionBusy}
                onClick={() => onToggleAccount?.(account)}
              >
                {enabled ? '停用账号' : '启用账号'}
              </Button>
              <Button variant="secondary" size="sm" disabled={actionBusy} onClick={() => onTestAccount?.(account)}>
                测试账号
              </Button>
              <Button variant="secondary" size="sm" disabled={actionBusy} onClick={() => onRefreshAccountProfile?.(account)}>
                刷新画像
              </Button>
              <Button variant="secondary" size="sm" disabled={actionBusy} onClick={() => onEditAccount?.(account)}>
                编辑账号
              </Button>
              {groupActions.map((action) => {
                const isCurrentGroup = currentGroupKey === action.key;
                const disabled = moveBusy || isCurrentGroup;
                const buttonText = isCurrentGroup ? `已在${action.label}` : `移到${action.label}`;
                const title = moveBusy
                  ? '正在更新组别，请稍候'
                  : isCurrentGroup
                    ? `当前账号已在${action.label}，无需移动`
                    : `将当前账号移动到${action.label}`;

                return (
                  <Button
                    key={action.key}
                    variant="secondary"
                    size="sm"
                    disabled={disabled}
                    title={title}
                    onClick={() => onMoveAccountToTier?.(account, action.key)}
                  >
                    {buttonText}
                  </Button>
                );
              })}
            </div>
            <div className="mt-3 border-t border-slate-100 pt-3">
              <Button variant="danger" size="sm" className="w-full" disabled={actionBusy} onClick={() => onDeleteAccount?.(account)}>
                删除账号
              </Button>
            </div>
          </SectionCard>
        </div>
      </aside>
    </div>,
    document.body
  );
};

export default AccountDetailsDrawer;
