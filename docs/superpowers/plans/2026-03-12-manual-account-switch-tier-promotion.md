# Manual Account Switch Tier Promotion Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把账号池手动切到主组/备组的行为从“单账号抽离”改成“按 priority 层整体移动”，并让点击账号在主组和备组两种手动切换下都成为该层当前使用账号。

**Architecture:** 后端继续以 `priority` 层作为主备切换单位，但重排时移动目标账号所属的整层，而不是单个账号；运行时缓存在主组和备组两种手动切换下都会写入 `activeAccount`，这样手动切到备组会立即接管当前流量。前端本地手动主备预览纯函数同步改成整层移动，并补齐按钮文案/可用性，避免 UI 预估和后端持久化结果不一致。

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
2. 其他层顺序按层整体重排，不出现单账号拆层；
3. 该层立即成为当前使用层，并优先命中被点击账号。

- [ ] **Step 3: 写“只有 1 个层级时切到备组是 no-op”的失败测试**

新增测试覆盖：

1. 当前只有一个 `priority` 层时，`MoveAccountToTier(ctx, targetID, 1)` 返回 `changed=false`；
2. 不清空现有快照，也不写入新的运行时活跃选择。

- [ ] **Step 4: 写“原本已在主组但切换不同账号仍刷新活跃账号”的失败测试**

保留现有“显式选择账号覆盖降级粘性”的断言，并补充同层另一账号被点击时也会更新 `activeAccount`。

- [ ] **Step 5: 写“原本已在备组但切换不同账号仍刷新活跃账号”的失败测试**

新增测试覆盖：

1. 目标层已经是备组位置时，点击同层另一账号仍返回 `changed=true`；
2. 下一次调度优先命中新点击的备组账号。

- [ ] **Step 6: 运行聚焦测试确认红灯**

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

保持 `MoveAccountToTier` 在切到主组和备组时都调用运行时 `selectAccount(accountID)`，哪怕该 tier 原本已经在目标层也要把切换当前账号视为有效切换。

- [ ] **Step 3: 明确备组 no-op 语义**

当当前只有 1 个层级、无法形成独立备组时，直接返回 `changed=false`，不清空快照，也不刷新运行时选择；当前端能识别该场景时，再进一步把按钮禁用或文案调整为不可执行。

- [ ] **Step 4: 运行服务层测试确认转绿**

Run: `go test ./internal/service -run 'TestMoveAccountToTier|TestPrepareSchedulableAccounts'`
Expected: 整层移动与主组显式选中相关测试全部 PASS。

## Chunk 2: Frontend Preview Consistency

### Task 3: 让前端手动主备预览和后端一致

**Files:**
- Modify: `frontend/src/pages/account-pool/utils/manualFailover.js`
- Create: `frontend/src/pages/account-pool/utils/manualFailover.test.js`
- Modify: `frontend/src/pages/account-pool/hooks/useAccountPoolActions.js`
- Modify: `frontend/src/pages/account-pool/components/AccountRow.jsx`
- Test: `frontend/src/pages/account-pool/utils/manualFailover.test.js`

- [ ] **Step 1: 先写前端纯函数红灯测试**

新增 `node --test` 可直接运行的前端纯函数测试，覆盖：

1. 同层账号一起升到主组；
2. 同层账号一起降到备组；
3. 仅点击账号作为层内优先对象，不拆散同层。

- [ ] **Step 2: 改造 `buildManualFailoverPriorityPlan`**

把前端预览逻辑改为按 tier 整体移动，保持与后端重排规则一致。

- [ ] **Step 3: 检查提示文案是否仍准确**

确认交互文案、tooltip 与刷新链路仍准确：

1. `changed=true` 时继续走成功 toast；
2. 同时刷新账号列表和最近一次调度快照；
3. 当数据库 priority 不变但主组或备组点击改变了运行时活跃账号时，仍不能落入“已在主组位置/备组位置”分支。
4. `AccountRow.jsx` 中“设为主组 / 设为备组”的 tooltip 与“整层移动 + 当前账号切换”语义一致，不再暗示只移动当前单账号。
5. 当当前只有 1 个 tier 时，前端不再把“设为备组”渲染成可执行操作，或至少明确为不可执行语义。

- [ ] **Step 4: 运行最小前端验证**

Run: `node --test frontend/src/pages/account-pool/utils/manualFailover.test.js`
Expected: PASS，证明前端纯函数的整层移动行为与设计一致。

- [ ] **Step 5: 运行前端构建验证**

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

按下面步骤执行并记录结果：

1. 运行后端聚焦测试：
   Run: `go test ./internal/service -run 'TestMoveAccountToTier|TestPrepareSchedulableAccounts'`
   Expected: PASS，且用例名能直接证明“整层移动”“主组显式选中”“切到备组后重置旧粘性”。
2. 运行前端纯函数验证：
   Run: `node --test frontend/src/pages/account-pool/utils/manualFailover.test.js`
   Expected: PASS，证明前端预览与整层移动语义一致。
3. 如需人工 spot check，启动桌面联调后在账号池页执行：
   - 准备两个同层账号 A/B 和一个备层账号 C；
   - 点击 A 或 B 的“切到主组”，确认列表里 A/B 同时处于主组 priority，且成功提示出现；
   - 再点击 B 的“切到主组”，确认即使 priority 不变也仍返回成功提示并刷新快照；
   - 点击 A 或 B 的“切到备组”，确认 A/B 一起下沉到备组层，旧快照被清空，且该层立即成为当前使用层；
   - 在当前备组层内再切换另一账号，确认即使 priority 不变也仍返回成功提示，并优先命中新账号。
   - 准备只有 1 个 tier 的账号池，确认“设为备组”不可执行，或执行后明确返回 `changed=false` 且快照不被清空。
4. 对照数据源检查：
   - Wails `MoveUpstreamAccountToTier` 返回 `changed=true`；
   - `GetLatestAccountScheduleSnapshot` 在有效手动切换后返回“无快照”，直到新请求重新生成；
   - 下一次 `/v1/responses` 的选中账号与测试断言一致。

- [ ] **Step 4: 提交实现**

```bash
git add internal/service/account_pool_manual_failover.go \
  internal/service/account_pool_runtime_cache.go \
  internal/service/account_pool_scheduler_test.go \
  internal/service/account_pool_test.go \
  frontend/src/pages/account-pool/utils/manualFailover.js \
  frontend/src/pages/account-pool/utils/manualFailover.test.js \
  frontend/src/pages/account-pool/hooks/useAccountPoolActions.js \
  frontend/src/pages/account-pool/components/AccountRow.jsx
git commit -m "fix: 调整账号池手动切号为整层提升" \
  -m "把手动主备切换的重排单位从单账号改为 priority 层。" \
  -m "保留点击账号的层内优先命中，并对齐前后端预览与调度结果。"
```
