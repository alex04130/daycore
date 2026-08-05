# Agent Loop 与聊天

> 实时文档：改 agent/SSE/chat/channel 必须同批更新本文件。最后全面核对：2026-07-14。

## 同步 SSE 端点 `POST /api/ai/companion`（handlers_ai_companion.go）

- 请求 `{message, timezone, assistantName, threadId?, conversationHistory?}`。
- ctx = `context.WithTimeout(r.Context(), cfg.AIRequestTimeout)`——**绑客户端连接，断开即取消**（这是同步/异步的分水岭）。
- SSE：event-stream 头 + `http.NewResponseController` 手动 flush，每帧 `data: {json}\n\n`。
- threadId 非空：注入线程 rolling Summary + `ListMessages` 最近 50 条（newest-first 反转）；结束后 `AppendMessages`（user+assistant 两条，用 `context.WithoutCancel` 尽力写）。无 threadId：用客户端 conversationHistory（≤20 条，角色白名单）。
- `maybeCompress`（context.go）：60% 阈值触发滑动窗口压缩，flash 模型压缩，可回写线程 Summary。
- 新消息会 `decisions.cancelForSession(sid)` 作废未决卡。

## runCompanionAgent（agent.go）

```go
func (s *Server) runCompanionAgent(ctx, sink /*阶段2起为 agentSink 接口*/, r *http.Request, provider, sid, locale, tz string, messages []ai.Message) string
```
- max `cfg.AgentMaxRounds`(6) 轮；15 工具（plan_add/plan_update/plan_remove/rule_upsert/rule_remove/memory_add/memory_remove/assignment_upsert/wish_add/mood_record/material_add/get_weather/web_search/list_upcoming/propose_decision）定义在 agent_tools.go `companionToolDefs`，后四个捕捉工具的实现在 tool_capture.go。每次流式调用（每轮）落一行 ai_call_logs（s.logAICall，endpoint=companion）；web_search 的结果片段经 untrustedWrap 包装后才进模型上下文（toolResult.ForModel），客户端拿到的仍是干净数据。
- `r` 只用于限流 key（`s.limiter.Allow(s.clientIP(r))`）。
- 返回累积 assistant 文本，供持久化。

## SSE v2 帧协议

`delta / reasoning / tool_start / tool_result / decision_card / error / done`，心跳 `: ping\n\n`。tool_result 带 `opId` 供撤销（POST /api/ops/{id}/revert）。契约细节见 `api/FRONTEND_HANDOFF.md` §B。

## 决策卡（agent.go decisionRegistry）

- 纯内存 `map[id]*pendingDecision`，每 session 同时一张（新卡顶旧卡）；进程重启即丢（单实例假设）。
- `propose_decision` 工具 → `runProposeDecision`：发 decision_card 帧 → 阻塞 select（答案 / 超时→`{"choice":"timeout"}` 让模型收尾 / 15s ping / ctx.Done→cancelled）。等待时长：decisionTimeoutSSE=45s，sink 实现 decisionWaiter 时用其值（异步 90s）。仅 interactive sink 拿得到这个工具。
- 响应端点 `POST /api/decisions/{id}/respond` `{choice, text}` → registry.resolve；未命中 404 decision_not_found。

## sink 体系（agentSink 接口，agent.go）

`runCompanionAgent(ctx, sink agentSink, r, provider, sid, locale, tz, messages, interactive)`：
- `sseSender{w, rc}`：真 SSE 输出（同步端点，interactive=true）。
- `discardSink{}`（channel_agent.go）：全丢，只要返回文本（通道回复，**interactive=false** → `companionToolDefs(caps, false)` 不注册 propose_decision，消除"卡片被丢还白等"的旧疾）。
- `recordingSink`（handlers_ai_companion_async.go）：录 tool_start/tool_result/decision_card 帧进内存；decision_card 到达即通过 onCard 把 toolEvents 落进占位消息；实现 `decisionWait()` → 90s（常量 decisionTimeoutAsync；SSE 默认 decisionTimeoutSSE=45s）。

## QQ 通道路径（channel_agent.go）

- `HandleInbound(ctx, reg, msg)`：查 ChannelBindings → `runChannelMessage` → `reg.Channel(...).Send` 回写；未绑定直接忽略。
- `runChannelMessage`：**无状态单轮**（system+user，无历史加载、不持久化 thread）；最小 `http.Request{RemoteAddr: sid}` 做限流 bucket。
- main.go：`go srv.HandleInbound(context.Background(), ...)` 每消息一 goroutine。

## Chat 线程（handlers_chat.go，6 路由）

GET/POST `/api/chat/threads`、PATCH/DELETE `/api/chat/threads/{id}`、GET/DELETE `/api/chat/threads/{id}/messages`。
- 分页：`?before=`（UnixMilli）+ `?limit=`（默认 50），`created_at < before ORDER BY created_at DESC`。
- ChatMessage：ID/ThreadID/SessionID/Role/Content/ToolEvents/Status/CreatedAt（Status：`""≡done`/pending/done/error）。
- ToolEvents 由异步端点写入（recordingSink），格式 = SSE v2 帧对象数组；同步 SSE 端点仍不写它。
- ChatRepository 含 GetMessage/UpdateMessage(ChatMessageUpdate)/FailPendingMessages（sqlstore/chat.go + mongostore/chat_repo.go 双实现）。
- ⚠️ mongostore `AppendMessages` 不回写生成的 ID（sqlstore 会）——需要 ID 时必须 handler 预生成。
- 批量写时间戳 `now + int64(i)` 保证同批递增（cursor 不跳过）。

## 异步端点 `POST /api/ai/companion/async`（handlers_ai_companion_async.go）

流程：rateLimit → threadId 必填 + 属主校验（ListThreads 匹配）→ cancelForSession → **预生成两个 uuid**（mongostore Append 不回写 ID）→ AppendMessages 写 user 消息 + pending 占位 → 202 `{threadId, userMessageId, messageId, status}` → `s.GoTracked` 背景 goroutine（`context.WithTimeout(Background, AIRequestTimeout)`）→ `buildCompanionMessages`（exclude 刚写入的两行；pending 行一律跳过）→ recordingSink 跑 agent → `UpdateMessage(WithoutCancel)` 写 content+toolEvents+status(done/error)，panic 有 recover 兜底标 error。
- 取结果：轮询 `GET /api/chat/messages/{id}` 或线程消息列表；status `""≡done` 兼容语义。
- 决策卡：卡片即时落 toolEvents（status 仍 pending），客户端走现有 respond 端点，等待 90s。
- 崩溃兜底：main.go 启动时 `FailPendingMessages` 清扫遗留 pending → error。
- 关停：`Server.asyncWG` + `GoTracked`/`WaitBackground`；main.go 在 httpSrv.Shutdown 后 WaitBackground；HandleInbound 的每消息 goroutine 也走 GoTracked；`worker.Stop()` 等 `cron.Stop().Done()`。
- `buildCompanionMessages(ctx, sid, threadID, locale, tz, name, userMsg, exclude)`（handlers_ai_companion.go）是同步/异步共享的消息组装（system+summary+50 条历史+压缩）。
