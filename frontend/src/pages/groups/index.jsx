// ============================================
// Groups 页面 - 组管理（模块化版本）
// 2025-12-01
// ============================================

import { useState, useEffect, useCallback } from 'react';
import { RefreshCw, Layers } from 'lucide-react';
import { Button, LoadingSpinner, ErrorMessage } from '@components/ui';
import { fetchGroups, activateGroup, pauseGroup } from '@utils/api.js';
import { GroupCard, GroupTableRow, StatsOverview, ViewToggle } from './components';

// ============================================
// Groups 页面主组件
// ============================================

const GroupsPage = () => {
  // ==================== 状态管理 ====================
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [error, setError] = useState(null);
  const [viewMode, setViewMode] = useState('grid'); // 'grid' | 'list'

  // ==================== 数据加载 ====================
  const loadGroups = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await fetchGroups();
      setGroups(data.groups || []);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  // ==================== 事件处理 ====================

  // 激活组
  const handleActivate = async (groupName) => {
    try {
      setActionLoading(true);
      await activateGroup(groupName);
      await loadGroups();
    } catch (err) {
      console.error('激活组失败:', err);
    } finally {
      setActionLoading(false);
    }
  };

  // 暂停组
  const handlePause = async (groupName) => {
    try {
      setActionLoading(true);
      await pauseGroup(groupName);
      await loadGroups();
    } catch (err) {
      console.error('暂停组失败:', err);
    } finally {
      setActionLoading(false);
    }
  };

  // ==================== 生命周期 ====================
  useEffect(() => {
    loadGroups();
  }, [loadGroups]);

  // ==================== 错误处理 ====================
  if (error) {
    return (
      <ErrorMessage
        title="组数据加载失败"
        message={error}
        onRetry={loadGroups}
      />
    );
  }

  // ==================== 渲染 ====================
  return (
    <div className="space-y-8 animate-in fade-in slide-in-from-bottom-2 duration-500">
      {/* 页面头部 */}
      <div>
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
          {/* 页面标题 */}
          <div className="flex items-center gap-3">
            <div className="p-2 bg-slate-900 rounded-lg text-white shadow-lg shadow-gray-200">
              <Layers className="w-6 h-6" />
            </div>
            <div>
              <h1 className="text-2xl font-bold text-gray-900 tracking-tight">组管理</h1>
              <p className="text-sm text-gray-500">管理 API 端点组分流策略</p>
            </div>
          </div>

          {/* 工具栏 */}
          <div className="flex items-center gap-3">
            {/* 视图切换 */}
            <ViewToggle viewMode={viewMode} onViewModeChange={setViewMode} />

            {/* 刷新按钮 */}
            <Button icon={RefreshCw} onClick={loadGroups} loading={loading}>
              刷新
            </Button>
          </div>
        </div>

        {/* 统计概览 */}
        <StatsOverview groups={groups} />
      </div>

      {/* Loading State */}
      {loading && <LoadingSpinner text="加载组数据..." />}

      {/* 视图内容 */}
      {!loading && (
        <>
          {viewMode === 'grid' ? (
            /* Grid 视图 */
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 animate-in fade-in duration-300">
              {groups.map((group) => (
                <GroupCard
                  key={group.name}
                  group={group}
                  onActivate={handleActivate}
                  onPause={handlePause}
                  loading={actionLoading}
                />
              ))}

              {/* 添加新分组占位 */}
              <button className="border border-dashed border-gray-300 rounded-2xl p-6 flex flex-col items-center justify-center text-gray-400 hover:border-indigo-400 hover:text-indigo-600 hover:bg-indigo-50/30 transition-all min-h-[220px] group">
                <div className="p-4 bg-gray-50 rounded-full group-hover:bg-white group-hover:shadow-md transition-all mb-3">
                  <Layers className="w-6 h-6" />
                </div>
                <span className="font-medium">添加新分组</span>
              </button>
            </div>
          ) : (
            /* List 视图 */
            <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden animate-in fade-in duration-300">
              <table className="w-full text-left text-sm">
                <thead className="bg-gray-50 border-b border-gray-100 text-gray-500">
                  <tr>
                    <th className="px-6 py-4 font-medium text-xs uppercase tracking-wider w-1/4">
                      分组名称 & 优先级
                    </th>
                    <th className="px-6 py-4 font-medium text-xs uppercase tracking-wider w-1/6">
                      状态
                    </th>
                    <th className="px-6 py-4 font-medium text-xs uppercase tracking-wider w-1/4">
                      连通性指标
                    </th>
                    <th className="px-6 py-4 font-medium text-xs uppercase tracking-wider w-1/6">
                      最近可达概览
                    </th>
                    <th className="px-6 py-4 font-medium text-xs uppercase tracking-wider text-right">
                      操作
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {groups.map((group) => (
                    <GroupTableRow
                      key={group.name}
                      group={group}
                      onActivate={handleActivate}
                      onPause={handlePause}
                      loading={actionLoading}
                    />
                  ))}
                </tbody>
              </table>

              {/* 添加新分组按钮 */}
              <div className="border-t border-gray-100 p-2 bg-gray-50/50">
                <button className="w-full py-3 border border-dashed border-gray-300 rounded-lg text-sm text-gray-500 hover:text-indigo-600 hover:border-indigo-300 hover:bg-white transition-all flex items-center justify-center gap-2">
                  <Layers className="w-4 h-4" /> 添加新分组
                </button>
              </div>
            </div>
          )}

          {/* Empty State */}
          {groups.length === 0 && (
            <div className="text-center py-12 text-slate-500">
              暂无组数据
            </div>
          )}
        </>
      )}
    </div>
  );
};

export default GroupsPage;
