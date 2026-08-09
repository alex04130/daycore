package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"daycore/internal/ai"
	"daycore/internal/auth"
)

// `daycore install` — turn one binary into a running deployment.
//
// # What it is for
//
// The distribution unit is a single static binary (zero cgo; the frontend
// deploys separately, so the server is API-only). Everything else a deployment
// needs — prompt templates, the L1 boundary block, a model catalog, secrets —
// either rides inside the binary or is generated here. Nothing requires the
// source tree, a Go toolchain, or a package manager.
//
// # The rule this file has to keep
//
// **Whatever install writes must be enough to boot.** It was not: it created an
// empty config/ and never wrote config/models.yaml, which LoadCatalog treats as
// fatal, so following the Quick Start it printed gave `exit 1`. That is the
// worst shape a setup command can have — it reports success and hands you a
// directory that cannot start. installFileSet + TestInstalledTreeBoots exist to
// make that specific failure impossible to reintroduce.
//
// # Boundary
//
// Install writes files; it does not talk to anything. No network call, no
// database connection, no validation of the API keys it collects. A setup step
// that needs the internet cannot be used to set up an air-gapped deployment,
// and one that connects to the database turns "I typed the DSN wrong" into a
// failure before there is anything to read the error from. Wrong values surface
// at first boot, where the server can explain them.

// ── terminal helpers ──────────────────────────────────────────────────────

const (
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cGreen  = "\033[32m"
	cCyan   = "\033[36m"
	cYellow = "\033[33m"
	cWhite  = "\033[97m"
)

func heading(s string) { fmt.Printf("\n  %s%s%s\n", cBold+cWhite, s, cReset) }
func done(s string)    { fmt.Printf("  %s✓%s %s\n", cGreen, cReset, s) }
func hint(s string)    { fmt.Printf("  %s%s%s\n", cDim, s, cReset) }
func prompt(s string)  { fmt.Printf("  %s%s%s", cCyan, s, cReset) }

func readLine(s *bufio.Scanner) string {
	if !s.Scan() {
		return ""
	}
	return strings.TrimSpace(s.Text())
}

// ── install ────────────────────────────────────────────────────────────────

// installFileSet is every file install must leave behind, and what each one is
// for. It is a list rather than prose because it is checked: the test walks it
// and boots the result, so adding a required file without extracting it fails.
var installFileSet = []struct {
	Path     string // relative to the install dir
	Required bool   // true = the server will not start without it
}{
	{"config/models.yaml", true},       // LoadCatalog returns an error → exit 1
	{"config/oauth.yaml", false},       // missing = no social login, not an error
	{"prompts/boundaries.json", false}, // overlays the embedded L1 block
	{".env", true},                     // secrets; nothing else generates them
}

func runInstall(dir string, force bool) error {
	return install(dir, force, os.Stdin)
}

// install takes its input as a reader so the whole flow can be exercised by a
// test. Reading os.Stdin directly is what kept this command untested while it
// was producing a directory that could not boot.
func install(dir string, force bool, in io.Reader) error {
	dir = filepath.Clean(dir)
	scanner := bufio.NewScanner(in)

	fmt.Println()
	fmt.Printf("  %s╔══════════════════════════════════════════════╗%s\n", cCyan, cReset)
	fmt.Printf("  %s║%s  %sDaycore %s— One-Stop Installation%s           %s║%s\n", cCyan, cReset, cBold+cWhite, cDim, cReset+cWhite, cCyan, cReset)
	fmt.Printf("  %s╚══════════════════════════════════════════════╝%s\n", cCyan, cReset)
	fmt.Println()

	// ═══ Step 1: Database ═══
	heading("Step 1 — Database")
	fmt.Println()
	hint("  Supported engines: sqlite (default), postgres, mysql, mongodb")

	prompt("  Engine [sqlite]: ")
	engine := readLine(scanner)
	if engine == "" {
		engine = "sqlite"
	}

	var dsn string
	switch engine {
	case "sqlite":
		dsn = "file:" + filepath.Join(dir, "daycore.db") + "?_journal_mode=WAL"
		hint("  SQLite → " + dsn)
	case "postgres":
		prompt("  PostgreSQL DSN [postgres://user:pass@localhost:5432/daycore]: ")
		dsn = readLine(scanner)
		if dsn == "" {
			dsn = "postgres://user:pass@localhost:5432/daycore"
		}
	case "mysql":
		prompt("  MySQL DSN [user:pass@tcp(localhost:3306)/daycore]: ")
		dsn = readLine(scanner)
		if dsn == "" {
			dsn = "user:pass@tcp(localhost:3306)/daycore"
		}
	case "mongodb":
		prompt("  MongoDB DSN [mongodb://localhost:27017/daycore]: ")
		dsn = readLine(scanner)
		if dsn == "" {
			dsn = "mongodb://localhost:27017/daycore"
		}
	default:
		engine = "sqlite"
		dsn = "file:" + filepath.Join(dir, "daycore.db") + "?_journal_mode=WAL"
	}
	done("Database: " + engine)
	fmt.Println()

	// ═══ Step 2: Prompt templates ═══
	heading("Step 2 — Prompt Templates")
	fmt.Println()
	count := 0
	ai.WalkPromptFS(func(path string, d fs.DirEntry, err error) error {
		// boundaries.json rides along with the templates: same directory, same
		// PROMPTS_DIR override rule. Extracting it is what makes the L1 block
		// discoverable and editable at all — it has no console surface.
		if err != nil || d.IsDir() ||
			(!strings.HasSuffix(path, ".tmpl") && filepath.Base(path) != ai.BoundaryFile) {
			return nil
		}
		dest := filepath.Join(dir, path)
		os.MkdirAll(filepath.Dir(dest), 0755)
		if !force {
			if _, e := os.Stat(dest); e == nil {
				return nil
			}
		}
		data, _ := fs.ReadFile(ai.PromptFS(), path)
		os.WriteFile(dest, data, 0644)
		count++
		return nil
	})
	done(fmt.Sprintf("%d template files → %s/prompts/", count, dir))
	fmt.Println()

	// ═══ Step 2b: Config seeds ═══
	heading("Step 2b — Config Files")
	fmt.Println()
	if err := writeSeed(filepath.Join(dir, "config", "models.yaml"), ai.CatalogSeed(), force); err != nil {
		return err
	}
	done("config/models.yaml — the model catalog (required; edit the API base URLs and ids)")
	if err := writeSeed(filepath.Join(dir, "config", "oauth.yaml"), auth.OAuthSeed(), force); err != nil {
		return err
	}
	done("config/oauth.yaml — social login (optional; inert until you fill in client_id)")
	fmt.Println()

	// ═══ Step 3: API Keys ═══
	heading("Step 3 — AI Provider API Keys")
	fmt.Println()
	hint("  At least one key is required for AI features.")

	prompt("  DeepSeek API Key: ")
	dsKey := readLine(scanner)

	prompt("  OpenAI API Key:   ")
	oaiKey := readLine(scanner)

	prompt("  Anthropic API Key: ")
	antKey := readLine(scanner)
	done("API keys configured")
	fmt.Println()

	hint("  Weather defaults to free Open-Meteo (+ wttr.in fallback); keys optional.")
	prompt("  QWeather (和风) API Key [skip]: ")
	qweatherKey := readLine(scanner)
	prompt("  OpenWeatherMap API Key [skip]: ")
	owmKey := readLine(scanner)
	fmt.Println()

	// ═══ Step 4: External Channels ═══
	heading("Step 4 — External Channels (optional)")
	fmt.Println()
	hint("  OneBot 11 / NapCat bridges a normal QQ account.")

	prompt("  OneBot WS URL [skip]: ")
	onebotURL := readLine(scanner)

	onebotToken := ""
	if onebotURL != "" {
		prompt("  OneBot Access Token: ")
		onebotToken = readLine(scanner)
		done("QQ bot: " + onebotURL)
	} else {
		hint("  Skipped — add later in .env")
	}
	fmt.Println()

	// ═══ Step 5: Secrets ═══
	heading("Step 5 — Auto-Generated Secrets")
	fmt.Println()

	adminToken := genHex(24)
	jwtSecret := genHex(32)
	cookieSecret := genHex(32)

	fmt.Printf("  Admin Token      %s%s%s\n", cDim, adminToken, cReset)
	fmt.Printf("  JWT Secret       %s%s%s\n", cDim, jwtSecret, cReset)
	fmt.Printf("  Cookie Secret    %s%s%s\n", cDim, cookieSecret, cReset)
	done("Secrets generated")
	fmt.Println()

	// ═══ Step 6: Write .env ═══
	heading("Step 6 — Writing .env")
	fmt.Println()

	var sb strings.Builder
	sb.WriteString("# ─── Daycore Environment ───\n")
	sb.WriteString(fmt.Sprintf("# Generated: %s\n\n", "daycore install"))
	sb.WriteString("# ─── Database ───\n")
	sb.WriteString(fmt.Sprintf("DB_TYPE=%s\n", engine))
	sb.WriteString(fmt.Sprintf("DB_DSN=%s\n\n", dsn))
	sb.WriteString("# ─── Secrets (auto-generated) ───\n")
	sb.WriteString(fmt.Sprintf("ADMIN_TOKEN=%s\n", adminToken))
	sb.WriteString(fmt.Sprintf("JWT_SECRET=%s\n", jwtSecret))
	sb.WriteString(fmt.Sprintf("COOKIE_SECRET=%s\n\n", cookieSecret))
	if dsKey != "" {
		sb.WriteString("# ─── AI: DeepSeek ───\n")
		sb.WriteString(fmt.Sprintf("DEEPSEEK_API_KEY=%s\n\n", dsKey))
	}
	if oaiKey != "" {
		sb.WriteString("# ─── AI: OpenAI ───\n")
		sb.WriteString(fmt.Sprintf("OPENAI_API_KEY=%s\n\n", oaiKey))
	}
	if antKey != "" {
		sb.WriteString("# ─── AI: Anthropic ───\n")
		sb.WriteString(fmt.Sprintf("ANTHROPIC_API_KEY=%s\n\n", antKey))
	}
	sb.WriteString("# ─── Weather (free Open-Meteo default, wttr.in fallback) ───\n")
	switch {
	case qweatherKey != "":
		sb.WriteString("WEATHER_PROVIDER=qweather\n")
		sb.WriteString(fmt.Sprintf("QWEATHER_API_KEY=%s\n\n", qweatherKey))
	case owmKey != "":
		sb.WriteString("WEATHER_PROVIDER=openweathermap\n")
		sb.WriteString(fmt.Sprintf("OPENWEATHERMAP_API_KEY=%s\n\n", owmKey))
	default:
		sb.WriteString("WEATHER_PROVIDER=open-meteo\n\n")
	}
	if onebotURL != "" {
		sb.WriteString("# ─── QQ Bot (OneBot 11 / NapCat) ───\n")
		sb.WriteString(fmt.Sprintf("ONEBOT_WS_URL=%s\n", onebotURL))
		if onebotToken != "" {
			sb.WriteString(fmt.Sprintf("ONEBOT_TOKEN=%s\n", onebotToken))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("# ─── Server ───\n")
	sb.WriteString("HOST=0.0.0.0\n")
	sb.WriteString("PORT=8080\n\n")
	sb.WriteString("# ─── Paths ───\n")
	// Only keys the server actually reads. DATA_DIR used to be written here and
	// nothing ever read it, which is a quiet way to make an operator believe they
	// configured something.
	//
	// Written even where they match config.Load's defaults, because those
	// defaults are relative to the working directory, not to the install dir.
	// `cd /somewhere && /opt/daycore/daycore` would otherwise look for
	// ./config/models.yaml and exit 1 — a failure whose message names a path the
	// operator never chose.
	sb.WriteString("# Edited templates here override the embedded ones, file by file.\n")
	sb.WriteString(fmt.Sprintf("PROMPTS_DIR=%s/prompts\n", dir))
	sb.WriteString(fmt.Sprintf("MODELS_CONFIG=%s/config/models.yaml\n", dir))
	sb.WriteString(fmt.Sprintf("OAUTH_CONFIG=%s/config/oauth.yaml\n\n", dir))
	sb.WriteString("# ─── Frontend ───\n")
	// Commented, not empty. getEnv treats a set-but-empty variable as unset
	// (config.go), so `STATIC_DIR=` cannot express "serve nothing" — it would
	// silently keep the default. API-only is what happens anyway when the
	// directory has no index.html, and the startup line now reports what is
	// actually being served rather than what is configured.
	sb.WriteString("# The frontend deploys separately. Point this at a build directory\n")
	sb.WriteString("# to have this binary serve one too; leave it commented for API-only.\n")
	sb.WriteString("# STATIC_DIR=/srv/daycore-web/dist\n")

	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}
	done(".env → " + envPath)
	fmt.Println()

	// ═══ Step 7: Config dir ═══
	os.MkdirAll(filepath.Join(dir, "config"), 0755)

	// ═══ Step 8: Summary ═══
	fmt.Printf("  %s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", cCyan, cReset)
	fmt.Println()
	fmt.Printf("  %sQuick Start%s\n", cBold+cWhite, cReset)
	fmt.Println()
	fmt.Printf("  set -a; . %s/.env; set +a\n", dir)
	fmt.Println("  ./daycore")
	fmt.Println()
	hint("  .env is not read by the server — export it first, or let your")
	hint("  service manager do it (systemd: EnvironmentFile=" + filepath.Join(dir, ".env") + ").")
	fmt.Println()
	fmt.Printf("  %sNext%s\n", cBold+cWhite, cReset)
	fmt.Println()
	hint("  • Edit " + filepath.Join(dir, "config", "models.yaml") + " — the seeded")
	hint("    upstream model names go stale; check your vendor's current list.")
	hint("  • This binary serves the API only. The frontend deploys separately.")
	hint("  • Admin console: send the token above as X-Admin-Token, or")
	hint("    POST /api/admin/session to trade it for a short-lived cookie.")
	fmt.Println()
	fmt.Printf("  %sSetup complete!%s\n\n", cGreen+cBold, cReset)
	return nil
}

// writeSeed extracts one embedded config file.
//
// Never overwrites without -force, the same rule the templates follow, and for
// a sharper reason here: config/models.yaml is where the operator's own API
// base URLs and model ids live. Re-running install after adding a model — which
// people do, to regenerate a lost admin token — must not silently take it back.
//
// The write is reported as an error rather than swallowed. The template loop
// above ignores its write errors, which is survivable there (a missing template
// falls back to the embedded one); a missing models.yaml is exit 1 at first
// boot, far away from the command that caused it.
func writeSeed(path string, content []byte, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			hint("  " + path + " exists — kept (use -force to overwrite)")
			return nil
		}
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func genHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
