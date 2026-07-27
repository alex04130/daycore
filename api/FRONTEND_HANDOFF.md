# Daycore v2 — 前端产品与对接规范（已合并版）

> **状态（2026-07-08，路径更新 2026-07-25）**：设计交付已完成，前端已按原型 **1:1 实现在 `web/frontend/`**（Vite + React 18）。设计系统 bundle 已 vendor 进 `web/frontend/src/ds/` 与 `src/vendor/ds-bundle.js`，**这两处即仓库内的权威副本**；设计交付原件不在仓库内（在 Claude Design 网页版）。本文件由原 handoff、增量 01（自定义主题）与 §7（长期记忆）合并而成，作为**产品意图 + 前端对接协议的唯一权威文档**；REST 契约以 `api/openapi.yaml` 为准。

**产品主线**：自主规划（Autonomous Planning）——系统汇总用户的所有资料（Canvas 作业与成绩、课程表、重复规则、长期记忆），一键生成当日/多日计划，用户再通过聊天微调。

---

## ⚠️ v2 协议更新（2026-07-13，取代 §2 / §3.3 的 v1 描述）

> 后端已从 v1 的「XML 标签 + `[DONE]` SSE」迁移到 **v2 tool-calls 帧协议**。凡本节与下文 §2「聊天标签协议」、§3.3 冲突处，**以本节为准**。审计确认旧的 `data:{"delta"}`＋`[DONE]`＋`<plan_update>`/`<rule_update>`/`<memory_update>` 标签方案已废弃。

### A. 交互三层（决定 UI 结构）

| 层 | 名称 | UI 表现 |
|----|------|---------|
| **L1** 静默执行 | AI 产出直接生效 | `origin=auto` 块用**虚线边框**；永不覆盖 `origin=manual`。 |
| **L2** 对话建议 | companion SSE 聊天 | AI 不确定时自然语言问，用户回一句话。 |
| **L3** 决策卡 | SSE `decision_card` 帧 | 一次一张非阻塞卡（冲突/空档/规则确认/资料关联/重排），可忽略；选后 POST 回。 |

### B. SSE v2 帧协议（唯一 SSE 端点：`POST /api/ai/companion`）

请求体 `{ message, timezone, assistantName, threadId?, conversationHistory? }`。有 `threadId` 时后端加载**并持久化**该线程历史（多轮上下文跨请求累积）；无 `threadId` 时用 `conversationHistory`（role 仅 user/assistant 被接受，`system` 被强制降级为 user）。

响应 `text/event-stream`，每帧 `data: {json}\n\n`，按 `type` 区分（**不再有 `[DONE]`**）：

| type | 结构 | 前端处理 |
|------|------|---------|
| `delta` | `{"type":"delta","text":"..."}` | 追加到助手气泡 |
| `reasoning` | `{"type":"reasoning","text":"..."}` | 可选思考过程（灰/折叠） |
| `tool_start` | `{"type":"tool_start","callId","tool","args":{}}` | 显示「正在…」动作卡 |
| `tool_result` | `{"type":"tool_result","callId","tool","ok","summary?","data?","opId?","error?"}` | 更新动作卡；存 `opId` 供撤销 |
| `decision_card` | `{"type":"decision_card","id","title","summary","options":[{"id","label"}]}` | 弹卡；选后 POST `/api/decisions/{id}/respond` `{choice,text?}` |
| `error` | `{"type":"error","code","message"}` | 显示错误（后随 `done`） |
| `done` | `{"type":"done"}` | 流结束（终止帧） |
| 心跳 | `: ping\n\n` | 忽略 |

**Agent 不再用 XML 标签**：所有计划/规则/记忆变更由后端 agent 通过 11 个工具（get_weather/web_search/list_upcoming/plan_add/plan_update/plan_remove/rule_upsert/rule_remove/memory_add/memory_remove/propose_decision，max 6 轮）执行，前端只需渲染 `tool_start`/`tool_result` 动作卡，**不再解析 `<plan_update>` 等标签、不再自己发 PATCH**。撤销用 `tool_result.opId` → `POST /api/ops/{id}/revert`。

**决策卡时序**：`decision_card` 发出后 agent 阻塞 ≤45s（期间 `: ping` 保活）→ 前端弹卡 → 用户选 → `POST /api/decisions/{id}/respond` → 后端解阻塞、流继续 → `done`。不选也可（超时/新消息取消）。一 session 同时仅一张待响应卡。

### C. 时区显示（TimeBlock.timeMode）

`floating`（默认，跟墙钟，无 `utcTime`）/ `fixed`（绝对时刻，有 `utcTime` RFC3339 锚点）/ `local`（生理节律，跟当前时区，有 `utcTime`）。fixed/local 按查看时区从 `utcTime` 反算；floating 直接用 `time`。后端 DST 已处理。

### D. 主动功能偏好（`GET/PATCH /api/session/preferences`）

`{ morningBrief, eveningReview, deadlineAlerts, rollingReplan, gapSuggestions, doNotDisturb, autoPlan }`（默认全开，doNotDisturb 关）。控制后台 worker 推送（需绑定通道才实际推送）。设置页提供开关面板。

### E. 通道绑定（`/api/channels`）

`GET /api/channels`（可用通道+绑定态）→ `POST /api/channels/{channel}/bind`（拿 12 位 token，10min 过期）→ 用户在平台发 token 给 bot → bot 侧 `POST .../verify {token, externalId}` → 绑定。`DELETE .../unbind` 解绑。

### F. PWA

后端托管静态资源（`STATIC_DIR`）。前端落实 `manifest.json`（standalone/图标/主题色）+ service worker（离线壳 + 缓存今日 `GET /api/plan?date=today`）+ 安装提示。

### G. 前后端分离 / 原生端：版本契约 + header 认证（2026-07-14 新增）

**版本契约 `GET /api/version`（公开）**：`{apiVersion, apiMinor, build, channel, minClient, locales}`。

- `apiVersion` = API 契约大版本，只在破坏性变更时 +1，**独立于构建版本**。客户端硬编码自己期望的值，不等 → 硬阻断并提示升级（web 前端把它存 `state.apiMismatch`）。
- `apiMinor` = 新增性变更（加端点/字段）+1；客户端可据此对可选功能降级。
- `build`/`channel` = 构建版本展示用（"2.2.0-beta"）；`minClient` = 服务器认为兼容的最老客户端构建（提示更新用，不阻断）。
- `locales` = `{available, defaultPrimary, defaultSecondary}`（2026-07-26 加）。`available` 是**这个安装能渲染的全部语言**，**不是编译期列表** —— 只有 zh-CN / en-US 编进二进制，丢一个 `<locale>.json` 进 `LOCALES_DIR` 或从控制台加一份就会多出来，所以前端必须读它而不是写死自己以为存在的那几种。后两个是**新用户的默认**一主一副。
- 老后端没有此端点 → 前端回退 `GET /api/healthz` 读 version/channel。老后端没有 `locales` 字段 → 按单语言处理，隐藏开关。

**语言开关怎么画**（每端首页必须有，且要长成那一端自己的样子，见 `docs/EXPERIENCE_CORE.md` §1.1）：

| 要什么 | 从哪读 | 怎么写 |
|---|---|---|
| 能选哪些语言（设置页的列表） | `GET /api/version` → `locales.available` | — |
| 我的一主一副 | `GET /api/session/preferences` → `primaryLocale` / `secondaryLocale`（空 = 用默认值） | `PATCH /api/session/preferences`，两个字段**一起校验**，改哪个都建议都发 |
| 我现在读的是哪一种（首页开关翻的就是它） | `GET /api/session` → `language` | `PATCH /api/session` `{language}` |

- `secondaryLocale` 为空 = 这个用户只要一种语言，**隐藏开关而不是禁用**。
- 主副相同会被拒（`400 unsupported_locale`）—— 在自己和自己之间切换的按钮什么都不做。
- 写 `language` 时值必须在用户自己那两种里，否则 `400`；但**读永远不失败** —— 落在配对外的存量值会被静默钳到主语言，换配对不会让谁的设置页打不开。改配对时后端顺手把 `language` 拉回配对内。

> ⚠️ `locales` 是**新增性变更**，按规则该让 `apiMinor` +1，但**这一次没有升** —— 落地计划要求把散在各批次里的 APIMinor 提升合并到 SPEC-FREEZE 一次做完，否则会撞出 1.1/1.2/1.3 三个真源。SPEC-FREEZE 时连同其余新增字段一起升。

**双轨认证（cookie 全保留，header 新增；分离部署/原生端主推 header）**：

| 轨道 | 匿名会话 | 登录用户 |
|---|---|---|
| Cookie（同源 web，现状不变） | `dc_sid` httpOnly | `dc_auth` httpOnly JWT |
| Header（原生/分离前端） | `X-Session-Token: <signed>`（值 = `POST /api/session/init` 带 `{"tokenInBody":true}` 时响应里的 `sessionToken`） | `Authorization: Bearer <jwt>`（值 = login/register 带 `{"tokenInBody":true}` 时响应里的 `token`） |

- header 优先于 cookie；两轨可混用。登录后的 Bearer 请求经服务端 canonical data session 解析，**无需再带 X-Session-Token**（无匿名会话的纯 Bearer 注册也已支持——服务端自动铸造 canonical session）。
- **登出语义**：`POST /api/auth/logout` 递增服务端 token_version → **该用户所有已签发 JWT 全部失效**（全设备登出）；原生客户端同时自行丢弃本地 token。
- **CSRF**：header 轨天然免疫（自定义 header/Authorization 必触发 preflight）；cookie 轨维持 SameSite=Lax。跨域 cookie 场景可配 `COOKIE_SAMESITE=none`（强制要求 SECURE_COOKIES），但主推 header 轨。
- **CORS**：分离部署时后端设 `ALLOWED_ORIGINS` 显式列出前端域名（credentials 只对显式 allowlist 发放；wildcard 反射不带 credentials）。
- **纯 API 模式**：`STATIC_DIR=""` 即不托管前端，nginx/CDN 托管 `web/frontend/dist`，只代理 `/api`。

### H. 异步聊天协议（2026-07-15 新增，`POST /api/ai/companion/async`）

发完就走、无需保持连接的聊天模式（移动端/原生端主用；同步 SSE 端点保留给"看字蹦"的场景）：

1. **发送**：`POST /api/ai/companion/async {message, threadId(必填), timezone?, assistantName?}` → **202** `{threadId, userMessageId, messageId, status:"pending"}`。服务端已立即持久化 user 消息 + 一条 `status=pending` 的空 assistant 占位消息，agent 在服务器后台跑（客户端断网/杀进程都不影响）。
2. **取结果**：轮询 `GET /api/chat/messages/{messageId}`（轻量，推荐 2-5s 间隔），或直接刷新线程消息列表。`status` 语义：`pending`（跑着）→ `done`（`content` 即最终回复，`toolEvents` 是 SSE v2 帧对象数组——tool_start/tool_result/decision_card/error，渲染方式与流式动作卡完全一致）或 `error`（`content` 是本地化失败文案）。**空字符串 status ≡ done**（老数据兼容）。
3. **决策卡**：agent 弹卡时会把 `decision_card` 帧**立即**写进占位消息的 `toolEvents`（status 仍 pending）——轮询看到卡片后走现有 `POST /api/decisions/{id}/respond`，agent 继续；异步模式等待预算 **90s**（同步 SSE 为 45s），超时返回 `{"choice":"timeout"}` 让模型保守收尾。
4. **崩溃兜底**：服务器重启会把所有遗留 pending 占位消息清扫为 error，客户端不会永远轮询下去。
5. QQ 等通道消息本就是这个模式（无 UI 回应决策卡，故通道 agent 不带 propose_decision 工具）。

### I. 随手记录 → AI 归类 → 类型化资料（2026-07-15 新增）

**类别体系**：`GET /api/materials/categories` 返回全量注册表（10 类：note/diet/health/academic/travel 默认开，finance/fitness/idea/shopping/media 默认关）+ 本会话 enabled 标记。用户在设置页开关（`PATCH /api/session/preferences {"materialCategories":{"finance":true}}`）；note 不可关。资料页按 category 分模块展示；`GET /api/materials?category=` 过滤。

**随手记录流程**（资料页顶部的统一输入框：吃了什么、身体状况、作业截止、出行计划……）：

1. 用户随手写 → `POST /api/inbox/process {text}`（拍照则先 `POST /api/inbox/upload` 拿 temp_id）。
2. AI 归类（只在启用类别里选）→ 响应带 `classification {category, confidence, title, summary, structured, suggested_action, advice?}` + `draftId` → 前端渲染**确认卡**（类别徽标 + 标题 + 摘要 + 结构化字段预览，可改类别/标题）。
3. 用户确认 → `POST /api/inbox/commit {draftId, overrides?}` → 落库为对应类别的 Material（Body 是 `{"text","structured"?,"advice"?}` JSON）→ 响应 `{type:"material", material}`。
4. AI 不可用/低置信度：响应只有老键 `{understanding, suggestions}`（关键词回退），无 draftId——前端按旧逻辑展示建议即可。
5. 健康类的 `advice` 是保守自护建议（模板强制含就医提醒，不做诊断）；渲染时附免责声明样式。

各类别 structured 字段约定见 openapi/inbox_classify 模板（diet 含营养估计、academic 含 due_at、travel 含起止日期等）。

**类别专属流程**：

- **饮食拍照**：`upload` 图片 → `process {temp_id}` → 后端自动跑食物识别（vision）→ 返回 diet 确认卡（items/total 营养估计）+ 并列一个「这其实是课表」的 extract_image 建议 → `commit` 落 diet Material。非食物图片/识别失败自动回退课表建议。图片本体不持久化（1h 临时存储足够确认流程）。
- **出行**：`POST /api/ai/travel {destination, startDate?, endDate?, notes?}` → `{title, summary, itinerary{days[{date,theme,items[]}]}, tips[], draftId}` → 展示行程卡 → `commit` 落 travel Material。
- **学业**：归类结果带 `due_at` 时 `suggested_action=create_assignment`，`commit` 会直接建**手动作业**（进作业列表、参与 auto-plan；响应 `{type:"assignment", assignment}`）；`alsoKeepNote:true` 可同时留一条 academic 笔记。手动加作业也可直接 `POST /api/assignments {title, dueAt?}`。
- **健康**：确认卡附 `advice`（保守自护建议 + 就医提醒），落库进 Body；前端渲染时加免责声明样式。

---

## 0. 硬性要求：多语言（i18n）

前端从第一行代码起支持 zh-CN 与 en-US 双语（后端 prompt 按 locale 组织，AI 回复语言随会话语言）：

- 所有 UI 字符串外部化（实现于 `web/frontend/src/i18n.js`，两语全量文案）。
- 设置页语言切换：`PATCH /api/session/settings {"language": "en-US"}` 写回后端，同时切换前端 locale。
- `POST /api/session/init` 响应带 `language`（后端按 Accept-Language 推断默认）——用它初始化前端 locale。
- 日期/时间用 `Intl.DateTimeFormat(locale)`；星期几走翻译，禁止硬编码。
- **API 错误按 `error` 码翻译**（服务端 message 是中文兜底）。错误码表：

| error 码 | 场景 |
|---|---|
| `no_session` | 未初始化会话 |
| `bad_request` | 参数缺失/格式错误 |
| `rate_limited` | AI 请求过频 |
| `no_schedule_info` / `no_material` | 无可规划内容（引导导入资料） |
| `parse_error` / `server_error` | AI 输出异常/服务异常（提示重试） |
| `no_image` / `image_too_large` / `unreadable_image` / `not_a_timetable` / `vision_unavailable` | 图片相关 |
| `crisis_detected` | 危机干预（全屏温和提示，展示 message 原文） |
| `invalid_rule` / `rule_not_found` | 规则校验/不存在 |
| `range_too_large` | auto-plan 范围超限（默认 7 天） |
| `invalid_import_token` / `unsupported_export_version` / `invalid_ics` | 导入相关 |
| `unsupported_locale` | 语言不支持 |
| `invalid_theme` / `theme_not_found` / `not_a_theme` | 自定义主题（§8） |
| `fact_not_found` | 记忆条目不存在（§7） |
| `plan_not_found` / `assignment_not_found` / `unknown_prompt` / `unauthorized` / `internal` | 其余 |

## 1. 架构与运行

- 前端：`web/frontend/`（Vite + React 18）。开发：`npm run dev`（:5173，`/api` 代理到 :8080）；构建：`npm run build` → `dist/`。
- 部署：Go 服务本身托管 `dist/`（`STATIC_DIR`，SPA fallback），单二进制即可上线；nginx 前置可选（`deploy/nginx.conf`）。**同源部署**，cookie 自动携带。
- 设计系统：`web/frontend/src/vendor/ds-bundle.js`（`window.DaycoreUI`，26 组件）+ `web/frontend/src/ds/`（样式）；tokens 见 §5。

## 2. 对接要点（务必遵守）

- **会话**：首屏 `POST /api/session/init` → httpOnly `dc_sid` cookie；响应含 `assistantName`、`currentTheme`、`language`。
- **所有 fetch** 带 `credentials: "include"`。
- **AI 聊天 SSE**：见顶部「⚠️ v2 协议更新 §B」——`POST /api/ai/companion` 用 v2 tool-calls 帧协议（`{"type":...}` 帧 + `{"type":"done"}`），**不再是** `data:{"delta"}`＋`[DONE]`。
- **~~聊天标签协议~~（已废弃）**：v1 的 `<plan_update>`/`<rule_update>`/`<memory_update>` XML 标签方案已被 v2 tool-calls 取代。计划/规则/记忆变更由后端 agent 工具执行，前端只渲染 `tool_start`/`tool_result` 动作卡，**不再解析标签、不再自己发 PATCH/POST/DELETE**。撤销走 `tool_result.opId` → `POST /api/ops/{id}/revert`。（下方 §2 其余 REST 要点仍有效。）
- **计划读取**：`GET /api/plan?date=` / `GET /api/plan/range?from=&to=`。blocks 已合并规则展开块：`rule_id` 非空 = 来自规则（UI 加 ↻「重复」标识）；`origin` ∈ `auto`（AI）/ `manual` / `rule`；`sourceType:"rules"` = 纯规则虚拟日。
- **单块操作一律 PATCH**（完成/改时间/删除）：后端自动物化当天规则块并处理墓碑；禁止用 POST /api/plan 全量覆盖做单块修改。
- **图片上传**：`imageBase64` 不带 `data:` 前缀。

## 3. 信息架构（六个界面，实现于 `web/frontend/src/pages/`）

### 3.0 引导 Onboarding
3 步全屏：欢迎（呼吸光球）→ 导入资料（Canvas / ICS / 截图三卡，真实导入，可跳过）→ 一键生成第一份计划（GenOverlay + `POST /api/ai/auto-plan`）。

### 3.1 今日 Today（主界面）
- WeekStrip 7 日横条（周一起始，计划状态点）+ 月历跳转。
- **自主规划 CTA**（最显眼）→ AutoPlanSheet：范围（今天/明天/3 天/7 天/自定义区间 ≤7 天）→ 摘要 chips（📚 规则 ⏰ 临期 💡 keyFacts）→ 附加要求 → 模式（`keep_manual` 默认 / `replace_all`）→ GenOverlay → 回填计划 + note 反馈；`no_material` 引导去资料页。
- 时间块流：TimeBlockCard + now-line + 未定时间分组 + 规则块 ↻ 角标；点块开 BlockDetailSheet（编辑/完成/删除；规则块删除 = 当日隐藏，附「编辑规则」入口）。
- 手动添加（次级）：文字 → `/api/ai/plan-text`；截图 → `/api/ai/plan-image`；候选勾选确认后逐块 PATCH add。

### 3.2 资料 Materials
- 三导入卡：**Canvas**（上传插件导出 JSON → `/api/import/canvas`；「直连推送」Token Sheet：`GET/POST /api/import/token`、复制、轮换确认）、**ICS**（`/api/import/ics {preview:true}` → 候选确认 → `/api/rules/batch`）、**课表截图**（`/api/ai/extract-schedule-image` → 同候选流）。
- **重复规则子页**：列表（频率自然语言描述 + 来源徽标 + 启用开关）+ RuleEditor（一次性/重复、每天/每周几/每月/每 N 天、时间、时长、起止；`by_weekday` 0=周日…6=周六）+ 删除确认。
- 作业列表（due 分组着色、完成/忽略/恢复、planned 提示）+ 课程与成绩。
- **导入历史（§7）**：`GET /api/import/history`，展示时间、来源、条目数——让用户知道哪些内容已进记忆库。

### 3.3 陪伴 Companion
气泡流式聊天（SSE v2 帧，见顶部 §B）+ **动作卡由 `tool_start`/`tool_result` 帧驱动**（含记忆卡撤销，走 `opId`）+ 决策卡（`decision_card` 帧）+ 空态引导 chips + 自增高 composer。上下文（临期作业/规则/记忆 facts/愿望池）由**后端注入 prompt**（前端无需再传，服务端从 session 组装）。~~三种 XML 标签处理~~ 已废弃。

### 3.4 心情 Mood
12 宫格 MoodTile → `POST /api/ai/mood`（记忆 facts 已注入，回应更贴身）→ 负向情绪附练习邀请 → 三种全屏练习（呼吸 4-7-8 / 伸展 5×20s / 着地 5-4-3-2-1）→ 历史列表。**注意**：`exerciseOffered` 先算本地变量再 `POST /api/mood`（v1 异步 bug 勿复现）。

### 3.5 设置 Settings
- 助手名称（失焦保存）、语言切换（zh-CN/en-US）。
- **主题工作室（§8）**：4 内置 + 自定义主题卡网格（右键/长按菜单：重命名 / AI 微调 / 删除）+「AI 帮我调一套」（描述 + 可选基准 → `POST /api/ai/theme` → 全屏实时预览浮条：保存/再试/取消 → `POST /api/themes` + 应用）。
- **记忆区块（§7）**：facts 列表（内容 + 来源徽标）、删单条、确认后一键清空（隐私硬性要求）。
- 数据与导入入口、重看引导。
- **关于行：`Daycore v{version}` + 「v2 测试版」Badge**（版本来自 `GET /api/healthz` 的 `version`/`channel`）。
- 账户（头像入口）：AuthSheet（OAuth + 邮箱注册/登录）/ AccountSheet（退出）。

## 4. 浏览器插件（仓库根 `extension/`，已实现）
Chrome MV3：登录的 Canvas 页抓课程/成绩/作业 due → 导出 JSON/CSV，或配置本地地址 + Import Token 一键直推 `POST /api/import/canvas`。前端负责 Token 展示与「装插件」引导。

## 5. 设计 Tokens（玻璃拟态，来源 `web/frontend/src/ds/styles.css`，勿硬编码）

### 5.1 四套内置主题（`data-theme`）

| 主题 | id | primary | accent | bg 渐变 | 文字主色 |
|---|---|---|---|---|---|
| 天空蓝 ☁️ | sky | `#3b82f6` | `#60a5fa` | `#e0f2fe → #f0f9ff` | `#0f172a` |
| 暖橙日落 🌅 | sunset | `#f97316` | `#fb923c` | `#fff7ed → #fef3c7` | `#1c1917` |
| 深夜紫 🌙 | night | `#a78bfa` | `#c4b5fd` | `#1e1b4b → #312e81` | `#f0edff` |
| 自然绿 🌿 | nature | `#16a34a` | `#22c55e` | `#f0fdf4 → #ecfdf5` | `#052e16` |

深夜紫为深色主题：surface 半透明深紫 `rgba(55,50,120,.75)`，文字必须够亮（`#f0edff` / `#c4b5fd` / `#8b7fc2`）。

### 5.2 通用
圆角 card 20 / chip 999 / button 14 / sheet 24；动效 120/280/400ms；状态色 success `#22c55e` warning `#f59e0b` error `#ef4444`；surface 半透明白 `.72`（hover `.88`）+ `backdrop-filter: blur(16px)`；字体 Inter/SF Pro 栈；`prefers-reduced-motion` 全关动画；移动端 `env(safe-area-inset-*)` + `100dvh`。

## 6. 无障碍
- 深色主题亮文字；选中/高亮用 primary；危险操作 error 红。
- 规则块 ↻ 角标与 auto/manual 区分不只靠颜色（形状/文字并用）；色板选中态加勾选图标。

## 7. 每用户长期记忆（已实现）

**目标：AI 记得住每个用户。** 用户上传的所有内容 append-only 入库（Canvas 导出、ICS、课表截图确认结果），带来源与时间戳，永不覆盖旧数据。

- **存储**（按会话隔离）：`memory_facts`（结构化事实，AI 经 `<memory_update>` 或用户手动写入）+ `import_history`（每次导入的原始 payload 存档）。
- **注入**：companion / auto-plan / mood 组装 prompt 时注入 facts（companion 上下文带 fact id，供 remove 引用）。
- **API**：`GET/POST /api/memory`、`DELETE /api/memory/{id}`、`DELETE /api/memory`（清空）、`GET /api/import/history`。
- **UI**：设置页记忆区块（列表/删单条/确认清空）；聊天「已记住」动作卡（可撤销）；资料页导入历史。

## 8. 自定义主题 + AI 主题助手（已实现）

- CSS 设计 token 存库（`custom_themes`）：**12 变量白名单**（`--primary` `--accent` `--bg-start` `--bg-end` `--text-primary` `--text-secondary` `--text-muted` `--surface` `--surface-hover` `--success` `--warning` `--error`），值仅接受颜色字面量（hex / rgb() / rgba() / hsl() / hsla() / transparent），服务端强校验杜绝 CSS 注入；AI 输出同样消毒（无效项进 `warnings`）。
- `session.currentTheme` 可为内置 id 或自定义主题 id；删除在用主题 → 响应 `currentThemeReset:true` 并已重置为 `sky`（前端同步切回）。
- **前端应用方式**（实现于 `web/frontend/src/ui.jsx` 的 `applyThemeVars`）：内置主题走 `data-theme`；自定义主题以 `dark ? 'night' : (base || 'sky')` 为底座（继承深色文字/表面处理），再把变量映射写入 `:root`（含 border/tabbar 的 color-mix 派生）；切回内置时清除 inline 覆盖。
- 端点：`GET/POST /api/themes`、`PATCH/DELETE /api/themes/{id}`、`POST /api/ai/theme {description, base?, themeId?}`（候选预览制，不落库）。

---

> 后端契约以 `api/openapi.yaml` 为准。本文件取代已删除的 `FRONTEND_HANDOFF_UPDATE.md` 与 `web/` 下的旧契约副本；设计交付原件（原型 `app/*.jsx` 与 `_ds/`）已于 2026-07-25 移出仓库，实现副本见 `web/frontend/src/ds/` 与 `src/vendor/`。
