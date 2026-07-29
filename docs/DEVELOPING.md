# 开发者手册

> 从 README 搬来（2026-07-29）—— README 只留「这是什么 / 怎么跑起来」，其余瀑布式下沉到 `docs/`。

### 项目结构

```
cmd/daycore/main.go         装配：注册驱动/格式 → 打开 Store → 迁移 → 起 HTTP（优雅关闭）
internal/
  domain/                   实体 + 通用 Store/Repository 接口（界面分离核心）
  config/                   环境配置（HOST/PORT/STATIC_DIR/…）
  version/                  版本号唯一来源（构建版本 + API 契约版本，别混）
  auth/                     argon2id / JWT / OAuth / 会话 cookie
  i18n/                     locale 协商 + 三层消息目录（DB → LOCALES_DIR → 内嵌）+ 用户级一主一副 Pair
  ai/                       AIProvider 接口 + catalog + 视觉编排 + 提示词
    formats/{openai,anthropic,ollama}/   wire-format（自注册）
    prompts/<locale>/*.tmpl 提示词（11 key × 2 locale；PROMPTS_DIR 可逐文件覆盖）
  schedule/                 重复规则展开引擎（纯函数）
  ics/                      最小 iCalendar + RRULE 子集解析器（零依赖）
  timeutil/                 石化线与墙钟换算（纯函数）
  rhythm/ mood/ rapport/    节律学习 / 心情窗口 / 默契评分 —— 三个都是纯函数、零存储、读时派生
  channels/                 通道插件框架（Registry + OneBot 11 适配器）
  search/                   web 搜索（Tavily→DDG）+ MaterialSearcher（原生 FTS + 子串兜底）
  weather/                  WeatherProvider registry + 四个 provider 子包（自注册）
  storage/{sqlstore,mongostore}/  SQLite+PG+MySQL（Dialect 抽象）/ MongoDB
  storage/storagetest/      行为一致性套件（29 例，两个后端跑同一份）
  server/                   路由（分散注册，见 routes.go）+ 中间件 + handlers + agent loop + cron Worker
api/                        openapi.yaml（生成物）+ spec/（按 tag 分片的源）+ FRONTEND_HANDOFF.md
web/frontend/               React 前端（Vite；npm run dev / build）
deploy/                     Dockerfile / docker-compose / nginx
testdata/                   canvas-export.sample.json / sample.ics
docs/                       实时项目文档（架构/认证/Agent/数据/AI/路由总表）
extension/                  Chrome MV3 插件（抓 Canvas → POST /api/import/canvas）
design-ui/                  设计原型：四套范式级不同的前端 + 共享 mock + 交接文档（只读参考，不参与构建）
docs/specs/                 对外协议：传输层 / 存储适配 / provider 适配 / 前端 manifest
```

### 扩展点（registry / 驱动模式）

1. **新增数据库**：实现 `domain.Store` + `storage.Register("foo", opener)`（SQL 类只需加一个 `Dialect`），main.go 加 blank import。**验收标准是 `internal/storage/storagetest` 的行为套件通过** —— 它测的是四个后端必须一致的行为，不是「能存能取」。不想写 Go 的话，将来可以走 [`docs/specs/storage-protocol.md`](specs/storage-protocol.md) 的 HTTP/子进程协议。
2. **新增 AI 厂商格式**：实现 `ai.AIProvider` + `ai.RegisterFormat("gemini", New)`，`models.yaml` 引用 `format: gemini`。
3. **新增模型（零代码）**：编辑 `config/models.yaml`，重启生效；视觉模型标 `vision: true`。
4. **新增 OAuth provider（零代码）**：`config/oauth.yaml` 加条目，回调 `<PUBLIC_BASE_URL>/api/auth/oauth/<name>/callback`。
5. **编辑提示词（运行时）**：`PUT /api/admin/prompts/{key}?locale=zh-CN|en-US`（`X-Admin-Token`），覆盖存 `prompt_overrides`。

### 开发命令

```bash
make run / test / vet / build / docker
make test-mongo                                     # 行为套件对真机 MongoDB 跑（需本机 mongod）
cd web/frontend && npm run dev / build
node web/frontend/scripts/check-i18n.mjs            # zh-CN / en-US key 对齐校验
```

### 端到端冒烟（curl）

见 `testdata/` 夹具；核心流：`POST /api/session/init` → `POST /api/import/token` → `POST /api/import/canvas`（X-Import-Token）→ `POST /api/import/ics` → `POST /api/rules` → `GET /api/plan?date=`（规则虚拟合并）→ `POST /api/ai/auto-plan` → SSE `/api/ai/companion`（**工具调用**，不是早先的 `<rule_update>` 标签协议；帧格式见 `api/FRONTEND_HANDOFF.md` §B）→ `GET /api/memory`。

### 相对 v1 的改进

- 主模式从「手动输入日程」变为「自主规划」（资料导入 → auto-plan → 聊天微调）。
- 新增重复/长期日程、Canvas/ICS/截图导入、每用户长期记忆、自定义主题 + AI 配色、i18n（含提示词双语）、版本体系与单二进制静态托管部署。
- 修复无鉴权数据端点（服务端签名 httpOnly cookie）、`incrementInteractionCount` 占位符 bug、日程加载 N+1、v1 MoodScreen `exerciseOffered` 异步 bug（前端已按正确方式实现）。

---


### 文档去哪找什么

| 想知道 | 看 |
|---|---|
| 这个仓库现在是什么样、为什么这样 | `docs/ARCHITECTURE.md` `docs/DATA.md` `docs/AUTH.md` `docs/AGENT.md` `docs/AI.md` |
| 有哪些 HTTP 路由 | `docs/API_SURFACE.md` + `api/openapi.yaml`（唯一权威；**改的是 `api/spec/paths/<tag>.yaml` 然后 `make api-bundle`**） |
| 端无关的产品语义（时间三层、提案、注意力阶梯、默契） | `docs/EXPERIENCE_CORE.md` |
| **别人要照着实现什么**（适配器、前端） | `docs/specs/` |
| 前端要对接的 API 细节 | `api/FRONTEND_HANDOFF.md` |

**实时文档铁律**：任何代码改动必须在同一批修改中更新 `docs/` 对应文件。文档与代码不同步视为改动未完成。

