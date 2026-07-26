package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"daycore/internal/ai"
)

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

func runInstall(dir string, force bool) error {
	dir = filepath.Clean(dir)
	scanner := bufio.NewScanner(os.Stdin)

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
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".tmpl") {
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
	sb.WriteString(fmt.Sprintf("DATA_DIR=%s\n", dir))
	sb.WriteString(fmt.Sprintf("PROMPTS_DIR=%s/prompts\n", dir))

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
	fmt.Printf("  source %s/.env && ./daycore\n", dir)
	fmt.Println()
	fmt.Printf("  %sLite Build%s (no embedded files, smaller binary)\n", cBold+cWhite, cReset)
	fmt.Println()
	fmt.Println("  go build -tags lite -o daycore-lite ./cmd/daycore")
	fmt.Printf("  source %s/.env && ./daycore-lite\n", dir)
	fmt.Println()
	fmt.Printf("  %sSetup complete!%s\n\n", cGreen+cBold, cReset)
	return nil
}

func genHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
