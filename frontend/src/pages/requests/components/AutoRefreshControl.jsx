// ============================================
// AutoRefreshControl - 自动刷新控制组件
// 2025-12-01 12:56:37
// ============================================

import { useState, useRef } from 'react';
import { Timer, ChevronDown, Check, RotateCcw } from 'lucide-react';

/**
 * AutoRefreshControl - 自动刷新控制组件（参考 request_test.jsx 优化版）
 *
 * 功能:
 * 1. 连体按钮设计（自动刷新 + 手动刷新）
 * 2. 下拉菜单直接选择间隔（包括 Off）
 * 3. 简洁的 spinner 动画
 *
 * @param {Object} props
 * @param {boolean} props.isEnabled - 是否启用自动刷新
 * @param {number} props.interval - 刷新间隔(秒)
 * @param {Function} props.onToggle - 切换自动刷新（已废弃，直接通过选择间隔控制）
 * @param {Function} props.onIntervalChange - 更改刷新间隔
 * @param {Function} props.onManualRefresh - 手动刷新回调
 */
const AutoRefreshControl = ({
  _isEnabled = false,
  interval = 5,
  onIntervalChange,
  onManualRefresh
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const dropdownRef = useRef(null);

  // 刷新间隔选项（包括 Off）
  const options = [
    { label: 'Off', value: 0 },
    { label: '5s', value: 5 },
    { label: '10s', value: 10 },
    { label: '30s', value: 30 },
    { label: '1m', value: 60 },
  ];

  return (
    <div className="relative flex shrink-0 items-center" ref={dropdownRef}>
      {/* 主切换/显示按钮 */}
      <button
        onClick={() => setIsOpen(!isOpen)}
        className={`flex items-center gap-1.5 xl:gap-2 px-2.5 xl:px-3 h-9 rounded-l-lg text-sm font-medium border transition-all relative ${
          interval > 0
            ? 'bg-indigo-50 border-indigo-200 text-indigo-700 z-10'
            : 'bg-white border-gray-200 text-gray-600 hover:bg-gray-50 z-0'
        }`}
        title="自动刷新设置"
      >
        {interval > 0 ? (
          <>
            <div className="relative w-3.5 h-3.5 flex items-center justify-center">
              {/* Spinner/倒计时动画 */}
              <svg className="animate-spin w-full h-full text-indigo-600" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
            </div>
            <span>
              <span className="hidden xl:inline">Auto </span>
              {interval < 60 ? `${interval}s` : '1m'}
            </span>
          </>
        ) : (
          <>
            <Timer className="w-4 h-4" />
            <span className="hidden xl:inline">Auto Off</span><span className="xl:hidden">Off</span>
          </>
        )}
        <ChevronDown className={`w-3 h-3 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
      </button>

      {/* 手动刷新按钮 - 连体右侧 */}
      <button
        onClick={onManualRefresh}
        className={`h-9 w-9 flex items-center justify-center border rounded-r-lg border-gray-200 bg-white text-gray-500 hover:text-indigo-600 hover:bg-gray-50 -ml-px transition-colors z-0 ${
          interval > 0 ? 'border-l-indigo-200' : ''
        }`}
        title="立即刷新"
      >
        <RotateCcw className="w-4 h-4" />
      </button>

      {/* 下拉菜单 */}
      {isOpen && (
        <>
          {/* 遮罩层 - 点击关闭 */}
          <div className="fixed inset-0 z-10" onClick={() => setIsOpen(false)}></div>

          {/* 菜单内容 */}
          <div className="absolute top-full right-0 mt-2 w-32 bg-white rounded-lg shadow-xl border border-gray-100 ring-1 ring-black/5 z-20 overflow-hidden animate-in fade-in slide-in-from-top-2 duration-200">
            <div className="py-1">
              {options.map((opt) => (
                <button
                  key={opt.value}
                  onClick={() => {
                    onIntervalChange?.(opt.value);
                    setIsOpen(false);
                  }}
                  className={`w-full text-left px-4 py-2 text-sm flex items-center justify-between ${
                    interval === opt.value
                      ? 'bg-indigo-50 text-indigo-700 font-medium'
                      : 'text-gray-700 hover:bg-gray-50'
                  }`}
                >
                  {opt.label}
                  {interval === opt.value && <Check className="w-3.5 h-3.5" />}
                </button>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  );
};

export default AutoRefreshControl;
