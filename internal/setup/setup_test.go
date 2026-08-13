package setup

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"daycore/internal/ai"
	"daycore/internal/auth"
	"daycore/internal/config"
	"daycore/internal/resources"

	_ "daycore/internal/ai/formats/anthropic"
	_ "daycore/internal/ai/formats/ollama"
	_ "daycore/internal/ai/formats/openai"
)

// TestMain points a lite build at the repository's own resource tree.
//
// Under -tags lite the binary embeds nothing, so without this every test here
// fails on a missing data directory rather than on anything it is testing. The
// full build ignores it. Doing this in TestMain rather than per-test is what
// keeps the two builds running the SAME assertions — a lite-only skip would
// mean the install flow is only ever exercised in one of the two shapes it
// ships in.
func TestMain(m *testing.M) {
	if resources.Lite {
		resources.SetDataDir("../resources/data")
	}
	os.Exit(m.Run())
}

// answers drives a whole run. Blank lines take every default, which is what a
// person gets by holding return — the path most installs actually take, and the
// one that has to produce something that starts.
func installInto(t *testing.T, dir string, answers ...string) *Session {
	t.Helper()
	script := strings.Join(answers, "\n") + strings.Repeat("\n", 40)
	env, err := LoadEnv(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSession(strings.NewReader(script), io.Discard, "en-US", dir, env, false, false)
	if err := Install(s); err != nil {
		t.Fatalf("install: %v", err)
	}
	return s
}

func envMap(t *testing.T, dir string) map[string]string {
	t.Helper()
	e, err := LoadEnv(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, k := range e.Keys() {
		out[k] = e.Get(k)
	}
	return out
}

// The one rule this command has: whatever it writes must be enough to boot.
//
// It was not. install created an empty config/ and never wrote models.yaml,
// which LoadCatalog treats as fatal, so following the Quick Start it printed
// gave exit 1 — a setup command reporting success over a directory that cannot
// start. This goes through config.Load rather than reading .env by hand,
// because half the ways install can betray a deployment read fine as text: a
// key under the wrong name, a seeded catalog whose ids do not match what
// DEFAULT_CHAT_MODEL falls back to, a path that only resolves in a source tree.
func TestInstalledTreeBoots(t *testing.T) {
	dir := t.TempDir()
	installInto(t, dir)
	for k, v := range envMap(t, dir) {
		t.Setenv(k, v)
	}
	// Hermetic: a stale DEFAULT_CHAT_MODEL in the ambient environment (the
	// developer's shell, a direnv hook) would otherwise poison config.Load —
	// godotenv never overrides a real env var, and the catalog rejects an id
	// that is not in models.yaml. The seeded catalog guarantees "chat".
	t.Setenv("DEFAULT_CHAT_MODEL", "chat")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("the generated .env does not load: %v", err)
	}

	catalog, err := ai.LoadCatalog(cfg.ModelsConfigPath, cfg.DefaultChatModel, cfg.DefaultVisionModel, cfg.DefaultPlannerModel)
	if err != nil {
		t.Fatalf("a freshly installed tree cannot load its model catalog: %v", err)
	}
	if len(catalog.List()) == 0 {
		t.Error("the seeded catalog is empty, so every AI call fails at first use")
	}
	if _, err := auth.LoadOAuthProviders(cfg.OAuthConfigPath, ""); err != nil {
		t.Fatalf("cannot load the generated oauth config: %v", err)
	}
	prompts, err := ai.NewPromptService(nil)
	if err != nil {
		t.Fatal(err)
	}
	n, err := prompts.LoadDiskDefaults(cfg.PromptsDir)
	if err != nil {
		t.Fatalf("cannot load the extracted prompts: %v", err)
	}
	if n == 0 {
		t.Error("PROMPTS_DIR is in .env but nothing was extracted there — install configured an override the server ignores")
	}
	b, err := ai.NewBoundaries()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.LoadDir(cfg.PromptsDir); err != nil {
		t.Fatalf("cannot load the extracted L1 boundaries: %v", err)
	}
}

// "Later" still writes the catalog. It is the one file whose absence is exit 1,
// so a deferral that left it out would be the installer offering to produce a
// deployment that cannot start — which is the whole bug this batch is about.
func TestModelsLaterStillLeavesABootableCatalog(t *testing.T) {
	dir := t.TempDir()
	// db(default) → models: option 3 = later
	installInto(t, dir, "", "3")

	path := filepath.Join(dir, "config", "models.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("choosing 'later' left no model catalog: %v", err)
	}
	if _, err := ai.LoadCatalog(path, "chat", "", ""); err != nil {
		t.Errorf("the catalog written by 'later' does not load: %v", err)
	}
}

// Paths are absolute so the binary can be started from anywhere. config.Load's
// defaults resolve against the working directory — `cd / && /opt/daycore/daycore`
// would look for ./config/models.yaml and exit 1 naming a path nobody chose.
func TestGeneratedEnvNamesTheInstallDir(t *testing.T) {
	dir := t.TempDir()
	installInto(t, dir)
	env := envMap(t, dir)

	for _, key := range []string{"MODELS_CONFIG", "OAUTH_CONFIG", "PROMPTS_DIR", "DB_DSN"} {
		if !strings.Contains(env[key], dir) {
			t.Errorf("%s = %q does not name the install dir", key, env[key])
		}
	}
	// A bare `KEY=` reads as configured and does nothing, because config.getEnv
	// treats set-but-empty as unset. Anything meaning "use the default" must be
	// absent, not empty.
	for k, v := range env {
		if v == "" {
			t.Errorf("%s was written empty; getEnv ignores it, so it configures nothing", k)
		}
	}
}

// Re-running install is normal — people do it to recover a lost admin token, or
// after an upgrade. It must not take back their edits.
func TestReinstallKeepsEdits(t *testing.T) {
	dir := t.TempDir()
	installInto(t, dir)

	models := filepath.Join(dir, "config", "models.yaml")
	edited := "models:\n  - id: mine\n    format: openai\n    model: x\n"
	if err := os.WriteFile(models, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	tmpl := filepath.Join(dir, "prompts", "zh-CN", "persona.tmpl")
	if err := os.WriteFile(tmpl, []byte("my own persona"), 0644); err != nil {
		t.Fatal(err)
	}
	before := envMap(t, dir)

	installInto(t, dir)

	if got, _ := os.ReadFile(models); string(got) != edited {
		t.Error("a second install overwrote the edited model catalog")
	}
	if got, _ := os.ReadFile(tmpl); string(got) != "my own persona" {
		t.Error("a second install overwrote an edited prompt template")
	}
	// Rotating secrets logs everybody out, so an install that was not asked to
	// must not do it.
	after := envMap(t, dir)
	for _, k := range []string{"JWT_SECRET", "COOKIE_SECRET", "ADMIN_TOKEN"} {
		if before[k] != after[k] {
			t.Errorf("a second install rotated %s, which logs every user out", k)
		}
	}
}

// The generated secrets are the deployment's only credentials.
func TestSecretsAreFreshAndPrivate(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	installInto(t, a)
	installInto(t, b)

	ea, eb := envMap(t, a), envMap(t, b)
	for _, key := range []string{"ADMIN_TOKEN", "JWT_SECRET", "COOKIE_SECRET"} {
		if ea[key] == "" {
			t.Errorf("%s was not generated", key)
		}
		if ea[key] == eb[key] {
			t.Errorf("two installs produced the same %s", key)
		}
	}
	st, err := os.Stat(filepath.Join(a, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0077 != 0 {
		t.Errorf(".env mode is %v — the signing key is readable by other users", st.Mode().Perm())
	}
}

// `daycore config <section>` may only write its own keys. One that quietly
// rewrote DB_DSN would turn "change the login provider" into an outage, and the
// person running it has no reason to re-read a file they meant to touch one
// line of.
func TestConfigSectionStaysInItsLane(t *testing.T) {
	dir := t.TempDir()
	installInto(t, dir)
	before := envMap(t, dir)

	env, _ := LoadEnv(filepath.Join(dir, ".env"))
	s := NewSession(strings.NewReader(strings.Repeat("\n", 20)), io.Discard, "en-US", dir, env, false, false)
	if err := Configure(s, []string{"channels"}); err != nil {
		t.Fatalf("config channels: %v", err)
	}

	after := envMap(t, dir)
	for k, v := range before {
		if k == "ONEBOT_WS_URL" || k == "ONEBOT_TOKEN" {
			continue
		}
		if after[k] != v {
			t.Errorf("`config channels` changed %s (%q → %q)", k, v, after[k])
		}
	}
}

// An unknown section is refused by name, with the list. Falling through to
// "run everything" would be catastrophic on a typo: `daycore config secretz`
// would rotate the signing key.
func TestConfigRefusesUnknownSection(t *testing.T) {
	dir := t.TempDir()
	installInto(t, dir)
	env, _ := LoadEnv(filepath.Join(dir, ".env"))
	s := NewSession(strings.NewReader("\n\n\n"), io.Discard, "en-US", dir, env, false, false)

	err := Configure(s, []string{"secretz"})
	if err == nil {
		t.Fatal("an unknown section was accepted")
	}
	if !strings.Contains(err.Error(), "secretz") {
		t.Errorf("the error does not name what was wrong: %v", err)
	}
}

// Every section's declared Keys must be the truth, because Configure enforces
// against that list — a section whose Keys are wrong either fails on a legal
// edit or lets an illegal one through.
func TestSectionKeysAreDeclaredNotGuessed(t *testing.T) {
	for _, sec := range Sections() {
		if len(sec.Keys) == 0 {
			t.Errorf("section %q declares no keys, so Configure cannot police it", sec.Name)
		}
		if sec.Run == nil || sec.TitleKey == "" {
			t.Errorf("section %q is incomplete", sec.Name)
		}
	}
}

// An unknown key an operator added by hand survives a config run. Silently
// dropping somebody's edit is worse than any question this flow asks.
func TestUnknownKeysSurvive(t *testing.T) {
	dir := t.TempDir()
	installInto(t, dir)
	path := filepath.Join(dir, ".env")
	raw, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(raw, []byte("\nHTTPS_PROXY=http://corp:3128\n")...), 0600); err != nil {
		t.Fatal(err)
	}

	env, _ := LoadEnv(path)
	s := NewSession(strings.NewReader(strings.Repeat("\n", 20)), io.Discard, "en-US", dir, env, false, false)
	if err := Configure(s, []string{"weather"}); err != nil {
		t.Fatal(err)
	}
	if envMap(t, dir)["HTTPS_PROXY"] != "http://corp:3128" {
		t.Error("a hand-added key was dropped by `daycore config`")
	}
}

// A DSN password must not reach the terminal through a driver error.
func TestPingErrorsRedactThePassword(t *testing.T) {
	err := PingDSN("postgres", "postgres://user:hunter2@127.0.0.1:1/nope?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Skip("something is listening on port 1")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("the DSN password reached the error text: %v", err)
	}
}
