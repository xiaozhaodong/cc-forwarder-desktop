// ============================================
// Account Pool 页面 - 账号管理
// 2026-03-05
// ============================================

import { useMemo } from 'react';
import { BrowserOpenURL } from '@wailsjs/runtime/runtime';
import { ErrorMessage, LoadingSpinner } from '@components/ui';
import {
  AccountFormDialog,
  AccountListSection,
  DeleteAccountDialog,
  LatestScheduleSnapshotCard,
  NoticeToast,
  PageHeader,
  StatsCards
} from './components';
import {
  buildManualFailoverTierSummary
} from './utils.js';
import {
  useAccountPoolAccounts,
  useAccountPoolActions,
  useAccountPoolForm,
  useLatestScheduleSnapshot,
  useNotice
} from './hooks';

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
  const { notice, showNotice, closeNotice } = useNotice();
  const { accounts, loading, error, loadData } = useAccountPoolAccounts();
  const {
    latestScheduleSnapshot,
    snapshotUnsupported,
    loadLatestScheduleSnapshot
  } = useLatestScheduleSnapshot({ showNotice });

  const {
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
  } = useAccountPoolForm({ loadData, showNotice });

  const {
    busyKey,
    deleteTarget,
    closeDeleteDialog,
    handleConfirmDeleteAccount,
    handleDeleteAccount,
    handleMoveAccountToTier,
    handleRefreshAccountProfile,
    handleRefreshAll,
    handleTestAccount,
    handleToggleAccount
  } = useAccountPoolActions({
    accounts,
    loadData,
    loadLatestScheduleSnapshot,
    showNotice
  });

  const accountCount = accounts.length;
  const activeAccountCount = accounts.filter(item => item.enabled && item.state !== 'disabled_auth').length;
  const authFailedCount = accounts.filter(item => item.state === 'disabled_auth').length;
  const priorityTierSummary = useMemo(() => buildManualFailoverTierSummary(accounts), [accounts]);
  const priorityTierMetaMap = useMemo(
    () => new Map(priorityTierSummary.map(item => [item.priority, item])),
    [priorityTierSummary]
  );

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
      <PageHeader loading={busyKey === 'reload'} onRefresh={handleRefreshAll} />

      <NoticeToast notice={notice} onClose={closeNotice} />

      <StatsCards
        accountCount={accountCount}
        activeAccountCount={activeAccountCount}
        authFailedCount={authFailedCount}
      />

      <LatestScheduleSnapshotCard
        snapshot={latestScheduleSnapshot}
        snapshotUnsupported={snapshotUnsupported}
      />

      <AccountListSection
        accounts={accounts}
        accountCount={accountCount}
        busyKey={busyKey}
        priorityTierSummary={priorityTierSummary}
        priorityTierMetaMap={priorityTierMetaMap}
        onCreate={openCreateAccount}
        onEdit={openEditAccount}
        onRefreshProfile={handleRefreshAccountProfile}
        onTest={handleTestAccount}
        onDelete={handleDeleteAccount}
        onToggle={handleToggleAccount}
        onMoveTier={handleMoveAccountToTier}
      />

      <AccountFormDialog
        open={accountModalOpen}
        editingAccount={editingAccount}
        accountSubmitting={accountSubmitting}
        accountForm={accountForm}
        setAccountForm={setAccountForm}
        onClose={closeAccountModal}
        onSubmit={submitAccountForm}
        oauthSectionExpanded={oauthSectionExpanded}
        setOauthSectionExpanded={setOauthSectionExpanded}
        oauthActionLoading={oauthActionLoading}
        oauthSession={oauthSession}
        oauthCallbackURL={oauthCallbackURL}
        setOauthCallbackURL={setOauthCallbackURL}
        onGenerateOAuthLink={handleGenerateOAuthLink}
        onExtractRTFromCallback={handleExtractRTFromCallback}
        onResetOAuthWorkflow={resetOAuthWorkflow}
        showNotice={showNotice}
        openExternalURL={openExternalURL}
      />

      <DeleteAccountDialog
        deleteTarget={deleteTarget}
        busyKey={busyKey}
        onClose={closeDeleteDialog}
        onSubmit={handleConfirmDeleteAccount}
      />
    </div>
  );
};

export default AccountPoolPage;
