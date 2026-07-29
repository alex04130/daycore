# 架构总览

> 实时文档：改代码必须同批更新本目录对应文件。最后全面核对：2026-07-14。

## 包结构（仓库根，Go module `daycore`，go 1.23）

| 包 | 职责 |
|---|---|
| `cmd/daycore/` | 入口 main.go（装配 config/store/catalog/prompts/server/worker/channels）+ install.go（prompt 模板导出器，walk embed FS，新模板自动导出） |
| `internal/domain/` | 纯数据结构 + Repository 接口，零外部依赖 |
| `internal/server/` | HTTP 路由、中间件、全部 handler、agent loop、cron Worker |
| `internal/storage/sqlstore/` | SQL 三方言（SQLite/PostgreSQL/MySQL），每实体一文件 |
| `internal/storage/mongostore/` | MongoDB 实现，每实体一 repo 文件（无测试） |
| `internal/ai/` | AIProvider 抽象、Catalog、PromptService（embed+DB override）、流式协议、vision 管线 |
| `internal/auth/` | 密码(argon2id)/OAuth/JWT(token.go)/签名 cookie(session.go) |
| `internal/channels/` | 通道插件框架（Registry + OneBot 11 适配器） |
| `internal/config/` | 环境变量配置（godotenv，见下） |
| `internal/search/` | web 搜索（Tavily→DDG）+ MaterialSearcher（原生 FTS 优先 + 子串兜底，见 DATA.md） |
| `internal/weather/` | WeatherProvider registry（open-meteo/qweather/owm/wttr.in，30min 缓存） |
| `internal/version/` | 版本唯一真源：Version="2.2.0" Channel="beta"；APIVersion 契约常量（阶段 1 加） |
| `internal/rapport/` `internal/rhythm/` `internal/mood/` | 默契评分与主动性门控 / 节律学习 + 20h 关怀 / 心情窗口（趋势+衰减+新鲜度）—— 三个都是纯函数、零存储、读时派生，见 DATA.md |
| `internal/schedule/` `internal/ics/` `internal/i18n/` | 规则展开引擎 / ICS 解析 / locale 协商 + **三层消息目录**（DB → 文件 → 内嵌 zh-CN·en-US）+ 用户级一主一副 `Pair`，见 DATA.md |

## 推荐 MongoDB，但四个后端都要能跑（2026-07-29）

**MongoDB 是推荐部署**，理由是这个数据模型确实更贴它：主题变量、提案的 rows/ops、rapport 分数、节律日 —— 一半的新实体本来就是自由文档，Mongo 存它们不需要「塞进一个 JSON 列再整体重写」。

**但兼容不是可选的。** 推荐与要求之间那条线要划清楚，否则「支持四个后端」会悄悄变成「只有 Mongo 真的测过」—— 而今天之前 `mongostore` 恰好是那个**零测试**的，方向还是反的。现在两边都跑同一套行为套件（见下）。

### SQL 侧的取舍规则：条件写用列，其余用 JSON

「大键值是个 JSON、每次整体重写」在**大部分**字段上是对的取舍 —— 省掉三方言 DDL、加字段零迁移、与 Mongo 的形状对齐。但它不能一刀切，因为整体重写**放弃了条件写**：

> **凡是出现在 `WHERE` 里、或被算术/`CASE` 更新的字段，必须是列。其余一律可以进 JSON blob。**

批次 C 已经是这么分的，把规则写下来是为了后面不走偏：

| 必须是列（参与条件写） | 可以是 JSON blob |
|---|---|
| `rev`（CAS）、`state`、`expires_at`、`ttl_policy`、`delivered_at`、`deliver_after`、`merge_key`、`created_at` | `rows_json`、`ops_json`、`applied_op_ids`、`accept_op_ids` |
| `first_min`/`last_min`/`signals`（`CASE WHEN` 扩边界） | — |
| `last_signal_at`/`run_since`（`WHERE last_signal_at < ?`） | — |
| `attempts`、`status`、`started_at`（占有与接管判定） | `error_text` |
| `fence`、`holder`、`expires_at`（选主） | — |
| `cursor_created_at`/`cursor_id`（游标续读） | `scores_json` |

把 `first_min` 塞进 blob，扩边界就退成读-改-写 —— 两个标签页开着就丢信号。这不是性能取舍，是正确性取舍。

### 行为一致性套件（`internal/storage/storagetest`，已落地）

一套按 `domain.Store` 写的行为套件，**两个后端都跑同一份**：sqlstore 用 SQLite，mongostore 用真机 Mongo（`MONGO_TEST_DSN` 未设则跳过，`make test-mongo`）。29 个用例，全部来自今天审查抓到的真实分歧 —— 不是「能存能取」，而是：

lease 只有一个持有者且 fence 只在交接时动 / `Acquire` 永不返回别人的行 / 空 holder 被拒 / 场次占有互斥 / 完成的场次不再被占 / 失败重试到上限 / 崩溃接管有界且回报真实 attempts / **接管轮换占有令牌使僵尸的 `Finish` 落空** / `Prune` 保留 running / nil 切片回来是空切片而非 nil / 指向零值时间的指针算「不存在」 / **`ProposalOp.Args` 的数字在每个后端都回来是 `float64`** / `Validate` 在 `Create` 与 `Update` 两侧都生效 / `rev` CAS 拒绝陈旧写 / TTL 不对称 / **同毫秒并列时 Supersede 恰好留一张** / 已投递的卡不被退休 / keeper 缺失是非事件 / 可投递集合排除过期与压后 / 序列化失败拒绝写入 / rapport 游标往返 / **学习作业不擦掉活的清醒标记** / `Touch` 只向前 / 分钟 0 是有意义的值 / 并发首写不丢信号 / 语言包往返与整语言卸载 / `RevertedBy` 精确且不跨会话 / `Scan` 最旧优先且游标续读无重无漏。

**加后端的验收标准就是这套套件通过**，包括计划中的 HTTP 转换层。这也是让第五个后端负担得起的唯一办法：两两分歧数随后端数平方增长，共享套件把它压平。

⚠️ CI 里没有 Mongo，所以 CI 只跑 SQLite 那一半。真机那半靠 `make test-mongo` 手动跑 —— 这是已知缺口，不是「测过了」。

## 存储的第五种后端：HTTP / 子进程转换层

**完整协议在 [`docs/specs/storage-protocol.md`](specs/storage-protocol.md)**，传输规则在 [`specs/transport.md`](specs/transport.md)。这里只记为什么与顺序。

「转换层 + 内部高效适配」这个模式扩到存储层是对的，天气/搜索/通道（F2）已经这么设计。但**最小接口不是基础 CRUD**：

`domain.Store` 有 179 个方法、**20 处条件写**。它们不是优化，是这个仓库唯一的互斥手段（**全包无事务**，`grep BeginTx` 零命中）—— 选主、任务场次占有、提案行级接受、节律日边界、清醒标记。纯 CRUD 表达不了任何一条，**少了条件写这些保证会全部静默降级成「通常能用」**，且降级不报错，只在并发下偶尔出错。

协议底线是 **CRUD，其中 insert 在 id 冲突时失败而不是覆盖**。有了这一个原语，条件写要么原生支持、要么适配层用一把锁补出来（代价：一次写从 1 个往返变成 4 个；而一把**错**的锁比没有锁更糟 —— 没锁是偶尔丢更新，锁错是整张表卡死。配方与四条硬要求见协议文档）。

**传输两种**：`http`（服务，可远端、可扩容）与 `exec`（后端监管的子进程）。`exec` 不是降级 —— 崩溃给**退出码 + stderr**，而 HTTP 适配层崩了只剩 `connection refused`。顺带记下：Go 的 `plugin` 包不适合第三方生态（仅 Linux/macOS、要求完全相同的工具链与依赖版本、无法卸载），所以「子进程 + 协议」不是绕开 Go 的弱点，**它就是 Go 的标准答案**。

⚠️ **顺序：先套件，后后端。** 批次 C 的审查在现有两个后端之间抓到约十五处行为分歧，而当时没有任何测试断言两者行为相同。再加一个后端只会把分歧从 O(1) 对变成 O(n²) 对。

### 行为一致性套件（`internal/storage/storagetest`，已落地）

按 `domain.Store` 写的一套行为套件，**两个后端跑同一份**：sqlstore 用 SQLite，mongostore 用真机 Mongo（`MONGO_TEST_DSN` 未设则跳过，`make test-mongo`）。29 个用例，全部来自审查抓到的真实分歧 —— 不是「能存能取」，而是 `ProposalOp.Args` 的数字必须回来是 `float64`、接管必须轮换占有令牌使僵尸的 `Finish` 落空、学习作业不能擦掉活的清醒标记、同毫秒并列时 `Supersede` 恰好留一张、`Scan` 游标续读无重无漏。

**加后端的验收标准就是这套套件通过**，HTTP 转换层也一样。

### SQL 侧存自由数据的三种写法

「一个大 JSON 每次整体重写」是最省事的一种，但不是唯一的，也不总是最好的：

| 写法 | 适用 | 代价 |
|---|---|---|
| **JSON 路径写**：`json_set(doc,'$.k',?) WHERE json_extract(doc,'$.k') > ?` | 自由结构**且**需要条件写 | 三方言语法不同（`json_set`/`jsonb_set`/`JSON_SET`），要走 `Dialect` 方法；索引靠生成列或表达式索引 |
| **侧表** `(entity_id, key, value)` | 键集开放**且**要按键单独查 | 一次 join、行数放大 |
| **热字段提列 + 其余 blob** | 大多数情况 | 加字段要动 DDL |
| **整包 blob 重写** | 从不被查询、从不被条件写 | 放弃条件写 |

已实测：SQLite（modernc，带 JSON1）**一条语句就能做条件式部分更新**，`json_set` 配 `WHERE json_extract(...)` 影响行数正确。Postgres 是 `jsonb_set`/`->>`，MySQL 8 是 `JSON_SET`/`->>`。

**Mongo 不是「不需要」，是同一个问题的另一种写法** —— `rhythmRepo.Observe` 用的 `$min`/`$max` 就是路径写，与 SQL 的 `json_set(...) WHERE json_extract(...)` 同构。所以「统包 blob」在两边都不是必需的。

**主题变量是侧表更合适的那一类**：F7 的补算要问「哪些主题缺 token X」，侧表是一个 `WHERE`，blob 是全表扫加逐个解析；「给所有主题加一个 token」也变成每主题一次 INSERT 而不是整体重写。

## 中间件链（server.go 底部，全局单链，无分组）

```
recoverMW → requestIDMW → loggingMW → corsMW → sessionMW → userMW → dataSessionMW → mux
```

三个身份中间件是**「解析不强制」**：cookie/header 有效才往 ctx 注入，从不拦截。真正的鉴权在 handler 内 `requireSession`（无 sid → 401 no_session）或 `adminAuthorized`。详见 AUTH.md。

## 路由注册模式

`server.go` 的 `New()` 里 `mux.HandleFunc("METHOD /path", s.handleXxx)` 集中注册（60+ 条），路由→handler 文件映射见 API_SURFACE.md，REST 细节以 `api/openapi.yaml` 为准。

## main.go 启动/关停

- 启动顺序：config.Load → store.Open+Migrate → catalog/prompts → server.New → Worker（cron：早报 07:30/晚报 21:00/2h deadline/30min replan，经绑定通道推送）→ channels Registry.StartAll(rootCtx) → inbound 消费循环。
- inbound 消费：每消息经 `srv.GoTracked` 起 goroutine（背景 ctx，不绑请求），纳入 `Server.asyncWG`。
- 启动清扫：Migrate 后 `store.Chats().FailPendingMessages` 把崩溃遗留的 pending 占位消息标为 error。
- 关停：SIGINT/SIGTERM → `httpSrv.Shutdown(15s)` 等 in-flight HTTP → `srv.WaitBackground(shutdownCtx)` 等后台 agent（异步聊天/通道回复）→ `worker.Stop()`（等 `cron.Stop().Done()`）→ `cancelRoot()`。TempContext/ChannelBinding 两个 cleanup ticker 仍是自由 goroutine（无状态，进程退出即弃，无碍）。

## 静态托管（static.go）

- `STATIC_DIR`（默认 `web/frontend/dist`）非空且有 index.html 时：`/api/` 前缀永不被静态遮蔽；`/assets/` immutable 长缓存；其余 SPA fallback 回 index.html（no-cache）。
- `STATIC_DIR=""` 或无构建产物 → **纯 API 模式**（不挂 `/` 路由）。`deploy/Dockerfile` 不 COPY 前端产物，本就是纯 API 镜像。

## 配置（internal/config/config.go，环境变量）

关键项：`APP_ENV`/`HOST`/`PORT`；`STATIC_DIR`；`ALLOWED_ORIGINS`（CSV，空=同源）；`DB_TYPE`(sqlite)/`DB_DSN`；`JWT_SECRET`/`COOKIE_SECRET`（prod 缺失报错，dev 回退不安全默认）；`JWT_TTL`(168h)；`SECURE_COOKIES`（prod 自动 true）；`AI_REQUEST_TIMEOUT`(120s)；`AI_RATE_LIMIT_PER_MIN`(30)/`AUTH_RATE_LIMIT_PER_MIN`(10)；`AGENT_MAX_ROUNDS`(6)；`MAX_IMAGE_BYTES`(8MiB)；`ADMIN_TOKEN`；`ONEBOT_WS_URL`/`ONEBOT_TOKEN`；`MODELS_CONFIG`/`OAUTH_CONFIG`；天气三项；`COOKIE_SAMESITE`（lax|strict|none，none 需 Secure）；**`DEFAULT_PRIMARY_LOCALE`/`DEFAULT_SECONDARY_LOCALE`/`LOCALES_DIR`**（前两个是**新用户的默认**一主一副，不限制用户能选什么，`Load()` 里 `i18n.NewPair` 校验、值不对启动失败；`LOCALES_DIR` 放 `<locale>.json` 语言包 —— 加语言不用重新编译，见 DATA.md「多语言机制」）。

## 仓库级布局

- `api/` = 契约唯一权威（openapi.yaml + FRONTEND_HANDOFF.md）。
- `docs/` = 实时项目文档，随代码同批更新；`EXPERIENCE_CORE.md` 是端无关语义总纲（v2.3）。
- `extension/` = Chrome MV3 插件（Canvas 抓取 → 导入）。
- `design-ui/` = 设计原型，只读参考不参与构建。四端源码（liuli / zhiyu / ting / liuli-classic）+ `core/daycore-core.js` 共享 mock + `HANDOFF/` 交接文档 + `_ds/` 设计系统原件。其 `API_CONTRACT.md` 的路径命名非权威（见该目录 CLAUDE.md 顶部裁决）。
- `web/frontend/` = 现役前端，将来被四端替换。
- 设计系统已 vendor 进 `web/frontend/src/ds/` 与 `src/vendor/ds-bundle.js`；原件在 `design-ui/_ds/`。

## 目标架构：前后端分离（进行中）

后端收敛为纯 API 服务，四个前端各自独立部署、做成 git 子仓库，只靠 **API 契约 + 版本协商**耦合：

- **后端报告**：`GET /api/version` → `{apiVersion, apiMinor, minClient, build, channel}`
- **前端声明**：各子仓 `package.json` 声明最低支持的 `apiVersion`/`apiMinor`，启动握手不满足则降级提示（不白屏 —— 语气铁律「给死路一条岔路」）
- **升版规则**：breaking 升 `APIVersion`，additive（新端点/新字段）升 `APIMinor`。整批工作统一升一次，不要每个改动各升各的。

版本三层的区分见根 `CLAUDE.md`「目标架构」一节。

## 配置分层（现状与待改造，2026-07-26 核实）

配置目前分三处，**问题是「能不能热改」没有被设计过**：

| 层 | 载体 | 生效方式 | 现状 |
|---|---|---|---|
| 启动期 | 25 个环境变量（`internal/config/config.go`） | 改了要重启 | 合理的部分：密钥、`DB_DSN`、`HOST`/`PORT` |
| 同上 | 但也塞了本该热改的：`WEATHER_PROVIDER`、`TAVILY_API_KEY`、`ONEBOT_WS_URL`、各阈值 | 改了要重启 | **错位** —— 运维控制台永远配不到 |
| 数据驱动 | `config/models.yaml`、`config/oauth.yaml` | 重启加载 | 方向对，但无端点、多实例不同步 |
| 运行时覆盖 | `prompt_overrides` 表 + `PUT /api/admin/prompts/{key}` | 立即生效 | **唯一做对的范式**，模型/OAuth/服务配置应照抄 |

两处 registry 不一致，也在待改造之列：

- `internal/weather` —— `Register(name, Factory)` + 四个 provider 子包 init 自注册 + `WEATHER_PROVIDER` 选择。**这是正确形状**。
- `internal/search` —— 硬编码 `if TavilyKey != "" { Tavily } else { DuckDuckGo }`，**无注册表**，物理上无法增删搜索通道。
- `internal/channels` —— 有 `Registry`，但 `cmd/daycore/main.go:141` 是 OneBot 硬编码单例，且**拿 `ONEBOT_WS_URL` 是否为空来决定要不要启动 Worker** —— 后果是没绑通道的用户，节律学习、定时 auto-plan、20h 关怀全都不跑。这是既有缺陷，落地节律后会非常显眼。

运维控制台（`design-ui/liuli/admin/`，8 个分区，**留在主仓库不做独立子仓库**）依赖的 `PUT /api/admin/{config,models,oauth}`、`POST /api/admin/models/test`、`restart-ack` 目前**全部不存在**；已有的只有 prompts / stats / ailogs / users / db 五组。

## 目标：外部能力走 HTTP 适配器，不再改代码

**接一个新的搜索源／天气源／消息通道，不应该需要改 Go 代码或写插件** —— 只应该需要写一个 HTTP 适配层，它可以独立部署、放在别的服务器、独立扩缩容。

设计沿用 `config/models.yaml` 已验证的模式（`format` 绑定 registry 里注册的实现，`base_url` 指外部服务），推广到 weather / search / channels：内置 provider 保持 Go 实现走各自 format，**外部适配器统一走 `format: http`**。对上层透明 —— `WeatherProvider` / `Searcher` / `Channel` 三个接口一行不用改。

两类协议形状不同，**载体也不同**：

**查询型（天气、搜索）= HTTP** —— 无状态请求／响应，天然可并发，WS 反而累赘：
```
GET  /manifest   → {name, displayName, logo, type:"query", capabilities:["weather"|"search"]}
POST /weather    {lat, lon, date, tz}   → {temp, condition, …}
POST /search     {query, limit, locale} → {results:[{title, url, snippet}]}
```

**通道型（QQ/napcat 等）= WebSocket** —— 双向、有状态、需要身份。只在真的收发消息时才有流量。帧协议：
```
后端 →  {"t":"hello", "protocol":1}
适配器 → {"t":"manifest", "name","displayName","logo","features":{attachments,markdown}}
适配器 → {"t":"inbound", "externalUserId","externalName","avatar","messageId","text","attachments","ts"}
后端 →  {"t":"send", "to":"<externalUserId>","text","replyTo":"<messageId>"}
后端 →  {"t":"ping"}   // 断线重连沿用 onebot 现有逻辑
```

**连接方向必须两种都支持** —— 取决于谁在 NAT 后面，协议不该替部署做决定：

| `mode` | 谁连谁 | 适用 |
|---|---|---|
| `dial` | 后端作 client 连出去（适配器当 server） | 适配器有可达地址。**现有 onebot 就是这种**（`websocket.DefaultDialer.DialContext`） |
| `listen` | 后端开 WS 端点，等适配器连进来 | 适配器在 NAT／内网后面，后端有公网地址 |

OneBot 11 本身就规定了三种接入方式，现有实现只覆盖第一种，都要补：

| `providers.yaml` 的 mode | OneBot 官方术语 | 现状 |
|---|---|---|
| `dial` | 正向 WebSocket（napcat 当 server） | ✅ 已实现 |
| `listen` | 反向 WebSocket（napcat 主动连后端） | ⬜ 待补 |
| `http` | HTTP 上报 + HTTP API 调用 | ⬜ 待补 |

`internal/channels` 的 `Channel` 接口（`Send(externalID, msg)` + `Start(inbound chan<-)` + `Stop()`）对三种模式**都适用** —— `Start` 里是拨出去还是挂个 handler 等连接，是实现细节。**零接口变更**。

`manifest` 必须自带 `displayName` 与 `logo` —— 控制台与前端据此渲染，**接一个新通道零前端改动**。

### 内置 vs 外部：写了的直接配置，没写的走转换层

| 能力 | 内置（Go 实现，直接配置） | 外部（写适配层） |
|---|---|---|
| 天气 | openmeteo / qweather / openweathermap / wttrin（已有四个） | `format: http` |
| 搜索 | tavily（有 key）+ DuckDuckGo（免费兜底） | `format: http` |
| 通道 | napcat/OneBot（已有）+ 后续常见可接 bot 的平台 | `format: ws` |

### 搜索有三层来源，第一层目前是死代码

1. **模型厂商原生搜索** —— 走 **Anthropic 的 web search server tool 规范**（DeepSeek 的 Anthropic 兼容端点即此；kimi 等同类可比照）。**结果不是「融进回答的黑盒」** —— 规范返回的是 `web_search_tool_result` 块，其 `.content` 是一个 **`web_search_result` 结构化列表**（带 citations），后端拿得到、可落库、可展示来源。

   ⚠️ **现状：完全未接线（2026-07-26 核实）**。`config/models.yaml` 的 `deepseek_search: true` 是死配置：
   - `Capabilities.DeepseekSearch` 只被写入（`internal/ai/models.go:86`），**没有任何消费方**
   - `ToolDef.ServerSide`（`internal/ai/provider.go:44`）**全仓零赋值**
   - `internal/ai/formats/anthropic/anthropic.go:106` 序列化 tools 时只输出 `{Name, Description, InputSchema}`，**没有 type-based server tool 分支**
   - 同文件只解析 `text` / `tool_use` / `content_block_delta` / `message_stop` 四种块，**不认识 `server_tool_use` 与 `web_search_tool_result`**

   要接通需要两件事：发请求时按 `{"type": "web_search_20250305", "name": "web_search", "max_uses": N}` 序列化，收响应时解析 `web_search_tool_result` 并把 `.content` 映射成 `SearchResult`。

2. **内置工具搜索** —— agent 调 `web_search` 工具 → `Searcher` 接口 → tavily / DuckDuckGo。已实现。
3. **外部适配器** —— 协议转换层，`format: http`。

三层的**执行位置不同，结果模型应当统一**：第 1 层由模型服务端执行，第 2、3 层由后端执行，但都产出「标题 + URL + 摘要」的列表，都该落到同一个 `SearchResult`，这样账本、引用展示、前端渲染只有一套。

### 厂商原生搜索有两种形状，配置入口不同

| 形状 | 机制 | 配置放哪 |
|---|---|---|
| **返回独立结果** | server tool 规范，响应里带结构化列表（Anthropic `web_search_tool_result` 即此，DeepSeek 走它） | **搜索配置** —— 要注册工具、要解析结果、要映射成 `SearchResult` |
| **直接嵌入提示词** | 没有结构化返回，靠提示词引导模型自己去搜并在回答里带上 | **模型配置** —— 本质是一段随模型走的提示词片段 |

模型有没有原生搜索、是哪种形状，由 `models.yaml` 的能力声明表达（把现有的 `deepseek_search` 布尔泛化成 provider 无关的 `server_search`）。

### 多个搜索源并存：把选择权交给模型

不做「后端挑一个搜索源」的硬路由，而是**把多个搜索源各自注册成工具**，每个带自己的描述，模型按当前问题自己选（这与体验内核共识 5「不设关键词硬规则、错了再改」同构）。描述通过提示词模板动态注入：

```gotemplate
{{if .Searches}}
## 可用的搜索来源
{{range .Searches}}- `{{.ToolName}}`：{{.Description}}
{{end}}{{end}}
```

## 配置驱动的提示词片段：哪里用得到，哪里就能改

**提示词片段应该跟着配置走，就近编辑，而不是全堆在 Prompt 管理页。** 一个搜索源该怎么描述给模型、一个消息通道有什么特性（QQ 不支持 markdown、回复要短），都是那个 provider 自己的属性 —— 编辑入口就应该在它自己的配置卡片旁边，而不是让人去 Prompt 页翻一个巨大的模板。

现有基建已经够用，不需要新机制：

- `internal/ai/prompts.go` 用 `text/template`，`Render(ctx, key, locale, data any)` 接任意数据 —— `{{if}}` / `{{range}}` 天然可用，且现有模板里已有 5 处 `{{if}}` 先例。
- `PromptService` 有 `Validate`（parse 校验），控制台保存模板时能挡住语法错误。
- `prompt_overrides` 表 + `PUT /api/admin/prompts/{key}` 已是「文件作种子 + DB 存覆盖 + 立即生效」的正确范式，provider 描述照搬。

落地形状：每个 provider 在 `providers.yaml` 里带一段 `description`（**zh-CN / en-US 双份 —— 提示词双 locale 是启动期硬校验**），控制台在该 provider 的配置卡片旁给「编辑描述」入口，改动落 DB 覆盖层，`companion_agent.tmpl` 用 `{{range}}` 消费。同一套机制覆盖搜索源、消息通道、天气源。

⚠️ `companion_agent.tmpl` 目前是**零插值**的纯规则清单（有意为之，见 `AGENTS.md`）。引入 `{{range}}` 是对它的第一次结构性改动，两个 locale 必须同批改。

落地计划见批次 F。
