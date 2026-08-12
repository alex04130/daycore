# 开发者手册

> ⚠️ **本文引用的 `/api/…` 路径实际服务在 `/api/v2/…` 下**（2026-08-11 起，规则见 `internal/apipath`）。
> 正文按**资源**写，因为哪个 major 在服务它是另一件事、只决定一次。
> 三条例外留在 `/api/` 外面：`/api/version`（发现）、`/api/healthz`（存活探针）、
> `/api/auth/oauth/{provider}/callback`（注册在第三方控制台里的重定向目标）。
> 权威的完整线上路径见 `docs/API_SURFACE.md` 与 `api/openapi.yaml`。


> 从 README 搬来（2026-07-29）—— README 只留「这是什么 / 怎么跑起来」，其余瀑布式下沉到 `docs/`。

### 项目结构

```
cmd/daycore/main.go         装配：注册驱动/格式 → 打开 Store → 迁移 → 起 HTTP（优雅关闭）
internal/
  domain/                   实体 + 通用 Store/Repository 接口（界面分离核心）
  config/                   环境配置（HOST/PORT/STATIC_DIR/…）
  version/                  版本号唯一来源（构建版本 + API 契约版本，别混）
  auth/                     argon2id / JWT / OAuth / 会话 cookie
  i18n/                     locale 协商 + 三层消息目录（DB → LOCALES_DIR → 内嵌）+ 用户级一主一副 Pair
  ai/                       AIProvider 接口 + catalog + 视觉编排 + 提示词
    formats/{openai,anthropic,ollama}/   wire-format（自注册）
    prompts/<locale>/*.tmpl 提示词（14 key × 2 locale；PROMPTS_DIR 可逐文件覆盖）
    prompts/boundaries.json L1 硬边界（只有磁盘+内嵌两层，控制台改不到，见 AI.md）
  schedule/                 重复规则展开引擎（纯函数）
  ics/                      最小 iCalendar + RRULE 子集解析器（零依赖）
  timeutil/                 石化线与墙钟换算（纯函数）
  rhythm/ mood/ rapport/    节律学习 / 心情窗口 / 默契评分 —— 三个都是纯函数、零存储、读时派生
  channels/                 通道插件框架（Registry + OneBot 11 适配器）
  search/                   web 搜索（Tavily→DDG）+ MaterialSearcher（原生 FTS + 子串兜底）
  weather/                  WeatherProvider registry + 四个 provider 子包（自注册）
  storage/{sqlstore,mongostore}/  SQLite+PG+MySQL（Dialect 抽象）/ MongoDB
  storage/storagetest/      行为一致性套件（43 例，四个后端跑同一份）
  server/                   路由（分散注册，见 routes.go）+ 中间件 + handlers + agent loop + cron Worker
api/                        openapi.yaml（生成物）+ spec/（按 tag 分片的源）+ FRONTEND_HANDOFF.md
web/frontend/               React 前端（Vite；npm run dev / build）
deploy/                     Dockerfile / docker-compose / nginx
testdata/                   canvas-export.sample.json / sample.ics
docs/                       实时项目文档（架构/认证/Agent/数据/AI/路由总表）
extension/                  Chrome MV3 插件（抓 Canvas → POST /api/import/canvas）
design-ui/                  设计原型：四套范式级不同的前端 + 共享 mock + 交接文档（只读参考，不参与构建）
docs/specs/                 对外协议：传输层 / 存储适配 / provider 适配 / 前端 manifest
```

### 扩展点（registry / 驱动模式）

1. **新增数据库**：实现 `domain.Store` + `storage.Register("foo", opener)`（SQL 类只需加一个 `Dialect`），main.go 加 blank import。**验收标准是 `internal/storage/storagetest` 的行为套件通过** —— 它测的是四个后端必须一致的行为，不是「能存能取」。不想写 Go 的话，将来可以走 [`docs/specs/storage-protocol.md`](specs/storage-protocol.md) 的 HTTP/子进程协议。
2. **新增 AI 厂商格式**：实现 `ai.AIProvider` + `ai.RegisterFormat("gemini", New)`，`models.yaml` 引用 `format: gemini`。
3. **新增模型（零代码）**：编辑 `config/models.yaml`，重启生效；视觉模型标 `vision: true`。
4. **新增 OAuth provider（零代码）**：`config/oauth.yaml` 加条目，回调 `<PUBLIC_BASE_URL>/api/auth/oauth/<name>/callback`。
5. **编辑提示词（运行时）**：`PUT /api/admin/prompts/{key}?locale=zh-CN|en-US`（`X-Admin-Token`），覆盖存 `prompt_overrides`。

### 开发命令

```bash
make run / test / vet / build / docker
make test-mongo                                     # 行为套件对真机 MongoDB 跑（需本机 mongod）
cd web/frontend && npm run dev / build
node web/frontend/scripts/check-i18n.mjs            # zh-CN / en-US key 对齐校验
```

### 端到端冒烟（curl）

见 `testdata/` 夹具；核心流：`POST /api/session/init` → `POST /api/import/token` → `POST /api/import/canvas`（X-Import-Token）→ `POST /api/import/ics` → `POST /api/rules` → `GET /api/plan?date=`（规则虚拟合并）→ `POST /api/ai/auto-plan` → SSE `/api/ai/companion`（**工具调用**，不是早先的 `<rule_update>` 标签协议；帧格式见 `api/FRONTEND_HANDOFF.md` §B）→ `GET /api/memory`。

### 相对 v1 的改进

- 主模式从「手动输入日程」变为「自主规划」（资料导入 → auto-plan → 聊天微调）。
- 新增重复/长期日程、Canvas/ICS/截图导入、每用户长期记忆、自定义主题 + AI 配色、i18n（含提示词双语）、版本体系与单二进制静态托管部署。
- 修复无鉴权数据端点（服务端签名 httpOnly cookie）、`incrementInteractionCount` 占位符 bug、日程加载 N+1、v1 MoodScreen `exerciseOffered` 异步 bug（前端已按正确方式实现）。

---


## 代码约定

> 2026-07-29 从 `AGENTS.md` 搬来 —— 那份文档里只有这几节是别处没有的，其余六成与本目录重复且更旧。

### 副作用归属

**副作用永远服务端执行。** 前端决不能直接调 store 写操作 —— 必须经过 agent 工具或 HTTP handler。即使将来加批量操作，也走 `POST /api/...` 由服务端执行、写日志、返回结果。这条保证了每个操作可审计、可撤销 —— 撤销体系整个建立在它上面。

**所有业务代码只 import `domain` 包**，绝不直接引用 `sqlstore` / `mongostore`。启动时 `main.go` 把 `Store` 传给 `Server`，之后 handler 完全不知道底层是哪个数据库。

**AI 只在服务端调用**：`ai.Chat` / `ai.ChatStream` 只出现在 `internal/server/` 的 handler 或 agent 循环里。

**永不信任客户端上送的上下文**：companion handler 不接收 `todayPlan` / `moodHistory` / `memoryContext` / `date` / `weekday` / `time`，全部由服务端从 store 组装。（客户端上送的对话历史会被角色白名单过滤 —— 见 `handlers_ai_companion.go` 里那段注释：客户端不能给自己注入一个 `system` 轮次。）

### 操作日志

所有写路径（plan add/update/remove/upsert/auto-plan、rule create/update/delete/batch、memory add/delete/clear，以及所有 agent 工具执行）必须调 `s.logOp`：

```go
func (s *Server) logOp(ctx context.Context, l *domain.OperationLog) string {
    if l.ID == "" { l.ID = uuid.NewString() }
    if l.Actor == "" { l.Actor = domain.ActorUser }
    if l.Status == "" { l.Status = domain.OpStatusOK }
    if l.RequestID == "" { l.RequestID = requestIDFrom(ctx) }
    _ = s.store.OpLogs().Add(ctx, l)
    return l.ID  // 返回 opID 供撤销链
}
```

best-effort（`_ =` 丢错），`detail` 存 before/after 快照 —— **撤销是从 before 快照逐键重建的**，所以快照不全等于那条操作撤不回来。

### 错误处理，四类各有约定

| 哪一层 | 怎么报 | 为什么 |
|---|---|---|
| Agent 工具失败 | `toolResult{OK: false, ErrMsg: "…"}` | **不中断 loop** —— 作为 `role=tool` 消息注回上下文，模型看到错误可以改参数重试一次，或者向用户口头解释 |
| HTTP handler | `s.writeErr(w, status, stableCode, humanMessage)` | `stableCode` 给前端做 i18n，`humanMessage` 只是回退。**不要只给 humanMessage** —— 那样前端只能拿字符串匹配 |
| 存储层「找不到」 | `domain.ErrNotFound` | 所有 repo 统一用它，行为一致性套件断言这一点 |
| AI 流出错 | `chunk.Err` → SSE error 帧 + done | 客户端必须收到 done，否则它会一直转 |

### 文件命名

- Go：`snake_case.go`
- React：`PascalCase.jsx`（页面/组件）、`camelCase.js`（工具/状态）

## 加东西的分步骨架

### 加一条 API 路由

1. 在 `internal/server/` 下写 handler（`sid, ok := s.requireSession(w, r)` 开头）。
2. **在同一个文件的 `init()` 里注册**：`registerRoutes("<组名>", func(s *Server, mux Mux) { mux.HandleFunc("GET /api/xxx", s.handleXxx) })` —— 不要去 `server.go`，那里已经没有集中清单了（见 ARCHITECTURE.md「路由注册模式」）。
3. **写进契约**：改 `api/spec/paths/<tag>.yaml`（文件名必须等于 operation 的 tag），然后 `make api-bundle`。跳过这步 `go test ./...` 会红 —— `routes_test.go` 与 openapi 双向核对。
4. **契约面变了就升版**：`internal/version/version.go` 的 `APIMinor`（additive）或 `APIVersion`（breaking）。一批只升一次，`api/spec/contract-lock.json` 会要求至少升过一次。
5. `make api-surface` 重生成 `docs/API_SURFACE.md`。
6. 前端要用就在 `store.js` 加函数调 `api.get/post/patch/del`；有新文案就在 `i18n.js` 两个语言块各加一条。

### 加一个 Agent 工具

1. 在 `agent_tools.go` 的 `companionToolDefs` 里加 `ai.ToolDef{Name, Description, Parameters: schemaObj(...)}`。**如果这个工具在某些场景下不可能工作，就别把它加进工具带** —— 先例是 `interactive`：回不了卡的沉降口不给 `propose_decision`，因为「不给」胜过「让 agent 等一个永远等不到的答复」。
2. 在 `runCompanionTool` 的 switch 里加 case。
3. 照 `toolMemoryAdd` 的骨架实现：

```go
func (s *Server) toolMyNewTool(ctx context.Context, sid, rawArgs string) toolResult {
    var args struct{ /* … */ }
    if err := json.Unmarshal([]byte(rawArgs), &args); err != nil { return toolFail("…") }
    // 调 s.store.* / s.weather.* / s.search.*
    opID := s.logOp(ctx, &domain.OperationLog{ /* … */ })
    return toolResult{OK: true, OpID: opID, Data: map[string]any{}, Summary: "…"}
}
```

4. **写操作要能撤销**：在 `handlers_ops.go` 用 `registerRevert(action, handler)` 注册逆操作，每种 `action` 至少一个用例。
5. 影响前端缓存（plan/rule/memory）的，在 `store.js` 的 `applyToolResult` 里加映射。

### 加一个模型

只改 `config/models.yaml` 一条记录，零代码 —— 只要 `format` 名已注册。重启生效。

### 加一门语言

丢一个 `<locale>.json` 进 `LOCALES_DIR`，或从控制台粘一份进 DB。**不改代码、不发版。** 前端目前还不是这样（`i18n.js` 是硬编码字典）。

## 项目规则

- **实现域唯一**：仓库根即实现面，后端在 `internal/`、前端在 `web/frontend/`。`design-ui/` 是只读设计参考，不要往里写业务代码。
- **无包袱直切**：Beta 阶段不做双轨/灰度/迁移脚本。前后端同 PR 合入，回滚靠 git revert。
- **测试是回归网**：日期解析、tool_call 解析、agent loop、operation_logs、存储行为一致性全覆盖。`make test` 先跑 i18n 校验再跑 Go test。
- **非构建/验证期的 Bash 不要做危险操作**（删库、`rm -rf` 等）。


### 文档去哪找什么

| 想知道 | 看 |
|---|---|
| **做什么、按什么顺序、为什么是这个顺序** | `docs/ROADMAP.md` |
| 这个仓库现在是什么样、为什么这样 | `docs/ARCHITECTURE.md` `docs/DATA.md` `docs/AUTH.md` `docs/AGENT.md` `docs/AI.md` |
| 有哪些 HTTP 路由 | `docs/API_SURFACE.md` + `api/openapi.yaml`（唯一权威；**改的是 `api/spec/paths/<tag>.yaml` 然后 `make api-bundle`**） |
| 端无关的产品语义（时间三层、提案、注意力阶梯、默契） | `docs/EXPERIENCE_CORE.md` |
| **别人要照着实现什么**（适配器、前端） | `docs/specs/` |
| 前端要对接的 API 细节 | `api/FRONTEND_HANDOFF.md` |

**实时文档铁律**：任何代码改动必须在同一批修改中更新 `docs/` 对应文件。文档与代码不同步视为改动未完成。

