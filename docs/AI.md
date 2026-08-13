# AI 子系统

> 实时文档：改 provider/prompts/vision 必须同批更新本文件。最后全面核对：2026-08-02。

## Provider 与 Catalog（internal/ai/）

- `AIProvider` 接口 + `ChatRequest{Messages, Tools, Temperature, MaxTokens, JSONMode, Stop, CacheKey}` + Capabilities（provider.go）。
- formats/：openai（最完整，流式 tool_calls + 夹具测试）、anthropic（text-only 流式）、ollama（NDJSON）。
- Catalog（models.go，config/models.yaml）：`DefaultChat()` / `Vision()` / `Planner()`；模型条目支持 extra_body/max_tokens。
  ⚠️ **推理不是一个 caps 布尔**：它由 `extra_body`（如 `thinking: {enabled: true}`）打开、由 `Usage.ReasoningTokens` 读回。曾经有个 `Capabilities.Thinking` 立在旁边、零读者，2026-08-12 与 `Capabilities.Stream` 一并删除。
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

`ContentPart{Type, Text, MIME, Data, URL, FileID, Name}` 是消息里的一段；`ChatResponse.Parts` 装产物，`ChatResponse.Usage` 装用量（含 `CachedTokens` / `ReasoningTokens`）。三个格式的 Chat 与 ChatStream 都解析 usage（流式经 `ai.Chunk.Usage` 末帧 → `StreamAccumulator` 汇总；openai 发 `stream_options.include_usage`，anthropic 拼 message_start/message_delta，ollama 取末帧计数）。server 侧 `s.logAICall`（best-effort）把**每一次模型调用**落进 `ai_call_logs`：10 个非流式端点 + companion 每轮一条。

⚠️ **有一条不进用户的用量计数：`theme_backfill`**（`logAICallNotBilledToTheAccount`，2026-08-11）。账本行照样带 `session_id` —— 那是「这笔钱买了什么」，是花销与它买到的那套主题之间唯一的连接；但 `sessions.usage_*` 回答的是「**这个账号在干什么**」，控制台拿它找谁在猛打 API，三小时的窗口正是为了反映行为。补算是运维按的按钮，不是账号做的事：一个两千套主题的 family 会同时把两千个账号的三小时窗口顶起来，然后去查原因的人看到的是两千个无辜用户，而不是那次部署。`endpoint` 列（`theme_backfill` 对 `theme_gen`）是账本区分两者的方式。Catalog 另有四个能力选择器（`Transcriber()`/`SpeechSynthesizer()`/`ImageGenerator()`/`Embedder()`，断言遍历目录首个命中），`GET /api/version` 的 `features` 字段由它们派生——端侧据此发现本部署能力，不硬编码假设。

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
- key 清单（2026-08-11 更新）：auto_plan、companion_agent（L1 纯规则清单零人格）、companion_context、day_plan_text、day_plan_image、mood、schedule_extract_image、theme_gen、**theme_backfill**、inbox_classify（随手记录归类器，Categories 注入启用类别清单）、food_recognize（拍照→diet 营养估计，走 vision）、travel_suggest（目的地→行程 JSON）、persona、brief、replan。

  ⚠️ **`theme_backfill` 是独立的一个 key，不是 `theme_gen` 的一个分支**。两者要的东西相反：`theme_gen` 从一句描述设计一整套配色，`theme_backfill` 往一套**用户已经选定、一个字都不许动**的配色里补几个值。合成一个模板，就是把「全部保留」和「全部重做」写进同一段字然后让模型自己挑。

  ⚠️ **`theme_gen` 与 `theme_backfill` 的可用变量清单不是常量**，它按调用方前端的 family 生成（名字 + kind + 该 kind 允许的形状，形状**从 kind 注册表问出来**）。此前它写死 13 条颜色 token 却按调用方的 family 过滤输出 —— 声明了别的 token 的前端，被要求产出它没有的变量，然后产出的每一条都被丢掉。`{{if .Builtin}}` 保留设计系统自己那套规则给不握手的现役前端。

  ⚠️ **前端自带的 `rules` 只有运维批准后才进模型**（`FamilyRules` 在未批准时是空串，模板转而按 token 清单机械生成规则）。与此配套：**token 的 `kind` 表达式本身也进提示词**，所以它同样有界 —— 见 `docs/specs/frontend-manifest.md`。
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

#### 这一块的全部边界（不要在不知道理由的情况下改动其中任何一条）

| 约束 | 它防的是什么 |
|---|---|
| **不要给它加 DB 覆盖层** | 那等于把删除边界的能力交给任何拿到 admin token 的人，且删掉的东西随**下一个请求**生效 |
| **不要给它加 admin 端点**（`RouteTable` 里出现 `boundar` 直接红） | 同上。这个改动看起来非常合理，正是 review 会挥手放过的形状 |
| **不认识的字段 = 启动失败**（`DisallowUnknownFields`） | `rules` 拼错会**静默变成零条规则**。这个文件最不能出的就是这种错，而「块看起来是完整的」时没有任何下游会注意到 |
| **`heading`/`closing` 为空、`rules` 为空 = 拒绝** | 空块更可能是文件写坏/写了一半，不是「我想删光边界」。真要削到只剩一条就写一条 —— 那是明确的意思表示 |
| **磁盘文件坏了 = 启动失败，不回退内嵌** | 运维是特意放那个文件的。无视它悄悄跑内嵌，是最安静的一种忽略安全改动 |
| **失败的加载不许半应用** | 第二个 locale 写坏时第一个不能已经生效。实现上是 parse 全过再改内存，测试用「重构形状的 bug」验过会红 |
| **`HardBoundaryReminder` 解析失败时 panic，不返回空串** | 空串 = 进程带着「没有边界」对外服务且毫无痕迹。这是唯一值得崩的失败。启动期已 `StdBoundaries()` 过一遍，所以这条路径在跑起来的服务里不可达 |
| **locale 落空时退到同语言其它区域 → `i18n.Default`，绝不返回空** | 读者是模型不是人。用另一门语言的边界远好于没有边界；模型照做不需要语言匹配 |
| **`_readme` 写在文件里** | 运维手上那份要能自己解释自己 —— 他不会同时打开仓库文档 |

**取舍：`daycore install` 会把这个文件解出来。** 好处是它可发现、可编辑（否则运维根本不知道有这么个东西、也没有起手拷贝）。代价是**装完那天起，上游新增的规则就到不了他手上了** —— 与提示词模板同一个已被接受的取舍，但这里更贵，因为漏掉的可能是一条新的安全规则。缓解手段是 `version`：磁盘那份比内嵌小就在启动日志里 warn，`BoundaryLoad.Stale()` 是这件事**唯一**会被说出来的地方。

**升 `version` 的判据**：内嵌的 `rules` 增删改了就升。只改措辞不升 —— 否则每个运维都会收到一条与他无关的 warn，而狼来了几次之后这行日志就没人看了。

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

## 附件进模型（`internal/server/attachments.go`，ε 批次）

`POST /api/ai/companion` 与 `/async` 收 `attachmentIds`（来自 `POST /api/files`），解析成 `ai.ContentPart` 挂到**最后一条 user 消息**上。

- **只用内联 base64**（`CarriageInline`）—— 三个格式都接受的唯一载运形式。签名 URL 需要一套文件总线还没有的签名机制，`CarriageFileID` 要先把字节传给厂商；两条都是后加的，现在猜错的代价是「模型静默无视用户的文件」。
- **模型读不了的附件不静默丢弃**，而是变成一条文本 part 把文件名念出来（`prompt.attachments.unreadable`，双 locale）。丢掉的话会得到最坏的一种回答：对一份没人读过的文档给出自信的答复。判据是 `Capabilities.Accepts(modality)` + `CarriagesFor(provider, modality).Has(inline)`。
- **挂载点从后往前找 role=user**，不是取最后一条：滑窗压缩器会在 user 之后补一条摘要，把图挂上去就等于当成 system 消息发出去了。
- 内联上限走 `MAX_IMAGE_BYTES` 而不是 `MAX_UPLOAD_BYTES` —— 一份 30 MiB 的 PDF 是个正常上传、是个糟糕的提示词。
- ⚠️ **通道入站附件（OneBot 图片/语音）还没接** —— 那条路径现在仍然只有文字。

**边界与取舍**：

| 约束 | 理由 / 它防的是什么 |
|---|---|
| **只用 `CarriageInline`**，不给 URL、不给 `file_id` | URL 要签名机制（文件总线还没有），`file_id` 要先把字节传给厂商。两条都是后加的；现在猜错的代价是「模型静默无视用户的文件」 |
| **读不了的不丢弃，变成文本 part** | 丢掉会得到最坏的一种回答：对一份没人读过的文档给出自信的答复 |
| 判据是 `Capabilities.Accepts(modality)` **且** `CarriagesFor(provider, modality).Has(inline)` | 两个条件是不同的东西：前者是模型能不能读（`models.yaml` 声明），后者是我们的 format 会不会序列化。少查后者就会把图发给一个把它丢掉的 format |
| **未知 MIME（`kind = file`）一律走「描述」而不是发送** | 声称一个模态会让 format 序列化成厂商会拒的形状 |
| 挂载点**从后往前找 `role=user`** | 压缩器会在 user 之后补一条摘要；挂上去等于当成 system 消息发出去，有的厂商直接拒，有的静默无视 |
| 一条消息都没有 user 时**什么也不做** | 不许为了挂附件而凭空造一条 user 消息 |
| 描述文案走 `i18n.Tf`（`prompt.attachments.unreadable`，双 locale） | 读者虽是模型，但它会照着回答的语言走；中文句子出现在英文对话里是往错方向推 |
| 内联上限用 `MAX_IMAGE_BYTES` 而不是 `MAX_UPLOAD_BYTES` | 见 ARCHITECTURE.md「两个上限是两个问题」 |

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


## 外部源描述进提示词（F2-A，2026-08-09）

可用的外部能力源（天气 / 搜索）连同各自一句描述，作为 **L3 上下文**的一节渲染给模型。

### 为什么是 L3 不是 L1

`docs/specs/provider-protocol.md` 原本写的是 `companion_agent.tmpl`（L1）。**那是错的家**，而那个文件自己就说明了理由：L1 是**纯规则清单、零插值**，有意为之，因为它是边界块。

- **会变的规则不是规则。** 一个源变得不健康就少一行 —— 那是数据不是规矩。
- **每轮重渲染的边界块没人能据以推理。** L1 的稳定性正是它能被缓存、也能被审计的原因。

L3 本来就是每轮的数据块、本来就带 `WeatherSummary`、本来就每轮不同。放这里**没有把任何原本稳定的东西弄不稳定**。

### 三条边界

1. **每一条都过了批准门。** `Source.PromptDescription` 只在描述被**按那段文字的哈希**批准过时才返回运维的话，否则返回一句由 id 和 format 机械生成的。适配层自报的 `manifestDescription` **到不了这个函数** —— 见 `internal/adapters/source.go` 与两道结构性闸门。
2. **与工具带的 enum 同源、同一轮取一次。** 描述一个模型调不到的源比不描述更糟：它引诱一次必定失败的调用，白花一个来回。
3. **只有一个源时整节不出现。** 那时工具连 `source` 参数都没有（`companionToolDefs` 不给单元素 enum），一句解释「没人能做的选择」的话是每次对话每一轮都在花的 token。

### 双 locale 同批，且有断言

`{{range}}` 只加进一个 locale 是「半数用户静默拿不到」的经典形状 —— 而所有 Go 侧的测试对一个**根本没有 `{{range}}` 的模板**也会全过。`TestSourceBlockRendersInBothLocales` 是唯一能抓到这件事的断言：它真的渲染，两个 locale 都查。
