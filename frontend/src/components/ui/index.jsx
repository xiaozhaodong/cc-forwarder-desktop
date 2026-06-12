// ============================================
// UI 组件库 - 基于 claude-dashboard 样式
// 2025-11-28
// ============================================

import { useState, useRef, useEffect, useCallback } from 'react';
import {
  CheckCircle2,
  Key,
  ChevronDown,
  ChevronUp,
  AlertCircle,
  XCircle,
  Clock,
  Loader2
} from 'lucide-react';

// ============================================
// KPI 卡片组件
// ============================================
export const KPICard = ({ title, value, tooltip, subText, icon: Icon, statusColor = 'bg-slate-100 text-slate-600' }) => (
  <div className="bg-white p-5 rounded-2xl border border-slate-200/60 shadow-sm flex flex-col justify-between hover:shadow-md transition-shadow">
    <div className="flex justify-between items-start mb-2">
      <div className={`p-2 rounded-lg ${statusColor}`}>
        <Icon size={20} />
      </div>
    </div>
    <div>
      <h3 className="text-slate-500 text-xs font-semibold uppercase tracking-wider mb-1">{title}</h3>
      <div
        className="text-xl font-bold text-slate-900 truncate cursor-default"
        title={tooltip || value}
      >
        {value}
      </div>
      {subText && <div className="text-xs text-slate-400 mt-1 font-medium">{subText}</div>}
    </div>
  </div>
);

// ============================================
// 统计详情项
// ============================================
export const StatDetailItem = ({ label, value, unit, valueColor }) => (
  <div className="bg-slate-50 rounded-xl p-4 flex flex-col items-center justify-center border border-slate-100 hover:border-slate-200 transition-colors group">
    <div className={`text-2xl font-bold mb-1 ${valueColor || 'text-slate-900'} group-hover:scale-105 transition-transform`}>
      {value}<span className="text-sm font-medium text-slate-500 ml-0.5">{unit}</span>
    </div>
    <div className="text-xs text-slate-400 font-medium">{label}</div>
  </div>
);

// ============================================
// 摘要卡片（追踪页面用）
// ============================================
export const TraceSummaryCard = ({ title, value, subValue, icon: Icon, colorClass, borderColorClass }) => (
  <div className="bg-white p-4 rounded-xl border border-slate-200/60 shadow-sm flex items-center space-x-4 relative overflow-hidden">
    <div className={`absolute left-0 top-0 bottom-0 w-1 ${borderColorClass}`}></div>
    <div className={`p-2.5 rounded-lg ${colorClass}`}>
      <Icon size={20} strokeWidth={2.5} />
    </div>
    <div>
      <div className="text-xl font-bold text-slate-900 leading-tight">{value}</div>
      <div className="text-xs text-slate-500 font-medium mt-0.5">
        {title} {subValue && <span className="text-slate-400">({subValue})</span>}
      </div>
    </div>
  </div>
);

// ============================================
// 状态徽章
// ============================================
export const StatusBadge = ({ status }) => {
  const configs = {
    healthy: {
      bg: 'bg-emerald-50',
      text: 'text-emerald-700',
      border: 'border-emerald-100',
      dot: 'bg-emerald-500',
      label: '健康'
    },
    unhealthy: {
      bg: 'bg-rose-50',
      text: 'text-rose-700',
      border: 'border-rose-100',
      dot: 'bg-rose-500',
      label: '异常'
    },
    completed: {
      bg: 'bg-emerald-100',
      text: 'text-emerald-700',
      border: 'border-emerald-200',
      icon: CheckCircle2,
      label: '已完成'
    },
    failed: {
      bg: 'bg-rose-100',
      text: 'text-rose-700',
      border: 'border-rose-200',
      icon: XCircle,
      label: '失败'
    },
    pending: {
      bg: 'bg-amber-100',
      text: 'text-amber-700',
      border: 'border-amber-200',
      icon: Clock,
      label: '等待中'
    },
    processing: {
      bg: 'bg-blue-100',
      text: 'text-blue-700',
      border: 'border-blue-200',
      icon: Loader2,
      label: '处理中'
    }
  };

  const config = configs[status] || configs.pending;
  const IconComponent = config.icon;

  return (
    <div className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium border ${config.bg} ${config.text} ${config.border}`}>
      {config.dot && <span className={`w-1.5 h-1.5 rounded-full mr-1.5 ${config.dot}`}></span>}
      {IconComponent && <IconComponent size={12} className={`mr-1 ${config.text}`} />}
      {config.label}
    </div>
  );
};

// ============================================
// 延迟指示器
// ============================================
export const LatencyIndicator = ({ ms }) => {
  // 处理各种输入格式：数字、"376ms" 字符串、"-" 等
  let value = ms;
  if (typeof ms === 'string') {
    if (ms === '-' || ms === '') return <span className="text-slate-400">-</span>;
    // 提取数字部分，去掉 "ms" 后缀
    value = parseFloat(ms.replace(/ms$/i, '')) || 0;
  }
  if (!value || value === 0) return <span className="text-slate-400">-</span>;

  let colorClass = 'text-emerald-600';
  if (value > 500) colorClass = 'text-amber-600';
  if (value > 1000) colorClass = 'text-rose-600';

  return (
    <span className={`font-mono font-medium ${colorClass}`}>
      {Math.round(value)}ms
    </span>
  );
};

// ============================================
// Token 下拉选择器
// ============================================
export const TokenSelector = ({ tokens = [], currentTokenId, onSelect, disabled = false }) => {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef(null);
  const currentToken = tokens.find(t => t.id === currentTokenId) || tokens[0];

  useEffect(() => {
    const handleClickOutside = (event) => {
      if (containerRef.current && !containerRef.current.contains(event.target)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  if (!currentToken) {
    return (
      <span className="text-xs text-slate-400 font-mono">--</span>
    );
  }

  return (
    <div className="relative inline-block text-left" ref={containerRef}>
      <button
        type="button"
        onClick={() => !disabled && setIsOpen(!isOpen)}
        disabled={disabled}
        className={`group inline-flex items-center justify-between w-36 px-3 py-1.5 text-sm bg-white border rounded-lg shadow-sm transition-all duration-200 ${
          disabled
            ? 'opacity-50 cursor-not-allowed border-slate-200'
            : isOpen
              ? 'border-blue-500 ring-2 ring-blue-100 z-10'
              : 'border-slate-200 hover:border-slate-300 hover:shadow'
        }`}
      >
        <div className="flex items-center min-w-0">
          <Key size={14} className="mr-2 text-amber-500 flex-shrink-0" fill="currentColor" fillOpacity={0.6} strokeWidth={2.5} />
          <span className={`font-medium truncate ${isOpen ? 'text-blue-600' : 'text-slate-700 group-hover:text-slate-900'}`}>
            {currentToken.name}
          </span>
        </div>
        <div className="ml-2 flex-shrink-0 text-slate-400 group-hover:text-slate-600">
          {isOpen ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        </div>
      </button>

      {isOpen && (
        <div className="absolute z-50 left-0 mt-2 w-64 origin-top-left bg-white border border-slate-100 rounded-xl shadow-2xl ring-1 ring-black/5 focus:outline-none overflow-hidden">
          <div className="px-4 py-2.5 bg-slate-50/80 border-b border-slate-100 backdrop-blur-sm">
            <span className="text-[10px] font-bold text-slate-400 uppercase tracking-wider">选择 Token</span>
          </div>
          <div className="py-1 max-h-60 overflow-y-auto custom-scrollbar">
            {tokens.map((token) => {
              const isActive = token.id === currentTokenId;
              return (
                <div
                  key={token.id}
                  onClick={() => {
                    onSelect?.(token.id);
                    setIsOpen(false);
                  }}
                  className={`group relative flex items-center justify-between px-4 py-3 cursor-pointer transition-colors ${
                    isActive ? 'bg-blue-50/60' : 'hover:bg-slate-50'
                  }`}
                >
                  <div className="flex flex-col min-w-0 pr-2">
                    <span className={`text-sm font-semibold truncate mb-0.5 ${isActive ? 'text-blue-600' : 'text-slate-700'}`}>
                      {token.name}
                    </span>
                    <span className="text-xs font-mono text-slate-400 truncate tracking-tight">
                      {token.keyMask || token.key_mask || '••••••••'}
                    </span>
                  </div>
                  {isActive ? (
                    <span className="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-500 text-white shadow-sm flex-shrink-0">当前</span>
                  ) : (
                    <span className="hidden group-hover:inline-flex text-xs text-slate-400">选择</span>
                  )}
                  {isActive && <div className="absolute left-0 top-0 bottom-0 w-1 bg-blue-500"></div>}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};

// ============================================
// 加载 Spinner
// ============================================
export const LoadingSpinner = ({ size = 'md', text = '加载中...' }) => {
  const sizeClasses = {
    sm: 'w-4 h-4',
    md: 'w-8 h-8',
    lg: 'w-12 h-12'
  };

  return (
    <div className="flex flex-col items-center justify-center py-12">
      <div className={`${sizeClasses[size]} border-2 border-slate-200 border-t-indigo-600 rounded-full animate-spin`}></div>
      {text && <p className="mt-3 text-sm text-slate-500">{text}</p>}
    </div>
  );
};

// ============================================
// 空状态
// ============================================
export const EmptyState = ({ icon: Icon, title, description, action }) => (
  <div className="flex flex-col items-center justify-center py-16 text-center">
    {Icon && (
      <div className="p-4 bg-slate-100 rounded-full mb-4">
        <Icon size={32} className="text-slate-400" />
      </div>
    )}
    <h3 className="text-lg font-semibold text-slate-700 mb-2">{title}</h3>
    {description && <p className="text-sm text-slate-500 max-w-md mb-4">{description}</p>}
    {action}
  </div>
);

// ============================================
// 错误提示
// ============================================
export const ErrorMessage = ({ title = '加载失败', message, onRetry }) => (
  <div className="flex flex-col items-center justify-center py-12 text-center">
    <div className="p-4 bg-rose-50 rounded-full mb-4">
      <AlertCircle size={32} className="text-rose-500" />
    </div>
    <h3 className="text-lg font-semibold text-rose-700 mb-2">{title}</h3>
    {message && <p className="text-sm text-rose-600 max-w-md mb-4">{message}</p>}
    {onRetry && (
      <button
        onClick={onRetry}
        className="px-4 py-2 bg-rose-600 text-white rounded-lg text-sm font-medium hover:bg-rose-700 transition-colors"
      >
        重试
      </button>
    )}
  </div>
);

// ============================================
// 按钮组件
// ============================================
export const Button = ({
  children,
  variant = 'primary',
  size = 'md',
  icon: Icon,
  loading = false,
  disabled = false,
  className = '',
  ...props
}) => {
  const variants = {
    primary: 'bg-indigo-600 text-white hover:bg-indigo-700 shadow-md hover:shadow-lg',
    secondary: 'bg-slate-100 text-slate-600 hover:bg-slate-200',
    success: 'bg-emerald-600 text-white hover:bg-emerald-700',
    danger: 'bg-rose-600 text-white hover:bg-rose-700',
    ghost: 'bg-transparent text-slate-600 hover:bg-slate-100'
  };

  const sizes = {
    sm: 'px-3 py-1.5 text-xs',
    md: 'px-4 py-2 text-sm',
    lg: 'px-6 py-3 text-base'
  };

  return (
    <button
      disabled={disabled || loading}
      className={`
        inline-flex items-center justify-center font-medium rounded-lg transition-all duration-200
        ${variants[variant]} ${sizes[size]}
        ${disabled || loading ? 'opacity-50 cursor-not-allowed' : 'hover:-translate-y-0.5'}
        ${className}
      `}
      {...props}
    >
      {loading ? (
        <Loader2 size={16} className="animate-spin mr-2" />
      ) : Icon && (
        <Icon size={16} className="mr-2" />
      )}
      {children}
    </button>
  );
};

// ============================================
// 输入框
// ============================================
export const Input = ({
  label,
  error,
  className = '',
  ...props
}) => (
  <div className="flex flex-col">
    {label && (
      <label className="text-sm font-medium text-slate-700 mb-1.5">{label}</label>
    )}
    <input
      className={`
        w-full px-3 py-2 border rounded-lg text-sm
        focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500
        ${error ? 'border-rose-300 focus:ring-rose-500 focus:border-rose-500' : 'border-slate-200'}
        ${className}
      `}
      {...props}
    />
    {error && (
      <span className="text-xs text-rose-500 mt-1">{error}</span>
    )}
  </div>
);

// ============================================
// 选择框（原生）
// ============================================
export const Select = ({
  label,
  options = [],
  className = '',
  ...props
}) => (
  <div className="flex flex-col">
    {label && (
      <label className="text-sm font-medium text-slate-700 mb-1.5">{label}</label>
    )}
    <select
      className={`
        px-3 py-2 bg-slate-50 border border-slate-200 rounded-lg text-sm text-slate-600
        focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500
        ${className}
      `}
      {...props}
    >
      {options.map((opt) => (
        <option key={opt.value} value={opt.value}>
          {opt.label}
        </option>
      ))}
    </select>
  </div>
);

// ============================================
// 自定义下拉选择器（统一样式）
// ============================================
export const CustomSelect = ({
  options = [],
  value,
  onChange,
  size = 'sm',
  placeholder = '请选择',
  disabled = false,
  className = ''
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [dropdownPosition, setDropdownPosition] = useState({ top: 0, left: 0, width: 0, isUpward: false });
  const containerRef = useRef(null);
  const buttonRef = useRef(null);
  const dropdownRef = useRef(null);

  const currentOption = options.find(opt => opt.value === value);

  // 更新下拉菜单位置（智能判断向上或向下）
  const updatePosition = useCallback(() => {
    if (buttonRef.current && dropdownRef.current) {
      const buttonRect = buttonRef.current.getBoundingClientRect();
      const dropdownHeight = dropdownRef.current.offsetHeight || 200; // 预估高度
      const viewportHeight = window.innerHeight;

      // 计算下方和上方的可用空间
      const spaceBelow = viewportHeight - buttonRect.bottom;
      const spaceAbove = buttonRect.top;

      // 判断是否向上展开
      const shouldOpenUpward = spaceBelow < dropdownHeight && spaceAbove > spaceBelow;

      setDropdownPosition({
        top: shouldOpenUpward ? buttonRect.top - dropdownHeight - 4 : buttonRect.bottom + 4,
        left: buttonRect.left,
        width: buttonRect.width,
        isUpward: shouldOpenUpward
      });
    }
  }, []);

  // 计算下拉菜单位置
  useEffect(() => {
    if (isOpen) {
      // 首次打开时计算位置
      updatePosition();

      // 监听滚动和窗口调整，更新位置
      window.addEventListener('scroll', updatePosition, true);
      window.addEventListener('resize', updatePosition);

      return () => {
        window.removeEventListener('scroll', updatePosition, true);
        window.removeEventListener('resize', updatePosition);
      };
    }
  }, [isOpen, updatePosition]);

  useEffect(() => {
    const handleClickOutside = (event) => {
      if (containerRef.current && !containerRef.current.contains(event.target)) {
        setIsOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const handleSelect = (optValue) => {
    onChange?.(optValue);
    setIsOpen(false);
  };

  const sizeClasses = {
    xs: 'px-2 py-1 text-xs min-w-[80px]',
    sm: 'px-2.5 py-1.5 text-xs min-w-[100px]',
    md: 'px-3 py-2 text-sm min-w-[120px]'
  };

  const dropdownSizeClasses = {
    xs: 'text-xs',
    sm: 'text-xs',
    md: 'text-sm'
  };

  return (
    <div className={`relative inline-block ${className}`} ref={containerRef}>
      <button
        ref={buttonRef}
        type="button"
        onClick={() => !disabled && setIsOpen(!isOpen)}
        disabled={disabled}
        className={`
          group inline-flex w-full items-center justify-between bg-white border rounded-lg
          transition-all duration-200 font-medium
          ${sizeClasses[size]}
          ${disabled
            ? 'opacity-50 cursor-not-allowed border-slate-200 text-slate-400'
            : isOpen
              ? 'border-indigo-500 ring-2 ring-indigo-100 text-indigo-600'
              : 'border-slate-200 text-slate-600 hover:border-slate-300 hover:text-slate-700'
          }
        `}
      >
        <span className="truncate">
          {currentOption?.label || placeholder}
        </span>
        <ChevronDown
          size={size === 'xs' ? 12 : 14}
          className={`ml-1.5 flex-shrink-0 transition-transform duration-200 ${
            isOpen ? 'rotate-180' : ''
          } ${disabled ? 'text-slate-300' : 'text-slate-400'}`}
        />
      </button>

      {isOpen && (
        <div
          ref={dropdownRef}
          className={`
            fixed z-[9999] bg-white border border-slate-200
            rounded-lg shadow-lg overflow-hidden animate-fade-in
            ${dropdownSizeClasses[size]}
          `}
          style={{
            top: `${dropdownPosition.top}px`,
            left: `${dropdownPosition.left}px`,
            minWidth: `${dropdownPosition.width}px`,
            maxHeight: '240px', // 最大高度，超出滚动
            transformOrigin: dropdownPosition.isUpward ? 'bottom' : 'top'
          }}
        >
          <div className="py-1 max-h-[240px] overflow-y-auto">
            {options.map((option) => {
              const isSelected = option.value === value;
              return (
                <div
                  key={option.value}
                  onClick={() => handleSelect(option.value)}
                  className={`
                    px-3 py-2 cursor-pointer transition-colors flex items-center justify-between whitespace-nowrap
                    ${isSelected
                      ? 'bg-indigo-50 text-indigo-600 font-medium'
                      : 'text-slate-600 hover:bg-slate-50'
                    }
                  `}
                >
                  <span>{option.label}</span>
                  {isSelected && (
                    <CheckCircle2 size={14} className="text-indigo-500 ml-2 flex-shrink-0" />
                  )}
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
};
