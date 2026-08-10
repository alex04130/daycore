package setup

import "daycore/internal/i18n"

// Every string this installer says, as catalog keys.
//
// # Why the CLI goes through i18n at all
//
// The rule the rest of the codebase follows — register a key, never branch on
// the locale — exists so that adding a language is dropping a JSON file rather
// than editing Go and cutting a release. That promise was false for the one
// surface a self-hoster meets first: `daycore install` was English literals, so
// a Chinese operator's very first interaction with a product whose default
// locale is zh-CN was in a language the product otherwise never uses.
//
// # Order of work, and why it is this order
//
// The mechanism landed before the new questions were written. Doing it the
// other way — write the expanded flow, then translate it — is the mistake batch
// δ already documented: the copy written in between is untranslatable by
// construction, and somebody has to go back for it. One pass costs less than
// one pass plus a sweep.
//
// # Boundary: no string here interpolates another translated string
//
// Only values — paths, numbers, provider ids. A sentence assembled from two
// translated halves cannot be reordered by a translator, and word order is
// exactly what differs between the languages this will grow into.
var (
	keyChoosePrompt = i18n.Reg("setup.choose", i18n.Text{
		"zh-CN": "选一个", "en-US": "Choose",
	})
	keyChooseOutOfRange = i18n.Reg("setup.choose.range", i18n.Text{
		"zh-CN": "请输入 1 到 %d 之间的数字", "en-US": "Enter a number between 1 and %d",
	})
	keyYesNoYesDefault = i18n.Reg("setup.yesno.yes", i18n.Text{
		"zh-CN": "[Y/n]", "en-US": "[Y/n]",
	})
	keyYesNoNoDefault = i18n.Reg("setup.yesno.no", i18n.Text{
		"zh-CN": "[y/N]", "en-US": "[y/N]",
	})

	keyBanner = i18n.Reg("setup.banner", i18n.Text{
		"zh-CN": "Daycore 安装", "en-US": "Daycore Setup",
	})
	keyBannerConfig = i18n.Reg("setup.banner.config", i18n.Text{
		"zh-CN": "Daycore 配置", "en-US": "Daycore Configuration",
	})
	KeyLocaleFallback = i18n.Reg("setup.locale.fallback", i18n.Text{
		"zh-CN": "没有 %s 的语言包，改用 %s。放一份 %s.json 到 LOCALES_DIR 就能用自己的语言。",
		"en-US": "No language pack for %s; using %s. Drop a %s.json into LOCALES_DIR to run this in your own language.",
	})

	// ── database ──
	keySecDatabase = i18n.Reg("setup.db.title", i18n.Text{
		"zh-CN": "数据库", "en-US": "Database",
	})
	keyDBEngine = i18n.Reg("setup.db.engine", i18n.Text{
		"zh-CN": "用哪个数据库？", "en-US": "Which database?",
	})
	keyDBSqlite = i18n.Reg("setup.db.sqlite", i18n.Text{
		"zh-CN": "SQLite", "en-US": "SQLite",
	})
	keyDBSqliteHint = i18n.Reg("setup.db.sqlite.hint", i18n.Text{
		"zh-CN": "单文件，零外部依赖。一台机器跑一份就用它。",
		"en-US": "One file, no external service. Right for a single instance.",
	})
	keyDBPostgres = i18n.Reg("setup.db.postgres", i18n.Text{
		"zh-CN": "PostgreSQL", "en-US": "PostgreSQL",
	})
	keyDBMySQL = i18n.Reg("setup.db.mysql", i18n.Text{
		"zh-CN": "MySQL", "en-US": "MySQL",
	})
	keyDBMongo = i18n.Reg("setup.db.mongo", i18n.Text{
		"zh-CN": "MongoDB", "en-US": "MongoDB",
	})
	keyDBServerHint = i18n.Reg("setup.db.server.hint", i18n.Text{
		"zh-CN": "要多实例、或者要用现成的备份运维时选它。",
		"en-US": "For multiple instances, or to reuse existing backup tooling.",
	})
	keyDBMongoHint = i18n.Reg("setup.db.mongo.hint", i18n.Text{
		"zh-CN": "推荐部署，但四个后端跑的是同一套行为套件。",
		"en-US": "The recommended deployment; all four back ends pass the same suite.",
	})
	keyDBDSN = i18n.Reg("setup.db.dsn", i18n.Text{
		"zh-CN": "连接串", "en-US": "Connection string",
	})
	keyDBCheck = i18n.Reg("setup.db.check", i18n.Text{
		"zh-CN": "现在连一下试试？", "en-US": "Try connecting now?",
	})
	keyDBCheckOK = i18n.Reg("setup.db.check.ok", i18n.Text{
		"zh-CN": "连上了", "en-US": "Connected",
	})
	keyDBCheckFail = i18n.Reg("setup.db.check.fail", i18n.Text{
		"zh-CN": "连不上：%s", "en-US": "Could not connect: %s",
	})
	keyDBCheckFailNote = i18n.Reg("setup.db.check.note", i18n.Text{
		"zh-CN": "先记下来继续。改好 DB_DSN 再启动即可，不用重跑安装。",
		"en-US": "Recorded anyway. Fix DB_DSN and start — you do not have to run this again.",
	})

	// ── model catalog ──
	keySecModels = i18n.Reg("setup.models.title", i18n.Text{
		"zh-CN": "模型目录", "en-US": "Model Catalog",
	})
	keyModelsIntro = i18n.Reg("setup.models.intro", i18n.Text{
		"zh-CN": "模型是数据驱动的：加模型是改 config/models.yaml，不用改代码。",
		"en-US": "The catalog is data-driven: adding a model edits config/models.yaml, not code.",
	})
	keyModelsHow = i18n.Reg("setup.models.how", i18n.Text{
		"zh-CN": "现在怎么处理模型目录？", "en-US": "What should happen to the catalog now?",
	})
	keyModelsSeed = i18n.Reg("setup.models.seed", i18n.Text{
		"zh-CN": "写一份带注释的起手目录", "en-US": "Write an annotated starter catalog",
	})
	keyModelsSeedHint = i18n.Reg("setup.models.seed.hint", i18n.Text{
		"zh-CN": "四个条目 + 说明。上游模型名会过期，装完记得核对。",
		"en-US": "Four entries with notes. Upstream names go stale — check them after install.",
	})
	keyModelsNow = i18n.Reg("setup.models.now", i18n.Text{
		"zh-CN": "现在就配一个能用的模型", "en-US": "Configure one working model now",
	})
	keyModelsNowHint = i18n.Reg("setup.models.now.hint", i18n.Text{
		"zh-CN": "问三个问题（接口地址 / 模型名 / key），写成默认对话模型。",
		"en-US": "Three questions (base URL / model / key), written as the default chat model.",
	})
	keyModelsLater = i18n.Reg("setup.models.later", i18n.Text{
		"zh-CN": "稍后再说", "en-US": "Later",
	})
	keyModelsLaterHint = i18n.Reg("setup.models.later.hint", i18n.Text{
		"zh-CN": "仍然写起手目录 —— 没有它服务器起不来。之后跑 `daycore config models`。",
		"en-US": "Still writes the starter catalog — the server will not start without one. Run `daycore config models` later.",
	})
	keyModelsBaseURL = i18n.Reg("setup.models.baseurl", i18n.Text{
		"zh-CN": "接口地址（OpenAI 兼容）", "en-US": "API base URL (OpenAI-compatible)",
	})
	keyModelsModel = i18n.Reg("setup.models.model", i18n.Text{
		"zh-CN": "模型名（上游的那个）", "en-US": "Model name (the vendor's)",
	})
	keyModelsKeyEnv = i18n.Reg("setup.models.keyenv", i18n.Text{
		"zh-CN": "API key 放在哪个环境变量里", "en-US": "Which environment variable holds the API key",
	})
	keyModelsWrote = i18n.Reg("setup.models.wrote", i18n.Text{
		"zh-CN": "config/models.yaml —— %d 个条目，默认对话模型是 %s",
		"en-US": "config/models.yaml — %d entries, default chat model %s",
	})
	keyModelsKept = i18n.Reg("setup.models.kept", i18n.Text{
		"zh-CN": "config/models.yaml 已存在，保持不动（要覆盖加 -force）",
		"en-US": "config/models.yaml exists; left alone (use -force to overwrite)",
	})
	keyModelsStale = i18n.Reg("setup.models.stale", i18n.Text{
		"zh-CN": "起手目录里的上游模型名是写下来那天为真的，厂商会下线和改名 —— 用之前对一遍。",
		"en-US": "The starter catalog's upstream names were real when written; vendors retire and rename. Check them before relying on them.",
	})

	// ── provider keys ──
	keySecKeys = i18n.Reg("setup.keys.title", i18n.Text{
		"zh-CN": "模型厂商 API Key", "en-US": "Model Provider API Keys",
	})
	keyKeysIntro = i18n.Reg("setup.keys.intro", i18n.Text{
		"zh-CN": "至少配一个，AI 功能才有东西可用。留空跳过。",
		"en-US": "At least one is needed for the AI features. Leave blank to skip.",
	})
	keyKeysOne = i18n.Reg("setup.keys.one", i18n.Text{
		"zh-CN": "%s API Key", "en-US": "%s API Key",
	})
	keyKeysNone = i18n.Reg("setup.keys.none", i18n.Text{
		"zh-CN": "一个 key 都没配 —— 服务器能起来，但所有 AI 功能会在调用时报错。",
		"en-US": "No keys configured — the server starts, but every AI feature fails when called.",
	})

	// ── weather ──
	keySecWeather = i18n.Reg("setup.weather.title", i18n.Text{
		"zh-CN": "天气", "en-US": "Weather",
	})
	keyWeatherIntro = i18n.Reg("setup.weather.intro", i18n.Text{
		"zh-CN": "默认用免费的 Open-Meteo，不需要 key。下面两个留空就是用它。",
		"en-US": "Defaults to the free Open-Meteo, no key required. Leave both blank to use it.",
	})
	keyWeatherQWeather = i18n.Reg("setup.weather.qweather", i18n.Text{
		"zh-CN": "和风天气 API Key", "en-US": "QWeather API key",
	})
	keyWeatherOWM = i18n.Reg("setup.weather.owm", i18n.Text{
		"zh-CN": "OpenWeatherMap API Key", "en-US": "OpenWeatherMap API key",
	})

	// ── oauth ──
	keySecOAuth = i18n.Reg("setup.oauth.title", i18n.Text{
		"zh-CN": "第三方登录（可选）", "en-US": "Social Login (optional)",
	})
	keyOAuthIntro = i18n.Reg("setup.oauth.intro", i18n.Text{
		"zh-CN": "Google 与 GitHub 是预设，只要 client_id / client_secret 两项。",
		"en-US": "Google and GitHub are presets: they need only client_id and client_secret.",
	})
	keyOAuthWant = i18n.Reg("setup.oauth.want", i18n.Text{
		"zh-CN": "现在配 %s 吗？", "en-US": "Configure %s now?",
	})
	keyOAuthClientID = i18n.Reg("setup.oauth.clientid", i18n.Text{
		"zh-CN": "%s client_id", "en-US": "%s client_id",
	})
	keyOAuthClientSecret = i18n.Reg("setup.oauth.secret", i18n.Text{
		"zh-CN": "%s client_secret", "en-US": "%s client_secret",
	})
	keyOAuthCallback = i18n.Reg("setup.oauth.callback", i18n.Text{
		"zh-CN": "回调地址要在厂商后台填成：%s", "en-US": "Register this callback URL with the provider: %s",
	})
	keyOAuthNeedsBase = i18n.Reg("setup.oauth.needsbase", i18n.Text{
		"zh-CN": "回调地址由 PUBLIC_BASE_URL 拼出来，现在还没有域名，所以下面按占位符显示。",
		"en-US": "The callback URL is built from PUBLIC_BASE_URL; no domain is set yet, so this shows a placeholder.",
	})
	keyOAuthWrote = i18n.Reg("setup.oauth.wrote", i18n.Text{
		"zh-CN": "config/oauth.yaml —— 启用了 %d 个", "en-US": "config/oauth.yaml — %d enabled",
	})
	keyOAuthInert = i18n.Reg("setup.oauth.inert", i18n.Text{
		"zh-CN": "config/oauth.yaml —— 占位条目，填了 client_id 才生效",
		"en-US": "config/oauth.yaml — placeholder entries, inert until client_id is filled in",
	})

	// ── providers ──
	keySecProviders = i18n.Reg("setup.providers.title", i18n.Text{
		"zh-CN": "外部能力源", "en-US": "External Capability Sources",
	})
	keyProvidersWrote = i18n.Reg("setup.providers.wrote", i18n.Text{
		"zh-CN": "config/providers.yaml —— 天气 / 搜索 / 消息通道的声明",
		"en-US": "config/providers.yaml — where weather, search and channels are declared",
	})
	keyProvidersNote = i18n.Reg("setup.providers.note", i18n.Text{
		"zh-CN": "可以删掉：没有它就按环境变量取默认源。里面写清了怎么接一个外部适配层。",
		"en-US": "Safe to delete: without it the default sources come from the environment. It documents how to attach an external adapter.",
	})

	// ── channels ──
	keySecChannels = i18n.Reg("setup.channels.title", i18n.Text{
		"zh-CN": "消息通道（可选）", "en-US": "Messaging Channels (optional)",
	})
	keyChannelsIntro = i18n.Reg("setup.channels.intro", i18n.Text{
		"zh-CN": "OneBot 11 / NapCat 可以把一个普通 QQ 号接进来。留空跳过。",
		"en-US": "OneBot 11 / NapCat bridges an ordinary QQ account. Leave blank to skip.",
	})
	keyChannelsWS = i18n.Reg("setup.channels.ws", i18n.Text{
		"zh-CN": "OneBot WebSocket 地址", "en-US": "OneBot WebSocket URL",
	})
	keyChannelsToken = i18n.Reg("setup.channels.token", i18n.Text{
		"zh-CN": "OneBot Access Token", "en-US": "OneBot access token",
	})
	keyChannelsSkipped = i18n.Reg("setup.channels.skipped", i18n.Text{
		"zh-CN": "没接通道 —— 早报、晚复盘这些照常跑，结果留在 app 里。",
		"en-US": "No channel — the proactive jobs still run; their results stay in the app.",
	})

	// ── server ──
	keySecServer = i18n.Reg("setup.server.title", i18n.Text{
		"zh-CN": "服务器", "en-US": "Server",
	})
	keyServerPublicURL = i18n.Reg("setup.server.publicurl", i18n.Text{
		"zh-CN": "对外访问地址（有域名就填，没有留空）",
		"en-US": "Public URL (leave blank if there is no domain yet)",
	})
	keyServerBind = i18n.Reg("setup.server.bind", i18n.Text{
		"zh-CN": "监听地址", "en-US": "Listen address",
	})
	keyServerPort = i18n.Reg("setup.server.port", i18n.Text{
		"zh-CN": "端口", "en-US": "Port",
	})
	keyServerProd = i18n.Reg("setup.server.prod", i18n.Text{
		"zh-CN": "这是生产部署吗？", "en-US": "Is this a production deployment?",
	})
	keyServerProdNote = i18n.Reg("setup.server.prod.note", i18n.Text{
		"zh-CN": "生产模式会打开 Secure cookie，所以必须走 HTTPS，否则登录不上。",
		"en-US": "Production turns on Secure cookies, so it must be served over HTTPS or logins will not stick.",
	})

	// ── secrets ──
	keySecSecrets = i18n.Reg("setup.secrets.title", i18n.Text{
		"zh-CN": "密钥", "en-US": "Secrets",
	})
	keySecretsGenerated = i18n.Reg("setup.secrets.generated", i18n.Text{
		"zh-CN": "已生成并写进 .env（权限 0600）", "en-US": "Generated and written to .env (mode 0600)",
	})
	keySecretsKept = i18n.Reg("setup.secrets.kept", i18n.Text{
		"zh-CN": "沿用 .env 里已有的密钥 —— 换掉它们会让所有人当场登出。",
		"en-US": "Reusing the existing secrets — rotating them logs everybody out immediately.",
	})
	keySecretsRotate = i18n.Reg("setup.secrets.rotate", i18n.Text{
		"zh-CN": "重新生成密钥？（所有人会被登出）", "en-US": "Regenerate the secrets? (everybody gets logged out)",
	})
	keySecretsAdmin = i18n.Reg("setup.secrets.admin", i18n.Text{
		"zh-CN": "管理面 token（记下来，它只在这里出现一次）",
		"en-US": "Admin token (write it down; this is the only place it appears)",
	})

	// ── files ──
	keySecFiles = i18n.Reg("setup.files.title", i18n.Text{
		"zh-CN": "文件", "en-US": "Files",
	})
	keyFilesPrompts = i18n.Reg("setup.files.prompts", i18n.Text{
		"zh-CN": "%d 个提示词模板 → %s", "en-US": "%d prompt templates → %s",
	})
	keyFilesPromptsNote = i18n.Reg("setup.files.prompts.note", i18n.Text{
		"zh-CN": "改这里的模板会逐文件覆盖内嵌那份 —— 改一个不必维护全部。",
		"en-US": "Edits here override the built-in copies file by file — changing one does not commit you to the rest.",
	})
	keyFilesBoundaries = i18n.Reg("setup.files.boundaries", i18n.Text{
		"zh-CN": "L1 硬边界 → %s", "en-US": "L1 hard boundaries → %s",
	})
	keyFilesBoundariesNote = i18n.Reg("setup.files.boundaries.note", i18n.Text{
		"zh-CN": "有意只能改文件、没有控制台入口 —— 能从网页删掉的边界不算边界。",
		"en-US": "Deliberately file-only, with no console surface: a boundary the web UI can delete is not a boundary.",
	})

	// ── summary ──
	keySummaryTitle = i18n.Reg("setup.summary.title", i18n.Text{
		"zh-CN": "装好了", "en-US": "Done",
	})
	keySummaryStart = i18n.Reg("setup.summary.start", i18n.Text{
		"zh-CN": "启动：", "en-US": "Start it:",
	})
	keySummaryEnvNote = i18n.Reg("setup.summary.envnote", i18n.Text{
		"zh-CN": ".env 是从工作目录读的，所以要先 cd 进去；交给 systemd 的话用 EnvironmentFile=%s。",
		"en-US": ".env is read from the working directory, so cd there first; under systemd use EnvironmentFile=%s.",
	})
	keySummaryNext = i18n.Reg("setup.summary.next", i18n.Text{
		"zh-CN": "接下来", "en-US": "Next",
	})
	keySummaryReconfig = i18n.Reg("setup.summary.reconfig", i18n.Text{
		"zh-CN": "改某一段配置：daycore config <段名>。段名：%s",
		"en-US": "Change one part later: daycore config <section>. Sections: %s",
	})
	keySummaryAPIOnly = i18n.Reg("setup.summary.apionly", i18n.Text{
		"zh-CN": "这个二进制只提供 API。前端单独部署，或者把 STATIC_DIR 指向它的构建目录。",
		"en-US": "This binary serves the API only. Deploy the frontend separately, or point STATIC_DIR at its build directory.",
	})
	keyConfigChanged = i18n.Reg("setup.config.changed", i18n.Text{
		"zh-CN": "改了 %d 项", "en-US": "%d settings changed",
	})
	keyConfigNoChange = i18n.Reg("setup.config.nochange", i18n.Text{
		"zh-CN": "没有改动", "en-US": "Nothing changed",
	})
	keyConfigRestart = i18n.Reg("setup.config.restart", i18n.Text{
		"zh-CN": "这些是启动期配置，改完要重启进程才生效。",
		"en-US": "These are boot-time settings; restart the process for them to take effect.",
	})
	keyUnknownSection = i18n.Reg("setup.config.unknown", i18n.Text{
		"zh-CN": "没有 %s 这一段。可用的：%s", "en-US": "No section named %s. Available: %s",
	})
)
