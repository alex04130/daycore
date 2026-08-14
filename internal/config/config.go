// Package config loads runtime configuration from environment variables (and an
// optional .env file). It deliberately does NOT parse the AI model catalog or
// the OAuth provider list itself — those live in the ai/auth packages to avoid
// an import cycle; config only carries their file paths.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"daycore/internal/i18n"

	"github.com/joho/godotenv"
)

// Config is the fully-resolved application configuration.
type Config struct {
	Env            string   // "development" | "production"
	Host           string   // listen host/interface; "" = all interfaces
	Port           string   // listen port, e.g. "8080"
	StaticDir      string   // frontend build dir served at "/" (SPA); "" or missing = API only
	AllowedOrigins []string // CORS allowlist (dev); empty = same-origin only

	// TrustProxyHeaders controls whether X-Forwarded-For is honored for client-IP
	// resolution (rate limiting, logs). Only enable when the app sits behind a
	// trusted reverse proxy that sets XFF — otherwise clients can spoof it to
	// bypass per-IP limits. Default false (use RemoteAddr).
	TrustProxyHeaders bool

	// Database — DBType selects the registered driver; DBDSN is its connection string.
	// SQLite default makes the app run with zero external services.
	DBType string // "sqlite" | "postgres" | "mysql" | "mongodb" | <custom>
	DBDSN  string

	// Auth
	JWTSecret     string        // HS256 signing key for session JWTs
	CookieSecret  string        // HMAC key for the anonymous-session cookie
	Pepper        string        // optional server-side secret mixed into password hashing
	JWTTTL        time.Duration // login token lifetime
	SecureCookies bool          // set Secure flag (true in production / behind TLS)

	// CookieSameSite is the SameSite attribute for the session/auth cookies:
	// "lax" (default), "strict", or "none". "none" is for cross-site cookie
	// deployments (frontend on another domain) and requires SecureCookies; the
	// recommended cross-origin path is header tokens, not cookies.
	CookieSameSite string
	PublicBaseURL  string // external URL, used to build OAuth redirect URIs

	// AI — model catalog + OAuth providers are loaded from these files by main.go.
	ModelsConfigPath string
	OAuthConfigPath  string
	// ProvidersConfigPath declares external capability sources — weather,
	// search, messaging channels. Boot-layer for the same reason as the two
	// above: the process builds an HTTP client or looks up a registered factory
	// out of every entry at startup.
	ProvidersConfigPath string
	// BlobStore selects the file-bus driver ("local", …; empty = no file bus).
	// Every feature that needs one checks and degrades to saying so, because
	// running without it is a supported configuration.
	BlobStore string
	// DataDir is the only writable path Daycore has. Every other directory
	// setting (StaticDir, LocalesDir, PromptsDir, ModelsConfigPath) is a
	// read-only input — before the file bus, nothing in internal/ wrote a file at
	// all. Used by the "local" blob driver.
	DataDir string
	// PromptsDir overlays prompt templates from disk onto the embedded defaults
	// (dir/<locale>/<key>.tmpl; absent files keep the embedded text). Empty = use
	// embedded only. `daycore install` extracts the templates here and writes this
	// key into the generated .env, so it must be read or the installer is setting
	// up an override nothing honours.
	PromptsDir          string
	DefaultChatModel    string // model id from the catalog used for text chat
	DefaultVisionModel  string // model id (caps.vision=true) used to read images
	DefaultPlannerModel string // model id used for autonomous planning; "" = default chat model
	AIRequestTimeout    time.Duration

	// Limits
	RateLimitPerMin     int   // per-IP requests/min on AI endpoints (0 = disabled)
	AuthRateLimitPerMin int   // per-IP requests/min on auth endpoints (0 = disabled)
	MaxImageBytes       int64 // max decoded image upload size for plan-image
	// MaxUploadBytes caps POST /api/files. Separate from MaxImageBytes because
	// they answer different questions: that one bounds what fits in a model
	// request, this one bounds what fits on the disk. A PDF the model reads a
	// page of at a time is legitimately larger than an image inlined into a
	// prompt, and one number for both would have to be the smaller of the two.
	MaxUploadBytes int64
	AgentMaxRounds int // companion agent tool rounds per message

	// Autonomous planning
	AutoPlanMaxDays         int // largest date range one auto-plan call may cover
	AssignmentLookaheadDays int // how far ahead assignments count as "upcoming"

	// AdminToken guards the prompt-editing admin endpoints (X-Admin-Token header).
	// Empty in development = open; required to be non-empty effect in production.
	AdminToken string

	// DefaultAssistantName is the name a freshly minted session carries before
	// the user renames the assistant. It used to be hardcoded "Leo" in the SQL
	// DDL and several fallback sites; a product that lets the user rename the
	// assistant must also let the operator choose the factory default.
	DefaultAssistantName string

	// Channels — OneBot (QQ). Empty OneBotWSURL disables the channel only: the
	// Worker runs unconditionally since 2026-07-29, because rhythm learning,
	// auto-plan and the care window are not things a QQ binding should gate.
	// With no channel, proactive output is persisted and not pushed.
	OneBotWSURL string
	OneBotToken string
	// WorkerDefaultTZ is the IANA timezone proactive briefs are scheduled in
	// (sessions don't carry a timezone yet).
	WorkerDefaultTZ string

	// Weather — provider selection + keys. Empty keys fall back to the free
	// default (open-meteo) with wttr.in as a chained fallback.
	WeatherProvider   string
	TavilyKey         string
	QWeatherKey       string
	OpenWeatherMapKey string

	// DefaultLocales is the language pair a user starts with before they choose
	// their own on the settings page: one main language and one to fall back
	// on, which is what the switch on every frontend's home page toggles
	// between. It is a *default*, not a restriction — a user may pick any two
	// of i18n.Available().
	//
	// Set from DEFAULT_PRIMARY_LOCALE / DEFAULT_SECONDARY_LOCALE. Leaving the
	// secondary empty starts users with no switch; they can still add one.
	DefaultLocales i18n.Pair

	// LocalesDir holds <locale>.json message packs, read at startup and
	// reloadable from the console. This is how a language is added without a
	// rebuild — the two locales compiled into the binary are a floor, not the
	// set of languages the product supports.
	LocalesDir string

	// ThemeKindsDir holds *.json files declaring extra theme value kinds, read
	// at startup and merged over the embedded six. Boot-layer for the same
	// reason LocalesDir is: the registry is built once and handed to the
	// server.
	ThemeKindsDir string

	// UsingDevSecrets is true when JWT/Cookie secrets fell back to the insecure
	// public dev defaults (no real secret configured). main.go warns on it.
	UsingDevSecrets bool

	// GeneratedAdminToken is true when no ADMIN_TOKEN was supplied and one was
	// invented for this process. main.go prints it — a generated credential
	// nobody is told about is the same as no credential at all, except harder to
	// diagnose.
	GeneratedAdminToken bool
}

// randomToken makes a URL-safe 256-bit secret.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Load reads .env (if present) then environment variables, applies defaults, and
// validates. It never fails just because .env is missing.
func Load() (*Config, error) {
	_ = godotenv.Load() // best-effort; real env always wins

	c := &Config{
		// Normalized to lower case: the value decides which security rules
		// apply, and "Production" quietly treated as development is a
		// deployment running dev fallback secrets while its owner believes
		// otherwise. The documented value is "production"; accepting any
		// casing is strictly the safe direction.
		Env:                     strings.ToLower(strings.TrimSpace(getEnv("APP_ENV", "development"))),
		Host:                    getEnv("HOST", ""),
		Port:                    getEnv("PORT", "8080"),
		StaticDir:               getEnv("STATIC_DIR", "web/frontend/dist"),
		AllowedOrigins:          splitCSV(getEnv("ALLOWED_ORIGINS", "")),
		TrustProxyHeaders:       getBool("TRUST_PROXY_HEADERS", false),
		DBType:                  getEnv("DB_TYPE", "sqlite"),
		DBDSN:                   getEnv("DB_DSN", "file:daycore.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"),
		JWTSecret:               getEnv("JWT_SECRET", ""),
		CookieSecret:            getEnv("COOKIE_SECRET", ""),
		Pepper:                  getEnv("PASSWORD_PEPPER", ""),
		JWTTTL:                  getDuration("JWT_TTL", 168*time.Hour),
		SecureCookies:           getBool("SECURE_COOKIES", false),
		CookieSameSite:          strings.ToLower(strings.TrimSpace(getEnv("COOKIE_SAMESITE", "lax"))),
		PublicBaseURL:           getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
		ModelsConfigPath:        getEnv("MODELS_CONFIG", "config/models.yaml"),
		OAuthConfigPath:         getEnv("OAUTH_CONFIG", "config/oauth.yaml"),
		ProvidersConfigPath:     getEnv("PROVIDERS_CONFIG", "config/providers.yaml"),
		PromptsDir:              getEnv("PROMPTS_DIR", ""),
		BlobStore:               getEnv("BLOB_STORE", ""),
		DataDir:                 getEnv("DATA_DIR", ""),
		DefaultChatModel:        getEnv("DEFAULT_CHAT_MODEL", "chat"),
		DefaultVisionModel:      getEnv("DEFAULT_VISION_MODEL", ""),
		DefaultPlannerModel:     getEnv("DEFAULT_PLANNER_MODEL", ""),
		AIRequestTimeout:        getDuration("AI_REQUEST_TIMEOUT", 120*time.Second),
		RateLimitPerMin:         getInt("AI_RATE_LIMIT_PER_MIN", 30),
		AuthRateLimitPerMin:     getInt("AUTH_RATE_LIMIT_PER_MIN", 10),
		AgentMaxRounds:          getInt("AGENT_MAX_ROUNDS", 6),
		MaxImageBytes:           int64(getInt("MAX_IMAGE_BYTES", 8*1024*1024)),
		MaxUploadBytes:          int64(getInt("MAX_UPLOAD_BYTES", 32*1024*1024)),
		AutoPlanMaxDays:         getInt("AUTO_PLAN_MAX_DAYS", 7),
		AssignmentLookaheadDays: getInt("ASSIGNMENT_LOOKAHEAD_DAYS", 14),
		AdminToken:              getEnv("ADMIN_TOKEN", ""),
		DefaultAssistantName:    getEnv("DEFAULT_ASSISTANT_NAME", "Leo"),
		OneBotWSURL:             getEnv("ONEBOT_WS_URL", ""),
		OneBotToken:             getEnv("ONEBOT_TOKEN", ""),
		WorkerDefaultTZ:         getEnv("WORKER_DEFAULT_TZ", "Asia/Shanghai"),
		WeatherProvider:         getEnv("WEATHER_PROVIDER", "open-meteo"),
		TavilyKey:               getEnv("TAVILY_API_KEY", ""),
		QWeatherKey:             getEnv("QWEATHER_API_KEY", ""),
		OpenWeatherMapKey:       getEnv("OPENWEATHERMAP_API_KEY", ""),
	}

	// Dev-only fallbacks so the app boots out of the box; production must set real secrets.
	// Whitespace-only values count as unset: JWT_SECRET=" " is not a secret
	// somebody chose, it is a misconfigured deployment that must not silently
	// carry on as if fine.
	c.JWTSecret = strings.TrimSpace(c.JWTSecret)
	c.CookieSecret = strings.TrimSpace(c.CookieSecret)
	c.AdminToken = strings.TrimSpace(c.AdminToken)
	if c.JWTSecret == "" {
		if c.Env == "production" {
			return nil, fmt.Errorf("JWT_SECRET is required in production")
		}
		c.JWTSecret = "dev-insecure-jwt-secret-change-me"
		c.UsingDevSecrets = true
	}
	if c.CookieSecret == "" {
		if c.Env == "production" {
			return nil, fmt.Errorf("COOKIE_SECRET is required in production")
		}
		c.CookieSecret = "dev-insecure-cookie-secret-change-me"
		c.UsingDevSecrets = true
	}
	if c.DBType == "" {
		return nil, fmt.Errorf("DB_TYPE must not be empty")
	}

	// An unset ADMIN_TOKEN used to mean "the admin API is open outside
	// production" (handlers_admin.go's old `return !IsProduction()`). That is an
	// UNAUTHENTICATED configuration API on every dev box, every staging
	// deployment, and every self-hosted instance whose owner never set APP_ENV —
	// and it is the branch that would have been most dangerous under the planned
	// degraded boot, where the whole point is to serve the console while storage
	// is down.
	//
	// Generating one instead removes the open state entirely without making
	// local development painful: it is printed once at startup, so getting in is
	// a copy-paste, and there is never a moment when there is no credential.
	// Production still refuses to invent one — a token nobody wrote down is a
	// token nobody can rotate, and a deployment that means to be administered
	// should say so.
	if c.AdminToken == "" {
		if c.IsProduction() {
			return nil, fmt.Errorf("ADMIN_TOKEN is required in production")
		}
		tok, err := randomToken()
		if err != nil {
			return nil, fmt.Errorf("generate a development admin token: %w", err)
		}
		c.AdminToken = tok
		c.GeneratedAdminToken = true
	}
	// In production, default the Secure cookie flag on unless the operator
	// explicitly opted out — a forgotten SECURE_COOKIES must not silently ship
	// session/JWT cookies over plaintext HTTP. "Set but empty" counts as
	// unset, matching getEnv — SECURE_COOKIES= (a leftover in an .env) must
	// not disable the production default, which is exactly the quiet failure
	// this default exists to prevent.
	if getEnv("SECURE_COOKIES", "") == "" && c.IsProduction() {
		c.SecureCookies = true
	}
	// SameSite=None cookies are rejected by browsers without Secure, so refuse
	// the combination outright instead of shipping silently broken auth.
	switch c.CookieSameSite {
	case "lax", "strict", "none":
	default:
		return nil, fmt.Errorf("COOKIE_SAMESITE must be lax, strict, or none (got %q)", c.CookieSameSite)
	}
	if c.CookieSameSite == "none" && !c.SecureCookies {
		return nil, fmt.Errorf("COOKIE_SAMESITE=none requires SECURE_COOKIES=true")
	}
	// Fail-safe: in development with no explicit HOST, bind loopback only. A
	// stock `./daycore` run carries insecure dev secrets and (with no ADMIN_TOKEN)
	// an open admin surface — those must not be reachable from the network by
	// default. Production binds all interfaces (typically behind a reverse proxy).
	// Same "set but empty counts as unset" rule as the Secure default above:
	// HOST= in an .env must not defeat the loopback fail-safe and bind the dev
	// server — dev secrets, generated admin token — to every interface.
	if getEnv("HOST", "") == "" && !c.IsProduction() {
		c.Host = "127.0.0.1"
	}
	// Message packs load before the pair is validated, because a pack is what
	// makes a locale valid to pick in the first place.
	// Theme value kinds: the same three-layer shape as the message catalogue,
	// so adding one is a file rather than a release. A missing directory is the
	// default and is not an error; a malformed file in it IS, because somebody
	// put it there on purpose. See internal/theme.
	c.ThemeKindsDir = getEnv("THEME_KINDS_DIR", "theme-kinds")

	c.LocalesDir = getEnv("LOCALES_DIR", "locales")
	if err := i18n.Std().LoadDir(c.LocalesDir); err != nil {
		return nil, err
	}
	// A typo here would otherwise surface as every new user silently starting
	// in the wrong language, so it fails the boot instead.
	pair, err := i18n.NewPair(
		getEnv("DEFAULT_PRIMARY_LOCALE", "zh-CN"),
		getEnv("DEFAULT_SECONDARY_LOCALE", "en-US"),
	)
	if err != nil {
		return nil, err
	}
	c.DefaultLocales = pair
	return c, nil
}

// IsProduction reports whether the app runs in production mode.
func (c *Config) IsProduction() bool { return c.Env == "production" }

// ListenAddr joins Host and Port into the address ListenAndServe expects.
// HOST can be an interface IP ("127.0.0.1") or a hostname; when nginx fronts
// the app on a domain, keep HOST empty and set server_name in nginx instead.
func (c *Config) ListenAddr() string { return c.Host + ":" + c.Port }

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func getBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	}
	return def
}

func getDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil {
			return d
		}
	}
	return def
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
