package server

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"daycore/internal/apipath"
	"daycore/internal/version"
)

// The gate over the hand-written TypeScript client.
//
// # ⚠️ Three mirrors, and until now none of them was checked
//
// packages/core is not a library. Two thirds of it — paths.ts, types.ts,
// endpoints.ts, boot.ts, manifest.ts — is a hand-maintained MIRROR of this
// backend's contract, and each of those files says so in its own header. The
// remaining third (i18n, the backend address, the build hash) is genuine shared
// mechanism that would be the same against any server.
//
// A mirror that lags is worse than no mirror: it is a confident wrong answer.
// paths.ts states the failure outright — "nothing in the build makes them meet
// — so when the backend's major moves, this file has to move with it, and the
// symptom of forgetting is every request 404ing at once."
//
// This file is what makes them meet. It is the same shape as
// internal/theme/frontend_manifest_test.go: Go reads TypeScript, because that is
// the only place the two languages can be compared.
//
// # ⚠️ It matters MORE after the frontends become submodules
//
// Today a contract change and its mirror are one commit, so the mirror mostly
// keeps up by hand. Once packages/core ships to four sibling repositories
// (docs/ROADMAP.md 阶段 κ), "mostly by hand" stops being available — the mirror
// will be edited in a different place from the thing it mirrors, possibly on a
// different day. These tests are the thing that survives that move.
//
// Skips when the package is not checked out, for the same reason the theme tests
// do: an optional directory must not become a mandatory one.

const corePathsTS = "../../packages/core/src/paths.ts"
const coreEndpointsTS = "../../packages/core/src/endpoints.ts"

// The version prefix the client puts on every request must be the one this
// backend serves.
//
// ⚠️ THE test of this file. Getting it wrong 404s every single request at once —
// which sounds loud, but the person who sees it is a user of a separately
// released frontend, and what they see is "the app is broken", not "these two
// disagree about a number".
func TestCoreClientUsesThisBackendsAPIPrefix(t *testing.T) {
	src, err := os.ReadFile(corePathsTS)
	if err != nil {
		t.Skipf("packages/core is not checked out here (%v)", err)
	}
	// ⚠️ The prefix must be BUILT from SPEAKS.major, not written again.
	//
	// It used to be the literal '/api/v2' sitting next to `MIN_API = 1`, and the
	// two disagreed: against a v1 backend the version check read `1 < 1`, passed,
	// and every request 404'd — the exact failure the check exists to prevent.
	// Deriving both from one constant is what makes that unrepresentable, so this
	// asserts the derivation rather than just the resulting string.
	if !regexp.MustCompile(`API_PREFIX\s*=\s*` + "`" + `/api/v\$\{SPEAKS\.major\}` + "`").MatchString(string(src)) {
		t.Fatal("paths.ts no longer builds API_PREFIX from SPEAKS.major — a second copy of " +
			"the contract version is exactly what this was consolidated to prevent")
	}
	m := regexp.MustCompile(`SPEAKS\s*=\s*\{\s*major:\s*(\d+)`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("paths.ts no longer declares SPEAKS as an object literal — has it changed shape?")
	}
	if got := "/api/v" + m[1]; got != apipath.Prefix {
		t.Errorf("packages/core sends every request to %q; this backend serves %q.\n"+
			"Bump SPEAKS in packages/core/src/paths.ts — the alternative is that\n"+
			"a released frontend 404s on its first request and reports it as an outage.",
			got, apipath.Prefix)
	}
}

// @daycore/core's package version IS which contract it speaks.
//
// # ⚠️ The scheme, and why "highest or lowest" turned out to be the wrong question
//
//	@daycore/core 的版本 = <APIVersion>.<APIMinor>.<patch>
//
//	major  the contract it speaks. NOT a range — paths.ts builds one prefix from
//	       it and every request carries that, so "the highest it supports" and
//	       "the lowest" are the same number. Pointing this package at an older
//	       major is not degraded operation, it is every request 404ing at once.
//	minor  the minor whose endpoints it uses — a genuine LOWER BOUND on the
//	       backend, because contract minors are additive: a backend reporting
//	       more is fine, one reporting less is missing calls this package makes.
//	patch  fixes that do not change which contract it speaks.
//
// This is what makes "the frontend pinned a core the backend does not match"
// reportable AT THE HANDSHAKE instead of as a screenful of 404s — which was the
// one new failure mode the repository split introduced (docs/ROADMAP.md 阶段 κ).
//
// ⚠️ Three places have to agree and none of them can see the other two:
// package.json's version, paths.ts's SPEAKS, and this backend. That is exactly
// the shape this file already exists for.
func TestCoreClientVersionMatchesTheContractItSpeaks(t *testing.T) {
	pkg, err := os.ReadFile("../../packages/core/package.json")
	if err != nil {
		t.Skipf("packages/core is not checked out here (%v)", err)
	}
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(pkg, &meta); err != nil {
		t.Fatalf("packages/core/package.json: %v", err)
	}
	parts := strings.SplitN(meta.Version, ".", 3)
	if len(parts) < 2 {
		t.Fatalf("packages/core version %q is not major.minor.patch", meta.Version)
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		t.Fatalf("packages/core version %q is not numeric", meta.Version)
	}
	if major != version.APIVersion || minor != version.APIMinor {
		t.Errorf("packages/core is version %s, so it claims to speak API %d.%d — "+
			"this backend serves %d.%d.\n"+
			"The version IS the claim: bump packages/core/package.json (and its tag, which\n"+
			"the four frontends pin) whenever the contract moves.",
			meta.Version, major, minor, version.APIVersion, version.APIMinor)
	}

	// And the constant the client actually branches on, which lives in a
	// different file and cannot see package.json.
	src, err := os.ReadFile(corePathsTS)
	if err != nil {
		t.Skipf("packages/core is not checked out here (%v)", err)
	}
	m := regexp.MustCompile(`SPEAKS\s*=\s*\{\s*major:\s*(\d+),\s*minor:\s*(\d+)`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("paths.ts no longer declares SPEAKS as an object literal — has it changed shape?")
	}
	sMajor, _ := strconv.Atoi(m[1])
	sMinor, _ := strconv.Atoi(m[2])
	if sMajor != major || sMinor != minor {
		t.Errorf("paths.ts says it speaks %d.%d; package.json says %d.%d. "+
			"One of them is what a consumer pins and the other is what the code does.",
			sMajor, sMinor, major, minor)
	}
}

// The paths the client refuses to version must be exactly the ones the backend
// refuses to version.
//
// ⚠️ Both directions, and each is broken in its own way. A path the client
// versions and the backend does not is a 404; a path the client leaves bare and
// the backend versions is ALSO a 404 — but the second one hits /api/version,
// which is the handshake, so the failure is "this build cannot introduce itself"
// rather than "one screen is broken".
func TestCoreClientAgreesOnWhichPathsAreUnversioned(t *testing.T) {
	src, err := os.ReadFile(corePathsTS)
	if err != nil {
		t.Skipf("packages/core is not checked out here (%v)", err)
	}
	m := regexp.MustCompile(`(?s)UNVERSIONED\s*=\s*\[(.*?)\]`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("paths.ts no longer declares UNVERSIONED as an array literal — has it changed shape?")
	}
	var got []string
	for _, q := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(m[1], -1) {
		got = append(got, q[1])
	}
	if len(got) == 0 {
		t.Fatal("read zero unversioned paths; this check is not reading what it thinks it is")
	}
	sort.Strings(got)

	for _, p := range got {
		if !apipath.IsUnversioned(p) {
			t.Errorf("packages/core sends %s unversioned; this backend versions it", p)
		}
	}
	// The other direction: anything the backend exempts and the client does not.
	// ⚠️ The OAuth callback is exempt by SHAPE rather than by name (see
	// internal/apipath), and no client ever calls it — the browser does. So the
	// comparison is over the named list only, which is why this walks the
	// client's list against IsUnversioned rather than comparing two slices.
	for _, p := range []string{"/api/version", "/api/healthz"} {
		if apipath.IsUnversioned(p) && !containsPath(got, p) {
			t.Errorf("this backend serves %s unversioned; packages/core versions it — "+
				"and %s in particular is the handshake, so a build cannot introduce itself", p, p)
		}
	}
}

// Every endpoint the client can call must be a route this backend serves.
//
// ⚠️ This closes the third side of a triangle. routes_test.go already checks the
// route table against api/openapi.yaml in both directions; nothing checked the
// hand-written CLIENT against either. So a wrapper could name a path that has
// never existed and every Go test would stay green — which is not hypothetical:
// an early draft of endpoints.ts called `/api/plan/today`, a string it had
// picked up from a TEST FIXTURE.
//
// ⚠️ Deliberately one-directional. A route with no wrapper is normal and always
// will be — the admin surface alone is 30-odd routes no user-facing frontend
// should ever call. Requiring coverage would push this package toward being a
// complete API client, which is the opposite of what it is for.
func TestEveryCoreClientCallHitsARealRoute(t *testing.T) {
	src, err := os.ReadFile(coreEndpointsTS)
	if err != nil {
		t.Skipf("packages/core is not checked out here (%v)", err)
	}

	known := map[string]bool{}
	for _, r := range RouteTable(&Server{}) {
		// Logical is the version-free form — the same shape the client writes
		// before apiPath() puts the prefix on.
		parts := strings.SplitN(r.Logical, " ", 2)
		if len(parts) == 2 {
			known[parts[1]] = true
		}
	}
	if len(known) < 50 {
		t.Fatalf("only %d routes registered; this check is not reading the real table", len(known))
	}

	// Every '/api/...' string literal and template literal in the file.
	//
	// ⚠️ Comments are stripped FIRST, by a scanner rather than a regex. The first
	// version scanned the raw text and reported `/api/plan/summary` — a path
	// named in a comment that exists precisely to say it has never existed. A
	// gate that flags prose is a gate whose failures get waved through, and the
	// wave-through habit is what kills the next real one.
	lit := regexp.MustCompile("['\"`](/api/[^'\"`]*)['\"`]")
	seen := map[string]bool{}
	for _, m := range lit.FindAllStringSubmatch(stripTSComments(string(src)), -1) {
		raw := m[1]
		// Drop a query string: routes are matched on the path.
		if i := strings.IndexAny(raw, "?"); i >= 0 {
			raw = raw[:i]
		}
		// `${encodeURIComponent(id)}` → `{id}`. What the placeholder is CALLED
		// does not matter to the mux, only that a segment is there.
		raw = regexp.MustCompile(`\$\{[^}]*\}`).ReplaceAllString(raw, "{x}")
		raw = strings.TrimSuffix(raw, "/")
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true

		if !matchesRoute(raw, known) {
			t.Errorf("packages/core calls %s, which this backend does not serve.\n"+
				"docs/API_SURFACE.md is generated from the real route table and is the\n"+
				"thing to check against — an early draft of this client took a path out\n"+
				"of a test fixture and nothing anywhere noticed.", raw)
		}
	}
	if len(seen) < 20 {
		t.Fatalf("only found %d client calls; the extraction is not reading endpoints.ts", len(seen))
	}
}

// stripTSComments removes line and block comments, leaving string and template
// literals intact.
//
// ⚠️ A scanner, not a regex, and the reason is the same one messages_test.go
// gives for using an AST walk: a regex over this has to decide whether a `//`
// is a comment or the middle of `https://`, and whether a `'` opens a string or
// sits inside a comment. Those are exactly the questions a scanner answers for
// free by remembering what it is inside of.
func stripTSComments(src string) string {
	var out strings.Builder
	out.Grow(len(src))
	const (
		code = iota
		lineComment
		blockComment
		str
	)
	state := code
	var quote byte
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch state {
		case code:
			switch {
			case c == '/' && i+1 < len(src) && src[i+1] == '/':
				state = lineComment
				i++
			case c == '/' && i+1 < len(src) && src[i+1] == '*':
				state = blockComment
				i++
			case c == '\'' || c == '"' || c == '`':
				state, quote = str, c
				out.WriteByte(c)
			default:
				out.WriteByte(c)
			}
		case lineComment:
			if c == '\n' {
				state = code
				out.WriteByte(c)
			}
		case blockComment:
			if c == '*' && i+1 < len(src) && src[i+1] == '/' {
				state = code
				i++
			}
		case str:
			out.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				i++
				out.WriteByte(src[i])
				continue
			}
			if c == quote {
				state = code
			}
		}
	}
	return out.String()
}

// matchesRoute compares a client path against the registered patterns, treating
// any `{...}` segment on either side as a wildcard.
func matchesRoute(path string, known map[string]bool) bool {
	want := strings.Split(strings.Trim(path, "/"), "/")
	for pattern := range known {
		got := strings.Split(strings.Trim(pattern, "/"), "/")
		if len(got) != len(want) {
			continue
		}
		ok := true
		for i := range got {
			g, w := got[i], want[i]
			if strings.HasPrefix(g, "{") || strings.HasPrefix(w, "{") {
				continue
			}
			if g != w {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func containsPath(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
