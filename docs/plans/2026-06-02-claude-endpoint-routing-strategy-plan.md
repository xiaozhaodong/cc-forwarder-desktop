# Claude 端点路由与手动切换策略设计计划

- 日期: 2026-06-02
- 状态: 可执行计划（V1.1，待实现）
- 版本: V1.1
- 适用范围: Claude `/v1/messages`、`/v1/messages/count_tokens` 及 Claude 端点链路
- 不适用范围: Codex `/v1/responses` 账号池链路

## 修订记录

| 版本 | 日期 | 修改要点 |
| --- | --- | --- |
| V0 | 2026-06-02 | 初稿，列出失败分类、断路器、能力画像、手动 override 等主体设计。 |
| V1 | 2026-06-02 | 1) 统一章节 1 与 6.4 的优先级层级；2) 补 manual override 并发读写语义与 group 兼容；3) 补 half_open 探测代价与失败处理；4) 补 capability 自动学习的 TTL 与失效；5) 第一版数据模型改为内存态 + 重启快照；6) 修复 `failure_threshold` 映射陷阱；7) 明确 `mode` 与 `strategy` 的关系；8) 补 fingerprint 示例与 Manual Fixed 错误体；9) 进一步收敛第一版实施范围。 |
| V1.1 | 2026-06-02 | 1) 第一版补"最小负向命中缓存"，避免 4 类不计入断路器的失败循环命中同一坏端点；2) 明确第一版同时改造 `TriggerRequestFailover` 与 `executeSelectionFailover` 两条切组路径；3) 14.1 与 22.2 冲突修正，第一版只保留"负向缓存"，完整 capability 推迟到 V2；4) 9.0 改 count_tokens 失败分流规则，子能力失败不再传染 messages 路由；5) 18.4 修正 `X-Route-*` 头表述，路由诊断改走 `request_logs` 与 Wails API。 |

## 0. TL;DR 与导航

本文件较长（约 23 章），按用途分三层阅读。

### 0.1 一句话结论

把 Claude 端点路由从“次数驱动失败窗口 + 隐式手动切换”收敛为：
**`manual_override` → `auto_sticky` → `classified_circuit_breaker` → `capability_filter` → `priority/strategy_sort`**
这条层级是路由层不变量。

### 0.2 第一版实施范围（必读）

第一版做 3 件事：

1. **失败分类（最小集合）**：
   - 新增 `client_cancel / model_unsupported / schema_incompatible / payload_too_large` 这 4 类**不计入端点失败窗口**。
   - 其他错误维持现有 `failure_threshold=3 / window=5m` 行为，避免静默语义变化。
2. **Manual override 状态 + 两条切组路径改造**：
   - 引入 `mode = auto | manual_preferred | manual_fixed`。
   - 路由选择第一步读 override。
   - **同时改造**两条自动切组路径，避免 manual_fixed 被绕过：
     - `failover.TriggerRequestFailover`（`internal/endpoint/failover.go:70`）
     - `executeSelectionFailover`（`internal/endpoint/endpoint_selection.go:73 / 454`）
   - 前端增加"恢复自动"和"严格固定"按钮。
3. **最小负向命中缓存**：
   - 4 类不计入断路器的失败仍会写入内存负向缓存（默认 TTL 30m）。
   - 三类键：`endpoint+model` / `endpoint+schema_field` / `endpoint+body_size_bucket`，外加 `endpoint`（count_tokens 不支持）。
   - 避免同模型 / 同 schema / 同体积请求循环命中同一坏端点。
   - 不持久化、不暴露给前端展示，仅作为路由层内部防抖。

第一版**暂不做**：分类评分断路器、half_open 真实请求探测、完整 capability 画像（含滑动统计/自动失效）、新数据库表、复杂诊断抽屉。

### 0.3 长期愿景（参考章节 10/11/14/17）

- 分类评分断路器（章节 10）。
- 端点-模型/端点-schema 能力画像（章节 11）。
- 路由状态持久化与诊断数据（章节 14）。
- 前端 circuit state、fallback reason 等可观测信息（章节 17）。

### 0.4 阅读建议

| 角色 | 建议阅读章节 |
| --- | --- |
| 决定第一版范围 | 0、1、6.4、22 |
| 实现第一版 | 0.2、6、7、9、12、14.1、16、18.4、19.2、19.4、22 |
| 推动长期方案 | 10、11、14.2、15、17、19.3、19.5、19.6 |
| 评审 / 风险评估 | 1、4、5、21、23 |

### 0.5 与 Codex 账号池的关系

本设计不适用于 Codex 链路。Codex `/v1/responses`、`/v1/responses/compact` 走账号池（见 `internal/proxy/account_pipeline.go`），与 Claude 端点路由是两套独立机制。两者唯一共用的是请求追踪与 SQLite 记账。

## 1. 结论

Claude 端点路由应从“路由测试驱动切换”收敛为“真实请求结果驱动断路器”。

**权威路由层级**（路由层不变量）：

```text
manual_override
  ↓
auto_sticky
  ↓
classified_circuit_breaker
  ↓
capability_filter
  ↓
priority / strategy_sort
```

各层级语义：

1. **manual_override**：用户显式选择端点，分 `manual_preferred / manual_fixed` 两档（章节 6.2 / 6.3）。
2. **auto_sticky**：自动模式下保持当前成功端点的粘性，减少抖动（章节 6.1）。
3. **classified_circuit_breaker**：真实 `/v1/messages` 失败按类别进入断路器，不再把所有错误都当成端点整体损坏（章节 9 / 10）。
4. **capability_filter**：模型权限、schema 不兼容、body 过大等问题进入能力画像，而不是粗暴冷却整个端点（章节 11）。
5. **priority / strategy_sort**：最终排序仍按现有策略（priority / fastest 等），不引入新的全局负载均衡。

`GET /v1/models` 这类轻量测试只做展示、排序参考和恢复探测，不直接作为主切换依据（章节 13）。

## 2. 背景

当前 Claude 端点链路已经采用请求级穿透思路：

1. 一个客户端请求通常只打一个上游端点。
2. 端点失败后直接把错误返回给 Claude Code SDK。
3. SDK 下一次重试请求进来时，路由层再根据失败窗口决定是否切换端点。
4. 默认失败窗口为 `5m`，阈值为 `3`，动作为 `failover`。
5. 故障端点进入冷却，默认冷却 `600s`。

这套机制避免了请求内重试导致的重复计费风险，是正确方向。

但当前仍有几个问题：

1. 失败分类粒度不够，`429`、`5xx`、schema `400`、模型权限 `403`、`413`、流式截断等容易混入同一个端点失败窗口。
2. 轻量连通性测试无法代表 Claude `/v1/messages` 的真实可用性。
3. 手动切换与自动 failover 的关系需要明确定义，否则用户手动判断容易被自动策略覆盖。
4. 前端缺少“为什么切换、为什么跳过、为什么没切”的可观测信息。

## 3. 现状代码依据

当前相关落点：

1. `internal/endpoint/failure_tracker.go`
   - 维护端点失败窗口。
   - 成功后清空失败记录。
   - 达到阈值后触发动作。

2. `internal/proxy/retry_manager.go`
   - 根据错误类型决定是否记录到 `FailureTracker`。
   - 当前请求穿透原则写在 `ShouldRetryWithDecision` 中。

3. `internal/endpoint/endpoint_selection.go`
   - 选择端点时跳过达到失败阈值或冷却中的端点。
   - 找到备用端点后执行选择阶段 failover。

4. `internal/endpoint/failover.go`
   - 对失败端点设置冷却。
   - 停用失败端点组并激活新端点。

5. `internal/endpoint/fast_tester.go`
   - `fastest` 策略使用轻量测试排序。
   - 当前 2xx/4xx 都被视为“可达”。

6. `internal/endpoint/health_check.go`
   - 当前健康检查主要是手动/批量连通性测试。
   - 后台定时轮询已经不再作为主调度依据。

## 4. 目标

### 4.1 用户体验目标

1. 用户仍然只使用一个本地端口。
2. 自动模式下，系统能在端点真实故障时自动切换。
3. 手动切换后，系统尊重用户选择，不因为其他端点更快就偷偷切回。
4. 严格固定模式下，用户可以稳定验证某个端点。
5. 请求失败时，用户能看到失败来自哪个端点、属于哪类问题、是否会触发切换。

### 4.2 工程目标

1. 保持请求级穿透，不恢复请求内多端点重试。
2. 失败分类与路由选择分层，减少重复逻辑。
3. 断路器状态与手动 override 只有一个权威来源。
4. 轻量测试、真实请求、手动切换的状态语义清晰隔离。
5. 先用较小改动落地，再逐步扩展能力画像和前端展示。

## 5. 非目标

本轮设计不做以下事项：

1. 不改变 Codex 账号池调度逻辑。
2. 不让 Claude 请求在同一个客户端请求内尝试多个上游端点。
3. 不用 `GET /v1/models` 替代真实 `/v1/messages` 结果。
4. 不做复杂的全局负载均衡、轮询、最少连接、权重随机。
5. 不自动修改用户端点优先级。
6. 不把所有历史请求日志都改造成新的统计表。

## 6. 路由模式

Claude 路由分为三种模式。

### 6.1 Auto

自动模式。

规则：

1. 没有手动 override。
2. 优先使用当前 sticky 端点。
3. sticky 端点不可用时，按候选排序选择下一个端点。
4. 成功请求会刷新 sticky。
5. 失败请求按分类进入断路器。

适合默认日常使用。

### 6.2 Manual Preferred

手动优先模式。

触发方式：

1. 用户在请求页或端点页手动切换到某个端点。
2. 系统记录一个 `manual_preferred` override。

规则：

1. 被手动选择的端点优先参与路由。
2. 不因为其他端点更快、延迟更低、排序更靠前而自动切走。
3. 若手动端点明确不可用，可临时 fallback 到备用端点。
4. fallback 不清除手动 override。
5. 手动端点恢复后，下次请求可回到该端点。

明确不可用包括：

1. 端点 disabled。
2. 端点被删除。
3. 端点处于 `open` 冷却状态。
4. 连接类强故障达到触发条件。
5. 认证失效或 token 不可用。

不应直接 fallback 的情况：

1. 单次普通 `429`。
2. 单次普通 `5xx`。
3. 单次流式质量问题。
4. 请求本身超大导致 `413`。
5. 当前模型不支持，但用户处于严格排障意图时。

这些情况应先按策略记录并返回错误，避免自动逻辑过早覆盖用户选择。

### 6.3 Manual Fixed

严格固定模式。

规则：

1. 只使用用户指定端点。
2. 不自动 fallback。
3. 不因断路器、轻量测试、最快策略切到其他端点。
4. 若指定端点不可用，直接返回错误给客户端。
5. 可继续记录失败事件，但不让该事件改变本次路由目标。

适用场景：

1. 验证某个渠道。
2. 排查 endpoint/schema/model 权限问题。
3. 压测某个端点。
4. 用户明确不希望自动兜底。

### 6.4 模式优先级

`mode` 字段决定 override 类型，与章节 1 的权威路由层级一致：

```text
manual_override (mode = manual_fixed | manual_preferred)
  ↓
auto_sticky (mode = auto)
  ↓
classified_circuit_breaker
  ↓
capability_filter
  ↓
priority / strategy_sort
```

`mode` 取值：

1. `manual_fixed`：占据 manual_override 层，且禁止 fallback。
2. `manual_preferred`：占据 manual_override 层，但允许在严格不可用时 fallback 到候选端点。
3. `auto`：不进入 manual_override 层，从 auto_sticky 开始决策。

`strategy` 字段（参见 `config.Strategy.Type`，如 `priority` / `fastest`）只影响最末层的端点排序，不影响层级顺序。`mode` 与 `strategy` 是正交的：例如 `mode=auto + strategy=fastest` 是合法的，表示自动模式下用 fastest 排序候选端点。

这条层级是路由层不变量，任何代码路径不得跳过更高优先级直接落到下层。

## 7. 手动切换状态模型

建议新增统一的 Claude 路由 override 状态。

第一版可以先放在 settings 表或现有运行态配置中，后续再抽独立表。

建议字段：

```text
mode: auto | manual_preferred | manual_fixed
endpoint_name: string
set_by: user | system
set_at: timestamp
expires_at: timestamp | null
fallback_enabled: bool
fallback_reason: string | null
last_effective_endpoint: string | null
revision: int64
```

字段语义：

1. `mode`
   - 当前路由模式。

2. `endpoint_name`
   - 用户指定端点。
   - `auto` 模式为空。

3. `fallback_enabled`
   - `manual_preferred` 下通常为 true。
   - `manual_fixed` 下必须为 false。

4. `last_effective_endpoint`
   - 最近一次实际使用端点。
   - 用于前端解释“手动端点未被使用”的原因。

5. `revision`
   - 每次写入自增。
   - 用于路由决策做 snapshot 一致性检查（见 7.3）。

### 7.1 清除规则

以下动作会清除手动 override：

1. 用户点击“恢复自动”。
2. 用户删除当前手动端点。
3. 用户禁用当前手动端点，并选择“同时恢复自动”。

以下动作不清除手动 override：

1. 手动端点临时冷却。
2. 手动端点单次请求失败。
3. 手动端点 fallback 到备用端点。
4. 应用重启。

是否跨重启保留：

1. `manual_preferred` 建议跨重启保留。
2. `manual_fixed` 建议跨重启保留，但前端要明显展示。
3. 如果用户担心忘记固定端点，可提供 `expires_at` 或“本次会话固定”。

### 7.2 与 groupManager 的关系

当前 `failover.TriggerRequestFailover`（`internal/endpoint/failover.go:70`）与 `executeSelectionFailover`（`internal/endpoint/endpoint_selection.go:454`）都通过 `groupManager.ManualActivateGroup` 切换组。这是“将端点对应组激活”的硬切，不区分是来自用户还是来自系统。

引入 override 后必须区分两类调用：

1. **用户切换**（来自前端 / Wails API）：
   - 写入 override。
   - 若该端点所属组未激活，仍然激活该组。
   - 记录 `set_by=user`。

2. **系统切换**（来自 `failover.TriggerRequestFailover`）：
   - **不**写入 override。
   - 仅在 `mode=auto` 或 `mode=manual_preferred` 且允许 fallback 时执行组激活。
   - `mode=manual_fixed` 下禁止调用 `ManualActivateGroup` 做切换。
   - 记录 `set_by=system`，仅用于 fallback_reason 展示，不改变 override 当前指向的端点。

为此建议把 `ManualActivateGroup` 改造为接受调用者来源参数，或在调用前由路由层统一判断是否允许调用。

### 7.3 并发读写语义

`override` 全局只有一份（id=1），多个请求会并发读取，前端也可能并发写入。

规则：

1. **读取一致性**：单个请求在路由决策开始时读取一次 override `snapshot`（包含 `revision`），整个决策过程不再重读。这避免“读 override → 计算 candidate → forward”中间被改写。
2. **决策幂等**：路由决策的输出只依赖输入 snapshot 与端点状态快照，不依赖外部时序。
3. **写入语义**：前端写入 override 时只更新本身字段并递增 `revision`，不主动取消正在进行的请求。
4. **正在进行的请求**：保持使用旧 snapshot 完成本次请求，下一次请求才会读到新 override。
5. **fallback 写回**：`fallback_reason` 与 `last_effective_endpoint` 由路由层异步写回，写回时不递增 `revision`，避免触发其他请求重新评估。

第一版可以接受最简实现：override 用 `sync.RWMutex` 保护，写入即生效，正在进行的请求保留旧值。`revision` 字段仅做日志/审计，不做并发栅栏。

### 7.4 持久化粒度

- `mode / endpoint_name / set_by / set_at / expires_at / fallback_enabled` 是用户/系统显式操作，立即持久化。
- `fallback_reason / last_effective_endpoint` 是高频运行态，建议只保留在内存，前端通过 `GetClaudeRoutingState` 读取，不每次写入 DB。

## 8. 候选端点过滤

请求进入后，先构造请求画像。

请求画像字段：

```text
path
model
stream
body_size
has_context_management
has_cache_control_scope
anthropic_beta_headers
client_request_id
```

候选过滤顺序：

1. 端点必须 enabled。
2. 端点必须存在有效 URL。
3. 端点必须不处于 `open` 冷却状态。
4. 若为 `manual_fixed`，只允许手动端点进入候选。
5. 若为 `manual_preferred`，先评估手动端点，再评估 fallback 候选。
6. 若能力画像显示明确不支持本次请求，应跳过或返回能力不匹配说明。

## 9. 失败分类

失败分类是本设计的核心。

### 9.0 失败窗口与请求 path 的关系

`/v1/messages` 与 `/v1/messages/count_tokens` 的失败语义不同：

| Path | 是否计费 | 是否流式 | 错误信号意义 |
| --- | --- | --- | --- |
| `/v1/messages` | 是 | 可能 | 端点真实可用性 |
| `/v1/messages/count_tokens` | 否 | 否 | 仅代表端点的子能力是否完整，不代表 `/v1/messages` 完全不可用 |

`Endpoint` 配置中已经存在 `supports_count_tokens` 字段，count_tokens 子能力问题不应传染到 `/v1/messages` 的路由决策。

#### 9.0.1 第一版规则（按错误分类分流）

count_tokens 的失败分类后再决定是否计入端点失败窗口：

| count_tokens 失败分类 | 是否计入 `/v1/messages` 端点失败窗口 | 额外动作 |
| --- | --- | --- |
| `connection_failure / connection_timeout` | 计入 | 与 `/v1/messages` 等同处理 |
| `auth_error` | 计入 | 与 `/v1/messages` 等同处理 |
| `response_timeout / server_error / rate_limit` | 计入（权重折半） | 因为可能也影响 messages |
| `model_unsupported / schema_incompatible / payload_too_large` | **不**计入 | 写入"最小负向命中缓存"`endpoint+model/schema_field`（章节 14.1） |
| `count_tokens_unsupported`（404 / 405 / Not Implemented） | **不**计入 | 标记 `supports_count_tokens=false`，后续 count_tokens 请求跳过该端点，但不影响 messages |
| `client_cancel` | 不计入 | 与现状一致 |

这一规则确保：

1. 端点连接坏、auth 坏、整体不可用 → count_tokens 失败也会促使路由切换。
2. 端点的 count_tokens 子能力不完整（如某些渠道根本没实现这个 path） → 只影响 count_tokens 选路，不冷却整个端点。

#### 9.0.2 第二版（V2）

观测稳定后，可考虑为 count_tokens 维护独立失败窗口（更精确，但实现成本高）。第一版以共用窗口 + 分类分流为准。

### 9.1 分类总览

| 分类 | 示例 | 是否计入端点断路器 | 默认权重 | 默认行为 |
| --- | --- | --- | --- | --- |
| `connection_failure` | DNS、TLS、connect refused、connection reset before response | 是 | 3 | 快速短冷却 |
| `connection_timeout` | 建连超时 | 是 | 3 | 快速短冷却 |
| `response_timeout` | 等待响应头超时 | 是 | 2 | 计入窗口 |
| `rate_limit` | 429 | 是 | 1 | 软失败，尊重 Retry-After |
| `server_error` | 500/502/503/504 | 是 | 1 | 软失败 |
| `auth_error` | 401/403 token 无效 | 是 | 3 | 长冷却或人工处理 |
| `model_unsupported` | model_not_found、无权访问某模型 | 不打坏整个端点 | 0 | 记录端点-模型能力 |
| `schema_incompatible` | 字段 rejected、Extra inputs are not permitted | 不打坏整个端点 | 0 | 记录端点-schema 能力 |
| `payload_too_large` | 413 | 不打坏整个端点 | 0 | 记录 size 限制 |
| `stream_quality` | incomplete_stream、stream_truncated | 是，低权重 | 0.5 | 降级，多次才冷却 |
| `client_cancel` | 499、下游断开 | 否 | 0 | 忽略 |
| `client_error` | 普通 4xx 请求问题 | 否 | 0 | 透传 |

### 9.2 连接类强故障

连接类强故障表示“请求可能尚未被上游业务处理”，适合快速避开。

包括：

1. DNS 解析失败。
2. TLS 握手失败。
3. connect timeout。
4. connection refused。
5. 建连前网络错误。

建议规则：

1. 单次权重为 `3`。
2. 默认阈值为 `3`，因此一次即可打开断路器。
3. 默认冷却为 `90s`。
4. 冷却到期进入 `half_open`。

### 9.3 响应超时

响应超时可能已经被上游处理，重打容易重复计费，因此仍然请求穿透。

建议规则：

1. 单次权重为 `2`。
2. 两次以内触发断路器。
3. 默认冷却为 `120s`。
4. 前端提示“等待响应头超时，可能是上游排队或不可用”。

### 9.4 429

429 可能是：

1. 全局限流。
2. 单 key 限流。
3. 模型限流。
4. 上游把不可用包装成 429。

建议规则：

1. 单次权重为 `1`。
2. `5m` 内累计 3 次后触发冷却。
3. 优先使用上游 `Retry-After`。
4. 无 `Retry-After` 时默认冷却 `180s`。
5. 如果错误体能识别为模型级限流，应记录 `endpoint + model` 级别，不直接打坏整个端点。

模型级限流的 fingerprint 识别（仅作示例，实际需要按 fingerprint 库迭代）：

```text
- HTTP 429 + body 含 "rate_limit_exceeded" 且包含 "model"
- HTTP 429 + body 含 "tokens per minute" / "TPM"
- HTTP 429 + body 含 "requests per day" / "daily quota"
- HTTP 429 + body 含中文 "模型" / "额度" / "限流"
- 上游为 anyrouter/cc-forwarder 等聚合端点时，错误体可能是 JSON 嵌套 `error.code = "model_rate_limit"`
```

不能匹配模型级 fingerprint 的 429 默认归入端点级 `rate_limit`。fingerprint 库建议维护在 `internal/endpoint/failure_classifier.go` 中，通过单元测试守住已知错误体格式。

### 9.5 5xx

普通 5xx 作为软失败。

建议规则：

1. 单次权重为 `1`。
2. `5m` 内累计 3 次后触发冷却。
3. 默认冷却 `120s`。
4. 如果错误体包含 `model_not_found`、`No available channel for model`，改分类为 `model_unsupported`。

### 9.6 认证与权限

401/403 不一定都是端点整体坏。

拆分规则：

1. token 无效、账号无权限、认证失败：
   - 分类为 `auth_error`。
   - 长冷却或标记需人工处理。

2. 某模型无权限：
   - 分类为 `model_unsupported`。
   - 写入端点-模型能力画像。
   - 不冷却整个端点。

3. 上游项目权限不足：
   - 如果错误明确绑定某模型，按模型能力处理。
   - 如果是全局权限不足，按 `auth_error` 处理。

### 9.7 schema 不兼容

示例：

1. `context_management: Extra inputs are not permitted`
2. `cache_control.scope` 不支持
3. beta header 不支持

判定 fingerprint（参考，需要随聚合端点的错误体格式扩展）：

```text
- HTTP 400/422 + body 含 "Extra inputs are not permitted"
- HTTP 400/422 + body 含 "context_management"
- HTTP 400 + body 含 "cache_control" 与 "scope"
- HTTP 400 + body 含 "anthropic-beta" 与 "not supported"
- HTTP 400 + body 含中文 "字段不支持" / "参数不允许" + 字段名命中已知 schema 列表
```

非 Claude 官方端点的错误体可能是 OpenAI 风格、中文混合、自定义 JSON 结构。`failure_classifier` 必须对未命中 fingerprint 的 400/422 默认归为 `client_error`（章节 9.1），不强行打坏端点。

建议规则：

1. 分类为 `schema_incompatible`。
2. 不打坏整个端点。
3. 记录端点能力限制。
4. 如已有 sanitizer，可在转发前处理（参见 `internal/proxy/handlers/forwarder.go` 的 `cache_control.scope` sanitizer 已落地）。
5. 若无法安全 sanitizer，则在能力过滤阶段跳过该端点。

### 9.8 流式质量问题

流式中途断开不应和连接失败同权。同时必须与已落地的 tail drain 机制（见 `internal/proxy/stream_processor.go`）配合，避免重复判定。

#### 9.8.1 判定时机

按"客户端是否断开 + 是否拿到 terminal event"切分：

| 客户端是否断开 | terminal event 是否到达 | 分类 | 说明 |
| --- | --- | --- | --- |
| 否 | 是 | success | 正常完成 |
| 否 | 否（上游中途断流） | `stream_quality` | 真正的流式截断 |
| 是 | tail drain 后到达 | success | 客户端先断，但上游补齐了 terminal |
| 是 | tail drain 后仍未到达 | `stream_quality`（低权重） | 上游补齐失败 |
| 是 | terminal 之前断开，且没有 drain 机会 | `client_cancel` | 计入 9.1 表的客户端取消，不打坏端点 |

实现要点：

1. `stream_quality` 的判定必须在 tail drain 完成之后做，不在流中途下结论。
2. tail drain 成功的请求一律算 success，不再二次打分。
3. 客户端断开但 terminal 已到达的请求，不算端点失败。

#### 9.8.2 权重与窗口

1. `stream_truncated`、`incomplete_stream` 计入 `stream_quality`。
2. 成功但有质量问题时，不清空所有失败窗口，只清空强故障窗口。
3. 多次流式质量问题后标记 `degraded`。
4. `degraded` 端点仍可使用，但排序降低。

## 10. 断路器状态

每个端点维护一个断路器状态。

### 10.1 状态定义

```text
closed
open
half_open
degraded
```

说明：

1. `closed`
   - 正常参与路由。

2. `open`
   - 冷却中。
   - 普通请求不选。

3. `half_open`
   - 冷却到期。
   - 允许一次恢复探测或一次真实请求。

4. `degraded`
   - 可用但近期有质量或软失败问题。
   - 参与路由但排序降低。

### 10.2 评分规则

建议使用滑动窗口评分，而不是单纯次数。

```text
score =
  connection_failure * 3
+ connection_timeout * 3
+ response_timeout   * 2
+ rate_limit         * 1
+ server_error       * 1
+ stream_quality     * 0.5
```

默认：

```text
window = 5m
open_threshold = 3
degraded_threshold = 2
```

### 10.3 打开规则

当 `score >= open_threshold`：

1. 端点进入 `open`。
2. 设置 `opened_until`。
3. 记录 `opened_reason`。
4. Auto 模式下切到下一候选端点。
5. Manual Preferred 下允许 fallback。
6. Manual Fixed 下不 fallback，只返回错误。

### 10.4 半开恢复

冷却到期后进入 `half_open`。

#### 10.4.1 探测来源

可选探测方式：

1. **轻量探测**（低成本）：
   - 走 `GET /v1/models` 或配置的 `health_path`。
   - 不计费、不流式。
   - 仅用于"快速确认端点恢复可达"。
2. **真实请求探测**（有成本）：
   - 命中该端点的下一次真实 `/v1/messages` 请求。
   - 成功后恢复 `closed`，失败则重新 `open`。
   - 真实请求探测一定计费，无法回滚。

#### 10.4.2 第一版策略（保守）

第一版默认采用"轻量探测优先 + 真实请求兜底"：

1. **Auto 模式**：
   - `half_open` 端点排序在 `closed` 之后。
   - 优先用候选中的 `closed` 端点，避免把真实请求当探测用。
   - 若候选只剩 `half_open`，才允许真实请求探测。
2. **Manual Preferred**：
   - 手动端点为 `half_open` 时，仍优先让该端点接真实请求（用户意图明确）。
   - 探测失败 → 视为 fallback 触发条件之一。
3. **Manual Fixed**：
   - 手动端点为 `half_open` 时，按用户固定意图允许真实请求探测。
   - 失败时直接把错误返回客户端，不切换端点。

#### 10.4.3 探测失败处理

half_open 探测失败必须立即重回 `open`，并使用指数退避，避免抖动：

```text
cooldown_next = min(cooldown_base * 2^failures, cooldown_max)
```

默认值：

```text
cooldown_base   = 该错误分类的默认冷却（章节 9）
cooldown_max    = 30m
failures        = 自 closed 以来的 half_open 探测失败次数
```

`failures` 在 `closed` 恢复后清零，避免历史抖动永久放大冷却。

#### 10.4.4 并发探测处理

如果有 N 个并发请求同时落到 half_open 端点上：

1. 路由层只允许一个请求作为"探测请求"。
2. 其他并发请求按"端点不可用"处理：
   - Auto / Manual Preferred + fallback 允许：fallback 到下一候选端点。
   - Manual Fixed：直接返回 503 / 端点暂不可用错误，不等待探测结果（避免阻塞）。
3. 探测请求完成后：
   - 成功 → 端点回 `closed`，下一批请求正常进入。
   - 失败 → 端点回 `open`，冷却时间指数退避。

并发探测控制建议落在 `route_state` 模块，使用 `sync.Map + CAS` 或 mutex 实现"first request wins"。

#### 10.4.5 计费风险声明

真实请求探测会产生真实计费。设计上必须满足以下不变量：

1. 任何一次 half_open 探测失败，**不**触发请求内重试到其他端点（继续保持请求级穿透原则）。
2. 用户能在前端清楚看到"上一次 X 端点恢复探测失败"，避免静默重复计费。
3. half_open 在 `mode=manual_fixed` 下不可绕过冷却 — 即使用户希望"重试一下试试"，应通过显式"清除冷却"按钮操作，而不是路由层暗中重试。

## 11. 能力画像

能力画像用于解决“端点不是坏，只是不适合某类请求”的问题。

### 11.0 V1 与 V2 的边界（重要）

本章节描述的是**完整 capability 画像**，属于 V2 长期方案。第一版（V1）不实现完整 capability，而是用 `internal/endpoint/route_state.go` 中的**最小负向命中缓存**作为过渡（见章节 14.1）：

| 维度 | V1 最小负向缓存 | V2 完整 capability |
| --- | --- | --- |
| 存储介质 | 内存 map + TTL | `endpoint_capabilities` 表（章节 14.2.3） |
| 学习触发 | 单次 `model_unsupported / schema_incompatible / payload_too_large` 失败即写入 | 滑动统计 + 高置信判定（章节 11.4.3） |
| 字段粒度 | `endpoint+model` / `endpoint+schema_field` / `endpoint+body_size_bucket` | `supports_*` / `*_patterns` / `max_body_bytes` 等结构化字段 |
| TTL | 单一值（默认 24h，可配置） | 各字段不同 TTL（章节 11.4.1） |
| 失效行为 | TTL 到期直接删除，下次请求重试 | TTL 到期降级为 `expired`，半开探测后再清除（章节 11.4.2） |
| 自动失效 | 否（只靠 TTL） | 是（TTL + 滑动统计 + 手动清除） |
| 可观测性 | 仅日志 | 前端诊断抽屉（章节 17.3） |

V1 的负向缓存**只解决"避免循环命中同一坏端点"**这一最小需求，不暴露给前端、不持久化、不做滑动统计。

本章节余下小节（11.1-11.5）描述 V2 的目标形态，作为 V1 → V2 演进的参考。

### 11.1 建议能力项

```text
supports_streaming
supports_context_management
supports_cache_control_scope
supports_count_tokens
supported_model_patterns
unsupported_model_patterns
max_body_bytes
required_headers
blocked_headers
schema_sanitizers
```

### 11.2 能力来源

能力来源分三类：

1. 手动配置。
2. 轻量探测。
3. 真实失败学习。

第一版建议只做真实失败学习 + 少量手动配置。

### 11.3 能力学习规则

示例：

1. `context_management` 被某端点拒绝：
   - 设置 `supports_context_management=false`。
   - 如果已有安全 sanitizer，则后续转发前清理。
   - 如果无 sanitizer，则该端点不处理带该字段的请求。

2. `model_not_found`：
   - 记录 `unsupported_model_patterns`。
   - 仅跳过对应模型请求，不影响其他模型。

3. `413`：
   - 根据 body size 记录 `max_body_bytes` 估计值。
   - 后续超大请求优先选支持更大 body 的端点。

### 11.4 能力失效机制（重要）

聚合端点（如 anyrouter / cc-forwarder）的模型列表是动态的：今天没有的模型，明天可能加上。如果能力学习是永久的，会导致"endpoint × model"组合永远被跳过。

为避免误学习造成永久回避，能力画像必须支持自动失效：

#### 11.4.1 TTL 默认值

| 能力项 | 默认 TTL | 失效后行为 |
| --- | --- | --- |
| `unsupported_model_patterns` | 24h | 重新尝试该端点 + 模型，失败再续命 24h |
| `supports_context_management=false` | 7d | schema 类问题变化较慢，可较长 |
| `supports_cache_control_scope=false` | 7d | 同上 |
| `max_body_bytes` | 永久（直到手动清除） | body size 上限通常是结构性限制 |
| `unsupported_anthropic_beta` | 7d | beta header 名变化较慢 |

TTL 起点为最近一次"学习确认"时间，每次新的失败会重新刷新 TTL。

#### 11.4.2 失效后探测策略

TTL 到期后并不直接清除能力记录，而是降级为"待重试"：

1. 路由层把该 endpoint × 能力组合标记为 `expired`。
2. 在没有更好候选时，允许把该端点重新作为候选（视作 half_open）。
3. 探测成功 → 清除能力记录。
4. 探测失败 → 重新写入能力记录并刷新 TTL。

#### 11.4.3 滑动统计兜底

光有 TTL 不够，还需要"高置信"才学：

1. 单次失败**不**立即写入能力画像。
2. 滑动窗口（默认 `1h`）内累计 2 次以上同类失败，才标记能力问题。
3. 已学习的能力问题，在滑动窗口内若有 1 次成功，立即清除该能力记录。

这一规则在错误体格式不稳定的聚合端点上尤其重要：偶发的 5xx 包装成 `model_not_found`，不应导致整个 endpoint × model 组合被锁死。

#### 11.4.4 手动清除

前端必须提供"清除端点能力画像"按钮，能力画像不是黑盒。手动清除一律重置 TTL 与滑动统计计数。

### 11.5 与 sanitizer 的关系

sanitizer 是能力画像的一个实现手段，不是替代品。

规则：

1. 能安全清理且不改变用户意图的字段，可以 sanitizer。
2. 清理会改变语义的字段，不应静默删除。
3. 每个 sanitizer 必须有端点/channel 边界。
4. sanitizer 行为必须有测试证明其他渠道完整透传。

## 12. 路由选择流程

### 12.1 总流程

```text
request -> build request profile
        -> load manual override
        -> evaluate manual fixed/preferred
        -> build candidate endpoints
        -> apply circuit breaker
        -> apply capability filter
        -> apply sticky
        -> sort candidates
        -> forward to exactly one endpoint
        -> classify result
        -> update route state
        -> return response/error to client
```

### 12.2 Auto 模式流程

```text
1. 如果 sticky endpoint 存在：
   1.1 enabled
   1.2 not open
   1.3 capability matched
   1.4 not manually disabled
   则选择 sticky endpoint。

2. sticky 不可用时：
   2.1 获取所有候选端点。
   2.2 过滤 open/disabled/能力不匹配。
   2.3 closed 优先，half_open 次之，degraded 再次。
   2.4 按 priority 排序。
   2.5 选择第一个。
```

### 12.3 Manual Preferred 流程

```text
1. 获取 manual endpoint。
2. 若 endpoint 可用且能力匹配：
   选择 manual endpoint。

3. 若 endpoint 不可用：
   3.1 记录 fallback reason。
   3.2 从自动候选中选择备用端点。
   3.3 不清除 manual override。

4. 若 endpoint 能力不匹配：
   4.1 若 fallback_enabled=true，选择能力匹配备用端点。
   4.2 若 fallback_enabled=false，直接返回能力不匹配错误。
```

### 12.4 Manual Fixed 流程

```text
1. 获取 manual endpoint。
2. 若 endpoint 不存在或 disabled，直接失败。
3. 若 endpoint open，仍直接失败，不 fallback。
4. 若能力不匹配，直接失败。
5. 只向该 endpoint 转发。
```

## 13. 路由测试定位

### 13.1 轻量连通性测试

轻量测试使用：

1. `GET /v1/models`
2. 或配置的 `health_path`
3. 或配置的 `fast_test_path`

用途：

1. 展示端点是否基本可达。
2. 展示最近延迟。
3. 给 `fastest` 排序提供参考。
4. 给 `half_open` 恢复提供低成本信号。

不用于：

1. 证明模型可用。
2. 证明 schema 兼容。
3. 证明流式稳定。
4. 直接覆盖手动选择。

### 13.2 能力探测

能力探测是更高成本操作。

可选方式：

1. 对指定模型发最小 `/v1/messages` 请求。
2. 使用很小 max_tokens。
3. 可选择是否带 `context_management`、cache_control、beta header。

建议：

1. 第一版不做自动能力探测。
2. 只做手动触发。
3. 后续可在 `half_open` 恢复时低频触发。

## 14. 数据模型建议

### 14.1 第一版最小实现（纯内存 + 重启快照）

第一版**不新增任何数据库表**，所有路由运行态都在内存中维护：

1. 分类失败窗口。
2. 断路器状态。
3. sticky endpoint。
4. **最小负向命中缓存**（替代完整 capability 画像，见 14.1.1）。
5. `fallback_reason / last_effective_endpoint`。

manual override 复用 settings 表（已有），仅持久化以下字段：

```text
mode / endpoint_name / set_by / set_at / expires_at / fallback_enabled
```

#### 14.1.1 最小负向命中缓存（替代 V2 完整 capability）

为了让第一版"4 类不计入断路器"的失败（`model_unsupported / schema_incompatible / payload_too_large / count_tokens_unsupported`）不在下一次同类请求时再次循环命中同一坏端点，引入内存负向缓存：

```go
type NegativeHitCache struct {
    // 三类缓存键，各自独立 TTL
    unsupportedModel        sync.Map // key: endpoint + "|" + model         val: expiresAt
    unsupportedSchemaField  sync.Map // key: endpoint + "|" + schema_field  val: expiresAt
    rejectedBodySizeBucket  sync.Map // key: endpoint + "|" + size_bucket   val: expiresAt
    unsupportedCountTokens  sync.Map // key: endpoint                       val: expiresAt
}
```

缓存键设计：

| 键 | 写入时机 | 命中后行为 |
| --- | --- | --- |
| `endpoint+model` | `model_unsupported` 失败 | 后续同 endpoint × model 请求跳过该端点 |
| `endpoint+schema_field` | `schema_incompatible` 失败，从 fingerprint 提取字段名 | 后续同 endpoint × schema_field 请求跳过该端点 |
| `endpoint+body_size_bucket` | `payload_too_large` 失败，按桶向上取整（如 256K / 512K / 1M / 2M / 4M / 8M / 16M） | 后续 body ≥ 该桶的请求跳过该端点 |
| `endpoint` | `count_tokens_unsupported` 失败 | 后续 count_tokens 请求跳过该端点（不影响 messages） |

#### 14.1.2 负向缓存参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| TTL | `30m` | 到期自动失效，下一个同类请求会重新尝试该端点 |
| 容量上限 | 每类 `1024` 项 | 防止恶意/错误的请求把缓存撑爆，LRU 淘汰 |
| 手动清除 | Wails API `ClearNegativeHitCache(endpoint)` | 用户手动 reset |
| 持久化 | 否 | 重启后从空缓存开始 |

注意：

1. 负向缓存**不**走滑动统计高置信判定，单次失败即写入。理由：第一版不做能力学习，缓存只是"避免立刻再次失败"的最小防抖，30 分钟 TTL 控制误伤窗口。
2. body_size_bucket 取桶向上 — 写入时记录"该端点不接受 ≥ N MB"，命中时按该桶或更大尺寸跳过。
3. 这套缓存不暴露给前端展示。前端只能看到"端点 A 最近一次因 model_not_found 失败"这类请求日志，不显示"缓存中的 endpoint+model 列表"。

V2 启用完整 capability 画像后，本缓存机制由 `endpoint_capabilities` 表 + TTL/滑动统计接管（章节 11.4）。

#### 14.1.3 重启快照

为了让用户在桌面端重启后不丢失"哪些端点正在冷却"这一关键状态，提供轻量快照：

1. 进程退出前（或 graceful shutdown 时）写一次：
   ```text
   { endpoint_name, circuit_state, opened_until, opened_reason }
   ```
   到 `settings` 表的一个 JSON 字段（例如 `claude_routing_state_snapshot`）。
2. 进程启动时一次性读回到内存。
3. `opened_until` 已过期的快照直接丢弃，进入 `closed`。
4. 负向命中缓存**不**进快照，重启后从空缓存开始。

**不在请求路径上写库**。每次失败 / 成功只更新内存中的失败窗口、断路器、负向缓存。这避免 SQLite 写入成为路由热路径瓶颈。

#### 14.1.4 第一版优缺点

优点：

1. 改动小，不引入新表。
2. 不引入路由热路径上的写 IO。
3. 便于策略迭代。

缺点：

1. 桌面端崩溃（非正常退出）会丢失断路器状态 — 但下次请求失败会快速重建，可以接受。
2. 历史失败事件分析能力弱 — 由请求追踪表（`request_logs`）兜底，足以排查具体请求。
3. 负向缓存无持久化，重启后会重新经历一次"试探-失败-写入缓存"过程 — 可接受。

### 14.2 长期持久化模型（V2+，参考）

策略稳定后，若需要长期分析与跨重启完整恢复，再引入下述表。**第一版不实施**。

#### 14.2.1 endpoint_route_state

```sql
CREATE TABLE endpoint_route_state (
    endpoint_name TEXT PRIMARY KEY,
    circuit_state TEXT NOT NULL DEFAULT 'closed',
    opened_until DATETIME,
    opened_reason TEXT,
    degraded_reason TEXT,
    last_success_at DATETIME,
    last_failure_at DATETIME,
    failure_score REAL NOT NULL DEFAULT 0,
    half_open_attempts INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL
);
```

写入策略（避免热路径 IO）：

1. **状态变化时写**：`circuit_state` 切换、进入 / 退出冷却、`half_open_attempts` 自增时。
2. **不在每次失败 / 成功时写** `failure_score` 与 `last_*_at`。这两个字段建议改为内存高频更新 + 周期性 flush（例如每 30s 或退出时）。
3. SQLite 在 WAL 模式下写并发可以接受，但仍应避免单请求路径 2-3 次写。

#### 14.2.2 endpoint_failure_events

```sql
CREATE TABLE endpoint_failure_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT,
    endpoint_name TEXT NOT NULL,
    path TEXT,
    model_name TEXT,
    failure_class TEXT NOT NULL,
    http_status INTEGER,
    error_fingerprint TEXT,
    error_preview TEXT,
    weight REAL NOT NULL DEFAULT 1,
    occurred_at DATETIME NOT NULL
);
```

写入策略：

1. 异步写，不阻塞路由决策。
2. 与现有 `request_logs` 有重叠，可考虑只记录"已被分类为 endpoint failure"的事件，避免重复存储。
3. 建议加索引 `(endpoint_name, occurred_at)` 与 `(failure_class, occurred_at)`。

#### 14.2.3 endpoint_capabilities

```sql
CREATE TABLE endpoint_capabilities (
    endpoint_name TEXT PRIMARY KEY,
    supports_streaming INTEGER,
    supports_context_management INTEGER,
    supports_cache_control_scope INTEGER,
    supports_count_tokens INTEGER,
    supported_model_patterns TEXT,
    unsupported_model_patterns TEXT,
    max_body_bytes INTEGER,
    blocked_headers TEXT,
    schema_sanitizers TEXT,
    source TEXT NOT NULL DEFAULT 'learned',
    learned_at DATETIME,
    expires_at DATETIME,
    updated_at DATETIME NOT NULL
);
```

`learned_at / expires_at` 与章节 11.4 的 TTL 配合，启动时若 `expires_at < now()` 则降级为 `expired`，不直接生效。

#### 14.2.4 claude_routing_override

```sql
CREATE TABLE claude_routing_override (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    mode TEXT NOT NULL DEFAULT 'auto',
    endpoint_name TEXT,
    set_by TEXT NOT NULL DEFAULT 'user',
    set_at DATETIME,
    expires_at DATETIME,
    fallback_enabled INTEGER NOT NULL DEFAULT 1,
    revision INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL
);
```

注意：`fallback_reason / last_effective_endpoint` 不放表里，只在内存。

### 14.3 数据迁移说明

从 V1（内存态）升级到 V2（持久化）时：

1. 新增表的初始数据从内存导入，不需要回填历史。
2. settings 中的 `claude_routing_state_snapshot` 在 V2 上线后停止写入，由 `endpoint_route_state` 接管。
3. settings 中已持久化的 override 字段迁移到 `claude_routing_override` 表，迁移完成后清除 settings 中对应字段。

## 15. 配置建议

### 15.0 `mode` 与 `strategy` 的关系

为避免概念混淆，先明确两个独立维度：

| 维度 | 字段 | 取值 | 作用 |
| --- | --- | --- | --- |
| 路由模式 | `claude_routing.mode` | `auto / manual_preferred / manual_fixed` | 决定章节 1 路由层级的最顶层（是否被 manual override 占据） |
| 排序策略 | `strategy.type` | `priority / fastest / round_robin / ...` | 仅影响最末层的候选端点排序 |

两者**正交组合**：例如 `mode=auto + strategy.type=fastest` 表示自动模式 + 用 fastest 排序。

注意：旧文档中"sticky_priority"等命名是**策略词汇而非模式词汇**，已废弃。新文档统一用 `mode` 表达模式，用 `strategy.type` 表达排序。

### 15.1 新配置块

建议后续引入：

```yaml
claude_routing:
  # 默认路由模式
  default_mode: "auto"          # auto | manual_preferred | manual_fixed
  sticky_success_ttl: "30m"

  manual:
    default_mode_after_switch: "manual_preferred"
    allow_fallback_in_preferred: true
    persist_manual_override: true

  circuit_breaker:
    window: "5m"
    open_threshold: 3
    degraded_threshold: 2
    half_open_max_attempts: 1
    cooldowns:
      connection_failure: "90s"
      connection_timeout: "90s"
      response_timeout: "120s"
      rate_limit_default: "180s"
      server_error: "120s"
      auth_error: "30m"
    half_open_backoff:
      base_multiplier: 2.0
      max: "30m"

  failure_weights:
    connection_failure: 3
    connection_timeout: 3
    response_timeout: 2
    rate_limit: 1
    server_error: 1
    stream_quality: 0.5

  capability_ttl:
    unsupported_model: "24h"
    schema_incompatible: "7d"
    unsupported_anthropic_beta: "7d"
    max_body_bytes: "permanent"

  probes:
    lightweight_path: "/v1/models"
    enable_capability_probe: false
```

### 15.2 与现有配置兼容（避免静默语义变化）

现有配置继续生效：

1. `request.failure_time_window`
2. `request.failure_threshold`
3. `request.failure_action`
4. `request.failover_cooldown`
5. `strategy.type`
6. `strategy.fast_test_*`
7. `health.health_path`

**兼容策略**：

| 旧配置 | 新配置 | 兼容规则 |
| --- | --- | --- |
| 无 `claude_routing` 块 | 全部走默认值 | 行为与现状一致：保留单一阈值 + `failure_threshold=3`，不启用分类评分 |
| 有 `claude_routing.circuit_breaker.failure_weights` | 启用分类评分 | 此时 `request.failure_threshold` 不再作为次数阈值，仅作兜底（如果分类评分未触发但裸次数 >= 10 仍冷却） |
| 仅设置 `claude_routing.default_mode` 但无 `circuit_breaker` 块 | manual override 生效，断路器维持旧行为 | 允许用户分阶段启用 |

#### 15.2.1 `failure_threshold` 映射陷阱（重要）

**不**直接把 `failure_threshold` 映射到 `open_threshold`。原因：

- 旧：3 次任意失败 → 触发。
- 新：3 次 429（权重 1） = 3 分，恰好达到；但 1 次 connection_failure（权重 3）= 3 分，一次就达到。

如果直接映射，行为会静默变化。

**正确做法**：

1. `claude_routing.circuit_breaker.open_threshold` 总是优先使用 `claude_routing` 块的值；缺省默认 `3`。
2. 不读取 `request.failure_threshold` 作为 `open_threshold`。
3. 启用分类评分时，必须在 release notes 与配置变更日志中明确"行为变化"。
4. 若用户希望保持旧语义，可显式设置：
   ```yaml
   claude_routing:
     circuit_breaker:
       open_threshold: 3
       window: "5m"
       failure_weights:
         connection_failure: 1
         connection_timeout: 1
         response_timeout: 1
         rate_limit: 1
         server_error: 1
         stream_quality: 1
   ```
   所有权重为 1 时，"评分阈值"等价于"次数阈值"，与旧行为一致。

### 15.3 第一版配置范围

第一版（参见章节 22）只允许以下新配置：

```yaml
claude_routing:
  default_mode: "auto"           # 唯一新增的模式开关
  manual:
    persist_manual_override: true
```

其他字段（`circuit_breaker / failure_weights / capability_ttl / probes`）在 V2 阶段引入。第一版的失败分类只用编码内置的 4 类"不计入断路器"规则，不暴露权重配置。

## 16. 代码落点

### 16.1 新增 route decision 模块

建议新增：

```text
internal/endpoint/route_decision.go
internal/endpoint/route_state.go
internal/endpoint/failure_classifier.go
```

职责：

1. 构建候选端点。
2. 应用 manual override。
3. 应用断路器。
4. 应用能力过滤。
5. 输出选择结果和解释。

### 16.2 failure_tracker 调整

现有 `FailureTracker` 从“失败次数”升级为“分类失败窗口”。

建议接口：

```go
type EndpointFailureEvent struct {
    EndpointName string
    RequestID    string
    Path         string
    Model        string
    Class        FailureClass
    HTTPStatus   int
    Fingerprint  string
    Weight       float64
    OccurredAt   time.Time
}
```

建议方法：

```go
RecordFailureEvent(event EndpointFailureEvent) EndpointCircuitSnapshot
RecordSuccess(endpointName string, quality RouteSuccessQuality)
ShouldOpen(endpointName string) bool
GetCircuitSnapshot(endpointName string) EndpointCircuitSnapshot
```

### 16.3 RetryManager 调整

当前 `RetryManager.ShouldRetryWithDecision` 已经承担失败是否记录的判断。

建议调整：

1. 不再只返回 `ShouldRecord bool`。
2. 返回 `FailureClass`、`FailureWeight`、`CooldownHint`。
3. handler 层将失败事件提交给 endpoint manager。

示例：

```go
type RetryDecision struct {
    RetrySameEndpoint bool
    SwitchEndpoint    bool
    SuspendRequest    bool
    FinalStatus       string
    Reason            string
    RetryAfterSeconds int
    FailureClass      FailureClass
    FailureWeight     float64
    RecordFailure     bool
}
```

### 16.4 Handler 调整

涉及：

1. `internal/proxy/handlers/regular.go`
2. `internal/proxy/handlers/streaming.go`

目标：

1. 继续保证每个客户端请求只转发一个上游。
2. 失败后分类并记录。
3. 不在 handler 深处直接决定端点是否整体故障。
4. 不在 handler 深处处理 manual override。

### 16.5 Endpoint selection 调整

涉及：

1. `internal/endpoint/endpoint_selection.go`
2. `internal/endpoint/failover.go`
3. `internal/endpoint/group_manager.go`

目标：

1. 选择阶段使用统一 route decision。
2. `executeSelectionFailover` 不再是唯一切换入口。
3. 冷却和 active group 变更由 route state 统一决定。
4. 兼容现有 group activation 行为。

#### 16.5.1 与 groupManager.ManualActivateGroup 的兼容策略

当前有**两条**独立的自动切组路径，第一版必须同时改造，否则 manual_fixed 会在另一条路径上被绕过：

| 路径 | 触发时机 | 代码点 |
| --- | --- | --- |
| `failover.TriggerRequestFailover` | 请求级 failover，由 handler 在请求失败后调用 | `internal/endpoint/failover.go:70`（`ManualActivateGroup`） |
| `executeSelectionFailover` | 选择阶段 failover，由 `GetHealthyEndpoints` 在端点达到失败阈值时调用 | `internal/endpoint/endpoint_selection.go:73`（触发） + `endpoint_selection.go:454`（`ManualActivateGroup`） |

两条路径都直接调用 `groupManager.ManualActivateGroup`，引入 override 后都必须精细化。

**核心问题**：用户手动 override 指向端点 A（属于组 G1），但当前活跃组是 G2。是否要为了 override 激活 G1？

**第一版规则（保守，最低改动）**：

| 场景 | mode | A 所属组 G1 | 当前活跃组 | 行为 |
| --- | --- | --- | --- | --- |
| 用户手动切换到 A | manual_fixed | G1 | G2 | 激活 G1，停用 G2，写入 override |
| 用户手动切换到 A | manual_preferred | G1 | G2 | 激活 G1，停用 G2，写入 override |
| 请求级 failover 欲切到 B | manual_fixed | G1 | G1 | **禁止切换**，返回错误给客户端 |
| 请求级 failover 欲切到 B | manual_preferred | G1 | G1 | 允许切到 B（fallback），但**不**写入 override；A 恢复后下次请求回到 A |
| 选择阶段 failover 欲切到 B | manual_fixed | G1 | G1 | **禁止切换**，跳过选择阶段 failover，返回"无可用端点"错误 |
| 选择阶段 failover 欲切到 B | manual_preferred | G1 | G1 | 允许切到 B；不写入 override；下次请求重新评估 |
| 自动 failover（无 override） | auto | - | - | 按现有 `executeSelectionFailover` / `TriggerRequestFailover` 逻辑切换 |

**实现要点**：

1. 两条路径**共用**一个 `caller_kind` 改造：
   - `failover.TriggerRequestFailover(failedEndpointName, reason, callerKind)` 增加入参。
   - `executeSelectionFailover(failedEndpoints, newEndpointName, callerKind)` 增加入参。
   - `caller_kind` 取值：`user / system_failover_request / system_failover_selection / startup_recovery`。
2. 两条路径在切换前都必须调用同一个 `routeOverride.AllowSystemSwitch(currentMode, fromEndpoint, toEndpoint)` 决策函数：
   - `mode = manual_fixed`：返回 `false`，调用方必须放弃切换并返回错误。
   - `mode = manual_preferred`：返回 `true`，但调用方必须记录 `fallback_reason`，**不**更新 override。
   - `mode = auto`：返回 `true`，按现状逻辑切换。
3. `GetHealthyEndpoints`（`endpoint_selection.go:14`）的逻辑也要加 override 感知：
   - `mode = manual_fixed` 时，候选列表只允许 override 指向的端点；其他端点即使活跃也不参与候选。
   - `mode = manual_preferred` 时，override 端点排在候选列表最前。
   - 这一步发生在 `FilterEndpointsByActiveGroups` 之后、`executeSelectionFailover` 之前，确保候选阶段就守住 override。
4. 用户切换走专用 API（`SetClaudeRoutingOverride`），由 App 层先写 override 再调用 `ManualActivateGroup`，确保两者顺序原子。
5. `manual_preferred` 下 A 恢复后，下次请求的路由层会重新读 override 并触发 `ManualActivateGroup` 回到 G1（如果当前不是 G1）。这一步发生在路由决策阶段，不在请求结果阶段。

**禁忌**：

1. 任何代码路径不得绕过 `routeOverride.AllowSystemSwitch` 直接调用 `groupManager.ManualActivateGroup`（用户主动切换除外）。
2. `caller_kind = user` 是唯一允许写 override 的路径；其他 caller 一律不写。

#### 16.5.2 fallback 期间的组激活语义

`manual_preferred` 下从 A fallback 到 B 时，需要决定：
- 是停用 G1 + 激活 G2？
- 还是同时让 G1 与 G2 都活跃？

第一版选择**仍然按现有 group 单一活跃逻辑**：fallback 时停用 G1、激活 G2。原因：

1. 当前 `groupManager` 是单活跃组模型，多活跃组改造工作量大。
2. fallback 期间用户预期"临时使用其他端点"，单活跃组语义可接受。
3. A 恢复后路由层会自动再次切回 G1。

**注意**：这意味着 fallback 期间 G1 的其他端点也不会被使用。这是一种妥协，但避免了短期内动 groupManager 的核心数据结构。第二版可以考虑"多活跃组 + 组内选择"模型。

#### 16.5.3 启动恢复

桌面启动时按以下顺序处理：

1. 读取 override。
2. 如果 override.mode 不是 `auto`，激活 override.endpoint_name 所属组。
3. 如果该端点 disabled / 删除，把 override 降级为 `auto` 并打日志。
4. 启动连通性检查（`app_startup_connectivity.go`）异步执行，不阻塞 override 生效。

### 16.6 前端 API

建议新增或扩展：

1. `GetClaudeRoutingState`
2. `SetClaudeRoutingOverride`
3. `ClearClaudeRoutingOverride`
4. `GetEndpointRouteDiagnostics`
5. `ClearEndpointCircuit`
6. `ProbeEndpointCapability`

返回信息至少包括：

```text
mode
manual_endpoint
effective_endpoint
fallback_reason
endpoint_states[]
last_route_decision
```

## 17. 前端交互设计

### 17.1 请求页切换器

当前请求页已有端点快捷切换器，应扩展显示：

1. 当前模式：自动 / 手动优先 / 严格固定。
2. 当前手动端点。
3. 实际生效端点。
4. 自动 fallback 是否开启。
5. 恢复自动按钮。
6. 严格固定开关。

展示文案建议：

```text
当前路由：手动优先 · mywechat
实际使用：xuanwulei
原因：手动端点处于冷却中，临时 fallback
```

### 17.2 端点列表

每个端点展示：

1. `closed` 正常。
2. `degraded` 降级。
3. `open` 冷却中。
4. `half_open` 恢复试探中。
5. 最近失败类别。
6. 冷却剩余时间。
7. 最近成功时间。
8. 失败窗口分数。

### 17.3 诊断抽屉

端点诊断抽屉展示：

1. 最近 10 条失败事件。
2. 每类失败计数。
3. 当前能力画像。
4. 为什么本次请求没有选中该端点。
5. 手动清除冷却按钮。
6. 手动能力探测按钮。

## 18. 行为示例

### 18.1 Auto 模式下主端点 429

1. 请求打到 A。
2. A 返回 429。
3. 分类为 `rate_limit`，权重 1。
4. 返回错误给 Claude SDK。
5. A 仍保持候选。
6. 5 分钟内第 3 次 429 后 A 进入 `open`。
7. 下一次 SDK 重试切到 B。

### 18.2 Auto 模式下 A 连接失败

1. 请求打到 A。
2. A 连接失败。
3. 分类为 `connection_failure`，权重 3。
4. A 立即进入 `open 90s`。
5. 当前请求返回给 SDK。
6. 下一次 SDK 重试切到 B。

### 18.3 Manual Preferred 下 A 冷却

1. 用户手动选择 A。
2. A 多次失败进入冷却。
3. 下一次请求优先检查 A。
4. A 为 `open`，记录 fallback reason。
5. 临时选择 B。
6. override 仍保留为 A。
7. A 恢复后可自动回到 A。

### 18.4 Manual Fixed 下 A 冷却

1. 用户严格固定 A。
2. A 进入冷却。
3. 下一次请求仍只允许 A。
4. 系统直接返回错误，不选择 B。
5. 前端提示"严格固定模式未启用 fallback"。

错误返回格式（响应直接发给 Claude SDK / Codex SDK 等下游客户端，保持与上游错误透传风格一致）：

```http
HTTP/1.1 503 Service Unavailable
Content-Type: application/json
Retry-After: <cooldown_remaining_seconds>

{
  "type": "error",
  "error": {
    "type": "endpoint_unavailable",
    "message": "Manual fixed endpoint 'A' is in cooldown for 87s and fallback is disabled by manual_fixed mode.",
    "code": "route_blocked_manual_fixed",
    "details": {
      "endpoint": "A",
      "circuit_state": "open",
      "cooldown_until": "2026-06-02T16:14:15+08:00",
      "opened_reason": "connection_failure"
    }
  }
}
```

状态码取值规则：

| 情况 | HTTP 状态码 |
| --- | --- |
| 端点 disabled / 不存在 | 404 + `code = "endpoint_not_found"` |
| 端点在冷却中 | 503 + `Retry-After` 头 |
| 端点能力不匹配（如不支持模型 X） | 422 + `code = "endpoint_capability_mismatch"` |
| 鉴权失效（auth_error 长冷却中） | 503 + `code = "endpoint_auth_failed"` |

#### 18.4.1 路由诊断信息的传递路径（重要）

响应主体是给 Claude SDK / Codex SDK 等下游客户端的，**桌面端前端**读不到这层 HTTP 响应。路由模式、fallback 原因、实际选中端点等诊断信息必须走两条独立通道：

1. **写入 `request_logs`**：每条请求记录里附带 `route_mode / requested_endpoint / effective_endpoint / fallback_reason / route_decision_at` 字段，前端在请求详情页读取。
2. **Wails API**：`GetClaudeRoutingState()` 返回当前 override 状态、最近一次决策；`GetEndpointRouteDiagnostics(endpoint)` 返回该端点的失败窗口、冷却状态、负向缓存命中详情。

#### 18.4.2 X-Route-* 调试头（可选，不依赖）

可以保留 `X-Route-Mode / X-Route-Endpoint / X-Route-Reason` 作为**调试用响应头**，但需要满足：

1. 默认 **关闭**，仅在配置 `claude_routing.debug_response_headers: true` 时启用。
2. 启用时也仅写入响应头，不作为前端获取诊断信息的来源 — 前端任何 UI 行为都不能依赖这些头。
3. 启用场景：用户用 `curl -v` / 自己写脚本排查上游问题时观察。
4. 头名前缀必须避开任何 Anthropic 或 OpenAI 官方头，防止与上游产生冲突。

如果团队评估觉得 X-Route-* 价值不大，第一版可以**完全不实现**这套头，只靠 `request_logs` + Wails API 即可。

### 18.5 模型不支持

1. 请求模型为 `claude-opus-4-x`。
2. A 返回 `model_not_found`。
3. 分类为 `model_unsupported`。
4. 不打开 A 的端点断路器。
5. 记录 A 不支持该模型。
6. 后续该模型请求跳过 A。
7. 其他模型请求仍可使用 A。

### 18.6 schema 不兼容

1. 请求携带 `context_management`。
2. A 返回 `Extra inputs are not permitted`。
3. 分类为 `schema_incompatible`。
4. 不打坏整个 A。
5. 若有安全 sanitizer，则后续自动处理。
6. 若没有 sanitizer，则该类请求跳过 A。

## 19. 实施阶段

### 19.1 Phase 0: 现状确认

交付物：

1. 梳理现有 `FailureTracker` 入口。
2. 梳理 regular/streaming 失败分类。
3. 梳理前端手动切换入口。
4. 明确当前运行时配置项。

验收：

1. 能解释一次请求失败后如何影响下一次路由。
2. 能列出所有 `RecordFailure` 入口。
3. 能列出成功清理失败窗口的入口。

### 19.2 Phase 1: 失败分类最小落地

目标：

1. 不改路由主流程。
2. 先把失败分类从 bool 扩展为 class。
3. 把明显不应打坏端点的失败排除。

改动：

1. 新增 `FailureClass`。
2. `RetryDecision` 增加失败分类。
3. `model_unsupported`、`schema_incompatible`、`payload_too_large`、`client_cancel` 不进入端点断路器。
4. 保留现有 `failure_threshold=3` 行为。

验收：

1. 429/5xx 仍可按窗口触发切换。
2. `model_not_found` 不冷却整个端点。
3. `context_management` schema 错误不冷却整个端点。
4. 413 不冷却整个端点。
5. 现有 regular/streaming 测试通过。

### 19.3 Phase 2（V2+）: 分类断路器

本阶段属于 V2+ 参考，不纳入第一版交付。第一版继续复用现有 `failure_threshold=3 / window=5m` 次数窗口，只把明确不应计入端点失败窗口的 4 类失败排除出去。

目标：

1. 从“失败次数”升级为“失败评分”。
2. 支持不同分类不同冷却。
3. 支持 `degraded` 与 `half_open`。

改动：

1. 新增分类窗口 tracker。
2. 连接失败一次打开断路器。
3. 429/5xx 三次打开断路器。
4. 冷却到期进入 `half_open`。
5. 成功后恢复 `closed`。

验收：

1. 连接失败一次冷却。
2. 普通 429 不立即冷却。
3. 5 分钟内 3 次 429 冷却。
4. 冷却到期后允许恢复。
5. 成功清空对应失败窗口。

### 19.4 Phase 3: 手动 override 一等化

目标：

1. 明确 Auto、Manual Preferred、Manual Fixed。
2. 前端手动切换写入统一状态。
3. 自动策略不能偷偷覆盖手动选择。

改动：

1. 新增 override 状态存取。
2. 路由选择先读 override。
3. Manual Preferred 支持 fallback。
4. Manual Fixed 禁止 fallback。
5. 前端增加“恢复自动”和“严格固定”。

验收：

1. 手动切到 A 后，A 正常时一直用 A。
2. Manual Preferred 下 A 冷却时临时用 B。
3. Manual Fixed 下 A 冷却时不使用 B。
4. 恢复自动后清除 override。

### 19.5 Phase 4（V2+）: 完整能力画像

本阶段属于 V2+ 参考，不纳入第一版交付。第一版只实现章节 14.1.1 的最小负向命中缓存，用于避免同模型 / 同 schema / 同体积请求短时间内反复命中同一坏端点。

目标：

1. 把模型不支持、schema 不兼容从端点故障中拆出来。
2. 让路由按请求画像筛端点。

改动：

1. 新增内存能力画像。
2. 识别 `model_not_found`。
3. 识别 `Extra inputs are not permitted`。
4. 能力过滤参与 route decision。
5. 可选落库。

验收：

1. 某端点不支持模型 X，不影响模型 Y。
2. 某端点不支持 `context_management`，只影响带该字段请求。
3. sanitizer 有明确端点/channel 边界。

### 19.6 Phase 5: 前端可观测

第一版只做最小可观测：路由模式、手动端点、实际端点、fallback reason、恢复自动、严格固定。复杂诊断抽屉、端点失败事件统计、完整 capability 展示推迟到 V2+。

目标：

1. 用户能看懂切换原因。
2. 用户能手动清除冷却/恢复自动。
3. 用户能验证路由策略是否符合预期。

改动：

1. 请求页显示路由模式和实际端点。
2. 端点页显示 circuit state。
3. 增加诊断抽屉。
4. 增加失败分类统计。

验收：

1. 能看到当前是 Auto/Manual Preferred/Manual Fixed。
2. 能看到当前手动端点和实际端点。
3. 能看到 fallback reason。
4. 能看到端点冷却剩余时间。

## 20. 测试计划

### 20.1 单元测试

第一版必测：

1. `FailureClass` 最小分类。
2. 4 类不计入断路器的失败不会调用 `RecordFailure`。
3. 429/5xx 等其他错误仍维持现有失败窗口行为。
4. Manual Preferred fallback。
5. Manual Fixed 不 fallback，且两条系统切组路径都被拦住。
6. 最小负向命中缓存写入、命中、TTL 过期与清除。

V2+ 再补：

1. 断路器评分。
2. `open/half_open/closed/degraded` 状态转换。
3. 完整能力过滤。
4. 成功清理分类失败窗口。

建议测试文件：

```text
internal/endpoint/failure_classifier_test.go
internal/endpoint/route_override_test.go
internal/endpoint/route_state_test.go
internal/proxy/retry_manager_test.go
```

### 20.2 集成测试

第一版必测：

1. A 连续 3 次 429 后，下一次请求选择 B。
2. A 单次 429 后，下一次仍可选择 A。
3. A `model_not_found` 后，同模型请求跳过 A，其他模型仍可用 A。
4. A schema 不兼容后，同 schema 请求跳过 A，其他请求仍可用 A。
5. A 413 后，同体积桶或更大请求跳过 A。
6. Manual Preferred 下 A 冷却 fallback 到 B。
7. Manual Fixed 下 A 冷却不 fallback。

V2+ 再补：

1. A 连接失败一次后立即打开分类断路器。
2. 冷却到期后进入 `half_open`。
3. 完整 capability 画像自动失效与重新学习。

### 20.3 前端测试

第一版覆盖：

1. 路由模式展示。
2. 手动优先状态。
3. 严格固定开关。
4. 恢复自动按钮。
5. fallback reason 展示。
6. 实际端点展示。

V2+ 再补：

1. endpoint circuit state 展示。
2. 复杂诊断抽屉。
3. 失败分类统计。

### 20.4 回归命令

根据第一版改动范围逐步执行：

```bash
go test ./internal/endpoint
go test ./internal/proxy
go test ./internal/proxy/handlers
go test ./internal/service
go test ./internal/tracking
cd frontend && npm run build
```

`request_logs` 若新增列，必须跑 `go test ./internal/tracking`，并补 SQLite 旧库迁移回归。

V2+ 若涉及完整 schema 或 store 变更，再补：

```bash
go test ./internal/store
```

## 21. 风险与取舍

### 21.1 风险: 策略过复杂

缓解：

1. 分阶段落地。
2. 第一版只做失败分类，不立刻新增所有表。
3. 保持现有配置兼容。

### 21.2 风险: 手动固定导致用户忘记恢复自动

缓解：

1. 前端明显展示“严格固定”。
2. 可选提供过期时间。
3. 请求页持续显示当前模式。

### 21.3 风险: 能力画像误学习

缓解：

1. 只对高置信错误学习。
2. 支持手动清除能力限制。
3. 错误 fingerprint 必须稳定。

### 21.4 风险: 断路器与现有 group activation 冲突

缓解：

1. 第一版不重写 group manager。
2. 只在 selection 阶段加入统一 route decision。
3. 后续再收敛 group active 状态和 route state。

## 22. 推荐第一版实现范围

### 22.1 第一版做什么（3 件事）

第一版做以下 3 件事，覆盖最痛的两个问题，并补一个最小防抖机制：

**A. 失败分类的最小集合**

新增 `FailureClass` 枚举，第一版只识别"不计入断路器"的 4 类：

1. `client_cancel`：客户端取消（HTTP 499、context canceled、connection reset by client）。
2. `model_unsupported`：错误体含 `model_not_found / No available channel for model` 等模型权限信号。
3. `schema_incompatible`：错误体含 `Extra inputs are not permitted / context_management / cache_control.scope` 等 schema 不兼容信号。
4. `payload_too_large`：HTTP 413。

这 4 类失败**不**调用 `FailureTracker.RecordFailure`，因此不进入端点失败窗口、不冷却整个端点。其他错误（429 / 5xx / connection / timeout / auth）维持现状走 `failure_threshold=3 / window=5m` 行为，避免静默语义变化。

**B. Manual override 状态 + 两条切组路径改造**

引入 `mode = auto | manual_preferred | manual_fixed`：

1. 复用 settings 表持久化 `mode / endpoint_name / set_by / set_at / fallback_enabled`。
2. 路由选择第一步读 override。
3. **同时改造**两条自动切组路径（见章节 16.5.1）：
   - `failover.TriggerRequestFailover`（`internal/endpoint/failover.go:70`）
   - `executeSelectionFailover`（`internal/endpoint/endpoint_selection.go:73 / 454`）
   两者都增加 `caller_kind` 入参，通过共用的 `routeOverride.AllowSystemSwitch` 决策函数处理 mode：
   - `manual_fixed` + 任何 `system_*` caller → 拒绝切换，返回章节 18.4 定义的 503 错误。
   - `manual_preferred` + 任何 `system_*` caller → 允许 fallback，记录 `fallback_reason`，**不**更新 override。
4. `GetHealthyEndpoints` 在候选阶段就守住 override：`manual_fixed` 下候选列表只允许 override 端点；`manual_preferred` 下 override 端点排在最前。
5. 前端增加"恢复自动"和"严格固定"两个按钮。

**C. 最小负向命中缓存**

为避免 A 项中"不计入断路器"的 4 类失败循环命中同一坏端点，新增内存负向缓存（章节 14.1.1）：

1. 三类缓存键：`endpoint+model` / `endpoint+schema_field` / `endpoint+body_size_bucket`，以及 `endpoint`（count_tokens 不支持）。
2. 单一 TTL（默认 30m），每类容量上限 1024 项，LRU 淘汰。
3. 单次失败即写入，**不**做滑动统计 / 高置信判定（V1 简化）。
4. 不持久化、不暴露给前端展示，仅作为路由层内部防抖。
5. Wails API `ClearNegativeHitCache(endpoint)` 支持手动 reset。

完整 capability 画像（含滑动统计、自动失效、前端诊断）推迟到 V2（章节 11）。

### 22.2 第一版不做（明确）

以下功能在 V2+ 阶段实施，第一版**不**触碰：

1. 分类评分断路器（章节 10）。
2. half_open 真实请求探测（章节 10.4）。
3. **完整 capability 画像与滑动统计学习**（章节 11；第一版只做 22.1.C 的最小负向缓存）。
4. 新增数据库表（章节 14.2，包括 `endpoint_route_state / endpoint_failure_events / endpoint_capabilities / claude_routing_override`）。
5. 复杂诊断抽屉、端点失败事件统计。
6. 多活跃组、组内选择改造。
7. count_tokens 独立失败窗口（共用即可，但按 9.0.1 分类分流）。
8. 配置 `claude_routing.circuit_breaker / failure_weights / capability_ttl / probes`。
9. `X-Route-*` 调试响应头（章节 18.4.2，可选；不实现也不影响第一版收益）。

### 22.3 第一版预期收益

1. `model_not_found / 413 / schema 错误` 不再粗暴打坏整个端点。
2. **同类失败不再循环命中同一坏端点**（负向缓存兜底）。
3. 用户手动切换不再被自动 failover 静默覆盖（**两条切组路径都受 override 约束**）。
4. 严格固定模式让用户能稳定排障。
5. 前端能展示当前路由模式（auto / manual_preferred / manual_fixed）和"恢复自动"按钮。

### 22.4 第一版改动量估计

| 模块 | 改动量 | 关键改动 |
| --- | --- | --- |
| `internal/proxy/handlers/*`（失败分类） | 中等 | 新增 FailureClass、改造 ErrorContext，识别 4 类 |
| `internal/proxy/retry_manager.go` | 小 | `ShouldRetryWithDecision` 内识别 4 类，`ShouldRecord=false` |
| `internal/endpoint/failover.go` | 中等 | `TriggerRequestFailover` 入参 `caller_kind`、调用 `AllowSystemSwitch` |
| `internal/endpoint/endpoint_selection.go` | **中等** | `executeSelectionFailover` 入参 `caller_kind`；`GetHealthyEndpoints` 加 override 候选过滤 |
| `internal/endpoint/route_override.go`（新增） | 中等 | override 状态、并发读写、settings 持久化、`AllowSystemSwitch` 决策函数 |
| `internal/endpoint/route_state.go`（新增） | 小-中 | 负向命中缓存（4 类键、TTL、容量上限、LRU） |
| Wails API（`SetClaudeRoutingOverride / ClearClaudeRoutingOverride / GetClaudeRoutingState / ClearNegativeHitCache`） | 小 | App 层封装 |
| `request_logs` 字段扩展 | 小 | 增 `route_mode / requested_endpoint / effective_endpoint / fallback_reason` |
| 前端（请求页切换器、端点页提示、请求详情页展示路由信息） | 中等 | 模式切换 UI + 诊断字段展示 |
| 测试 | 中等 | 失败分类、override 持久化、两条切组路径 Manual Fixed 拒绝、负向缓存命中 |

不涉及 SQLite schema 大改（settings 表已存在，只新增 JSON 字段；`request_logs` 增列需走迁移）。

## 23. 最终验收标准

1. Auto 模式下，真实端点故障达到阈值后自动切换。
2. Manual Preferred 下，用户选择端点优先，明确不可用时才 fallback。
3. Manual Fixed 下，任何自动策略都不能切到其他端点。
4. `429/5xx` 继续可触发自动 failover。
5. `model_not_found` 不再冷却整个端点。
6. schema 不兼容不再冷却整个端点。
7. 轻量测试不再直接覆盖真实请求路由决策。
8. 前端能展示当前路由模式、实际端点、切换原因和冷却状态。
