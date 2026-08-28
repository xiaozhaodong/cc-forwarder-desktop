// ============================================
// PortInfo - 端口信息显示组件
// v5.1.0 (2025-12-08)
// ============================================

import { CheckCircle2, AlertTriangle, Server } from 'lucide-react';

const PortInfo = ({ portInfo, loading = false }) => {
  if (loading || !portInfo) {
    return (
      <div className="bg-surface-sub rounded-xl p-4 animate-pulse">
        <div className="h-4 bg-surface-emph rounded w-24 mb-2"></div>
        <div className="h-6 bg-surface-emph rounded w-16"></div>
      </div>
    );
  }

  const { preferred_port, actual_port, was_occupied } = portInfo;

  return (
    <div className="bg-gradient-to-r from-surface-sub to-accent-soft rounded-xl p-4 border border-line">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent-soft rounded-lg">
            <Server size={20} className="text-accent" />
          </div>
          <div>
            <div className="text-xs text-fg-subtle font-medium">当前服务端口</div>
            <div className="flex items-center gap-2 mt-0.5">
              <span className="text-2xl font-bold text-fg">{actual_port}</span>
              {was_occupied ? (
                <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-warn-soft text-warn">
                  <AlertTriangle size={10} className="mr-1" />
                  端口 {preferred_port} 被占用
                </span>
              ) : (
                <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-success-soft text-success">
                  <CheckCircle2 size={10} className="mr-1" />
                  正常
                </span>
              )}
            </div>
          </div>
        </div>

        <div className="text-right">
          <div className="text-xs text-fg-subtle">首选端口</div>
          <div className="text-sm font-mono text-fg-body">{preferred_port}</div>
        </div>
      </div>

      {was_occupied && (
        <div className="mt-3 pt-3 border-t border-line">
          <p className="text-xs text-fg-muted">
            首选端口 {preferred_port} 已被其他程序占用，系统自动切换到端口 {actual_port}。
            如需使用端口 {preferred_port}，请关闭占用该端口的程序后重启应用。
          </p>
        </div>
      )}
    </div>
  );
};

export default PortInfo;
