# Startup Async Connectivity Check Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不阻塞桌面应用启动的前提下，为已配置的端点和 Codex 账号补一轮启动异步连通性检查，并仅更新最近状态与展示信息。

**Architecture:** 在 `App.startup()` 尾部新增一个 fire-and-forget 编排入口，由它分别调度端点批量连通性测试和账号池批量启动连通性测试。端点侧继续复用 `endpoint.Manager.BatchHealthCheckAll()`，账号侧新增服务层批处理方法复用单账号 `TestUpstreamAccount`，所有失败均只记日志，不反向影响启动成功与调度状态。

**Tech Stack:** Go, Wails, existing `endpoint.Manager`, existing `service.AccountPoolService`, Go unit tests

---

## Chunk 1: Startup Orchestration

### Task 1: 在 App 启动尾部接入异步启动检查

**Files:**
- Modify: `app.go`
- Create: `app_startup_connectivity.go`
- Test: `app_startup_connectivity_test.go`

实现约束：
- 在 `app_startup_connectivity.go` 中为测试预留未导出的函数注入点，例如 `startupEndpointCheckRunner` / `startupAccountCheckRunner`，生产环境走默认实现，测试可替换。
- 不新增导出 API，也不把测试 seam 暴露给前端或其他包。

- [ ] **Step 1: 写一个失败测试，验证启动调度不会阻塞主流程**

```go
func TestScheduleStartupConnectivityChecks_DoesNotBlockStartup(t *testing.T) {
	app := NewApp()
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	app.runStartupEndpointChecks = func() {
		started <- struct{}{}
		<-release
	}

	begin := time.Now()
	app.scheduleStartupConnectivityChecks()
	elapsed := time.Since(begin)

	if elapsed > 50*time.Millisecond {
		t.Fatalf("expected async startup checks, took %v", elapsed)
	}
	<-started
	close(release)
}
```

- [ ] **Step 2: 运行测试，确认它因为缺少启动检查编排入口而失败**

Run: `go test . -run TestScheduleStartupConnectivityChecks_DoesNotBlockStartup`
Expected: FAIL，提示 `scheduleStartupConnectivityChecks` 或注入点不存在

- [ ] **Step 3: 写一个失败测试，验证无端点且无账号时会直接跳过**

```go
func TestScheduleStartupConnectivityChecks_SkipsWhenNothingConfigured(t *testing.T) {
	app := NewApp()

	endpointCalled := false
	accountCalled := false
	app.runStartupEndpointChecks = func() { endpointCalled = true }
	app.runStartupAccountChecks = func() { accountCalled = true }

	app.scheduleStartupConnectivityChecks()
	time.Sleep(20 * time.Millisecond)

	if endpointCalled || accountCalled {
		t.Fatalf("expected no startup checks when nothing is configured")
	}
}
```

- [ ] **Step 4: 运行测试，确认它失败并暴露出当前缺少条件判断**

Run: `go test . -run TestScheduleStartupConnectivityChecks_SkipsWhenNothingConfigured`
Expected: FAIL，提示启动检查逻辑尚未实现

- [ ] **Step 5: 用最小实现新增启动检查编排文件**

实现要求：
- 在 `app_startup_connectivity.go` 中新增 `scheduleStartupConnectivityChecks()`
- 在 `app.go` 的 `startup()` 尾部调用该方法
- 通过轻量 helper 判断是否存在端点 / 可测账号
- 使用 goroutine 调度端点任务和账号任务
- 所有错误只进日志，不返回给 `startup()`

- [ ] **Step 6: 运行编排测试，确认通过**

Run: `go test . -run 'TestScheduleStartupConnectivityChecks_'`
Expected: PASS

- [ ] **Step 7: 提交这一小步**

```bash
git add app.go app_startup_connectivity.go app_startup_connectivity_test.go
git commit -m "feat: 接入启动异步连通性检查编排" -m "在 App.startup 末尾调度一次性异步连通性检查。 
保持启动主流程不等待网络请求，避免影响桌面应用可用速度。 
为后续端点与账号批量检查预留稳定编排边界。"
```

## Chunk 2: Account Pool Startup Checks

### Task 2: 为账号池增加启动批量连通性检查入口

**Files:**
- Create: `internal/service/account_pool_startup_connectivity.go`
- Create: `internal/service/account_pool_startup_connectivity_test.go`
- Modify: `internal/service/account_pool.go`

- [ ] **Step 1: 写一个失败测试，验证只检查启用且有凭据的账号**

```go
func TestRunStartupConnectivityChecks_FiltersDisabledOrCredentiallessAccounts(t *testing.T) {
	svc, ids := newStartupConnectivityTestService(t)

	result := svc.RunStartupConnectivityChecks(context.Background(), 4)

	if result.Total != 2 {
		t.Fatalf("expected 2 eligible accounts, got %d", result.Total)
	}
	if !reflect.DeepEqual(result.CheckedIDs, ids.Eligible()) {
		t.Fatalf("unexpected checked ids: %#v", result.CheckedIDs)
	}
}
```

- [ ] **Step 2: 运行测试，确认它因为批量入口不存在而失败**

Run: `go test ./internal/service -run TestRunStartupConnectivityChecks_FiltersDisabledOrCredentiallessAccounts`
Expected: FAIL，提示 `RunStartupConnectivityChecks` 未定义

- [ ] **Step 3: 写一个失败测试，验证单账号失败不会阻断其他账号**

```go
func TestRunStartupConnectivityChecks_ContinuesAfterSingleAccountFailure(t *testing.T) {
	svc := newStartupConnectivityTestServiceWithFailures(t)

	result := svc.RunStartupConnectivityChecks(context.Background(), 2)

	if result.SuccessCount != 1 || result.FailureCount != 1 {
		t.Fatalf("unexpected summary: %+v", result)
	}
}
```

- [ ] **Step 4: 运行测试，确认它失败并指向错误隔离逻辑缺失**

Run: `go test ./internal/service -run TestRunStartupConnectivityChecks_ContinuesAfterSingleAccountFailure`
Expected: FAIL

- [ ] **Step 5: 写最小实现**

实现要求：
- 在新文件中定义启动检查摘要结构体，例如 `StartupConnectivityCheckSummary`
- 新增 `RunStartupConnectivityChecks(ctx context.Context, concurrency int)` 方法
- 内部通过 `ListAccounts(ctx, true)` 取数并过滤 `enabled=true && credential_raw 非空`
- 用受限并发执行 `TestUpstreamAccount(ctx, id)`
- 汇总成功数、失败数、跳过数与失败详情
- 失败结果只进入摘要与日志，不新增 `last_error` 等持久化失败写回
- 不修改 cooldown、fail_count、active_selection 等调度状态

- [ ] **Step 6: 运行服务层测试，确认通过**

Run: `go test ./internal/service -run TestRunStartupConnectivityChecks_`
Expected: PASS

- [ ] **Step 7: 提交这一小步**

```bash
git add internal/service/account_pool.go internal/service/account_pool_startup_connectivity.go internal/service/account_pool_startup_connectivity_test.go
git commit -m "feat: 增加账号池启动连通性批量检查" -m "新增仅供启动阶段使用的账号池批量连通性检查入口。 
复用单账号 TestUpstreamAccount 逻辑，并限制并发避免启动时过度打上游。 
失败仅进入摘要和日志，不改动调度相关状态。"
```

## Chunk 3: Integration, Logging, and Regression Coverage

### Task 3: 把账号池批量启动检查接回 App，并补日志与回归测试

**Files:**
- Modify: `app_startup_connectivity.go`
- Modify: `app_startup_connectivity_test.go`

- [ ] **Step 1: 写一个失败测试，验证存在账号时会调度账号池启动检查**

```go
func TestScheduleStartupConnectivityChecks_TriggersAccountChecksWhenEligibleAccountsExist(t *testing.T) {
	app := newStartupConnectivityTestAppWithAccountPool(t)
	done := make(chan struct{}, 1)

	app.runStartupAccountChecks = func() {
		done <- struct{}{}
	}

	app.scheduleStartupConnectivityChecks()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected startup account checks to run")
	}
}
```

- [ ] **Step 2: 运行测试，确认它失败并暴露 app/service 接线缺口**

Run: `go test . -run TestScheduleStartupConnectivityChecks_TriggersAccountChecksWhenEligibleAccountsExist`
Expected: FAIL

- [ ] **Step 3: 运行既有端点基线测试，确认 manager 启动语义仍受保护**

Run: `go test ./internal/endpoint -run TestManagerStartDoesNotPerformAutomaticConnectivityChecks`
Expected: PASS

- [ ] **Step 4: 完成 App 与账号池接线，并补统一日志**

实现要求：
- `app_startup_connectivity.go` 调用账号池新方法
- 日志统一增加“启动连通性检查”前缀
- 本次不新增账号池即时前端推送事件，账号状态由后续主动拉取获取
- 端点和账号任一分支失败都不得影响应用启动完成日志

- [ ] **Step 5: 跑目标测试集**

Run: `go test . ./internal/service ./internal/endpoint -run 'TestScheduleStartupConnectivityChecks_|TestRunStartupConnectivityChecks_|TestManagerStartDoesNotPerformAutomaticConnectivityChecks'`
Expected: PASS

- [ ] **Step 6: 跑回归测试**

Run: `go test ./internal/service ./internal/endpoint`
Expected: PASS

- [ ] **Step 7: 提交这一小步**

```bash
git add app_startup_connectivity.go app_startup_connectivity_test.go
git commit -m "feat: 接回账号池启动连通性检查编排" -m "将账号池批量启动检查接入 App 启动异步编排流程。 
统一启动连通性检查日志语义，并保持账号结果只写回状态不即时推送前端。 
补齐根包编排测试，防止接线回归。"
```

## Chunk 4: Final Verification

### Task 4: 完整验证并准备合并

**Files:**
- Modify: `app.go`
- Modify: `app_startup_connectivity.go`
- Modify: `internal/service/account_pool_startup_connectivity.go`
- Test: `app_startup_connectivity_test.go`
- Test: `internal/service/account_pool_startup_connectivity_test.go`
- Test: `internal/endpoint/manager_test.go`

- [ ] **Step 1: 运行与本次改动直接相关的 Go 测试**

Run: `go test . ./internal/service ./internal/endpoint -run 'TestScheduleStartupConnectivityChecks_|TestRunStartupConnectivityChecks_|TestManagerStartDoesNotPerformAutomaticConnectivityChecks'`
Expected: PASS

- [ ] **Step 2: 运行格式化并确认代码树整洁**

Run: `go fmt ./...`
Expected: 所有相关 Go 文件完成格式化

- [ ] **Step 3: 如本机已安装 goimports，则执行 import 整理**

Run: `goimports -w app.go app_startup_connectivity.go app_startup_connectivity_test.go internal/service/account_pool.go internal/service/account_pool_startup_connectivity.go internal/service/account_pool_startup_connectivity_test.go`
Expected: 相关文件 import 顺序与格式正确；若命令不存在，则记录为环境缺口

- [ ] **Step 4: 运行账号池相关回归测试**

Run: `go test ./internal/store ./internal/accountauth ./internal/service ./internal/proxy ./internal/proxy/handlers ./internal/tracking`
Expected: PASS

- [ ] **Step 5: 记录验证结果并检查工作区**

Run: `git status --short`
Expected: 只包含本次实现相关文件改动

- [ ] **Step 6: 最终提交**

```bash
git add app.go app_startup_connectivity.go app_startup_connectivity_test.go internal/service/account_pool.go internal/service/account_pool_startup_connectivity.go internal/service/account_pool_startup_connectivity_test.go internal/endpoint/manager_test.go
git commit -m "feat: 启动时异步补充端点与账号连通性检查" -m "应用启动后会并行补一轮端点与账号连通性检查。 
检查过程不阻塞启动，也不会把失败结果直接写成调度层面的禁用或冷却。 
补齐相关单测与回归验证，保证现有手动连通性语义不回退。"
```
