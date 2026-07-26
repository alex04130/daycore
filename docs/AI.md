# AI 子系统

> 实时文档：改 provider/prompts/vision 必须同批更新本文件。最后全面核对：2026-07-14。

## Provider 与 Catalog（internal/ai/）

- `AIProvider` 接口 + `ChatRequest{Messages, Tools, Temperature, MaxTokens, JSONMode, Stop, CacheKey}` + Capabilities（provider.go）。
- formats/：openai（最完整，流式 tool_calls + 夹具测试）、anthropic（text-only 流式）、ollama（NDJSON）。
- Catalog（models.go，config/models.yaml）：`DefaultChat()` / `Vision()` / `Planner()`；模型条目支持 extra_body/max_tokens/thinking。
- StreamAccumulator（stream.go）按 index 拼合流式 tool_calls。

## 提示词（prompts.go）

- `//go:embed prompts/*/*.tmpl`，`PromptService.Render(ctx, key, locale, data)`（text/template，missingkey=zero）；DB override 优先（admin API 可编辑）。
- **双 locale 铁律**：zh-CN 与 en-US 必须同名成对，缺一 `NewPromptService` 启动报错。
- key 清单（2026-07-15）：auto_plan、companion_agent（L1 纯规则清单零人格）、companion_context、day_plan_text、day_plan_image、mood、schedule_extract_image、theme_gen、inbox_classify（随手记录归类器，Categories 注入启用类别清单）、food_recognize（拍照→diet 营养估计，走 vision）、travel_suggest（目的地→行程 JSON）。
- 组装顺序：L1 → L3 → L2(persona) → L1_reminder。
- install 导出器 walk embed FS——新模板自动被导出，无需改 install.go。

## 一次性（非 agent loop）AI 调用范式

```go
prompt, _ := s.prompts.Render(ctx, ai.PromptXxx, s.requestLocale(r), data)
resp, err := s.catalog.DefaultChat().Chat(ctx, ai.ChatRequest{
    Messages: []ai.Message{{Role: RoleSystem, Content: prompt}, {Role: RoleUser, Content: input}},
    JSONMode: true, Temperature: 0.3, MaxTokens: 4096,
})
obj, ok := extractJSONObject(resp)   // handlers_ai_helpers.go：取首 { 到末 }
// 检查 obj["error"] → 返回
```
现有样例：handlers_ai_mood.go（单 user 消息，temp 0.7）、handlers_ai_plan.go（system+user，JSONMode）、handlers_autoplan.go（8192 tokens）。

## Vision 管线（vision.go）

- `Orchestrator.PlanFromImage(ctx, chat, systemPrompt, imageB64, mime) (string, error)`——**与提示词无关**，任意 system prompt + 图皆可复用。
- 三分支：chat 模型自带 vision → 直接给图；无 vision 无 tools → 转 catalog 的 vision 模型；无 vision 有 tools → read_image/zoom_image 工具循环 ≤6 轮，超轮强制无工具出 JSON。
- 传图：raw base64（**无 `data:` 前缀**）+ mime，`ContentPart{Type: PartImage}`。

## 上下文压缩（server/context.go）

`maybeCompress`：估算 token 超 60% 阈值 → flash 模型压缩滑窗 → 开环提取为 memory，可回写 ChatThread.Summary。
