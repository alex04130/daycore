package theme

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
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
	pattern := extractTSPatterns(t, string(src))["shadow"]
	if pattern == "" {
		t.Fatal("汀's manifest no longer proposes a `shadow` kind — has it changed shape?")
	}

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

// extractTSPatterns pulls every proposed kind out of a manifest, keyed by name,
// undoing TypeScript's escaping so what comes back is the runtime value the
// frontend would actually send.
//
// ⚠️ Keyed by NAME rather than returning the first match. There is more than one
// frontend now and more than one kind per frontend — a helper that returned
// "the pattern" would silently check whichever happened to be written first,
// and pass while the other went unexamined.
func extractTSPatterns(t *testing.T, src string) map[string]string {
	t.Helper()
	re := regexp.MustCompile(`(?s)name:\s*'([\w-]+)',\s*(?://[^\n]*\n\s*)*pattern:\s*\n?\s*'((?:[^'\\]|\\.)*)'`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		unquoted, err := strconv.Unquote(`"` + strings.ReplaceAll(m[2], `"`, `\"`) + `"`)
		if err != nil {
			t.Fatalf("could not unescape %s's pattern: %v", m[1], err)
		}
		out[m[1]] = unquoted
	}
	// ⚠️ An empty result is NOT an error here, and it used to be.
	//
	// While every frontend proposed something, `t.Fatal("no proposed kinds
	// found")` was a fair guard against the regex silently ceasing to match. Then
	// 琉璃初版 arrived proposing none — on purpose, because it needs none — and
	// the guard turned into a helper asserting a fact about the world that had
	// stopped being true.
	//
	// The guard is right, it just belongs to the CALLERS that require kinds:
	// each of them names the kinds it expects and fails on a missing one, which
	// is a stricter check than "at least one of something".
	return out
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

// 纸屿 proposes two kinds, and neither is expressible with the embedded six —
// the SECOND independent frontend to land in that position on day one.
//
// ⚠️ Same shape as 汀's test and for the same reason: the pattern is written in
// TypeScript and enforced in Go, and this is the only place the two meet.
func TestZhiyusProposedKindsAcceptZhiyusOwnValues(t *testing.T) {
	src, err := os.ReadFile("../../web/zhiyu/src/manifest.ts")
	if err != nil {
		t.Skipf("纸屿 is not checked out here (%v)", err)
	}
	kinds := extractTSPatterns(t, string(src))
	for _, want := range []string{"easing", "radius"} {
		if kinds[want] == "" {
			t.Fatalf("纸屿's manifest no longer proposes %q", want)
		}
	}

	r := NewRegistry()
	var decl []Kind
	for name, pattern := range kinds {
		if _, cerr := CompileCheck(pattern); cerr != nil {
			t.Fatalf("纸屿's %s pattern would be refused by the backend: %v", name, cerr)
		}
		decl = append(decl, Kind{Name: name, Pattern: pattern})
	}
	if problems := r.SetDBKinds(decl); len(problems) > 0 {
		t.Fatalf("纸屿's kinds did not register: %v", problems)
	}

	// Every value 纸屿's own stylesheet ships for a token of each kind. Read from
	// the CSS, not restated — see the note on tingShadowValues.
	for kind, values := range cssValuesByKind(t) {
		for _, v := range values {
			if err := r.Validate(kind, v); err != nil {
				t.Errorf("纸屿 ships %s value %q and its own kind refuses it: %v", kind, v, err)
			}
		}
	}

	for _, bad := range []struct{ kind, why, value string }{
		{"easing", "steps() changes the KIND of motion, deliberately out", "steps(4, end)"},
		{"easing", "not an easing at all", "12px"},
		{"easing", "the character floor still applies", "cubic-bezier(0,0,1,1); color:red"},
		{"radius", "a bare number is not a length", "12"},
		{"radius", "not a radius", "cubic-bezier(0,0,1,1)"},
	} {
		if err := r.Validate(bad.kind, bad.value); err == nil {
			t.Errorf("%s: 纸屿's %s kind accepted %q", bad.why, bad.kind, bad.value)
		}
	}
}

// cssValuesByKind reads 纸屿's stylesheet for the tokens whose kind it proposes.
func cssValuesByKind(t *testing.T) map[string][]string {
	t.Helper()
	css, err := os.ReadFile("../../web/zhiyu/src/theme.css")
	if err != nil {
		t.Skipf("纸屿's stylesheet is not checked out here (%v)", err)
	}
	out := map[string][]string{}
	for kind, vars := range map[string][]string{
		"easing": {"--dc-ease"},
		"radius": {"--dc-r-card", "--dc-r-sm", "--dc-r-sheet", "--pin-shape"},
	} {
		for _, v := range vars {
			re := regexp.MustCompile(regexp.QuoteMeta(v) + `:\s*([^;}]+)`)
			for _, m := range re.FindAllStringSubmatch(string(css), -1) {
				val := strings.TrimSpace(m[1])
				// Skip var() indirections — they resolve at render time and are
				// not what a theme would set.
				if strings.HasPrefix(val, "var(") {
					continue
				}
				out[kind] = append(out[kind], val)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("纸屿's stylesheet declares none of the tokens whose kinds it proposes")
	}
	return out
}

// 长卷 proposes one kind, and it is the first that unions two PRIMITIVES rather
// than describing a new syntax: an inset is a ratio or a length, and no
// combinator over the embedded six expresses a union.
//
// ⚠️ It also declares a `list-of<length>` token and proposes NOTHING for it —
// that combinator already says what a dash pattern is. Worth asserting: a
// frontend that proposed a kind it did not need would put a decision on an
// operator's desk for nothing, and the tier is only cheap while that stays rare.
func TestLiulisProposedKindsAcceptLiulisOwnValues(t *testing.T) {
	src, err := os.ReadFile("../../web/liuli/src/manifest.ts")
	if err != nil {
		t.Skipf("长卷 is not checked out here (%v)", err)
	}
	kinds := extractTSPatterns(t, string(src))
	pattern := kinds["ratio-or-length"]
	if pattern == "" {
		t.Fatalf("长卷's manifest no longer proposes ratio-or-length (found %v)", keysOf(kinds))
	}
	if len(kinds) != 1 {
		t.Errorf("长卷 proposes %v; every extra kind is a decision on an operator's desk", keysOf(kinds))
	}

	r := NewRegistry()
	if problems := r.SetDBKinds([]Kind{{Name: "ratio-or-length", Pattern: pattern}}); len(problems) > 0 {
		t.Fatalf("did not register: %v", problems)
	}
	for _, ok := range []string{"0", "0.06", ".5", "1", "1.0", "12px", "1.5rem"} {
		if err := r.Validate("ratio-or-length", ok); err != nil {
			t.Errorf("长卷's inset kind refuses %q: %v", ok, err)
		}
	}
	for _, bad := range []string{"1.5", "-0.5", "12", "12pt", "red", "0.5; color:red"} {
		if err := r.Validate("ratio-or-length", bad); err == nil {
			t.Errorf("长卷's inset kind accepted %q", bad)
		}
	}

	// ⚠️ The dash token uses a COMBINATOR the deployment already has. If this
	// ever stops being true the frontend has to propose something, and the
	// silent failure is a token nothing can validate.
	if !r.Known("list-of<length>") {
		t.Error("list-of<length> is not known, so 长卷's dash token has no validator")
	}
	if err := r.Validate("list-of<length>", "4px 3px"); err != nil {
		t.Errorf("长卷 ships --cj-ghost-dash: 4px 3px and the combinator refuses it: %v", err)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Every kind a frontend's tokens DECLARE must be resolvable — by the embedded
// floor, by a combinator, or by that frontend's own proposal.
//
// ⚠️ The gap this closes was found by a mutation that survived: changing a
// token's kind from `list-of<length>` to an invented `dash-pattern` broke
// nothing, because the per-frontend tests only checked the PROPOSED patterns
// and never the kinds the tokens actually name. A token whose kind nothing can
// resolve is a token the handshake refuses — and the frontend learns that from
// a 400 at runtime rather than from a build.
//
// One test over all three, because the failure is identical in each and three
// copies is three places to forget the fourth frontend.
func TestEveryDeclaredTokenKindCanBeResolved(t *testing.T) {
	for _, app := range []struct{ name, dir string }{
		{"汀", "ting"},
		{"纸屿", "zhiyu"},
		{"长卷", "liuli"},
		{"琉璃初版", "liuli-classic"},
	} {
		src, err := os.ReadFile("../../web/" + app.dir + "/src/manifest.ts")
		if err != nil {
			t.Logf("%s is not checked out here; skipping", app.name)
			continue
		}
		proposed := extractTSPatterns(t, string(src))
		r := NewRegistry()
		var decl []Kind
		for name, pattern := range proposed {
			decl = append(decl, Kind{Name: name, Pattern: pattern})
		}
		if problems := r.SetDBKinds(decl); len(problems) > 0 {
			t.Errorf("%s: its own proposals do not compile: %v", app.name, problems)
			continue
		}

		kinds := extractTokenKinds(t, string(src))
		if len(kinds) < 5 {
			t.Errorf("%s: only found %d token kinds; this check is not reading the manifest", app.name, len(kinds))
		}
		for token, kind := range kinds {
			if !r.Known(kind) {
				t.Errorf("%s declares %s as kind %q, which nothing can resolve — "+
					"the handshake refuses it and the frontend finds out from a 400 at runtime",
					app.name, token, kind)
			}
		}
	}
}

// 琉璃初版 proposes NOTHING, and that is pinned rather than left to drift.
//
// The other three each needed a third-tier kind on day one — which is what made
// the tier worth building. 初版 needs none: every value it lets a theme touch is
// a colour, a length or a duration. The tier stays cheap only while a proposal
// is rare, so "we did not need one" is a fact worth a test: adding one here
// should be a decision somebody takes on purpose, not something that slips in
// while copying another frontend's manifest.
func TestLiuliClassicProposesNoKinds(t *testing.T) {
	src, err := os.ReadFile("../../web/liuli-classic/src/manifest.ts")
	if err != nil {
		t.Skipf("琉璃初版 is not checked out here (%v)", err)
	}
	if got := extractTSPatterns(t, string(src)); len(got) != 0 {
		t.Errorf("琉璃初版 now proposes %v. That is allowed, but it puts a decision on an "+
			"operator's desk — say why in the manifest and update this test deliberately.", got)
	}
}

// Every built-in theme must have a NAME in every frontend that shows a theme
// picker.
//
// ⚠️ The one cross-language gap the frontends' own i18n gate cannot close.
// web/*/src/locales.test.ts checks each `t()` family against a list in its own
// source — but the built-in theme ids come from HERE, so from the frontend's
// side the family has no knowable domain and the check has to wave it through.
// Add a fifth theme to domain.BuiltinThemes and the picker renders a bare
// `theme.whatever` with every frontend suite green.
//
// Written as "a frontend that has any theme.* key must have all of them", so it
// costs nothing to the three that do not show a picker, and starts applying to
// any that later does.
func TestEveryBuiltinThemeHasAName(t *testing.T) {
	// The ids, read from the source rather than restated — same rule as the
	// patterns above. domain is not imported here on purpose: internal/theme is
	// below internal/domain and must stay there.
	src, err := os.ReadFile("../domain/theme.go")
	if err != nil {
		t.Fatalf("cannot read the built-in theme list: %v", err)
	}
	m := regexp.MustCompile(`BuiltinThemes\s*=\s*\[\]string\{([^}]*)\}`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("domain.BuiltinThemes no longer looks like a string slice literal — has it moved?")
	}
	var ids []string
	for _, q := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(m[1], -1) {
		ids = append(ids, q[1])
	}
	if len(ids) == 0 {
		t.Fatal("read zero built-in themes; this check is not reading what it thinks it is")
	}

	for _, dir := range []string{"ting", "zhiyu", "liuli", "liuli-classic"} {
		for _, locale := range []string{"zh-CN", "en-US"} {
			path := "../../web/" + dir + "/public/locales/" + locale + ".json"
			raw, err := os.ReadFile(path)
			if err != nil {
				continue // not checked out, or ships a different set of packs
			}
			var pack map[string]string
			if err := json.Unmarshal(raw, &pack); err != nil {
				t.Errorf("%s: %v", path, err)
				continue
			}
			shows := false
			for k := range pack {
				if strings.HasPrefix(k, "theme.") {
					shows = true
					break
				}
			}
			if !shows {
				continue // no theme picker; nothing to keep in step
			}
			for _, id := range ids {
				if _, ok := pack["theme."+id]; !ok {
					t.Errorf("%s has a theme picker but no name for the built-in theme %q — "+
						"it renders the bare key `theme.%s`", path, id, id)
				}
			}
		}
	}
}

// extractTokenKinds reads the `{ name: '--x', kind: 'y' }` entries.
func extractTokenKinds(t *testing.T, src string) map[string]string {
	t.Helper()
	re := regexp.MustCompile(`\{\s*name:\s*'(--[\w-]+)',\s*kind:\s*'([^']+)'`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		out[m[1]] = m[2]
	}
	return out
}
