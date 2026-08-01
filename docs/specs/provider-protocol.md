# 外部能力适配协议（天气 / 搜索 / 消息通道，v1）

> 状态：**协议已定案、未实现**。落地是批次 F2。
>
> 给**写适配器的人**看的，包括不是本项目成员的人。传输层规则在 [transport.md](transport.md)，此处只写各能力自己的握手与操作。

## 目标

外部能力不再靠「改 Go 代码 / 往仓库里加插件」接入，而是**写一个适配层**：可以独立部署、可以放在别的机器、可以并发。

内置实现与外部适配层走同一个接口，上层完全看不出区别 —— `internal/weather` 的 `WeatherProvider`、`internal/search` 的 `Searcher`、`internal/channels` 的 `Channel` 一行都不用改。

| 能力 | 内置实现 | 外部适配层 |
|---|---|---|
| 天气 | openmeteo / qweather / openweathermap / wttrin（**已有四个**） | `format: http` 或 `exec` |
| 搜索 | tavily（有 key）+ DuckDuckGo（免费兜底） | `format: http` 或 `exec` |
| 通道 | napcat/OneBot（**已有**）+ 后续常见可接 bot 的平台 | `format: ws`、`http` 或 `exec` |

## 查询型（天气、搜索）

**无状态请求／响应，天然可并发。** 用 HTTP 或 `exec`，不要用 WebSocket —— 没有需要保持的状态，长连接只是负担。

```
GET  /v0/manifest
  200 {
    "name": "my-weather",
    "displayName": "我的天气源",
    "logo": "data:image/svg+xml;base64,…",     // 控制台与前端直接渲染
    "type": "query",
    "capabilities": ["weather"],               // weather | search
    "description": {                           // 双 locale 是启动期硬校验
      "zh-CN": "覆盖东亚，逐小时，数据来自 …",
      "en-US": "East Asia, hourly, sourced from …"
    }
  }
```

```
POST /v0/weather
  {"location":"北京","lat":39.9,"lon":116.4,"date":"2026-07-29","days":3,"tz":"Asia/Shanghai","locale":"zh-CN"}
  200 {"location":"北京","days":[
        {"date":"2026-07-29","code":3,"text":"阴","tempMin":22,"tempMax":31,"precipProb":40}, …]}
```

```
POST /v0/search
  {"query":"…","limit":5,"locale":"zh-CN"}
  200 {"results":[{"title":"…","url":"…","snippet":"…"}]}
```

- **`text` 由适配层本地化**（按请求的 `locale`），因为只有它知道上游支持哪些语言。上游没有该语言就退回它自己的默认 —— 语言不对的天气预报仍然告诉你会下雨。
- `code` 是 WMO 天气码，没有就填 `-1`。
- **不可用**：**不走后端的有序 fallback 链** —— 查询型能力注册成 agent 工具，源是工具的一个参数，enum 是当下可用的源，模型自己选（详见 [transport.md](transport.md#通用的失败语义)，与本文档下面「`manifest.description` 会进提示词」一节是同一条）。一个源内部的重试是那个源自己的事。
- **缓存在 Daycore 侧，适配层不要自己再缓存**，否则「为什么数据是旧的」会说不清。天气是 30 分钟（`internal/weather`）。

### `manifest.description` 会进提示词

它被 `companion_agent.tmpl` 用 `{{range .Searches}}` 动态列出，让模型自己按当前问题选搜索源（与共识 5「不设关键词硬规则」同构）。所以：

- **双 locale 必填**，缺一在启动时报错。
- 长度有上限（4 KiB），换行会被去掉。
- ⚠️ **这是客户端提供、要进 LLM 的文本。** 与前端 manifest 的 `theme.rules` 同一类问题：未经运维批准的描述**不进提示词**，后端按 `name`/`capabilities` 机械生成一句。批准是「我读过这段文字」的显式动作。控制台在每个 provider 卡片旁给「编辑描述」入口。

## 通道型（QQ / napcat / 其它可接 bot 的平台）

**双向、有状态、要身份 → WebSocket。** 只在真收发消息时才有流量，且不需要后端暴露入站端点。

### 连接方向两种都要支持

取决于谁在 NAT 后面 —— **协议不该替部署做决定**：

| `mode` | 谁连谁 | 用在 |
|---|---|---|
| `dial` | 后端主动连出去，适配层当 server | 现有 onebot 就是这种（正向 WS） |
| `listen` | 后端开 WS 端点等适配层连进来 | 适配层在内网时 |

OneBot 11 标准本身有三种接入方式，现有实现只覆盖第一种，**三种都要补**：`dial`＝正向 WS（✅ 已有）／`listen`＝反向 WS（⬜）／`http`＝HTTP 上报 + API 调用（⬜）。

`internal/channels` 的 `Channel` 接口（`Send(externalID, msg)` + `Start(inbound chan<-)` + `Stop()`）对三种模式**都适用** —— `Start` 里是拨出去还是挂个 handler 等连接，纯属实现细节。**零接口变更。**

### 帧

```
后端 →   {"t":"hello","protocol":1}
适配层 → {"t":"manifest","name":"qq","displayName":"QQ","logo":"data:…",
          "features":{"attachments":true,"markdown":false}}
适配层 → {"t":"inbound","externalUserId":"12345","externalName":"小明","avatar":"…",
          "messageId":"m-1","text":"帮我把复习挪到晚上","attachments":[],"ts":1785000000000}
后端 →   {"t":"send","to":"12345","text":"挪好了","replyTo":"m-1"}
后端 →   {"t":"ping"}   适配层 → {"t":"pong"}
```

- **`manifest` 自带 `displayName` 与 `logo`** —— 控制台与前端直接渲染，**接一个新通道零前端改动**。
- `features` 是能力声明，不是建议：`markdown:false` 的通道，后端不会发 markdown。
- 断线重连沿用 onebot 现有逻辑（指数退避）。
- 入站仍复用 `internal/server/channel_agent.go` 的 `HandleInbound`（已实现「解析绑定 → 跑 agent loop → 回推」）。

## 搜索的第三层来源：模型厂商自带搜索

**形状特殊，不要混进 `Searcher`。** 两种，配置入口不同：

| 形状 | 例 | 配置放哪 | 为什么 |
|---|---|---|---|
| **返回独立结果**（server tool 规范） | DeepSeek 走 Anthropic 的搜索接口 | **搜索配置** | 要注册工具、解析结果、映射成 `SearchResult` |
| **直接融进回答**（无结构化返回） | 部分厂商的内置检索 | **模型配置** | 它本质是一段随模型走的提示词片段 |

✅ **2026-07-29 已删掉**（`ai.Capabilities.DeepseekSearch` + `models.yaml` 里两处 `deepseek_search: true`）。它声明了但没有任何调用路径读它：没有 format 查它、没有工具从它注册，而默认配置里还给两个模型设成 `true`。**一个宣称了又没实现的能力比缺一个能力更糟 —— 它是个承诺。** 厂商原生搜索由 F2 重新引入，届时按上表两种形状之一落地。（`yaml.Unmarshal` 非严格，所以已有配置里残留这个键只会被忽略。）

## 两种传输

规则在 [transport.md](transport.md)：HTTP 状态码语义、超时与重试、子进程握手与生命周期、`providers.yaml` 形状。以下是本协议特有的。

⚠️ **路径前缀 2026-07-30 从 `/v1/` 改成 `/v0/`。** `transport.md` 是四种适配层共用的规范，它在共用的 HTTP 一章里规定路径统一带 `/v0/`；这份分册当时写的是 `/v1/`。共用规范与分册打架，正是 transport.md 建立起来要消灭的那类漂移 —— **共用的那份赢**。（版本号本身仍然各走各的：适配协议的版本在 `manifest.protocol` 里，与路径前缀是两件事。）

### `format: exec` 的握手行

适配层在握手那一行里**追加 `manifest`**，这样后端在建连接之前就知道它是谁、能干什么，控制台也能在它还没就绪时先把名字和 logo 显示出来：

```json
{"protocol":"1","transport":"unix","address":"/tmp/daycore-weather-3b1f.sock","pid":9001,
 "manifest":{"name":"my-weather","displayName":"我的天气源","type":"query","capabilities":["weather"]}}
```

`DAYCORE_ADAPTER_KIND` 为 `weather` / `search` / `channel`。

**通道型用 `exec` 时**，WS 的方向仍由 `mode` 决定，但地址是握手行给的 socket：`mode: dial` 时后端连 `address`；`mode: listen` 时子进程连后端在环境变量 `DAYCORE_INBOUND_WS` 里给出的地址。

### `format: http` 的额外要求

- 查询型是**冷路径**（天气 30 分钟缓存一次、搜索按需），所以不要求 keep-alive，但要能承受被并发调用。
- `GET /v0/manifest` 必须便宜，它同时是健康检查。
- **不要在适配层里重试**。后端知道 deadline 与预算，适配层的重试只会把一次超时变成三次。

### `format: ws`（仅通道型）

通用 WS 适配层，帧同上。`mode: dial` 时适配层是 server、后端是 client；`mode: listen` 反之。

## 与提示词的关系

提示词片段**跟着配置走**：一个搜索源该怎么描述给模型、一个通道有什么特性（QQ 不支持 markdown、回复要短），是那个 provider 自己的属性，编辑入口就该在它的配置卡片旁边，而不是让人去 Prompt 页翻一个巨大模板。

基建零新增：`internal/ai/prompts.go` 是 `text/template` + `Render(ctx, key, locale, data)`，`{{if}}`/`{{range}}` 天然可用；`PromptService.Validate` 能 parse 校验，控制台保存时挡住语法错；`prompt_overrides` 的「文件种子 + DB 覆盖 + 立即生效」直接复用。

⚠️ `companion_agent.tmpl` 目前是**零插值**的纯规则清单（有意为之）。引入 `{{range}}` 是对它的第一次结构性改动，**两个 locale 必须同批改**。

## 未定的口子

- **附件**：`inbound.attachments` 的形状（内联 base64 还是 URL）未定。
- **通道的富消息**：卡片、按钮、快速回复要不要进协议未定。现在只有 text。
- **查询型的批量**：一次问多个城市的天气未定。
- **认证**：`token_env` 只设想 bearer。
