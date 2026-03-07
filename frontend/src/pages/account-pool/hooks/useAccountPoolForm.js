// ============================================
// Account Pool 账号表单与 OAuth Hook
// 2026-03-07
// ============================================

import { useCallback, useState } from 'react';
import {
  createUpstreamAccount,
  exchangeChatGPTOAuthCallback,
  generateChatGPTOAuthLink,
  updateUpstreamAccount
} from '@utils/api.js';
import {
  DEFAULT_BASE_URL,
  EMPTY_ACCOUNT_FORM,
  authMethodToProviderType,
  buildOAuthCredentialRaw,
  maskSessionId,
  providerTypeToAuthMethod,
  resolveAccountId,
  summarizeCallbackURL
} from '../utils.js';

const useAccountPoolForm = ({ loadData, showNotice }) => {
  const [accountModalOpen, setAccountModalOpen] = useState(false);
  const [accountSubmitting, setAccountSubmitting] = useState(false);
  const [editingAccount, setEditingAccount] = useState(null);
  const [accountForm, setAccountForm] = useState(EMPTY_ACCOUNT_FORM);
  const [oauthActionLoading, setOauthActionLoading] = useState(false);
  const [oauthSectionExpanded, setOauthSectionExpanded] = useState(true);
  const [oauthSession, setOauthSession] = useState(null);
  const [oauthCallbackURL, setOauthCallbackURL] = useState('');

  const resetOAuthWorkflow = useCallback(() => {
    setOauthSession(null);
    setOauthCallbackURL('');
  }, []);

  const handleGenerateOAuthLink = useCallback(async () => {
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
  }, [showNotice]);

  const handleExtractRTFromCallback = useCallback(async () => {
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
  }, [oauthCallbackURL, oauthSession, showNotice]);

  const closeAccountModal = useCallback(() => {
    if (accountSubmitting) return;
    setAccountModalOpen(false);
    setEditingAccount(null);
    setAccountForm(EMPTY_ACCOUNT_FORM);
    setOauthSectionExpanded(true);
    resetOAuthWorkflow();
  }, [accountSubmitting, resetOAuthWorkflow]);

  const openCreateAccount = useCallback(() => {
    setEditingAccount(null);
    setAccountForm(EMPTY_ACCOUNT_FORM);
    setOauthSectionExpanded(true);
    resetOAuthWorkflow();
    setAccountModalOpen(true);
  }, [resetOAuthWorkflow]);

  const openEditAccount = useCallback((account) => {
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
  }, [resetOAuthWorkflow]);

  const submitAccountForm = useCallback(async (event) => {
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
  }, [accountForm, editingAccount, loadData, resetOAuthWorkflow, showNotice]);

  return {
    accountModalOpen,
    accountSubmitting,
    editingAccount,
    accountForm,
    setAccountForm,
    oauthActionLoading,
    oauthSectionExpanded,
    setOauthSectionExpanded,
    oauthSession,
    oauthCallbackURL,
    setOauthCallbackURL,
    resetOAuthWorkflow,
    handleGenerateOAuthLink,
    handleExtractRTFromCallback,
    closeAccountModal,
    openCreateAccount,
    openEditAccount,
    submitAccountForm
  };
};

export default useAccountPoolForm;
