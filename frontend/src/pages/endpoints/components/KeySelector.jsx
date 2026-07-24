// ============================================
// Key 选择器组件 - Command Palette 风格
// 用于显示和切换端点的 Token/API Key
// 2025-12-01 升级版
// ============================================

import { useState, useRef } from 'react';
import { ChevronDown, Key, Check, Search, Settings2, Gauge } from 'lucide-react';
import useModalLifecycle from '@hooks/useModalLifecycle.js';

/**
 * 根据 Token 名称推断类型
 * @param {string} name - Token 名称
 * @returns {string} - 'Pro' | 'Ent' | 'Free' | 'Std'
 */
const inferTokenType = (name) => {
  if (!name) return 'Std';
  const lowerName = name.toLowerCase();
  if (lowerName.includes('pro') || lowerName.includes('特价')) return 'Pro';
  if (lowerName.includes('ent') || lowerName.includes('主号')) return 'Ent';
  if (lowerName.includes('free') || lowerName.includes('测试')) return 'Free';
  return 'Std';
};

/**
 * 获取 Token 类型对应的颜色样式
 * @param {string} type - Token 类型
 * @returns {string} - Tailwind 类名
 */
const getTypeColorClass = (type) => {
  const colorMap = {
    'Pro': 'bg-purple-100 text-purple-700 border-purple-200',
    'Ent': 'bg-blue-100 text-blue-700 border-blue-200',
    'Free': 'bg-emerald-100 text-emerald-700 border-emerald-200',
    'Std': 'bg-slate-100 text-slate-600 border-slate-200'
  };
  return colorMap[type] || colorMap['Std'];
};

/**
 * 模拟使用率（基于 index 生成伪随机值）
 * TODO: 后续可从后端 API 获取真实使用率数据
 * @param {number} index - Key 索引
 * @returns {number} - 使用率百分比 (0-100)
 */
const mockUsagePercentage = (index) => {
  const seed = index * 17 + 42;
  return (seed % 95) + 5; // 5-100 之间
};

/**
 * Command Palette 风格的 Token 选择模态框
 */
const TokenSelectionModal = ({ isOpen, onClose, keys, activeKey, onSelect, switching, endpointName }) => {
  const [searchTerm, setSearchTerm] = useState('');
  const searchInputRef = useRef(null);

  useModalLifecycle({ open: isOpen, onClose, initialFocusRef: searchInputRef });

  if (!isOpen) return null;

  // 搜索过滤
  const filteredKeys = keys.filter(k => {
    const name = k.name || `Key ${k.index + 1}`;
    const masked = k.masked || '';
    return name.toLowerCase().includes(searchTerm.toLowerCase()) ||
           masked.toLowerCase().includes(searchTerm.toLowerCase());
  });

  return (
    <div className="fixed inset-0 z-[10000] flex items-center justify-center p-4">
      {/* 背景遮罩 - 模糊效果 */}
      <div
        className="absolute inset-0 bg-slate-900/20 backdrop-blur-sm transition-opacity animate-in fade-in duration-200"
        onClick={onClose}
      />

      {/* 模态框内容 - Command Palette 样式 */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label={`选择 ${endpointName} 的 Token`}
        className="relative w-full max-w-lg bg-white rounded-2xl shadow-2xl ring-1 ring-black/5 flex flex-col max-h-[80vh] animate-in zoom-in-95 fade-in duration-200"
      >

        {/* 搜索头部 */}
        <div className="p-4 border-b border-slate-100 flex items-center gap-3">
          <Search className="w-5 h-5 text-slate-400 flex-shrink-0" />
          <input
            ref={searchInputRef}
            type="text"
            placeholder={`搜索 ${endpointName} 的 Token 配置...`}
            className="flex-1 text-base bg-transparent border-none focus:ring-0 focus:outline-none placeholder:text-slate-400 text-slate-900 p-0"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
          <div className="hidden sm:flex items-center gap-1">
            <kbd className="px-2 py-1 bg-slate-100 border border-slate-200 rounded text-[10px] text-slate-500 font-sans">
              ESC
            </kbd>
          </div>
        </div>

        {/* Token 列表 */}
        <div className="flex-1 overflow-y-auto p-2 scrollbar-thin scrollbar-thumb-slate-200">
          <div className="text-xs font-medium text-slate-400 px-3 py-2 uppercase tracking-wider">
            可用的 Token 配置
          </div>

          {filteredKeys.length > 0 ? (
            filteredKeys.map((key) => {
              const isActive = key.index === activeKey?.index;
              const tokenName = key.name || `Key ${key.index + 1}`;
              const tokenType = inferTokenType(tokenName);
              const tokenTypeColor = getTypeColorClass(tokenType);
              const usagePercent = mockUsagePercentage(key.index);

              return (
                <button
                  key={key.index}
                  onClick={() => onSelect(key.index)}
                  disabled={switching}
                  className={`
                    w-full flex items-center justify-between p-3 rounded-xl transition-all group
                    ${isActive
                      ? 'bg-indigo-50 border border-indigo-100'
                      : 'hover:bg-slate-50 border border-transparent'}
                    ${switching ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}
                  `}
                >
                  {/* 左侧：图标 + 信息 */}
                  <div className="flex items-center gap-4 flex-1 min-w-0">
                    <div className={`p-2 rounded-lg transition-all ${
                      isActive
                        ? 'bg-white text-indigo-600 shadow-sm'
                        : 'bg-slate-100 text-slate-500 group-hover:bg-white group-hover:shadow-sm'
                    }`}>
                      <Key className="w-5 h-5" />
                    </div>

                    <div className="text-left flex-1 min-w-0">
                      {/* Token 名称 + 类型标签 */}
                      <div className="flex items-center gap-2 mb-0.5">
                        <span className={`font-medium truncate ${
                          isActive ? 'text-indigo-900' : 'text-slate-900'
                        }`}>
                          {tokenName}
                        </span>
                        <span className={`text-[10px] px-1.5 py-0.5 rounded font-bold border ${tokenTypeColor} flex-shrink-0`}>
                          {tokenType}
                        </span>
                      </div>

                      {/* Masked Key */}
                      <div className="text-xs text-slate-400 font-mono truncate">
                        {key.masked}
                      </div>
                    </div>
                  </div>

                  {/* 右侧：使用率 + 状态 */}
                  <div className="flex items-center gap-6 ml-4">
                    {/* 使用率指示器 */}
                    <div className="hidden sm:flex flex-col items-end gap-1 min-w-[80px]">
                      <div className="text-[10px] text-slate-400 uppercase tracking-wider flex items-center gap-1">
                        <Gauge className="w-3 h-3" /> Quota
                      </div>
                      <div className="w-full h-1.5 bg-slate-100 rounded-full overflow-hidden">
                        <div
                          className={`h-full rounded-full transition-all ${
                            usagePercent > 90 ? 'bg-rose-500' :
                            usagePercent > 70 ? 'bg-amber-500' :
                            'bg-emerald-500'
                          }`}
                          style={{ width: `${usagePercent}%` }}
                        />
                      </div>
                    </div>

                    {/* 当前激活标记 */}
                    {isActive ? (
                      <Check className="w-5 h-5 text-indigo-600 flex-shrink-0" />
                    ) : (
                      <div className="w-5 h-5 flex-shrink-0" />
                    )}
                  </div>
                </button>
              );
            })
          ) : (
            <div className="p-8 text-center text-slate-500">
              <p>未找到 &quot;{searchTerm}&quot;</p>
              <button className="mt-2 text-indigo-600 font-medium text-sm hover:underline">
                创建新 Token
              </button>
            </div>
          )}
        </div>

        {/* 底部信息栏 */}
        <div className="p-3 border-t border-slate-100 bg-slate-50/50 rounded-b-2xl flex justify-between items-center text-xs text-slate-500">
          <span>共有 {keys.length} 个配置</span>
          <button className="flex items-center gap-1.5 hover:text-slate-900 transition-colors">
            <Settings2 className="w-3.5 h-3.5" /> 管理 Tokens
          </button>
        </div>
      </div>
    </div>
  );
};

/**
 * Key 选择器主组件
 * @param {string} endpointName - 端点名称
 * @param {string} keyType - 'token' | 'api_key'
 * @param {Array} keys - Key 列表 [{index, name, masked, is_active}]
 * @param {Function} onSwitch - 切换回调 (endpointName, keyType, index) => Promise
 * @param {boolean} disabled - 是否禁用
 */
const KeySelector = ({
  endpointName,
  keyType = 'token',
  keys = [],
  onSwitch,
  disabled = false
}) => {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [switching, setSwitching] = useState(false);

  // 找到当前激活的 Key
  const activeKey = keys.find(k => k.is_active) || keys[0];

  // 如果只有一个 Key 或没有 Key，只显示静态标签
  if (keys.length <= 1) {
    if (!activeKey) return <span className="text-slate-400 text-xs">-</span>;

    const tokenType = inferTokenType(activeKey.name);
    const tokenTypeColor = getTypeColorClass(tokenType);

    return (
      <div className="inline-flex items-center gap-2 px-2.5 py-1.5 bg-white border border-slate-200 rounded-lg w-[160px]">
        {/* 类型标签 */}
        <span className={`text-[10px] px-1.5 py-0.5 rounded font-bold border ${tokenTypeColor} flex-shrink-0`}>
          {tokenType}
        </span>
        {/* Token 名称 */}
        <span className="text-xs font-medium text-slate-700 truncate flex-1" title={activeKey.masked}>
          {activeKey.name || 'default'}
        </span>
      </div>
    );
  }

  // 切换处理
  const handleSelect = async (index) => {
    if (switching || index === activeKey?.index) {
      setIsModalOpen(false);
      return;
    }

    setSwitching(true);
    try {
      await onSwitch(endpointName, keyType, index);
      setIsModalOpen(false);
    } catch (error) {
      console.error('Key 切换失败:', error);
      alert('Key 切换失败: ' + (error.message || '未知错误'));
    } finally {
      setSwitching(false);
    }
  };

  const displayName = activeKey?.name || `Key ${(activeKey?.index || 0) + 1}`;
  const tokenType = inferTokenType(displayName);
  const tokenTypeColor = getTypeColorClass(tokenType);

  return (
    <>
      {/* 触发按钮 - Command Palette 风格 */}
      <button
        onClick={() => !disabled && !switching && setIsModalOpen(true)}
        disabled={disabled || switching}
        className={`
          group relative inline-flex items-center gap-2 px-2.5 py-1.5 bg-white border rounded-lg
          transition-all duration-200 w-[160px]
          ${disabled || switching
            ? 'opacity-50 cursor-not-allowed border-slate-200'
            : 'cursor-pointer border-slate-200 hover:border-indigo-300 hover:ring-2 hover:ring-indigo-100 hover:shadow-sm'}
        `}
        title={`点击更换 Token - ${displayName}`}
      >
        {/* 类型指示器 */}
        <div className={`flex items-center justify-center h-6 px-1.5 rounded-md text-[10px] font-bold uppercase tracking-wider border ${tokenTypeColor} flex-shrink-0`}>
          {tokenType}
        </div>

        {/* Token 名称 */}
        <span className="flex-1 text-xs font-medium text-slate-700 truncate text-left">
          {switching ? '切换中...' : displayName}
        </span>

        {/* 下拉图标 */}
        <div className="flex-shrink-0 w-4 h-4 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
          <ChevronDown className="w-3.5 h-3.5 text-indigo-400" />
        </div>
      </button>

      {/* Command Palette 模态框 */}
      <TokenSelectionModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        keys={keys}
        activeKey={activeKey}
        onSelect={handleSelect}
        switching={switching}
        endpointName={endpointName}
      />
    </>
  );
};

export default KeySelector;
