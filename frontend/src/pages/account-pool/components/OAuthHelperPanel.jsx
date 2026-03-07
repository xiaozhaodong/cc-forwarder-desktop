// ============================================
// Account Pool OAuth 辅助面板
// 2026-03-07
// ============================================

import { Button } from '@components/ui';

const OAuthHelperPanel = ({
  editingAccount,
  oauthSectionExpanded,
  setOauthSectionExpanded,
  oauthActionLoading,
  oauthSession,
  oauthCallbackURL,
  setOauthCallbackURL,
  onGenerateOAuthLink,
  onExtractRTFromCallback,
  showNotice,
  openExternalURL
}) => (
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
            onClick={onGenerateOAuthLink}
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
          onClick={onExtractRTFromCallback}
          loading={oauthActionLoading}
          disabled={!oauthSession?.session_id || !oauthCallbackURL.trim()}
        >
          从回调 URL 自动提取 RT
        </Button>
      </div>
    )}
  </div>
);

export default OAuthHelperPanel;
