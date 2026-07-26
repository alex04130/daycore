# Daycore v2 — AI 自主规划日程助手 / AI Autonomous Day Planner

**v2.2.0-beta（v2 测试版 / v2 beta）**

[中文](#中文) · [English](#english)

---

## 中文

Daycore 是一个面向学生的「AI 自主规划 + 温和陪伴」应用：把 Canvas 作业成绩、课程表、长期习惯交给它，每天一键生成合理的日程，再通过聊天随口微调。Go 单二进制后端 + React 前端 + Chrome 插件。

### 功能一览

- **自主规划**：汇总课表规则、临期作业（按 due 与分值倒排）、长期记忆，一键生成单日或多日（≤7 天）计划；`keep_manual` 模式保护你手动改过的块。
- **重复 / 长期日程**：每天 / 每周某几天 / 每月 / 每 N 天 / 远期精确日期；读计划自动展开、删除当日不复活（墓碑）。
- **资料导入**：Chrome 插件抓 Canvas（课程/成绩/作业 due）导出 JSON 或直推；ICS 课表（零依赖解析器）；周课表截图 AI 识别 → 候选确认。
- **陪伴聊天（SSE 流式）**：说"以后每 3 天提醒我浇花"就建规则、"把会议改到 3 点"就改计划、"记住我 11 点后不干活"就写入长期记忆（`<plan_update>`/`<rule_update>`/`<memory_update>` 协议）。
- **每用户长期记忆**：所有上传 append-only 归档 + AI 可读写的事实库；设置页可删单条/清空（隐私）。
- **主题工作室**：4 套内置玻璃拟态主题 + 自定义主题存库 + AI 生成配色（"像深夜的海"→ 实时预览 → 保存）。
- **心情打卡**：12 种心情 + AI 回应 + 呼吸/伸展/着地三种练习。
- **双语**：zh-CN / en-US 全站切换，AI 回复语言跟随（提示词按 locale 组织，6 key × 2 locale，运行时可覆盖）。

### 快速开始（本地测试版）

```bash
# 1) 后端（默认 SQLite，零外部依赖；:8080）—— 在仓库根执行
cp .env.example .env               # 填 DEEPSEEK_API_KEY 才能用 AI 功能
go run ./cmd/daycore

# 2) 前端（开发模式，:5173，/api 自动代理到 :8080）
cd web/frontend
npm install
npm run dev                        # 打开 http://localhost:5173
```

或者本地跑「单二进制模式」（更接近生产）：

```bash
cd web/frontend && npm run build      # 产出 dist/
cd ../.. && go run ./cmd/daycore      # STATIC_DIR 默认 web/frontend/dist，直接打开 http://localhost:8080
```

### 部署到服务器（域名）

```bash
# 构建
cd web/frontend && npm install && npm run build
cd ../.. && make build                # CGO_ENABLED=0 → bin/daycore（静态单文件）

# 上传 bin/daycore + config/ + web/frontend/dist 到服务器，然后：
APP_ENV=production \
HOST=127.0.0.1 PORT=8080 \                       # 只监听本机，由 nginx 对外
STATIC_DIR=/opt/daycore/web/frontend/dist \
PUBLIC_BASE_URL=https://day.example.com \        # OAuth 回调用
JWT_SECRET=$(openssl rand -hex 32) \
COOKIE_SECRET=$(openssl rand -hex 32) \
SECURE_COOKIES=true \
DEEPSEEK_API_KEY=sk-… \
./bin/daycore
```

nginx（`deploy/nginx.conf` 为容器版样例；裸机把 `server_name` 换成你的域名并加 TLS）：

```nginx
server {
    listen 443 ssl http2;
    server_name day.example.com;                  # ← 你的域名
    # ssl_certificate … (certbot)
    location / {
        proxy_pass http://127.0.0.1:8080;         # Go 同时托管前端与 /api
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_buffering off;                      # SSE 必须关闭缓冲
        proxy_read_timeout 300s;                  # AI 请求较慢
    }
}
```

> 也可以完全不用 nginx：`HOST=`（留空监听所有网卡）+ `PORT=443` 前面挂 Caddy，或直接 `PORT=8080` 裸跑测试。Docker：`make docker && docker compose -f deploy/docker-compose.yml up app`。

浏览器插件直推：打开插件设置，把「Daycore 服务器地址」改成 `https://day.example.com`，粘贴应用设置页生成的 Import Token，点「保存」——**Chrome 会弹窗请求该域名的访问权限，必须允许**（MV3 下没有 host permission 的跨域 fetch 会被 CORS 拦掉）。点「测试连接」可立即验证地址与权限是否就绪。

### 本地测试版 vs 服务器部署版

| | 本地测试 | 服务器部署 |
|---|---|---|
| `APP_ENV` | `development`（缺省密钥自动用 dev 值） | `production`（**必须**设置 `JWT_SECRET`/`COOKIE_SECRET`） |
| 前端 | Vite dev server :5173 + `/api` 代理（热更新） | `npm run build` → Go `STATIC_DIR` 托管（或 nginx 直接发 dist） |
| `HOST` | 留空（全网卡）或缺省 | `127.0.0.1`（nginx 前置时）；直接暴露则留空 |
| `SECURE_COOKIES` | `false` | `true`（HTTPS 后必须） |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | `https://你的域名`（OAuth 回调依赖它） |
| 数据库 | SQLite 单文件（缺省） | SQLite 即可；多实例/备份需求换 `postgres`/`mysql`/`mongodb`（`DB_TYPE`+`DB_DSN`） |
| CORS | Vite 代理下无需配置 | 同源部署无需配置；仅当前端另起域名才设 `ALLOWED_ORIGINS` |
| 插件直推 | `http://localhost:8080` | `https://你的域名`（`/api/import/*` 已对任意 Origin 放行，token 鉴权） |
| AI 限流 | 缺省 30 req/min/IP | 按需调 `AI_RATE_LIMIT_PER_MIN` |

### 配置参考

全部环境变量见 [`.env.example`](.env.example)（含注释）。要点：`DB_TYPE`/`DB_DSN`（sqlite/postgres/mysql/mongodb 一键切换）、`MODELS_CONFIG`（`config/models.yaml`，加模型零代码）、`AUTO_PLAN_MAX_DAYS`、`ASSIGNMENT_LOOKAHEAD_DAYS`、`ADMIN_TOKEN`（提示词在线编辑，`PUT /api/admin/prompts/{key}?locale=`）。

### 版本

版本号唯一来源：[`internal/version/version.go`](internal/version/version.go)（`2.<minor>.<patch>` + `beta` 渠道；minor=功能里程碑，patch=修复）。`GET /api/healthz` 返回 `version`/`channel`，设置页展示「Daycore v2.2.0-beta · v2 测试版」。

---

## English

Daycore is an "AI autonomous planning + gentle companion" app for students: hand it your Canvas assignments & grades, class timetable and long-term habits, and it plans every day in one tap — then you fine-tune by just chatting. Go single-binary backend + React frontend + Chrome extension.

### Features

- **Autonomous planning**: aggregates timetable rules, upcoming deadlines (backward-planned by due date & points) and long-term memory into a one-tap plan for a day or a range (≤7 days); `keep_manual` mode protects blocks you edited.
- **Recurring / long-term schedule**: daily / weekly on weekdays / monthly / every-N-days / precise far-future dates; occurrences expand on read, per-day hide uses tombstones.
- **Material imports**: Chrome extension scrapes Canvas (courses/grades/due dates) to JSON export or direct push; ICS timetables (zero-dependency parser); weekly-timetable screenshots via AI extraction with candidate confirmation.
- **Companion chat (streaming SSE)**: "remind me to water the plants every 3 days" creates a rule, "move the meeting to 3pm" edits the plan, "remember I don't work after 11pm" writes long-term memory (`<plan_update>`/`<rule_update>`/`<memory_update>` protocol).
- **Per-user long-term memory**: every upload archived append-only + an AI-writable fact store; delete one / clear all from Settings (privacy).
- **Theme studio**: 4 built-in glassmorphism themes + custom themes stored server-side + AI-generated palettes ("like the sea at midnight" → live preview → save).
- **Mood check-ins**: 12 moods + AI responses + breathing/stretch/grounding exercises.
- **Bilingual**: zh-CN / en-US across the app; AI replies follow the session language (prompts organized per locale, runtime-editable).

### Quick start (local)

```bash
# 1) Backend (SQLite by default, zero external services; :8080) — run from repo root
cp .env.example .env               # set DEEPSEEK_API_KEY to enable AI features
go run ./cmd/daycore

# 2) Frontend (dev mode, :5173, /api proxied to :8080)
cd web/frontend
npm install && npm run dev         # open http://localhost:5173
```

Single-binary mode (closer to production): `cd web/frontend && npm run build`, then `go run ./cmd/daycore` — the Go server serves `web/frontend/dist` itself at http://localhost:8080.

### Deploy to a server (custom domain)

```bash
cd web/frontend && npm install && npm run build
cd ../.. && make build             # CGO_ENABLED=0 → bin/daycore (static binary)

# Ship bin/daycore + config/ + web/frontend/dist, then run:
APP_ENV=production HOST=127.0.0.1 PORT=8080 \
STATIC_DIR=/opt/daycore/web/frontend/dist \
PUBLIC_BASE_URL=https://day.example.com \
JWT_SECRET=$(openssl rand -hex 32) COOKIE_SECRET=$(openssl rand -hex 32) \
SECURE_COOKIES=true DEEPSEEK_API_KEY=sk-… \
./bin/daycore
```

Front it with nginx (swap `server_name`, add TLS; `proxy_buffering off` is required for SSE) as shown in the Chinese section above, or skip nginx entirely and expose the Go server directly. Docker: `make docker && docker compose -f deploy/docker-compose.yml up app`.

For the browser extension: open its Options, set the Daycore server URL to `https://day.example.com`, paste the Import Token from the app's Settings, and hit Save — **Chrome will prompt for access to that origin and you must allow it** (under MV3 a cross-origin fetch without a host permission is blocked by CORS). "Test connection" verifies the URL and the grant right away.

### Local vs server profiles

| | Local testing | Server deployment |
|---|---|---|
| `APP_ENV` | `development` (dev-fallback secrets) | `production` (**must** set `JWT_SECRET`/`COOKIE_SECRET`) |
| Frontend | Vite dev server :5173 with `/api` proxy (HMR) | `npm run build` → served by Go via `STATIC_DIR` |
| `HOST` | empty (all interfaces) | `127.0.0.1` behind nginx; empty if exposed directly |
| `SECURE_COOKIES` | `false` | `true` (behind HTTPS) |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | `https://your.domain` (OAuth callbacks) |
| Database | SQLite file (default) | SQLite is fine; switch `DB_TYPE` to postgres/mysql/mongodb for multi-instance |
| Extension push | `http://localhost:8080` | `https://your.domain` (`/api/import/*` is CORS-open, token-authenticated) |

### Configuration & versioning

All env vars are documented in [`.env.example`](.env.example). Version single source: [`internal/version/version.go`](internal/version/version.go) (`2.<minor>.<patch>` + `beta` channel; minor = feature milestone, patch = fixes). `GET /api/healthz` returns `version`/`channel`; Settings shows "Daycore v2.2.0-beta · v2 beta".

---

## 开发者手册（中文）

### 项目结构

```
cmd/daycore/main.go         装配：注册驱动/格式 → 打开 Store → 迁移 → 起 HTTP（优雅关闭）
internal/
  domain/                   实体 + 通用 Store/Repository 接口（界面分离核心）
  config/                   环境配置（HOST/PORT/STATIC_DIR/…）
  version/                  版本号唯一来源
  auth/                     argon2id / JWT / OAuth / 会话 cookie
  i18n/                     locale 归一化 / Accept-Language 协商
  ai/                       AIProvider 接口 + catalog + 视觉编排 + 提示词
    formats/{openai,anthropic,ollama}/   wire-format（自注册）
    prompts/<locale>/*.tmpl 提示词（7 key × 2 locale）
  schedule/                 重复规则展开/合并引擎（纯函数）
  ics/                      最小 iCalendar + RRULE 子集解析器（零依赖）
  storage/{sqlstore,mongostore}/  SQLite+PG+MySQL（Dialect 抽象）/ MongoDB
  server/                   路由 + 中间件 + handlers（SSE / auto-plan / 导入 / 记忆 / 主题 / 静态托管）
api/                        openapi.yaml + FRONTEND_HANDOFF.md（产品与协议规范）
web/frontend/               React 前端（Vite；npm run dev / build）
deploy/                     Dockerfile / docker-compose / nginx
testdata/                   canvas-export.sample.json / sample.ics
docs/                       实时项目文档（架构/认证/Agent/数据/AI/路由总表）
extension/                  Chrome MV3 插件（抓 Canvas → POST /api/import/canvas）
design-ui/                  前后端分离式前端的落地点（空占位，后端未跟进）
```

### 扩展点（registry / 驱动模式）

1. **新增数据库**：实现 `domain.Store` + `storage.Register("foo", opener)`（SQL 类只需加一个 `Dialect`），main.go 加 blank import。
2. **新增 AI 厂商格式**：实现 `ai.AIProvider` + `ai.RegisterFormat("gemini", New)`，`models.yaml` 引用 `format: gemini`。
3. **新增模型（零代码）**：编辑 `config/models.yaml`，重启生效；视觉模型标 `vision: true`。
4. **新增 OAuth provider（零代码）**：`config/oauth.yaml` 加条目，回调 `<PUBLIC_BASE_URL>/api/auth/oauth/<name>/callback`。
5. **编辑提示词（运行时）**：`PUT /api/admin/prompts/{key}?locale=zh-CN|en-US`（`X-Admin-Token`），覆盖存 `prompt_overrides`。

### 开发命令

```bash
make run / test / vet / build / docker
cd web/frontend && npm run dev / build
node web/frontend/scripts/check-i18n.mjs            # zh-CN / en-US key 对齐校验
```

### 端到端冒烟（curl）

见 `testdata/` 夹具；核心流：`POST /api/session/init` → `POST /api/import/token` → `POST /api/import/canvas`（X-Import-Token）→ `POST /api/import/ics` → `POST /api/rules` → `GET /api/plan?date=`（规则虚拟合并）→ `POST /api/ai/auto-plan` → SSE `/api/ai/companion`（`<rule_update>`/`<memory_update>`）→ `GET /api/memory`。

### 相对 v1 的改进

- 主模式从「手动输入日程」变为「自主规划」（资料导入 → auto-plan → 聊天微调）。
- 新增重复/长期日程、Canvas/ICS/截图导入、每用户长期记忆、自定义主题 + AI 配色、i18n（含提示词双语）、版本体系与单二进制静态托管部署。
- 修复无鉴权数据端点（服务端签名 httpOnly cookie）、`incrementInteractionCount` 占位符 bug、日程加载 N+1、v1 MoodScreen `exerciseOffered` 异步 bug（前端已按正确方式实现）。

---

## 许可证 / License

**GNU Lesser General Public License v3.0 or later**（LGPL-3.0-or-later）。
完整条款见 [`COPYING.LESSER`](COPYING.LESSER)（LGPL 附加条款）与
[`COPYING`](COPYING)（其所依据的 GPL-3.0 正文）。

Licensed under the **GNU LGPL v3.0 or later**. See
[`COPYING.LESSER`](COPYING.LESSER) plus [`COPYING`](COPYING) for the full terms.

> 通过 HTTP 调用本服务的 API 不构成衍生作品 —— 独立前端、移动端或第三方客户端
> 可以自行选择许可证，包括闭源。copyleft 义务作用于**分发本项目代码或其修改版**
> 的情形（例如自行改造后端后再分发二进制或源码，需一并提供对应源码）。
>
> API calls over HTTP do not create a derivative work — a separate frontend,
> mobile app, or third-party client may be licensed however you like, including
> closed source. The copyleft obligations attach to **distributing this project's
> code or a modified version of it**.
