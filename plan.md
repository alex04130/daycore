# Daycore vNext 项目全景规划

> **基线**：v2.2.0-beta（仓库根为唯一实现面）
> **更新**：2026-07-13
> **性质**：单一权威路线图 + 已实现功能全量文档

---

## 一、产品定义

Daycore 是**替不太会规划的人省心的 AI 日程管家**——「静默执行 + 随口纠正」。

- **一句话**：系统持续产出默认正确的计划，用户只做两件事——丢东西进收件箱、对卡片点确认或撤销
- **成功标准**：用户每天主动操作 < 5 次、每次 < 3 秒；系统每天替用户做 > 20 个决定，每个决定可见可撤销
- **目标用户**：学生群体（Canvas 课程、作业 deadline、课表截图），架构不限学生场景

### 交互分层

| 层级 | 名称 | 机制 |
|------|------|------|
| L1 | 静默执行 | AI 产出直接生效，`origin=auto` 虚线边框。永不覆盖 manual 块。连续 5 次被撤销降级为「仅建议」 |
| L2 | 对话建议 | AI 不确定时自然语言问，用户回一句话即可 |
| L3 | 决策卡 | 五类场景弹出（冲突/空档推荐/规则确认/资料关联/重排预确认），一次一张，非阻塞可忽略 |

---

## 二、设计原则

### 2.1 提示词分层铁律

```
┌─────────────────────────────────────────────┐
│ L1_hard    │ 纯防线    │ 工具纪律 + 安全底线 │ ← ZERO 人格
│ L3_context │ 数据层    │ 日期/计划/记忆/天气 │ ← 纯数据
│ L2_persona │ 角色层    │ 默认"好伙伴"+ 自定义│ ← ALL 人格
│ L1_reminder│ 防线重申  │ 硬约束末尾再声明    │ ← ZERO 人格
└─────────────────────────────────────────────┘
```

组装：`L1_hard → L3_context → L2_persona → L1_reminder`。硬边界首尾各出现一次，L2 夹在中间无法覆盖 L1。

### 2.2 通道插件化

Channel 接口 + Registry。QQ(OneBot)/微信/飞书/Telegram 都是实现。类似 OpenClaw 插件模型。

### 2.3 KV 缓存 per-provider

| Provider | 机制 | 实现 |
|----------|------|------|
| Anthropic | 显式 `cache_control: {"type":"ephemeral"}` | system 消息自动追加 |
| OpenAI | `prompt-cache-key` header | ChatRequest.CacheKey → HTTP header |
| DeepSeek | 全自动磁盘缓存 | 零代码改动，前缀一致即可 |
| Gemini | 独立 cache API | 远期 |

前缀排序：`[tools] → [L1] → [reminder] → [L3] → [L2] → [history] → [user msg]`。固定在前，动态在后。

### 2.4 搜索适配器

Searcher 接口（只关心输入/输出）。SQLite→FTS5, PG→tsvector, MySQL→FULLTEXT, Mongo→text index。

### 2.5 通用适配器模式

所有接口按适配器模式——只关心输入什么结果，不关心内部实现。接口放 `domain/`，实现放各自包，Server 依赖注入接受接口。

| 接口 | 状态 |
|------|------|
| AIProvider | ✅ 接口，含 Chat/ChatStream/Capabilities/Model |
| Searcher | ✅ 接口 + MaterialSearcher 实现（跨方言子串匹配；原生 FTS5/tsvector/FULLTEXT/text 索引为后续性能优化） |
| Channel | ✅ 接口，Name/Send/Start/Stop |
| Store (DB) | ✅ 接口，4 后端 |
| WeatherProvider | ✅ 接口 + 4 实现（Open-Meteo/QWeather/OpenWeatherMap/wttr.in）+ registry 自注册 + fallback 链（主→wttr.in）+ 30min 缓存 |
| STTProvider | P5（语音转文字，QQ 语音→agent） |
| TTSProvider | P∞ |
| ImageProvider | P∞ |

### 2.6 时区架构

三类时间模式：`floating`（跟墙钟）| `fixed`（UTC 锚点）| `local`（生理节律，跟随当地时区）

TimeBlock 含 `UTCTime`(*string)、`OffsetMin`(*int)、`OffsetRef`(string)。`timeutil/convert.go`：ToUTC/FromUTC。

### 2.7 提示词分发

三层链：DB 覆盖(Admin UI) > 磁盘文件(PROMPTS_DIR) > go:embed(编译默认)

`daycore install` 向导式安装提取模板到磁盘。`DEV_MODE=true` + fsnotify 热重载。`-tags lite` 构建零嵌入二进制。

### 2.8 其他拍板

- 工具上限：10+
- L2 默认「好伙伴」非「管家」
- 回退语言：`en-US`
- 前端 = Claude Design。`FRONTEND_HANDOFF.md` + `openapi.yaml` = API 契约
- 每张新表 = 3 方言 DDL + mongostore 文档 + Store 接口

---

## 三、全量 API 路由

### 3.1 会话与认证

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | /api/session/init | 创建/获取匿名 session |
| POST | /api/session/theme | 记录主题切换 |
| PATCH | /api/session/settings | 更新助手名/主题/语言/PersonaPrompt |
| PATCH | /api/session/preferences | 更新主动功能 7 个 toggle |
| GET | /api/session/preferences | 获取偏好 |
| POST | /api/auth/register | 注册 |
| POST | /api/auth/login | 登录 → claim + merge data session |
| POST | /api/auth/logout | 登出 |
| GET | /api/auth/providers | OAuth 提供商列表 |
| GET | /api/auth/oauth/{provider} | OAuth 发起 |
| GET | /api/auth/oauth/{provider}/callback | OAuth 回调 |
| GET | /api/me | 当前用户 |

### 3.2 日程与规则

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | /api/plan?date= | 单日计划（含规则虚拟展开） |
| POST | /api/plan | 保存/更新 |
| PATCH | /api/plan | 增量修改 |
| GET | /api/plan/range?from=&to= | 日期范围查询 |
| GET/POST | /api/rules | 规则列表/创建 |
| POST | /api/rules/batch | 批量创建 |
| PATCH/DELETE | /api/rules/{id} | 修改/删除 |

### 3.3 AI 端点

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | /api/ai/companion | 核心——Agent SSE v2（11 工具 loop + threadId + 服务端上下文 + maybeCompress） |
| POST | /api/ai/auto-plan | 自主规划 |
| POST | /api/ai/plan-text | 自然语言 → 日程 JSON |
| POST | /api/ai/plan-image | 图片 → 日程（vision pipeline） |
| POST | /api/ai/extract-schedule-image | 课表截图 → 规则候选 |
| POST | /api/ai/mood | 心情 → AI 回应 |
| POST | /api/ai/theme | AI 生成主题配色 |
| POST | /api/decisions/{id}/respond | 决策卡响应 |

### 3.4 聊天线程

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | /api/chat/threads | 列出线程（首次自动从 companion_memory 导入） |
| POST | /api/chat/threads | 创建 |
| PATCH | /api/chat/threads/{id} | 重命名/归档/更新 Summary |
| DELETE | /api/chat/threads/{id} | 删除 + 全部消息 |
| GET | /api/chat/threads/{id}/messages?before=&limit= | cursor 分页 |
| DELETE | /api/chat/threads/{id}/messages | 清空消息（保留线程） |

### 3.5 资料库

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | /api/materials | 列表（?category=&q=&limit=&offset=） |
| POST | /api/materials | 创建 |
| GET | /api/materials/search?q=... | 全文搜索（Searcher 接口） |
| GET/PATCH/DELETE | /api/materials/{id} | 详情/编辑/删除 |

### 3.6 愿望池

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | /api/wishes?status= | 列表 |
| POST | /api/wishes | 创建 |
| GET | /api/wishes/{id} | 详情 |
| PATCH | /api/wishes/{id} | 编辑/标记完成 |
| DELETE | /api/wishes/{id} | 删除 |

### 3.7 临时上下文

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | /api/temp-context?key= | 获取（含 TTL 检查） |
| PUT | /api/temp-context | 写入 `{key, payload, ttl}` |

### 3.8 收件箱

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | /api/inbox/process | AI 理解 → 确认卡建议 |
| POST | /api/inbox/upload | 文件上传（≤10MB），返回临时 ID |

### 3.9 通道绑定

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | /api/channels | 可用通道 + 绑定状态 |
| POST | /api/channels/{channel}/bind | 生成 12 位 token（10 分钟过期） |
| POST | /api/channels/{channel}/verify | token + externalId 验证 |
| DELETE | /api/channels/{channel}/unbind | 解绑 |

### 3.10 反馈与偏好

| 方法 | 路由 | 说明 |
|------|------|------|
| POST | /api/feedback | 记录 `{messageId, useful}` |
| GET | /api/session/preferences | 获取 7 toggle 状态 |
| PATCH | /api/session/preferences | 部分更新偏好 |

### 3.11 记忆、心情、主题

| 方法 | 路由 | 说明 |
|------|------|------|
| GET/POST/DELETE | /api/memory | 长期记忆 CRUD（含 type:open_loop/habit） |
| DELETE | /api/memory/{id} | 删除单条 |
| GET/POST/PATCH | /api/mood | 心情打卡 |
| GET/POST | /api/themes | 主题列表/创建 |
| PATCH/DELETE | /api/themes/{id} | 主题修改/删除 |

### 3.12 导入、课程、作业

| 方法 | 路由 | 说明 |
|------|------|------|
| GET/POST | /api/import/token | 导入 Token 管理 |
| POST | /api/import/canvas | Canvas 直推 |
| POST | /api/import/ics | ICS 上传 |
| GET | /api/import/history | 导入历史 |
| GET | /api/courses | 课程列表 |
| GET | /api/assignments | 作业列表 |
| PATCH | /api/assignments/{id} | 状态更新 |

### 3.13 操作、管理

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | /api/ops | 操作日志 |
| POST | /api/ops/{id}/revert | 撤销 |
| GET | /api/admin/prompts | 提示词列表 |
| GET | /api/admin/prompts/{key}?locale= | 单个（含默认值对照） |
| PUT | /api/admin/prompts/{key}?locale= | 覆盖（template 验证） |
| GET | /api/admin/stats | 仪表盘聚合 |
| GET | /api/admin/ailogs | AI 日志（分页+筛选） |
| GET | /api/admin/users | 用户列表 |
| DELETE | /api/admin/users/{id} | 删除+级联 |
| GET | /api/admin/db/tables | 表列表 |
| GET | /api/admin/db/table/{name} | 浏览 |
| DELETE | /api/admin/db/table/{name}/{id} | 单行删除 |
| GET | /api/admin/db/export | JSON 导出 |
| POST | /api/admin/db/import | JSON 导入 |
| GET | /api/admin/db/backup | SQLite 备份 |

### 3.14 其他

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | /api/healthz | 健康检查（version/channel） |
| GET | /api/models | 模型目录 |

---

## 四、系统架构

### 4.1 请求生命周期

```
用户消息 (+ 附件 + 通道入站)
 → sessionMW → dataSessionMW → userMW
 → companionSystemPrompt(L1+L3+L2+reminder)
 → maybeCompress(>60%阈值→flash压缩→摘要+开环)
 → ChatStream(tools=11, stream=true, cache_control)
 → StreamAccumulator(聚合delta+tool_call分片)
 → 若tool_calls: 服务端执行→operation_log→追加role=tool→再调模型(≤6轮)
 → 若propose_decision: decision_card帧+阻塞(≤45s)→user respond→继续
 → SSE v2 帧流 → 前端/通道
 → AppendMessages 持久化到 chat_threads
 → 后台worker(cron): 简报/DDL检测/重排→已绑定通道推送
```

### 4.2 存储

四后端兼容：SQLite(默认零依赖) / PostgreSQL / MySQL / MongoDB。

sqlstore 22 文件、mongostore 17 文件、domain 14 文件。

### 4.3 AI

DeepSeek V4 Flash(默认) / Reasoner / OpenAI GPT-4o / Anthropic Claude / 本地 Ollama。

`models.yaml` 数据驱动，零代码换模型。`prompts/` 三层分辨率链（DB > 磁盘 > embed）。

### 4.4 安装

```bash
daycore install --dir ./data
```

6 步向导：
1. Database — 引擎选择 (sqlite/pg/mysql/mongo) + DSN
2. Templates — 从 embed 解包 .tmpl 到 disk
3. API Keys — DeepSeek/OpenAI/Anthropic（回车跳过）
4. Channels — OneBot WS URL + Token（可选）
5. Secrets — 自动生成 Admin Token / JWT Secret / Cookie Secret
6. .env — 完整配置写入

轻量二进制：`go build -tags lite -o daycore-lite ./cmd/daycore`

---

## 五、数据模型

| 实体 | 主键 | 关键字段 |
|------|------|---------|
| Session | session_id | AssistantName, PersonaPrompt, Language, Preferences(JSON) |
| User | user_id | Email, Name, IsAnonymous, DataSessionID |
| DayPlan | (session_id,date) | Blocks(TimeBlock数组) |
| TimeBlock | — | Date, Time, Title, Type, DurationMin, TimeMode(floating/fixed/local), Timezone, UTCTime, OffsetMin, OffsetRef, Origin(auto/manual/rule), Completed, Hidden |
| ScheduleRule | (session_id,id) | Kind(recurring/once), Freq, ByWeekday, StartTime, Timezone, TimeMode |
| MemoryFact | (session_id,id) | Fact, Source(chat/user/import/auto), Type(fact/open_loop/habit) |
| ChatThread | (session_id,id) | Title, Summary, Archived |
| ChatMessage | (thread_id,id) | Role, Content, ToolEvents(JSON) |
| ChannelBinding | (channel,external_id) | SessionID, DisplayName, Metadata(JSON), VerifiedAt |
| FeedbackLog | (session_id,id) | MessageID, Useful |
| Material | (session_id,id) | Category, Title, Summary, Body, Source, MimeType, Tags, StorageRef |
| Wish | (session_id,id) | Title, Note, EffortMin, Status(active/done/archived) |
| TempContext | (session_id,key) | Payload, TTL |
| OperationLog | (session_id,id) | Actor, Action, TargetID, Before/After 快照 |
| AICallLog | (session_id,id) | Endpoint, Model, PromptTokens, CompTokens, DurationMs |
| MoodCheckin | (session_id,id) | Mood, AIResponse |
| CustomTheme | (session_id,id) | Name, Base, Dark, Variables(CSS JSON) |

---

## 六、Agent 工具

| 工具 | 说明 |
|------|------|
| plan_add | 新增时间块（支持 time_mode/timezone） |
| plan_update | 修改时间块（match + changes） |
| plan_remove | 删除时间块 |
| rule_upsert | 创建/修改重复规则 |
| rule_remove | 删除规则 |
| memory_add | 记住一条事实 |
| memory_remove | 忘记一条事实 |
| get_weather | 天气预报（Open-Meteo） |
| web_search | 联网搜索（Tavily→DuckDuckGo） |
| list_upcoming | 未来计划 + 作业总览 |
| propose_decision | 决策卡（阻塞等用户选择） |

---

## 七、后台 Worker

`internal/server/worker.go`（~350 行），基于 `robfig/cron/v3`：

| 任务 | 触发 | 说明 |
|------|------|------|
| 晨简报 | 07:30 | AI 生成今日概览+天气+开环回问，通道推送 |
| 晚复盘 | 21:00 | 完成度+心情+明日最重要一件事 |
| Deadline 检测 | 每 2h | 48h 内 assignments.due_at 未完成→推送预警 |
| 滚动重排 | 每 30min | today blocks 超时→decision_card |

所有任务尊重 `session.preferences`（7 个 toggle）和 DoNotDisturb。

---

## 八、通道安全

| 功能 | 实现 |
|------|------|
| 白名单 | ValidateBinding 回调——仅已绑定用户 |
| 频控 | 每用户 10 条/60 秒，超限→冷却消息 |
| 群聊过滤 | 仅 @机器人 或 /command 触发 |
| Token 绑定 | 12 位随机 token，10 分钟过期，用户发给 bot 防骚扰 |

---

## 九、已完成进度总览

### P0 — 止血与信任 ✅
修日期链 / 任意日期 / SSE 加固 / operation_logs 全链路 / 日期回归 / MaxTokens / i18n 校验

### P1 — Agent 脊椎 ✅
流式 tool_calls / 模型配置 / weather+search / 11 工具 loop / SSE v2 / 决策卡 / 撤销 / 前端事件化 / DeepSeek 搜索

### P2 — 身份、会话、管理员 ✅
匿名+canonical session+claim / ChatThreads+threadId+clear / 上下文摘要+开环 / L2 提示词 / Admin 12+ 路由

### 提示词架构重构 ✅
L1 纯防线（13 条规则，零人格）/ L2 好伙伴角色（DefaultPersona+用户可覆盖）/ L1_reminder 防线重申

### 代码拆分 ✅
mongostore 1→17 / dialect 1→4 / server +10 / domain 1→12

### P3 — 通道 + 主动性 ✅
Channel 接口+Registry+OneBot / channel_bindings 全链路 / worker cron 四项 / feedback_logs / session.preferences / Leo 学习习惯

### 时区 UTC 转换 ✅
timeutil/convert.go / TimeBlock UTC 字段 / TimeLocal 模式

### P4 — 资料库+愿望池 ✅
Searcher 接口 / Material 全链路 / Wish 全链路+心情推测 / TempContext TTL / 收件箱

### 安装工具 ✅
daycore install 6 步向导 + lite build tag + .env 生成

### KV 缓存 ✅
Anthropic cache_control + OpenAI prompt_cache_key

### 验证补完批次（2026-07-13）✅
全面审计发现「已完成」与实现有差距，本批次修复补全：
- **安全**：XFF 限速绕过、CORS `*`+credentials、OAuth 未验证邮箱接管、dev 弱密钥+开放 admin（改 loopback fail-safe）、JWT 撤销（token_version）、auth 限速+argon2 并发上限、admin 恒定时间比较、Mongo `$regex` ReDoS、mood IDOR、health 错误泄漏、agent memory 长度、message role 白名单。
- **P0-P2 bug**：撤销四路（rule_delete/rule_batch/memory_delete/memory_clear 补 `{before,after}`）、登录合并补全 8 类表迁移 + ruleMergeKey 全字段、chat 实时对话服务端持久化、分页 tied-timestamp、压缩 TrimPrefix/开环去重/summary 回读、rule PATCH 验证前置、matchesAll panic 防护。
- **时区**：实装 floating/fixed/local 三模式（保存锚定 UTCTime + 读取回填 + DST），补 timeutil 测试。
- **P3 接线**：main.go 接线 Registry/OneBot/Worker + inbound 消费 + ScheduleUser；bind→verify 修复（pending-token 查询 + Promote + TTL sweep）；cron CRON_TZ（修时区+6字段解析失败）；OneBot @ 精确匹配 + 频控 map 驱逐；GapSuggestions 生效；feedback 接入 admin stats。
- **P4 补完**：Searcher（MaterialSearcher 跨方言）+ 接线；TempContext 塌缩修复（UPDATE-then-INSERT + TTL 过滤 + GET null）；inbox upload 持久化+auth；Wish JSON camelCase；wish↔mood 联动。
- **天气**：重构为 WeatherProvider 适配器（见 §2.5）。
- 回归测试：撤销/规则/通道绑定/时区/TempContext 隔离/搜索/天气 provider 均覆盖。

---

## 十、P5 实际计划

已拍板本阶段实际落地的三项（其余发散功能移入 P∞ 愿景）。

### 10.1 多语言 Phase 1（ja / ko / es / fr / de）

| 阶段 | 语言 | L1 | L3 | L2 |
|------|------|----|----|-----|
| 当前 | zh-CN, en-US | ✅ | ✅ | ✅ |
| Phase 1 | ja, ko, es, fr, de | ❌ | ❌ | ❌ |

每语言 = 8 `.tmpl`（auto_plan/day_plan_text/day_plan_image/companion_agent/companion_context/mood/schedule_extract_image/theme_gen）+ `i18n.Supported` 更新。回退 en-US。

### 10.2 STT（语音转文字）
新增 `domain.STTProvider` 接口（照 WeatherProvider 适配器：接口放 domain，实现放子包，registry 自注册）。实现 Whisper / whisper.cpp。OneBot 语音消息（`[CQ:record]`）→ 转文字 → 现有 agent 管线。TTS/语音克隆留 P∞。

### 10.3 PWA
后端仅需 manifest + service worker 静态文件支持；UI/离线策略由 Claude Design 落实（写入 FRONTEND_HANDOFF）。

---

## 十一、P∞ 愿景

长期发散方向，无排期。

- **文生图**：`ImageProvider` 接口——DALL-E/Stable Diffusion。日程配图、主题背景、心情可视化。
- **TTS + 语音克隆**：OpenAI TTS/Edge/Piper 播报；OpenVoice/Coqui/GPT-SoVITS 让 Leo 用用户声音说话（差异化功能）。
- **数据交叉分析（VIP）**：mood×完成率周报 / 精力画像 / 学期报告 / 预测引擎。
- **社区市场**：规则模板市场 / 主题市场 / 翻译贡献。
- **评测**：golden 评测集 CI / A/B 提示词对比。
- **自由插件**：开放工具定义 API / Webhook 通知。
- **原生全文搜索**：Searcher 升级为 SQLite FTS5 / PG tsvector / MySQL FULLTEXT / Mongo text index（当前为跨方言子串匹配）。
- **APP 深化**：Capacitor 打包 → CalDAV 同步。
- **每用户时区**：session 增加 timezone 字段，worker 简报按真实用户时区（当前用 `WORKER_DEFAULT_TZ` 全局默认）。

---

## 十二、已拍板决策

| 决策 | 结论 |
|------|------|
| Beta | 无包袱直切 |
| 身份 | 匿名可用+登录合并 canonical session |
| 搜索 | Tavily→DuckDuckGo；DeepSeek 原生备选 |
| 通道 | OneBot 先行，微信/飞书/Telegram 后续 |
| 天气 | WeatherProvider 适配器：Open-Meteo 默认 + QWeather/OpenWeatherMap + wttr.in fallback + 30min 缓存 |
| 模型 | DeepSeek V4 Flash 默认 |
| 提示词 | L1=纯防线+零人格；L2=好伙伴+用户覆盖；reminder 末尾兜底 |
| 前端 | Claude Design——FRONTEND_HANDOFF+openapi 为契约 |
| 时区 | 三类模式；UTC 存储；前端按需显示 |
| 工具上限 | 10+ |
| 回退语言 | en-US |
| KV 缓存 | per-provider 策略（Anthropic 显式/OpenAI header/DeepSeek 自动） |
| 分发 | `daycore install` + `-tags lite` 零嵌入二进制 |

---

## 十三、环境变量参考

| 变量 | 默认 | 说明 |
|------|------|------|
| DEEPSEEK_API_KEY | — | AI 功能必需 |
| DB_TYPE | sqlite | 数据库 |
| DB_DSN | file:daycore.db?... | 连接串 |
| JWT_SECRET | — | 生产必须 |
| COOKIE_SECRET | — | 生产必须 |
| ADMIN_TOKEN | — | Admin API |
| HOST | (all) | 监听 |
| PORT | 8080 | HTTP |
| DATA_DIR | — | 数据目录 |
| PROMPTS_DIR | ./prompts | 提示词路径 |
| DEV_MODE | false | 热重载 |
| ONEBOT_WS_URL | — | QQ Bot（配置后才接线 Worker/通道） |
| ONEBOT_TOKEN | — | QQ Bot Token |
| WORKER_DEFAULT_TZ | Asia/Shanghai | 简报调度时区（session 暂无 tz 字段） |
| AGENT_MAX_ROUNDS | 6 | 工具轮次上限 |
| AI_RATE_LIMIT_PER_MIN | 30 | AI 端点限速 |
| AUTH_RATE_LIMIT_PER_MIN | 10 | 登录/注册限速（防爆破+argon2 DoS） |
| AI_REQUEST_TIMEOUT | 120s | 超时 |
| APP_ENV | development | 生产必须设 production（强制密钥、fail-safe） |
| TRUST_PROXY_HEADERS | false | 仅可信反代后开启，才信任 X-Forwarded-For |
| SECURE_COOKIES | false（prod 默认 true） | Cookie Secure 标志 |
| WEATHER_PROVIDER | open-meteo | open-meteo/qweather/openweathermap/wttr |
| QWEATHER_API_KEY | — | 和风天气 key（选 qweather 时） |
| OPENWEATHERMAP_API_KEY | — | OpenWeatherMap key |

---

> 本文为规划共识稿。进度跟踪：`/home/alex/.claude/plans/plan-md-project-md-daycore-agent-final-whimsical-wall.md`
> 代码仓库审计：[[daycore-codebase-audit]]
