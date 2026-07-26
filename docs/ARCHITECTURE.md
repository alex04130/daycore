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
- `docs/` = 实时项目文档，随代码同批更新。
- `extension/` = Chrome MV3 插件（Canvas 抓取 → 导入）。
- `design-ui/` = 前后端分离式前端的落地点，当前为空占位。
- 设计系统已 vendor 进 `web/frontend/src/ds/` 与 `src/vendor/ds-bundle.js`，仓库内不再保留设计交付原件（原件在 Claude Design 网页版）。
