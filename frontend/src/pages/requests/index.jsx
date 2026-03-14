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
  buildManualSwitchSuccessMessage,
  compareAccountsByManualPriority,
  resolveAccountId,
} from '@pages/account-pool/utils.js';
import {
  fetchRequests,
  fetchModels,
  fetchUsageStats,
  fetchEndpoints,
  fetchGroups,
  activateGroup,
  moveUpstreamAccountToTier,
  fetchUpstreamAccounts,
  fetchLatestAccountScheduleSnapshot
} from '@utils/api.js';
import { buildQueryParamsFromFilters, createInitialFilters, useFilters } from './hooks/useFilters.js';
import { useColumnConfig } from './hooks/useColumnConfig.js';
import { useTimeRange } from './hooks/useTimeRange.js';
import { useAutoRefresh } from './hooks/useAutoRefresh.js';
import { FiltersPanel, StatsOverview, RequestsTable, Toolbar, RequestDetailModal } from './components';
import { PAGINATION_CONFIG } from './utils/constants.js';
import { resolveDisplayedActiveAccount } from './utils/accountSwitcherState.js';
import { buildTimeRangeSelectionState } from './utils/timeRangeSelection.js';

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
  const [groups, setGroups] = useState([]); // v4.0: 端点列表（一个端点=一个组）
  const [activeGroup, setActiveGroup] = useState('');
  const [accounts, setAccounts] = useState([]);
  const [latestScheduleSnapshot, setLatestScheduleSnapshot] = useState({ hasSnapshot: false, has_snapshot: false, candidates: [] });
  const [accountSwitching, setAccountSwitching] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const loadRequestIdRef = useRef(0);

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

  // 从端点列表提取唯一渠道
  const channels = useMemo(() => {
    const channelSet = new Set();
    channelSet.add('account-pool');
    endpoints.forEach(ep => {
      const channel = ep.channel || ep.Channel;
      if (channel) channelSet.add(channel);
    });
    return Array.from(channelSet).sort();
  }, [endpoints]);

  const sortedAccounts = useMemo(() => {
    return [...accounts].sort(compareAccountsByManualPriority);
  }, [accounts]);

  const requestFilterEndpoints = useMemo(() => {
    const options = new Set();
    endpoints.forEach((endpoint) => {
      const endpointName = endpoint?.name || endpoint?.endpoint_name || endpoint?.Name || '';
      if (endpointName) {
        options.add(endpointName);
      }
    });
    accounts.forEach((account) => {
      const accountName = account?.account_name || account?.accountName || '';
      if (accountName) {
        options.add(accountName);
      }
    });
    return Array.from(options).sort((left, right) => left.localeCompare(right));
  }, [accounts, endpoints]);

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
    return resolveDisplayedActiveAccount({
      accounts: sortedAccounts,
      recentSelectedAccountId
    });
  }, [recentSelectedAccountId, sortedAccounts]);

  // ==================== 数据加载 ====================

  const loadData = useCallback(async (silent = false) => {
    const requestId = ++loadRequestIdRef.current;
    try {
      // 静默刷新时不改变 loading 状态，避免闪屏
      if (!silent) {
        setLoading(true);
      }
      setError(null);

      const requestsQueryParams = {
        ...appliedQueryParams,
        source_view: 'all'
      };

      // 为stats API添加默认时间范围（30天），避免无数据问题
      const statsParams = {
        ...requestsQueryParams,
        period: '30d'
      };

      // v4.0: 简化数据获取，移除 keysData
      const [requestsData, statsData, modelsData, endpointsData, groupsData, accountsData, latestSnapshotData] = await Promise.all([
        fetchRequests({
          ...requestsQueryParams,
          page: pagination.page,
          pageSize: pagination.pageSize
        }),
        fetchUsageStats(statsParams),
        fetchModels(),
        fetchEndpoints(),
        fetchGroups(),
        fetchUpstreamAccounts().catch((err) => {
          console.error('❌ 加载账号池账号失败:', err);
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

      if (loadRequestIdRef.current !== requestId) {
        return;
      }

      setRequests(requestsData.requests);
      setPagination(prev => ({
        ...prev,
        total: requestsData.total,
        totalPages: requestsData.totalPages
      }));

      // 解包stats数据：后端返回 {success: true, data: {...}}
      const statsDataUnpacked = statsData?.data || statsData;
      setStats(statsDataUnpacked);

      setModels(Array.isArray(modelsData) ? modelsData : []);

      const endpointsList = endpointsData.endpoints || endpointsData || [];
      setEndpoints(Array.isArray(endpointsList) ? endpointsList : []);

      // v4.0: 端点列表（一个端点=一个组）
      const groupsList = groupsData?.groups || [];
      setGroups(Array.isArray(groupsList) ? groupsList : []);

      setAccounts(Array.isArray(accountsData) ? accountsData : []);
      setLatestScheduleSnapshot(latestSnapshotData && typeof latestSnapshotData === 'object'
        ? latestSnapshotData
        : { hasSnapshot: false, has_snapshot: false, candidates: [] });

      // 从组数据中找到活跃端点
      const activeGroupObj = groupsList.find(g => g.is_active);
      if (activeGroupObj) {
        setActiveGroup(activeGroupObj.name);
      }
    } catch (err) {
      if (loadRequestIdRef.current !== requestId) {
        return;
      }
      setError(err.message);
    } finally {
      // 只有手动刷新才会改变 loading 状态
      if (!silent && loadRequestIdRef.current === requestId) {
        setLoading(false);
      }
    }
  }, [appliedQueryParams, pagination.page, pagination.pageSize]);

  // 自动刷新 Hook (必须在 loadData 定义之后)
  const autoRefresh = useAutoRefresh(loadData);

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
    const nextFilters = createInitialFilters();
    updateFilters(nextFilters);
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

  // 快捷时间选择（筛选面板内）
  const handleQuickTimeSelect = (_range) => {
    // 这里可以实现快捷时间选择的逻辑
    // 简化实现：直接更新到"今天"
    const todayRange = {
      startDate: filters.startDate,
      endDate: filters.endDate
    };
    updateFilters(todayRange);
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

  // 端点切换回调
  const handleGroupSwitch = async (endpointName) => {
    try {
      // 只有端点变化时才调用 API 激活
      if (endpointName !== activeGroup) {
        console.log('🔄 切换端点:', endpointName);
        await activateGroup(endpointName);
        setActiveGroup(endpointName);
      }

      // 切换后刷新数据
      await loadData(true);
    } catch (err) {
      console.error('❌ 切换失败:', err);
      throw err; // 让 ActiveGroupSwitcher 组件知道切换失败
    }
  };

  const handleAccountSwitch = useCallback(async (account) => {
    const accountId = resolveAccountId(account);
    const currentActiveAccountId = resolveAccountId(activeAccount);
    const accountName = account?.account_name || account?.accountName || '目标账号';

    if (accountId === undefined || accountId === null || accountId === '') {
      const err = new Error('账号缺少 ID，无法切换');
      showNotice('error', err.message);
      return;
    }

    if (String(accountId) === String(currentActiveAccountId ?? '')) {
      showNotice('info', `「${accountName}」已经是主组账号`);
      return;
    }

    setAccountSwitching(true);
    try {
      const result = await moveUpstreamAccountToTier(accountId, 'primary');

      if (result?.unsupported) {
        showNotice('info', result.message || '当前后端版本暂不支持手动切主');
        return;
      }

      if (result?.changed !== true) {
        showNotice('info', `「${accountName}」已经是主组账号`);
        return;
      }

      showNotice('success', buildManualSwitchSuccessMessage(accountName, 'primary', { includeRequestPath: true }));
      await loadData(true);
    } catch (err) {
      showNotice('error', err.message || '切换账号失败');
      return;
    } finally {
      setAccountSwitching(false);
    }
  }, [activeAccount, loadData, showNotice]);

  // ==================== 生命周期 ====================

  useEffect(() => {
    loadData();
  }, [loadData]);

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
            groups={groups}
            activeGroup={activeGroup}
            onGroupSwitch={handleGroupSwitch}
            accounts={accounts}
            activeAccount={activeAccount}
            recentSelectedAccountId={recentSelectedAccountId}
            onAccountSwitch={handleAccountSwitch}
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
            channels={channels}
            endpoints={requestFilterEndpoints}
            onQuickTimeSelect={handleQuickTimeSelect}
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
