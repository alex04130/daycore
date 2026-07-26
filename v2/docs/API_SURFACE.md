# 路由 → Handler 映射总表

> 实时文档：加/改路由必须同批更新本表 + `v2/api/openapi.yaml`。REST 细节（请求/响应 schema）以 openapi.yaml 为准，本表只管「路由在哪个文件」。最后全面核对：2026-07-14。

## 公开（无需 session）

| 路由 | Handler 文件 |
|---|---|
| GET /api/healthz（含 version/channel/env） | handlers_misc.go |
| GET /api/models | handlers_misc.go |
| GET /api/version（API 契约版本，2026-07-14 起） | handlers_misc.go |
| POST /api/session/init | handlers_session.go |
| POST /api/auth/register·login·logout, GET /api/auth/providers, GET /api/me | handlers_auth.go |
| GET /api/auth/oauth/{provider}[/callback] | handlers_auth_oauth.go |

## 会话/偏好/主题

| 路由 | Handler 文件 |
|---|---|
| GET·PATCH /api/session/settings, /api/session/preferences | handlers_session.go（preferences 在 handlers_preferences.go） |
| GET·POST /api/themes, PATCH·DELETE /api/themes/{id} | handlers_themes.go |
| POST /api/ai/theme | handlers_ai_theme.go |

## 计划/规则/自动规划

| 路由 | Handler 文件 |
|---|---|
| GET·POST·PATCH /api/plan, GET /api/plan/range | handlers_plan.go + plan_patch.go |
| GET·POST /api/rules, PATCH·DELETE /api/rules/{id}, POST /api/rules/batch | handlers_rules.go + rule_validation.go |
| POST /api/ai/auto-plan | handlers_autoplan.go |
| POST /api/ai/plan-text · plan-image · extract-schedule-image | handlers_ai_plan.go |

## AI 聊天

| 路由 | Handler 文件 |
|---|---|
| POST /api/ai/companion（同步 SSE） | handlers_ai_companion.go（loop 在 agent.go + agent_tools.go） |
| POST /api/ai/companion/async（202 + 轮询取结果） | handlers_ai_companion_async.go |
| POST /api/decisions/{id}/respond | agent.go handleDecisionRespond |
| GET·POST /api/chat/threads, PATCH·DELETE /api/chat/threads/{id}, GET·DELETE /api/chat/threads/{id}/messages | handlers_chat.go |
| GET /api/chat/messages/{id}（异步轮询） | handlers_chat.go |
| POST /api/ai/mood | handlers_ai_mood.go |

## 资料/收件箱/学业/心情

| 路由 | Handler 文件 |
|---|---|
| GET·POST /api/materials, GET·PATCH·DELETE /api/materials/{id}, GET /api/materials/search | handlers_materials_full.go（⚠️ **handlers_materials.go 放的是课程/作业 handler**） |
| GET /api/materials/categories（注册表+启用标记） | handlers_materials_full.go |
| POST /api/inbox/process（AI 归类+draftId）· upload · commit | handlers_inbox.go |
| POST /api/ai/travel（行程建议+draftId） | handlers_ai_travel.go |
| GET /api/courses, GET·POST /api/assignments, PATCH /api/assignments/{id} | handlers_materials.go |
| GET·POST·PATCH /api/mood | handlers_mood.go |

## 记忆/导入/撤销/通道

| 路由 | Handler 文件 |
|---|---|
| GET·POST /api/memory, DELETE /api/memory[/{id}] | handlers_memory.go |
| POST /api/import/canvas · ics, GET·POST /api/import/token, GET /api/import/history | handlers_import.go + handlers_import_canvas.go |
| GET /api/ops, POST /api/ops/{id}/revert | handlers_ops.go |
| GET /api/channels, POST /api/channels/{ch}/bind · verify, DELETE .../unbind | handlers_channels.go（agent 侧 channel_agent.go） |

## Admin（X-Admin-Token）

| 路由 | Handler 文件 |
|---|---|
| prompts CRUD | handlers_admin.go |
| stats/ailogs/users | handlers_admin_stats.go |
| db tables/browse/delete/export/import/backup | handlers_admin_db.go |
