# Manual Account Switch Tier Promotion Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把账号池手动切到主组/备组的行为从“单账号抽离”改成“按 priority 层整体移动，同时保留点击账号的层内优先命中”。

**Architecture:** 后端继续以 `priority` 层作为主备切换单位，但重排时移动目标账号所属的整层，而不是单个账号；运行时缓存仍保留 `activeAccount`，确保整层移动后由点击账号优先命中。前端本地手动主备预览纯函数同步改成整层移动，避免 UI 预估和后端持久化结果不一致。

**Tech Stack:** Go, SQLite-backed account pool store, runtime account cache, React/Wails frontend, Go tests

---

## Chunk 1: Backend Tier Reorder

### Task 1: 先把“整层移动”语义锁成红灯测试

**Files:**
- Modify: `internal/service/account_pool_scheduler_test.go`
- Modify: `internal/service/account_pool_test.go`

- [ ] **Step 1: 写“切到主组会整层提升”的失败测试**

新增服务层测试，构造两个同层账号和一个备层账号，断言调用 `MoveAccountToTier(ctx, targetID, 0)` 后：

1. 目标账号原本同 `priority` 的账号一起变成 `10`；
2. 原主组层整体顺延到下一层；
3. 点击账号仍是下一次调度的首选。

- [ ] **Step 2: 写“切到备组会整层下沉”的失败测试**

新增测试覆盖：

1. 目标账号原本同层账号一起下沉到备组层位；
2. 其他层顺序按层整体重排，不出现单账号拆层。

- [ ] **Step 3: 写“原本已在主组但切换不同账号仍刷新活跃账号”的失败测试**

保留现有“显式选择账号覆盖降级粘性”的断言，并补充同层另一账号被点击时也会更新 `activeAccount`。

- [ ] **Step 4: 运行聚焦测试确认红灯**

Run: `go test ./internal/service -run 'TestMoveAccountToTier|TestPrepareSchedulableAccounts'`
Expected: 新增测试失败，失败点体现当前实现仍会把目标账号单独抽成新层。

### Task 2: 实现后端整层重排

**Files:**
- Modify: `internal/service/account_pool_manual_failover.go`
- Modify: `internal/service/account_pool_runtime_cache.go`
- Test: `internal/service/account_pool_scheduler_test.go`
- Test: `internal/service/account_pool_test.go`

- [ ] **Step 1: 调整手动主备分层模型**

把 `buildManualFailoverPriorityUpdates` 的目标对象从“单个账号”改为“目标账号所属 tier”，实现：

1. 找到目标 tier；
2. 从层列表中移除整个 tier；
3. 插入到目标层位；
4. 统一重算 `10, 20, 30...` 优先级。

- [ ] **Step 2: 保留层内显式账号选择**

保持 `MoveAccountToTier` 在切到主组时调用运行时 `selectAccount(accountID)`，哪怕该 tier 原本已在主组也要视为有效切换。

- [ ] **Step 3: 运行服务层测试确认转绿**

Run: `go test ./internal/service -run 'TestMoveAccountToTier|TestPrepareSchedulableAccounts'`
Expected: 整层移动与主组显式选中相关测试全部 PASS。

## Chunk 2: Frontend Preview Consistency

### Task 3: 让前端手动主备预览和后端一致

**Files:**
- Modify: `frontend/src/pages/account-pool/utils/manualFailover.js`
- Modify: `frontend/src/pages/account-pool/hooks/useAccountPoolActions.js`
- Test/Verify: `frontend/src/pages/account-pool/utils/manualFailover.js`

- [ ] **Step 1: 先写或补前端纯函数断言**

如果已有纯函数测试入口则补测试；若暂时没有测试文件，则至少补一段最小可运行的验证用例设计，覆盖：

1. 同层账号一起升到主组；
2. 同层账号一起降到备组；
3. 仅点击账号作为层内优先对象，不拆散同层。

- [ ] **Step 2: 改造 `buildManualFailoverPriorityPlan`**

把前端预览逻辑改为按 tier 整体移动，保持与后端重排规则一致。

- [ ] **Step 3: 检查提示文案是否仍准确**

确认 `handleMoveAccountToTier` 的成功提示不需要改接口，但语义上与“整层移动 + 点击账号层内优先”保持一致。

- [ ] **Step 4: 运行最小前端验证**

Run: `cd frontend && npm run build`
Expected: PASS，且无手动主备相关构建错误。

## Chunk 3: End-to-End Verification

### Task 4: 跑回归并整理结论

**Files:**
- Verify only

- [ ] **Step 1: 运行账号池相关 Go 测试**

Run: `go test ./internal/store ./internal/accountauth ./internal/service ./internal/proxy ./internal/proxy/handlers ./internal/tracking`
Expected: PASS

- [ ] **Step 2: 运行前端构建验证**

Run: `cd frontend && npm run build`
Expected: PASS

- [ ] **Step 3: 检查用户可见结果**

至少确认：

1. 手动切到主组/备组后，不再把目标账号从原层单独拆出；
2. 下一次 `/v1/responses` 在目标账号可调度时优先命中它；
3. 前端手动主备预览与后端实际优先级结果一致。

- [ ] **Step 4: 提交实现**

```bash
git add internal/service/account_pool_manual_failover.go \
  internal/service/account_pool_runtime_cache.go \
  internal/service/account_pool_scheduler_test.go \
  internal/service/account_pool_test.go \
  frontend/src/pages/account-pool/utils/manualFailover.js \
  frontend/src/pages/account-pool/hooks/useAccountPoolActions.js
git commit -m "fix: 调整账号池手动切号为整层提升" \
  -m "把手动主备切换的重排单位从单账号改为 priority 层。" \
  -m "保留点击账号的层内优先命中，并对齐前后端预览与调度结果。"
```
