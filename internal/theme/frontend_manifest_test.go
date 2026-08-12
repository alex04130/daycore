package theme

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The kind 汀 proposes, checked against the registry that would have to accept it.
//
// # ⚠️ Why this test lives in Go and reads TypeScript
//
// A frontend's proposed pattern is written in one language and enforced in
// another, and nothing else in the build makes the two meet. 汀 can typecheck
// its manifest all day without discovering that the pattern it ships refuses
// the value it also ships — which is exactly what happened: the first draft
// required a unit on every offset, and 汀's own default shadow starts with a
// unitless `0`. An operator would have approved a pattern that broke the
// frontend that asked for it.
//
// # ⚠️ It READS the pattern rather than restating it
//
// A copy of the regex in this file would be a second source for one fact, and
// the copy is the one that stays correct while the real one drifts. Reading
// web/ting/src/manifest.ts means editing that file is what runs this test.
//
// Skips when the frontend is not checked out — 汀 is planned to become its own
// submodule (docs/ROADMAP.md 阶段 κ), and a backend test that fails because an
// optional directory is missing would make the optional thing mandatory.
func TestTingsProposedKindAcceptsTingsOwnValues(t *testing.T) {
	const manifestPath = "../../web/ting/src/manifest.ts"
	src, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Skipf("汀 is not checked out here (%v)", err)
	}
	pattern := extractTSPattern(t, string(src))

	if _, err := CompileCheck(pattern); err != nil {
		t.Fatalf("汀's proposed pattern would be refused by the backend outright: %v", err)
	}
	r := NewRegistry()
	if problems := r.SetDBKinds([]Kind{{Name: "shadow", Pattern: pattern}}); len(problems) > 0 {
		t.Fatalf("the kind did not register: %v", problems)
	}

	// Every shadow value 汀's own stylesheet ships. Read from the CSS for the
	// same reason the pattern is read from the manifest.
	for _, v := range tingShadowValues(t) {
		if err := r.Validate("shadow", v); err != nil {
			t.Errorf("汀 ships %q and its own kind refuses it: %v", v, err)
		}
	}

	for _, bad := range []struct{ why, value string }{
		{"no colour at all", "0 24px 70px"},
		{"not a shadow", "red"},
		{"the character floor still applies", "0 24px 70px url(x)"},
		{"inset is a different kind, deliberately", "inset 0 2px 4px rgba(0,0,0,.5)"},
		{"a declaration smuggled after a valid value", "0 24px 70px rgba(0,0,0,.5); color:red"},
		{"a bare number is not a length", "0 24 70px rgba(0,0,0,.5)"},
	} {
		if err := r.Validate("shadow", bad.value); err == nil {
			t.Errorf("%s: 汀's kind accepted %q", bad.why, bad.value)
		}
	}
}

// extractTSPattern pulls the `pattern:` string literal out of the manifest and
// undoes TypeScript's escaping, so what comes back is the runtime value the
// frontend would actually send.
func extractTSPattern(t *testing.T, src string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)pattern:\s*\n?\s*'((?:[^'\\]|\\.)*)'`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("no `pattern:` literal in web/ting/src/manifest.ts — has the manifest changed shape?")
	}
	// A TS single-quoted literal: \\ is one backslash, \' is a quote.
	unquoted, err := strconv.Unquote(`"` + strings.ReplaceAll(m[1], `"`, `\"`) + `"`)
	if err != nil {
		t.Fatalf("could not unescape the pattern literal: %v", err)
	}
	return unquoted
}

// tingShadowValues reads every --tg-shadow declaration out of 汀's stylesheet.
func tingShadowValues(t *testing.T) []string {
	t.Helper()
	css, err := os.ReadFile("../../web/ting/src/theme.css")
	if err != nil {
		t.Skipf("汀's stylesheet is not checked out here (%v)", err)
	}
	re := regexp.MustCompile(`--tg-shadow:\s*([^;}]+)`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(string(css), -1) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	if len(out) == 0 {
		t.Fatal("汀's stylesheet declares no --tg-shadow; this test would pass for the wrong reason")
	}
	return out
}
