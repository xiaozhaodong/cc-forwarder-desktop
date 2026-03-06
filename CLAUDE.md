# CLAUDE.md

本文件为 Claude Code 提供项目指导信息。

## 项目概述

**CC-Forwarder Desktop** 是一款基于 Wails 的跨平台桌面应用，用于 Claude / OpenAI 请求转发、端点故障转移、请求追踪，以及 ChatGPT Codex 账号池管理。

- 技术栈：Go + Wails v2 + React + Vite + SQLite
- 平台：macOS / Windows / Linux
- 版本号以 `main.go` 与 `wails.json` 为准，不在此文档中硬编码

### 当前核心能力

- 多端点智能路由、健康检查、故障转移与自愈
- 请求生命周期追踪、Token/成本统计、SQLite 持久化
- Wails 前端页面：概览、端点管理、请求追踪、配置、日志、账号池
- ChatGPT OAuth 授权链接生成、回调兑换、RT / id_token 存储
- Codex 账号池：单账号测试、调度、冷却、鉴权失效处理
- 账号画像与额度刷新：
  - `plan_type`
  - `chatgpt_account_id / chatgpt_user_id / organization_id`
  - `quota_5h_* / quota_weekly_* / quota_status / quota_refreshed_at`

## 关键文件

### 后端

- `main.go`：程序入口与版本号
- `app.go`：Wails App 初始化与服务装配
- `app_api_account_pool.go`：账号池 Wails API
- `app_api_openai_oauth.go`：ChatGPT OAuth API
- `internal/accountauth/openai_profile.go`：OAuth 画像解析
- `internal/accountauth/openai_refresh_token.go`：RT -> AT 刷新与缓存
- `internal/service/account_pool.go`：账号池 CRUD、测试连通性
- `internal/service/account_pool_profile.go`：账号画像与 quota 刷新
- `internal/store/account_pool.go`：`upstream_accounts` 存储层
- `internal/proxy/account_pipeline.go`：账号池转发链路
- `internal/tracking/schema.sql`：SQLite schema
- `internal/tracking/sqlite_adapter.go`：SQLite 迁移逻辑

### 前端

- `frontend/src/pages/account-pool/index.jsx`：账号池页面
- `frontend/src/utils/api.js`：HTTP/Wails 统一 API 封装
- `frontend/src/utils/wailsApi.js`：Wails 绑定适配层
- `frontend/src/wailsjs/go/*`：Wails 生成的前端绑定

## 关键数据流

### ChatGPT OAuth

1. 前端调用 `GenerateChatGPTOAuthLink`
2. 浏览器完成登录并回调
3. 前端调用 `ExchangeChatGPTOAuthCallback`
4. 后端解析 `id_token`，提取账号画像
5. `credential_raw` 存储 RT / AT / id_token / 账号画像字段

### 账号测试与画像刷新

1. 前端点击“测试连通性”
2. 后端 `TestUpstreamAccount` 请求 `/v1/responses` 或 Codex `/backend-api/codex/responses`
3. 成功时写回 `last_success_at`
4. 若为 OAuth 账号，成功后自动触发 `RefreshAccountProfile`
5. `RefreshAccountProfile` 通过 `wham/usage` 更新 quota 字段

### 账号池转发

1. `internal/proxy/account_pipeline.go` 从 store 读取可调度账号
2. 逐个尝试转发
3. `401/403` 标记鉴权失效
4. `429` 或普通 `5xx` 标记瞬时失败并冷却
5. `503 no_available_providers` 直接短路透传，不做整池 failover

## 当前 quota 语义

- `free`：只有周额度，前端展示为 `d7`
- 非 `free`：展示 `5h` 与 `d7`
- `api_key`：前端默认显示 `5h / d7 = 无限额`
- 账号页进度条和数字统一展示“剩余”

## 常用命令

```bash
# Wails 开发
wails dev

# 直接运行 Go 程序
go run . -config config/config.yaml

# 构建
go build ./...
cd frontend && npm run build
```

## 测试建议

日常开发不要默认直接跑全量 `go test ./...`。优先跑修改相关模块。

```bash
# 账号池 / OAuth / quota
go test ./internal/accountauth
go test ./internal/service
go test ./internal/proxy

# 请求追踪 / SQLite
go test ./internal/tracking

# 前端最小检查
cd frontend && npx eslint src/pages/account-pool/index.jsx src/utils/api.js src/utils/wailsApi.js
cd frontend && npm run build
```

如果修改了以下内容，建议至少补跑对应测试：

- `internal/service/account_pool*.go`：`go test ./internal/service`
- `internal/proxy/account_pipeline.go`：`go test ./internal/proxy`
- `internal/accountauth/*`：`go test ./internal/accountauth`
- `internal/tracking/schema.sql` / `sqlite_adapter.go`：`go test ./internal/tracking`

## 调试提示

- 请求追踪：用 `req-xxxxxxxx` 过滤日志
- 账号池问题优先看：
  - `last_success_at`
  - `quota_status`
  - `quota_refreshed_at`
  - `state`
- OAuth 画像问题优先看：
  - `credential_raw` 中的 `id_token`
  - `plan_type / chatgpt_account_id / chatgpt_user_id / organization_id`
- 前端账号页问题优先看：
  - `frontend/src/pages/account-pool/index.jsx`
  - `frontend/src/utils/api.js`
  - `frontend/src/utils/wailsApi.js`
