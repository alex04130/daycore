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
| `LOCALES_DIR` | `LocalesDir` |  | language packs are loaded into the catalog at startup |
| `MODELS_CONFIG` | `ModelsConfigPath` |  | the catalog is parsed once into provider instances |
| `OAUTH_CONFIG` | `OAuthConfigPath` |  | providers are constructed once, with their redirect URIs baked in |
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
| `QWEATHER_API_KEY` | `QWeatherKey` | 🔑 | — |
| `OPENWEATHERMAP_API_KEY` | `OpenWeatherMapKey` | 🔑 | — |
| `ONEBOT_TOKEN` | `OneBotToken` | 🔑 | — |
| `ONEBOT_WS_URL` | `OneBotWSURL` |  | the websocket is dialled once at startup. F3 makes channels reconfigurable; until the reconnect path exists, calling this runtime would be a lie |

### 运行时（可热改；环境变量是种子）

| 环境变量 | Config 字段 | 密钥 | 为什么 / 注意 |
|---|---|---|---|
| `DEFAULT_CHAT_MODEL` | `DefaultChatModel` |  | selects among already-constructed catalog entries |
| `DEFAULT_VISION_MODEL` | `DefaultVisionModel` |  | same |
| `DEFAULT_PLANNER_MODEL` | `DefaultPlannerModel` |  | same |
| `WEATHER_PROVIDER` | `WeatherProvider` |  | the provider chain is rebuilt per lookup |
| `AI_REQUEST_TIMEOUT` | `AIRequestTimeout` |  | — |
| `AI_RATE_LIMIT_PER_MIN` | `RateLimitPerMin` |  | ⚠️ the limiter is constructed in New with this value baked in — it is runtime by nature and NOT yet hot. Making it so means giving rateLimiter a setter, which is F4b's job |
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

## 还没做的：`settings` 覆盖表

ROADMAP 的 F1 要求运行时那一层迁进一张 `settings` 表，环境变量降级为**种子**，DB 有值以 DB 为准。**有意还没建**，理由与本仓反复说的那条一样：它唯一的消费者是控制台端点（F4b），而这个仓库已经交付过六个「写完、测过、没人调用」的机制。

**分类必须先存在** —— 表的形状设计不出来如果没有它，而上面那道闸门是在此期间维持它为真的东西。

⚠️ 表里两处诚实标注：`AI_RATE_LIMIT_PER_MIN` / `AUTH_RATE_LIMIT_PER_MIN` **按性质是运行时，但今天还不是热的**（限流器在 `New` 里就把值烘进去了）。没有标成启动期，因为控制台需要知道的是哪一种 —— 标错会让「将来要给它加 setter」这件事从待办变成一条错误的事实。

## 相关

- 版本三层（构建版本 / API 契约版本 / 各前端版本）见 [`internal/version/version.go`](../internal/version/version.go) 的包注释与 [ROADMAP.md](ROADMAP.md)。
- 提示词与 L1 硬边界的分层（DB → `PROMPTS_DIR` → 内嵌，以及边界那一份**没有 DB 层**）见 [AI.md](AI.md)。
- 语言包三层（DB → `LOCALES_DIR` → 内嵌）见 [DATA.md](DATA.md)。
