# 配置分层

> 实时文档：改 `internal/config/` 必须同批更新本文件。**下面的表是生成的** —— 源是 `internal/config/layer.go` 的 `Settings`，跑 `make config-doc` 重生成；`go test ./internal/config/` 会因它过期而红。

## 这份文档回答的唯一问题

**每一个旋钮，能不能在进程跑着的时候改。**

答案既不能从类型看出来也不能从名字看出来：`AGENT_MAX_ROUNDS` 每个请求都读一次、随时可以变；`DB_DSN` 决定了这个进程打开的是哪个数据库。两个都是同一个 struct 里的字符串。

不写下来的后果不是「有点乱」，是**控制台把一个改不动的东西做成了可改的**：用户在网页上关掉某个开关、保存成功、而进程继续按老值跑 —— 「我明明关了它还在做」。这是配置界面最贵的一种 bug，因为它看起来像功能正常。

## 两层

| | 判据 | 控制台 |
|---|---|---|
| **启动期** | 进程读了一次，并**用它造出了别的东西**：一个 socket、一个数据库句柄、一把签名密钥、一份解析过的配置文件。给新值改不了已经造出来的那个东西。 | **只读展示，永不提供编辑** |
| **运行时** | 每次用的时候现读。没有任何东西是用它造出来的，所以新值下次读就生效。 | 可改 |

**密钥一律启动期，无论它怎么被读。** 这是两条不同的理由，而只有一条跟重启有关：一个能显示签名密钥的控制台已经把它弄丢了，一个能改它的控制台会用一次手滑换掉所有人的会话。所以 `Editable()` 是独立的谓词，不是「层 == 运行时」的同义词 —— 将来「启动期但可改（提示重启）」落地时，这条不会跟着被一起放开。

## 闸门

`TestEveryConfigFieldIsClassified`：**`Config` 上任何一个没分类的字段直接红**，反向也查（分类了但字段没了 = 控制台会渲染一个什么都不做的旋钮）。

这样「能不能热改」这个问题由**加这个字段的人在他有上下文的那一刻回答一次**，而不是由写控制台那天的人回答四十次 —— 而错的那些答案正是危险的那些。

另有两条：密钥不许是运行时也不许可编辑（并逐个点名 `JWT_SECRET`/`COOKIE_SECRET`/`PASSWORD_PEPPER`/`ADMIN_TOKEN`/`DB_DSN`，防止将来重构 `Settings` 时悄悄漏掉一个）；**分类反直觉的必须写理由**（每请求都读却标启动期的那几个 —— 那是有意的安全决定，没有理由的话下一个人会把它当疏漏「修掉」）。

<!-- BEGIN GENERATED CONFIG TABLE -->

### 启动期（只走环境变量，控制台只读）

| 环境变量 | Config 字段 | 密钥 | 为什么 / 注意 |
|---|---|---|---|
| `APP_ENV` | `Env` |  | decides secret handling and cookie defaults at construction time |
| `HOST` | `Host` |  | — |
| `PORT` | `Port` |  | — |
| `STATIC_DIR` | `StaticDir` |  | the static handler is mounted once, and only if the directory exists |
| `DATA_DIR` | `DataDir` |  | the blob driver opened it and holds it |
| `BLOB_STORE` | `BlobStore` |  | selects a driver; the Store is constructed once |
| `PROMPTS_DIR` | `PromptsDir` |  | templates and the hard boundaries are overlaid at startup |
| `THEME_KINDS_DIR` | `ThemeKindsDir` |  | 主题取值 kind 的文件层，启动时读一次合进注册表；改完要重启。加一种 kind 是丢一个 JSON 文件，不是发一次版。第三档（DB 层）可以热改，但只重建 DB 那一层 —— 见 docs/specs/frontend-manifest.md |
| `LOCALES_DIR` | `LocalesDir` |  | language packs are loaded into the catalog at startup |
| `MODELS_CONFIG` | `ModelsConfigPath` |  | the catalog is parsed once into provider instances |
| `OAUTH_CONFIG` | `OAuthConfigPath` |  | providers are constructed once, with their redirect URIs baked in |
| `PROVIDERS_CONFIG` | `ProvidersConfigPath` |  | every entry becomes an HTTP client or a registered factory at startup. The console CAN change enabled/description/approved per source — those live in provider_overrides, not here, because they are read fresh at each use. base_url deliberately cannot: it is the SSRF entrance, and requiring shell access to change it is the whole defence |
| `DB_TYPE` | `DBType` |  | the handle is open |
| `DB_DSN` | `DBDSN` | 🔑 | the handle is open, and a DSN carries a password |
| `JWT_SECRET` | `JWTSecret` | 🔑 | — |
| `COOKIE_SECRET` | `CookieSecret` | 🔑 | — |
| `PASSWORD_PEPPER` | `Pepper` | 🔑 | changing it invalidates every stored password hash |
| `ADMIN_TOKEN` | `AdminToken` | 🔑 | the credential the console itself authenticates with; editable from the console means editable by whoever already got in |
| `JWT_TTL` | `JWTTTL` |  | security-shaped; a console that can set it to a year is a console that can mint a permanent token |
| `SECURE_COOKIES` | `SecureCookies` |  | a security boundary; turning it off from the web is how it gets turned off |
| `COOKIE_SAMESITE` | `CookieSameSite` |  | same |
| `ALLOWED_ORIGINS` | `AllowedOrigins` |  | a security boundary. It is read per request and COULD be hot, which is exactly why it is listed here with a reason: editable CORS from the console means one compromised console opens the API to any origin |
| `TRUST_PROXY_HEADERS` | `TrustProxyHeaders` |  | wrongly true lets any client forge its IP and escape rate limiting |
| `PUBLIC_BASE_URL` | `PublicBaseURL` |  | OAuth redirect URIs are built from it at load and registered with the provider |
| `TAVILY_API_KEY` | `TavilyKey` | 🔑 | the engine is built once. It only became a Config field in F2-A — before that internal/search read it straight from the environment, which meant this table, and the gate that walks Config's fields, could not see it at all |
| `QWEATHER_API_KEY` | `QWeatherKey` | 🔑 | — |
| `OPENWEATHERMAP_API_KEY` | `OpenWeatherMapKey` | 🔑 | — |
| `ONEBOT_TOKEN` | `OneBotToken` | 🔑 | — |
| `ONEBOT_WS_URL` | `OneBotWSURL` |  | the websocket is dialled once at startup. F3 makes channels reconfigurable; until the reconnect path exists, calling this runtime would be a lie |

### 运行时（可热改；环境变量是种子）

| 环境变量 | Config 字段 | 密钥 | 为什么 / 注意 |
|---|---|---|---|
| `DEFAULT_CHAT_MODEL` | `DefaultChatModel` |  | ⚠️ runtime by nature — it only selects among already-constructed catalog entries — but NOT hot yet: LoadCatalog bakes the choice in at startup. Making it hot means a setter on the Catalog. Classified by nature rather than by today's wiring, because the console needs to know which it is |
| `DEFAULT_VISION_MODEL` | `DefaultVisionModel` |  | same |
| `DEFAULT_PLANNER_MODEL` | `DefaultPlannerModel` |  | same |
| `WEATHER_PROVIDER` | `WeatherProvider` |  | the id a lookup with no source prefers. NOT a fallback chain — F2-A deleted that, and its removal is the point: the old chain answered from a different source under the primary's name, silently, with the primary's error discarded. Genuinely hot now: Sources.SetDefault is the setter and ReloadSettings calls it |
| `AI_REQUEST_TIMEOUT` | `AIRequestTimeout` |  | — |
| `AI_RATE_LIMIT_PER_MIN` | `RateLimitPerMin` |  | ⚠️ the limiter is constructed in New with this value baked in — runtime by nature, NOT hot yet. Making it so means a setter on rateLimiter |
| `AUTH_RATE_LIMIT_PER_MIN` | `AuthRateLimitPerMin` |  | same |
| `AGENT_MAX_ROUNDS` | `AgentMaxRounds` |  | — |
| `MAX_IMAGE_BYTES` | `MaxImageBytes` |  | — |
| `MAX_UPLOAD_BYTES` | `MaxUploadBytes` |  | — |
| `AUTO_PLAN_MAX_DAYS` | `AutoPlanMaxDays` |  | — |
| `ASSIGNMENT_LOOKAHEAD_DAYS` | `AssignmentLookaheadDays` |  | — |
| `WORKER_DEFAULT_TZ` | `WorkerDefaultTZ` |  | only the fallback for a session with no timezone of its own (ζ-4); changing it does not move anybody who has one |
| `DEFAULT_PRIMARY_LOCALE` | `DefaultLocales` |  | the default pair for NEW users; a user's own choice is stored per session |
| `DEFAULT_SECONDARY_LOCALE` | `DefaultLocales` |  | same |
<!-- END GENERATED CONFIG TABLE -->

## `settings` 覆盖表（θ-F4b 落地）

运行时那一层的新值存在 `settings` 表里，环境变量降级为**种子**：没有行就用环境变量，有行就以行为准。域实体 [`internal/domain/setting.go`](../internal/domain/setting.go)，四个后端各一份实现，行为由 `storagetest` 的 `Setting/OverrideRoundTripAndReset` 守。

F1 当时**有意没建这张表** —— 它唯一的消费者是控制台端点，而本仓已经交付过六个「写完、测过、没人调用」的机制。F4b 是它第一次有消费者，两件事同批落地。

### 快照，不是每次查库

进程持有一份 `atomic.Pointer[config.Config]`：`Server.runtime()` 读它，`ReloadSettings` 在启动时和每次 PUT 之后原子换掉整份。

- **取舍**：换掉的是整份配置而不是逐字段，所以一次请求里两次 `s.runtime()` 可能拿到不同的两份快照。接受这个 —— 替代方案是给每个读者传一份快照，那要改一百多个签名，换来的是一个没人会注意到的一致性。
- **它防的是**：每次读一个阈值就查一次库。companion 组一次提示词读八次配置。
- **边界**：`s.cfg` 是**启动期原件**，`Apply` 返回副本、绝不改它 —— 它是「重置」要还原到的东西，也是所有启动期构造物共享的那一份。运行时字段一律走 `s.runtime()`，由 [`config_snapshot_test.go`](../internal/server/config_snapshot_test.go) 的 AST 闸门守着；删掉一处 `runtime()` 改回 `s.cfg` 会红。

### 端点：`GET·PUT /api/admin/config`

- **密钥永不返回** —— 不是值、不是前缀、也不是长度。打码的密钥仍然告诉攻击者它有多长、两次读之间变没变，而这两样控制台都不需要。密钥只回 `set: true|false`。
- **启动期项只读**：写它会被 400 `not_editable` 拒掉，而不是存下来。**一行控制台显示、进程忽略的配置是这里能出的最坏结果**（「我明明关掉了它还在跑」），所以宁可拒绝。
- **整份请求先校验完再写**。控制台没有事务可用，写一半会让运维手里剩下一部分改动而无从分辨是哪部分。
- **`null` 重置到环境种子，且与 `""` 是两个不同的请求** —— 对字符串型的旋钮，「恢复默认」和「设成空」不是一回事，只有其中一个的控制台撤销不了一次误操作。
- **`requiresRestart` 按 key 报**，不是一个布尔。运维接下来要问的正是「我改的哪一条还在等」。
- **降级模式下 GET 可用、PUT 返回 503**。存储没起来时控制台正是运维用来看哪里坏了的地方，配置页必须能读。

### 按性质分类，不按今天的接线

`notHotYet`（[handlers_admin_config.go](../internal/server/handlers_admin_config.go)）列出那些**按性质是运行时、但今天的值在构造时就被烘进去**的旋钮：两个限流器（`New` 里构造）、三个默认模型（`LoadCatalog` 解析一次）、`WEATHER_PROVIDER`（`weather.New` 建链一次）。

它们没有被标成启动期，因为控制台需要知道的是哪一种；标错会让「将来给它加 setter」从待办变成一条错误的事实。存覆盖是接受的、也是对的，只是要重启才生效 —— 端点如实说出这件事，这是一个诚实的控制台和一个安静撒谎的控制台之间的差别。这张名单随每个 setter 落地而变短；它是写死的而不是推导的，因为「这个值有没有在启动时被复制走」从类型上看不出来。

### 坏行不能连累其他行

`Apply` 逐 key 收集问题而不是整体失败：一条读不动的行（旧版本留下的、或手写的 INSERT）不该让运维丢掉其余全部覆盖，启动时尤其不该。`Apply` 内部会**再查一次** `Overridable` —— 端点已经查过了，这里仍然查，因为一行可能早于一次重分类，也可能是绕过端点直接写进库的。

## 相关

- 版本三层（构建版本 / API 契约版本 / 各前端版本）见 [`internal/version/version.go`](../internal/version/version.go) 的包注释与 [ROADMAP.md](ROADMAP.md)。
- 提示词与 L1 硬边界的分层（DB → `PROMPTS_DIR` → 内嵌，以及边界那一份**没有 DB 层**）见 [AI.md](AI.md)。
- 语言包三层（DB → `LOCALES_DIR` → 内嵌）见 [DATA.md](DATA.md)。
