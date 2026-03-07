// ============================================
// Account Pool 页面 - 账号管理
// 2026-03-05
// ============================================

import { useCallback, useEffect, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import { BrowserOpenURL } from '@wailsjs/runtime/runtime';
import {
  AlertCircle,
  CheckCircle2,
  Edit3,
  Info,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  Users
} from 'lucide-react';
import { Button, EmptyState, ErrorMessage, LoadingSpinner } from '@components/ui';
import {
  createUpstreamAccount,
  deleteUpstreamAccount,
  exchangeChatGPTOAuthCallback,
  fetchUpstreamAccounts,
  formatTimestamp,
  generateChatGPTOAuthLink,
  refreshUpstreamAccountProfile,
  testUpstreamAccount,
  toggleUpstreamAccount,
  updateUpstreamAccount
} from '@utils/api.js';

const DEFAULT_BASE_URL = 'https://api.openai.com';

const AUTH_METHOD_OPTIONS = [
  {
    value: 'chatgpt_refresh_token',
    label: 'ChatGPT RT',
    description: 'ChatGPT 账号授权（使用 Refresh Token / rt）'
  },
  {
    value: 'api_key',
    label: 'API Key',
    description: 'OpenAI Responses API Key'
  }
];

const providerTypeToAuthMethod = (providerType = '') => {
  const type = String(providerType).trim().toLowerCase();
  if (['chatgpt_refresh_token', 'chatgpt_rt', 'refresh_token', 'rt', 'oauth', 'openai_oauth'].includes(type)) return 'chatgpt_refresh_token';
  return 'api_key';
};

const authMethodToProviderType = (authMethod = '') => {
  if (authMethod === 'chatgpt_refresh_token') return 'chatgpt_refresh_token';
  return 'api_key';
};

const toAccountAuthLabel = (providerType = '') => {
  const type = String(providerType).trim().toLowerCase();
  if (['chatgpt_refresh_token', 'chatgpt_rt', 'refresh_token', 'rt', 'oauth', 'openai_oauth'].includes(type)) return 'ChatGPT RT';
  if (type === 'api_key') return 'API Key';
  return providerType || '-';
};

const ACCOUNT_STATE_STYLE = {
  active: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  cooldown: 'bg-amber-50 text-amber-700 border-amber-200',
  disabled_auth: 'bg-rose-50 text-rose-700 border-rose-200',
  disabled: 'bg-slate-100 text-slate-600 border-slate-200'
};

const QUOTA_STATUS_STYLE = {
  ok: 'bg-sky-50 text-sky-700 border-sky-200',
  unavailable: 'bg-slate-100 text-slate-600 border-slate-200',
  exhausted: 'bg-amber-50 text-amber-700 border-amber-200',
  workspace_deactivated: 'bg-rose-50 text-rose-700 border-rose-200',
  auth_invalid: 'bg-rose-50 text-rose-700 border-rose-200',
  pending: 'bg-slate-100 text-slate-600 border-slate-200'
};

const PLAN_TYPE_LABELS = {
  free: 'Free',
  plus: 'Plus',
  team: 'Team',
  enterprise: 'Enterprise',
  unknown: 'Unknown'
};

const EMPTY_ACCOUNT_FORM = {
  account_name: '',
  auth_method: 'chatgpt_refresh_token',
  provider_type: 'chatgpt_refresh_token',
  priority: '1',
  enabled: true,
  credential_raw: '',
  base_url: DEFAULT_BASE_URL
};

const buildOAuthCredentialRaw = (result = {}) => {
  if (result?.credential_raw) {
    return result.credential_raw;
  }

  const payload = {};
  if (result?.refresh_token) payload.refresh_token = result.refresh_token;
  if (result?.access_token) payload.access_token = result.access_token;
  if (result?.id_token) payload.id_token = result.id_token;
  if (result?.expires_at) payload.expires_at = result.expires_at;
  if (result?.chatgpt_account_id) payload.chatgpt_account_id = result.chatgpt_account_id;
  return JSON.stringify(payload);
};

const Badge = ({ text, className, title }) => (
  <span title={title} className={`inline-flex items-center px-2 py-0.5 text-xs rounded-full border ${className}`}>
    {text}
  </span>
);

const FormField = ({ label, required = false, children }) => (
  <label className="block">
    <div className="text-xs font-medium text-slate-600 mb-1.5">
      {label}
      {required && <span className="text-rose-500 ml-1">*</span>}
    </div>
    {children}
  </label>
);

const Modal = ({
  open,
  title,
  submitText,
  submitVariant = 'primary',
  cancelText = '取消',
  onClose,
  onSubmit,
  submitting,
  children
}) => {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center px-4">
      <div className="absolute inset-0 bg-slate-900/40" onClick={() => !submitting && onClose()} />
      <form
        onSubmit={onSubmit}
        className="relative w-full max-w-xl bg-white rounded-2xl border border-slate-200 shadow-2xl"
      >
        <div className="px-6 py-4 border-b border-slate-100 flex items-center justify-between">
          <h3 className="text-lg font-semibold text-slate-900">{title}</h3>
          <button
            type="button"
            className="text-slate-400 hover:text-slate-600 text-sm"
            onClick={onClose}
            disabled={submitting}
          >
            关闭
          </button>
        </div>

        <div className="px-6 py-5 space-y-4 max-h-[70vh] overflow-y-auto">
          {children}
        </div>

        <div className="px-6 py-4 border-t border-slate-100 flex items-center justify-end gap-2">
          <Button type="button" variant="secondary" onClick={onClose} disabled={submitting}>
            {cancelText}
          </Button>
          <Button type="submit" variant={submitVariant} loading={submitting}>
            {submitText}
          </Button>
        </div>
      </form>
    </div>
  );
};

const AccountFormModal = ({
  open,
  title,
  submitText,
  cancelText = '取消',
  onClose,
  onSubmit,
  submitting,
  children
}) => {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-[60] flex items-start justify-center px-4 pt-[15vh]">
      <div className="absolute inset-0 bg-slate-900/40" onClick={() => !submitting && onClose()} />
      <form
        onSubmit={onSubmit}
        className="relative flex w-full max-w-2xl max-h-[75vh] flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl"
      >
        <div className="flex items-center justify-between border-b border-slate-100 px-6 py-4">
          <h3 className="text-lg font-semibold text-slate-900">{title}</h3>
          <button
            type="button"
            className="text-sm text-slate-400 hover:text-slate-600"
            onClick={onClose}
            disabled={submitting}
          >
            关闭
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-6 py-5">
          <div className="space-y-6">
            {children}
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-slate-100 bg-white px-6 py-4">
          <Button type="button" variant="ghost" onClick={onClose} disabled={submitting}>
            {cancelText}
          </Button>
          <Button type="submit" loading={submitting}>
            {submitText}
          </Button>
        </div>
      </form>
    </div>
  );
};

const NoticeToast = ({ notice, onClose }) => {
  if (!notice || typeof document === 'undefined') return null;

  return createPortal(
    <div
      className="fixed top-4 right-4 z-[70] pointer-events-none"
      style={{ animation: 'account-pool-toast-slide-in 0.25s ease-out' }}
    >
      <style>{`
        @keyframes account-pool-toast-slide-in {
          from { opacity: 0; transform: translateY(-8px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>
      <div className={`
        pointer-events-auto flex items-center gap-2 px-4 py-3 rounded-lg text-sm border shadow-lg max-w-md
        ${notice.type === 'success' ? 'bg-emerald-50 text-emerald-700 border-emerald-200' : ''}
        ${notice.type === 'error' ? 'bg-rose-50 text-rose-700 border-rose-200' : ''}
        ${notice.type === 'info' ? 'bg-slate-50 text-slate-600 border-slate-200' : ''}
      `}>
        {notice.type === 'success' ? <CheckCircle2 size={16} className="shrink-0" /> : notice.type === 'info' ? <Info size={16} className="shrink-0" /> : <AlertCircle size={16} className="shrink-0" />}
        <span className="break-words">{notice.text}</span>
        <button
          type="button"
          onClick={onClose}
          className="ml-auto shrink-0 p-0.5 rounded hover:bg-black/5 transition-colors"
        >
          <span className="sr-only">关闭</span>
          <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"><path d="M3.5 3.5l7 7M10.5 3.5l-7 7" /></svg>
        </button>
      </div>
    </div>,
    document.body
  );
};

const toDisplayTime = (value) => (value ? formatTimestamp(value) : '-');

const normalizePlanType = (value = '') => {
  const normalized = String(value || '').trim().toLowerCase();
  return normalized || '';
};

const toPlanTypeLabel = (value = '') => {
  const normalized = normalizePlanType(value);
  if (!normalized) {
    return '';
  }
  if (PLAN_TYPE_LABELS[normalized]) {
    return PLAN_TYPE_LABELS[normalized];
  }
  return normalized
    .replace(/[_-]+/g, ' ')
    .split(/\s+/)
    .filter(Boolean)
    .map((part) => part[0].toUpperCase() + part.slice(1))
    .join(' ');
};

const toRemainingPercent = (usedPercent) => {
  const used = Number.parseFloat(usedPercent);
  if (!Number.isFinite(used)) {
    return null;
  }
  return Math.max(0, Math.min(100, 100 - used));
};

const toQuotaProgressClass = (remainingPercent) => {
  if (!Number.isFinite(remainingPercent)) {
    return 'bg-slate-200';
  }
  if (remainingPercent > 50) {
    return 'bg-emerald-400';
  }
  if (remainingPercent > 20) {
    return 'bg-amber-400';
  }
  return 'bg-rose-400';
};

const toQuotaStatusLabel = (status = '') => {
  const normalized = String(status || '').trim().toLowerCase();
  const labels = {
    ok: '正常',
    unavailable: '暂不可用',
    exhausted: '已耗尽',
    workspace_deactivated: '工作区停用',
    auth_invalid: '鉴权失效',
    pending: '未刷新'
  };
  return labels[normalized] || labels.pending;
};

const toAccountStateLabel = (state) => {
  const stateMap = {
    active: '可用',
    cooldown: '冷却中',
    disabled_auth: '鉴权失效',
    disabled: '已禁用'
  };
  return stateMap[state] || (state || '未知');
};

const MANUAL_FAILOVER_TIER_PRESETS = [
  {
    label: '主组',
    className: 'bg-indigo-50 text-indigo-700 border-indigo-200',
    description: '当前请求优先尝试这一层账号'
  },
  {
    label: '备组',
    className: 'bg-cyan-50 text-cyan-700 border-cyan-200',
    description: '主组全部失败后切到这一层'
  },
  {
    label: '兜底组',
    className: 'bg-violet-50 text-violet-700 border-violet-200',
    description: '前两层都不可用时，再切到这一层'
  }
];

const normalizePriorityValue = (value) => {
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : null;
};

const buildManualFailoverTierSummary = (accounts = []) => {
  const counts = new Map();

  accounts.forEach((account) => {
    const priority = normalizePriorityValue(account?.priority ?? account?.Priority);
    if (!Number.isFinite(priority)) {
      return;
    }
    counts.set(priority, (counts.get(priority) || 0) + 1);
  });

  return Array.from(counts.entries())
    .sort((left, right) => left[0] - right[0])
    .map(([priority, count], index) => {
      const preset = MANUAL_FAILOVER_TIER_PRESETS[index];
      return {
        priority,
        count,
        order: index + 1,
        label: preset?.label || `第 ${index + 1} 层`,
        className: preset?.className || 'bg-slate-100 text-slate-700 border-slate-200',
        description: preset?.description || '更低优先级的手动兜底层'
      };
    });
};

const compareAccountsByManualPriority = (left = {}, right = {}) => {
  const leftPriority = normalizePriorityValue(left?.priority ?? left?.Priority);
  const rightPriority = normalizePriorityValue(right?.priority ?? right?.Priority);

  if (Number.isFinite(leftPriority) && Number.isFinite(rightPriority) && leftPriority !== rightPriority) {
    return leftPriority - rightPriority;
  }
  if (Number.isFinite(leftPriority) && !Number.isFinite(rightPriority)) {
    return -1;
  }
  if (!Number.isFinite(leftPriority) && Number.isFinite(rightPriority)) {
    return 1;
  }

  const leftId = resolveAccountId(left);
  const rightId = resolveAccountId(right);
  if (typeof leftId === 'number' && typeof rightId === 'number' && leftId !== rightId) {
    return leftId - rightId;
  }
  return String(leftId ?? '').localeCompare(String(rightId ?? ''));
};

const buildManualFailoverTierGroups = (accounts = []) => {
  const sorted = [...accounts].sort(compareAccountsByManualPriority);
  const groups = [];

  sorted.forEach((account) => {
    const priority = normalizePriorityValue(account?.priority ?? account?.Priority);
    const lastGroup = groups[groups.length - 1];

    if (lastGroup && lastGroup.priority === priority) {
      lastGroup.accounts.push(account);
      return;
    }

    groups.push({
      priority,
      accounts: [account]
    });
  });

  return groups;
};

const buildManualFailoverPriorityPlan = ({ accounts = [], targetAccountId, targetTierIndex }) => {
  const tiers = buildManualFailoverTierGroups(accounts);
  if (!tiers.length || targetAccountId === null || targetAccountId === undefined) {
    return [];
  }

  const remainingTiers = [];
  let targetAccount = null;

  tiers.forEach((tier) => {
    const nextAccounts = [];
    tier.accounts.forEach((account) => {
      const accountId = resolveAccountId(account);
      if (targetAccount === null && accountId === targetAccountId) {
        targetAccount = account;
        return;
      }
      nextAccounts.push(account);
    });
    if (nextAccounts.length > 0) {
      remainingTiers.push({ priority: tier.priority, accounts: nextAccounts });
    }
  });

  if (!targetAccount) {
    return [];
  }

  const insertIndex = Math.max(0, Math.min(targetTierIndex, remainingTiers.length));
  remainingTiers.splice(insertIndex, 0, { priority: null, accounts: [targetAccount] });

  return remainingTiers.flatMap((tier, index) => {
    const nextPriority = (index + 1) * 10;
    return tier.accounts
      .filter((account) => normalizePriorityValue(account?.priority ?? account?.Priority) !== nextPriority)
      .map((account) => ({ account, priority: nextPriority }));
  });
};

const buildAccountUpdatePayload = (account, priority) => ({
  provider_type: String(account?.provider_type ?? account?.providerType ?? '').trim(),
  account_name: String(account?.account_name ?? account?.accountName ?? '').trim(),
  credential_raw: String(account?.credential_raw ?? account?.credentialRaw ?? '').trim(),
  base_url: String(account?.base_url ?? account?.baseURL ?? DEFAULT_BASE_URL).trim() || DEFAULT_BASE_URL,
  priority,
  enabled: account?.enabled !== false
});

const normalizeEntityId = (value) => {
  if (value === null || value === undefined) return null;
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return null;
    const numeric = Number(trimmed);
    return Number.isNaN(numeric) ? trimmed : numeric;
  }
  return null;
};

const resolveAccountId = (account = {}) => normalizeEntityId(
  account.id
  ?? account.ID
  ?? account.Id
  ?? account.account_id
  ?? account.accountId
  ?? account.accountID
  ?? account.AccountID
  ?? null
);

const summarizeCallbackURL = (raw = '') => {
  const text = String(raw || '').trim();
  if (!text) {
    return {
      normalized: '',
      hasCode: false,
      hasState: false,
      hasRefreshToken: false,
      hasFragment: false,
      oauthError: '',
      oauthErrorDescription: ''
    };
  }
  try {
    const parsed = new URL(text);
    const hasCode = !!parsed.searchParams.get('code');
    const hasState = !!parsed.searchParams.get('state');
    const hasRefreshToken = !!(parsed.searchParams.get('refresh_token') || parsed.searchParams.get('rt'));
    let oauthError = parsed.searchParams.get('error') || '';
    let oauthErrorDescription = parsed.searchParams.get('error_description') || '';

    let hasFragment = false;
    if (parsed.hash) {
      const fragQuery = parsed.hash.startsWith('#') ? parsed.hash.slice(1) : parsed.hash;
      const fragParams = new URLSearchParams(fragQuery);
      hasFragment = fragParams.toString() !== '';
      if (!oauthError) oauthError = fragParams.get('error') || '';
      if (!oauthErrorDescription) oauthErrorDescription = fragParams.get('error_description') || '';
    }

    return {
      normalized: `${parsed.protocol}//${parsed.host}${parsed.pathname}`,
      hasCode,
      hasState,
      hasRefreshToken,
      hasFragment,
      oauthError,
      oauthErrorDescription
    };
  } catch {
    return {
      normalized: text.slice(0, 160),
      hasCode: text.includes('code='),
      hasState: text.includes('state='),
      hasRefreshToken: text.includes('refresh_token=') || text.includes('rt='),
      hasFragment: text.includes('#'),
      oauthError: text.includes('error=') ? 'unknown' : '',
      oauthErrorDescription: ''
    };
  }
};

const maskSessionId = (sessionId = '') => {
  const text = String(sessionId || '').trim();
  if (text.length <= 6) return '***';
  return `${text.slice(0, 6)}***`;
};

const openExternalURL = (url = '') => {
  const target = String(url || '').trim();
  if (!target) return false;

  if (typeof window !== 'undefined' && window.runtime?.BrowserOpenURL) {
    BrowserOpenURL(target);
    return true;
  }

  const opened = window.open(target, '_blank', 'noopener,noreferrer');
  return opened !== null;
};

const AccountPoolPage = () => {
  const [accounts, setAccounts] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busyKey, setBusyKey] = useState('');
  const [notice, setNotice] = useState(null);

  const [accountModalOpen, setAccountModalOpen] = useState(false);
  const [accountSubmitting, setAccountSubmitting] = useState(false);
  const [editingAccount, setEditingAccount] = useState(null);
  const [accountForm, setAccountForm] = useState(EMPTY_ACCOUNT_FORM);
  const [oauthActionLoading, setOauthActionLoading] = useState(false);
  const [oauthSectionExpanded, setOauthSectionExpanded] = useState(true);
  const [oauthSession, setOauthSession] = useState(null);
  const [oauthCallbackURL, setOauthCallbackURL] = useState('');
  const [deleteTarget, setDeleteTarget] = useState(null);

  const resetOAuthWorkflow = useCallback(() => {
    setOauthSession(null);
    setOauthCallbackURL('');
  }, []);

  const accountCount = accounts.length;
  const activeAccountCount = accounts.filter(item => item.enabled && item.state !== 'disabled_auth').length;
  const authFailedCount = accounts.filter(item => item.state === 'disabled_auth').length;
  const priorityTierSummary = useMemo(() => buildManualFailoverTierSummary(accounts), [accounts]);
  const priorityTierMetaMap = useMemo(
    () => new Map(priorityTierSummary.map(item => [item.priority, item])),
    [priorityTierSummary]
  );

  useEffect(() => {
    if (!notice) return undefined;
    const timer = setTimeout(() => setNotice(null), 4000);
    return () => clearTimeout(timer);
  }, [notice]);

  const showNotice = useCallback((type, text) => {
    setNotice({ type, text });
  }, []);

  const loadData = useCallback(async ({ silent = false } = {}) => {
    try {
      if (!silent) {
        setLoading(true);
      }
      setError('');

      const accountData = await fetchUpstreamAccounts();
      setAccounts(Array.isArray(accountData) ? accountData : []);
    } catch (err) {
      setError(err.message || '加载账号数据失败');
    } finally {
      if (!silent) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const runRowAction = useCallback(async (key, action, successText, { skipRefresh = false } = {}) => {
    setBusyKey(key);
    try {
      const result = await action();

      if (result?.unsupported) {
        showNotice('info', result.message || '当前后端版本暂不支持该操作');
      } else if (result?.success === false) {
        showNotice('error', result.message || '操作失败');
      } else if (successText) {
        const text = typeof successText === 'function' ? successText(result) : successText;
        if (text) {
          showNotice('success', text);
        }
      }

      if (!skipRefresh) {
        await loadData({ silent: true });
      }
      return result;
    } catch (err) {
      showNotice('error', err.message || '操作失败');
      return null;
    } finally {
      setBusyKey('');
    }
  }, [loadData, showNotice]);

  const handleGenerateOAuthLink = async () => {
    setOauthActionLoading(true);
    try {
      const result = await generateChatGPTOAuthLink();
      if (result?.unsupported) {
        showNotice('error', result.message || '当前后端版本暂不支持生成授权链接');
        return;
      }
      if (!result?.auth_url || !result?.session_id) {
        showNotice('error', '生成授权链接失败：返回数据不完整');
        return;
      }
      setOauthSession(result);
      showNotice('success', '授权链接已生成，请登录后粘贴回调 URL');
    } catch (err) {
      showNotice('error', err.message || '生成授权链接失败');
    } finally {
      setOauthActionLoading(false);
    }
  };

  const handleExtractRTFromCallback = async () => {
    if (!oauthSession?.session_id) {
      showNotice('error', '请先生成授权链接');
      return;
    }
    const callbackURL = oauthCallbackURL.trim();
    if (!callbackURL) {
      showNotice('error', '请粘贴回调 URL');
      return;
    }

    setOauthActionLoading(true);
    const callbackMeta = summarizeCallbackURL(callbackURL);
    console.info('[AccountPool][OAuth] 开始回调解析', {
      sessionId: maskSessionId(oauthSession.session_id),
      callback: callbackMeta.normalized,
      hasCode: callbackMeta.hasCode,
      hasState: callbackMeta.hasState,
      hasRefreshToken: callbackMeta.hasRefreshToken,
      hasFragment: callbackMeta.hasFragment,
      oauthError: callbackMeta.oauthError || ''
    });
    if (callbackMeta.oauthError) {
      const detail = callbackMeta.oauthErrorDescription
        ? `${callbackMeta.oauthError} - ${callbackMeta.oauthErrorDescription}`
        : callbackMeta.oauthError;
      console.warn('[AccountPool][OAuth] 回调 URL 包含 OAuth 错误参数', {
        sessionId: maskSessionId(oauthSession.session_id),
        callback: callbackMeta.normalized,
        oauthError: callbackMeta.oauthError,
        oauthErrorDescription: callbackMeta.oauthErrorDescription || ''
      });
      showNotice('error', `OAuth 授权失败：${detail}`);
      setOauthActionLoading(false);
      return;
    }
    try {
      const result = await exchangeChatGPTOAuthCallback(oauthSession.session_id, callbackURL);
      if (result?.unsupported) {
        console.warn('[AccountPool][OAuth] 回调解析不受支持', result);
        showNotice('error', result.message || '当前后端版本暂不支持回调解析');
        return;
      }
      if (result?.success !== true || !(result?.credential_raw || result?.refresh_token)) {
        console.warn('[AccountPool][OAuth] 回调解析失败', {
          sessionId: maskSessionId(oauthSession.session_id),
          callback: callbackMeta.normalized,
          result
        });
        showNotice('error', result?.message || '未能提取到 refresh token');
        return;
      }
      setAccountForm(prev => ({
        ...prev,
        credential_raw: buildOAuthCredentialRaw(result),
        provider_type: 'chatgpt_refresh_token',
        auth_method: 'chatgpt_refresh_token'
      }));
      console.info('[AccountPool][OAuth] 回调解析成功', {
        sessionId: maskSessionId(oauthSession.session_id),
        callback: callbackMeta.normalized
      });
      showNotice('success', '已自动提取 RT 并填入账号凭据');
    } catch (err) {
      console.error('[AccountPool][OAuth] 回调解析异常', {
        sessionId: maskSessionId(oauthSession.session_id),
        callback: callbackMeta.normalized,
        error: err
      });
      showNotice('error', err.message || '回调解析失败');
    } finally {
      setOauthActionLoading(false);
    }
  };

  const openCreateAccount = () => {
    setEditingAccount(null);
    setAccountForm(EMPTY_ACCOUNT_FORM);
    setOauthSectionExpanded(true);
    resetOAuthWorkflow();
    setAccountModalOpen(true);
  };

  const openEditAccount = (account) => {
    const providerType = account.provider_type || account.providerType || 'chatgpt_refresh_token';
    setEditingAccount(account);
    setAccountForm({
      account_name: account.account_name || account.accountName || '',
      auth_method: providerTypeToAuthMethod(providerType),
      provider_type: providerType,
      priority: String(account.priority || 1),
      enabled: account.enabled !== false,
      credential_raw: account.credential_raw || account.credentialRaw || '',
      base_url: account.base_url || account.baseURL || DEFAULT_BASE_URL
    });
    setOauthSectionExpanded(false);
    resetOAuthWorkflow();
    setAccountModalOpen(true);
  };

  const submitAccountForm = async (event) => {
    event.preventDefault();

    const accountName = accountForm.account_name.trim();
    const authMethod = accountForm.auth_method || providerTypeToAuthMethod(accountForm.provider_type);
    const providerType = authMethodToProviderType(authMethod);
    const credentialRaw = accountForm.credential_raw.trim();

    if (!accountName) {
      showNotice('error', '请填写账号名称');
      return;
    }
    if (!credentialRaw) {
      showNotice('error', authMethod === 'chatgpt_refresh_token' ? '请填写 ChatGPT Refresh Token (rt)' : '请填写 API Key');
      return;
    }

    const priorityValue = Number.parseInt(accountForm.priority, 10);
    const baseURL = (accountForm.base_url || DEFAULT_BASE_URL).trim() || DEFAULT_BASE_URL;

    setAccountSubmitting(true);
    try {
      const payload = {
        ...accountForm,
        auth_method: authMethod,
        account_name: accountName,
        provider_type: providerType,
        priority: Number.isNaN(priorityValue) ? 1 : priorityValue,
        enabled: accountForm.enabled !== false,
        credential_raw: credentialRaw,
        base_url: baseURL
      };

      if (editingAccount) {
        const accountId = resolveAccountId(editingAccount);
        if (accountId === undefined || accountId === null || accountId === '') {
          throw new Error('账号缺少 ID，无法更新');
        }
        await updateUpstreamAccount(accountId, payload);
        showNotice('success', `已更新账号「${accountName}」`);
      } else {
        await createUpstreamAccount(payload);
        showNotice('success', `已新增账号「${accountName}」`);
      }

      setAccountModalOpen(false);
      setEditingAccount(null);
      setAccountForm(EMPTY_ACCOUNT_FORM);
      resetOAuthWorkflow();
      await loadData({ silent: true });
    } catch (err) {
      showNotice('error', err.message || '保存账号失败');
    } finally {
      setAccountSubmitting(false);
    }
  };

  const handleDeleteAccount = async (account) => {
    const accountId = resolveAccountId(account);
    if (accountId === undefined || accountId === null || accountId === '') {
      showNotice('error', '账号缺少 ID，无法删除');
      return;
    }
    setDeleteTarget(account);
  };

  const handleConfirmDeleteAccount = async (event) => {
    event.preventDefault();
    if (!deleteTarget) return;

    const accountId = resolveAccountId(deleteTarget);
    if (accountId === undefined || accountId === null || accountId === '') {
      showNotice('error', '账号缺少 ID，无法删除');
      setDeleteTarget(null);
      return;
    }

    const result = await runRowAction(
      `account-delete-${accountId}`,
      () => deleteUpstreamAccount(accountId),
      `已删除账号「${deleteTarget.account_name || deleteTarget.accountName}」`
    );

    if (result && result.success !== false && !result.unsupported) {
      setDeleteTarget(null);
    }
  };

  const handleToggleAccount = async (account) => {
    const accountId = resolveAccountId(account);
    if (accountId === undefined || accountId === null || accountId === '') {
      showNotice('error', '账号缺少 ID，无法切换状态');
      return;
    }

    const nextEnabled = !account.enabled;
    await runRowAction(
      `account-toggle-${accountId}`,
      () => toggleUpstreamAccount(accountId, nextEnabled),
      nextEnabled ? `已启用「${account.account_name || account.accountName}」` : `已停用「${account.account_name || account.accountName}」`
    );
  };

  const handleMoveAccountToTier = async (account, targetTier) => {
    const accountId = resolveAccountId(account);
    if (accountId === undefined || accountId === null || accountId === '') {
      showNotice('error', '账号缺少 ID，无法切换顺序');
      return;
    }

    const targetTierIndex = targetTier === 'backup' ? 1 : 0;
    const changes = buildManualFailoverPriorityPlan({
      accounts,
      targetAccountId: accountId,
      targetTierIndex
    });

    if (changes.length === 0) {
      showNotice('info', targetTier === 'backup' ? '该账号已在备组位置' : '该账号已在主组位置');
      return;
    }

    setBusyKey(`account-switch-${accountId}`);
    try {
      for (const change of changes) {
        const changeId = resolveAccountId(change.account);
        if (changeId === undefined || changeId === null || changeId === '') {
          throw new Error('存在缺少 ID 的账号，无法更新顺序');
        }
        await updateUpstreamAccount(changeId, buildAccountUpdatePayload(change.account, change.priority));
      }

      const accountName = account.account_name || account.accountName || `账号 ${accountId}`;
      showNotice(
        'success',
        targetTier === 'backup'
          ? `已将「${accountName}」切到备组，顺序已立即生效`
          : `已将「${accountName}」切到主组，顺序已立即生效`
      );
      await loadData({ silent: true });
    } catch (err) {
      showNotice('error', err.message || '手动切换顺序失败');
    } finally {
      setBusyKey('');
    }
  };

  const handleTestAccount = async (account) => {
    const accountId = resolveAccountId(account);
    if (accountId === undefined || accountId === null || accountId === '') {
      showNotice('error', '账号缺少 ID，无法测试');
      return;
    }

    await runRowAction(
      `account-test-${accountId}`,
      () => testUpstreamAccount(accountId),
      (result) => result?.message || `已触发账号「${account.account_name || account.accountName}」连通性测试`
    );
  };

  const handleRefreshAccountProfile = async (account) => {
    const accountId = resolveAccountId(account);
    if (accountId === undefined || accountId === null || accountId === '') {
      showNotice('error', '账号缺少 ID，无法刷新账号信息');
      return;
    }

    await runRowAction(
      `account-profile-${accountId}`,
      () => refreshUpstreamAccountProfile(accountId),
      (result) => result?.message || `已刷新账号「${account.account_name || account.accountName}」的信息`
    );
  };

  if (error && !loading) {
    return (
      <ErrorMessage
        title="账号数据加载失败"
        message={error}
        onRetry={() => loadData()}
      />
    );
  }

  if (loading) {
    return <LoadingSpinner text="加载账号数据..." />;
  }

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-2 duration-500">
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
        <Button
          icon={RefreshCw}
          variant="secondary"
          onClick={async () => {
            setBusyKey('reload');
            await loadData({ silent: true });
            setBusyKey('');
          }}
          loading={busyKey === 'reload'}
        >
          刷新数据
        </Button>
      </div>

      <NoticeToast notice={notice} onClose={() => setNotice(null)} />

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="bg-white rounded-xl border border-slate-200/60 p-4 shadow-sm">
          <div className="text-2xl font-bold text-slate-900">{accountCount}</div>
          <div className="text-sm text-slate-500">账号总数</div>
        </div>
        <div className="bg-white rounded-xl border border-emerald-200/70 p-4 shadow-sm">
          <div className="text-2xl font-bold text-emerald-700">{activeAccountCount}</div>
          <div className="text-sm text-emerald-600">可用账号</div>
        </div>
        <div className="bg-white rounded-xl border border-rose-200/70 p-4 shadow-sm">
          <div className="text-2xl font-bold text-rose-700">{authFailedCount}</div>
          <div className="text-sm text-rose-600">鉴权失效</div>
        </div>
      </div>

      <section className="bg-white rounded-2xl border border-slate-200/70 shadow-sm overflow-hidden">
        <div className="px-5 py-4 border-b border-slate-100 flex flex-col md:flex-row md:items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <Users size={18} className="text-indigo-600" />
            <h2 className="text-base font-semibold text-slate-900">账号列表</h2>
          </div>
          <Button icon={Plus} size="sm" onClick={openCreateAccount}>
            新增账号
          </Button>
        </div>

        <div className="px-5 pt-4">
          <div className="rounded-xl border border-sky-100 bg-sky-50/70 px-4 py-3">
            <div className="flex items-start gap-3">
              <div className="mt-0.5 rounded-lg bg-white/80 p-1.5 text-sky-600 shadow-sm">
                <Info size={14} />
              </div>
              <div className="min-w-0 flex-1">
                <div className="text-sm font-medium text-slate-900">当前为手动主备模式</div>
                <div className="mt-1 text-xs leading-5 text-slate-600">
                  priority 越小越优先，相同 priority 视为同一层；请求会按层顺序自动切换，V0 暂不在同层内按额度或健康度自动择优。
                </div>
                <div className="mt-1 text-xs leading-5 text-slate-500">
                  可直接在账号行点击“设为主组 / 设为备组”快速切换，也可以继续手动编辑 priority。
                </div>
                {priorityTierSummary.length > 0 && (
                  <div className="mt-3 flex flex-wrap gap-2">
                    {priorityTierSummary.map((tier) => (
                      <Badge
                        key={`tier-summary-${tier.priority}`}
                        text={`${tier.label} · P${tier.priority}${tier.count > 1 ? ` · ${tier.count} 个账号` : ''}`}
                        className={tier.className}
                        title={tier.description}
                      />
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>

        {accountCount === 0 ? (
          <EmptyState
            icon={Users}
            title="暂无账号"
            description="新增账号后可直接用于 /v1/responses 调度。"
            action={(
              <Button icon={Plus} size="sm" onClick={openCreateAccount}>
                添加账号
              </Button>
            )}
          />
        ) : (
          <div className="divide-y divide-slate-100">
            {accounts.map((account) => {
              const accountId = resolveAccountId(account) ?? account.account_name ?? account.accountName;
              const accountName = account.account_name || account.accountName || '-';
              const state = account.state || 'active';
              const stateClass = ACCOUNT_STATE_STYLE[state] || 'bg-slate-100 text-slate-600 border-slate-200';
              const quotaStatus = String(account.quota_status || account.quotaStatus || '').trim().toLowerCase() || 'pending';
              const quotaStatusClass = QUOTA_STATUS_STYLE[quotaStatus] || QUOTA_STATUS_STYLE.pending;
              const planType = account.plan_type || account.planType || '';
              const normalizedPlanType = normalizePlanType(planType);
              const normalizedProviderType = String(account.provider_type || account.providerType || '').trim().toLowerCase();
              const isAPIKeyAccount = normalizedProviderType === 'api_key';
              const planTypeLabel = toPlanTypeLabel(planType);
              const priority = normalizePriorityValue(account.priority ?? account.Priority);
              const tierMeta = Number.isFinite(priority) ? priorityTierMetaMap.get(priority) : null;
              const canSetAsPrimary = accountCount > 1 && (!tierMeta || tierMeta.order !== 1 || tierMeta.count > 1);
              const canSetAsBackup = accountCount > 1 && (!tierMeta || tierMeta.order !== 2 || tierMeta.count > 1);
              const refreshedAt = account.quota_refreshed_at || account.quotaRefreshedAt;
              const rowBusy = busyKey.startsWith('account-') && busyKey.includes(String(accountId));

              const quota5hUsed = Number.parseFloat(account.quota_5h_used_percent ?? account.quota5hUsedPercent);
              const quotaWeeklyUsed = Number.parseFloat(account.quota_weekly_used_percent ?? account.quotaWeeklyUsedPercent);
              const quota5hRemaining = toRemainingPercent(quota5hUsed);
              const quotaWeeklyRemaining = toRemainingPercent(quotaWeeklyUsed);
              const quota5hResetAt = account.quota_5h_reset_at || account.quota5hResetAt;
              const quotaWeeklyResetAt = account.quota_weekly_reset_at || account.quotaWeeklyResetAt;

              return (
                <div
                  key={String(accountId)}
                  className={`px-5 py-4 hover:bg-slate-50/60 transition-colors ${!account.enabled ? 'opacity-60' : ''}`}
                >
                  {/* 第一行：开关 + 名称 + 徽章 + 状态 + 操作 */}
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
                    {/* 开关 */}
                    <button
                      type="button"
                      role="switch"
                      aria-checked={account.enabled}
                      onClick={() => handleToggleAccount(account)}
                      disabled={rowBusy}
                      title={account.enabled ? '停用' : '启用'}
                      className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors duration-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400 focus-visible:ring-offset-2 ${
                        account.enabled ? 'bg-indigo-500' : 'bg-slate-300'
                      } ${rowBusy ? 'opacity-50 cursor-not-allowed' : ''}`}
                    >
                      <span
                        aria-hidden="true"
                        className={`pointer-events-none inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ${
                          account.enabled ? 'translate-x-[18px]' : 'translate-x-[2px]'
                        }`}
                      />
                    </button>

                    {/* 名称 */}
                    <span className="min-w-0 max-w-full truncate text-sm font-semibold text-slate-900 sm:max-w-[220px]" title={accountName}>
                      {accountName}
                    </span>

                    {/* 授权方式徽章 */}
                    <Badge
                      text={toAccountAuthLabel(account.provider_type || account.providerType || '')}
                      className="bg-indigo-50 text-indigo-600 border-indigo-100"
                    />

                    {tierMeta && (
                      <Badge
                        text={tierMeta.label}
                        className={tierMeta.className}
                        title={`${tierMeta.description}${tierMeta.count > 1 ? `（当前同层共 ${tierMeta.count} 个账号）` : ''}`}
                      />
                    )}

                    {/* 优先级徽章 */}
                    <Badge
                      text={`优先级 ${Number.isFinite(priority) ? priority : '-'}`}
                      className="bg-amber-50 text-amber-700 border-amber-100"
                    />

                    {tierMeta?.count > 1 && (
                      <Badge
                        text={`同层 ${tierMeta.count} 个`}
                        className="bg-white text-slate-500 border-slate-200"
                        title="相同 priority 的账号属于同一层，按手动主备规则依次切换"
                      />
                    )}

                    {/* 账号类型徽章 */}
                    {planTypeLabel && (
                      <Badge text={planTypeLabel} className="bg-violet-50 text-violet-600 border-violet-100" />
                    )}

                    {/* 状态徽章组 */}
                    <div className="flex items-center gap-1.5 md:ml-auto">
                      <Badge text={toQuotaStatusLabel(quotaStatus)} className={quotaStatusClass} />
                      <Badge text={toAccountStateLabel(state)} className={stateClass} />
                    </div>

                    {/* 分隔线 */}
                    <div className="hidden h-5 w-px shrink-0 bg-slate-200 md:block" />

                    <div className="flex items-center gap-1 shrink-0">
                      {canSetAsPrimary ? (
                        <button
                          type="button"
                          onClick={() => handleMoveAccountToTier(account, 'primary')}
                          disabled={rowBusy}
                          className="inline-flex items-center rounded-md border border-indigo-200 bg-indigo-50 px-2 py-1 text-xs font-medium text-indigo-700 transition-colors hover:bg-indigo-100 disabled:cursor-not-allowed disabled:opacity-50"
                          title="将当前账号提升为新的主组，其他账号顺延"
                        >
                          设为主组
                        </button>
                      ) : (
                        <Badge text="当前主组" className="bg-indigo-50 text-indigo-700 border-indigo-200" />
                      )}

                      {canSetAsBackup && (
                        <button
                          type="button"
                          onClick={() => handleMoveAccountToTier(account, 'backup')}
                          disabled={rowBusy}
                          className="inline-flex items-center rounded-md border border-cyan-200 bg-cyan-50 px-2 py-1 text-xs font-medium text-cyan-700 transition-colors hover:bg-cyan-100 disabled:cursor-not-allowed disabled:opacity-50"
                          title="将当前账号切到备组，主组仍优先，其他组顺延"
                        >
                          设为备组
                        </button>
                      )}

                      {!canSetAsBackup && tierMeta?.order === 2 && tierMeta.count === 1 && (
                        <Badge text="当前备组" className="bg-cyan-50 text-cyan-700 border-cyan-200" />
                      )}
                    </div>

                    {/* 操作按钮 */}
                    <div className="flex items-center gap-0.5 shrink-0">
                      <button
                        type="button"
                        className="p-1.5 text-slate-400 hover:bg-indigo-50 hover:text-indigo-600 rounded-md transition-colors cursor-pointer"
                        onClick={() => openEditAccount(account)}
                        disabled={rowBusy}
                        title="编辑"
                      >
                        <Edit3 size={14} />
                      </button>
                      <button
                        type="button"
                        className="p-1.5 text-slate-400 hover:bg-indigo-50 hover:text-indigo-600 rounded-md transition-colors cursor-pointer"
                        onClick={() => handleRefreshAccountProfile(account)}
                        disabled={rowBusy}
                        title="刷新账号信息"
                      >
                        <RefreshCw size={14} />
                      </button>
                      <button
                        type="button"
                        className="p-1.5 text-slate-400 hover:bg-indigo-50 hover:text-indigo-600 rounded-md transition-colors cursor-pointer"
                        onClick={() => handleTestAccount(account)}
                        disabled={rowBusy}
                        title="测试连通性"
                      >
                        <ShieldCheck size={14} />
                      </button>
                      <button
                        type="button"
                        className="p-1.5 text-slate-400 hover:bg-rose-50 hover:text-rose-500 rounded-md transition-colors cursor-pointer"
                        onClick={() => handleDeleteAccount(account)}
                        disabled={rowBusy}
                        title="删除"
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </div>

                  {/* 第二行：配额进度条 + 刷新时间 */}
                  <div className="mt-3 flex flex-col gap-3 md:ml-12 md:gap-4 lg:flex-row lg:items-center lg:gap-6">
                    {/* 5h 配额进度条 */}
                    <div className="flex min-w-0 w-full items-center gap-2 lg:max-w-[280px]">
                      <span className="text-[11px] text-slate-400 shrink-0 w-10">5h</span>
                      {isAPIKeyAccount ? (
                        <span className="text-[11px] text-slate-500">无限额</span>
                      ) : normalizedPlanType === 'free' ? (
                        <span className="text-[11px] text-slate-300">无额度</span>
                      ) : (
                        <>
                          <div className="flex-1 h-1.5 bg-slate-100 rounded-full overflow-hidden">
                            <div
                              className={`h-full rounded-full transition-all duration-500 ${toQuotaProgressClass(quota5hRemaining)}`}
                              style={{ width: `${Number.isFinite(quota5hRemaining) ? quota5hRemaining : 0}%` }}
                            />
                          </div>
                          <span className="text-[11px] font-medium text-slate-600 shrink-0 w-10 text-right" title={quota5hResetAt ? `重置 ${formatTimestamp(quota5hResetAt)}` : ''}>
                            {Number.isFinite(quota5hRemaining) ? `${quota5hRemaining.toFixed(0)}%` : '-'}
                          </span>
                        </>
                      )}
                    </div>

                    {/* 周配额进度条 */}
                    <div className="flex min-w-0 w-full items-center gap-2 lg:max-w-[280px]">
                      <span className="text-[11px] text-slate-400 shrink-0 w-10">d7</span>
                      {isAPIKeyAccount ? (
                        <span className="text-[11px] text-slate-500">无限额</span>
                      ) : (
                        <>
                          <div className="flex-1 h-1.5 bg-slate-100 rounded-full overflow-hidden">
                            <div
                              className={`h-full rounded-full transition-all duration-500 ${toQuotaProgressClass(quotaWeeklyRemaining)}`}
                              style={{ width: `${Number.isFinite(quotaWeeklyRemaining) ? quotaWeeklyRemaining : 0}%` }}
                            />
                          </div>
                          <span className="text-[11px] font-medium text-slate-600 shrink-0 w-10 text-right" title={quotaWeeklyResetAt ? `重置 ${formatTimestamp(quotaWeeklyResetAt)}` : ''}>
                            {Number.isFinite(quotaWeeklyRemaining) ? `${quotaWeeklyRemaining.toFixed(0)}%` : '-'}
                          </span>
                        </>
                      )}
                    </div>

                    {/* 弹性空间 */}
                    <div className="hidden flex-1 lg:block" />

                    {/* 最近刷新 */}
                    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-slate-400 lg:shrink-0">
                      <span>刷新 {toDisplayTime(refreshedAt)}</span>
                      {(account.last_success_at || account.lastSuccessAt) && (
                        <span className="text-emerald-500">
                          连通 {formatTimestamp(account.last_success_at || account.lastSuccessAt)}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>

      <AccountFormModal
        open={accountModalOpen}
        title={editingAccount ? '编辑账号' : '新增账号'}
        submitText={editingAccount ? '保存修改' : '创建账号'}
        onClose={() => {
          if (accountSubmitting) return;
          setAccountModalOpen(false);
          setEditingAccount(null);
          setAccountForm(EMPTY_ACCOUNT_FORM);
          setOauthSectionExpanded(true);
          resetOAuthWorkflow();
        }}
        onSubmit={submitAccountForm}
        submitting={accountSubmitting}
      >
        <section className="space-y-4">
          <h4 className="text-sm font-semibold uppercase tracking-wide text-slate-500">基本信息</h4>

          <FormField label="账号名称" required>
            <input
              type="text"
              value={accountForm.account_name}
              onChange={(event) => setAccountForm(prev => ({ ...prev, account_name: event.target.value }))}
              className="w-full rounded-lg border border-slate-200 px-3 py-2 text-sm focus:border-indigo-400 focus:outline-none focus:ring-2 focus:ring-indigo-200"
              placeholder="例如：openai-auth-main"
            />
          </FormField>

          <div className="space-y-2">
            <div className="text-xs font-medium text-slate-600">
              授权方式
              <span className="ml-1 text-rose-500">*</span>
            </div>
            <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
              {AUTH_METHOD_OPTIONS.map((option) => {
                const active = accountForm.auth_method === option.value;
                return (
                  <button
                    key={option.value}
                    type="button"
                    onClick={() => {
                      const switchingIntoChatGPT = accountForm.auth_method !== 'chatgpt_refresh_token' && option.value === 'chatgpt_refresh_token';
                      setAccountForm(prev => ({
                        ...prev,
                        auth_method: option.value,
                        provider_type: authMethodToProviderType(option.value)
                      }));
                      if (option.value !== 'chatgpt_refresh_token') {
                        setOauthSectionExpanded(false);
                        resetOAuthWorkflow();
                      } else if (!editingAccount || switchingIntoChatGPT) {
                        setOauthSectionExpanded(true);
                      }
                    }}
                    className={`rounded-lg border px-3 py-2 text-left transition-colors ${
                      active
                        ? 'border-emerald-300 bg-emerald-50'
                        : 'border-slate-200 hover:bg-slate-50'
                    }`}
                  >
                    <div className={`text-sm font-medium ${active ? 'text-emerald-700' : 'text-slate-700'}`}>{option.label}</div>
                    <div className="text-xs text-slate-500">{option.description}</div>
                  </button>
                );
              })}
            </div>
          </div>
        </section>

        <section className="space-y-4">
          <h4 className="text-sm font-semibold uppercase tracking-wide text-slate-500">路由配置</h4>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <FormField label="优先级">
              <div className="space-y-1.5">
                <input
                  type="number"
                  min="1"
                  value={accountForm.priority}
                  onChange={(event) => setAccountForm(prev => ({ ...prev, priority: event.target.value }))}
                  className="w-full rounded-lg border border-slate-200 px-3 py-2 text-sm focus:border-indigo-400 focus:outline-none focus:ring-2 focus:ring-indigo-200"
                />
                <div className="rounded-lg bg-slate-50 px-3 py-2 text-xs leading-5 text-slate-500">
                  priority 越小越优先；相同 priority 视为同一层。V0 只做手动主备切换，不会在同层内自动择优。
                </div>
              </div>
            </FormField>

            <FormField label="Base URL（可选）">
              <input
                type="url"
                value={accountForm.base_url}
                onChange={(event) => setAccountForm(prev => ({ ...prev, base_url: event.target.value }))}
                className="w-full rounded-lg border border-slate-200 px-3 py-2 text-sm focus:border-indigo-400 focus:outline-none focus:ring-2 focus:ring-indigo-200"
                placeholder={DEFAULT_BASE_URL}
              />
            </FormField>
          </div>

          <label className="inline-flex items-start gap-2 text-sm text-slate-700">
            <input
              type="checkbox"
              checked={accountForm.enabled}
              onChange={(event) => setAccountForm(prev => ({ ...prev, enabled: event.target.checked }))}
              className="mt-0.5 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
            />
            <span>{editingAccount ? '保持账号启用状态' : '创建后立即启用'}</span>
          </label>
        </section>

        <section className="space-y-4">
          <h4 className="text-sm font-semibold uppercase tracking-wide text-slate-500">认证信息</h4>

          <FormField label={accountForm.auth_method === 'chatgpt_refresh_token' ? 'ChatGPT Refresh Token (rt)' : 'API Key'} required>
            <textarea
              value={accountForm.credential_raw}
              onChange={(event) => setAccountForm(prev => ({ ...prev, credential_raw: event.target.value }))}
              className="min-h-[120px] w-full rounded-lg border border-slate-200 px-3 py-2 text-sm font-mono focus:border-indigo-400 focus:outline-none focus:ring-2 focus:ring-indigo-200"
              placeholder={
                accountForm.auth_method === 'chatgpt_refresh_token'
                  ? '粘贴 Refresh Token，例如：rt-xxxxxx'
                  : '例如: sk-xxxxxx'
              }
            />
          </FormField>

          {accountForm.auth_method === 'chatgpt_refresh_token' && (
            <div className="rounded-lg border border-emerald-200 bg-emerald-50/60 p-3 space-y-3">
              <button
                type="button"
                onClick={() => setOauthSectionExpanded(prev => !prev)}
                className="flex w-full items-start justify-between gap-3 text-left"
              >
                <div>
                  <div className="text-sm font-semibold text-emerald-800">
                    {editingAccount ? '重新授权 / 更新 RT（可选）' : 'OAuth 快速提取 RT'}
                  </div>
                  <div className="mt-1 text-xs text-emerald-700">
                    {editingAccount
                      ? '已有 RT 可直接在上方凭据框修改；仅在需要重新登录提取时再展开。'
                      : '不想手动复制 RT 时，可在这里完成登录并自动提取。'}
                  </div>
                </div>
                <span className="shrink-0 text-xs font-medium text-emerald-700">
                  {oauthSectionExpanded ? '收起' : '展开'}
                </span>
              </button>

              {oauthSectionExpanded && (
                <div className="space-y-3 border-t border-emerald-200/80 pt-3">
                  <div className="text-xs text-emerald-700">
                    1) 生成授权链接并完成登录 2) 复制浏览器最终回调 URL 3) 粘贴后自动提取 refresh token
                  </div>

                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      onClick={handleGenerateOAuthLink}
                      loading={oauthActionLoading}
                    >
                      生成授权链接
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      onClick={() => {
                        if (!oauthSession?.auth_url) {
                          showNotice('error', '请先生成授权链接');
                          return;
                        }
                        const opened = openExternalURL(oauthSession.auth_url);
                        if (!opened) {
                          showNotice('error', '打开授权页失败，请改用复制授权链接');
                        }
                      }}
                      disabled={!oauthSession?.auth_url || oauthActionLoading}
                    >
                      打开授权页
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      onClick={async () => {
                        if (!oauthSession?.auth_url) {
                          showNotice('error', '请先生成授权链接');
                          return;
                        }
                        try {
                          await navigator.clipboard.writeText(oauthSession.auth_url);
                          showNotice('success', '授权链接已复制');
                        } catch {
                          showNotice('error', '复制失败，请手动复制');
                        }
                      }}
                      disabled={!oauthSession?.auth_url || oauthActionLoading}
                    >
                      复制授权链接
                    </Button>
                  </div>

                  {oauthSession?.auth_url && (
                    <div className="space-y-1">
                      <div className="text-xs text-slate-600">授权链接</div>
                      <textarea
                        value={oauthSession.auth_url}
                        readOnly
                        rows={2}
                        className="w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-xs font-mono"
                      />
                    </div>
                  )}

                  <div className="space-y-1">
                    <div className="text-xs text-slate-600">回调 URL</div>
                    <textarea
                      value={oauthCallbackURL}
                      onChange={(event) => setOauthCallbackURL(event.target.value)}
                      rows={2}
                      className="w-full rounded-lg border border-slate-200 px-3 py-2 text-xs font-mono focus:border-indigo-400 focus:outline-none focus:ring-2 focus:ring-indigo-200"
                      placeholder="粘贴登录后浏览器地址栏中的完整回调 URL"
                    />
                  </div>

                  <Button
                    type="button"
                    size="sm"
                    onClick={handleExtractRTFromCallback}
                    loading={oauthActionLoading}
                    disabled={!oauthSession?.session_id || !oauthCallbackURL.trim()}
                  >
                    从回调 URL 自动提取 RT
                  </Button>
                </div>
              )}
            </div>
          )}
        </section>
      </AccountFormModal>

      <Modal
        open={Boolean(deleteTarget)}
        title="确认删除账号"
        submitText="确认删除"
        submitVariant="danger"
        onClose={() => {
          if (deleteTarget) {
            const deleteId = resolveAccountId(deleteTarget);
            if (busyKey === `account-delete-${deleteId}`) return;
          }
          setDeleteTarget(null);
        }}
        onSubmit={handleConfirmDeleteAccount}
        submitting={deleteTarget ? busyKey === `account-delete-${resolveAccountId(deleteTarget)}` : false}
      >
        <div className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700">
          此操作不可撤销。
        </div>
        <p className="text-sm text-slate-700">
          确认删除账号
          <span className="font-semibold mx-1">
            {`「${deleteTarget?.account_name || deleteTarget?.accountName || '-'}」`}
          </span>
          吗？
        </p>
      </Modal>
    </div>
  );
};

export default AccountPoolPage;
