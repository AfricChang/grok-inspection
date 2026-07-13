# Grok Inspection 改动说明（2026-07-13）

本文整理今日在 `grok-inspection` 插件上的主要改动，便于自测、部署与向原作者提 PR 时对照。

---

## 1. 背景与目标

原插件在「管理页操作 / 状态查询」上存在以下问题：

- 删除、执行建议时可能通过 Management HTTP **重入**，导致 **status 卡住、删除无返回（死锁感）**
- 结果只活在内存，重启 CPA 后列表丢失
- 缺少增量巡检、按筛选批量操作、导出
- 空闲时仍高频轮询 status，请求偏多
- UI 上卡片筛选与按钮筛选重复

今日改动围绕：**不堵主业务、可落盘、可批量、可增量、CI 可编、文档可懂**。

---

## 2. 异步执行与性能说明（重点）

### 2.1 为什么要异步

巡检、批量禁用/启用/删除、执行建议，都可能涉及：

- 大量账号
- 上游 Grok 探测（`host.http.do`）
- CPA Auth 读写 / Management 删除

若在 **同一次 Management 请求里同步做完**，会长时间占用插件 `management.handle`，表现为：

- status 一直转圈
- 页面无响应
- 严重时影响同一 CPA 上的管理能力（主体运维/调度侧体感「卡死」）

### 2.2 现在的策略

| 能力 | 接口行为 | 后台行为 |
|------|----------|----------|
| 完整 / 增量巡检 | `POST /start` **立即返回** | goroutine + worker 并发探测（默认 6，1–16） |
| 执行建议 / 批量禁用启用删除 / 单行操作 | `POST /apply`、`POST /action` 返回 **202**，立即结束请求 | **另一条后台任务串行**执行 |
| 查询进度 | `GET /status` **只读内存快照** | 不探测、不改 Auth |

**status 在空闲时不再定时狂刷**；仅当 `running` 或 `applying` 为 true 时每 1.5s 轮询，任务结束后自动停止。

### 2.3 速度不会「很快」——这是刻意取舍

请务必知晓：

1. **批量禁用 / 启用 / 删除目前是串行的**（一条接一条），不是像巡检那样 6 路并发。  
   - 目的：降低对 CPA Auth、Management API、磁盘的冲击，**优先保证主业务不被拖垮**。  
   - 几千～上万账号时，批量可能跑很久，进度看页面 `apply_done / apply_total`。

2. **巡检虽有并发，但仍受上游与网络限制**。  
   - 默认 6 worker，可调到最多 16；调太高可能打满代理/上游或影响同机其他请求。

3. **异步 ≠ 瞬间完成**。  
   - 异步只保证：**点按钮后接口马上返回、status 还能刷、不堵 Management 主路径**。  
   - 实际改账号、删文件仍要按条完成，**总耗时可能很长**，这是正常现象。

4. **执行中的保护**  
   - `applying` 时：批量按钮灰掉，表格「操作」列显示 `-`，避免重复点击。  
   - 与巡检互斥：busy 时不能同时开新的巡检/批量。

**一句话：慢一点、稳一点，换的是 CPA 主体不被巡检插件拖死。**

---

## 3. 功能改动清单

### 3.1 巡检

| 功能 | 说明 |
|------|------|
| 完整巡检 | 清空当前结果，对 Auth 中符合条件的 xAI/Grok 账号全量探测 |
| 增量巡检 | 保留已有结果，只探测 Auth 中相对上次结果**新增**的账号（需先有结果） |
| 并发 | 默认 6，范围 1–16，前后端校验 |
| 探测内容 | 请求体为 **`ping`**（非 hello）；Responses 优先，失败再 Chat Completions |
| 分类 | 健康 / 权限被拒 / 额度用尽 / 需重登 / 模型不可用 / 探测异常等 |

### 3.2 结果落盘

| 项 | 说明 |
|----|------|
| 路径 | `data/grok-inspection/results.json`（相对 CPA 工作目录） |
| 覆盖 | 环境变量 `GROK_INSPECTION_DATA_DIR` |
| 内容 | **轻量展示字段**（无 token，不是 Auth 目录镜像） |
| 时机 | 巡检过程中周期性、结束时、禁用/删除后 |
| 注意 | Docker **无 volume** 时重建容器会丢；换插件 so/dll 本身不应清文件，完整巡检会清空列表 |

### 3.3 批量与单行操作

| 操作 | 范围 | 数量逻辑 |
|------|------|----------|
| 批量导出 | 当前卡片分类下**全部**条（非仅当前页） | 分类内条数 |
| 批量禁用 | 当前分类下 **已启用** | 仅统计可禁用数 |
| 批量启用 | 当前分类下 **已禁用** | 仅统计可启用数 |
| 批量删除 | 当前分类下全部 | 分类内条数 |
| 执行建议 | 全表有建议动作的行（**不受**卡片分类限制） | 建议条数 |
| 单行操作 | 该行建议 | — |

禁用/启用走 **host.auth.get + host.auth.save**（不重入 Management HTTP）。  
删除走本机 Management `DELETE /v0/management/auth-files`（须进程环境 `MANAGEMENT_PASSWORD` / `CPA_MANAGEMENT_KEY`），且只在后台执行，避免死锁；成功后同步改本地 JSON。

### 3.4 UI 调整

- 去掉与卡片重复的筛选按钮行；**只保留卡片**筛选（全部 / 健康 / 权限被拒 / 额度用尽 / 需重登 / **异常**）。
- 「异常」= 探测异常、模型不可用、未知等（**不是**「已禁用」账号集合）。
- 操作区：批量导出、批量禁用、**批量启用**、批量删除。
- 操作前 **弹窗**：标明当前分类、影响数量、确定/取消。
- 空闲停止 status 轮询。

### 3.5 CI / 构建

- GitHub Actions：push/PR 自动测试并编 Linux/Windows/macOS 产物。
- 修复 Node 20 弃用：升级 checkout / setup-go / upload-artifact / download-artifact。
- 无 `go.sum` 时关闭 Go module cache 警告。
- macOS runner 固定 `macos-15`。
- Artifact **只上传插件本体**，避免「zip 里再套 zip」。
- Go 版本对齐 1.22.x。

### 3.6 文档

- README：架构、工作流、按钮流转、全部接口、安装与 Docker `docker cp` 示例、检测结果与建议说明、文首跳转安装。

---

## 4. 接口一览（部署后）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v0/resource/plugins/grok-inspection/status` | 页面 HTML |
| GET | `/v0/management/plugins/grok-inspection/status` | 进度与结果（内存） |
| POST | `.../start` | 开始巡检（`incremental` 可选） |
| POST | `.../stop` | 停止巡检 |
| POST | `.../apply` | 建议或强制批量（`force_action` + `auth_indexes`），异步 202 |
| POST | `.../action` | 单行操作，异步 202 |

上游探测（经 host）：

- `GET https://cli-chat-proxy.grok.com/v1/models`
- `POST .../v1/responses`（input: `ping`）
- 必要时 `POST .../v1/chat/completions`

---

## 5. 涉及主要文件

| 文件 | 改动要点 |
|------|----------|
| `engine.go` | 异步 apply/action、增量巡检、落盘联动、禁用 host 路径、删除后台+超时 |
| `store.go` | `results.json` 读写 |
| `main.go` | 路由、202、workers 校验错误码 |
| `ui.go` | 卡片/批量/弹窗/轮询策略/启用禁用计数 |
| `classify.go` | （既有）分类逻辑 |
| `.github/workflows/ci.yml` / `release.yml` | 多平台构建与警告修复 |
| `README.md` | 使用与架构文档 |

---

## 6. 部署与自测建议

1. 用 Actions Artifact 取对应平台 `grok-inspection.so` / `.dll`。  
2. 覆盖 CPA 插件目录后 **重启**（Docker：`docker cp` + `docker restart`）。  
3. 浏览器 **强制刷新**；新版特征：6 张卡片、批量启用、操作弹窗写「当前分类」。  
4. 需要持久结果：给 `data/` 或 `GROK_INSPECTION_DATA_DIR` 挂 volume。  
5. 删除功能：CPA 进程需配置 Management 密码环境变量。  
6. **大批量操作请预留时间**，看进度条即可，勿重复连点。

---

## 7. 给使用者的预期话术（可直接转发）

> 巡检和批量操作都在 **CPA 插件进程后台**执行，点开始后接口会马上返回，页面用 status 看进度，**不会把管理接口一直占死**，尽量不影响主体业务。  
> 但账号一多，**批量禁用/启用/删除是逐条做的，不会很快**；这是为了保护 CPA 和 Auth，属于正常现象。请耐心等进度跑完，执行中请勿重复提交。

---

## 8. 版本与后续可选优化

- 插件版本号当前代码中为 **0.1.13**（以 `main.go` 为准；README / Release 默认 tag 已对齐）。  
- 单行操作需轮询 light `/status` 的 `recent_row_actions` 确认完成后再改 UI（202 仅表示已接受）。  
- 增量巡检跳过逻辑仅用 `auth_index`（优先）或 `file_id` / `file_name+size+mtime`，不再用邮箱/显示名单独匹配。

---

*文档日期：2026-07-13*
