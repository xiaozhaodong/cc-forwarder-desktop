// ============================================
// Endpoints 页面 - 端点管理
// 2025-11-28 (Updated 2025-12-24 for card layout)
// ============================================

import React, { useState, useEffect, useCallback, useRef } from 'react';
import {
  Activity,
  RefreshCw,
  Database,
  Server,
  Info
} from 'lucide-react';
import {
  Button,
  LoadingSpinner,
  ErrorMessage
} from '@components/ui';
import useEndpointsData from '@hooks/useEndpointsData.js';
import {
  EndpointForm,
  ChannelCard,
  DeleteConfirmDialog,
  groupEndpointsByChannel
} from './components';
import {
  getEndpointStorageStatus,
  getEndpointRecords,
  createEndpointRecord,
  updateEndpointRecord,
  deleteEndpointRecord,
  toggleEndpointRecord,
  isWailsEnvironment,
  subscribeToEvent
} from '@utils/wailsApi.js';

// ============================================
// Endpoints 页面
// ============================================

const EndpointsPage = () => {
  // 使用端点数据 Hook
  const {
    endpoints,
    loading,
    error,
    stats,
    refresh,
    performBatchHealthCheckAll,
    activateEndpointGroup,
    sseConnectionStatus,
    lastUpdate
  } = useEndpointsData();

  // 存储模式状态
  const [storageStatus, setStorageStatus] = useState(null);
  const [storageEndpoints, setStorageEndpoints] = useState([]);

  // 批量连通性测试状态
  const [batchCheckLoading, setBatchCheckLoading] = useState(false);

  // 表单状态
  const [showForm, setShowForm] = useState(false);
  const [editingEndpoint, setEditingEndpoint] = useState(null);
  const [formLoading, setFormLoading] = useState(false);

  // 删除确认状态
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  // 加载存储状态
  const loadStorageStatus = useCallback(async () => {
    try {
      const status = await getEndpointStorageStatus();
      setStorageStatus(status);

      // 如果是 SQLite 模式，加载存储的端点
      if (status.storageType === 'sqlite' && status.enabled) {
        const records = await getEndpointRecords();
        setStorageEndpoints(records);
      }
    } catch (err) {
      console.error('获取存储状态失败:', err);
      // 默认使用 YAML 模式
      setStorageStatus({ enabled: false, storageType: 'yaml' });
    }
  }, []);

  // 初始化加载存储状态
  useEffect(() => {
    loadStorageStatus();
  }, [loadStorageStatus]);

  // SQLite 模式下监听 Wails 事件，实时刷新端点数据
  const isSqliteModeRef = useRef(false);
  useEffect(() => {
    isSqliteModeRef.current = storageStatus?.storageType === 'sqlite' && storageStatus?.enabled;
  }, [storageStatus]);

  useEffect(() => {
    if (!isWailsEnvironment()) return;

    // 订阅端点更新事件
    const unsubscribe = subscribeToEvent('endpoint:update', () => {
      // 只在 SQLite 模式下刷新数据
      if (isSqliteModeRef.current) {
        console.log('📡 [Endpoints] 收到端点更新事件，刷新 SQLite 数据');
        loadStorageStatus();
      }
    });

    return () => {
      if (typeof unsubscribe === 'function') {
        unsubscribe();
      }
    };
  }, [loadStorageStatus]);

  // 批量连通性测试处理
  const handleBatchHealthCheck = async () => {
    setBatchCheckLoading(true);
    try {
      await performBatchHealthCheckAll();
      // 刷新数据以获取最新的连通性状态、响应时间等
      if (isSqliteMode) {
        await loadStorageStatus();
      }
    } catch (err) {
      console.error('批量连通性测试失败:', err);
      alert(`批量连通性测试失败: ${err.message}`);
    } finally {
      setBatchCheckLoading(false);
    }
  };

  // 判断存储模式
  const isSqliteMode = storageStatus?.storageType === 'sqlite' && storageStatus?.enabled;

  // 获取要显示的端点列表
  const displayEndpoints = isSqliteMode ? storageEndpoints : endpoints;

  // 计算统计数据
  const displayStats = isSqliteMode
    ? {
        total: storageEndpoints.length,
        healthy: storageEndpoints.filter(e => e.healthy).length,
        unhealthy: storageEndpoints.filter(e => !e.healthy && e.never_checked !== true && e.neverChecked !== true && (e.lastCheck || e.last_check)).length,
        unchecked: storageEndpoints.filter(e => e.never_checked === true || e.neverChecked === true || !(e.lastCheck || e.last_check)).length,
        cooldown: storageEndpoints.filter(e => e.in_cooldown || e.inCooldown).length,
        healthPercentage: storageEndpoints.filter(e => e.never_checked !== true && e.neverChecked !== true && (e.lastCheck || e.last_check)).length > 0
          ? ((storageEndpoints.filter(e => e.healthy).length / storageEndpoints.filter(e => e.never_checked !== true && e.neverChecked !== true && (e.lastCheck || e.last_check)).length) * 100).toFixed(1)
          : 0
      }
    : { ...stats, cooldown: 0 };

  // 按渠道分组
  const groupedEndpoints = groupEndpointsByChannel(displayEndpoints);

  // ============================================
  // CRUD 操作处理
  // ============================================

  // 新建端点
  const handleCreate = () => {
    setEditingEndpoint(null);
    setShowForm(true);
  };

  // 编辑端点
  const handleEdit = (endpoint) => {
    setEditingEndpoint(endpoint);
    setShowForm(true);
  };

  // 删除端点
  const handleDelete = (endpoint) => {
    setDeleteTarget(endpoint);
  };

  // 切换端点启用状态
  const handleToggle = async (name, enabled) => {
    try {
      await toggleEndpointRecord(name, enabled);
      // 刷新列表
      await loadStorageStatus();
    } catch (err) {
      console.error('切换端点状态失败:', err);
      alert(`操作失败: ${err.message}`);
    }
  };

  // 保存端点
  const handleSave = async (formData) => {
    setFormLoading(true);
    try {
      if (editingEndpoint) {
        // 编辑模式
        await updateEndpointRecord(editingEndpoint.name, formData);
      } else {
        // 新建模式
        await createEndpointRecord(formData);
      }
      setShowForm(false);
      setEditingEndpoint(null);
      // 刷新列表
      await loadStorageStatus();
    } catch (err) {
      console.error('保存失败:', err);
      throw err;
    } finally {
      setFormLoading(false);
    }
  };

  // 确认删除
  const handleConfirmDelete = async () => {
    if (!deleteTarget) return;

    setDeleteLoading(true);
    try {
      await deleteEndpointRecord(deleteTarget.name);
      setDeleteTarget(null);
      // 刷新列表
      await loadStorageStatus();
    } catch (err) {
      console.error('删除失败:', err);
      alert(`删除失败: ${err.message}`);
    } finally {
      setDeleteLoading(false);
    }
  };

  // 错误状态
  if (error && !isSqliteMode) {
    return (
      <ErrorMessage
        title="端点数据加载失败"
        message={error}
        onRetry={refresh}
      />
    );
  }

  // 加载状态
  if (loading && displayEndpoints.length === 0 && !storageStatus) {
    return <LoadingSpinner text="加载端点数据..." />;
  }

  return (
    <div className="animate-fade-in">
      {/* 页面标题 */}
      <div className="flex justify-between items-end mb-8">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Endpoints Management</h1>
          <p className="text-slate-500 text-sm mt-1">
            管理上游 API 端点配置、认证与路由策略
            {lastUpdate && (
              <span className="ml-2 text-slate-400">· 更新于 {lastUpdate}</span>
            )}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {/* SSE 状态指示器 */}
          <div className="flex items-center gap-1.5 text-xs text-slate-500">
            <span className={`w-2 h-2 rounded-full ${
              sseConnectionStatus === 'connected' ? 'bg-emerald-400' :
              sseConnectionStatus === 'connecting' ? 'bg-amber-400 animate-pulse' :
              'bg-slate-300'
            }`} />
            {sseConnectionStatus === 'connected' ? '实时' : '离线'}
          </div>

          {/* 刷新按钮 */}
          <Button
            variant="ghost"
            size="sm"
            icon={RefreshCw}
            onClick={isSqliteMode ? loadStorageStatus : refresh}
            loading={loading}
          >
            刷新
          </Button>

          {/* 批量检测按钮 */}
          <Button
            icon={Activity}
            loading={batchCheckLoading}
            onClick={handleBatchHealthCheck}
          >
            测试连通性
          </Button>

          {/* 新建端点按钮 (SQLite 模式) */}
          {isSqliteMode && (
            <Button
              icon={Server}
              onClick={handleCreate}
            >
              添加端点
            </Button>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2 mb-6 px-3 py-2 bg-slate-50/80 rounded-lg border border-slate-100 text-xs text-slate-500">
        <Info className="w-3.5 h-3.5 flex-shrink-0 text-slate-400" />
        <span>
          提示：列表中的“可达/不可达”仅代表最近一次连通性测试结果。实际路由调度主要依据真实请求失败追踪与冷却状态，而不是后台轮询探测。
        </span>
      </div>

      {/* 统计卡片 */}
      <div className="grid grid-cols-5 gap-4 mb-6">
        <div className="bg-white rounded-xl border border-slate-200/60 p-4 shadow-sm">
          <div className="text-2xl font-bold text-slate-900">{displayStats.total}</div>
          <div className="text-sm text-slate-500">总端点数</div>
        </div>
        {isSqliteMode && (
          <div className="bg-white rounded-xl border border-indigo-200/60 p-4 shadow-sm">
            <div className="text-2xl font-bold text-indigo-600">
              {storageEndpoints.filter(e => e.enabled).length}
            </div>
            <div className="text-sm text-slate-500">
              当前激活
              {storageEndpoints.find(e => e.enabled) && (
                <div className="text-xs text-indigo-500 mt-1 truncate">
                  {storageEndpoints.find(e => e.enabled).name}
                </div>
              )}
            </div>
          </div>
        )}
        <div className="bg-white rounded-xl border border-emerald-200/60 p-4 shadow-sm">
          <div className="text-2xl font-bold text-emerald-600">{displayStats.healthy}</div>
          <div className="text-sm text-slate-500">最近可达</div>
        </div>
        <div className="bg-white rounded-xl border border-rose-200/60 p-4 shadow-sm">
          <div className="text-2xl font-bold text-rose-600">{displayStats.unhealthy}</div>
          <div className="text-sm text-slate-500">最近不可达</div>
        </div>
        {/* 冷却中端点卡片 - 仅在有冷却端点时显示 */}
        {displayStats.cooldown > 0 && (
          <div className="bg-white rounded-xl border border-amber-200/60 p-4 shadow-sm">
            <div className="text-2xl font-bold text-amber-600">{displayStats.cooldown}</div>
            <div className="text-sm text-slate-500">冷却中</div>
          </div>
        )}
        <div className="bg-white rounded-xl border border-slate-200/60 p-4 shadow-sm">
          <div className="text-2xl font-bold text-slate-400">{displayStats.unchecked}</div>
          <div className="text-sm text-slate-500">未检测</div>
        </div>
      </div>

      {/* 渠道卡片网格 - 2 列布局 */}
      {displayEndpoints.length === 0 ? (
        <div className="bg-white rounded-2xl border border-slate-200/60 shadow-sm p-12 text-center">
          <div className="flex flex-col items-center gap-3">
            <Database size={40} className="text-slate-300" />
            <p className="text-slate-500">暂无端点配置</p>
            {isSqliteMode && (
              <Button icon={Server} onClick={handleCreate}>
                添加第一个端点
              </Button>
            )}
          </div>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-4">
            {groupedEndpoints.map(({ channel, endpoints: channelEndpoints }) => (
              <ChannelCard
                key={channel}
                channel={channel}
                endpoints={channelEndpoints}
                storageMode={isSqliteMode ? 'sqlite' : 'yaml'}
                onActivateGroup={activateEndpointGroup}
                onEdit={handleEdit}
                onDelete={handleDelete}
                onToggle={handleToggle}
              />
            ))}
          </div>

          {/* 底部统计 */}
          <div className="text-xs text-slate-500 text-center py-2">
            共 {groupedEndpoints.length} 个渠道，{displayEndpoints.length} 个端点
            {displayStats.healthy + displayStats.unhealthy > 0 && (
              <span className="ml-2 text-indigo-600">
                · {displayStats.healthPercentage}% 最近可达率
              </span>
            )}
          </div>
        </>
      )}

      {/* 端点表单弹窗 */}
      {showForm && (
        <EndpointForm
          endpoint={editingEndpoint}
          onSave={handleSave}
          onCancel={() => {
            setShowForm(false);
            setEditingEndpoint(null);
          }}
          loading={formLoading}
        />
      )}

      {/* 删除确认弹窗 */}
      {deleteTarget && (
        <DeleteConfirmDialog
          endpoint={deleteTarget}
          onConfirm={handleConfirmDelete}
          onCancel={() => setDeleteTarget(null)}
          loading={deleteLoading}
        />
      )}
    </div>
  );
};

export default EndpointsPage;
