package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"daycore/internal/ai"
	"daycore/internal/auth"
	"daycore/internal/config"

	_ "daycore/internal/ai/formats/anthropic"
	_ "daycore/internal/ai/formats/openai"
)

// runInstallInto drives the whole interactive flow with empty answers, i.e. the
// defaults a person gets by pressing return eight times.
func runInstallInto(t *testing.T, dir string) {
	t.Helper()
	out, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	stdout := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = stdout }()

	if err := install(dir, false, strings.NewReader(strings.Repeat("\n", 8))); err != nil {
		t.Fatalf("install: %v", err)
	}
}

func envFrom(t *testing.T, dir string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}

// The one rule this command has: whatever it writes must be enough to boot.
//
// It was not. install created an empty config/ and never wrote models.yaml,
// which LoadCatalog treats as fatal, so the Quick Start it printed produced
// `exit 1` — a setup command reporting success and handing over a directory
// that cannot start.
//
// This goes through config.Load rather than reading the .env by hand, because
// half the ways install can betray a deployment are invisible otherwise: a key
// written under the wrong name, a seeded catalog whose ids do not match what
// DEFAULT_CHAT_MODEL falls back to, a path that only resolves relative to the
// source tree. Those all read fine as text and fail at startup.
func TestInstalledTreeBoots(t *testing.T) {
	dir := t.TempDir()
	runInstallInto(t, dir)
	env := envFrom(t, dir)
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("the generated .env does not load: %v", err)
	}

	catalog, err := ai.LoadCatalog(cfg.ModelsConfigPath, cfg.DefaultChatModel, cfg.DefaultVisionModel, cfg.DefaultPlannerModel)
	if err != nil {
		t.Fatalf("a freshly installed tree cannot load its model catalog: %v", err)
	}
	if len(catalog.List()) == 0 {
		t.Error("the seeded catalog has no models, so every AI call would fail at first use")
	}
	if _, err := auth.LoadOAuthProviders(env["OAUTH_CONFIG"], ""); err != nil {
		t.Fatalf("a freshly installed tree cannot load its oauth config: %v", err)
	}
	prompts, err := ai.NewPromptService(nil)
	if err != nil {
		t.Fatalf("prompt service: %v", err)
	}
	n, err := prompts.LoadDiskDefaults(env["PROMPTS_DIR"])
	if err != nil {
		t.Fatalf("a freshly installed tree cannot load its prompts: %v", err)
	}
	if n == 0 {
		t.Error("PROMPTS_DIR is in .env but nothing was extracted there — the installer set up an override the server ignores")
	}
	b, err := ai.NewBoundaries()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.LoadDir(env["PROMPTS_DIR"]); err != nil {
		t.Fatalf("a freshly installed tree cannot load its L1 boundaries: %v", err)
	}
}

// Every file the install contract names is actually produced. Separate from the
// boot test because a file can exist and still be unusable, and because the
// required/optional split is the thing that decides whether a missing one is a
// crash or a missing feature.
func TestInstallWritesItsWholeFileSet(t *testing.T) {
	dir := t.TempDir()
	runInstallInto(t, dir)

	for _, f := range installFileSet {
		st, err := os.Stat(filepath.Join(dir, f.Path))
		if err != nil {
			t.Errorf("install did not write %s (required=%v)", f.Path, f.Required)
			continue
		}
		if st.Size() == 0 {
			t.Errorf("install wrote %s empty", f.Path)
		}
	}
}

// The paths in .env are absolute-ish rather than relative to the working
// directory, so the binary can be started from anywhere. config.Load's defaults
// are relative — `cd / && /opt/daycore/daycore` would look for
// ./config/models.yaml and exit 1 naming a path the operator never chose.
func TestInstalledEnvNamesTheInstallDir(t *testing.T) {
	dir := t.TempDir()
	runInstallInto(t, dir)
	env := envFrom(t, dir)

	for _, key := range []string{"MODELS_CONFIG", "OAUTH_CONFIG", "PROMPTS_DIR", "DB_DSN"} {
		if v := env[key]; !strings.Contains(v, dir) {
			t.Errorf("%s = %q does not name the install dir %q", key, v, dir)
		}
	}
	// STATIC_DIR must not be written as a bare empty assignment: getEnv treats
	// set-but-empty as unset, so it would read as "configured" while doing
	// nothing.
	if v, ok := env["STATIC_DIR"]; ok && v == "" {
		t.Error("`STATIC_DIR=` was written; getEnv ignores it, so it configures nothing")
	}
}

// Re-running install is a thing people do — to regenerate a lost admin token,
// or after an upgrade. It must not take back the operator's edits.
func TestInstallDoesNotOverwriteEdits(t *testing.T) {
	dir := t.TempDir()
	runInstallInto(t, dir)

	models := filepath.Join(dir, "config", "models.yaml")
	edited := "models:\n  - id: mine\n    format: openai\n    model: something\n"
	if err := os.WriteFile(models, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	tmpl := filepath.Join(dir, "prompts", "zh-CN", "persona.tmpl")
	if err := os.WriteFile(tmpl, []byte("my own persona"), 0644); err != nil {
		t.Fatal(err)
	}

	runInstallInto(t, dir)

	if got, _ := os.ReadFile(models); string(got) != edited {
		t.Error("a second install overwrote the edited model catalog")
	}
	if got, _ := os.ReadFile(tmpl); string(got) != "my own persona" {
		t.Error("a second install overwrote an edited prompt template")
	}

	// -force is the way to take the seed back, and it has to work or there is
	// no way to recover from a broken edit.
	out, _ := os.Open(os.DevNull)
	defer out.Close()
	stdout := os.Stdout
	os.Stdout = out
	err := install(dir, true, strings.NewReader(strings.Repeat("\n", 8)))
	os.Stdout = stdout
	if err != nil {
		t.Fatalf("install -force: %v", err)
	}
	if got, _ := os.ReadFile(models); string(got) == edited {
		t.Error("-force did not restore the seeded catalog")
	}
}

// The generated secrets are the deployment's only credentials, so they must be
// distinct per install and not readable by other users on the box.
func TestInstalledSecretsAreFreshAndPrivate(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	runInstallInto(t, a)
	runInstallInto(t, b)

	ea, eb := envFrom(t, a), envFrom(t, b)
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

// The seed is what a fresh deployment runs on, so it has to satisfy the same
// rules a hand-written catalog does. Checked here rather than in internal/ai
// because being installable is what makes it a seed.
func TestSeedsAreValid(t *testing.T) {
	dir := t.TempDir()
	models := filepath.Join(dir, "models.yaml")
	if err := os.WriteFile(models, ai.CatalogSeed(), 0644); err != nil {
		t.Fatal(err)
	}
	// The seed must satisfy the DEFAULTS, not just parse: DEFAULT_CHAT_MODEL
	// falls back to a fixed id, and a seed whose entries are named anything else
	// is a models.yaml that exists and still exits 1.
	def, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ai.LoadCatalog(models, def.DefaultChatModel, def.DefaultVisionModel, def.DefaultPlannerModel)
	if err != nil {
		t.Fatalf("the embedded catalog seed does not satisfy the config defaults: %v", err)
	}
	if !cat.HasVision() {
		t.Error("the seed declares no vision model, so image features are off out of the box")
	}

	oauth := filepath.Join(dir, "oauth.yaml")
	if err := os.WriteFile(oauth, auth.OAuthSeed(), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := auth.LoadOAuthProviders(oauth, "")
	if err != nil {
		t.Fatalf("the embedded oauth seed does not load: %v", err)
	}
	// Inert until edited — that is what makes writing it safe. A half-registered
	// provider would give users a login button that fails.
	if got := m.Providers(); len(got) != 0 {
		t.Errorf("the oauth seed enabled %v out of the box", got)
	}
}
