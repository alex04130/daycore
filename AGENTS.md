# Daycore v2 开发指南

Daycore v2 是一个 Go 1.23 单二进制后端 + React/Vite 前端的 AI 日程管家。支持 SQLite / PostgreSQL / MySQL / MongoDB 四数据库后端一键切换，DeepSeek V4 Flash 为默认模型，全程流式 Agent 循环管理用户日程。

## 1. 技术栈

| 层面 | 技术选择 | 说明 |
|------|----------|------|
| **后端语言** | Go 1.23 | 编译为单二进制静态文件（CGO_ENABLED=0），零外部运行时依赖 |
| **数据库** | SQLite 默认 / PostgreSQL / MySQL / MongoDB | Dialect 抽象，`DB_TYPE` + `DB_DSN` 一键切换 |
| **AI 引擎** | DeepSeek API 默认（V4 Flash 对话 / V4 Pro 规划 / Anthropic 端点原生搜索） | `ai.Provider` 接口 + registry 自注册模式，OpenAI / Anthropic / Ollama 三种格式可插拔 |
| **模型配置** | `config/models.yaml` | 零代码新增模型——标 `tools: true` 即启用 function calling，标 `vision: true` 即启用视觉，标 `thinking: true` 即透传 DeepSeek reasoning |
| **身份认证** | argon2id 密码哈希 + JWT（ES256K/HS256）+ httpOnly session cookie（`dc_sid`）+ OAuth（yaml 零代码可配） | 匿名优先——首次访问自动建匿名 User，登录时通过 canonical data session 合并数据 |
| **前端** | React + Vite | 开发模式 `:5173` 代理到 `:8080`（热更新）；生产由 Go 单二进制直接托管 `STATIC_DIR` |
| **浏览器插件** | Chrome MV3 | 抓取 Canvas Dashboard → 课程/成绩/作业 JSON 导出或直推 |
| **i18n** | zh-CN / en-US 全站切换 | AI 回复语言跟随 session locale；提示词按 locale 组织（8 key × 2 locale = 16 个模板）；前端 391 keys 单文件扁平字典 |
| **部署** | Go 单二进制 + nginx/Caddy/Docker | 单二进制 = 一个文件部署；`STATIC_DIR` 指向前端构建产物即可自托管 |
| **调度** | 环境变量控制 | `AUTO_PLAN_MAX_DAYS`（默认 7）/ `AGENT_MAX_ROUNDS`（默认 6）/ `AI_RATE_LIMIT_PER_MIN`（默认 30）/ `AI_REQUEST_TIMEOUT`（默认 120s） |
| **版本管理** | `internal/version/version.go` | 单一来源：`2.1.0-beta`；`GET /api/healthz` 返回 `version` / `channel` |
| **Admin** | `ADMIN_TOKEN` 鉴权 | 提示词运行时覆盖 `PUT /api/admin/prompts/{key}?locale=` |

## 2. 快速开始

### 2.1 最简启动（SQLite + DeepSeek）

```bash
cd v2

# 1. 设置 AI 密钥
echo 'DEEPSEEK_API_KEY=sk-your-key' > .env

# 2. 构建前端（首次或前端改过后）
cd web/frontend
npm install
npm run build
cd ../..

# 3. 启动 —— 单二进制，:8080
go run ./cmd/daycore
```

打开 `http://localhost:8080` 即可使用。

### 2.2 开发模式（前端热更新）

```bash
# 终端 1 —— 启动后端
cd v2
go run ./cmd/daycore

# 终端 2 —— 启动前端开发服务器
cd v2/web/frontend
npm run dev    # Vite :5173, /api → :8080 代理
```

打开 `http://localhost:5173`，前端改代码立即热更新。

### 2.3 切数据库

```bash
# PostgreSQL
DB_TYPE=postgres DB_DSN='postgres://user:pass@localhost:5432/daycore?sslmode=disable' go run ./cmd/daycore

# MySQL
DB_TYPE=mysql DB_DSN='user:pass@tcp(localhost:3306)/daycore?parseTime=true' go run ./cmd/daycore

# MongoDB
DB_TYPE=mongodb DB_DSN='mongodb://localhost:27017/daycore' go run ./cmd/daycore
```

### 2.4 启用 thinking 模型做自治规划

```bash
DEFAULT_PLANNER_MODEL=deepseek-reasoner go run ./cmd/daycore
```

### 2.5 启用联网搜索

```bash
TAVILY_API_KEY=tvly-your-key go run ./cmd/daycore
```

不设置 `TAVILY_API_KEY` 时自动降级为 DuckDuckGo 免费搜索。也可以用 `deepseek-search` 模型（走 DeepSeek Anthropic 端点的服务端原生搜索）。

## 3. 开发命令

```bash
cd v2

# 后端
go run ./cmd/daycore          # 启动（默认 SQLite, :8080）
make run                      # 同上
make test                     # 全量测试（含 i18n 校验 → go test ./...）
make vet                      # 静态分析
make build                    # CGO_ENABLED=0 → bin/daycore
make docker                   # Docker 镜像

# 前端
cd web/frontend
npm install                   # 安装依赖
npm run dev                   # Vite 开发服务器 (:5173, /api → :8080)
npm run build                 # 生产构建 → dist/
npm run check:i18n            # i18n key 双向校验

# 单二进制模式
cd web/frontend && npm run build
cd ../.. && go run ./cmd/daycore  # Go 托管前端静态文件 (:8080)

# 只跑特定包的测试
go test ./internal/server/ -run TestAgent -v
go test ./internal/ai/formats/openai/ -v
go test ./internal/storage/sqlstore/ -run TestOperationLogs -v

# 端到端测试（Playwright 驱动）
node web/frontend/scripts/drive.mjs http://localhost:5173 <fixtures> <outdir>
```

### 3.1 数据库

项目不使用 ORM——全部手写 SQL，通过 Dialect 抽象兼容三引擎。MongoDB 独立实现。

```bash
# 建表在启动时自动执行（Migrate()），无需手动命令
# 以下为检查/调试用：

# SQLite 直接查看
sqlite3 daycore.db ".tables"
sqlite3 daycore.db ".schema operation_logs"

# 所有表在 sqlstore/dialect.go 的三个 Migrations() 中定义
# 列补丁在 sessionColumnMigrations() 中定义，启动时自动执行
```

## 4. 项目结构

```
v2/
├── cmd/daycore/main.go                # 装配入口：注册驱动/格式 → 打开 Store → Migrate → HTTP 服务（优雅关闭）
├── config/
│   ├── models.yaml                    # 模型目录（数据驱动，加模型零代码改）
│   └── oauth.yaml                     # OAuth provider 配置（零代码新增）
├── internal/
│   ├── ai/                           # AI 抽象层
│   │   ├── provider.go               #   AIProvider 接口（Chat / ChatStream） + Chunk（ContentDelta / ReasoningDelta / ToolCallDelta / FinishReason） + Message / ToolDef / ToolCall 类型
│   │   ├── models.go                 #   从 YAML 加载模型目录（modelEntry → Catalog）；DefaultChat / Planner / Vision 选择器
│   │   ├── registry.go               #   格式自注册（RegisterFormat）；ModelConfig → Factory → AIProvider
│   │   ├── stream.go                 #   StreamAccumulator（流式 tool_call 按 index 聚合） + StreamViaChat（伪流适配器）
│   │   ├── tools.go                  #   vision 工具定义（read_image / zoom_image）
│   │   ├── vision.go                 #   3 策略图像处理管道 + 多轮非流式 tool loop（max 6 轮，Agent 循环的参考实现）
│   │   ├── datectx.go                #   BuildDateContext（相对日期对照表，支持 zh/en 双语标签，周一起点语义）+ WeekdayName + nextWeekdayDate
│   │   ├── prompts.go                #   PromptService：go:embed 内嵌 8 key × 2 locale = 16 个 .tmpl，admin 可运行时覆盖
│   │   ├── imageutil.go              #   图片工具（base64 编解码、MIME 检测）
│   │   ├── prompts/
│   │   │   ├── zh-CN/                #   auto_plan / companion_agent / companion_context / day_plan_image / day_plan_text / mood / schedule_extract_image / theme_gen
│   │   │   └── en-US/                #   同上 × en
│   │   └── formats/
│   │       ├── openai/openai.go      #   OpenAI 兼容格式（DeepSeek 也用这个）：流式 tool_calls/reasoning 分片解析 + extra_body 浅合并 + max_tokens 默认
│   │       ├── anthropic/anthropic.go #  Anthropic Messages API：非流式 tool_use/tool_result 块 + ServerSide 工具（web_search_20250305）+ ChatStream 自适应
│   │       └── ollama/ollama.go      #   Ollama 原生 /api/chat：base64 视觉图片 + newline JSON streaming
│   ├── server/                       # HTTP 层
│   │   ├── server.go                 #   路由注册（60+ 条路由），中间件链（recover → requestID → logging → cors → sessionMW → userMW → dataSessionMW），上下文键定义
│   │   ├── agent.go                  #   runCompanionAgent 主循环（全程流式，max 6 轮，工具失败不中断） + streamRound + runProposeDecision（决策卡阻塞 ≤45s）+ decisionRegistry（per-session 单卡）+ SSE v2 sender（send / ping / fail）
│   │   ├── agent_tools.go            #   11 个工具定义（companionToolDefs，按 caps 条件注入 ServerSide 工具） + runCompanionTool 执行器 + 每个 tool 的独立实现（toolPlanAdd / toolPlanPatch / toolRuleUpsert / toolRuleRemove / toolMemoryAdd / toolMemoryRemove / toolGetWeather / toolWebSearch / toolListUpcoming）
│   │   ├── handlers_ai.go            #   handleAICompanion（SSE v2 入口，服务端权威上下文） + companionSystemPrompt（L1+L2+L3 组装）+ planBlocksJSON / moodHistoryContext + 其他 AI handler（plan-text / plan-image / extract-schedule-image / mood）
│   │   ├── handlers_autoplan.go      #   自主规划器（聚合 rules/assignments/memory → AI → JSON → parseAutoBlocks → Upsert）
│   │   ├── handlers_plan.go          #   计划 CRUD + applyPlanPatch 共享核（HTTP PATCH 与 agent 工具共用，match.date 防御内建）+ planBlocksForDate / visibleSorted
│   │   ├── handlers_rules.go         #   规则 CRUD（Create/Batch/Patch/Delete）+ ruleInput → toRule → validateRule 校验链
│   │   ├── handlers_memory.go        #   长期记忆 CRUD + DELETE 快照（可撤销）+ CLEAR 快照（可撤销）
│   │   ├── handlers_chat.go          #   ChatThread CRUD（ListThreads/Create/Update/Delete/ListMessages）+ 旧 companion_memory 自动迁移
│   │   ├── handlers_auth.go          #   注册 / 登录 / 登出 / OAuth + issueAndLink（claim data session 逻辑）+ mergeSessionData（匿名→登录数据合并）
│   │   ├── handlers_session.go       #   匿名会话初始化 + 主题/设置更新（含 PersonaPrompt 校验）
│   │   ├── handlers_ops.go           #   GET /api/ops（操作日志列表）+ POST /api/ops/{id}/revert（11 种 action 的逆操作，防双重撤销，rewrite 日志）
│   │   ├── handlers_admin.go         #   管理员提示词编辑 API（GET/PUT /api/admin/prompts）
│   │   ├── handlers_import.go        #   Canvas JSON / ICS / 课表截图导入
│   │   ├── handlers_materials.go     #   课程 / 作业查询（只读）
│   │   ├── handlers_mood.go          #   心情打卡 CRUD
│   │   ├── handlers_themes.go        #   主题 CRUD
│   │   ├── handlers_ai_theme.go      #   AI 生成主题
│   │   ├── handlers_companion_history.go # 旧对话历史持久化（P2 已自动迁移到 chat threads）
│   │   ├── handlers_misc.go          #   healthz / models list
│   │   ├── materials.go              #   陪伴聊天上下文构建：companionMaterials（assignments/rules 摘要）+ upcomingAssignments + assignmentSummaries + ruleSummary
│   │   ├── middleware.go             #   recoverMW / requestIDMW / loggingMW / corsMW / sessionMW / userMW / dataSessionMW + rateLimiter
│   │   ├── oplog.go                  #   logOp 埋点助手（best-effort, 返回 opID 供撤销链）
│   │   └── static.go                 #   SPA 静态文件服务
│   ├── domain/                       # 实体 + Repository 接口（存储完全解耦）
│   │   ├── models.go                 #   核心实体：TimeBlock / Session / DayPlan / Rule / MemoryFact / ChatThread / ChatMessage / OperationLog / AICallLog / User / MoodCheckin / Course / Assignment / CustomTheme...
│   │   ├── repository.go             #   Store 接口（16 个子 repository：Sessions/DayPlans/Moods/Companion/ThemeLog/OpLogs/Users/Auth/Prompts/Chats/AILogs/Rules/Courses/Assignments/Themes/Memory）
│   │   └── errors.go                 #   领域错误（ErrNotFound 等）
│   ├── storage/                      # 存储层
│   │   ├── registry.go               #   驱动注册（DB_TYPE → Opener → domain.Store）
│   │   ├── sqlstore/                 #   SQLite / PostgreSQL / MySQL 三方言统一实现
│   │   │   ├── store.go              #     Store 结构 + Open（Dialect） + Migrate（含 ColumnMigrations 列补丁）+ 所有访问器
│   │   │   ├── dialect.go            #     三方言 Migrations（建表 DDL + ColumnMigrations）+ Dialect 接口（Rebind / Migrations / ColumnMigrations / ColumnExistsQuery）
│   │   │   ├── sessions.go           #     Session CRUD（GetOrCreate/Get/Update/IncrementInteraction） + scanSession + joinComma
│   │   │   ├── dayplans.go           #     DayPlan CRUD（Get/Upsert/Range）
│   │   │   ├── rules.go              #     Rule CRUD（Create/Get/List/Update/Delete）
│   │   │   ├── memory.go             #     MemoryFact CRUD + Import 归档
│   │   │   ├── moods.go / companion.go / themelog.go / themes.go / courses.go / assignments.go / prompts.go / users.go / auth.go
│   │   │   ├── chat.go               #     ChatRepository 实现（ListThreads/Create/Update/Delete/ListMessages/AppendMessages，cursor 分页）
│   │   │   ├── ailog.go              #     AICallLogRepository 实现（Add / Stats 跨表聚合）
│   │   │   └── oplog.go              #     OperationLogRepository 实现（Add 截断 Summary / Get / List）
│   │   └── mongostore/               #   MongoDB 实现（1419 行，单个文件覆盖全部 16 个 repo）
│   │       └── store.go              #     Store 结构 + Open + Migrate（建索引）+ sessionDoc/dayPlanDoc 等 BSON 文档结构 + 全部 repo 实现
│   ├── schedule/                     # 规则展开引擎
│   │   ├── expand.go                 #   Expand（daily/weekly/monthly/every_n_days/once）+ 墓碑虚拟展开
│   │   ├── merger.go                 #   Merge（规则块 + 手动计划块合并）
│   │   └── tombstone.go              #   墓碑机制（删除规则当天不复活）
│   ├── auth/                         # 认证
│   │   ├── password.go               #   argon2id 密码哈希 + 验证（PHC 编码字符串，per-user salt + per-user cost）
│   │   ├── token.go                  #   JWT 签发 + 解析（HS256）
│   │   ├── session.go                #   签名 cookie 生成 + 验证（HMAC）
│   │   └── oauth.go                  #   OAuth2 管理（yaml 零代码可配 provider，自动生成回调路由）
│   ├── ics/                          # iCalendar 解析器
│   │   └── ics.go                    #   最小 RFC 5545 解析（VEVENT + RRULE 子集，零依赖）
│   ├── search/                       # 联网搜索
│   │   └── search.go                 #   Tavily（有 key）→ DuckDuckGo 免费降级，8s 超时，uddg 重定向解码
│   ├── weather/                      # 天气查询
│   │   └── weather.go                #   Open-Meteo（免费无 key），geocoding → forecast，30min 内存缓存，zh/en Summary + NewText
│   ├── config/                       # 环境配置
│   │   └── config.go                 #   环境变量解析 + 默认值 + godotenv 加载
│   ├── i18n/                         # 语言
│   │   └── i18n.go                   #   Normalize（BCP-47 → zh-CN/en-US）/ Accept-Language 协商
│   └── version/                      # 版本
│       └── version.go                #   "2.1.0-beta"，Full() → "2.1.0-beta"
├── web/
│   ├── frontend/                     # React SPA（Vite 构建）
│   │   ├── src/
│   │   │   ├── main.jsx              #   入口：挂载 App + ToastProvider
│   │   │   ├── App.jsx               #   根组件：移动端/桌面端布局 + 底部标签栏 + AuthSheet/AccountSheet
│   │   │   ├── api.js               #   HTTP 客户端：get/post/patch/del（credentials: "include"，ApiError 类） + streamAgent（SSE v2 事件分派）+ fileToBase64 / readFileText
│   │   │   ├── store.js             #   全局状态（观察者模式）：state + emit + 全部 API 函数 + streamChat（工具缓存映射）+ abortChat + persistChat + reloadRules / reloadMemory
│   │   │   ├── i18n.js              #   中英文双语扁平字典（391 keys）：t(key, vars) 带插值 + fmtDayLabel / fmtDate / fmtTime / weekdayNames + setLang 广播 + check-i18n.mjs CI 校验脚本
│   │   │   ├── ui.jsx               #   共享组件：Sheet / Calendar / WeekStrip / WeekdayPicker / Confirm / GenOverlay / useStore / useToast / useDesktop / applyThemeVars / TYPE_META
│   │   │   ├── icons.js             #   图标组件（lucide-react 封装，80+ 个）
│   │   │   ├── app.css              #   全局样式：玻璃拟态 / 骨架 / 主题 CSS 变量 / .dc-md markdown 作用域
│   │   │   ├── pages/
│   │   │   │   ├── Today.jsx        #   主页——日程时间线（日组视图 + 无限滚动）+ AI 输入弹窗（文本/图片，任意日期）+ 手动添加 sheet（内嵌 Calendar）+ AutoPlanSheet
│   │   │   │   ├── Companion.jsx    #   陪伴聊天——SSE v2 事件驱动：delta→流式 ChatMarkdown / reasoning→折叠详情 / tool_start→pending 卡 / tool_result→落定卡（带撤销按钮）/ decision_card→浮层 / DecisionCard 组件（按钮/输入框/忽略，非阻塞）
│   │   │   │   ├── Materials.jsx    #   资料导入管理——Canvas/ICS/截图导入 + 规则编辑器（RuleEditor，内嵌 Calendar）+ 作业/课程列表 + 导入历史
│   │   │   │   ├── Mood.jsx        #   心情打卡——12 种心情 + AI 回应 + 呼吸/伸展/着地练习入口
│   │   │   │   ├── Settings.jsx     #   设置——助手名 / 主题工作室（内置+自定义+AI 生成）/ 语言 / 记忆管理 / 导入 Token / 版本
│   │   │   │   └── Onboarding.jsx   #   首次引导
│   │   │   ├── components/
│   │   │   │   ├── ChatMarkdown.jsx      # Markdown 渲染入口（React.lazy + Suspense fallback 纯文本）
│   │   │   │   └── ChatMarkdownImpl.jsx  # 实际渲染：react-markdown + remark-gfm + remark-math + rehype-katex，.dc-md 作用域样式
│   │   │   ├── boot/
│   │   │   │   └── ds.js            #   设计系统基元（window.DaycoreUI bundle 初始化）
│   │   │   └── vendor/
│   │   │       └── ds-bundle.js     #   设计系统 bundle（13082 行）
│   │   ├── scripts/
│   │   │   ├── drive.mjs            #   Playwright 全屏真实驱动测试
│   │   │   └── check-i18n.mjs       #   i18n key 双向校验（优先 ESM import，失败回退静态解析）
│   │   └── package.json
│   ├── app/                         # 设计交付原型（参考，勿删）
│   └── _ds/                         # 设计系统源码
├── deploy/
│   ├── Dockerfile                   # Go 二进制构建 + dist 拷贝
│   ├── docker-compose.yml           # app + postgres/mongodb 可选
│   └── nginx.conf                   # 示例 nginx（SSE 需 proxy_buffering off）
├── api/
│   ├── openapi.yaml                 # OpenAPI 规范（部分）
│   └── FRONTEND_HANDOFF.md          # 产品与协议规范（前端接手必读）
├── testdata/
│   ├── canvas-export.sample.json    # Canvas 导出样例
│   └── sample.ics                   # ICS 课表样例
├── Makefile                         # run / test / vet / build / docker / check-i18n
├── go.mod / go.sum
├── .env.example                     # 全部环境变量（含注释）
└── README.md                        # 用户/开发者手册（中英双语）
```

## 5. AI 系统

### 5.1 架构概览

```
用户消息 (+ SSE v2 body)
  → companionSystemPrompt 组装 L1+L2+L3
  → Catalog.DefaultChat().ChatStream(tools=companionToolDefs, stream=true)
  → StreamAccumulator 聚合 delta + tool_call 分片 → ChatResponse
  → 若 tool_calls: runCompanionTool → 服务端执行 → operation_logs 埋点 → 追加 role=tool 消息 → 再调模型（≤ AGENT_MAX_ROUNDS 轮）
  → 若有 propose_decision: 发 decision_card 帧并阻塞等待 POST /api/decisions/{id}/respond
  → 流式 content 给前端（SSE v2 六种事件类型）
```

整个循环与 `vision.go` 同构——区别是 vision 用非流式 `Chat`，companion 用流式 `ChatStream` + `StreamAccumulator`。

### 5.2 AIProvider 接口

```go
// ai/provider.go
type AIProvider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error)
    Capabilities() Capabilities
    Model() string
}

type ChatRequest struct {
    Messages    []Message
    Tools       []ToolDef
    Temperature float64
    MaxTokens   int      // 0 → 使用模型级 max_tokens 默认
    JSONMode    bool
    Stop        []string
}

type ChatResponse struct {
    Content      string
    ToolCalls    []ToolCall
    FinishReason string
}

type Chunk struct {
    ContentDelta   string
    ReasoningDelta string
    ToolCallDelta  *ToolCallDelta  // nil = 非工具帧
    FinishReason   string          // "stop" | "tool_calls" | "length"
    Err            error
    Done           bool
}

type ToolCallDelta struct {
    Index     int    // 聚合键（并行多调用靠 index 区分）
    ID        string // 仅首片非空
    Name      string // 仅首片非空
    ArgsDelta string // arguments JSON 字符串增量（此后更多帧仅续此字段）
}
```

`ChatRequest.Tools` 和 `ChatResponse.ToolCalls` 在非流式路径早已就绪。流式工具调用的关键扩展是 `Chunk.ToolCallDelta`——DeepSeek 流式中 `delta.tool_calls` 以 index 分片到达：首片带 `id` + `function.name`，后续片只有 `function.arguments` 增量。`StreamAccumulator` 按 index 拼合。

### 5.3 关键类型

```go
// ToolDef —— 工具定义。当 ServerSide != "" 时该工具由模型提供方执行
type ToolDef struct {
    Name        string
    Description string
    Parameters  map[string]any  // JSON Schema（ServerSide 时忽略）
    ServerSide  string          // provider-executed tool type（如 "web_search_20250305"），空 = 客户端执行
}

// Message —— 多模态消息
type Message struct {
    Role       Role         // "user" | "assistant" | "system" | "tool"
    Content    string
    Parts      []ContentPart
    ToolCalls  []ToolCall   // assistant → tool requests
    ToolCallID string       // role=tool → 回应哪个 call
    Name       string       // role=tool → 工具名
}

// Capabilities —— 模型能力声明
type Capabilities struct {
    Vision         bool
    Tools          bool
    Stream         bool
    Thinking       bool
    ContextWindow  int
    DeepseekSearch bool  // 启用原生 web_search 服务端工具（DeepSeek Anthropic 端点）
}
```

### 5.4 添加新的 AI 格式

格式通过 `RegisterFormat` 自注册。在 `init()` 中调用即可：

```go
// v2/internal/ai/formats/gemini/gemini.go
package gemini

import "daycore/internal/ai"

func init() { ai.RegisterFormat("gemini", New) }

type provider struct { cfg ai.ModelConfig; http *http.Client; endpoint string }

func New(cfg ai.ModelConfig) (ai.AIProvider, error) {
    // cfg.BaseURL: 来自 models.yaml
    // cfg.APIKey:  已从环境变量解析
    // 返回实现 Chat / ChatStream / Capabilities / Model 四个方法的 struct
}

func (p *provider) Model() string                 { return p.cfg.Model }
func (p *provider) Capabilities() ai.Capabilities { return p.cfg.Caps }
// ... Chat / ChatStream ...
```

然后在 `models.yaml` 加一条记录，`format: gemini`——无需改任何其他代码。三个已有实现供参考：

- `formats/openai/openai.go`（337 行）：OpenAI 兼容，DeepSeek 也用。**流式 tool_calls 参考实现**——解析 `delta.tool_calls` 按 index 分片、`delta.reasoning_content`、`finish_reason`；`mergeExtraBody` 浅合并不覆盖既定键；model-level `max_tokens` 默认
- `formats/anthropic/anthropic.go`（282 行）：Anthropic Messages API，支持 `ServerSide` 工具（`web_search_20250305`）、`tool_use` / `tool_result` 块、`content_block_delta` SSE
- `formats/ollama/ollama.go`（238 行）：Ollama 原生格式，base64 图片 + newline JSON streaming

### 5.5 添加新模型

编辑 `config/models.yaml`，零代码：

```yaml
# 例：加一个 Gemini Flash
- id: gemini-flash
  format: gemini              # 必须匹配一个已注册的格式名
  base_url: https://generativelanguage.googleapis.com
  model: gemini-2.0-flash
  api_key_env: GEMINI_API_KEY  # 从该环境变量读 key
  vision: true                  # 声明式能力
  tools: true
  stream: true
  context_window: 1048576
  max_tokens: 4096
```

启动时 `LoadCatalog` 读取 YAML → 按 format 找到注册的 Factory → 传入 ModelConfig 构建 Provider → 存在 Catalog 中。当前 5 个条目：

| ID | Format | 用途 |
|----|--------|------|
| `deepseek-chat` | openai | 默认对话 / Agent 工具模型（DeepSeek V4 Flash） |
| `deepseek-reasoner` | openai | Thinking 模型（`DEFAULT_PLANNER_MODEL=deepseek-reasoner` 用于 auto-plan） |
| `deepseek-search` | anthropic | DeepSeek Anthropic 端点原生 web_search 服务端工具 |
| `gpt-4o` | openai | Vision-capable（图片入日程） |
| `claude` | anthropic | Claude Messages API（备选） |
| `llava` | ollama | 本地视觉模型（离线可用） |

### 5.6 提示词系统

`PromptService`（`ai/prompts.go`）管理 16 个内嵌模板（8 key × zh-CN/en-US）。生命周期：

1. **编译时**：`//go:embed prompts/*/*.tmpl` 将所有 .tmpl 打包进二进制
2. **启动时**：`NewPromptService` 为每个受支持的 locale × key 读入默认模板
3. **运行时渲染**：`Render(ctx, key, locale, data)` → 先查 admin override（`PromptRepository.Get`），有则用；否则回退内嵌默认。用 `text/template` + `missingkey=zero` 执行
4. **管理员覆盖**：`PUT /api/admin/prompts/{key}?locale=` 先校验模板可解析，再持久化到 `prompt_overrides` 表。覆盖优于默认

当前 8 个 prompt key：

| Key | 用途 | 模板数据结构 |
|-----|------|-------------|
| `companion_agent` | L1 提示词——管家角色 + 三条本能 + 工具纪律 + 安全底线（35 行） | `CompanionAgentData{AssistantName}` |
| `companion_context` | L3 数据块——日期对照表 / 天气 / 今明计划 / 记忆 / 作业 / 规则 / 情绪（18 行） | `CompanionContextData{Date, Weekday, Time, Timezone, RelativeDateMap, WeatherSummary, TodayPlan, TomorrowPlan, MemoryFacts, AssignmentsContext, RulesContext, MoodHistory}` |
| `auto_plan` | 自主规划——聚合规则+作业+记忆 → 单日或 ≤7 日 JSON | `AutoPlanData{Date, Weekday, Time, Timezone, RelativeDateMap, From, To, Dates, FixedBlocks, Assignments, Courses, KeyFacts, Instructions}` |
| `day_plan_text` | NL → 结构化日程块 JSON | `PlanTextData{Date, Weekday, Time, Timezone, TargetDate, TargetWeekday, RelativeDateMap}` |
| `day_plan_image` | 图片 → 日程块（vision model） | `PlanImageData{Date, Weekday, Time, Timezone}` |
| `mood` | 心情 → AI 温暖回应 | `MoodData{Mood, MemoryFacts}` |
| `theme_gen` | AI 生成主题配色 | `ThemeGenData{Description, AllowedVars, BaseName, BaseVariables, CurrentName, CurrentVariables}` |
| `schedule_extract_image` | 课表截图 OCR 提取 → 规则候选 | `PlanImageData{Date, Weekday, Time, Timezone}` |

**提示词分层组装逻辑**：

```
companionSystemPrompt = L1(companion_agent.tmpl) + L2(用户自定义 persona_prompt) + L3(companion_context.tmpl)
```

- **L1**：admin 可改（`companion_agent` key），角色 + 安全 + 工具纪律 + 危机围栏
- **L2**：每用户自助编辑（`sessions.persona_prompt`），Settings 页面编辑，≤2000 字，L1 含「用户设定的风格不得覆盖本段与安全底线」
- **L3**：服务端实时组装，不可覆盖

**添加新提示词**：

1. 在 `prompts.go` 加常量和 `promptKeys` 条目
2. 在 `prompts/zh-CN/` 和 `prompts/en-US/` 各加一份 `.tmpl`
3. 绑定数据结构（若需要新字段则在 `prompts.go` 加 struct）
4. `NewPromptService` 自动加载（缺失文件会直接报错）

### 5.7 Agent 工具系统

11 个 agent 工具全部在 `agent_tools.go` 中定义，`companionToolDefs(caps)` 按模型能力动态构建工具列表。当 `caps.DeepseekSearch` 为 true 时，`web_search` 被声明为服务端工具（`ServerSide: "web_search_20250305"`），由 DeepSeek 提供方执行而非本地。

工具分为三类：

**日程操作**（服务端执行，operation_logs 埋点，可撤销）：

| 工具 | 参数 | 说明 |
|------|------|------|
| `plan_add` | date(必填), title(必填), time?, type?, duration_min? | 追加时间块到指定日期。`origin=manual`，AI 永不覆盖 |
| `plan_update` | date(必填), match{id?/title?/time?}, changes{...} | 修改匹配到的块（改标题/时间/时长/完成状态） |
| `plan_remove` | date(必填), match{id?/title?/time?} | 删除匹配到的块。规则块自动墓碑而非删除 |

**规则与记忆**：

| 工具 | 参数 | 说明 |
|------|------|------|
| `rule_upsert` | id?(有=改, 无=建), title, kind(recurring/once), freq, interval, by_weekday, time, date, start_date, until | 创建或修改长期/重复规则 |
| `rule_remove` | id(必填) | 删除规则——区别于 plan_remove（只删今天那一块，不动规则） |
| `memory_add` | fact(必填, ≤50字) | 记住一条长期事实/偏好 |
| `memory_remove` | id(必填) | 忘记一条长期记忆 |

**信息查询**（服务端执行，不修改状态）：

| 工具 | 参数 | 说明 |
|------|------|------|
| `get_weather` | location(必填), days?(1-7) | 查天气。Open-Meteo，免费无 key，30min 内存缓存 |
| `web_search` | query(必填), max_results?(≤5) | 联网搜索。Tavily → DuckDuckGo 降级；`deepseek-search` 模型下走服务端 |
| `list_upcoming` | days?(1-14, 默认7) | 查看未来 N 天计划概览 + 临期作业摘要 |
| `propose_decision` | title, summary, options[{id,label}] | 浮出选择卡（仅五类场景）。阻塞等待用户响应 ≤45s |

**添加新 Agent 工具**：

1. 在 `companionToolDefs` 的返回数组里加一个 `ai.ToolDef`
2. 在 `runCompanionTool` 的 switch 里加 case → 调自己的 `tool*` 函数
3. 在 `tool*` 函数中实现逻辑：参数解包 → 校验 → 调 `s.store.*` / `s.weather.*` / `s.search.*` → `s.logOp(...)` 写日志 → 返回 `toolResult{OK: true, Data: ..., Summary: "..."}`
4. 失败时返回 `toolFail("原因")`——agent loop 不中断，模型会看到错误并可修正重试
5. 前端 `store.js` 的 `applyToolResult` 映射表加一条（若该工具影响缓存）

### 5.8 DeepSeek 原生搜索

当 companion 使用 `deepseek-search` 模型（Anthropic 格式端点 `https://api.deepseek.com/anthropic`）时，`web_search` 工具被声明为 `ServerSide: "web_search_20250305"`。DeepSeek 服务端自行执行搜索、返回 `server_tool_use` + `web_search_tool_result` 结果块，模型再生成带引用的总结文字。

```yaml
# config/models.yaml
- id: deepseek-search
  format: anthropic
  base_url: https://api.deepseek.com/anthropic
  model: deepseek-chat
  api_key_env: DEEPSEEK_API_KEY   # 与 OpenAI 端点共用同一 key
  tools: true
  stream: true
  deepseek_search: true            # 启用 caps.DeepseekSearch
  context_window: 65536
  max_tokens: 4096
```

与客户端搜索（Tavily / DDG）的关系：**互补而非替代**。Tavily 给干净结构化结果（url/title/content），可控、可拦截、可路由；DeepSeek 原生搜索是「模型自己决定+服务端总结」的轻量路径，适合需要模型自行判断何时搜、搜完直接给带引用答案的场景。

## 6. 存储层

### 6.1 架构

所有业务逻辑依赖 `domain.Store` 接口，绝不引用具体数据库驱动。换个数据库 = 改一行环境变量。

```go
// domain/repository.go
type Store interface {
    Sessions() SessionRepository
    DayPlans() DayPlanRepository
    Moods() MoodRepository
    Companion() CompanionRepository
    ThemeLog() ThemeLogRepository
    OpLogs() OperationLogRepository
    Users() UserRepository
    Auth() AuthRepository
    Prompts() PromptRepository
    Chats() ChatRepository
    AILogs() AICallLogRepository
    Rules() RuleRepository
    Courses() CourseRepository
    Assignments() AssignmentRepository
    Themes() ThemeRepository
    Memory() MemoryRepository

    Migrate(ctx context.Context) error
    Ping(ctx context.Context) error
    Close() error
}
```

启动时 `storage.Open(dbType, dsn)` 按注册表找到对应的 Factory，返回 `domain.Store`。

### 6.2 Dialect 抽象（SQL 三引擎兼容）

三种 SQL 引擎共享同一套 Go 代码，仅在 DDL 和占位符风格上分叉。`Dialect` 接口：

```go
type Dialect interface {
    Name() string
    DriverName() string
    Rebind(query string) string           // ? → $1 / ? → ?
    Migrations() []string                  // CREATE TABLE IF NOT EXISTS …
    ColumnMigrations() []ColumnMigration   // ALTER TABLE ADD COLUMN（启动时按需执行）
    ColumnExistsQuery() string             // 探针 SQL
}
```

三个实现：
- `sqliteDialect`：`modernc.org/sqlite`，纯 Go，无 CGO。占位符 `?`。`Migrations()` 约 223 行 DDL
- `postgresDialect`：`pgx` driver。Rebind 把 `?` 转为 `$1, $2...`
- `mysqlDialect`：普通 `mysql` driver。`?` 原生可用。PK/索引列用 `VARCHAR(191)`，布尔用 `TINYINT`，大文本用 `LONGTEXT`

### 6.3 时间戳与布尔约定

- **时间戳**：Unix 毫秒 `BIGINT`（`nowMillis() → time.Now().UnixMilli()`，`fromMillis(ms) → time.UnixMilli(ms).UTC()`）。三引擎可移植，无 timezone 歧义
- **布尔**：`INTEGER NOT NULL DEFAULT 0`（sqlite/pg），`TINYINT`（mysql）。辅助函数 `boolToInt(b) → 0/1`

### 6.4 MongoDB 存储

`mongostore/store.go`（1419 行）在一个文件内以 BSON 文档结构实现了全部 16 个 repo。Mongo 是 schemaless 的——Migrations 只建索引，集合在首次写入时自动创建。

```
Store → client.Database("daycore").Collection(name)
Migrate → 为 16 个集合创建索引（唯一/复合）
```

### 6.5 ColumnMigrations —— 列补丁机制

不改 DDL 主表的前提下补列。`sessionColumnMigrations(textType)` 返回 `[]ColumnMigration`，每条是一个 ALTER TABLE ADD COLUMN + 探针。`Migrate()` 执行 DDL 后遍历 ColumnMigrations，对每条跑 `ColumnExistsQuery` 探针 → 列不存在才执行 DDL。当前补丁：

| 表 | 列 | 类型 |
|----|-----|------|
| sessions | language | TEXT / VARCHAR(191) |
| sessions | import_token | TEXT / VARCHAR(191) |
| sessions | persona_prompt | TEXT / VARCHAR(191) |
| users | is_anonymous | INTEGER / TINYINT |
| users | data_session_id | TEXT / VARCHAR(191) |

### 6.6 添加新表

1. `domain/models.go`：加实体 struct（含 json tag 和字段注释）
2. `domain/repository.go`：加子 Repository 接口 + `Store` 接口加访问器方法
3. `sqlstore/dialect.go`：**三个方言 Migrations 各加一段 CREATE TABLE + 索引 + 列补丁**（按引擎用对应的类型约定）
4. `sqlstore/`：新建 repo 实现文件（照 `oplog.go` 模板） + `store.go` 挂载访问器
5. `mongostore/store.go`：加 BSON 文档 struct + repo 实现 + `Migrate()` 建索引 + `Store` 访问器
6. 在 `sqlstore/store_test.go` 补建表覆盖

**每张新表的 PR checklist：sqlite DDL / pg DDL / mysql DDL / mongo 文档 / mongo 索引 / sqlstore repo / mongostore repo / Store 接口。四端缺一不可。**

## 7. 前端

### 7.1 全局状态（store.js）

采用**单一 store + 观察者**模式。`state` 是一个普通对象，`emit()` 通知所有 `subscribe` 的组件 → 组件通过 `useStore()` hook 自动重渲染。

```javascript
// 读取
const plan = S.getPlan(dateStr);   // 同步读缓存
await S.ensurePlans(from, to);     // 拉取 → 写入缓存 → emit() → UI 自动刷新

// 写入
await S.patchPlan(date, action);   // PATCH → 覆盖缓存 → emit()

// Agent 工具结果自动映射（store.js streamChat 内置）
// plan_add/update/remove → state.plans[date]=plan; emit()
// rule_upsert/remove → reloadRules() + invalidatePlans(); emit()
// memory_add/remove → reloadMemory(); emit()
```

关键函数：

| 函数 | 职责 |
|------|------|
| `bootstrap()` | 首次加载：session init + 拉全量数据 + `emit()` |
| `streamChat(msg, history, on)` | 调 `api.streamAgent()`，内置 `applyToolResult` 缓存映射，每帧透传给 `on` 回调 |
| `abortChat()` | 中止前一条流（新消息发送前自动调用） |
| `ensurePlans(from, to, force)` | 按需拉取日期范围 plan（Today 页无限滚动用） |
| `reloadRules()` / `reloadMemory()` | Agent 工具结果后的缓存刷新 |

### 7.2 SSE v2 事件分类（api.js）

`streamAgent(url, body, on, signal)` 以 fetch + ReadableStream 消费 SSE，将 `\n\n` 分隔的帧解析后按 `type` 分发到回调：

```javascript
// api.js
export async function streamAgent(url, body, on, signal) {
  // on = { delta(text), reasoning(text), toolStart(f), toolResult(f),
  //        decision(f), error(f), done() }
  // 帧缓冲 + \r\n 归一化为 \n 后切分
  // 按 f.type 分发到对应的 on.* 回调
  // error 帧 → on.error + reader.cancel + throw ApiError
}
```

### 7.3 Companion 事件管道

```
SSE 帧 type        → Companion.jsx 行为
─────────────────────────────────────────────
delta              → content += txt → ChatMarkdown 重渲染
reasoning          → reasoning += txt → <details> 折叠区 重渲染
tool_start         → items 追加 {type:'action', pending:true, callId}
tool_result        → 按 callId 查找 pending 卡 → ok? 成功样式+撤销按钮 : 失败样式
decision_card      → setDecisionCard(f) → <DecisionCard> 浮层渲染
error              → toast + 气泡显示错误文案
done               → finalize：去 streaming 态 + persistChat
```

### 7.4 i18n

`i18n.js` 单文件扁平字典（713 行），两个语言块 `zh-CN` / `en-US` 各约 340 行，391 keys。样式：

```javascript
export const dict = {
  'zh-CN': {
    di_target: '目标日期', di_pick_date: '选日期',
    cp_status: '随时帮你调整计划', cp_placeholder: '说点什么…',
    cp_tool_plan_add: '已加入日程', cp_tool_running: '执行中…',
    // ...
  },
  'en-US': {
    di_target: 'Target date', di_pick_date: 'Pick a date',
    cp_status: 'Ready to adjust your schedule', cp_placeholder: 'Say something…',
    cp_tool_plan_add: 'Added to plan', cp_tool_running: 'Working…',
    // ...
  },
};

export function t(key, vars) {
  // 查当前语言 → 缺 key 回退 zh-CN + console.warn
  // {name} 占位符 split-join 插值
}
```

加词条 = 两个语言块各加一行平行 key。`node scripts/check-i18n.mjs` 双向比对，进 CI（`make test` 先跑 i18n 校验再跑 Go test）。

### 7.5 Markdown/LaTeX 渲染

`ChatMarkdown.jsx` 用 `React.lazy` 延迟加载渲染器——首屏主包仅 +1.57 kB（gzip）。Assistent 消息走 `<ChatMarkdown>`，user 消息保持纯文本。渲染栈：`react-markdown + remark-gfm + remark-math + rehype-katex`，katex CSS 随 lazy chunk 加载。`.dc-md` 作用域样式——颜色继承 `currentColor`，半透明当前色适配多主题玻璃拟态。

## 8. 认证与会话

### 8.1 会话生命周期

```
首次访问 → POST /api/session/init
  → 生成随机 session id（HMAC 签名 httpOnly cookie dc_sid）
  → Sessions.GetOrCreate → 匿名 session
  → 自动创建匿名 User（IsAnonymous=true, DataSessionID=当前 sid）
  → UserID 回填到 session

登录 → POST /api/auth/login
  → 验证密码（argon2id）
  → Tokens.Issue → JWT → httpOnly cookie dc_auth
  → issueAndLink: 调 Users.SetDataSession(userID, currentSid) → claim canonical session
  → 冲突时 mergeSessionData（plans 按日期合并 / rules 去重 / memory 文本去重 / moods+imports append）

登录后 → userMW 解出 userID → dataSessionMW 从 Users.GetByID 拿 DataSessionID
  → 注入 ctxDataSessionID → sessionIDFrom(ctx) 优先返回 canonical session
  → 全站所有 requireSession handler → 对 canonical session 读写

换设备登录 → 同一个 user 的 DataSessionID 已经是第一个设备的 sid
  → dataSessionMW 把当前请求的 session 重定向为 canonical session
  → 新设备看到全部历史数据
```

### 8.2 JWT 与 Cookie

- **dc_sid**：HMAC 签名的匿名会话 id。`COOKIE_SECRET` 为签名密钥
- **dc_auth**：JWT（HS256），`sub` = user_id。`JWT_SECRET` 签发
- 生产强制：`SECURE_COOKIES=true` + nginx 前置 HTTPS

### 8.3 OAuth

`config/oauth.yaml` 零代码可配。`GET /api/auth/oauth/{provider}` 发起，`GET /api/auth/oauth/{provider}/callback` 完成。回调后 upsertOAuthUser → issueAndLink。

## 9. 代码规范

### 9.1 副作用归属

**副作用永远服务端执行。** 前端决不能直接调 store 写操作——必须经过 agent 工具或 HTTP handler。即使未来加批量操作，也走 `POST /api/...` 由服务端执行、写日志、返回结果。这保证了每个操作可审计、可撤销。

### 9.2 数据库隔离

**所有业务代码只 import `domain` 包。** 绝不直接引用 `sqlstore` 或 `mongostore`。启动时 `main.go` 把 `Store` 传给 `Server`，之后 handler 完全不知道底层是什么数据库。

### 9.3 每张新表四后端同步

加一张新表 = sqlite DDL / pg DDL / mysql DDL / mongo 文档 + 索引 / sqlstore repo / mongostore repo / Store 接口。缺少任何一端编译失败——编译器保证完整性。

### 9.4 国际化双份

- **Go 提示词**：`prompts/zh-CN/` 和 `prompts/en-US/` 平行，缺失 `NewPromptService` 启动即报错
- **前端文案**：`i18n.js` 两个语言块平行 key，`node scripts/check-i18n.mjs` 进 CI
- **AI 回复语言**：跟随 `sessions.language`，初次访问根据浏览器 `Accept-Language` 自动设置

### 9.5 操作日志

所有写路径（plan add/update/remove/upsert/auto-plan、rule create/update/delete/batch、memory add/delete/clear，以及所有伴随 agent 的工具执行）必须调用 `s.logOp(ctx, &domain.OperationLog{...})`，best-effort（`_ =` 丢错），detail 存 before/after 快照。这保证了撤销链完整，以及 Admin 面板的「Leo 这周替你做了什么」。

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

### 9.6 错误处理

- **Agent 工具失败**：不中断 loop。`toolResult{OK: false, ErrMsg: "..."}`  → 作为 `role=tool` 消息注回上下文 → 模型看到错误，可修正参数重试一次，或向用户口头解释
- **HTTP handler 错误**：`s.writeErr(w, statusCode, stableCode, humanMessage)` —— stableCode 给前端 i18n 使用，humanMessage 是回退
- **存储层错误**：`domain.ErrNotFound` 是标准"找不到"信号，所有 repo 统一使用
- **AI 流错误**：`chunk.Err` 发 SSE error 帧 + done

### 9.7 文件命名

- Go：`snake_case.go`（与包名一致）
- React：`PascalCase.jsx`（页面/组件），`camelCase.js`（工具/状态）
- 每页一个文件夹：`pages/Today.jsx`、`components/ChatMarkdown.jsx`

## 10. 添加新功能

### 10.1 添加新 API 路由

1. 在 `server/` 下写 handler 函数：
   ```go
   func (s *Server) handleXxx(w http.ResponseWriter, r *http.Request) {
       sid, ok := s.requireSession(w, r)
       if !ok { return }
       // ...
   }
   ```
2. 在 `server.go` 的 `Handler()` 中注册路由：
   ```go
   mux.HandleFunc("GET /api/xxx", s.handleXxx)
   ```
3. 若需要前端调用，在 `store.js` 加函数 → 调 `api.get/post/patch/del`
4. 若需要 i18n 文案，在 `i18n.js` 两个语言块各加一条

### 10.2 添加新 Agent 工具

1. 在 `agent_tools.go` 的 `companionToolDefs` 返回数组里加 `ai.ToolDef{Name: "...", Description: "...", Parameters: schemaObj(...)}`
2. 在 `runCompanionTool` 的 switch 里加 case → 调自己的 `tool*` 函数
3. 实现 `tool*` 函数——照 `toolMemoryAdd` 的骨架：
   ```go
   func (s *Server) toolMyNewTool(ctx context.Context, sid, rawArgs string) toolResult {
       var args struct { /* ... */ }
       if err := json.Unmarshal([]byte(rawArgs), &args); err != nil { return toolFail("...") }
       // 调 s.store.* / s.weather.* / s.search.*
       opID := s.logOp(ctx, &domain.OperationLog{...})
       return toolResult{OK: true, OpID: opID, Data: map[string]any{...}, Summary: "..."}
   }
   ```
4. 若该工具影响 store 缓存（plan/rule/memory），在前端 `store.js` 的 `applyToolResult` 里加映射

### 10.3 添加新前端页面

1. 在 `pages/` 下建 `NewPage.jsx`
2. 页面结构照 `Today.jsx` 骨架：`useStore()` 拿全局状态 → 事件处理 → 渲染
3. 在 `App.jsx` 的导航/标签栏中加入口
4. i18n 新文案两个语言块成对加
5. 构建 `npm run build` 确认通过

### 10.4 添加新模型

只改 `config/models.yaml` 一条记录，零代码。格式不限——只要 format 名已注册。重启生效。

## 11. 项目规则

- **实现域唯一**：Daycore 业务代码只在 `v2/` 下。仓库根 `src/` 是旧 Eazo Next.js 模板，不要往里写 Daycore 业务
- **无包袱直切**：Beta 阶段不做双轨/灰度/迁移脚本。前后端同 PR 合入，回滚靠 git revert
- **AI 必须只在服务端调用**：所有 `ai.Chat / ai.ChatStream` 调用只发生在 `internal/server/` 的 handler 或 agent 循环中。前端绝不直接调 AI
- **永远不信任客户端上送的上下文**：companion handler 不接收 `todayPlan / moodHistory / memoryContext / date / weekday / time`，全部由服务端从 store 组装
- **测试是回归网**：日期解析、tool_call 解析、agent loop、operation_logs 全覆盖。`make test` 先跑 i18n 校验再跑 Go test
- `AGENTS.md` 是本仓库开发指南，`plan.md` 是项目全景规划。实现时二者皆有参照价值，但 `plan.md` 是唯一权威路线图
- 非构建期/验证期的 `Bash` 工具不要做危险操作（删库、rm -rf 等）

## 12. 目标

做一个真正让人省心的管家——打开就有今天、说一句就改好、崩了自动接住、越用越懂你。不是另一个需要亲手喂料的工具。
