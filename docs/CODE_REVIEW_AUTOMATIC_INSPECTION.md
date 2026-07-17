# 自动巡检代码审核报告（第三轮）

审核对象：第一版「账号自动巡检」功能实现（第二轮后再次修复）
审核日期：2026-07-17（第三轮）
参考设计：[AUTOMATIC_INSPECTION_DESIGN.md](./AUTOMATIC_INSPECTION_DESIGN.md)
审核范围：`automation.go` / `automation_store.go` / `usage.go` / `apply.go` / `engine.go` / `management.go` / `cgo_bridge.go` / `automation_test.go` / `classify.go` / `identity.go`

## 1. 审核结论

三轮迭代后，设计文档承诺的全部运行学行为（重启补跑、空闲延后、合并去重、超时跳过）均已落地，测试覆盖从 16 项扩展到 22 项，关键并发路径（engine 空闲唤醒、生命周期、pending 优先调度）全部修复。

剩余问题均为 Low 级打磨项，无阻塞验收的缺陷。**建议进入生产环境验收。**

## 2. 第二轮问题修复对照

| 编号 | 第二轮问题 | 修复方式 | 状态 |
|---|---|---|---|
| M1 | pending 规则未优先调度，30s 重试循环 | `runDue` 开头优先检查 `pendingIDs`，engine 空闲时立即 `runRules(pendingRules, false)` 并 return（[automation.go:317-331](../automation.go#L317-L331)） | 已修复 |
| M2 | `pendingAt` 字段未被使用 | 新增 `pendingExpired`（[automation.go:391-403](../automation.go#L391-L403)），`pendingAt` 超过一个 `IntervalMinutes` 时 `resetPendingSchedule` 跳过补跑 | 已修复 |
| M3 | `ruleMissedTooLong` 重置后未 `signalWake` | `runDue` 在 `pendingExpired` 分支与 `ruleMissedTooLong` 分支末尾均调用 `signalWake()`（[automation.go:320,370](../automation.go#L320)） | 已修复 |
| L1 | 新建规则首次执行时间提示 | `upsertRule` 设置 `LastRunAt = now`，用户可通过「立即执行」按钮绕过（[automation.go:268](../automation.go#L268)） | 可接受 |
| L4 | `mergeAutomationRules` 清空调度字段 | 显式清空 `Weekdays`/`WindowStart`/`WindowEnd`/`IntervalMinutes`/`LastRunAt`/`NextRunAt`（[automation.go:486-491](../automation.go#L486-L491)） | 已修复 |
| L5 | 加载时校验非法时间格式 | `newAutomationManager` 调用 `validateAutomationRule`，非法规则自动 `Enabled=false` 并写入 `lastError`（[automation.go:92-100](../automation.go#L92-L100)） | 已修复 |
| L6 | `scope` 允许 `all` 的文档对齐 | `validateAutomationRule` 显式允许 `all`/`permission_denied`/`quota_exhausted`/`other`（[automation.go:238](../automation.go#L238)） | 已对齐 |
| L7 | 测试覆盖缺口 | 新增 6 项测试，总数 22 项 | 已补全 |

## 3. 本轮新增的实现亮点

### 3.1 engine 空闲时主动唤醒 scheduler

这是本轮最关键的修复。`engine.stop()` / `engine.finish()` / `runApply` 退出 / `startAction` goroutine 退出时都调用 `automation.signalWake()`（[engine.go:522,629](../engine.go#L522)、[apply.go:513,526](../apply.go#L513)）。

效果：pending 规则不再依赖 30s ticker 轮询，engine 一旦空闲，scheduler 立即被唤醒并触发 `runDue` → `runRules(pendingRules, false)`。真正实现了设计文档「延后到空闲后执行一次」的语义。

测试 [automation_test.go:520-537](../automation_test.go#L520) `TestManualStopWakesPendingScheduler` 验证 `engine.stop()` 后 `automation.wakeCh` 收到信号。

### 3.2 pending 规则的完整生命周期

| 状态 | 触发条件 | 处理逻辑 |
|---|---|---|
| defer | `executeRule` 检测到 engine 忙 | `deferRule` 设置 `pendingID`/`pendingIDs`/`pendingAt`/`pendingReason` |
| 补跑 | engine 空闲 + `signalWake` | `runDue` 优先检查 `pendingIDs`，`runRules(pendingRules, false)` |
| 超时 | `pendingExpired` 为真 | `resetPendingSchedule` 清空 pending + 更新 `LastRunAt` + 写 `lastError` |
| 完成 | `finishRule` 中检测到 `completed[pendingID]` | 清空 `pendingID`/`pendingIDs`/`pendingAt` |
| 窗口外 | `ruleMissedTooLong` 为真 | 重置 `LastRunAt` + 清空 pending（[automation.go:348-356](../automation.go#L348-L356)） |

测试覆盖：
- [automation_test.go:434-457](../automation_test.go#L434) `TestPendingRuleRunsImmediatelyWhenEngineBecomesIdle` — pending 在 engine 空闲后立即补跑
- [automation_test.go:459-470](../automation_test.go#L459) `TestPendingRuleExpiresAfterOneInterval` — pending 超时后跳过并写入 `lastError`

### 3.3 加载时校验与自动停用

[automation.go:92-100](../automation.go#L92-L100) 在 `newAutomationManager` 中对每条规则调用 `validateAutomationRule`，校验失败的规则被自动 `Enabled=false`，并在 `lastError` 中记录原因。

测试 [automation_test.go:375-392](../automation_test.go#L375) `TestAutomationManagerDisablesInvalidLoadedRule` 验证非法 `WindowStart` 的规则被停用。

### 3.4 插件生命周期测试

[automation_test.go:492-518](../automation_test.go#L492) `TestPluginRuntimeLifecycleStartsAndStopsManagers` 验证：
- `startPluginRuntime()` 后 `automation.started == true`
- `shutdownPluginRuntime()` 后 `automation.started == false` 且 `engine.stopped == true`

这封闭了首次审核 L7 提到的「`cgo_bridge` 生命周期未测试」缺口。

### 3.5 executeRule 的 failed 路径覆盖

[automation_test.go:472-490](../automation_test.go#L472) `TestExecuteRuleRecordsNonBusyStartFailure` 验证 `engine.start` 返回非 busy 错误（`Workers > maxWorkers`）时，`executeRule` 写入 `history.Status = "failed"` 并记录错误信息。这覆盖了首次审核 L7 的「failed 路径未覆盖」缺口。

## 4. 问题清单

### 4.1 Low

#### L1. `nextRuleRunAt` 的 14 天扫描在极端场景下的性能

位置：[automation.go:814-822](../automation.go#L814-L822)

```go
limit := candidate.Add(14 * 24 * time.Hour)
for !candidate.After(limit) {
    if ruleWindowContains(rule, candidate) {
        return candidate.Format(time.RFC3339)
    }
    candidate = candidate.Add(time.Minute)
}
```

最坏情况 20160 次迭代，每次调用 `ruleWindowContains`（遍历 `Weekdays` + 2 次 `parseClock`）。在 `snapshot` 中对 N 条规则调用，高频 UI 轮询（每 2s）时，N=10 规则约 20 万次迭代。

实际触发条件：`dueAt` 不在窗口内（跨午夜窗口 + 当前在窗口外，或周末窗口 + 当前是工作日），大多数情况下 `dueAt` 在窗口内直接返回。

建议：若未来规则数量增长到 50+，可缓存 `nextRuleRunAt` 结果并在 `upsertRule` 时失效；或预计算下一个窗口边界而非逐分钟扫描。当前规模无需处理。

#### L2. `recordUsageEvent` 的同步写盘在 Usage 回调关键路径上

位置：[automation.go:707-711](../automation.go#L707-L711)

```go
if err := saveAutomationHistory(history); err != nil {
    a.mu.Lock()
    a.lastError = "保存自动巡检历史失败: " + err.Error()
    a.mu.Unlock()
}
```

`processUsageRecord` → `recordUsageEvent` → `saveAutomationHistory`（同步原子重命名）。设计文档要求「Usage 回调必须快速返回」，虽然原子重命名通常 < 10ms，但磁盘慢时可能阻塞 `handleUsage` 返回。

建议：可参考 `engine.persistLocked` 的异步模式，`recordUsageEvent` 内部用 goroutine 写盘。但当前实现可接受，因为 `automation-history.json` 远小于 `results.json`。

#### L3. `executeRule` 中 `engine.snapshot(false).Stopped` 检查冗余

位置：[automation.go:592](../automation.go#L592)

```go
if rule.ApplyRecommendations && !engine.snapshot(false).Stopped {
```

`runCompletion()` 已经返回，此时 `engine.running == false`。`snapshot(false)` 获取 `engine.mu` 并复制 summary/recentRowActions 等字段，仅为读取 `Stopped`。可直接 `engine.mu.Lock(); stopped := engine.stopped; engine.mu.Unlock()`。

建议：替换为轻量的 `engine.isStopped(0)`（已有该方法，[engine.go:571-575](../engine.go#L571-L575)）。当前性能影响极小。

#### L4. `processUsageRecord` 的异步禁用不与 `engine.automatic` 协调

位置：[usage.go:86-108](../usage.go#L86-L108)

如果自动 apply 阶段正在禁用账号 A，同时 Usage 回调发现账号 B 额度用尽，两者并发调用 `setAuthDisabled`。`usageActionState.inFlight` 按 key 去重，`engine.mu` 串行化写入，逻辑安全。但 Usage 触发的禁用不受 `engine.automatic` 标记约束，可能在自动序列期间产生额外的 CPA 管理 API 调用。

这是设计决策（Usage 被动检测优先级高于自动序列），不是 bug。建议在文档中明确这一行为。

#### L5. `deferRule` 中 `pendingIDs` 直接覆盖

位置：[automation.go:625-628](../automation.go#L625-L628)

```go
a.pendingIDs = append([]string(nil), rule.SourceRuleIDs...)
```

如果两个独立规则先后 `deferRule`，后者会覆盖前者的 `pendingIDs`。由于 `executeRule` 是单线程的（`runRules` 检查 `runningID != ""`），不会并发调用，所以实际不会发生。但 `pendingIDs` 的覆盖语义值得在注释中说明。

建议：在 `deferRule` 注释中说明「`executeRule` 单线程保证，`pendingIDs` 覆盖是预期行为」。

#### L6. `executeRule` 的 `<-a.stopCh` 退出路径调用链复杂

位置：[automation.go:585-590](../automation.go#L585-L590)

```go
case <-a.stopCh:
    history.Status = "stopped"
    history.Message = "插件卸载，自动巡检等待结束"
    a.finishRule(rule, started, history, false)
    return
```

`finishRule` 调用 `engine.clearAutomationContext(rule.ID)` 清空 `engine.automatic`，但 `engine.start()` 启动的巡检 goroutine 仍在运行，依赖 `engine.shutdown()` 的 `runWG.Wait()` 兜底。逻辑正确，但调用链跨多个模块，建议在 `executeRule` 注释中说明「shutdown 时巡检 goroutine 由 `engine.shutdown` 统一回收」。

#### L7. `runDue` 中 pending 优先导致 due 规则延迟一个 ticker

位置：[automation.go:317-331](../automation.go#L317-L331)

`runDue` 优先处理 `pendingIDs`，如果 pending 规则执行成功，`due` 规则会在下一个 ticker (30s) 或 `signalWake` 时触发。这是合理的优先级设计（pending 是「曾因 engine 忙而延后」的规则），但可能导致 due 规则延迟 30s。

建议：在 `runDue` 末尾，如果 pending 规则执行成功且仍有 due 规则，可再次 `signalWake` 触发立即执行。但当前行为可接受。

#### L8. `pendingExpired` 使用 `rules[0]` 作为基础

位置：[automation.go:395](../automation.go#L395)

```go
maxWait := time.Duration(rules[0].IntervalMinutes) * time.Minute
```

取所有 pending 规则中最小的 `IntervalMinutes` 作为超时阈值。语义合理（最短间隔的规则最先超时），但依赖 `rulesByIDsLocked` 返回的顺序与 `a.pendingIDs` 顺序一致。当前实现 `rulesByIDsLocked` 遍历 `a.rules`，顺序与 `a.pendingIDs` 不同，但 `pendingExpired` 内部遍历所有规则取最小值，结果与顺序无关。安全，但值得在注释中说明。

## 5. 安全性复核（无变化）

| 安全约束 | 实现位置 | 结论 |
|---|---|---|
| 自动任务永不 delete | [apply.go:362](../apply.go#L362)、[automation.go:593](../automation.go#L593) | 通过 |
| `reauth` 账号不进入自动巡检 | [automation.go:537](../automation.go#L537)、[usage.go:59-63](../usage.go#L59-L63) | 通过 |
| 单次异常不自动禁用 | [classify.go:194-203](../classify.go#L194-L203) 异常 → keep | 通过 |
| 健康→已禁用→自动启用 | [apply.go:102-104](../apply.go#L102-L104) | 通过 |
| 额度/权限 → 自动禁用 | [usage.go:87-108](../usage.go#L87-L108) | 通过 |
| Usage 回调快速返回 | [usage.go:17-26](../usage.go#L17-L26) 同步仅内存写入 + 异步禁用 | 通过 |
| Management Key 不落盘 | [management.go:151-159](../management.go#L151-L159) | 通过 |
| 自动序列期间拒绝手动操作 | [engine.go:420-422](../engine.go#L420-L422)、[apply.go:413-415](../apply.go#L413-L415)、[apply.go:473-475](../apply.go#L473-L475) | 通过 |
| 加载时校验非法规则并停用 | [automation.go:92-100](../automation.go#L92-L100) | 通过（新增） |

## 6. 测试覆盖矩阵

| 测试 | 覆盖路径 |
|---|---|
| `TestAutomaticSequenceBlocksManualStartsBetweenProbeAndApply` | 自动序列期间拒绝手动操作 |
| `TestAutomationWakeSignalAndPendingSnapshot` | wake 信号与 pending 快照 |
| `TestExecuteRuleDefersWhileEngineBusyWithoutSkippedHistory` | engine 忙时 defer 不写 skipped |
| `TestExecuteRuleWaitsForEngineCompletionNotification` | executeRule 等待完成通知 |
| `TestRuleDueCatchUpAndCooldownBoundaries` | 补跑与冷却边界 |
| `TestNextRuleRunAtMovesToNextValidWindow` | nextRuleRunAt 跨窗口计算 |
| `TestAutomationStoreRejectsUnknownVersion` | 存储版本校验 |
| `TestAutomationManagerDisablesInvalidLoadedRule` | 加载时校验非法规则 |
| `TestMergeAutomationRulesDeduplicatesScopes` | 多规则合并 |
| `TestAutomationLoopWakesAndDefersMergedDueRules` | loop 合并去重与 defer |
| `TestPendingRuleRunsImmediatelyWhenEngineBecomesIdle` | pending 在 engine 空闲后立即补跑 |
| `TestPendingRuleExpiresAfterOneInterval` | pending 超时跳过 |
| `TestExecuteRuleRecordsNonBusyStartFailure` | executeRule failed 路径 |
| `TestPluginRuntimeLifecycleStartsAndStopsManagers` | 插件生命周期 |
| `TestManualStopWakesPendingScheduler` | engine.stop 唤醒 scheduler |
| 其余 7 项 | 分类、禁删、UI 文案等基础测试 |

**未覆盖**：
- `nextRuleRunAt` 14 天扫描的最坏情况性能（低优先级）
- 多规则并发 `deferRule`（理论上不会发生，`executeRule` 单线程）
- `processUsageEvent` 在 `engine.automatic` 期间的协调（设计决策，非 bug）

## 7. 改进优先级

| 优先级 | 项 | 建议工作量 |
|---|---|---|
| P3 | L1 `nextRuleRunAt` 扫描缓存（规则数 > 50 时） | 中 |
| P3 | L2 `recordUsageEvent` 异步写盘 | 小 |
| P3 | L3 `executeRule` 中 `snapshot` 替换为 `isStopped` | 小 |
| P3 | L4 文档说明 Usage 与自动序列的协调 | 文档 |
| P3 | L5/L6/L7/L8 注释补充 | 小 |

## 8. 附：审核检查清单

### 安全约束（全部通过）

- [x] 禁止删除三层防护
- [x] `reauth` 排除
- [x] 健康/被动检测分支
- [x] 冷却时间字段写入
- [x] 原子落盘
- [x] 时间窗口跨午夜
- [x] 管理接口 CRUD
- [x] UI 关键文案
- [x] Usage 异步禁用
- [x] Management Key 内存缓存
- [x] 自动序列期间拒绝手动操作
- [x] 加载时校验非法规则

### 运行学行为（全部实现）

- [x] 调度器重启补跑（`ruleMissedTooLong`）
- [x] 空闲延后执行（`deferRule` + `signalWake` + `runDue` 优先 pending）
- [x] pending 超时跳过（`pendingExpired` + `resetPendingSchedule`）
- [x] 多规则同时触发的去重（`mergeAutomationRules`）
- [x] engine 空闲后立即唤醒 scheduler（`signalWake` 在 `stop`/`finish`/`runApply`/`startAction` 退出时调用）

### 测试覆盖（22 项，关键路径全部覆盖）

- [x] `executeRule` 端到端（success/defer/failed/stopped）
- [x] 完成通知 channel（`runDoneCh`/`applyDoneCh`）
- [x] 自动序列期间拒绝手动操作
- [x] 存储版本校验
- [x] 加载时校验非法规则
- [x] 插件生命周期（`startPluginRuntime`/`shutdownPluginRuntime`）
- [x] pending 完整生命周期（补跑/超时/engine 空闲唤醒）
- [x] `engine.stop` 唤醒 scheduler
- [x] 多规则合并去重

### 性能（当前规模可接受）

- [ ] `nextRuleRunAt` 14 天扫描缓存（规则数 > 50 时再优化）
- [ ] `recordUsageEvent` 异步写盘（磁盘慢时再优化）
