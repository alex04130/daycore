package setup

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A Section is one part of the backend configuration.
//
// # Why sections exist
//
// `daycore install` is the whole list in order; `daycore config <name>` is one
// of them against a deployment that already exists. Before this split there was
// only install, so changing the database after setup meant hand-editing .env —
// and the one thing a person hand-editing .env cannot see is which keys the
// server actually reads. Two commands, one list, no second copy to drift.
//
// # Boundary: a section owns its keys and only its keys
//
// Running `config oauth` must not touch DB_DSN. That is what makes re-running
// one section safe, and it is enforced by Keys(): the runner diffs the .env
// afterwards and refuses to save if a section wrote outside its own set. A
// section that quietly widened its reach would turn "change the login provider"
// into an outage.
type Section struct {
	Name     string   // the argument to `daycore config`
	TitleKey string   // heading
	Keys     []string // every .env key this section may write
	Run      func(s *Session) error
}

// Sections is the ordered list. Order is install order, and it is not
// arbitrary: the database comes first because it is the answer most likely to
// be wrong, and the summary is more useful when the thing that failed is the
// thing you just typed. Secrets come last so their output is the final thing on
// screen — an admin token printed twenty lines up gets scrolled away.
func Sections() []Section {
	return []Section{
		{Name: "db", TitleKey: keySecDatabase, Keys: []string{"DB_TYPE", "DB_DSN"}, Run: runDatabase},
		{Name: "models", TitleKey: keySecModels, Keys: []string{"MODELS_CONFIG", "DEFAULT_CHAT_MODEL"}, Run: runModels},
		{Name: "keys", TitleKey: keySecKeys, Keys: []string{"DEEPSEEK_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"}, Run: runKeys},
		{Name: "weather", TitleKey: keySecWeather, Keys: []string{"WEATHER_PROVIDER", "QWEATHER_API_KEY", "OPENWEATHERMAP_API_KEY"}, Run: runWeather},
		{Name: "oauth", TitleKey: keySecOAuth, Keys: []string{"OAUTH_CONFIG"}, Run: runOAuth},
		{Name: "channels", TitleKey: keySecChannels, Keys: []string{"ONEBOT_WS_URL", "ONEBOT_TOKEN"}, Run: runChannels},
		{Name: "server", TitleKey: keySecServer, Keys: []string{"APP_ENV", "HOST", "PORT", "PUBLIC_BASE_URL", "SECURE_COOKIES"}, Run: runServer},
		{Name: "secrets", TitleKey: keySecSecrets, Keys: []string{"ADMIN_TOKEN", "JWT_SECRET", "COOKIE_SECRET"}, Run: runSecrets},
	}
}

// SectionNames is the list `daycore config` accepts, for help and errors.
func SectionNames() []string {
	out := make([]string, 0, len(Sections()))
	for _, s := range Sections() {
		out = append(out, s.Name)
	}
	return out
}

// FindSection looks one up by name.
func FindSection(name string) (Section, bool) {
	for _, s := range Sections() {
		if s.Name == name {
			return s, true
		}
	}
	return Section{}, false
}

// ─── database ────────────────────────────────────────────────────────────────

func runDatabase(s *Session) error {
	engine := s.Choose(keyDBEngine, []Option{
		{ID: "sqlite", LabelKey: keyDBSqlite, HintKey: keyDBSqliteHint},
		{ID: "postgres", LabelKey: keyDBPostgres, HintKey: keyDBServerHint},
		{ID: "mysql", LabelKey: keyDBMySQL, HintKey: keyDBServerHint},
		{ID: "mongodb", LabelKey: keyDBMongo, HintKey: keyDBMongoHint},
	}, defaultOr(s.Env.Get("DB_TYPE"), "sqlite"))

	var dsn string
	switch engine {
	case "sqlite":
		dsn = "file:" + filepath.Join(s.Dir, "daycore.db") + "?_journal_mode=WAL"
		s.Plain(dsn)
	default:
		dsn = s.Ask(keyDBDSN, defaultOr(s.Env.Get("DB_DSN"), sampleDSN(engine)))
	}
	s.Env.SetIn("Database", "DB_TYPE", engine, "")
	s.Env.SetIn("Database", "DB_DSN", dsn, "")

	// Opt-in, and a failure never stops the flow. A mistyped DSN is the single
	// most common setup error, so offering the check is worth it; making it
	// mandatory would mean the installer cannot be used before the database
	// exists, which is the normal order on a fresh box.
	if engine != "sqlite" && s.Confirm(keyDBCheck, true) {
		if err := PingDSN(engine, dsn); err != nil {
			s.Warn(keyDBCheckFail, err.Error())
			s.Hint(keyDBCheckFailNote)
		} else {
			s.Done(keyDBCheckOK)
		}
	}
	return nil
}

func sampleDSN(engine string) string {
	switch engine {
	case "postgres":
		return "postgres://user:pass@localhost:5432/daycore?sslmode=disable"
	case "mysql":
		return "user:pass@tcp(localhost:3306)/daycore?parseTime=true"
	case "mongodb":
		return "mongodb://localhost:27017/daycore"
	}
	return ""
}

// ─── model catalog ───────────────────────────────────────────────────────────

func runModels(s *Session) error {
	s.Hint(keyModelsIntro)
	path := filepath.Join(s.Dir, "config", "models.yaml")
	s.Env.SetIn("Paths", "MODELS_CONFIG", path, "")

	if exists(path) && !s.Force {
		s.Done(keyModelsKept)
		return nil
	}

	switch s.Choose(keyModelsHow, []Option{
		{ID: "seed", LabelKey: keyModelsSeed, HintKey: keyModelsSeedHint},
		{ID: "now", LabelKey: keyModelsNow, HintKey: keyModelsNowHint},
		{ID: "later", LabelKey: keyModelsLater, HintKey: keyModelsLaterHint},
	}, "seed") {
	case "now":
		base := s.Ask(keyModelsBaseURL, "https://api.deepseek.com/v1")
		model := s.Ask(keyModelsModel, "deepseek-v4-flash")
		keyEnv := s.Ask(keyModelsKeyEnv, "DEEPSEEK_API_KEY")
		if err := writeFile(path, minimalCatalog(base, model, keyEnv), s.Force); err != nil {
			return err
		}
		s.Env.SetIn("AI", "DEFAULT_CHAT_MODEL", "chat", "")
		s.Done(keyModelsWrote, 1, "chat")
	default:
		// "later" writes the seed too. The catalog is the one file whose absence
		// is exit 1, so a "later" that leaves it out would be a setup command
		// offering to produce a deployment that cannot start.
		seed, err := CatalogSeedBytes()
		if err != nil {
			return err
		}
		if err := writeFile(path, seed, s.Force); err != nil {
			return err
		}
		s.Done(keyModelsWrote, 4, "chat")
		s.Hint(keyModelsStale)
	}
	return nil
}

func minimalCatalog(base, model, keyEnv string) []byte {
	return []byte(fmt.Sprintf(`# Written by daycore install.
#
# id is OUR stable name (DEFAULT_CHAT_MODEL and the database reference it);
# model is the vendor's, and the vendor's is the half that rots. Rotating to a
# newer upstream model is a one-line edit to model: and touches nothing else.
models:
  - id: chat
    format: openai
    base_url: %s
    model: %s
    api_key_env: %s
    tools: true
    stream: true
    context_window: 65536
    max_tokens: 4096
`, base, model, keyEnv))
}

// ─── provider keys ───────────────────────────────────────────────────────────

func runKeys(s *Session) error {
	s.Hint(keyKeysIntro)
	any := false
	for _, p := range []struct{ label, env string }{
		{"DeepSeek", "DEEPSEEK_API_KEY"},
		{"OpenAI", "OPENAI_API_KEY"},
		{"Anthropic", "ANTHROPIC_API_KEY"},
	} {
		if v := s.AskSecret(keyKeysOne, p.label); v != "" {
			s.Env.SetIn("AI", p.env, v, "")
			any = true
		} else if s.Env.Has(p.env) {
			any = true
		}
	}
	if !any {
		s.Warn(keyKeysNone)
	}
	return nil
}

// ─── weather ─────────────────────────────────────────────────────────────────

func runWeather(s *Session) error {
	s.Hint(keyWeatherIntro)
	q := s.AskSecret(keyWeatherQWeather)
	o := s.AskSecret(keyWeatherOWM)
	switch {
	case q != "":
		s.Env.SetIn("Weather", "WEATHER_PROVIDER", "qweather", "")
		s.Env.SetIn("Weather", "QWEATHER_API_KEY", q, "")
	case o != "":
		s.Env.SetIn("Weather", "WEATHER_PROVIDER", "openweathermap", "")
		s.Env.SetIn("Weather", "OPENWEATHERMAP_API_KEY", o, "")
	default:
		s.Env.SetIn("Weather", "WEATHER_PROVIDER", "open-meteo", "")
	}
	return nil
}

// ─── oauth ───────────────────────────────────────────────────────────────────

func runOAuth(s *Session) error {
	s.Hint(keyOAuthIntro)
	path := filepath.Join(s.Dir, "config", "oauth.yaml")
	s.Env.SetIn("Paths", "OAUTH_CONFIG", path, "")

	base := s.Env.Get("PUBLIC_BASE_URL")
	if base == "" {
		base = "https://<PUBLIC_BASE_URL>"
		s.Hint(keyOAuthNeedsBase)
	}

	type prov struct{ name, id, secret string }
	var enabled []prov
	for _, name := range []string{"google", "github"} {
		if !s.Confirm(keyOAuthWant, false, name) {
			continue
		}
		s.Hint(keyOAuthCallback, base+"/api/auth/oauth/"+name+"/callback")
		id := s.Ask(keyOAuthClientID, "", name)
		if id == "" {
			continue
		}
		enabled = append(enabled, prov{name, id, s.AskSecret(keyOAuthClientSecret, name)})
	}

	if len(enabled) == 0 {
		seed, err := OAuthSeedBytes()
		if err != nil {
			return err
		}
		if err := writeFile(path, seed, s.Force); err != nil {
			return err
		}
		s.Done(keyOAuthInert)
		return nil
	}
	var sb strings.Builder
	sb.WriteString("# Written by daycore install. Entries with an empty client_id are skipped.\nproviders:\n")
	for _, p := range enabled {
		fmt.Fprintf(&sb, "  - name: %s\n    client_id: %q\n    client_secret: %q\n", p.name, p.id, p.secret)
	}
	if err := writeFile(path, []byte(sb.String()), true); err != nil {
		return err
	}
	s.Done(keyOAuthWrote, len(enabled))
	return nil
}

// ─── channels ────────────────────────────────────────────────────────────────

func runChannels(s *Session) error {
	s.Hint(keyChannelsIntro)
	ws := s.Ask(keyChannelsWS, s.Env.Get("ONEBOT_WS_URL"))
	if ws == "" {
		s.Env.Unset("ONEBOT_WS_URL")
		s.Env.Unset("ONEBOT_TOKEN")
		s.Hint(keyChannelsSkipped)
		return nil
	}
	s.Env.SetIn("Channels", "ONEBOT_WS_URL", ws, "")
	if tok := s.AskSecret(keyChannelsToken); tok != "" {
		s.Env.SetIn("Channels", "ONEBOT_TOKEN", tok, "")
	}
	return nil
}

// ─── server ──────────────────────────────────────────────────────────────────

func runServer(s *Session) error {
	if url := s.Ask(keyServerPublicURL, s.Env.Get("PUBLIC_BASE_URL")); url != "" {
		s.Env.SetIn("Server", "PUBLIC_BASE_URL", strings.TrimRight(url, "/"), "")
	} else {
		s.Env.Unset("PUBLIC_BASE_URL")
	}
	s.Env.SetIn("Server", "HOST", s.Ask(keyServerBind, defaultOr(s.Env.Get("HOST"), "0.0.0.0")), "")
	s.Env.SetIn("Server", "PORT", s.Ask(keyServerPort, defaultOr(s.Env.Get("PORT"), "8080")), "")

	if s.Confirm(keyServerProd, s.Env.Get("APP_ENV") == "production") {
		s.Env.SetIn("Server", "APP_ENV", "production", "")
		s.Env.SetIn("Server", "SECURE_COOKIES", "true", "")
		s.Hint(keyServerProdNote)
	} else {
		s.Env.Unset("APP_ENV")
		s.Env.Unset("SECURE_COOKIES")
	}
	return nil
}

// ─── secrets ─────────────────────────────────────────────────────────────────

func runSecrets(s *Session) error {
	have := s.Env.Has("JWT_SECRET") && s.Env.Has("COOKIE_SECRET") && s.Env.Has("ADMIN_TOKEN")
	if have && !s.Confirm(keySecretsRotate, false) {
		s.Done(keySecretsKept)
		return nil
	}
	admin := genHex(24)
	s.Env.SetIn("Secrets", "ADMIN_TOKEN", admin, "")
	s.Env.SetIn("Secrets", "JWT_SECRET", genHex(32), "")
	s.Env.SetIn("Secrets", "COOKIE_SECRET", genHex(32), "")
	s.Done(keySecretsGenerated)
	s.Hint(keySecretsAdmin)
	s.Plain(admin)
	return nil
}

func genHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing means there is no entropy source. Continuing with a
		// predictable signing key would be worse than any crash.
		panic("setup: no entropy: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// ─── shared helpers ──────────────────────────────────────────────────────────

func defaultOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func writeFile(path string, content []byte, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if !force && exists(path) {
		return nil
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
