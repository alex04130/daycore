package setup

import (
	"bufio"
	"io"
	"os"
	"strings"

	"daycore/internal/i18n"
)

// Session is one interactive run: where input comes from, where output goes,
// which language to speak, and the .env being built.
type Session struct {
	In     *bufio.Scanner
	Out    io.Writer
	Locale string
	Dir    string // the install directory
	Env    *Env
	Force  bool
	// Color is off when the output is not a terminal. ANSI escapes in a piped
	// log are noise, and in a CI transcript they are noise nobody can grep past.
	Color bool
}

// NewSession builds a session. locale is already canonical.
func NewSession(in io.Reader, out io.Writer, locale, dir string, env *Env, force, color bool) *Session {
	return &Session{
		In: bufio.NewScanner(in), Out: out,
		Locale: locale, Dir: dir, Env: env, Force: force, Color: color,
	}
}

// ─── locale resolution ───────────────────────────────────────────────────────

// ResolveLocale picks the language for the CLI.
//
// # Why the CLI needs its own resolution
//
// Every other locale decision in this codebase reads the user's stored
// preference, and there is no user here — `install` runs before there is a
// database, and often before there is a config file. So it reads the operating
// system, which is the only signal available.
//
// Order: an explicit flag, then DAYCORE_LOCALE, then the POSIX locale
// variables, then the deployment default. LC_ALL outranks LANG because that is
// what POSIX says, and getting that backwards means the one variable a person
// sets to force a language is the one that loses.
//
// # Boundary: no fallback to "whatever is installed"
//
// If the requested language has no pack, the CLI says so once and continues in
// the default rather than silently switching. A person who set LANG=ja_JP and
// gets Chinese with no explanation will assume the flag is broken; a person who
// is told "no ja-JP pack found, using zh-CN — drop one in LOCALES_DIR" knows
// exactly what to do, and that sentence is the whole of the third-party
// language-pack story.
func ResolveLocale(flagValue string, fallback string) (locale string, requested string) {
	if fallback == "" {
		fallback = "zh-CN"
	}
	for _, cand := range []string{flagValue, os.Getenv("DAYCORE_LOCALE"), posixLocale()} {
		if cand == "" {
			continue
		}
		c := i18n.Canonical(cand)
		if c == "" {
			continue
		}
		for _, have := range i18n.Available() {
			if have == c {
				return c, c
			}
		}
		return fallback, c // asked for something we do not have
	}
	return fallback, ""
}

// posixLocale turns LC_ALL / LC_MESSAGES / LANG into a BCP-47-ish tag.
// "zh_CN.UTF-8" → "zh-CN"; "C" and "POSIX" mean "no preference", not a language.
func posixLocale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		if i := strings.IndexAny(v, ".@"); i >= 0 {
			v = v[:i]
		}
		return strings.ReplaceAll(v, "_", "-")
	}
	return ""
}

// LoadLanguagePacks adds <dir>/<locale>.json to the message catalog, so a
// third party can ship a translation of this installer without touching Go.
//
// Same directory and same format as the server's LOCALES_DIR, deliberately: one
// pack translates the CLI and the API both, and a translator who has done one
// has done the other. A missing directory is the normal case.
func LoadLanguagePacks(dir string) error { return i18n.Std().LoadDir(dir) }

// ─── prompting ───────────────────────────────────────────────────────────────

func (s *Session) line() string {
	if !s.In.Scan() {
		return ""
	}
	return strings.TrimSpace(s.In.Text())
}

// Ask prints a question with its default and returns the answer, or def when
// the answer is empty.
func (s *Session) Ask(key, def string, args ...any) string {
	q := i18n.Tf(key, s.Locale, args...)
	if def != "" {
		s.printf("  %s%s [%s]: %s", s.c(cCyan), q, def, s.c(cReset))
	} else {
		s.printf("  %s%s: %s", s.c(cCyan), q, s.c(cReset))
	}
	if v := s.line(); v != "" {
		return v
	}
	return def
}

// AskSecret is Ask for a value that should not be echoed back in the summary.
// It reads the same way — this is not a no-echo terminal read, and pretending
// otherwise would be worse than not claiming it.
func (s *Session) AskSecret(key string, args ...any) string {
	s.printf("  %s%s: %s", s.c(cCyan), i18n.Tf(key, s.Locale, args...), s.c(cReset))
	return s.line()
}

// Choose presents a numbered menu and returns the chosen option's id.
//
// Numbers rather than free text: a menu that accepts "postgres" also accepts
// "postgress", and the version of this flow that did fell through to a default
// branch that silently reset the engine to sqlite. A number is either in range
// or it is not.
func (s *Session) Choose(key string, options []Option, def string) string {
	s.printf("  %s%s%s\n", s.c(cBold), i18n.T(key, s.Locale), s.c(cReset))
	defIdx := 0
	for i, o := range options {
		mark := " "
		if o.ID == def {
			mark, defIdx = "*", i+1
		}
		s.printf("   %s%d)%s %s  %s%s%s\n", mark, i+1, "", i18n.T(o.LabelKey, s.Locale),
			s.c(cDim), i18n.T(o.HintKey, s.Locale), s.c(cReset))
	}
	for {
		ans := s.Ask(keyChoosePrompt, itoa(defIdx))
		n := atoi(ans)
		if n >= 1 && n <= len(options) {
			return options[n-1].ID
		}
		s.Warn(keyChooseOutOfRange, len(options))
	}
}

// Confirm is a yes/no question. def is what an empty answer means.
func (s *Session) Confirm(key string, def bool, args ...any) bool {
	hint := i18n.T(keyYesNoNoDefault, s.Locale)
	if def {
		hint = i18n.T(keyYesNoYesDefault, s.Locale)
	}
	s.printf("  %s%s %s%s%s ", s.c(cCyan), i18n.Tf(key, s.Locale, args...), s.c(cDim), hint, s.c(cReset))
	switch strings.ToLower(s.line()) {
	case "y", "yes", "是", "1":
		return true
	case "n", "no", "否", "0":
		return false
	}
	return def
}

// Option is one entry in a Choose menu.
type Option struct {
	ID       string
	LabelKey string
	HintKey  string
}
