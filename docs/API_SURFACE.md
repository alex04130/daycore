# 路由 → Handler 映射总表

> ⚠️ **路径带版本：`/api/v2/…`**（2026-08-11）。规则在 `internal/apipath`，服务器和 openapi 生成器共用同一个函数 —— 两份会漂。
>
> **三条留在 `/api/` 外面**，各有各的理由：`GET|POST /api/version`（发现：得先知道版本才能问版本是鸡生蛋）、`GET /api/healthz`（存活探针配在基础设施里，配的人不跟这个契约）、`/api/auth/oauth/{provider}/callback`（注册在 Google/GitHub 控制台里的重定向目标，没有任何客户端调它）。

> **下面的表是生成的**，源是各 handler 文件 `init()` 里的 `registerRoutes`。跑 `make api-surface` 重生成；`go test ./internal/server/` 会因它过期而红。
>
> 手写的只有末尾「读表须知」一节 —— 那里放的是表格表达不了的东西（哪个文件名骗人、哪条路由有隐藏语义）。
>
> 三件事由测试兜住，不靠自觉（`internal/server/routes_test.go`）：本表与代码一致、**代码与 `api/openapi.yaml` 双向一致**（服务了没写进契约 / 写进契约没人服务都红）、pattern 不重复。REST 细节（请求/响应 schema）以 `api/openapi.yaml` 为准 —— 但**改的是 `api/spec/paths/<tag>.yaml` 然后 `make api-bundle`**，openapi.yaml 是生成物，手改会被覆盖。
>
> 当前 **146 条路由 / 36 个组**。

<!-- BEGIN GENERATED ROUTES -->
## admin (config)（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/config` | handlers_admin_config.go |
| `PUT /api/v2/admin/config` | handlers_admin_config.go |

## admin (frontends)（7 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/frontends` | handlers_admin_frontends.go |
| `PUT /api/v2/admin/frontends/builds/{hash}/family` | handlers_admin_frontends.go |
| `PUT /api/v2/admin/frontends/families/{id}` | handlers_admin_frontends.go |
| `DELETE /api/v2/admin/frontends/families/{id}` | handlers_admin_frontends.go |
| `GET /api/v2/admin/frontends/families/{id}/backfill` | handlers_admin_frontends.go |
| `POST /api/v2/admin/frontends/families/{id}/backfill` | handlers_admin_frontends.go |
| `DELETE /api/v2/admin/frontends/families/{id}/backfill` | handlers_admin_frontends.go |

## admin (health)（1 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/health` | handlers_admin_health.go |

## admin (models)（3 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/models` | handlers_admin_models.go |
| `POST /api/v2/admin/models/{id}/test` | handlers_admin_models.go |
| `GET /api/v2/admin/oauth` | handlers_admin_models.go |

## admin (pairings)（5 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/pairings` | handlers_admin_pairings.go |
| `POST /api/v2/admin/pairings` | handlers_admin_pairings.go |
| `DELETE /api/v2/admin/pairings/{id}` | handlers_admin_pairings.go |
| `PUT /api/v2/admin/pairings/{id}/full` | handlers_admin_pairings.go |
| `PUT /api/v2/admin/pairings/{id}/roles` | handlers_admin_pairings.go |

## admin (prompts)（3 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/prompts` | handlers_admin.go |
| `GET /api/v2/admin/prompts/{key}` | handlers_admin.go |
| `PUT /api/v2/admin/prompts/{key}` | handlers_admin.go |

## admin (providers)（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/providers` | handlers_admin_providers.go |
| `PUT /api/v2/admin/providers` | handlers_admin_providers.go |

## admin (restart)（1 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/v2/admin/restart` | restart.go |

## admin (roles)（6 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/permissions` | handlers_admin_roles.go |
| `GET /api/v2/admin/roles` | handlers_admin_roles.go |
| `PUT /api/v2/admin/roles/{name}` | handlers_admin_roles.go |
| `DELETE /api/v2/admin/roles/{name}` | handlers_admin_roles.go |
| `PUT /api/v2/admin/users/{id}/owner` | handlers_admin_roles.go |
| `PUT /api/v2/admin/users/{id}/roles` | handlers_admin_roles.go |

## admin (session)（3 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/session` | handlers_admin_session.go |
| `POST /api/v2/admin/session` | handlers_admin_session.go |
| `DELETE /api/v2/admin/session` | handlers_admin_session.go |

## admin (stats, users, DB)（11 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/ailogs` | handlers_admin_stats.go |
| `GET /api/v2/admin/db/backup` | handlers_admin_db.go |
| `GET /api/v2/admin/db/export` | handlers_admin_db.go |
| `POST /api/v2/admin/db/import` | handlers_admin_db.go |
| `GET /api/v2/admin/db/table/{name}` | handlers_admin_db.go |
| `DELETE /api/v2/admin/db/table/{name}/{id}` | handlers_admin_db.go |
| `GET /api/v2/admin/db/tables` | handlers_admin_db.go |
| `GET /api/v2/admin/stats` | handlers_admin_stats.go |
| `GET /api/v2/admin/usage` | handlers_admin_stats.go |
| `GET /api/v2/admin/users` | handlers_admin_stats.go |
| `DELETE /api/v2/admin/users/{id}` | handlers_admin_stats.go |

## admin (theme kinds)（3 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/admin/theme-kinds` | handlers_admin_kinds.go |
| `PUT /api/v2/admin/theme-kinds/{name}` | handlers_admin_kinds.go |
| `DELETE /api/v2/admin/theme-kinds/{name}` | handlers_admin_kinds.go |

## ai（10 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/v2/ai/auto-plan` | handlers_autoplan.go |
| `POST /api/v2/ai/companion` | handlers_ai_companion.go |
| `POST /api/v2/ai/companion/async` | handlers_ai_companion_async.go |
| `POST /api/v2/ai/extract-schedule-image` | handlers_ai_plan.go |
| `POST /api/v2/ai/mood` | handlers_ai_mood.go |
| `POST /api/v2/ai/plan-image` | handlers_ai_plan.go |
| `POST /api/v2/ai/plan-text` | handlers_ai_plan.go |
| `POST /api/v2/ai/theme` | handlers_ai_theme.go |
| `POST /api/v2/ai/travel` | handlers_ai_travel.go |
| `POST /api/v2/decisions/{id}/respond` | agent.go |

## auth（7 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/auth/oauth/{provider}/callback` | handlers_auth_oauth.go |
| `POST /api/v2/auth/login` | handlers_auth.go |
| `POST /api/v2/auth/logout` | handlers_auth.go |
| `GET /api/v2/auth/oauth/{provider}` | handlers_auth_oauth.go |
| `GET /api/v2/auth/providers` | handlers_auth_oauth.go |
| `POST /api/v2/auth/register` | handlers_auth.go |
| `GET /api/v2/me` | handlers_auth.go |

## canvas materials（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/assignments` | handlers_materials.go |
| `POST /api/v2/assignments` | handlers_materials.go |
| `PATCH /api/v2/assignments/{id}` | handlers_materials.go |
| `GET /api/v2/courses` | handlers_materials.go |

## channels (user binds an account)（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/channels` | handlers_channels.go |
| `POST /api/v2/channels/{channel}/bind` | handlers_channels.go |
| `DELETE /api/v2/channels/{channel}/unbind` | handlers_channels.go |
| `POST /api/v2/channels/{channel}/verify` | handlers_channels.go |

## chat threads（7 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/chat/messages/{id}` | handlers_chat.go |
| `GET /api/v2/chat/threads` | handlers_chat.go |
| `POST /api/v2/chat/threads` | handlers_chat.go |
| `PATCH /api/v2/chat/threads/{id}` | handlers_chat.go |
| `DELETE /api/v2/chat/threads/{id}` | handlers_chat.go |
| `GET /api/v2/chat/threads/{id}/messages` | handlers_chat.go |
| `DELETE /api/v2/chat/threads/{id}/messages` | handlers_chat.go |

## companion history（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/companion-history` | handlers_companion_history.go |
| `POST /api/v2/companion-history` | handlers_companion_history.go |

## console（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /admin` | console.go |
| `GET /admin/` | console.go |

## custom themes（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/themes` | handlers_themes.go |
| `POST /api/v2/themes` | handlers_themes.go |
| `PATCH /api/v2/themes/{id}` | handlers_themes.go |
| `DELETE /api/v2/themes/{id}` | handlers_themes.go |

## feedback（1 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/v2/feedback` | handlers_feedback.go |

## files（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/files` | handlers_files.go |
| `POST /api/v2/files` | handlers_files.go |
| `GET /api/v2/files/{id}` | handlers_files.go |
| `DELETE /api/v2/files/{id}` | handlers_files.go |

## health & diagnostics（3 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/healthz` | handlers_misc.go |
| `GET /api/v2/models` | handlers_misc.go |
| `GET /api/version` | handlers_misc.go |

## imports（4 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/v2/import/canvas` | handlers_import_canvas.go |
| `POST /api/v2/import/ics` | handlers_import.go |
| `GET /api/v2/import/token` | handlers_import.go |
| `POST /api/v2/import/token` | handlers_import.go |

## inbox（3 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/v2/inbox/commit` | handlers_inbox.go |
| `POST /api/v2/inbox/process` | handlers_inbox.go |
| `POST /api/v2/inbox/upload` | handlers_inbox.go |

## long-term memory（5 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/import/history` | handlers_memory.go |
| `GET /api/v2/memory` | handlers_memory.go |
| `POST /api/v2/memory` | handlers_memory.go |
| `DELETE /api/v2/memory` | handlers_memory.go |
| `DELETE /api/v2/memory/{id}` | handlers_memory.go |

## materials（7 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/materials` | handlers_materials_full.go |
| `POST /api/v2/materials` | handlers_materials_full.go |
| `GET /api/v2/materials/categories` | handlers_materials_full.go |
| `GET /api/v2/materials/search` | handlers_materials_full.go |
| `GET /api/v2/materials/{id}` | handlers_materials_full.go |
| `PATCH /api/v2/materials/{id}` | handlers_materials_full.go |
| `DELETE /api/v2/materials/{id}` | handlers_materials_full.go |

## meta（1 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/version` | handlers_handshake.go |

## moods（4 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/mood` | handlers_mood.go |
| `POST /api/v2/mood` | handlers_mood.go |
| `PATCH /api/v2/mood` | handlers_mood.go |
| `GET /api/v2/mood/kinds` | handlers_mood.go |

## operation logs & undo（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/ops` | handlers_ops.go |
| `POST /api/v2/ops/{id}/revert` | handlers_ops.go |

## plans（5 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/plan` | handlers_plan.go |
| `POST /api/v2/plan` | handlers_plan.go |
| `PATCH /api/v2/plan` | handlers_plan.go |
| `POST /api/v2/plan/lock` | handlers_plan.go |
| `GET /api/v2/plan/range` | handlers_plan.go |

## proposals（3 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/v2/plan/conflict` | proposals.go |
| `GET /api/v2/proposals` | proposals.go |
| `POST /api/v2/proposals/{id}/respond` | proposals.go |

## schedule rules（5 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/rules` | handlers_rules.go |
| `POST /api/v2/rules` | handlers_rules.go |
| `POST /api/v2/rules/batch` | handlers_rules.go |
| `PATCH /api/v2/rules/{id}` | handlers_rules.go |
| `DELETE /api/v2/rules/{id}` | handlers_rules.go |

## session（5 条）

| 路由 | Handler 文件 |
|---|---|
| `POST /api/v2/session/init` | handlers_session.go |
| `GET /api/v2/session/preferences` | handlers_preferences.go |
| `PATCH /api/v2/session/preferences` | handlers_preferences.go |
| `PATCH /api/v2/session/settings` | handlers_session.go |
| `POST /api/v2/session/theme` | handlers_session.go |

## temp-context (session-scoped key-value store)（2 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/temp-context` | handlers_tempcontext.go |
| `PUT /api/v2/temp-context` | handlers_tempcontext.go |

## wishes（5 条）

| 路由 | Handler 文件 |
|---|---|
| `GET /api/v2/wishes` | handlers_wishes.go |
| `POST /api/v2/wishes` | handlers_wishes.go |
| `GET /api/v2/wishes/{id}` | handlers_wishes.go |
| `PATCH /api/v2/wishes/{id}` | handlers_wishes.go |
| `DELETE /api/v2/wishes/{id}` | handlers_wishes.go |

<!-- END GENERATED ROUTES -->

## 读表须知（手写）

- ⚠️ **`handlers_materials.go` 里放的是课程/作业 handler**（`/api/courses`、`/api/assignments`）。真正的素材库在 `handlers_materials_full.go`。这两个文件名是全仓最容易走错的一对。
- ⚠️ **`GET /api/import/history` 注册在 `handlers_memory.go`**，不在两个 `handlers_import*.go` 里 —— handler 住在哪就在哪注册，所以表里那一格是对的，只是反直觉。
- `GET /api/version` 除契约版本外还带 `locales`：`available`（这个安装装了哪些语言）+ `defaultPrimary`/`defaultSecondary`（新用户默认）。**用户自己的一主一副在 `/api/session/preferences`**，不在这里。
- `POST /api/ai/companion` 是同步 SSE；`/async` 那条返回 202，结果靠 `GET /api/chat/messages/{id}` 轮询。agent loop 本身在 `agent.go` + `agent_tools.go`，不在 handler 文件里。
- `/api/plan` 的 PATCH 语义在 `plan_patch.go`；`/api/rules` 的校验在 `rule_validation.go`。两者都不注册路由，但改行为要连它们一起读。
- `/api/channels/*` 是**用户**给自己绑 QQ/OneBot 账号，不是运维端点（曾经被归进 `admin (prompts)` 组，2026-07-29 改正）。agent 侧的入站处理在 `channel_agent.go`。
- Admin 那三组走 `X-Admin-Token`（**不查库**，见 `docs/AUTH.md`；该鉴权方案本身待改造）。`/api/admin/db/*` 是裸库读写删，是全仓权限最高的一组。
