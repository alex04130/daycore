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
| `internal/schedule/` `internal/ics/` `internal/i18n/` | 规则展开引擎 / ICS 解析 / locale（默认 en-US，支持 zh-CN） |

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

关键项：`APP_ENV`/`HOST`/`PORT`；`STATIC_DIR`；`ALLOWED_ORIGINS`（CSV，空=同源）；`DB_TYPE`(sqlite)/`DB_DSN`；`JWT_SECRET`/`COOKIE_SECRET`（prod 缺失报错，dev 回退不安全默认）；`JWT_TTL`(168h)；`SECURE_COOKIES`（prod 自动 true）；`AI_REQUEST_TIMEOUT`(120s)；`AI_RATE_LIMIT_PER_MIN`(30)/`AUTH_RATE_LIMIT_PER_MIN`(10)；`AGENT_MAX_ROUNDS`(6)；`MAX_IMAGE_BYTES`(8MiB)；`ADMIN_TOKEN`；`ONEBOT_WS_URL`/`ONEBOT_TOKEN`；`MODELS_CONFIG`/`OAUTH_CONFIG`；天气三项；`COOKIE_SAMESITE`（lax|strict|none，none 需 Secure）。

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

运维控制台（`design-ui/liuli/admin/`，8 个分区）依赖的 `PUT /api/admin/{config,models,oauth}`、`POST /api/admin/models/test`、`restart-ack` 目前**全部不存在**；已有的只有 prompts / stats / ailogs / users / db 五组。
