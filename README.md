# Daycore — AI 自主规划日程助手 / AI Autonomous Day Planner

**v2.3.0-beta** · Go 单二进制后端 + React 前端 + Chrome 插件 · LGPL-3.0-or-later

> 面向学生的「AI 自主规划 + 温和陪伴」：把 Canvas 成绩、课程表、长期习惯交给它，
> 每天一键生成合理的日程，再通过聊天随口微调。**计划迁就人，不是人迁就计划。**

[中文](#中文) · [English](#english) · [文档地图 / Doc Map](#文档地图--doc-map)

---

## 中文

### 这是什么

Daycore 替不擅长规划的人省心：**Daemon（后台守护）持续产出默认正确的计划，用户只做两件事——丢东西进来、对提案点头或摇头。**

三句话说明它的行为方式：

1. **界面即答案，忽略永远安全**：系统做的每件事要么直接可撤销（账本随手可翻），要么是一张「不理它也不出事」的提案卡。
2. **AI 主动发现**：聊天里说「下周三交离散」「想学 Rust」「累死了」，它自己判断该建作业、投愿望池还是打心情卡——不等你说「请记住」。
3. **信任靠证据爬坡**：第一天的响应式智能是全功率的；主动打扰你的权利（自发卡片、推送）按「证据量」逐步解锁，且撤销会教它变保守。

产品语义的完整定义（时间三层、提案、注意力阶梯、默契评分）见 [docs/EXPERIENCE_CORE.md](docs/EXPERIENCE_CORE.md)。

### 功能一览

| 功能 | 说明 |
|---|---|
| **自主规划** | 汇总课表规则、临期作业（按 due 与分值倒排）、长期记忆，一键生成单日或多日（≤7 天）计划；`keep_manual` 保护手动改过的块 |
| **重复/长期日程** | 每天/每周/每月/每 N 天/远期精确日期；读时展开，删除当日用墓碑（下次展开不复活） |
| **资料导入** | Chrome 插件抓 Canvas（课程/成绩/作业）直推；ICS 课表（零依赖解析器，含时区语义）；课表截图 AI 识别 → 候选确认 |
| **陪伴聊天（SSE 流式）** | 说「以后每 3 天提醒我浇花」就建规则、「把会议改到 3 点」就改计划、「记住我 11 点后不干活」就写长期记忆——**真实工具调用**（15 个工具），不是标签协议。帧协议见 [api/FRONTEND_HANDOFF.md](api/FRONTEND_HANDOFF.md) §B |
| **每用户长期记忆** | 上传 append-only 归档 + AI 可读写的事实库；设置页可删单条/清空 |
| **主题工作室** | 内置玻璃拟态主题 + 自定义主题存库 + AI 生成配色（「像深夜的海」→ 实时预览 → 保存） |
| **心情打卡** | 12 种心情 + AI 回应 + 呼吸/伸展/着地三种练习；心情「窗口」有趋势与时效 |
| **一键撤销** | 所有写操作走 append-only 操作日志，**20 种动作可反向补偿**；撤销是新的一笔，不是橡皮擦 |
| **收件箱/决策卡** | 随手丢的东西先归类再落库；需要拍板的走提案卡（agent 的 `propose_decision`） |
| **许愿池** | 想做但没安排的事进池子，排计划时 agent 自己拿出来提 |
| **多语言** | 用户自选**一主一副**、首页一键切换；后端三层语言目录（DB → `LOCALES_DIR/<locale>.json` → 内嵌），**加一门语言=丢一个 JSON，不用改代码不发版**。⚠️ 现役前端还是硬编码双语字典，这句目前只对后端成立 |
| **多实例** | Lease 选主 + `job_runs` 唯一索引，可水平部署（须共享同一数据库） |
| **监控与运维** | 控制台（模型/服务配置/用户权限/前端/AI 账本/DB 浏览器）、`GET /api/admin/health`、降级启动（存储挂了控制台仍可用） |

### 快速开始（本地）

```bash
# 1) 后端（默认 SQLite，零外部依赖；:8080）—— 仓库根
cp .env.example .env               # 填 DEEPSEEK_API_KEY 才能用 AI
go run ./cmd/daycore

# 2) 前端（开发模式 :5173，/api 自动代理到 :8080）
cd web/frontend && npm install && npm run dev
```

单二进制模式（更接近生产）：`cd web/frontend && npm run build`，再 `go run ./cmd/daycore` —— Go 自己托管 `dist/`，打开 http://localhost:8080。

### 部署到服务器

**分发单位是一个静态二进制**（零 CGO）：提示词、L1 硬边界、模型目录种子都在它肚子里，目标机器不需要源码树、Go 工具链或包管理器。

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o daycore ./cmd/daycore   # 约 23.5 MB
./daycore install -dir /opt/daycore   # 八段问答，写一个「能直接启动」的目录
cd /opt/daycore && ./daycore
```

`install` 写出的东西：`config/models.yaml`（必需）、`config/oauth.yaml`（惰性可选）、提示词模板与 `boundaries.json`（内嵌是地板，磁盘逐文件覆盖）、`.env`（密钥 `0600`，重跑不覆盖已改文件）。改其中一段用 `daycore config <段名>`。CLI 语言跟着 `LOCALES_DIR` 走。

生产在 `.env` 再补（`install` 不问，因为它们绑域名）：

```ini
APP_ENV=production          # 缺 JWT_SECRET/COOKIE_SECRET/ADMIN_TOKEN 直接拒启
HOST=127.0.0.1              # nginx 前置时只听本机
PUBLIC_BASE_URL=https://day.example.com   # OAuth 回调按它拼
SECURE_COOKIES=true
```

nginx 三行不能省（`deploy/nginx.conf` 有完整样例）：`proxy_set_header Host $host`（不写管理面 403）、`proxy_buffering off`（不写 SSE 转圈）、`proxy_read_timeout 300s`（AI 慢）。

前端另部署（四端各自独立构建），或 `STATIC_DIR` 指向 `dist` 让二进制一起托管。`ADMIN_TOKEN` 不设时自动生成并打印在启动日志——「管理面敞开」那一档已删除。

> **多实例已可用**：Lease 选主管节流、`job_runs` 唯一索引管正确性（[ARCHITECTURE.md](docs/ARCHITECTURE.md)「多实例」）。⚠️ 前提是**共享同一个数据库**。

> **插件直推**：设置页生成 Import Token → 插件填服务器地址 + token → **Chrome 弹窗要允许该域名的访问权限**（MV3 无 host permission 的跨域 fetch 被 CORS 拦）。

本地 vs 服务器完整对照、环境变量全集：见 [docs/CONFIG.md](docs/CONFIG.md)（本地测试 vs 生产只在 `APP_ENV`/`HOST`/`SECURE_COOKIES`/`PUBLIC_BASE_URL` 四处不同，[.env.example](.env.example) 是常用子集）。

### 版本

唯一来源 [`internal/version/version.go`](internal/version/version.go)，**只有一个数字**（2026-08-09 起）。`APIVersion`/`APIMinor` 仍由 `GET /api/version` 报出、仍是客户端比对的字段，但**从 `Version` 推导**——线上形状没变。⚠️ 它是**批次标记不是发布号**：`2.2 → 2.3` = 一整份规划从头到尾实现完。契约面变了必须升版，`api/spec/contract-lock.json` + `go test ./...` 强制。

---

## English

### What it is

Daycore is an "AI autonomous planning + gentle companion" app for students: hand it your Canvas grades, class timetable and long-term habits, and a **daemon produces a sane plan every day** — you just drop things in and nod or shake your head at proposals.

### Features (compact)

- **Autonomous planning**: timetable rules + deadlines (ordered by due & points) + long-term memory → one-tap plan, day or range (≤7 days); `keep_manual` protects blocks you edited.
- **Recurring schedule**: daily / weekly / monthly / every-N-days; occurrences expand on read, per-day hides use tombstones.
- **Imports**: Chrome extension scrapes Canvas and pushes directly; ICS timetables (zero-dependency parser with timezone semantics); timetable screenshots via AI extraction with confirmation.
- **Companion chat (streaming SSE)**: real tool calls (15 tools) — "remind me every 3 days" creates a rule, "move the meeting to 3pm" edits the plan, "remember I don't work after 11pm" writes memory. Frame protocol: [api/FRONTEND_HANDOFF.md](api/FRONTEND_HANDOFF.md) §B.
- **Per-user long-term memory**, **theme studio with AI palettes**, **mood check-ins** (12 kinds), **wish pool**, **inbox / decision cards**, **one-tap undo** (append-only ledger, 20 reversible action kinds), **multi-language** (user picks a primary + secondary; backend language packs are drop-in JSON files), **multi-instance** deployments.

The full feature table, the local-vs-production profile table and the deploy walkthrough live in the [Chinese section](#功能一览) — the left columns are the variable names and read fine without Chinese; duplicating long tables here is exactly the drift this README used to have.

### Quick start

```bash
cp .env.example .env          # set DEEPSEEK_API_KEY for AI features
go run ./cmd/daycore          # backend :8080, SQLite by default
cd web/frontend && npm install && npm run dev   # frontend :5173
```

### Deploy

One static binary (no cgo) ships everything: `CGO_ENABLED=0 go build -ldflags="-s -w" -o daycore ./cmd/daycore`, then `./daycore install -dir /opt/daycore` walks through eight questions and writes a directory that starts. Add the domain-bound settings in production:

```ini
APP_ENV=production
HOST=127.0.0.1              # behind nginx
PUBLIC_BASE_URL=https://day.example.com
SECURE_COOKIES=true
```

nginx needs `proxy_set_header Host $host`, `proxy_buffering off` (SSE) and `proxy_read_timeout 300s` — see [deploy/nginx.conf](deploy/nginx.conf). The frontend deploys separately (or `STATIC_DIR` points at its `dist/`). **Every env var** and the local-vs-production differences: [docs/CONFIG.md](docs/CONFIG.md).

### Docs

See the [文档地图 / Doc Map](#文档地图--doc-map) below — the map itself is bilingual-ish and the linked documents are mostly Chinese (project language), with the specs folder holding the English-facing protocols.

---

## 文档地图 / Doc Map

一套文档、各司其职，同一主题只在一处详述、别处引用。

| 文档 | 回答什么 | 读者 |
|---|---|---|
| [README.md](README.md)（本文件） | 是什么、怎么跑起来、怎么部署 | 所有人 |
| [AGENTS.md](AGENTS.md) | 仓库「指路 + 铁律」：布局、构建验证、代码约定、测试策略、部署与安全要点 | **AI 助手 / 贡献者**（先读它） |
| [docs/ROADMAP.md](docs/ROADMAP.md) | **总规划**：做什么、按什么顺序、为什么（决策账本） | 想理解方向的人 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 架构总览：包结构、装配与关停、多实例、时区、分发与降级启动 | 改架构的人 |
| [docs/DATA.md](docs/DATA.md) | 数据层：实体、四个存储后端、迁移、行为套件钉死的语义 | 改存储/domain 的人 |
| [docs/AUTH.md](docs/AUTH.md) | 认证三轨、CORS、管理面权限与配对 | 改认证/中间件的人 |
| [docs/AGENT.md](docs/AGENT.md) | agent loop、SSE 帧协议、工具带、决策卡、异步端点 | 改 agent/聊天的人 |
| [docs/AI.md](docs/AI.md) | provider、提示词三层、L1 边界、vision、wire-format | 改 AI 的人 |
| [docs/CONFIG.md](docs/CONFIG.md) | **每个配置项能不能热改**（启动期/运行时/密钥，含生成总表） | 改配置/写控制台的人 |
| [docs/EXPERIENCE_CORE.md](docs/EXPERIENCE_CORE.md) | 端无关的产品语义：时间三层、提案、注意力阶梯、默契 | 所有实现者 |
| [docs/STRATEGY.md](docs/STRATEGY.md) | 战略认知、商业化、陪伴边界、全功能审计（讨论区） | 做产品决策的人 |
| [docs/DEVELOPING.md](docs/DEVELOPING.md) | 开发命令、扩展点、加路由/工具/模型/语言的分步骨架、切仓流程 | 日常开发者 |
| [docs/API_SURFACE.md](docs/API_SURFACE.md) | 路由 → handler 映射总表（**生成物**，`make api-surface`） | 查路由的人 |
| [docs/specs/](docs/specs/README.md) | **对外协议**：传输 / 存储 / provider / 前端 manifest | 第三方适配器与前端作者 |
| [api/openapi.yaml](api/openapi.yaml) | API 契约（**生成物**，源在 `api/spec/`） | 写客户端的人 |

原则：**实时文档铁律** —— 任何代码改动必须同批更新对应文档；设计约束（边界/取舍/防的是什么）必须写进文档而不是只写进注释。

---

## 许可证 / License

**GNU Lesser General Public License v3.0 or later**（LGPL-3.0-or-later）。完整条款见 [`COPYING.LESSER`](COPYING.LESSER) 与 [`COPYING`](COPYING)。

> 通过 HTTP 调用本服务的 API 不构成衍生作品 —— 独立前端、移动端或第三方客户端可自行选择许可证，包括闭源。copyleft 义务作用于**分发本项目代码或其修改版**的情形（如改造后端后再分发二进制或源码，需一并提供对应源码）。

> API calls over HTTP do not create a derivative work — a separate frontend, mobile app, or third-party client may be licensed however you like, including closed source. The copyleft obligations attach to **distributing this project's code or a modified version of it**.
