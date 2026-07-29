package server

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// -update regenerates the table instead of checking it. `make api-surface`.
var updateSurface = flag.Bool("update", false, "rewrite the generated table in docs/API_SURFACE.md")

const surfaceDoc = "../../docs/API_SURFACE.md"

const (
	surfaceBegin = "<!-- BEGIN GENERATED ROUTES -->"
	surfaceEnd   = "<!-- END GENERATED ROUTES -->"
)

// renderSurface builds the generated section from the live registry. The file
// column comes from runtime.Caller inside registerRoutes, so it cannot drift
// from where the code actually is.
func renderSurface() (table string, routes, groups int) {
	all := RouteTable(&Server{})
	byGroup := map[string][]Route{}
	for _, r := range all {
		byGroup[r.Group] = append(byGroup[r.Group], r)
	}
	names := make([]string, 0, len(byGroup))
	for g := range byGroup {
		names = append(names, g)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("\n")
	for _, g := range names {
		rs := byGroup[g]
		// Path first, then method in request-lifecycle order — reading a
		// resource's verbs together beats reading them alphabetically.
		sort.Slice(rs, func(i, j int) bool {
			pi, pj := pathOf(rs[i].Pattern), pathOf(rs[j].Pattern)
			if pi != pj {
				return pi < pj
			}
			return verbRank(rs[i].Pattern) < verbRank(rs[j].Pattern)
		})
		fmt.Fprintf(&b, "## %s（%d 条）\n\n| 路由 | Handler 文件 |\n|---|---|\n", g, len(rs))
		for _, r := range rs {
			fmt.Fprintf(&b, "| `%s` | %s |\n", r.Pattern, r.File)
		}
		b.WriteString("\n")
	}
	return b.String(), len(all), len(byGroup)
}

func pathOf(pattern string) string {
	if _, p, ok := strings.Cut(pattern, " "); ok {
		return p
	}
	return pattern
}

func verbRank(pattern string) int {
	verb, _, _ := strings.Cut(pattern, " ")
	for i, v := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		if v == verb {
			return i
		}
	}
	return 9
}

// docs/API_SURFACE.md's generated table must match the registered routes.
//
// The table used to be hand-written with compressed notation
// ("POST /api/auth/register·login·logout"), which read nicely and made drift
// undetectable — by the time it was checked it was missing five whole groups
// (wishes, companion-history, temp-context, feedback, inbox), listed a route
// that does not exist (`GET /api/session/settings` — only PATCH is registered,
// so a frontend following the doc would have hit the SPA fallback), and put
// `GET /api/import/history` in the wrong file.
//
// Now the table is generated and this test is what keeps it honest. Regenerate
// with `make api-surface`.
func TestRouteSurfaceDocIsCurrent(t *testing.T) {
	raw, err := os.ReadFile(surfaceDoc)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)

	const begin, end = surfaceBegin, surfaceEnd
	i, j := strings.Index(doc, begin), strings.Index(doc, end)
	if i < 0 || j < 0 || j < i {
		t.Fatalf("the generated-table markers are missing from docs/API_SURFACE.md — did someone hand-edit it back? Run `make api-surface`.")
	}
	if *updateSurface {
		fresh, routes, groups := renderSurface()
		head := regenerateCounts(doc[:i], routes, groups)
		if err := os.WriteFile(surfaceDoc, []byte(head+begin+fresh+doc[j:]), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("regenerated %s: %d routes / %d groups", surfaceDoc, routes, groups)
		return
	}
	table := doc[i+len(begin) : j]

	// | `GET /api/plan` | handlers_plan.go |
	row := regexp.MustCompile("(?m)^\\|\\s*`([^`]+)`\\s*\\|\\s*(\\S+)\\s*\\|")
	documented := map[string]string{}
	for _, m := range row.FindAllStringSubmatch(table, -1) {
		documented[m[1]] = m[2]
	}

	registered := map[string]bool{}
	for _, r := range RouteTable(&Server{}) {
		registered[r.Pattern] = true
	}

	var missing, ghost []string
	for pat := range registered {
		if _, ok := documented[pat]; !ok {
			missing = append(missing, pat)
		}
	}
	for pat := range documented {
		if !registered[pat] {
			ghost = append(ghost, pat)
		}
	}
	sort.Strings(missing)
	sort.Strings(ghost)

	if len(missing) > 0 {
		t.Errorf("%d routes are served but not in docs/API_SURFACE.md — run `make api-surface`:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(ghost) > 0 {
		// The expensive direction: a documented route nothing serves sends a
		// frontend author to an endpoint that answers with the SPA fallback.
		t.Errorf("%d routes are documented but nothing serves them — run `make api-surface`:\n  %s",
			len(ghost), strings.Join(ghost, "\n  "))
	}

	// The header quotes the counts; a stale number there is a small lie in the
	// first thing anyone reads.
	groups := map[string]bool{}
	for _, r := range RouteTable(&Server{}) {
		groups[r.Group] = true
	}
	for _, want := range []string{
		itoaSurface(len(registered)) + " 条路由",
		itoaSurface(len(groups)) + " 个组",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("the header does not say %q — run `make api-surface`", want)
		}
	}
}

func itoaSurface(n int) string {
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

// regenerateCounts keeps the header's two numbers in step with the table. They
// are the first thing anyone reads, so a stale count there is a small lie in the
// most-read sentence.
func regenerateCounts(head string, routes, groups int) string {
	head = regexp.MustCompile(`\*\*\d+ 条路由`).ReplaceAllString(head, "**"+itoaSurface(routes)+" 条路由")
	return regexp.MustCompile(`\d+ 个组\*\*`).ReplaceAllString(head, itoaSurface(groups)+" 个组**")
}
