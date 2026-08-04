// ============================================
// Requests 页面 - 请求追踪（重构版）
// 2025-12-06 10:40:38 v4.0: 简化端点切换（移除 Token 级联）
// ============================================

import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { BarChart3 } from 'lucide-react';
import { ErrorMessage } from '@components/ui';
import { NoticeToast } from '@pages/account-pool/components';
import { useNotice } from '@pages/account-pool/hooks';
import {
  compareAccountsByManualPriority,
  isValidAccountId,
  resolveAccountId,
} from '@pages/account-pool/utils.js';
import {
  fetchRequests,
  fetchModels,
  fetchUsageStats,
  fetchEndpoints,
  fetchClaudeRoutingState,
  setClaudeRoutingOverride,
  clearClaudeRoutingOverride,
  clearNegativeHitCache,
  enableAutomaticAccountSelection,
  pinUpstreamAccountSelection,
  fetchUpstreamAccounts,
  fetchLatestAccountScheduleSnapshot
} from '@utils/api.js';
import { subscribeToEvent, isWailsEnvironment } from '@utils/wailsApi.js';
import { buildQueryParamsFromFilters, useFilters } from './hooks/useFilters.js';
import { useColumnConfig } from './hooks/useColumnConfig.js';
import { useTimeRange } from './hooks/useTimeRange.js';
import { useAutoRefresh } from './hooks/useAutoRefresh.js';
import { FiltersPanel, StatsOverview, RequestsTable, Toolbar, RequestDetailModal } from './components';
import { PAGINATION_CONFIG } from './utils/constants.js';
import { isRuntimeActiveSelection, isSamePinnedAccount } from './utils/accountSwitcherState.js';
import { buildTimeRangeSelectionState } from './utils/timeRangeSelection.js';
import { filterUpstreamOptionsByFamily } from './utils/requestSource.js';

// ============================================
// Requests 页面
// ============================================

const RequestsPage = () => {
  // ==================== 状态管理 ====================
  const { notice, showNotice, closeNotice } = useNotice();

  // 数据状态
  const [requests, setRequests] = useState([]);
  const [stats, setStats] = useState(null);
  const [models, setModels] = useState([]);
  const [endpoints, setEndpoints] = useState([]);
  const [claudeRoutingState, setClaudeRoutingState] = useState({ mode: 'auto', endpointName: '', fallbackEnabled: true });
  const [accounts, setAccounts] = useState([]);
  const [latestScheduleSnapshot, setLatestScheduleSnapshot] = useState({ hasSnapshot: false, has_snapshot: false, candidates: [] });
  const [routeSwitching, setRouteSwitching] = useState(false);
  const [accountSwitching, setAccountSwitching] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const loadRequestIdRef = useRef(0);
  const loadDataIdRef = useRef(0);

  // 面板状态
  const [isFilterOpen, setIsFilterOpen] = useState(false);
  const [isViewConfigOpen, setIsViewConfigOpen] = useState(false);

  // 详情模态框状态
  const [selectedRequest, setSelectedRequest] = useState(null);
  const [isDetailModalOpen, setIsDetailModalOpen] = useState(false);

  // 筛选器 Hook
  const {
    filters,
    updateFilter,
    updateFilters,
    resetFilters,
    buildQueryParams
  } = useFilters();
  const [appliedQueryParams, setAppliedQueryParams] = useState(() => buildQueryParams());

  // 列配置 Hook
  const {
    visibleColumns,
    toggleColumn,
    resetColumns,
    allColumns: columnConfigs
  } = useColumnConfig();

  // 时间范围 Hook
  const { activeRange, selectRange } = useTimeRange((timeRange) => {
    const nextState = buildTimeRangeSelectionState(filters, timeRange);
    updateFilters(nextState.filters);
    setAppliedQueryParams(nextState.appliedQueryParams);
    setPagination(prev => ({ ...prev, page: 1 }));
  });

  // 分页状态
  const [pagination, setPagination] = useState({
    page: 1,
    pageSize: PAGINATION_CONFIG.DEFAULT_PAGE_SIZE,
    total: 0,
    totalPages: 1
  });

  const sortedAccounts = useMemo(() => {
    return [...accounts].sort(compareAccountsByManualPriority);
  }, [accounts]);

  const upstreamOptions = useMemo(() => filterUpstreamOptionsByFamily(requests, filters.requestFamily), [filters.requestFamily, requests]);

  const recentSelectedAccountId = useMemo(() => {
    const hasSnapshot = latestScheduleSnapshot?.hasSnapshot === true || latestScheduleSnapshot?.has_snapshot === true;
    if (!hasSnapshot) {
      return null;
    }

    const finalOutcome = String(
      latestScheduleSnapshot?.finalOutcome
      ?? latestScheduleSnapshot?.final_outcome
      ?? ''
    ).trim().toLowerCase();
    if (finalOutcome === 'transient_failure' || finalOutcome === 'auth_failed' || finalOutcome === 'no_schedulable_accounts') {
      return null;
    }

    return latestScheduleSnapshot?.selectedAccountId
      ?? latestScheduleSnapshot?.selected_account_id
      ?? null;
  }, [latestScheduleSnapshot]);

  const activeAccount = useMemo(() => {
    return sortedAccounts.find(isRuntimeActiveSelection) || null;
  }, [sortedAccounts]);

  // ==================== 数据加载 ====================

  const refreshRequestData = useCallback(async () => {
    const requestId = ++loadRequestIdRef.current;
    const requestsQueryParams = {
      ...appliedQueryParams,
      source_view: 'all'
    };
    const statsParams = {
      ...requestsQueryParams,
      period: '30d'
    };

    const [requestsData, statsData] = await Promise.all([
      fetchRequests({
        ...requestsQueryParams,
        page: pagination.page,
        pageSize: pagination.pageSize
      }),
      fetchUsageStats(statsParams)
    ]);

    if (loadRequestIdRef.current !== requestId) {
      return;
    }

    setRequests(requestsData.requests);
    setPagination(prev => ({
      ...prev,
      total: requestsData.total,
      totalPages: requestsData.totalPages
    }));
    setStats(statsData?.data || statsData);
  }, [appliedQueryParams, pagination.page, pagination.pageSize]);

  const loadData = useCallback(async (silent = false) => {
    const loadId = ++loadDataIdRef.current;
    try {
      // 静默刷新时不改变 loading 状态，避免闪屏
      if (!silent) {
        setLoading(true);
      }
      setError(null);

      // 请求明细与统计单独封装，实时事件只刷新这两组动态数据。
      const [, modelsData, endpointsData, routeStateData, accountsData, latestSnapshotData] = await Promise.all([
        refreshRequestData(),
        fetchModels(),
        fetchEndpoints(),
        fetchClaudeRoutingState().catch((err) => {
          console.error('❌ 加载 Claude 路由状态失败:', err);
          return { mode: 'auto', endpointName: '', fallbackEnabled: true };
        }),
        fetchUpstreamAccounts().catch((err) => {
          console.error('❌ 加载账号池账号失败:', err);
          showNotice('error', err?.message || '加载账号池账号失败，账号切换器暂不可用');
          return [];
        }),
        fetchLatestAccountScheduleSnapshot().catch((err) => {
          console.error('❌ 加载最近一次账号调度结果失败:', err);
          return {
            unsupported: true,
            hasSnapshot: false,
            has_snapshot: false,
            candidates: []
          };
        })
      ]);

      if (loadDataIdRef.current !== loadId) {
        return;
      }

      setModels(Array.isArray(modelsData) ? modelsData : []);

      const endpointsList = endpointsData.endpoints || endpointsData || [];
      setEndpoints(Array.isArray(endpointsList) ? endpointsList : []);

      setClaudeRoutingState(routeStateData && typeof routeStateData === 'object'
        ? routeStateData
        : { mode: 'auto', endpointName: '', fallbackEnabled: true });

      setAccounts(Array.isArray(accountsData) ? accountsData : []);
      setLatestScheduleSnapshot(latestSnapshotData && typeof latestSnapshotData === 'object'
        ? latestSnapshotData
        : { hasSnapshot: false, has_snapshot: false, candidates: [] });

    } catch (err) {
      if (loadDataIdRef.current !== loadId) {
        return;
      }
      setError(err.message);
    } finally {
      if (loadDataIdRef.current === loadId) {
        setLoading(false);
      }
    }
  }, [refreshRequestData, showNotice]);

  // 请求变化由 Wails 事件触发，只刷新明细与统计；事件不可用时低频降级。
  const autoRefresh = useAutoRefresh(refreshRequestData);

  // ==================== 事件处理 ====================

  // 筛选面板切换
  const handleFilterToggle = () => {
    setIsFilterOpen(!isFilterOpen);
    setIsViewConfigOpen(false); // 关闭列配置
  };

  // 列配置面板切换
  const handleViewConfigToggle = () => {
    setIsViewConfigOpen(!isViewConfigOpen);
    setIsFilterOpen(false); // 关闭筛选面板
  };

  // 应用筛选
  const handleApplyFilters = () => {
    setAppliedQueryParams(buildQueryParams());
    setPagination(prev => ({ ...prev, page: 1 }));
  };

  // 重置筛选
  const handleResetFilters = () => {
    const nextFilters = resetFilters();
    setAppliedQueryParams(buildQueryParamsFromFilters(nextFilters));
    setPagination(prev => ({ ...prev, page: 1 }));
  };

  // 页码变更
  const handlePageChange = (newPage) => {
    setPagination(prev => ({ ...prev, page: newPage }));
  };

  // 每页条数变更
  const handlePageSizeChange = (newPageSize) => {
    setPagination(prev => ({
      ...prev,
      pageSize: newPageSize,
      page: 1 // 重置到第一页
    }));
  };

  // 双击行打开详情
  const handleRowDoubleClick = (request) => {
    setSelectedRequest(request);
    setIsDetailModalOpen(true);
  };

  // 关闭详情模态框
  const handleCloseDetailModal = () => {
    setIsDetailModalOpen(false);
    setSelectedRequest(null);
  };

  // Claude 端点路由切换回调
  const handleClaudeEndpointSwitch = async (endpointName, mode = 'manual_preferred') => {
    try {
      setRouteSwitching(true);
      console.log('🔄 切换 Claude 路由:', endpointName, mode);
      const nextState = await setClaudeRoutingOverride({
        mode,
        endpointName,
        fallbackEnabled: mode !== 'manual_fixed'
      });

      if (!nextState?.unsupported) {
        setClaudeRoutingState(nextState);
        showNotice(
          'success',
          mode === 'manual_fixed'
            ? `已严格固定 Claude 端点「${endpointName}」`
            : `已优先使用 Claude 端点「${endpointName}」`
        );
      } else showNotice('info', nextState.message || '当前后端暂不支持保存 Claude 路由模式');

      // 切换后刷新数据
      await loadData(true);
    } catch (err) {
      console.error('❌ 切换失败:', err);
      throw err;
    } finally {
      setRouteSwitching(false);
    }
  };

  const handleRestoreClaudeAuto = useCallback(async () => {
    setRouteSwitching(true);
    try {
      const nextState = await clearClaudeRoutingOverride();
      if (nextState?.unsupported) {
        showNotice('info', nextState.message || '当前后端暂不支持恢复自动路由');
        return;
      }
      setClaudeRoutingState(nextState);
      showNotice('success', 'Claude 路由已恢复自动调度');
      await loadData(true);
    } catch (err) {
      showNotice('error', err.message || '恢复自动路由失败');
    } finally {
      setRouteSwitching(false);
    }
  }, [loadData, showNotice]);

  const handleClearRouteCache = useCallback(async (endpointName = '') => {
    setRouteSwitching(true);
    try {
      const result = await clearNegativeHitCache(endpointName);
      if (result?.unsupported) {
        showNotice('info', result.message || '当前后端暂不支持清理路由缓存');
        return;
      }
      showNotice('success', endpointName ? `已清理「${endpointName}」的路由缓存` : '已清理 Claude 路由缓存');
      await loadData(true);
    } catch (err) {
      showNotice('error', err.message || '清理路由缓存失败');
    } finally {
      setRouteSwitching(false);
    }
  }, [loadData, showNotice]);

  const handleAccountSwitch = useCallback(async (account) => {
    const accountId = resolveAccountId(account);
    const accountName = account?.account_name || account?.accountName || '目标账号';

    if (!isValidAccountId(accountId)) {
      const err = new Error('账号缺少 ID，无法切换');
      showNotice('error', err.message);
      return;
    }

    if (isSamePinnedAccount({ displayedActiveAccount: activeAccount, targetAccount: account })) {
      showNotice('info', `「${accountName}」已经固定为当前账号`);
      return;
    }

    setAccountSwitching(true);
    try {
      const result = await pinUpstreamAccountSelection(accountId);

      if (result?.unsupported) {
        showNotice('info', result.message || '当前后端版本暂不支持固定账号');
        return;
      }

      if (result?.changed !== true) {
        showNotice('info', `「${accountName}」已经固定为当前账号`);
        return;
      }

      showNotice('success', result?.message || `已固定使用「${accountName}」，除非该账号严格不可用才会自动切走`);
      await loadData(true);
    } catch (err) {
      showNotice('error', err.message || '固定账号失败');
      return;
    } finally {
      setAccountSwitching(false);
    }
  }, [activeAccount, loadData, showNotice]);

  const handleEnableAutoSelection = useCallback(async () => {
    if (activeAccount === null) {
      showNotice('info', '当前已按编排自动调度');
      return;
    }

    setAccountSwitching(true);
    try {
      const result = await enableAutomaticAccountSelection();

      if (result?.unsupported) {
        showNotice('info', result.message || '当前后端版本暂不支持启用编排');
        return;
      }

      if (result?.changed !== true) {
        showNotice('info', result?.message || '当前已按编排自动调度');
        return;
      }

      showNotice('success', result?.message || '已启用编排自动调度');
      await loadData(true);
    } catch (err) {
      showNotice('error', err.message || '启用编排失败');
    } finally {
      setAccountSwitching(false);
    }
  }, [activeAccount, loadData, showNotice]);

  // ==================== 生命周期 ====================

  useEffect(() => {
    loadData();
  }, [loadData]);

  // v8:订阅 Claude 路由事件,切换/请求生效后即时同步页头状态(不依赖下一次 loadData)
  useEffect(() => {
    if (!isWailsEnvironment()) return undefined;
    const unsubscribe = subscribeToEvent('claude-routing:update', (state) => {
      if (!state || typeof state !== 'object') return;
      setClaudeRoutingState(state);
    });
    return () => {
      if (typeof unsubscribe === 'function') {
        unsubscribe();
      }
    };
  }, []);

  // ==================== 渲染 ====================

  if (error) {
    return (
      <ErrorMessage
        title="请求数据加载失败"
        message={error}
        onRetry={loadData}
      />
    );
  }

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-2 duration-500 relative">
      <NoticeToast notice={notice} onClose={closeNotice} />

      {/* 页面标题 & 工具栏（单行） */}
      <div className="flex items-center gap-2 xl:gap-3 relative z-30">
        {/* 页面标题 */}
        <div className="flex items-center gap-2 shrink-0">
          <div className="p-1.5 bg-indigo-600 rounded-lg text-white shadow-lg shadow-indigo-200/50">
            <BarChart3 className="w-5 h-5" />
          </div>
          <h1 className="text-lg xl:text-xl font-bold text-gray-900 tracking-tight whitespace-nowrap">请求追踪</h1>
        </div>

        {/* 工具栏 */}
        <div className="flex-1 min-w-0 flex justify-end">
          <Toolbar
            activeTimeRange={activeRange}
            onTimeRangeChange={selectRange}
            isFilterOpen={isFilterOpen}
            onFilterToggle={handleFilterToggle}
            isViewConfigOpen={isViewConfigOpen}
            onViewConfigToggle={handleViewConfigToggle}
            onRefresh={loadData}
            columns={columnConfigs}
            visibleColumns={visibleColumns}
            onToggleColumn={toggleColumn}
            onResetColumns={resetColumns}
            autoRefresh={autoRefresh}
            endpoints={endpoints}
            claudeRoutingState={claudeRoutingState}
            onClaudeEndpointSwitch={handleClaudeEndpointSwitch}
            onRestoreClaudeAuto={handleRestoreClaudeAuto}
            onClearRouteCache={handleClearRouteCache}
            routeSwitching={routeSwitching}
            accounts={accounts}
            activeAccount={activeAccount}
            recentSelectedAccountId={recentSelectedAccountId}
            onAccountSwitch={handleAccountSwitch}
            onEnableAutoSelection={handleEnableAutoSelection}
            accountSwitching={accountSwitching}
          />
        </div>

        {/* 筛选面板（弹出式） */}
        <div className="absolute top-full left-0 right-0 z-10">
          <FiltersPanel
            isOpen={isFilterOpen}
            onClose={() => setIsFilterOpen(false)}
            filters={filters}
            updateFilter={updateFilter}
            onApply={handleApplyFilters}
            onReset={handleResetFilters}
            models={models}
            upstreams={upstreamOptions}
          />
        </div>
      </div>

      {/* 统计概览 - 面板打开时blur */}
      <StatsOverview
        stats={stats}
        total={pagination.total}
        isBlurred={isFilterOpen || isViewConfigOpen}
      />

      {/* 请求列表表格 - 面板打开时blur */}
      <div className={`transition-all duration-300 ${isFilterOpen || isViewConfigOpen ? 'opacity-40 pointer-events-none blur-[1px]' : ''}`}>
        <RequestsTable
          requests={requests}
          loading={loading}
          pagination={pagination}
          onPageChange={handlePageChange}
          onPageSizeChange={handlePageSizeChange}
          visibleColumns={visibleColumns}
          columnConfigs={columnConfigs}
          onRowDoubleClick={handleRowDoubleClick}
        />
      </div>

      {/* 请求详情模态框 */}
      <RequestDetailModal
        isOpen={isDetailModalOpen}
        onClose={handleCloseDetailModal}
        request={selectedRequest}
      />
    </div>
  );
};

export default RequestsPage;
