package setup

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"daycore/internal/i18n"
)

// EnvFileHeader goes at the top of every generated .env.
const EnvFileHeader = "Daycore — written by `daycore install`. Re-run `daycore config <section>` to change one part."

// Install runs every section against a fresh (or existing) directory and writes
// the files a deployment needs.
//
// The contract, and the only rule this command has: **what it writes must be
// enough to boot.** The version before this one created an empty config/ and
// never wrote models.yaml — which LoadCatalog treats as fatal — so following
// the Quick Start it printed produced exit 1. A setup command that reports
// success over a directory that cannot start is the worst shape available here,
// and TestInstalledTreeBoots exists to make that specific failure unrepeatable.
func Install(s *Session) error {
	// Before the first question: a run that cannot finish must not spend eight
	// sections of somebody's attention first, and must not leave half a
	// deployment behind on its way out.
	if err := Preflight(); err != nil {
		return err
	}
	s.Heading(keyBanner)

	for _, sec := range Sections() {
		s.Heading(sec.TitleKey)
		if err := sec.Run(s); err != nil {
			return err
		}
	}

	s.Heading(keySecFiles)
	n, err := ExtractPrompts(s.Dir, s.Force)
	if err != nil {
		return err
	}
	s.Done(keyFilesPrompts, n, filepath.Join(s.Dir, "prompts"))
	s.Hint(keyFilesPromptsNote)
	s.Done(keyFilesBoundaries, filepath.Join(s.Dir, "prompts", "boundaries.json"))
	s.Hint(keyFilesBoundariesNote)

	// Paths are written even where they equal config.Load's defaults, because
	// those defaults resolve against the working directory rather than the
	// install directory. `cd / && /opt/daycore/daycore` would otherwise look for
	// ./config/models.yaml and exit 1 naming a path nobody chose.
	s.Env.SetIn("Paths", "PROMPTS_DIR", filepath.Join(s.Dir, "prompts"),
		"Edited templates here override the built-in ones, file by file.")
	if dir := DataDirForEnv(); dir != "" {
		s.Env.SetIn("Paths", DataDirEnvKey, dir,
			"This build embeds nothing; every resource is read from here.")
	}

	envPath := filepath.Join(s.Dir, ".env")
	if err := s.Env.Save(envPath, EnvFileHeader); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}
	s.Done(keySummaryTitle)

	summary(s, envPath)
	return nil
}

// Configure re-runs named sections against an existing deployment and merges
// the result back into .env.
//
// # Boundary: a section may only write its own keys
//
// Checked, not trusted. `daycore config oauth` that quietly rewrote DB_DSN
// would turn "change the login provider" into an outage, and the person running
// it has no reason to re-read a file they only meant to touch one line of.
func Configure(s *Session, names []string) error {
	before := LoadEnvCopy(s.Env)
	s.Heading(keyBannerConfig)

	allowed := map[string]bool{}
	for _, name := range names {
		sec, ok := FindSection(name)
		if !ok {
			return fmt.Errorf(i18n.Tf(keyUnknownSection, s.Locale, name, strings.Join(SectionNames(), ", ")))
		}
		for _, k := range sec.Keys {
			allowed[k] = true
		}
		s.Heading(sec.TitleKey)
		if err := sec.Run(s); err != nil {
			return err
		}
	}

	added, changed, removed := s.Env.Diff(before)
	for _, k := range append(append(append([]string{}, added...), changed...), removed...) {
		if !allowed[k] {
			return fmt.Errorf("setup: section wrote %s, which it does not own — refusing to save", k)
		}
	}

	s.Rule()
	total := len(added) + len(changed) + len(removed)
	if total == 0 {
		s.Done(keyConfigNoChange)
		return nil
	}
	if err := s.Env.Save(filepath.Join(s.Dir, ".env"), EnvFileHeader); err != nil {
		return err
	}
	s.Done(keyConfigChanged, total)
	s.Hint(keyConfigRestart)
	return nil
}

// LoadEnvCopy snapshots an Env for diffing.
func LoadEnvCopy(e *Env) *Env {
	out := NewEnv()
	for _, k := range e.Keys() {
		out.Set(k, e.Get(k))
	}
	return out
}

func summary(s *Session, envPath string) {
	s.Rule()
	s.Heading(keySummaryStart)
	s.Plain("cd " + s.Dir + " && ./" + filepath.Base(exeName()))
	s.Hint(keySummaryEnvNote, envPath)
	s.Heading(keySummaryNext)
	s.Hint(keySummaryReconfig, strings.Join(SectionNames(), " / "))
	s.Hint(keySummaryAPIOnly)
}

func exeName() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "daycore"
}

// ExtractPrompts writes the prompt templates and boundaries.json under
// <dir>/prompts, skipping files that already exist unless force.
//
// Never overwriting by default matters more here than it looks: these are the
// files an operator edits, and people re-run install to recover a lost admin
// token. Taking their persona template back as a side effect of that would be
// silent and unrecoverable.
func ExtractPrompts(dir string, force bool) (int, error) {
	src := PromptResources()
	if src == nil {
		return 0, fmt.Errorf("no prompt resources available (see %s)", DataDirEnvKey)
	}
	count := 0
	err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".tmpl") && filepath.Base(path) != "boundaries.json" {
			return nil
		}
		dest := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if !force && exists(dest) {
			return nil
		}
		data, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}
