package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"daycore/internal/resources"
	"daycore/internal/setup"
)

// The `install` and `config` subcommands.
//
// Thin on purpose: flags in, a Session out, and the flow itself lives in
// internal/setup where it can be driven by a test. The version of this that
// read os.Stdin from inside the flow was never tested, and it shipped a command
// that produced a directory the server could not start from.

// runSetup dispatches `daycore install` and `daycore config`, or reports that
// this is not a setup invocation.
func runSetup(args []string) (handled bool, err error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "install":
		return true, cmdInstall(args[1:])
	case "config":
		return true, cmdConfig(args[1:])
	}
	return false, nil
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	dir := fs.String("dir", ".", "directory to install into")
	force := fs.Bool("force", false, "overwrite files that already exist")
	lang := fs.String("lang", "", "language for this command (e.g. en-US); defaults to the system locale")
	locales := fs.String("locales", "", "directory of <locale>.json language packs")
	data := fs.String("data", "", "resource directory for lite builds (default: ./data next to the binary)")
	fetch := fs.Bool("fetch", false, "download the resource pack for this version (lite builds only)")
	fs.Parse(args)

	if *data != "" {
		resources.SetDataDir(*data)
	}
	if *fetch {
		if err := fetchDataPack(resources.DataDir()); err != nil {
			return err
		}
	}
	sess, err := newSession(*dir, *lang, *locales, *force)
	if err != nil {
		return err
	}
	return setup.Install(sess)
}

func cmdConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	dir := fs.String("dir", ".", "the deployment directory")
	lang := fs.String("lang", "", "language for this command")
	locales := fs.String("locales", "", "directory of <locale>.json language packs")
	fs.Parse(args)

	names := fs.Args()
	if len(names) == 0 {
		fmt.Fprintf(os.Stderr, "usage: daycore config [-dir <path>] <section>...\nsections: %s\n",
			strings.Join(setup.SectionNames(), " "))
		return fmt.Errorf("no section given")
	}
	sess, err := newSession(*dir, *lang, *locales, false)
	if err != nil {
		return err
	}
	return setup.Configure(sess, names)
}

// newSession resolves the language, loads any third-party language pack, and
// reads the existing .env.
//
// Language packs load before the first printed line, including the one that
// says a pack was not found — otherwise the message telling you how to get your
// language would itself be in the wrong language.
func newSession(dir, lang, localesDir string, force bool) (*setup.Session, error) {
	dir = filepath.Clean(dir)
	if localesDir == "" {
		localesDir = os.Getenv("LOCALES_DIR")
	}
	if err := setup.LoadLanguagePacks(localesDir); err != nil {
		return nil, err
	}

	locale, requested := setup.ResolveLocale(lang, os.Getenv("DEFAULT_PRIMARY_LOCALE"))
	env, err := setup.LoadEnv(filepath.Join(dir, ".env"))
	if err != nil {
		return nil, err
	}
	sess := setup.NewSession(os.Stdin, os.Stdout, locale, dir, env, force, isTerminal(os.Stdout))
	if requested != "" && requested != locale {
		sess.Warn(setup.KeyLocaleFallback, requested, locale, requested)
	}
	return sess, nil
}

// isTerminal reports whether ANSI escapes are worth emitting.
//
// Character-device detection rather than a TERM lookup: the case that matters
// is output redirected to a file or a CI log, where escapes are noise nobody
// can grep past, and that is exactly what a non-character device means.
func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}
