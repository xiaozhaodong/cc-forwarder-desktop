// ============================================
// Account Pool 页面 - 账号授权管理
// 2026-03-05
// ============================================

import { useCallback, useEffect, useState } from 'react';
import {
  AlertCircle,
  CheckCircle2,
  Edit3,
  Pause,
  Play,
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

const Badge = ({ text, className }) => (
  <span className={`inline-flex items-center px-2 py-0.5 text-xs rounded-full border ${className}`}>
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

const toDisplayTime = (value) => (value ? formatTimestamp(value) : '-');

const toAccountStateLabel = (state) => {
  const stateMap = {
    active: '可用',
    cooldown: '冷却中',
    disabled_auth: '鉴权失效',
    disabled: '已禁用'
  };
  return stateMap[state] || (state || '未知');
};

const rowActionClass = (danger = false) => (
  `inline-flex items-center px-2 py-1 text-xs rounded-md border transition-colors ${
    danger
      ? 'border-rose-200 text-rose-600 hover:bg-rose-50'
      : 'border-slate-200 text-slate-600 hover:bg-slate-50'
  }`
);

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

    if (result?.success !== false && !result?.unsupported) {
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

  const handleTestAccount = async (account) => {
    const accountId = resolveAccountId(account);
    if (accountId === undefined || accountId === null || accountId === '') {
      showNotice('error', '账号缺少 ID，无法测试');
      return;
    }

    await runRowAction(
      `account-test-${accountId}`,
      () => testUpstreamAccount(accountId),
      (result) => result?.message || `已触发账号「${account.account_name || account.accountName}」连通性测试`,
      { skipRefresh: true }
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
            <h1 className="text-2xl font-bold text-slate-900">账号授权</h1>
            <p className="text-sm text-slate-500">仅保留 ChatGPT 账号授权管理（RT，不包含订阅同步与 Sora）</p>
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

      {notice && (
        <div className={`
          flex items-center gap-2 px-4 py-3 rounded-lg text-sm border
          ${notice.type === 'success' ? 'bg-emerald-50 text-emerald-700 border-emerald-200' : ''}
          ${notice.type === 'error' ? 'bg-rose-50 text-rose-700 border-rose-200' : ''}
          ${notice.type === 'info' ? 'bg-slate-50 text-slate-600 border-slate-200' : ''}
        `}>
          {notice.type === 'success' ? <CheckCircle2 size={16} /> : <AlertCircle size={16} />}
          {notice.text}
        </div>
      )}

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
          <div className="overflow-x-auto">
            <table className="w-full min-w-[1120px]">
              <thead>
                <tr className="bg-slate-50 text-slate-500 text-xs uppercase tracking-wide">
                  <th className="px-4 py-3 text-left font-semibold">账号名</th>
                  <th className="px-4 py-3 text-left font-semibold">授权方式</th>
                  <th className="px-4 py-3 text-left font-semibold">优先级</th>
                  <th className="px-4 py-3 text-left font-semibold">启用</th>
                  <th className="px-4 py-3 text-left font-semibold">状态</th>
                  <th className="px-4 py-3 text-left font-semibold">最后成功</th>
                  <th className="px-4 py-3 text-left font-semibold">最后错误</th>
                  <th className="px-4 py-3 text-right font-semibold">操作</th>
                </tr>
              </thead>
              <tbody>
                {accounts.map((account) => {
                  const accountId = resolveAccountId(account) ?? account.account_name ?? account.accountName;
                  const accountName = account.account_name || account.accountName || '-';
                  const state = account.state || 'active';
                  const stateClass = ACCOUNT_STATE_STYLE[state] || 'bg-slate-100 text-slate-600 border-slate-200';
                  const rowBusy = busyKey.startsWith('account-') && busyKey.includes(String(accountId));

                  return (
                    <tr key={String(accountId)} className="border-t border-slate-100 hover:bg-slate-50/50">
                      <td className="px-4 py-3 font-medium text-slate-900">{accountName}</td>
                      <td className="px-4 py-3 text-sm text-slate-600">{toAccountAuthLabel(account.provider_type || account.providerType || '')}</td>
                      <td className="px-4 py-3 text-sm text-slate-600">{account.priority || 1}</td>
                      <td className="px-4 py-3">
                        <Badge
                          text={account.enabled ? '启用' : '停用'}
                          className={account.enabled ? 'bg-emerald-50 text-emerald-700 border-emerald-200' : 'bg-slate-100 text-slate-600 border-slate-200'}
                        />
                      </td>
                      <td className="px-4 py-3">
                        <Badge text={toAccountStateLabel(state)} className={stateClass} />
                      </td>
                      <td className="px-4 py-3 text-sm text-slate-600">{toDisplayTime(account.last_success_at || account.lastSuccessAt)}</td>
                      <td className="px-4 py-3 text-sm text-rose-600 max-w-[280px] truncate" title={account.last_error || account.lastError || ''}>
                        {account.last_error || account.lastError || '-'}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          <button
                            type="button"
                            className={rowActionClass()}
                            onClick={() => openEditAccount(account)}
                            disabled={rowBusy}
                          >
                            <Edit3 size={12} className="mr-1" /> 编辑
                          </button>
                          <button
                            type="button"
                            className={rowActionClass()}
                            onClick={() => handleToggleAccount(account)}
                            disabled={rowBusy}
                          >
                            {account.enabled ? <Pause size={12} className="mr-1" /> : <Play size={12} className="mr-1" />}
                            {account.enabled ? '停用' : '启用'}
                          </button>
                          <button
                            type="button"
                            className={rowActionClass()}
                            onClick={() => handleTestAccount(account)}
                            disabled={rowBusy}
                          >
                            <ShieldCheck size={12} className="mr-1" /> 测试连通性
                          </button>
                          <button
                            type="button"
                            className={rowActionClass(true)}
                            onClick={() => handleDeleteAccount(account)}
                            disabled={rowBusy}
                          >
                            <Trash2 size={12} className="mr-1" /> 删除
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <Modal
        open={accountModalOpen}
        title={editingAccount ? '编辑账号' : '新增账号'}
        submitText={editingAccount ? '保存修改' : '创建账号'}
        onClose={() => {
          if (accountSubmitting) return;
          setAccountModalOpen(false);
          setEditingAccount(null);
          setAccountForm(EMPTY_ACCOUNT_FORM);
          resetOAuthWorkflow();
        }}
        onSubmit={submitAccountForm}
        submitting={accountSubmitting}
      >
        <FormField label="账号名称" required>
          <input
            type="text"
            value={accountForm.account_name}
            onChange={(event) => setAccountForm(prev => ({ ...prev, account_name: event.target.value }))}
            className="w-full px-3 py-2 rounded-lg border border-slate-200 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400"
            placeholder="例如：openai-auth-main"
          />
        </FormField>

        <div className="space-y-2">
          <div className="text-xs font-medium text-slate-600">
            授权方式
            <span className="text-rose-500 ml-1">*</span>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
            {AUTH_METHOD_OPTIONS.map((option) => {
              const active = accountForm.auth_method === option.value;
              return (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => {
                    setAccountForm(prev => ({
                      ...prev,
                      auth_method: option.value,
                      provider_type: authMethodToProviderType(option.value)
                    }));
                    if (option.value !== 'chatgpt_refresh_token') {
                      resetOAuthWorkflow();
                    }
                  }}
                  className={`text-left px-3 py-2 rounded-lg border transition-colors ${
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

        {accountForm.auth_method === 'chatgpt_refresh_token' && (
          <div className="rounded-lg border border-emerald-200 bg-emerald-50/60 p-3 space-y-3">
            <div className="text-sm font-semibold text-emerald-800">OAuth 快速提取 RT</div>
            <div className="text-xs text-emerald-700">
              1) 先生成授权链接并完成登录 2) 复制浏览器最终回调 URL 3) 粘贴后自动提取 refresh token
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
                  window.open(oauthSession.auth_url, '_blank', 'noopener,noreferrer');
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
                  className="w-full px-3 py-2 rounded-lg border border-slate-200 text-xs font-mono bg-white"
                />
              </div>
            )}

            <div className="space-y-1">
              <div className="text-xs text-slate-600">回调 URL</div>
              <textarea
                value={oauthCallbackURL}
                onChange={(event) => setOauthCallbackURL(event.target.value)}
                rows={2}
                className="w-full px-3 py-2 rounded-lg border border-slate-200 text-xs font-mono focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400"
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

        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          <FormField label="优先级">
            <input
              type="number"
              min="1"
              value={accountForm.priority}
              onChange={(event) => setAccountForm(prev => ({ ...prev, priority: event.target.value }))}
              className="w-full px-3 py-2 rounded-lg border border-slate-200 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400"
            />
          </FormField>

          <FormField label="Base URL（可选）">
            <input
              type="url"
              value={accountForm.base_url}
              onChange={(event) => setAccountForm(prev => ({ ...prev, base_url: event.target.value }))}
              className="w-full px-3 py-2 rounded-lg border border-slate-200 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400"
              placeholder={DEFAULT_BASE_URL}
            />
          </FormField>
        </div>

        <FormField label={accountForm.auth_method === 'chatgpt_refresh_token' ? 'ChatGPT Refresh Token (rt)' : 'API Key'} required>
          <textarea
            value={accountForm.credential_raw}
            onChange={(event) => setAccountForm(prev => ({ ...prev, credential_raw: event.target.value }))}
            className="w-full px-3 py-2 rounded-lg border border-slate-200 text-sm font-mono min-h-[92px] focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400"
            placeholder={
              accountForm.auth_method === 'chatgpt_refresh_token'
                ? '粘贴 Refresh Token，例如：rt-xxxxxx'
                : '例如: sk-xxxxxx'
            }
          />
        </FormField>

        <label className="inline-flex items-center gap-2 text-sm text-slate-700">
          <input
            type="checkbox"
            checked={accountForm.enabled}
            onChange={(event) => setAccountForm(prev => ({ ...prev, enabled: event.target.checked }))}
            className="rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
          />
          创建后立即启用
        </label>
      </Modal>

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
