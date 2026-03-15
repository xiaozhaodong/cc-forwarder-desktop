// ============================================
// GroupCard - Grid 视图组卡片组件
// 2025-12-01
// ============================================

import { Play, Pause, Clock, Activity, Server, MoreVertical } from 'lucide-react';

const GroupCard = ({ group, onActivate, onPause, loading }) => {
  const isActive = group.is_active;
  const isCooldown = group.in_cooldown;
  const uncheckedCount = group.unchecked_endpoints || 0;
  const checkedCount = Math.max((group.total_endpoints || 0) - uncheckedCount, 0);
  const healthyPercent = checkedCount > 0
    ? Math.round((group.healthy_endpoints / checkedCount) * 100)
    : 0;

  return (
    <div
      className={`relative rounded-2xl overflow-hidden transition-all duration-300 flex flex-col justify-between ${
        isActive
          ? 'bg-slate-900 text-white shadow-xl shadow-slate-200 ring-4 ring-white border-none scale-[1.02]'
          : 'bg-white text-gray-900 border border-gray-200 shadow-sm hover:shadow-md hover:border-gray-300'
      }`}
    >
      <div className="p-6">
        {/* 头部 */}
        <div className="flex justify-between items-start mb-6">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <h3 className="font-bold text-lg tracking-tight">{group.name}</h3>
              {isActive && (
                <span className="w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.6)] animate-pulse" />
              )}
            </div>
            <div className={`text-xs font-medium px-2 py-0.5 rounded-md inline-block ${
              isActive ? 'bg-slate-800 text-slate-300' : 'bg-gray-100 text-gray-500'
            }`}>
              优先级 {group.priority}
            </div>
          </div>
          <button
            className={`p-1.5 rounded-lg transition-colors ${
              isActive ? 'text-slate-400 hover:bg-slate-800 hover:text-white' : 'text-gray-300 hover:bg-gray-100 hover:text-gray-600'
            }`}
          >
            <MoreVertical className="w-5 h-5" />
          </button>
        </div>

        {/* 统计数据 */}
        <div className="grid grid-cols-2 gap-6 mb-2">
          <div>
            <div className={`text-xs mb-1 ${isActive ? 'text-slate-400' : 'text-gray-400'}`}>最近可达率</div>
            <div className="flex items-center gap-2">
              <Activity className={`w-4 h-4 ${
                checkedCount === 0
                  ? (isActive ? 'text-slate-500' : 'text-slate-400')
                  : healthyPercent === 100
                    ? (isActive ? 'text-emerald-400' : 'text-emerald-500')
                    : 'text-rose-500'
              }`} />
              <span className="font-bold text-xl">{checkedCount > 0 ? `${healthyPercent}%` : '未检测'}</span>
            </div>
          </div>
          <div>
            <div className={`text-xs mb-1 ${isActive ? 'text-slate-400' : 'text-gray-400'}`}>最近可达</div>
            <div className="flex items-center gap-2">
              <Server className={`w-4 h-4 ${isActive ? 'text-slate-500' : 'text-gray-400'}`} />
              {checkedCount > 0 ? (
                <span className="font-bold text-xl">
                  {group.healthy_endpoints}
                  <span className={`text-sm ml-1 font-normal ${isActive ? 'text-slate-500' : 'text-gray-400'}`}>
                    / {checkedCount}
                  </span>
                </span>
              ) : (
                <span className={`font-bold text-xl ${isActive ? 'text-slate-300' : 'text-gray-500'}`}>未检测</span>
              )}
            </div>
          </div>
        </div>

        {/* 连通性进度条 */}
        <div className={`h-1 w-full rounded-full overflow-hidden mt-4 ${isActive ? 'bg-slate-800' : 'bg-gray-100'}`}>
          <div
            className={`h-full rounded-full ${
              checkedCount === 0 ? 'bg-transparent' :
              healthyPercent === 100 ? (isActive ? 'bg-emerald-500' : 'bg-emerald-500') :
              healthyPercent === 0 ? 'bg-transparent' : 'bg-amber-500'
            }`}
            style={{ width: `${healthyPercent}%` }}
          />
        </div>

        {/* 冷却信息 */}
        {isCooldown && (
          <div className="mt-4 bg-amber-500/10 border border-amber-500/20 rounded-lg p-3">
            <div className="flex items-center text-amber-600 text-xs">
              <Clock size={12} className="mr-1.5" />
              剩余冷却: {group.cooldown_remaining || '计算中...'}
            </div>
          </div>
        )}
      </div>

      {/* 底部操作按钮 */}
      <div className={`px-6 py-4 border-t ${isActive ? 'border-slate-800 bg-slate-800/50' : 'border-gray-50 bg-gray-50/50'}`}>
        {isActive ? (
          <button
            onClick={() => onPause(group.name)}
            disabled={loading}
            className="w-full flex items-center justify-center gap-2 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-white font-medium text-sm transition-all border border-slate-700 disabled:opacity-50"
          >
            <Pause className="w-4 h-4" /> 暂停运行
          </button>
        ) : (
          <button
            onClick={() => onActivate(group.name)}
            disabled={loading || isCooldown}
            className="w-full flex items-center justify-center gap-2 py-2 rounded-lg bg-white border border-gray-200 text-gray-600 font-medium text-sm hover:bg-gray-50 hover:text-indigo-600 hover:border-indigo-200 transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Play className="w-4 h-4" /> 切换至此组
          </button>
        )}
      </div>
    </div>
  );
};

export default GroupCard;
