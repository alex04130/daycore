# AI 子系统

> 实时文档：改 provider/prompts/vision 必须同批更新本文件。最后全面核对：2026-08-02。

## Provider 与 Catalog（internal/ai/）

- `AIProvider` 接口 + `ChatRequest{Messages, Tools, Temperature, MaxTokens, JSONMode, Stop, CacheKey}` + Capabilities（provider.go）。
- formats/：openai（最完整，流式 tool_calls + 夹具测试）、anthropic（text-only 流式）、ollama（NDJSON）。
- Catalog（models.go，config/models.yaml）：`DefaultChat()` / `Vision()` / `Planner()`；模型条目支持 extra_body/max_tokens/thinking。
- StreamAccumulator（stream.go）按 index 拼合流式 tool_calls。

## 模态层（modality.go / generators.go，2026-08-02 落地）

**只有地基，没有接任何一家。** 类型系统能表达所有常见模态的进与出；具体接哪家、哪个音色、画什么图不在这一层。

**模态**（`Modality`）：`text` / `image` / `audio` / `document` / `video`。`Modalities` 是**排序存储**的集合 —— 由它派生的任何东西（工具定义、提示词里的能力行）必须逐字节稳定，不稳定会静默毁掉提示词缓存（见 `specs/transport.md`）。

**三种载运形式**（`Carriage`），因为厂商真的不一样，装成一种就会造出「声称支持、一半静默失败」的假能力：

| | 是什么 | 谁需要它 |
|---|---|---|
| `inline` | base64 进请求体 | 通用兜底；小图片在多数厂商只有这一条路 |
| `url` | 厂商自己去取 | Anthropic document、OpenAI file input；也让对象存储直接给签名 URL，省一次代理 |
| `file_id` | 先传到厂商，这是句柄 | **不是优化** —— Qwen-Long / Kimi 这类国产 OpenAI 兼容网关只认这条（传 `/v1/files` 后把 `fileid://…` 塞进 **system 消息**，不是 content part） |

**能力发现一律靠可选接口断言**，不进 `AIProvider`：`PartCarrier`（`CarriagesFor`）、`ToolStreamer`、`ImageGenerator`、`SpeechSynthesizer`、`Transcriber`、`Embedder`，各有 `As*` 助手函数。**沉默一律当「不支持」** —— 猜低只多一次 base64 往返，猜高是用户看着模型无视了他的附件。

`ContentPart{Type, Text, MIME, Data, URL, FileID, Name}` 是消息里的一段；`ChatResponse.Parts` 装产物，`ChatResponse.Usage` 装用量（含 `CachedTokens` / `ReasoningTokens`）。三个格式的 Chat 与 ChatStream 都解析 usage（流式经 `ai.Chunk.Usage` 末帧 → `StreamAccumulator` 汇总；openai 发 `stream_options.include_usage`，anthropic 拼 message_start/message_delta，ollama 取末帧计数）。server 侧 `s.logAICall`（best-effort）把**每一次模型调用**落进 `ai_call_logs`：9 个非流式端点 + companion 每轮一条。Catalog 另有四个能力选择器（`Transcriber()`/`SpeechSynthesizer()`/`ImageGenerator()`/`Embedder()`，断言遍历目录首个命中），`GET /api/version` 的 `features` 字段由它们派生——端侧据此发现本部署能力，不硬编码假设。

`GeneratedMedia` 拿的是字节不是 `blob.Ref`：这一层不许依赖存储层，而且 OpenAI 的图像端点现在只返回 base64，字节本来就是实际到手的东西。落不落盘由调用方决定。

## 提示词（prompts.go）

- **三层，内嵌是地板**（2026-07-29 接线，此前磁盘那层是死的）：

  | 层 | 来源 | 谁改 |
  |---|---|---|
  | DB 覆盖（最优先） | `prompt_overrides` 表 | 控制台，立即生效，多实例共享 |
  | 磁盘覆盖 | `PROMPTS_DIR/<locale>/<key>.tmpl` | 运维改文件，重启生效 |
  | 内嵌（地板） | `//go:embed prompts/*/*.tmpl` | 发版 |

  磁盘层是**逐文件覆盖**，缺的文件保留内嵌文本 —— 与语言目录同一条规则。原先的 `NewPromptServiceDisk` 要求磁盘上每个模板都在，那意味着「改一条提示词」要付二十二个文件的维护成本，之后每次上游改模板都悄悄到不了你手上。空文件与 parse 不过的模板**在加载时就报错**，不留到第一个请求（否则表现是某一个功能坏了，而完全看不出是磁盘上一个文件导致的）。

  ⚠️ 这层此前是**死的**：`NewPromptServiceDisk` 无人调用、`config.Load` 不读 `PROMPTS_DIR`，可 `daycore install` 却把模板解出来并把 `PROMPTS_DIR` 写进生成的 `.env` —— 安装器在配置一个服务器无视的东西。同时被清掉的还有生成 `.env` 里那行 `DATA_DIR`（同样从来没人读）。

- `PromptService.Render(ctx, key, locale, data)`（text/template，missingkey=zero）。
- **双 locale 铁律**：zh-CN 与 en-US 必须同名成对，缺一 `NewPromptService` 启动报错。
- key 清单（2026-07-15）：auto_plan、companion_agent（L1 纯规则清单零人格）、companion_context、day_plan_text、day_plan_image、mood、schedule_extract_image、theme_gen、inbox_classify（随手记录归类器，Categories 注入启用类别清单）、food_recognize（拍照→diet 营养估计，走 vision）、travel_suggest（目的地→行程 JSON）。
- 组装顺序：L1 → L3 → L2(persona) → L1_reminder。
- install 导出器 walk embed FS——新模板自动被导出，无需改 install.go。

### L1 硬边界：唯一一处没有 DB 层的提示词（`boundaries.go`）

`L1_reminder` 拼在 L2 人格**之后**，用来兜住「忽略以上所有指令」型的人格注入。它的编辑面只有一个：

| 层 | 来源 | 谁改 |
|---|---|---|
| 磁盘覆盖 | `PROMPTS_DIR/boundaries.json` | 运维改文件，**重启生效** |
| 内嵌（地板） | `//go:embed prompts/boundaries.json` | 发版 |

**没有 DB 层是有意的**。模板与消息目录两层都能从控制台改，那对语气与文案是对的取舍；对边界是错的 —— 能被控制台改写的边界，等于拿到控制台的人可以直接删，且删掉的东西**随下一个请求生效而不是随下一次发版**。要文件编辑 + 重启，就把这个改动放到了运维面前，而那才是该做这个决定的人。

它此前是 Go 字面量 + `HasPrefix(locale, "zh")` 的 switch（2026-08-06 移出）。那份写法买到的是同样的不可改写，代价却有两条不必要的：不可翻译，且**加减一条规则要发一次版** —— 而边界是会被调的，调它恰恰值得一次 diff 而不是一次重编译。

- 结构：`locales.<locale> = {heading, lead, rules[], closing}`，**逐 locale 整块覆盖**，文件里没写的 locale 用内嵌那份 —— 与模板、语言包同一条规则。
- 加一门语言 = 加一个键。读者是模型不是人，所以 locale 落空时**退到同语言其它区域 → `i18n.Default`**，宁可用另一门语言的边界，也不能没有边界。
- **不认识的字段直接启动失败**：`rules` 拼错会静默变成零条规则，而这正是这个文件最不能出的错。同理 `heading`/`closing` 为空、`rules` 为空一律拒绝 —— 空块更可能是文件写坏了，不是「我想删光边界」。
- 磁盘文件坏了 = **启动失败**，不是回退内嵌：运维是特意放那个文件的，无视它悄悄跑内嵌是最安静的一种忽略安全改动。
- `version` 比内嵌小 → 启动 **warn**（不是 error）：运维那份拷贝说了算，但它挡住了新版本新增的规则，这行日志是唯一会说出来的地方。
- 三道测试守着「控制台够不着」：`Set/Get` 拒绝任何 boundaries 形状的 key；改遍所有真 prompt key + 所有消息覆盖后 `Reminder()` 逐字节不变；`RouteTable` 里不许出现含 `boundar` 的路由。

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

## ⚠️ 只有 openai format 能流式送工具调用（2026-08-01 修）

`ToolCallDelta` 在三个 format 里的出现次数：**openai 1、anthropic 0、ollama 0**。anthropic 的 `ChatStream` 只处理 `content_block_delta` 与 `message_stop`，`tool_use` 块与 `input_json_delta` **落地丢弃**；ollama 只读 `Message.Content`。

而 `config/models.yaml` 里 `vision` 与 `chat-search` 都是 anthropic format、都写着 `tools: true`。所以把 `DEFAULT_CHAT_MODEL` 指向它们中任何一个，**companion 的工具会全部失效** —— 模型请求写计划，format 把请求扔了，loop 看到一轮没有工具调用就结束。任何地方都不报错。

修法用的是仓库里**早就为此存在、却没有生产调用方**的那座桥：`ai.StreamViaChat`（它的注释写着 "for formats whose ChatStream cannot carry tool calls yet (anthropic/ollama)"）。

- 新增可选接口 `ai.ToolStreamer` —— 它是 **format 的属性不是模型的属性**，所以不做成 `Capabilities` 字段：那样就得在 `models.yaml` 里声明，而运维无从知道我们的 anthropic 实现有没有解析 `tool_use` 块。
- `ai.StreamsToolCalls(p)` 对**不表态的 provider 一律判否**。两个方向的代价不对称：猜「否」的代价是一轮不增量（用户等整段而不是看它打字），猜「是」的代价是每个工具调用静默消失。
- `agent.openRound` 据此选路；**没有工具的请求永远走原生流式**（没什么可失去，增量文本正是重点）。
- `internal/ai/toolstream_test.go` 断言三个 format 的声明与实现相符，并要求**新增 format 必须在同一次改动里加一行** —— 这正是重点。

⚠️ 这个 bug 的形状值得记住：它不是某个函数写错了，是**换一行配置就悄悄坏掉**。单独测任何一个 format 都是通过的。

## 上下文压缩（server/context.go）

`maybeCompress`：估算 token 超 60% 阈值 → flash 模型压缩滑窗 → 开环提取为 memory，可回写 ChatThread.Summary。

## 添加一种 AI wire-format

格式通过 `ai.RegisterFormat`（`internal/ai/registry.go:32`）自注册，在 `init()` 里调用即可 —— 然后 `models.yaml` 加一条 `format: gemini` 的记录，**其余代码一行不改**。

```go
// internal/ai/formats/gemini/gemini.go
package gemini

import "daycore/internal/ai"

func init() { ai.RegisterFormat("gemini", New) }

type provider struct {
    cfg      ai.ModelConfig
    http     *http.Client
    endpoint string
}

// cfg.BaseURL 来自 models.yaml；cfg.APIKey 已从环境变量解析。
// 返回值要实现 Chat / ChatStream / Capabilities / Model 四个方法。
func New(cfg ai.ModelConfig) (ai.AIProvider, error) { /* … */ }

func (p *provider) Model() string                 { return p.cfg.Model }
func (p *provider) Capabilities() ai.Capabilities { return p.cfg.Caps }
```

三个已有实现按用途挑一个抄：

- `formats/openai/openai.go` —— OpenAI 兼容（DeepSeek 也走它）。**流式 tool_calls 的参考实现**：按 index 分片拼 `delta.tool_calls`、`delta.reasoning_content`、`finish_reason`；`mergeExtraBody` 浅合并且不覆盖既定键。
- `formats/anthropic/anthropic.go` —— Anthropic Messages API，支持 `ServerSide` 工具（`web_search_20250305`）、`tool_use`/`tool_result` 块、`content_block_delta` SSE。
- `formats/ollama/ollama.go` —— Ollama 原生格式，base64 图片 + newline JSON streaming。

（2026-07-29 从 `AGENTS.md` §5.4 搬来。）

## KV 缓存：策略跟着 provider 走

三家的机制不一样，所以这件事不能有一个统一实现：

| Provider | 机制 | 我们这边怎么做 |
|---|---|---|
| Anthropic | 显式 `cache_control: {"type":"ephemeral"}` | system 消息自动追加（`formats/anthropic/anthropic.go:128`） |
| OpenAI | `prompt-cache-key` 请求头 | `ChatRequest.CacheKey` → HTTP header |
| DeepSeek | 全自动磁盘缓存 | **零代码** —— 只要前缀一致就命中 |
| Gemini | 独立的 cache API | 远期，没做 |

⚠️ **前缀顺序与注入防护是一对真取舍，别以为能两全**（2026-07-29 澄清）。缓存想要「静态内容全在前面」，而现在的组装顺序是 `L1 → L3 → L3extra → L2 → reminder`（`handlers_ai_companion.go:218`）—— **reminder 故意放在最后**，这样 L2 里的「忽略前面的指令」压不掉硬边界。代价是可缓存前缀只到 L1 为止：它后面就是逐请求变化的 L3。

把 reminder 挪到 L1 后面能把静态前缀拉长，但会丢掉「硬边界是最后一句话」这个性质。**当前选的是后者**，因为注入防护比省 token 贵。

（这张表 2026-07-29 从已删除的 `plan.md` §2.3 搬来。那份文档自己 §2.1 与 §2.3 对 reminder 位置的说法是矛盾的，以代码为准。）
