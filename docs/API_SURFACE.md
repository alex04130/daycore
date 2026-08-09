# 路由 → Handler 映射总表

> **下面的表是生成的**，源是各 handler 文件 `init()` 里的 `registerRoutes`。跑 `make api-surface` 重生成；`go test ./internal/server/` 会因它过期而红。
>
> 手写的只有末尾「读表须知」一节 —— 那里放的是表格表达不了的东西（哪个文件名骗人、哪条路由有隐藏语义）。
>
> 三件事由测试兜住，不靠自觉（`internal/server/routes_test.go`）：本表与代码一致、**代码与 `api/openapi.yaml` 双向一致**（服务了没写进契约 / 写进契约没人服务都红）、pattern 不重复。REST 细节（请求/响应 schema）以 `api/openapi.yaml` 为准 —— 但**改的是 `api/spec/paths/<tag>.yaml` 然后 `make api-bundle`**，openapi.yaml 是生成物，手改会被覆盖。
>
> 当前 **118 条路由 / 28 个组**。

<!-- BEGIN GENERATED ROUTES -->
## admin (config)（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/admin/config` | handlers_admin_config.go |
| `PUT /api/admin/config` | handlers_admin_config.go |

## admin (models)（3 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/admin/models` | handlers_admin_models.go |
| `POST /api/admin/models/{id}/test` | handlers_admin_models.go |
| `GET /api/admin/oauth` | handlers_admin_models.go |

## admin (prompts)（3 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/admin/prompts` | handlers_admin.go |
| `GET /api/admin/prompts/{key}` | handlers_admin.go |
| `PUT /api/admin/prompts/{key}` | handlers_admin.go |

## admin (providers)（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/admin/providers` | handlers_admin_providers.go |
| `PUT /api/admin/providers` | handlers_admin_providers.go |

## admin (session)（2 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/admin/session` | handlers_admin_session.go |
| `DELETE /api/admin/session` | handlers_admin_session.go |

## admin (stats, users, DB)（10 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/admin/ailogs` | handlers_admin_stats.go |
| `GET /api/admin/db/backup` | handlers_admin_db.go |
| `GET /api/admin/db/export` | handlers_admin_db.go |
| `POST /api/admin/db/import` | handlers_admin_db.go |
| `GET /api/admin/db/table/{name}` | handlers_admin_db.go |
| `DELETE /api/admin/db/table/{name}/{id}` | handlers_admin_db.go |
| `GET /api/admin/db/tables` | handlers_admin_db.go |
| `GET /api/admin/stats` | handlers_admin_stats.go |
| `GET /api/admin/users` | handlers_admin_stats.go |
| `DELETE /api/admin/users/{id}` | handlers_admin_stats.go |

## ai（10 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/ai/auto-plan` | handlers_autoplan.go |
| `POST /api/ai/companion` | handlers_ai_companion.go |
| `POST /api/ai/companion/async` | handlers_ai_companion_async.go |
| `POST /api/ai/extract-schedule-image` | handlers_ai_plan.go |
| `POST /api/ai/mood` | handlers_ai_mood.go |
| `POST /api/ai/plan-image` | handlers_ai_plan.go |
| `POST /api/ai/plan-text` | handlers_ai_plan.go |
| `POST /api/ai/theme` | handlers_ai_theme.go |
| `POST /api/ai/travel` | handlers_ai_travel.go |
| `POST /api/decisions/{id}/respond` | agent.go |

## auth（7 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/auth/login` | handlers_auth.go |
| `POST /api/auth/logout` | handlers_auth.go |
| `GET /api/auth/oauth/{provider}` | handlers_auth_oauth.go |
| `GET /api/auth/oauth/{provider}/callback` | handlers_auth_oauth.go |
| `GET /api/auth/providers` | handlers_auth_oauth.go |
| `POST /api/auth/register` | handlers_auth.go |
| `GET /api/me` | handlers_auth.go |

## canvas materials（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/assignments` | handlers_materials.go |
| `POST /api/assignments` | handlers_materials.go |
| `PATCH /api/assignments/{id}` | handlers_materials.go |
| `GET /api/courses` | handlers_materials.go |

## channels (user binds an account)（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/channels` | handlers_channels.go |
| `POST /api/channels/{channel}/bind` | handlers_channels.go |
| `DELETE /api/channels/{channel}/unbind` | handlers_channels.go |
| `POST /api/channels/{channel}/verify` | handlers_channels.go |

## chat threads（7 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/chat/messages/{id}` | handlers_chat.go |
| `GET /api/chat/threads` | handlers_chat.go |
| `POST /api/chat/threads` | handlers_chat.go |
| `PATCH /api/chat/threads/{id}` | handlers_chat.go |
| `DELETE /api/chat/threads/{id}` | handlers_chat.go |
| `GET /api/chat/threads/{id}/messages` | handlers_chat.go |
| `DELETE /api/chat/threads/{id}/messages` | handlers_chat.go |

## companion history（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/companion-history` | handlers_companion_history.go |
| `POST /api/companion-history` | handlers_companion_history.go |

## custom themes（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/themes` | handlers_themes.go |
| `POST /api/themes` | handlers_themes.go |
| `PATCH /api/themes/{id}` | handlers_themes.go |
| `DELETE /api/themes/{id}` | handlers_themes.go |

## feedback（1 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/feedback` | handlers_feedback.go |

## files（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/files` | handlers_files.go |
| `POST /api/files` | handlers_files.go |
| `GET /api/files/{id}` | handlers_files.go |
| `DELETE /api/files/{id}` | handlers_files.go |

## health & diagnostics（3 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/healthz` | handlers_misc.go |
| `GET /api/models` | handlers_misc.go |
| `GET /api/version` | handlers_misc.go |

## imports（4 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/import/canvas` | handlers_import_canvas.go |
| `POST /api/import/ics` | handlers_import.go |
| `GET /api/import/token` | handlers_import.go |
| `POST /api/import/token` | handlers_import.go |

## inbox（3 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/inbox/commit` | handlers_inbox.go |
| `POST /api/inbox/process` | handlers_inbox.go |
| `POST /api/inbox/upload` | handlers_inbox.go |

## long-term memory（5 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/import/history` | handlers_memory.go |
| `GET /api/memory` | handlers_memory.go |
| `POST /api/memory` | handlers_memory.go |
| `DELETE /api/memory` | handlers_memory.go |
| `DELETE /api/memory/{id}` | handlers_memory.go |

## materials（7 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/materials` | handlers_materials_full.go |
| `POST /api/materials` | handlers_materials_full.go |
| `GET /api/materials/categories` | handlers_materials_full.go |
| `GET /api/materials/search` | handlers_materials_full.go |
| `GET /api/materials/{id}` | handlers_materials_full.go |
| `PATCH /api/materials/{id}` | handlers_materials_full.go |
| `DELETE /api/materials/{id}` | handlers_materials_full.go |

## moods（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/mood` | handlers_mood.go |
| `POST /api/mood` | handlers_mood.go |
| `PATCH /api/mood` | handlers_mood.go |
| `GET /api/mood/kinds` | handlers_mood.go |

## operation logs & undo（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/ops` | handlers_ops.go |
| `POST /api/ops/{id}/revert` | handlers_ops.go |

## plans（5 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/plan` | handlers_plan.go |
| `POST /api/plan` | handlers_plan.go |
| `PATCH /api/plan` | handlers_plan.go |
| `POST /api/plan/lock` | handlers_plan.go |
| `GET /api/plan/range` | handlers_plan.go |

## proposals（3 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/plan/conflict` | proposals.go |
| `GET /api/proposals` | proposals.go |
| `POST /api/proposals/{id}/respond` | proposals.go |

## schedule rules（5 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/rules` | handlers_rules.go |
| `POST /api/rules` | handlers_rules.go |
| `POST /api/rules/batch` | handlers_rules.go |
| `PATCH /api/rules/{id}` | handlers_rules.go |
| `DELETE /api/rules/{id}` | handlers_rules.go |

## session（5 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/session/init` | handlers_session.go |
| `GET /api/session/preferences` | handlers_preferences.go |
| `PATCH /api/session/preferences` | handlers_preferences.go |
| `PATCH /api/session/settings` | handlers_session.go |
| `POST /api/session/theme` | handlers_session.go |

## temp-context (session-scoped key-value store)（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/temp-context` | handlers_tempcontext.go |
| `PUT /api/temp-context` | handlers_tempcontext.go |

## wishes（5 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/wishes` | handlers_wishes.go |
| `POST /api/wishes` | handlers_wishes.go |
| `GET /api/wishes/{id}` | handlers_wishes.go |
| `PATCH /api/wishes/{id}` | handlers_wishes.go |
| `DELETE /api/wishes/{id}` | handlers_wishes.go |

<!-- END GENERATED ROUTES -->

## 读表须知（手写）

- ⚠️ **`handlers_materials.go` 里放的是课程/作业 handler**（`/api/courses`、`/api/assignments`）。真正的素材库在 `handlers_materials_full.go`。这两个文件名是全仓最容易走错的一对。
- ⚠️ **`GET /api/import/history` 注册在 `handlers_memory.go`**，不在两个 `handlers_import*.go` 里 —— handler 住在哪就在哪注册，所以表里那一格是对的，只是反直觉。
- `GET /api/version` 除契约版本外还带 `locales`：`available`（这个安装装了哪些语言）+ `defaultPrimary`/`defaultSecondary`（新用户默认）。**用户自己的一主一副在 `/api/session/preferences`**，不在这里。
- `POST /api/ai/companion` 是同步 SSE；`/async` 那条返回 202，结果靠 `GET /api/chat/messages/{id}` 轮询。agent loop 本身在 `agent.go` + `agent_tools.go`，不在 handler 文件里。
- `/api/plan` 的 PATCH 语义在 `plan_patch.go`；`/api/rules` 的校验在 `rule_validation.go`。两者都不注册路由，但改行为要连它们一起读。
- `/api/channels/*` 是**用户**给自己绑 QQ/OneBot 账号，不是运维端点（曾经被归进 `admin (prompts)` 组，2026-07-29 改正）。agent 侧的入站处理在 `channel_agent.go`。
- Admin 那三组走 `X-Admin-Token`（**不查库**，见 `docs/AUTH.md`；该鉴权方案本身待改造）。`/api/admin/db/*` 是裸库读写删，是全仓权限最高的一组。
