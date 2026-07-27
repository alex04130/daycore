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
| `internal/rapport/` `internal/rhythm/` | 默契评分与主动性门控 / 节律学习 + 20h 关怀（均为纯函数，零存储，见 DATA.md） |
| `internal/schedule/` `internal/ics/` `internal/i18n/` | 规则展开引擎 / ICS 解析 / locale 协商（默认 en-US，支持 zh-CN）+ `Text`/`Pick` 翻译表与回退链，见 DATA.md |

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

关键项：`APP_ENV`/`HOST`/`PORT`；`STATIC_DIR`；`ALLOWED_ORIGINS`（CSV，空=同源）；`DB_TYPE`(sqlite)/`DB_DSN`；`JWT_SECRET`/`COOKIE_SECRET`（prod 缺失报错，dev 回退不安全默认）；`JWT_TTL`(168h)；`SECURE_COOKIES`（prod 自动 true）；`AI_REQUEST_TIMEOUT`(120s)；`AI_RATE_LIMIT_PER_MIN`(30)/`AUTH_RATE_LIMIT_PER_MIN`(10)；`AGENT_MAX_ROUNDS`(6)；`MAX_IMAGE_BYTES`(8MiB)；`ADMIN_TOKEN`；`ONEBOT_WS_URL`/`ONEBOT_TOKEN`；`MODELS_CONFIG`/`OAUTH_CONFIG`；天气三项；`COOKIE_SAMESITE`（lax|strict|none，none 需 Secure）；**`PRIMARY_LOCALE`/`SECONDARY_LOCALE`**（本部署给用户的一主一副，`Load()` 里 `i18n.Offer` 校验，值不对直接启动失败 —— 否则表现为所有用户默默看到错误语言；副语言留空 = 单语言安装，前端隐藏切换）。

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
