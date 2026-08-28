// 日志页面常量配置
import {
  Info,
  AlertTriangle,
  XCircle,
  CheckCircle,
} from 'lucide-react';

// 日志级别配置
export const LOG_LEVELS = {
  DEBUG: {
    color: 'text-fg-muted',
    bg: 'bg-surface-sub',
    icon: Info
  },
  INFO: {
    color: 'text-info',
    bg: 'bg-info-soft',
    icon: CheckCircle
  },
  WARN: {
    color: 'text-warn',
    bg: 'bg-warn-soft',
    icon: AlertTriangle
  },
  ERROR: {
    color: 'text-danger',
    bg: 'bg-danger-soft',
    icon: XCircle
  },
};

// 日志级别列表
export const LOG_LEVEL_LIST = ['DEBUG', 'INFO', 'WARN', 'ERROR'];
