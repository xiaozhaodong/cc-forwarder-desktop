# Repository Guidelines

## 项目结构与模块组织
- **main.go**：初始化 CLI、加载配置并启动转发器。
- **internal/**：核心业务模块，当前主要包含 endpoint（端点调度）、proxy（请求处理）、transport（HTTP 客户端）、logging、monitor、tracking、service、store、accountauth 等子包，新增功能请按现有分层扩展。
- **internal/accountauth/**：ChatGPT OAuth / refresh token / id_token 解析与账号画像提取。
- **internal/service/account_pool*.go**：账号池业务逻辑，包含单账号测试、画像刷新、quota 拉取与状态回写。
- **internal/store/account_pool.go**：`upstream_accounts` 存储层；账号画像、quota、账号成本倍率字段变更需同步这里与 schema。
- **config/**：示例与运行配置，`config/example.yaml` 为基础模板，生产部署请复制为 `config/config.yaml`。
- **data/** 与 `docs/`：分别存储运行时数据库、设计文档；保证数据目录可写。
- **frontend/src/pages/account-pool/**：Codex 账号池前端页面，展示授权方式、优先级、账号类型、`5h` / `d7`、状态、最近刷新，以及 `api_key` 账号成本倍率配置/展示。
- **tests/**：单元与集成测试分层，`tests/unit/{module}` 与 `tests/integration/request_suspend` 对应主要质量保障。

## 构建、测试与开发命令
- `go build -o cc-forwarder`：构建本地二进制，默认启用当前平台。
- `go run . -config config/config.yaml`：快速本地验证，支持 `--no-tui`。
- `wails dev`：桌面端联调主入口。
- `cd frontend && npm run build`：验证前端与 Wails 绑定改动是否可打包。
- `./scripts/build.sh vX.Y.Z`：多平台交叉编译并打包 dist，生成校验和。
- `./scripts/run_tests.sh` 或 `--unit` / `--integration`：执行测试矩阵并输出 `test_reports`。
- `docker-compose up -d`：启动示例依赖，便于联调 Web 界面与数据库。

## 编码风格与命名规范
- 所有 Go 代码提交前必须执行 `go fmt ./...` 与 `goimports -w`，保持标准格式。
- 导出函数、结构体使用 PascalCase，包内私有标识符使用 camelCase；常量采用 UPPER_SNAKE_CASE。
- 配置、YAML 键保持 snake_case，HTTP 路由遵循 kebab-case，例如 `/api/v1/request-suspend`。
- 注释与文档统一使用中文，必要时补充英文术语缩写以便检索。

## 测试指南
- 单元测试位于 `tests/unit`，命名遵循 `Test{模块}_{功能}_{场景}`；集成测试集中在 `tests/integration/request_suspend`。
- 推荐使用 `./scripts/run_tests.sh --coverage` 生成覆盖率报告，并提交关键变更的覆盖率摘要。
- 并发或边界场景请附加 `go test -race ./...` 执行记录，避免引入数据竞争。
- 新增测试数据统一放置于 `tests/testdata`，避免污染生产配置。
- 如果修改账号池 / OAuth / quota，至少补跑：
  - `go test ./internal/store`
  - `go test ./internal/accountauth`
  - `go test ./internal/service`
  - `go test ./internal/proxy`
  - `go test ./internal/proxy/handlers`
  - `go test ./internal/tracking`
  - `cd frontend && npm run build`

## 提交与合并请求规范
- Git 历史沿用 Conventional Commits，使用 `feat|fix|docs|chore` 等前缀，中文描述具体变化，可在末尾附版本标签例如 `v3.2.1`。
- 每个 PR 需列出变更摘要、测试结论、相关 Issue/需求链接；Web 或 CLI 交互变更请附截图或录屏。
- 在评审前确保 `go build` 与 `./scripts/run_tests.sh --unit` 全部通过，并更新 `CHANGELOG.md` 或文档（若影响用户）。
- 大型重构建议拆分多次提交，并在描述中标注潜在风险与回滚策略。

## 配置与安全提示
- 生产环境建议基于 `config/example.yaml` 创建独立配置，敏感 Token 通过环境变量或外部密钥管理注入，避免直接提交。
- 确保运行时 `usage.db` 所在目录具备备份策略，重要操作前执行离线快照；桌面默认路径遵循 `utils.GetDataDir()`（macOS 通常为 `~/Library/Application Support/CC-Forwarder/data/usage.db`）。
- Web 管理界面默认监听 `0.0.0.0:8010`，对外暴露时请置于受控网络或启用反向代理身份验证。
- `credential_raw` 当前仍可能包含 `refresh_token / access_token / id_token`，排查问题时避免直接输出到日志、截图或文档。

## 账号池与 OAuth 约定
- `upstream_accounts` 现已包含账号画像字段：`plan_type`、`chatgpt_account_id`、`chatgpt_user_id`、`organization_id`。
- `upstream_accounts` 现已包含 quota 字段：`quota_5h_*`、`quota_weekly_*`、`quota_status`、`quota_refreshed_at`。
- `upstream_accounts` 现已包含 6 个账号成本倍率字段：`cost_multiplier`、`input_cost_multiplier`、`output_cost_multiplier`、`cache_creation_cost_multiplier`、`cache_creation_cost_multiplier_1h`、`cache_read_cost_multiplier`。
- `free` 账号只有周额度；前端使用 `d7` 标签显示。
- `api_key` 账号在前端默认显示 `5h / d7 = 无限额`。
- 仅 `provider_type == api_key` 允许自定义成本倍率；其他账号类型倍率固定为 `1.0`。
- `TestUpstreamAccount` 成功时应写回 `last_success_at`；OAuth 账号成功后会自动触发画像刷新。
- `503 no_available_providers` 在账号池测试链路中视为 reachable，在账号池转发链路中应短路透传，不做整池 cooldown。
- `/v1/responses` 的成本计算口径中，`input_tokens` 已包含缓存读；实际输入计费需按 `input_tokens - cache_read_tokens`，避免对 `cache_read_tokens` 重复按输入价计费。
- Claude `/v1/messages` 与 Codex `/v1/responses` 的流式链路均已引入 tail drain；若 terminal event 已确认到达，后续尾部 `context canceled` 不应再落为 `cancelled`。
