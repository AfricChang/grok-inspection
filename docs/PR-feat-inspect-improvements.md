# PR: Grok 巡检增强（异步 / 批量 / 修复 / 性能）

> Branch: `feat/inspect-improvements`  
> Base: `ywddd/grok-inspection` `main`（`7a09872`）  
> Plugin version: **0.1.13**

---

## Summary

在作者 main 的基础上，增强 Grok 账号巡检插件：

- **增量巡检**、结果 **本地落盘**、**异步** apply/action（避免 management 重入死锁）
- 批量启用 / 禁用 / 删除、批量导出、UI 与移动端优化
- **修复** 启用/禁用不生效、删除鉴权失败、操作反馈与状态同步问题
- **性能**：轻量 `/status` 轮询、批量删除走 CPA 本体批量接口、启用/禁用并发 PATCH

---

## 相对作者 main 的主要能力

### 巡检

| 能力 | 说明 |
|------|------|
| 完整巡检 | 清空结果后全量探测 |
| 增量巡检 | 只测新增账号；跳过键优先 `auth_index`，否则 `file_id` / `file+size+mtime`（不用邮箱/name 单独匹配） |
| 并发可配 | workers 1–16，默认 6 |
| 结果落盘 | `data/grok-inspection/results.json`（可用 `GROK_INSPECTION_DATA_DIR`） |
| 空闲不轮询 | 仅 running/applying 时 light poll |

### 操作

| 能力 | 说明 |
|------|------|
| 单行启用 / 禁用 / 删除 | 每行均提供（不依赖「建议动作」） |
| 批量启用 / 禁用 | 按当前筛选；**CPA 无批量接口**，插件侧并发 PATCH（约 6 路） |
| 批量删除 | **走 CPA 本体** `DELETE /auth-files` 多 `names`，**每批 50** |
| 执行建议操作 | 异步应用 classification 建议 |
| 单行操作确认 | 202 仅表示已接受；light `/status` 的 `recent_row_actions[action_seq]` 确认成功/失败后再改 UI |

### Management Key

| 来源 | 说明 |
|------|------|
| 管理面板 localStorage | 自动读取 `cli-proxy-auth`（`enc::v1::` 解混淆，与 Management Center 一致） |
| 插件本地存储 | `grokInspectionManagementKey` |
| 请求头 | `Authorization: Bearer` / `X-Management-Key` 传给后台 |
| 环境变量 fallback | `MANAGEMENT_PASSWORD` / `CPA_MANAGEMENT_KEY` |

> 说明：CPA 配置里的 `secret-key` 为 bcrypt，**无法**从进程反解明文；无法从 Host API 读取管理密码。

### 合并自作者的修复（rebase 保留）

- 页面 Management Key 用于 apply/delete（第三方安装不必设进程 env）
- 管理 API 固定本机 `127.0.0.1:8317`（或 `CPA_BASE_URL` / `PORT`），**不用**反代 Host 端口
- delete 幂等、结果移除

---

## 重要 Bug 修复

### 1. 启用/禁用对 CPA 本体不生效

**原因：** 曾用 `host.auth.save` 写 JSON `disabled`。  
CPA `buildAuthFromFileData` **不会**把 JSON 的 `disabled` 提升到 `Auth.Disabled`，主界面仍显示启用。

**修复：** 与作者一致，走：

```http
PATCH /v0/management/auth-files/status
{"name":"<file>.json","disabled":true|false}
```

在 **后台 goroutine** 调用，避免 management 重入死锁。

### 2. 删除报 `CPA management password is unavailable`

**原因：** 无明文 Key 时无法调 Management API。

**修复：**

- 优先用请求头里的页面 Key
- 自动从管理面板同源 `localStorage` 读取
- 文案提示「记住密码」/ 手动填写

### 3. 操作「感觉很慢」但 `/action` 很快

**原因：**

- `/action` 返回 202 后，前端曾轮询等成功
- 后端每次操作全量 `host.auth.list` + 校验 + 整表落盘

**修复：** 乐观 UI、本地结果解析文件名、去掉全量 list 校验、合并落盘。

---

## 性能优化

### GET `/status` 轻量模式

| 请求 | 行为 |
|------|------|
| `?include_results=0` 或 `light=1` | 仅进度 / summary / `results_gen`，**不含** results 数组 |
| `?include_results=1`（默认） | 全量 results |

UI：任务进行中 light 轮询；任务结束再拉全量。字段 `results_gen` 用于感知列表变更。

### 批量操作

| 操作 | 策略 |
|------|------|
| **删除** | 本体批量 `DELETE` + body `{"names":[...]}`，**每批 50** |
| **启用/禁用** | 本体仅单条 PATCH → 插件 **并发约 6 路**；弹窗说明可能较慢，建议大量清理用删除 |
| 落盘 | 批量过程每 25 条写一次 + 结束写一次；`results.json` 用 compact JSON |

---

## 与 CPA 本体 API 对应关系

| 插件操作 | CPA API | 批量？ |
|----------|---------|--------|
| 列表 / 巡检探测 | `host.auth.list` / `host.http.do` | 一次 list |
| 启用/禁用 | `PATCH /v0/management/auth-files/status` | **否**（单 name） |
| 删除（单） | `DELETE /v0/management/auth-files?name=` | 单 |
| 删除（批量） | `DELETE /v0/management/auth-files` + `{"names":[...]}` | **是**（插件每批 50） |

---

## 主要改动文件

| 文件 | 变更 |
|------|------|
| `engine.go` | 异步 apply/action、鉴权、批量删除、并发禁用启用、轻量 snapshot、落盘策略 |
| `main.go` | 路由、`status?include_results=`、版本号、slim 202 响应 |
| `ui.go` | 批量/单行操作、Key 自动读取、进度高亮、light poll、确认文案 |
| `store.go` / `store_test.go` | 结果 JSON 落盘 |
| `engine_test.go` / `main_test.go` | 异步、light status、batch 等测试 |
| `README.md` / `docs/` | 文档与 changelog |
| `.github/workflows/*` | CI/release 依赖与 artifact 升级 |

---

## Commits（相对 base）

```
92a5514 feat: incremental inspect, async ops, idle poll, docs
2ea26bd ci: fix Actions deprecation warnings and go cache
a512787 ci: bump upload-artifact to v7 (Node 24)
d3fbff7 优化ui
4a1f8f0 fix: restore mobile overflow-x for actions row
baf2523 增加批量启用按钮
13b1b15 add doc
c7241f1 合并作者更新并添加修复一些功能问题
d690ce4 批量操作调用本体接口
```

---

## 测试建议

- [ ] 完整巡检 / 增量巡检 / 停止
- [ ] 单行：启用、禁用（对照 CPA 主界面 Auth 状态）
- [ ] 单行：删除（行淡出 + 主界面凭证消失）
- [ ] 批量删除（>50 条时看分批进度）
- [ ] 批量启用/禁用（弹窗有「无本体批量接口」说明；进度条更新）
- [ ] 从 `/management.html` 登录并记住密码后打开插件，Key 自动填充
- [ ] 巡检/批量进行中 Network：`/status?include_results=0` 无大 results；结束后有全量
- [ ] 重启插件后结果从盘恢复

```bash
go test ./...
# 再按项目 build.ps1 / build.sh 编译插件并加载到 CPA
```

---

## 风险与后续

| 项 | 说明 |
|----|------|
| 批量启用/禁用 | 依赖 CPA 单条 PATCH；上千账号仍受 CPA 写入速度限制 |
| 若需更快启用/禁用 | 需上游提供批量 `PATCH status`（如 `names[]` + `disabled`） |
| `results.json` 体积 | 账号很多时落盘/全量 status 仍不小；已用 light poll + compact JSON 缓解 |

---

## Checklist

- [x] Rebase 作者 main（Management Key + 本机管理端口）
- [x] 禁用/启用走 Management PATCH
- [x] 删除鉴权 + 批量删除走本体
- [x] 异步防死锁
- [x] 轻量 status + 批量性能
- [ ] Reviewer 实机验证上述测试项
