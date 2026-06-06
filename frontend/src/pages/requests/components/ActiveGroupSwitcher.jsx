// ============================================
// ActiveGroupSwitcher - 端点快捷切换器
// 2025-12-09 17:29:26 v5.0: 显示渠道名称，按优先级排序
// ============================================

import { useState, useEffect, useRef, useMemo } from 'react';
import { ArrowLeftRight, Server, Check, AlertCircle, LockKeyhole, RotateCcw, Trash2 } from 'lucide-react';

/**
 * ActiveGroupSwitcher - 端点快捷切换器
 * @param {Object} props
 * @param {Array} props.groups - 所有端点列表（v4.0: 一个端点=一个组）
 * @param {string} props.activeGroup - 当前活跃端点名称
 * @param {Function} props.onSwitch - 切换回调 (endpointName) => Promise<void>
 * @param {boolean} props.loading - 是否正在切换中
 */
const ActiveGroupSwitcher = ({
  groups = [],
  activeGroup = '',
  routingState = null,
  onSwitch,
  onRestoreAuto,
  onClearRouteCache,
  loading = false
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [switching, setSwitching] = useState(false);
  const containerRef = useRef(null);

  // 按优先级排序（数值越小优先级越高）- Hook 必须在条件返回之前
  const sortedGroups = useMemo(() => {
    return [...groups].sort((a, b) => (a.priority ?? 999) - (b.priority ?? 999));
  }, [groups]);

  const routeMode = routingState?.mode || 'auto';
  const requestedEndpoint = routingState?.endpointName || routingState?.endpoint_name || '';
  const isManualFixed = routeMode === 'manual_fixed';

  // 点击外部关闭
  useEffect(() => {
    const handleClickOutside = (event) => {
      if (containerRef.current && !containerRef.current.contains(event.target)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // ESC 键关闭
  useEffect(() => {
    const handleKeyDown = (e) => {
      if (e.key === 'Escape') {
        setIsOpen(false);
      }
    };
    if (isOpen) {
      window.addEventListener('keydown', handleKeyDown);
    }
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen]);

  // 处理端点选择
  const handleEndpointSelect = async (endpoint) => {
    if (switching || loading) return;

    if (
      endpoint.name === activeGroup &&
      routeMode === 'manual_preferred' &&
      requestedEndpoint === endpoint.name
    ) {
      setIsOpen(false);
      return;
    }

    setSwitching(true);
    try {
      await onSwitch?.(endpoint.name, 'manual_preferred');
      setIsOpen(false);
    } catch (error) {
      console.error('切换失败:', error);
      alert(`切换失败: ${error.message || '未知错误'}`);
    } finally {
      setSwitching(false);
    }
  };

  const handleFixedSelect = async (endpoint) => {
    if (switching || loading) return;

    setSwitching(true);
    try {
      await onSwitch?.(endpoint.name, 'manual_fixed');
      setIsOpen(false);
    } catch (error) {
      console.error('固定失败:', error);
      alert(`固定失败: ${error.message || '未知错误'}`);
    } finally {
      setSwitching(false);
    }
  };

  const handleRestoreAuto = async () => {
    if (switching || loading) return;
    setSwitching(true);
    try {
      await onRestoreAuto?.();
      setIsOpen(false);
    } catch (error) {
      console.error('恢复自动失败:', error);
      alert(`恢复自动失败: ${error.message || '未知错误'}`);
    } finally {
      setSwitching(false);
    }
  };

  const handleClearCache = async () => {
    if (switching || loading) return;
    setSwitching(true);
    try {
      await onClearRouteCache?.(requestedEndpoint || activeGroup || '');
      setIsOpen(false);
    } catch (error) {
      console.error('清理缓存失败:', error);
      alert(`清理缓存失败: ${error.message || '未知错误'}`);
    } finally {
      setSwitching(false);
    }
  };

  // 获取健康状态样式
  const getHealthStyle = (endpoint) => {
    const healthyCount = endpoint.healthy_endpoints ?? 0;
    const uncheckedCount = endpoint.unchecked_endpoints ?? 0;
    const checkedCount = Math.max((endpoint.total_endpoints ?? 0) - uncheckedCount, 0);

    if (checkedCount === 0) {
      return {
        dot: 'bg-slate-300',
        text: '未检测',
        color: 'text-slate-500'
      };
    }

    if (healthyCount > 0) {
      return {
        dot: 'bg-emerald-400',
        text: '最近可达',
        color: 'text-emerald-600'
      };
    }
    return {
      dot: 'bg-rose-400',
      text: '最近不可达',
      color: 'text-rose-600'
    };
  };

  // 无端点数据时的占位显示
  if (!groups.length) {
    return (
      <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm text-gray-400 bg-gray-50 border border-gray-200">
        <AlertCircle className="w-3.5 h-3.5" />
        <span>无可用端点</span>
      </div>
    );
  }

  // 查找当前活跃端点
  const activeEndpoint = sortedGroups.find(g => g.name === activeGroup) || sortedGroups[0];
  const activeHealth = getHealthStyle(activeEndpoint);
  const modeLabel = routeMode === 'manual_fixed' ? '固定' : routeMode === 'manual_preferred' ? '优选' : '自动';
  const modeClass = routeMode === 'manual_fixed'
    ? 'bg-rose-50 text-rose-600 border-rose-100'
    : routeMode === 'manual_preferred'
      ? 'bg-indigo-50 text-indigo-600 border-indigo-100'
      : 'bg-slate-50 text-slate-500 border-slate-100';

  return (
    <div className="relative" ref={containerRef}>
      {/* 触发按钮 */}
      <button
        onClick={() => setIsOpen(!isOpen)}
        disabled={switching || loading}
        className={`group flex items-center justify-between gap-2 w-[170px] lg:w-[190px] xl:w-[220px] px-3 py-1.5 bg-white border rounded-lg text-sm font-medium transition-all shadow-sm ${
          isOpen
            ? 'border-indigo-300 ring-2 ring-indigo-100 text-indigo-700'
            : 'border-gray-200 text-gray-700 hover:border-indigo-300 hover:text-indigo-600'
        } ${(switching || loading) ? 'opacity-60 cursor-wait' : ''}`}
      >
        {/* 活跃状态指示器 */}
        <span className="relative flex h-2 w-2">
          <span className={`animate-ping absolute inline-flex h-full w-full rounded-full ${activeHealth.dot} opacity-75`}></span>
          <span className={`relative inline-flex rounded-full h-2 w-2 ${activeHealth.dot}`}></span>
        </span>

        <div className="flex items-center gap-1.5 min-w-0 flex-1">
          <Server className="w-3.5 h-3.5 text-gray-400 shrink-0" />
          <span className="hidden xl:inline font-semibold text-xs text-gray-500 shrink-0">端点:</span>
          <span className="font-bold truncate">{activeGroup || '未选择'}</span>
        </div>

        <span className={`hidden lg:inline-flex px-1.5 py-0.5 rounded border text-[10px] font-semibold shrink-0 ${modeClass}`}>
          {modeLabel}
        </span>
        <ArrowLeftRight className={`w-3.5 h-3.5 ml-1 text-gray-400 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
      </button>

      {/* 下拉面板 */}
      {isOpen && (
        <div className="absolute top-full left-0 mt-2 w-[280px] bg-white rounded-xl shadow-xl border border-gray-100 ring-1 ring-black/5 z-50 overflow-hidden animate-in fade-in slide-in-from-top-2 duration-200">
          {/* 标题 */}
          <div className="px-4 py-3 bg-gray-50 border-b border-gray-100">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-xs font-semibold text-gray-500 uppercase tracking-wider">
                  选择端点
                </div>
                <div className="text-[10px] text-gray-400 mt-0.5">
                  共 {sortedGroups.length} 个端点
                </div>
              </div>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={handleRestoreAuto}
                  disabled={switching || loading || routeMode === 'auto'}
                  title="恢复自动路由"
                  className="p-1.5 rounded-md text-slate-400 hover:text-indigo-600 hover:bg-white border border-transparent hover:border-slate-200 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  <RotateCcw className="w-3.5 h-3.5" />
                </button>
                <button
                  type="button"
                  onClick={handleClearCache}
                  disabled={switching || loading}
                  title="清理路由缓存"
                  className="p-1.5 rounded-md text-slate-400 hover:text-rose-600 hover:bg-white border border-transparent hover:border-slate-200 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            </div>
          </div>

          {/* 端点列表 */}
          <div className="p-2 max-h-[320px] overflow-y-auto">
            {sortedGroups.map((endpoint) => {
              const isActive = endpoint.name === activeGroup;
              const health = getHealthStyle(endpoint);

              return (
                <div
                  key={endpoint.name}
                  className={`w-full flex items-center justify-between px-3 py-2.5 rounded-lg text-sm text-left transition-colors ${
                    isActive
                      ? 'bg-indigo-50 text-indigo-700'
                      : 'hover:bg-gray-50 text-gray-700'
                  } ${switching ? 'opacity-50 cursor-wait' : ''}`}
                >
                  <button
                    type="button"
                    onClick={() => handleEndpointSelect(endpoint)}
                    disabled={switching || loading}
                    className="flex min-w-0 flex-1 items-center gap-3 text-left disabled:cursor-wait"
                  >
                    {/* 连通性状态指示点 */}
                    <span className={`w-2 h-2 rounded-full ${health.dot}`} />

                    <div className="flex min-w-0 flex-col">
                      {/* 端点名称 + 渠道 */}
                      <div className="flex items-center gap-2">
                        <span className={`font-medium truncate ${isActive ? 'text-indigo-700' : 'text-gray-800'}`}>
                          {endpoint.name}
                        </span>
                        {endpoint.channel && (
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 text-slate-500">
                            {endpoint.channel}
                          </span>
                        )}
                      </div>
                      {/* 健康状态 */}
                      <span className={`text-[10px] ${health.color}`}>
                        {health.text}
                        {endpoint.in_cooldown && ' · 冷却中'}
                      </span>
                    </div>
                  </button>

                  {/* 选中状态 */}
                  <div className="flex items-center gap-2">
                    {endpoint.paused && (
                      <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-100 text-amber-600">
                        已暂停
                      </span>
                    )}
                    {isActive && <Check className="w-4 h-4 text-indigo-600" />}
                    <button
                      type="button"
                      onClick={() => handleFixedSelect(endpoint)}
                      disabled={switching || loading || (isManualFixed && requestedEndpoint === endpoint.name)}
                      title="严格固定到此端点"
                      className={`p-1 rounded-md border transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
                        isManualFixed && requestedEndpoint === endpoint.name
                          ? 'bg-rose-50 text-rose-600 border-rose-100'
                          : 'text-slate-400 border-transparent hover:bg-rose-50 hover:text-rose-600 hover:border-rose-100'
                      }`}
                    >
                      <LockKeyhole className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};

export default ActiveGroupSwitcher;
