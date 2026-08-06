package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"daycore/internal/i18n"
)

// The gate that makes the message catalog stick.
//
// Before this batch, 279 error messages were Chinese string literals inline at
// their call sites — readable, and completely untranslatable. The repo's own
// claim is that adding a language means dropping a JSON file into LOCALES_DIR;
// none of these could be reached that way, so the claim was true of the
// mechanism and false of the surface it was supposed to cover.
//
// Migrating them once fixes today. This test is what stops it happening again,
// which matters more: "remember to use the catalog" is a rule, and this repo
// has been bitten repeatedly by rules that depended on remembering.
func TestNoHardcodedUserFacingText(t *testing.T) {
	// s.writeErr's fourth argument, when it is a literal. writeErrL and
	// writeErrf take a key instead, so they never match.
	call := regexp.MustCompile(`s\.writeErr\(w,[^,]+,\s*"[a-z_]*",\s*"([^"]*)"`)

	entries, err := os.ReadDir("./")
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			m := call.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if !hasCJK(m[1]) {
				continue
			}
			offenders = append(offenders, filepath.Join("internal/server", name)+":"+itoa(i+1)+"  "+m[1])
		}
	}
	if len(offenders) > 0 {
		t.Errorf("%d user-visible message(s) bypass the catalog.\n"+
			"Register the text in messages.go and call s.writeErrL (or writeErrf for one\n"+
			"that carries a value) — a literal here cannot be translated by dropping a\n"+
			"file into LOCALES_DIR, which is the whole point of the three-layer catalog.\n\n%s",
			len(offenders), strings.Join(offenders, "\n"))
	}
}

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// How much of a language is actually translated, as a number.
//
// It does not assert a threshold — en-US sits far below 100% today and that is
// a known, deliberate state: the 241 error messages were registered with zh-CN
// only, because authoring their English is copy work and copy is frozen by
// project rule, while the mechanism is not. What this prints is the worklist
// size, so "how far along is en-US" stops being a feeling.
//
// i18n.Export(locale) hands a translator every key with its current fallback,
// which is the file they fill in and drop into LOCALES_DIR.
func TestReportCatalogCoverage(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		have, total := i18n.Coverage(locale)
		t.Logf("%s: %d/%d keys (%d untranslated)", locale, have, total, total-have)
	}
	if have, total := i18n.Coverage("zh-CN"); have != total {
		t.Errorf("zh-CN is the source language and must be complete: %d/%d", have, total)
	}
}
