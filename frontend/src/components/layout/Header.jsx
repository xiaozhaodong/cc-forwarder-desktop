// ============================================
// 布局组件 - Header 导航栏
// 2025-11-28 (Updated 2025-12-12 for proxy status display)
// ============================================

import { Command } from 'lucide-react';
import { isWailsEnvironment } from '@utils/wailsApi.js';

// 导航标签项
const NavItem = ({ label, active, onClick }) => (
  <button
    onClick={onClick}
    className={`shrink-0 whitespace-nowrap px-3 xl:px-4 py-2 text-sm font-medium rounded-full transition-all duration-200 ${
      active
        ? 'bg-slate-900 text-white shadow-md'
        : 'text-slate-500 hover:text-slate-900 hover:bg-slate-100'
    }`}
  >
    {label}
  </button>
);

const Header = ({ activeTab, onTabChange, connectionStatus = 'connected', proxyStatus = {} }) => {
  const tabs = [
    { name: 'overview', label: '概览' },
    { name: 'endpoints', label: 'CC 端点管理' },
    { name: 'account-pool', label: 'Codex 账号池' },
    // v4.0: 组管理入口已移除，配置简化后不再需要独立的组管理页面
    // { name: 'groups', label: '组管理' },
    { name: 'requests', label: '请求追踪' },
    // v6.1: 隐私保护
    { name: 'privacy-protection', label: '隐私保护' },
    { name: 'pricing', label: '基础定价' },
    // v5.1: 新增日志页面
    { name: 'logs', label: '系统日志' },
    // v5.1: 配置改为设置
    { name: 'settings', label: '设置' }
  ];

  // v5.1+: 使用真实的代理网关状态
  const { running: proxyRunning = false, port: proxyPort = 0 } = proxyStatus;

  // 确定显示状态：优先使用代理网关真实状态，回退到连接状态
  const getDisplayStatus = () => {
    // 如果 Wails 事件通道未连接
    if (connectionStatus !== 'connected') {
      return {
        color: 'bg-amber-500',
        text: connectionStatus === 'connecting' ? '连接中...' : '事件通道断开',
        ping: connectionStatus === 'connecting'
      };
    }
    // Wails 事件通道已连接，检查代理网关状态
    if (proxyRunning && proxyPort > 0) {
      return {
        color: 'bg-emerald-500',
        text: `代理端口 :${proxyPort}`,
        ping: true
      };
    }
    // 代理网关未运行或端口未知
    return {
      color: 'bg-rose-500',
      text: '代理未运行',
      ping: false
    };
  };

  const status = getDisplayStatus();

  // Wails 环境下为窗口按钮预留顶部空间
  const isWails = isWailsEnvironment();
  const titlebarPadding = isWails ? 'pt-7' : '';

  return (
    <nav className={`sticky top-0 z-50 bg-white/80 backdrop-blur-xl border-b border-slate-200/60 supports-[backdrop-filter]:bg-white/60 ${titlebarPadding}`}
         style={isWails ? { WebkitAppRegion: 'drag' } : {}}>
      <div className="max-w-7xl mx-auto px-4 xl:px-6 h-16 flex items-center justify-between gap-4 xl:gap-6"
           style={isWails ? { WebkitAppRegion: 'no-drag' } : {}}>
        {/* 左侧：Logo + 导航 */}
        <div className="flex min-w-0 flex-1 items-center gap-4 xl:gap-8 overflow-hidden">
          {/* Logo */}
          <div
            className="flex items-center space-x-2.5 group cursor-pointer"
            onClick={() => onTabChange('overview')}
          >
            <div className="w-8 h-8 bg-slate-900 rounded-lg flex items-center justify-center text-white shadow-lg">
              <Command size={16} strokeWidth={3} />
            </div>
            <span className="font-bold text-base xl:text-lg tracking-tight text-slate-900 whitespace-nowrap">
              CC Gateway
            </span>
          </div>

          {/* 导航标签 */}
          <div className="hidden md:flex min-w-0 flex-1 xl:flex-none xl:w-auto items-center overflow-x-auto xl:overflow-visible bg-slate-100/50 p-1 rounded-full border border-slate-200/60 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {tabs.map(tab => (
              <NavItem
                key={tab.name}
                label={tab.label}
                active={activeTab === tab.name}
                onClick={() => onTabChange(tab.name)}
              />
            ))}
          </div>
        </div>

        {/* 右侧：状态指示器 + 头像 */}
        <div className="shrink-0 flex items-center space-x-3 lg:space-x-4">
          {/* 连接状态 */}
          <div className="hidden sm:flex items-center px-3 py-1.5 bg-white border border-slate-200 rounded-md shadow-sm text-xs text-slate-500">
            <span className="relative flex h-2 w-2 mr-2">
              {status.ping && (
                <span className={`animate-ping absolute inline-flex h-full w-full rounded-full ${status.color} opacity-75`}></span>
              )}
              <span className={`relative inline-flex rounded-full h-2 w-2 ${status.color}`}></span>
            </span>
            {status.text}
          </div>

          {/* 用户头像 */}
          <div className="w-8 h-8 rounded-full bg-gradient-to-tr from-indigo-500 to-purple-500 border-2 border-white shadow-md cursor-pointer hover:ring-2 ring-indigo-200 transition-all"></div>
        </div>
      </div>

      {/* 移动端导航 */}
      <div className="md:hidden flex items-center space-x-2 px-4 pb-3 overflow-x-auto">
        {tabs.map(tab => (
          <NavItem
            key={tab.name}
            label={tab.label}
            active={activeTab === tab.name}
            onClick={() => onTabChange(tab.name)}
          />
        ))}
      </div>
    </nav>
  );
};

export default Header;
