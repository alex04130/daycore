package config

// Configuration layering: which knobs can be changed while the process is
// running, and which cannot.
//
// # Why this exists as a table rather than as a habit
//
// The console (batch θ) is supposed to show and edit configuration. Before
// anyone can build that screen, somebody has to answer, for every single knob,
// "can this change without a restart" — and the answer is not derivable from
// the type or the name. `AGENT_MAX_ROUNDS` is read on every request and could
// change any time; `DB_DSN` decides which database the process opened; both are
// strings in the same struct.
//
// Left to a habit, that question gets answered forty times by whoever is writing
// the screen that day, and the wrong answers are the dangerous ones: a knob
// wrongly marked runtime is a setting the console changes and the process
// ignores — "I turned it off and it kept doing it".
//
// # The line
//
//	Boot     the process read it once and built something out of it: a socket, a
//	         database handle, a signing key, a parsed config file. Changing the
//	         value cannot change the thing already built. The console may SHOW
//	         these; it must never offer to edit them.
//	Runtime  read fresh each time it is used. Nothing was built out of it, so a
//	         new value takes effect on the next read.
//
// Secrets are Boot regardless of how they are read. A console that can display a
// signing key is a console that has already lost it, and one that can change it
// invalidates every session in exchange for a typo.
//
// # What this file does NOT do yet
//
// It does not implement the override table. STRATEGY's F1 calls for runtime
// knobs to move into a `settings` table with the environment variable demoted to
// a seed — correct, and deliberately not built here, because its only consumer
// is the console endpoint (F4b) and this repo has shipped six mechanisms with no
// callers already. The classification is the part that has to exist FIRST: the
// table cannot be designed without it, and the gate below is what keeps it true
// in the meantime.

// Layer is when a setting can change.
type Layer string

const (
	// LayerBoot: read once at startup and built into something. Console shows,
	// never edits.
	LayerBoot Layer = "boot"
	// LayerRuntime: read fresh at each use. Safe to change while running.
	LayerRuntime Layer = "runtime"
)

// Setting describes one knob.
type Setting struct {
	Env   string // environment variable name
	Field string // the Config field it fills
	Layer Layer
	// Secret marks a value that must never leave the process — not to the
	// console, not to a log line, not to an error message. Always LayerBoot.
	Secret bool
	// Why is the reason for the layer, in the cases where it is not obvious.
	// Empty where the answer is self-evident (a listen port is a listen port).
	Why string
}

// Settings is every knob, classified. A field missing from here fails
// TestEveryConfigFieldIsClassified — which is the point: adding configuration
// should force somebody to answer the question once, at the moment they have the
// context to answer it.
var Settings = []Setting{
	// ── the socket and the filesystem ──
	{Env: "APP_ENV", Field: "Env", Layer: LayerBoot,
		Why: "decides secret handling and cookie defaults at construction time"},
	{Env: "HOST", Field: "Host", Layer: LayerBoot},
	{Env: "PORT", Field: "Port", Layer: LayerBoot},
	{Env: "STATIC_DIR", Field: "StaticDir", Layer: LayerBoot,
		Why: "the static handler is mounted once, and only if the directory exists"},
	{Env: "DATA_DIR", Field: "DataDir", Layer: LayerBoot,
		Why: "the blob driver opened it and holds it"},
	{Env: "BLOB_STORE", Field: "BlobStore", Layer: LayerBoot,
		Why: "selects a driver; the Store is constructed once"},
	{Env: "PROMPTS_DIR", Field: "PromptsDir", Layer: LayerBoot,
		Why: "templates and the hard boundaries are overlaid at startup"},
	{Env: "LOCALES_DIR", Field: "LocalesDir", Layer: LayerBoot,
		Why: "language packs are loaded into the catalog at startup"},
	{Env: "MODELS_CONFIG", Field: "ModelsConfigPath", Layer: LayerBoot,
		Why: "the catalog is parsed once into provider instances"},
	{Env: "OAUTH_CONFIG", Field: "OAuthConfigPath", Layer: LayerBoot,
		Why: "providers are constructed once, with their redirect URIs baked in"},

	// ── the database ──
	{Env: "DB_TYPE", Field: "DBType", Layer: LayerBoot,
		Why: "the handle is open"},
	{Env: "DB_DSN", Field: "DBDSN", Layer: LayerBoot, Secret: true,
		Why: "the handle is open, and a DSN carries a password"},

	// ── secrets and the shape of authentication ──
	{Env: "JWT_SECRET", Field: "JWTSecret", Layer: LayerBoot, Secret: true},
	{Env: "COOKIE_SECRET", Field: "CookieSecret", Layer: LayerBoot, Secret: true},
	{Env: "PASSWORD_PEPPER", Field: "Pepper", Layer: LayerBoot, Secret: true,
		Why: "changing it invalidates every stored password hash"},
	{Env: "ADMIN_TOKEN", Field: "AdminToken", Layer: LayerBoot, Secret: true,
		Why: "the credential the console itself authenticates with; editable from the console means editable by whoever already got in"},
	{Env: "JWT_TTL", Field: "JWTTTL", Layer: LayerBoot,
		Why: "security-shaped; a console that can set it to a year is a console that can mint a permanent token"},
	{Env: "SECURE_COOKIES", Field: "SecureCookies", Layer: LayerBoot,
		Why: "a security boundary; turning it off from the web is how it gets turned off"},
	{Env: "COOKIE_SAMESITE", Field: "CookieSameSite", Layer: LayerBoot, Why: "same"},
	{Env: "ALLOWED_ORIGINS", Field: "AllowedOrigins", Layer: LayerBoot,
		Why: "a security boundary. It is read per request and COULD be hot, which is exactly why it is listed here with a reason: editable CORS from the console means one compromised console opens the API to any origin"},
	{Env: "TRUST_PROXY_HEADERS", Field: "TrustProxyHeaders", Layer: LayerBoot,
		Why: "wrongly true lets any client forge its IP and escape rate limiting"},
	{Env: "PUBLIC_BASE_URL", Field: "PublicBaseURL", Layer: LayerBoot,
		Why: "OAuth redirect URIs are built from it at load and registered with the provider"},

	// ── external credentials ──
	{Env: "QWEATHER_API_KEY", Field: "QWeatherKey", Layer: LayerBoot, Secret: true},
	{Env: "OPENWEATHERMAP_API_KEY", Field: "OpenWeatherMapKey", Layer: LayerBoot, Secret: true},
	{Env: "ONEBOT_TOKEN", Field: "OneBotToken", Layer: LayerBoot, Secret: true},
	{Env: "ONEBOT_WS_URL", Field: "OneBotWSURL", Layer: LayerBoot,
		Why: "the websocket is dialled once at startup. F3 makes channels reconfigurable; until the reconnect path exists, calling this runtime would be a lie"},

	// ── genuinely runtime ──
	{Env: "DEFAULT_CHAT_MODEL", Field: "DefaultChatModel", Layer: LayerRuntime,
		Why: "selects among already-constructed catalog entries"},
	{Env: "DEFAULT_VISION_MODEL", Field: "DefaultVisionModel", Layer: LayerRuntime, Why: "same"},
	{Env: "DEFAULT_PLANNER_MODEL", Field: "DefaultPlannerModel", Layer: LayerRuntime, Why: "same"},
	{Env: "WEATHER_PROVIDER", Field: "WeatherProvider", Layer: LayerRuntime,
		Why: "the provider chain is rebuilt per lookup"},
	{Env: "AI_REQUEST_TIMEOUT", Field: "AIRequestTimeout", Layer: LayerRuntime},
	{Env: "AI_RATE_LIMIT_PER_MIN", Field: "RateLimitPerMin", Layer: LayerRuntime,
		Why: "⚠️ the limiter is constructed in New with this value baked in — it is runtime by nature and NOT yet hot. Making it so means giving rateLimiter a setter, which is F4b's job"},
	{Env: "AUTH_RATE_LIMIT_PER_MIN", Field: "AuthRateLimitPerMin", Layer: LayerRuntime, Why: "same"},
	{Env: "AGENT_MAX_ROUNDS", Field: "AgentMaxRounds", Layer: LayerRuntime},
	{Env: "MAX_IMAGE_BYTES", Field: "MaxImageBytes", Layer: LayerRuntime},
	{Env: "MAX_UPLOAD_BYTES", Field: "MaxUploadBytes", Layer: LayerRuntime},
	{Env: "AUTO_PLAN_MAX_DAYS", Field: "AutoPlanMaxDays", Layer: LayerRuntime},
	{Env: "ASSIGNMENT_LOOKAHEAD_DAYS", Field: "AssignmentLookaheadDays", Layer: LayerRuntime},
	{Env: "WORKER_DEFAULT_TZ", Field: "WorkerDefaultTZ", Layer: LayerRuntime,
		Why: "only the fallback for a session with no timezone of its own (ζ-4); changing it does not move anybody who has one"},
	{Env: "DEFAULT_PRIMARY_LOCALE", Field: "DefaultLocales", Layer: LayerRuntime,
		Why: "the default pair for NEW users; a user's own choice is stored per session"},
	{Env: "DEFAULT_SECONDARY_LOCALE", Field: "DefaultLocales", Layer: LayerRuntime, Why: "same"},

	// ── derived, not configured ──
	{Env: "", Field: "UsingDevSecrets", Layer: LayerBoot,
		Why: "computed during Load from whether the secrets were supplied; not a knob"},
}

// SettingFor returns the classification of a Config field.
func SettingFor(field string) (Setting, bool) {
	for _, s := range Settings {
		if s.Field == field {
			return s, true
		}
	}
	return Setting{}, false
}

// Editable reports whether the console may offer to change this field.
//
// Secrets are excluded even though they are already LayerBoot, because the two
// reasons are different and only one of them is about restarts: a secret must
// not be editable OR VISIBLE, and stating that as its own predicate keeps the
// next reader from "fixing" it when boot-time settings eventually become
// editable-with-restart.
func Editable(field string) bool {
	s, ok := SettingFor(field)
	return ok && s.Layer == LayerRuntime && !s.Secret
}
