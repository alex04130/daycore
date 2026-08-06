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
- **陪伴聊天（SSE 流式）**：说"以后每 3 天提醒我浇花"就建规则、"把会议改到 3 点"就改计划、"记住我 11 点后不干活"就写入长期记忆（**工具调用**：15 个工具，`agent_tools.go`；早先的 `<plan_update>` 标签协议已废弃，对接细节以 [`api/FRONTEND_HANDOFF.md`](api/FRONTEND_HANDOFF.md) §B 的 SSE v2 帧协议为准）。
- **每用户长期记忆**：所有上传 append-only 归档 + AI 可读写的事实库；设置页可删单条/清空（隐私）。
- **主题工作室**：4 套内置玻璃拟态主题 + 自定义主题存库 + AI 生成配色（"像深夜的海"→ 实时预览 → 保存）。
- **心情打卡**：12 种心情 + AI 回应 + 呼吸/伸展/着地三种练习。
- **一键撤销**：所有写操作走 append-only 操作日志（`GET /api/ops`），逐条可反向补偿（`POST /api/ops/{id}/revert`，当前 16 种动作）—— 撤销是新的一笔，不是橡皮擦。
- **收件箱 / 决策卡**：随手丢进来的东西先归类再落库（`/api/inbox/*`）；需要你拍板的走提案卡（agent 的 `propose_decision` 工具），不理它也不出事。
- **许愿池**：想做但还没安排的事进池子，agent 全局可读可写，排计划时自己拿出来提。
- **多语言**：用户自选**一主一副**两种语言、首页一键切换，AI 回复语言跟随。提示词按 locale 组织（11 key × 2 locale，运行时可在控制台覆盖）。后端的界面文案是**三层目录**（DB 覆盖 → `LOCALES_DIR/<locale>.json` → 内嵌 zh-CN/en-US），**给后端加一门语言是丢一个翻译文件，不用改代码也不用发版**。
  ⚠️ 前端还不是 —— `web/frontend/src/i18n.js` 目前是硬编码的两语言字典，所以「丢个 json 就多一门语言」这句现在只对后端成立。

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
# 注释一律写在行上方 —— 行尾 `\` 后面跟 `#` 会让续行断掉（见下方警告）
APP_ENV=production \
HOST=127.0.0.1 PORT=8080 \
STATIC_DIR=/opt/daycore/web/frontend/dist \
PUBLIC_BASE_URL=https://day.example.com \
JWT_SECRET=$(openssl rand -hex 32) \
COOKIE_SECRET=$(openssl rand -hex 32) \
ADMIN_TOKEN=$(openssl rand -hex 32) \
SECURE_COOKIES=true \
DEEPSEEK_API_KEY=sk-… \
./bin/daycore
```

`HOST=127.0.0.1` 只监听本机、由 nginx 对外；`PUBLIC_BASE_URL` 给 OAuth 回调用；**`ADMIN_TOKEN` 不是可选的** —— 不设它时管理面的鉴权退化成「只看 `APP_ENV` 是不是 production」（`handlers_admin.go:24`），而管理面包含 `GET`/`DELETE /api/admin/db/table/{name}` 这样的裸库读删。

> ⚠️ **2026-07-29 修正的一个真 bug**：这一段先前把注释写在行尾 `\` 之后。反斜杠转义的是它后面那个空格而不是换行，**续行当场断裂** —— 复制粘贴下来，`APP_ENV`、`HOST`、`PORT`、`STATIC_DIR`、`PUBLIC_BASE_URL` 五项全部丢失，shell 还会报两行 `: command not found`（很容易被当成无害噪音）。丢掉 `APP_ENV=production` 最贵：`IsProduction()` 变 false，于是**没设 `ADMIN_TOKEN` 时整个管理面对外敞开**。照那段部署过的实例请立刻确认这两项。

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

> ⚠️ **只跑一个实例。** 后台的主动任务（早报、晚复盘、deadline 巡检）现在由每个进程各自排程，没有选主。起第二份副本 = 用户收到两遍推送。换成 PostgreSQL/MySQL/MongoDB 是为了备份与运维，不代表可以多副本 —— 选主在 `docs/ROADMAP.md` 的 ζ 批次。

浏览器插件直推：打开插件设置，把「Daycore 服务器地址」改成 `https://day.example.com`，粘贴应用设置页生成的 Import Token，点「保存」——**Chrome 会弹窗请求该域名的访问权限，必须允许**（MV3 下没有 host permission 的跨域 fetch 会被 CORS 拦掉）。点「测试连接」可立即验证地址与权限是否就绪。

### 本地测试版 vs 服务器部署版

| | 本地测试 | 服务器部署 |
|---|---|---|
| `APP_ENV` | `development`（缺省密钥自动用 dev 值） | `production`（**必须**设置 `JWT_SECRET`/`COOKIE_SECRET`） |
| 前端 | Vite dev server :5173 + `/api` 代理（热更新） | `npm run build` → Go `STATIC_DIR` 托管（或 nginx 直接发 dist） |
| `HOST` | 显式 `HOST=`（空值）= 全网卡；**完全不设**且非 production 会被强制改写成 `127.0.0.1` | `127.0.0.1`（nginx 前置时）；直接暴露则显式留空 |
| `SECURE_COOKIES` | `false` | `true`（HTTPS 后必须） |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | `https://你的域名`（OAuth 回调依赖它） |
| 数据库 | SQLite 单文件（缺省） | SQLite 即可；备份/运维需求换 `postgres`/`mysql`/`mongodb`（`DB_TYPE`+`DB_DSN`） |
| CORS | Vite 代理下无需配置 | 同源部署无需配置；仅当前端另起域名才设 `ALLOWED_ORIGINS` |
| 插件直推 | `http://localhost:8080` | `https://你的域名`（`/api/import/*` 已对任意 Origin 放行，token 鉴权） |
| AI 限流 | 缺省 30 req/min/IP | 按需调 `AI_RATE_LIMIT_PER_MIN` |

### 配置参考

环境变量的**完整**清单在 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md#配置internalconfigconfiggo环境变量)（[`.env.example`](.env.example) 是常用子集，缺 9 项，其中 `COOKIE_SAMESITE` 取值不合法会直接启动失败）。要点：`DB_TYPE`/`DB_DSN`（sqlite/postgres/mysql/mongodb 一键切换）、`MODELS_CONFIG`（`config/models.yaml`，加模型零代码）、`AUTO_PLAN_MAX_DAYS`、`ASSIGNMENT_LOOKAHEAD_DAYS`、`ADMIN_TOKEN`（提示词在线编辑，`PUT /api/admin/prompts/{key}?locale=`）。

### 版本

版本号唯一来源：[`internal/version/version.go`](internal/version/version.go)。**同一个文件里有三层，不要混**：

| 层 | 常量 | 谁在用 |
|---|---|---|
| 构建版本 | `Version` + `Channel`（`2.<minor>.<patch>` + `beta`；minor=功能里程碑，patch=修复） | `GET /api/healthz`、设置页显示「Daycore v2.2.0-beta」 |
| **API 契约版本** | `APIVersion` / `APIMinor` | `GET /api/version`；**各前端握手用的是这个** —— breaking 升 major，additive 升 minor |
| 各前端自己的版本 | 不在本仓 | 独立迭代，与上面两层解耦 |

契约面变了就必须升版，这条由 `api/spec/contract-lock.json` + `go test ./...` 强制（详见 [`api/spec/README.md`](api/spec/README.md)）。

---

## English

Daycore is an "AI autonomous planning + gentle companion" app for students: hand it your Canvas assignments & grades, class timetable and long-term habits, and it plans every day in one tap — then you fine-tune by just chatting. Go single-binary backend + React frontend + Chrome extension.

### Features

- **Autonomous planning**: aggregates timetable rules, upcoming deadlines (backward-planned by due date & points) and long-term memory into a one-tap plan for a day or a range (≤7 days); `keep_manual` mode protects blocks you edited.
- **Recurring / long-term schedule**: daily / weekly on weekdays / monthly / every-N-days / precise far-future dates; occurrences expand on read, per-day hide uses tombstones.
- **Material imports**: Chrome extension scrapes Canvas (courses/grades/due dates) to JSON export or direct push; ICS timetables (zero-dependency parser); weekly-timetable screenshots via AI extraction with candidate confirmation.
- **Companion chat (streaming SSE)**: "remind me to water the plants every 3 days" creates a rule, "move the meeting to 3pm" edits the plan, "remember I don't work after 11pm" writes long-term memory (**tool calls** — 15 tools in `agent_tools.go`; the old `<plan_update>` tag protocol is gone. Frame protocol: [`api/FRONTEND_HANDOFF.md`](api/FRONTEND_HANDOFF.md) §B).
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
ADMIN_TOKEN=$(openssl rand -hex 32) \
SECURE_COOKIES=true DEEPSEEK_API_KEY=sk-… \
./bin/daycore
```

`ADMIN_TOKEN` is **not optional**: without it, admin auth degrades to "is `APP_ENV` production?" (`handlers_admin.go:24`), and the admin surface includes raw `GET`/`DELETE /api/admin/db/table/{name}`. Keep comments on their own lines — a `#` after a trailing `\` silently breaks the continuation.

Front it with nginx (swap `server_name`, add TLS; `proxy_buffering off` is required for SSE) as shown in the Chinese section above, or skip nginx entirely and expose the Go server directly. Docker: `make docker && docker compose -f deploy/docker-compose.yml up app`.

⚠️ **Run exactly one instance.** Proactive background jobs (morning brief, evening review, deadline sweep) are scheduled per-process with no leader election, so a second replica means every user gets everything twice. Switching to PostgreSQL/MySQL/MongoDB buys you backups and operational tooling, not replicas — leader election is batch ζ in `docs/ROADMAP.md`.

For the browser extension: open its Options, set the Daycore server URL to `https://day.example.com`, paste the Import Token from the app's Settings, and hit Save — **Chrome will prompt for access to that origin and you must allow it** (under MV3 a cross-origin fetch without a host permission is blocked by CORS). "Test connection" verifies the URL and the grant right away.

### Configuration & versioning

Reference material is not duplicated here, because a parallel copy drifts — and this one already had: the Chinese deploy block was broken while this one was fine, and the two profile tables had diverged to nine rows versus seven.

- **Every env var**, with which ones are boot-time versus hot-reloadable → [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md#配置internalconfigconfiggo环境变量). [`.env.example`](.env.example) is a commonly-used subset, not the full list.
- **Local vs server profiles** → the Chinese table above (`APP_ENV`, `HOST`, `SECURE_COOKIES`, `PUBLIC_BASE_URL`, `DB_TYPE`, `ALLOWED_ORIGINS`, rate limits). It reads fine without Chinese: the left column is the variable, the middle is local, the right is production.
- **Versioning** → single source [`internal/version/version.go`](internal/version/version.go), **three layers in one file**: the build version (`Version`+`Channel`, `2.<minor>.<patch>`+`beta`, shown by `GET /api/healthz`), the **API contract version** (`APIVersion`/`APIMinor` — what frontends handshake on, served by `GET /api/version`), and each frontend's own version (out of tree). A contract-surface change must bump one of the first two; `api/spec/contract-lock.json` plus `go test ./...` enforce that.

---

## 开发者手册

搬到 [`docs/DEVELOPING.md`](docs/DEVELOPING.md)：项目结构、扩展点（registry / 驱动模式）、开发命令、端到端冒烟。

其余文档：[`docs/`](docs/) 是实时项目文档（这个仓库现在是什么样），[`docs/specs/`](docs/specs/) 是对外协议（别人要照着实现什么 —— 适配器与前端）。

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
