# Daycore — API 契约（v4 · beta，琉璃 / 纸屿共用）

> 本文是**唯一权威契约**。两套前端 UI 风格不同、能力对齐、同步演进，都只依赖这一套 HTTP API：
> - **琉璃 liuli/** — 玻璃拟态，基于 DaycoreUI 设计系统；用户端 `liuli/index.html`，管理台 `liuli/admin.html`（独立页面）。
> - **纸屿 zhiyu/** — 纸感手帐风，自带 dc.css；用户端与管理台同一 SPA（`zhiyu/index.html`，管理台在 `#/admin`）。
>
> 部署时可任选其一，或两个一起部署（如 `/` 与 `/paper/` 各挂一套 STATIC_DIR）。beta 阶段无需后向兼容。
> 两套 UI 目前各自带 mock（localStorage，互不共享演示数据）；接真后端后数据自然共享。

## 0. 通用约定
- 错误信封：非 2xx 返回 `{"error":"<code>","message":"人类可读"}`。已用 code：
  `unauthorized forbidden invalid_credentials email_taken weak_password invalid_email last_admin self_delete unknown_key invalid_value invalid_models invalid_oauth not_found`。
- 鉴权：HttpOnly session cookie（`credentials:'include'`）；`GET /api/me` → `{user|null, onboarded, profile}`，`user.role ∈ user|admin`。管理台入口仅对 admin 显示，服务端仍须校验。
- 时间：日期 `YYYY-MM-DD`，时刻 `HH:mm`，时间戳 ISO8601。

## 1. 用户侧端点
```
GET  /api/me                          → {user|null, onboarded, profile}
POST /api/auth/register|login|logout
GET  /api/auth/oauth/{p}/start        整页跳转，回调后 302 回前端
PUT  /api/profile                     patch；见 §3 profile（含 l2Prompt）
GET  /api/plan?date=                  → block[]
PUT  /api/plan/blocks                 upsert（无 id 即创建）
DELETE /api/plan/blocks/{id}?date=
POST /api/plan/blocks/{id}/toggle-done | toggle-lock
POST /api/ai/auto-plan                {dates[],instruction,mode:keep|replace} → {days:[{date,blocks[]}],summary}
POST /api/plan/undo-last              → {label}
GET|PUT|DELETE /api/courses[/{id}]
GET|PUT|DELETE /api/assignments[/{id}]
POST /api/assignments/{id}/status     {status: todo|doing|done}
GET|POST|DELETE /api/rules[/{id}]     + POST /api/rules/{id}/toggle
GET  /api/inbox                       随手记收集箱
POST /api/inbox/capture               {text} → item（parsed:{kind:assignment|block|memory,payload}）
POST /api/inbox/{id}/confirm          body 可携带用户改过的 parsed → {kind}
POST /api/inbox/{id}/dismiss
GET  /api/imports                     导入历史
POST /api/import/canvas | /ics        → {added,skipped}
POST /api/import/screenshot           multipart → {preview:[course…]}（识别不落库）
POST /api/import/courses/apply        {courses[]} → {added,skipped}
GET|POST /api/moods                   POST {mood,note?} → {…,reply}（AI 一句回应；见 §3 mood）
GET|POST|DELETE /api/memory[/{id}]
POST /api/ai/theme                    {desc} → {name,desc,base,vars}（vars 为 CSS 变量覆盖；主题存储属于设备，云同步则加 /api/themes CRUD）
GET|POST /api/data/export|import
```

## 2. 陪伴（流式协议）
```
GET|POST|DELETE /api/companion/threads[/{id}]
POST /api/companion/threads/{id}/messages   (SSE, event:frame, data:JSON)
  {t:"start"}
  {t:"tool_start", label}            0..n，可与 tool_result 交错
  {t:"tool_result", label, result}
  {t:"delta", delta, content}        content=累计全文
  {t:"decision_card", card}          可选
  {t:"done", message}                message={id,role:"assistant",content,ts,card|null}
POST /api/companion/threads/{id}/messages/{mid}/decision  {accept:bool} → {summary}

card = {id,status:pending|accepted|dismissed,title,date,summary,
        changes:[{action:"add",block:{date,start,end,title,type}}|{action:"remove",match,label}]}
```
上下文注入顺序（服务端拼装）：系统提示词（管理台 prompts）→ 用户 `profile.l2Prompt`（L2，逐用户）→ memory → 课表/作业/规则/近期计划。

## 3. 领域形状
```
profile    {userName?, assistantName, wake:"07:30", sleep:"23:30",
            focus: morning|afternoon|evening, l2Prompt: string ≤2000}
block      {id,date,start,end,title,note,type:course|task|meal|break|exercise|custom,
            source:auto|manual|companion,done,locked}   locked/manual 在 mode=keep 时不被重排
assignment {id,title,course,due,status:todo|doing|done,notes,
            source:canvas|manual|companion|capture,createdAt}
rule       {id,text,active,source:user|companion,createdAt}   （自由文本约定，规划必须遵守）
mood       {id,ts,date,mood:<happy|calm|excited|grateful|okay|bored|tired|stressed|anxious|sad|angry|lonely>,
            note:"用户自定义一句话（可空）", reply:"AI 回应"}
memory     {id,text,source:user|companion|capture,createdAt}
course     {id,name,teacher,room,day:1-7,start,end}
theme      {id,name,desc,base:sky|sunset|night|nature,vars:{--*…},createdAt}
```

## 4. 管理台端点（全部需 admin）
```
GET  /api/admin/system        版本/uptime/db/counts/needsRestart/restartKeys/configPaths
GET|PUT /api/admin/env        条目 {key,group,label,type,options,isSecret,isSet,
                              source: env|file|default, editable, restartRequired, value(secret→掩码)}
                              PUT {KEY:value,…}：热键立即生效，其余累积 restartKeys；""=清除
GET|PUT /api/admin/models     {models[],defaults{chat,vision,planner},requestTimeout,agentMaxRounds}
                              校验：id 唯一 [a-z0-9._-]；format∈openai|anthropic|ollama；远程需 apiKeyEnv；
                              defaults.chat 必须存在且 enabled；响应可带 warnings[]
POST /api/admin/models/test   {id}|{config} → {ok,latencyMs,reply,error,errorCode}
GET|PUT /api/admin/oauth      GET：clientId 掩码 + clientSecretSet + callbackUrl(只读)；
                              PUT：clientSecret ""=保持；预设 google/github 只需 id/secret，
                              自定义须补全 authUrl/tokenUrl/userinfoUrl + 字段映射
GET  /api/admin/users         含 lastActiveAt, sessionCount
PATCH  /api/admin/users/{id}  {role} — 错误 last_admin
DELETE /api/admin/users/{id}  — 错误 self_delete | last_admin
GET /api/admin/prompts, PUT /api/admin/prompts/{key}     全局系统提示词（L1）
GET /api/admin/stats          {series:[{date,messages,plans,moods}]×7, models:[{id,calls,tokens}]}
GET /api/admin/ai-logs | /audit
GET /api/admin/db/tables | /db/{table}      browse 限前 N 行
POST /api/admin/db/backup     → 文件下载
POST /api/admin/restart-ack   管理员确认已重启，清除 needsRestart
```

## 5. 两套 UI 的适配说明
- **纸屿**：`zhiyu/app/store*.js` 即本契约的 mock 镜像（方法名 ≈ 端点，形状一致），换 fetch 即接入；细节见 `zhiyu/HANDOFF.md`。
- **琉璃**：`liuli/app/store.js`/`store-ext.js` 与 `liuli/admin/admin-store.js` 沿用早期 v2 字段命名
  （如 `assignment.dueAt`(datetime)、`mood.aiResponse`、带重复规则的 rules）。接真后端时在其 store 层做**字段适配**
  （dueAt↔due+时刻、aiResponse↔reply、recurring rules 由后端展开为 blocks），页面不动。契约冲突处一律以本文为准。
- 功能对齐清单（两边都有）：随手记收集箱、三入口导入（顶部常驻）、自主规划+撤销、陪伴流式+决策卡、
  心情打卡（含自定义一句话 note）+ 三个练习、AI 主题生成、L2 自定义提示词、管理台（模型/环境/OAuth/用户/日志数据）+ 用户端管理台入口。
