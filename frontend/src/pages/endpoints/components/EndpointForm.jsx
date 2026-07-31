// ============================================
// 端点编辑表单组件 (v5.0+ SQLite 存储模式)
// 2025-12-05
// ============================================

import { useRef, useState } from 'react';
import { X, Save, AlertCircle, ChevronDown, ChevronUp, Eye, EyeOff, Plus, Trash2 } from 'lucide-react';
import { Button } from '@components/ui';
import useModalLifecycle from '@hooks/useModalLifecycle.js';
import {
  createEmptyEndpointModelRewriteRule,
  parseEndpointModelRewriteSettings,
  serializeEndpointModelRewriteRules
} from '../utils/modelRewrite.js';

// ============================================
// 表单输入组件
// ============================================

const FormInput = ({ label, name, value, onChange, type = 'text', placeholder, required, disabled, help }) => (
  <div className="space-y-1">
    <label className="block text-sm font-medium text-slate-700">
      {label}
      {required && <span className="text-rose-500 ml-1">*</span>}
    </label>
    <input
      type={type}
      name={name}
      value={value || ''}
      onChange={onChange}
      placeholder={placeholder}
      disabled={disabled}
      className={`
        w-full px-3 py-2 border border-slate-200 rounded-lg text-sm
        focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500
        disabled:bg-slate-50 disabled:text-slate-400
        ${disabled ? 'cursor-not-allowed' : ''}
      `}
    />
    {help && <p className="text-xs text-slate-400">{help}</p>}
  </div>
);

// 密码输入组件（带显示/隐藏切换）
const PasswordInput = ({ label, name, value, onChange, placeholder, required, help }) => {
  const [showPassword, setShowPassword] = useState(false);

  return (
    <div className="space-y-1">
      <label className="block text-sm font-medium text-slate-700">
        {label}
        {required && <span className="text-rose-500 ml-1">*</span>}
      </label>
      <div className="relative">
        <input
          type={showPassword ? 'text' : 'password'}
          name={name}
          value={value || ''}
          onChange={onChange}
          placeholder={placeholder}
          className="w-full px-3 py-2 pr-10 border border-slate-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500"
        />
        <button
          type="button"
          onClick={() => setShowPassword(!showPassword)}
          className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-slate-400 hover:text-slate-600 transition-colors"
          title={showPassword ? '隐藏' : '显示'}
        >
          {showPassword ? <EyeOff size={18} /> : <Eye size={18} />}
        </button>
      </div>
      {help && <p className="text-xs text-slate-400">{help}</p>}
    </div>
  );
};

const FormCheckbox = ({ label, name, checked, onChange, help }) => (
  <div className="flex items-start gap-3">
    <input
      type="checkbox"
      name={name}
      checked={checked || false}
      onChange={onChange}
      className="mt-1 w-4 h-4 text-indigo-600 border-slate-300 rounded focus:ring-indigo-500"
    />
    <div>
      <label className="text-sm font-medium text-slate-700">{label}</label>
      {help && <p className="text-xs text-slate-400">{help}</p>}
    </div>
  </div>
);

// ============================================
// 端点表单组件
// ============================================

const EndpointForm = ({
  endpoint = null,  // null = 新建模式, object = 编辑模式
  onSave,
  onCancel,
  loading = false
}) => {
  const isEditMode = !!endpoint;

  // 计算初始表单数据
  const getInitialFormData = () => {
    if (endpoint) {
      const modelRewriteSettings = parseEndpointModelRewriteSettings(endpoint.modelRewriteRules || '');
      return {
        channel: endpoint.channel || '',
        name: endpoint.name || '',
        url: endpoint.url || '',
        token: endpoint.token || '', // v5.0: 本地桌面应用，直接显示已保存的 Token
        apiKey: endpoint.apiKey || '', // v5.0: 本地桌面应用，直接显示已保存的 ApiKey
        priority: endpoint.priority || 1,
        failoverEnabled: endpoint.failoverEnabled !== false,
        cooldownSeconds: endpoint.cooldownSeconds || '',
        timeoutSeconds: endpoint.timeoutSeconds || 300,
        supportsCountTokens: endpoint.supportsCountTokens || false,
        modelRewriteEnabled: modelRewriteSettings.enabled,
        modelRewriteRules: modelRewriteSettings.rules,
        costMultiplier: endpoint.costMultiplier || 1.0,
        inputCostMultiplier: endpoint.inputCostMultiplier || 1.0,
        outputCostMultiplier: endpoint.outputCostMultiplier || 1.0,
        cacheCreationCostMultiplier: endpoint.cacheCreationCostMultiplier || 1.0,
        cacheCreationCostMultiplier1h: endpoint.cacheCreationCostMultiplier1h || 1.0,
        cacheReadCostMultiplier: endpoint.cacheReadCostMultiplier || 1.0
      };
    }
    return {
      channel: '',
      name: '',
      url: '',
      token: '',
      apiKey: '',
      priority: 1,
      failoverEnabled: true,
      cooldownSeconds: '',
      timeoutSeconds: 300,
      supportsCountTokens: false,
      modelRewriteEnabled: false,
      modelRewriteRules: [createEmptyEndpointModelRewriteRule()],
      costMultiplier: 1.0,
      inputCostMultiplier: 1.0,
      outputCostMultiplier: 1.0,
      cacheCreationCostMultiplier: 1.0,
      cacheCreationCostMultiplier1h: 1.0,
      cacheReadCostMultiplier: 1.0
    };
  };

  // 表单状态
  const [formData, setFormData] = useState(getInitialFormData);

  // 高级选项展开状态
  const [showAdvanced, setShowAdvanced] = useState(false);

  // 表单错误
  const [errors, setErrors] = useState({});

  // 挂载即打开（父组件条件渲染），卸载时由 hook 清理
  const closeButtonRef = useRef(null);
  const handleRequestClose = () => {
    if (!loading) onCancel();
  };
  useModalLifecycle({
    open: true,
    onClose: handleRequestClose,
    initialFocusRef: closeButtonRef
  });

  // 处理输入变化
  const handleChange = (e) => {
    const { name, value, type, checked } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: type === 'checkbox' ? checked : value
    }));
    // 清除对应字段的错误
    if (errors[name]) {
      setErrors(prev => ({ ...prev, [name]: null }));
    }
  };

  const updateModelRewriteRules = (updater) => {
    setFormData(prev => {
      const current = Array.isArray(prev.modelRewriteRules) && prev.modelRewriteRules.length > 0
        ? prev.modelRewriteRules
        : [createEmptyEndpointModelRewriteRule()];
      return {
        ...prev,
        modelRewriteRules: updater(current)
      };
    });
    if (errors.modelRewriteRules) {
      setErrors(prev => ({ ...prev, modelRewriteRules: null }));
    }
  };

  // 表单验证
  const validateForm = () => {
    const newErrors = {};

    if (!formData.channel.trim()) {
      newErrors.channel = '请输入渠道名称';
    }
    if (!formData.name.trim()) {
      newErrors.name = '请输入端点名称';
    }
    if (!formData.url.trim()) {
      newErrors.url = '请输入端点 URL';
    } else if (!/^https?:\/\/.+/.test(formData.url)) {
      newErrors.url = '请输入有效的 URL (以 http:// 或 https:// 开头)';
    }
    if (!isEditMode && !formData.token.trim()) {
      newErrors.token = '请输入 Token';
    }
    if (formData.modelRewriteEnabled) {
      const rules = Array.isArray(formData.modelRewriteRules) ? formData.modelRewriteRules : [];
      if (!rules.length) {
        newErrors.modelRewriteRules = '请至少添加一条模型改写规则';
      } else if (rules.some((rule) => !String(rule.source || '').trim() || !String(rule.target || '').trim())) {
        newErrors.modelRewriteRules = '请完整填写每条规则的来源模型和目标模型';
      } else if (rules.some((rule) => String(rule.source || '').trim().toLowerCase() === String(rule.target || '').trim().toLowerCase())) {
        newErrors.modelRewriteRules = '来源模型和目标模型不能相同';
      }
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  // 提交表单
  const handleSubmit = async (e) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    try {
      await onSave({
        ...formData,
        modelRewriteRules: formData.modelRewriteEnabled
          ? serializeEndpointModelRewriteRules(formData.modelRewriteRules)
          : ''
      });
    } catch (error) {
      console.error('保存失败:', error);
      setErrors({ submit: error.message || '保存失败' });
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-start justify-center z-50 animate-fade-in pt-[15vh]">
      <div
        role="dialog"
        aria-modal="true"
        aria-label={isEditMode ? '编辑端点' : '新建端点'}
        className="bg-white rounded-2xl shadow-xl w-full max-w-2xl max-h-[75vh] flex flex-col overflow-hidden"
      >
        {/* 标题栏 */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-100 flex-shrink-0">
          <h2 className="text-lg font-semibold text-slate-900">
            {isEditMode ? '编辑端点' : '新建端点'}
          </h2>
          <button
            ref={closeButtonRef}
            aria-label="关闭表单"
            onClick={handleRequestClose}
            className="p-2 text-slate-400 hover:text-slate-600 hover:bg-slate-100 rounded-lg transition-colors"
          >
            <X size={20} />
          </button>
        </div>

        {/* 表单内容 - 可滚动区域 */}
        <form onSubmit={handleSubmit} className="flex flex-col flex-1 overflow-hidden">
          <div className="p-6 overflow-y-auto flex-1">
          {/* 提交错误提示 */}
          {errors.submit && (
            <div className="mb-4 p-3 bg-rose-50 border border-rose-200 rounded-lg flex items-center gap-2 text-rose-700 text-sm">
              <AlertCircle size={16} />
              {errors.submit}
            </div>
          )}

          {/* 基本信息 */}
          <div className="space-y-4 mb-6">
            <h3 className="text-sm font-semibold text-slate-500 uppercase tracking-wide">
              基本信息
            </h3>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <FormInput
                  label="渠道"
                  name="channel"
                  value={formData.channel}
                  onChange={handleChange}
                  placeholder="e.g. official, backup"
                  required
                  help="用于分组展示端点"
                />
                {errors.channel && (
                  <p className="text-xs text-rose-500 mt-1">{errors.channel}</p>
                )}
              </div>

              <div>
                <FormInput
                  label="端点名称"
                  name="name"
                  value={formData.name}
                  onChange={handleChange}
                  placeholder="e.g. api-primary"
                  required
                  disabled={isEditMode}
                  help={isEditMode ? '名称不可修改' : '唯一标识符'}
                />
                {errors.name && (
                  <p className="text-xs text-rose-500 mt-1">{errors.name}</p>
                )}
              </div>
            </div>

            <div>
              <FormInput
                label="URL"
                name="url"
                value={formData.url}
                onChange={handleChange}
                placeholder="https://api.example.com"
                required
              />
              {errors.url && (
                <p className="text-xs text-rose-500 mt-1">{errors.url}</p>
              )}
            </div>
          </div>

          {/* 认证信息 */}
          <div className="space-y-4 mb-6">
            <h3 className="text-sm font-semibold text-slate-500 uppercase tracking-wide">
              认证信息
            </h3>

            <div>
              <PasswordInput
                label="Token"
                name="token"
                value={formData.token}
                onChange={handleChange}
                placeholder="sk-..."
                required={!isEditMode}
                help="Bearer Token 认证。清空后保存将保留原值"
              />
              {errors.token && (
                <p className="text-xs text-rose-500 mt-1">{errors.token}</p>
              )}
            </div>

            <div>
              <PasswordInput
                label="API Key (可选)"
                name="apiKey"
                value={formData.apiKey}
                onChange={handleChange}
                placeholder="可选的 API Key"
                help="备用认证方式。清空后保存将保留原值"
              />
            </div>
          </div>

          {/* 路由配置 */}
          <div className="space-y-4 mb-6">
            <h3 className="text-sm font-semibold text-slate-500 uppercase tracking-wide">
              路由配置
            </h3>

            <div className="grid grid-cols-3 gap-4">
              <FormInput
                label="优先级"
                name="priority"
                value={formData.priority}
                onChange={handleChange}
                type="number"
                placeholder="1"
                help="数字越小优先级越高"
              />

              <FormInput
                label="超时时间 (秒)"
                name="timeoutSeconds"
                value={formData.timeoutSeconds}
                onChange={handleChange}
                type="number"
                placeholder="300"
              />

              <FormInput
                label="冷却时间 (秒)"
                name="cooldownSeconds"
                value={formData.cooldownSeconds}
                onChange={handleChange}
                type="number"
                placeholder="使用全局配置"
                help="留空使用全局配置"
              />
            </div>

            <div className="flex gap-6">
              <FormCheckbox
                label="参与故障转移"
                name="failoverEnabled"
                checked={formData.failoverEnabled}
                onChange={handleChange}
                help="当此端点不可用时自动切换"
              />

              <FormCheckbox
                label="支持 count_tokens"
                name="supportsCountTokens"
                checked={formData.supportsCountTokens}
                onChange={handleChange}
                help="端点是否支持 Token 计数 API"
              />
            </div>
          </div>

          {/* 模型兼容改写 */}
          <div className="space-y-4 mb-6">
            <h3 className="text-sm font-semibold text-slate-500 uppercase tracking-wide">
              模型兼容
            </h3>

            <label className="flex items-start gap-3 text-sm text-slate-700">
              <input
                type="checkbox"
                checked={Boolean(formData.modelRewriteEnabled)}
                onChange={(event) => {
                  const enabled = event.target.checked;
                  setFormData(prev => ({
                    ...prev,
                    modelRewriteEnabled: enabled,
                    modelRewriteRules: Array.isArray(prev.modelRewriteRules) && prev.modelRewriteRules.length > 0
                      ? prev.modelRewriteRules
                      : [createEmptyEndpointModelRewriteRule()]
                  }));
                  if (errors.modelRewriteRules) {
                    setErrors(prev => ({ ...prev, modelRewriteRules: null }));
                  }
                }}
                className="mt-1 w-4 h-4 text-indigo-600 border-slate-300 rounded focus:ring-indigo-500"
              />
              <span>
                <span className="block font-medium">启用模型兼容改写</span>
                <span className="block text-xs text-slate-400 mt-0.5">
                  将 Claude Code 请求中的模型名精确替换为当前端点支持的模型
                </span>
              </span>
            </label>

            {formData.modelRewriteEnabled && (
              <div className="p-4 bg-slate-50 rounded-lg space-y-3">
                {(Array.isArray(formData.modelRewriteRules) && formData.modelRewriteRules.length > 0
                  ? formData.modelRewriteRules
                  : [createEmptyEndpointModelRewriteRule()]
                ).map((rule, index, rules) => (
                  <div key={index} className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_1fr_auto]">
                    <FormInput
                      label={`来源模型${rules.length > 1 ? ` ${index + 1}` : ''}`}
                      value={rule.source || ''}
                      onChange={(event) => updateModelRewriteRules(current => current.map((item, itemIndex) => (
                        itemIndex === index ? { ...item, source: event.target.value } : item
                      )))}
                      placeholder="claude-sonnet-4-5"
                    />
                    <FormInput
                      label={`目标模型${rules.length > 1 ? ` ${index + 1}` : ''}`}
                      value={rule.target || ''}
                      onChange={(event) => updateModelRewriteRules(current => current.map((item, itemIndex) => (
                        itemIndex === index ? { ...item, target: event.target.value } : item
                      )))}
                      placeholder="provider-sonnet"
                    />
                    <div>
                      <div className="mb-1 text-sm font-medium text-transparent select-none" aria-hidden="true">删除</div>
                      <button
                        type="button"
                        aria-label={`删除模型改写规则 ${index + 1}`}
                        title="删除规则"
                        disabled={rules.length <= 1}
                        onClick={() => updateModelRewriteRules(current => current.filter((_, itemIndex) => itemIndex !== index))}
                        className={`inline-flex h-[38px] w-10 items-center justify-center rounded-lg border transition-colors ${
                          rules.length <= 1
                            ? 'cursor-not-allowed border-slate-100 text-slate-300'
                            : 'border-slate-200 text-slate-400 hover:border-rose-200 hover:bg-rose-50 hover:text-rose-500'
                        }`}
                      >
                        <Trash2 size={15} />
                      </button>
                    </div>
                  </div>
                ))}

                <button
                  type="button"
                  onClick={() => updateModelRewriteRules(rules => [...rules, createEmptyEndpointModelRewriteRule()])}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-600 transition-colors hover:border-indigo-200 hover:bg-indigo-50 hover:text-indigo-600"
                >
                  <Plus size={15} />
                  添加规则
                </button>

                {errors.modelRewriteRules && (
                  <p className="text-xs text-rose-500">{errors.modelRewriteRules}</p>
                )}
                <p className="text-xs leading-5 text-slate-500">
                  仅在 <code>/v1/messages</code> 与 <code>/v1/messages/count_tokens</code> 转发前生效；所有规则均为精确匹配，且按顺序使用第一条命中规则。
                </p>
              </div>
            )}
          </div>

          {/* 高级选项（可折叠） */}
          <div className="mb-6">
            <button
              type="button"
              onClick={() => setShowAdvanced(!showAdvanced)}
              className="flex items-center gap-2 text-sm font-semibold text-slate-500 uppercase tracking-wide hover:text-slate-700 transition-colors"
            >
              成本倍率配置
              {showAdvanced ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
            </button>

            {showAdvanced && (
              <div className="mt-4 p-4 bg-slate-50 rounded-lg space-y-4">
                <p className="text-xs text-slate-500 mb-3">
                  成本倍率用于调整不同端点的计费比例，默认为 1.0
                </p>

                <div className="grid grid-cols-2 gap-4">
                  <FormInput
                    label="总成本倍率"
                    name="costMultiplier"
                    value={formData.costMultiplier}
                    onChange={handleChange}
                    type="number"
                    step="0.1"
                    placeholder="1.0"
                  />

                  <FormInput
                    label="输入成本倍率"
                    name="inputCostMultiplier"
                    value={formData.inputCostMultiplier}
                    onChange={handleChange}
                    type="number"
                    step="0.1"
                    placeholder="1.0"
                  />

                  <FormInput
                    label="输出成本倍率"
                    name="outputCostMultiplier"
                    value={formData.outputCostMultiplier}
                    onChange={handleChange}
                    type="number"
                    step="0.1"
                    placeholder="1.0"
                  />

                  <FormInput
                    label="缓存读取成本倍率"
                    name="cacheReadCostMultiplier"
                    value={formData.cacheReadCostMultiplier}
                    onChange={handleChange}
                    type="number"
                    step="0.1"
                    placeholder="1.0"
                  />

                  <FormInput
                    label="5分钟缓存创建倍率"
                    name="cacheCreationCostMultiplier"
                    value={formData.cacheCreationCostMultiplier}
                    onChange={handleChange}
                    type="number"
                    step="0.1"
                    placeholder="1.0"
                    help="Claude 默认缓存类型"
                  />

                  <FormInput
                    label="1小时缓存创建倍率"
                    name="cacheCreationCostMultiplier1h"
                    value={formData.cacheCreationCostMultiplier1h}
                    onChange={handleChange}
                    type="number"
                    step="0.1"
                    placeholder="1.0"
                    help="长效缓存类型"
                  />
                </div>
              </div>
            )}
          </div>
          </div>

          {/* 按钮 - 固定在底部 */}
          <div className="flex justify-end gap-3 px-6 py-4 border-t border-slate-100 bg-white flex-shrink-0">
            <Button
              type="button"
              variant="ghost"
              onClick={handleRequestClose}
              disabled={loading}
            >
              取消
            </Button>
            <Button
              type="submit"
              icon={Save}
              loading={loading}
            >
              {isEditMode ? '保存修改' : '创建端点'}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};

export default EndpointForm;
