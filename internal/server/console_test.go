package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func consoleGet(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	s.handleConsole(rec, req)
	return rec
}

// A build with no console says so in words.
//
// 404 with an empty body is the one answer that does not help: somebody typed
// /admin because they have a question, and "not found" leaves them wondering
// whether the URL is wrong, the build is wrong, or the server is broken. This
// is also the state of every fresh clone, since the committed placeholder
// exists precisely so `go build ./...` works before anybody runs npm.
func TestAConsolelessBuildExplainsItself(t *testing.T) {
	s := &Server{}
	rec := consoleGet(t, s, "/admin")
	body := rec.Body.String()

	if ConsoleBuilt() {
		// A real console is present, so this test has nothing to check — but say
		// so rather than passing silently, because a test that quietly does
		// nothing reads exactly like one that passed.
		if rec.Code != http.StatusOK {
			t.Errorf("a built console returned %d", rec.Code)
		}
		if !strings.Contains(strings.ToLower(body), "<!doctype html") && !strings.Contains(body, "<html") {
			t.Errorf("a built console served something that is not HTML: %.80s", body)
		}
		return
	}

	// The placeholder path. It still answers, and what it says is the point.
	if !strings.Contains(body, "console") {
		t.Errorf("the answer does not mention the console at all: %.120s", body)
	}
	for _, want := range []string{"web/console", "npm"} {
		if !strings.Contains(body, want) {
			t.Errorf("the answer does not say how to fix it (missing %q): %.200s", want, body)
		}
	}
	// And it must not claim the API is broken — that is the wrong conclusion to
	// lead an operator to when only the static bundle is missing.
	if strings.Contains(strings.ToLower(body), "internal server error") {
		t.Error("a missing console bundle was reported as a server error")
	}
}

// ConsoleBuilt must not count the placeholder.
//
// The startup line reports it, and an operator told "console: yes" who then
// finds a page explaining that there is no console has been told a lie by the
// one line they were supposed to be able to trust.
func TestConsoleBuiltDoesNotCountThePlaceholder(t *testing.T) {
	rec := consoleGet(t, &Server{}, "/admin")
	isPlaceholder := strings.Contains(rec.Body.String(), consolePlaceholderMark)
	if isPlaceholder && ConsoleBuilt() {
		t.Error("the committed placeholder is being reported as a built console")
	}
	if !isPlaceholder && !ConsoleBuilt() {
		t.Error("a real console is served but ConsoleBuilt() says no")
	}
}

// An unknown path under /admin is a client-side route, not a missing file.
// Without this the console's own navigation 404s on a page refresh, which is
// the classic SPA deployment bug.
func TestUnknownConsolePathsFallThroughToTheApp(t *testing.T) {
	if !ConsoleBuilt() {
		t.Skip("no console in this build")
	}
	rec := consoleGet(t, &Server{}, "/admin/providers")
	if rec.Code != http.StatusOK {
		t.Errorf("a client-side route returned %d — refreshing a console page would 404", rec.Code)
	}
}

// The console is static and public by design, so it must carry nothing that the
// API would refuse to serve unauthenticated. This checks the one thing a
// grep can check: no obvious credential material in the shipped bytes.
func TestConsoleBundleCarriesNoCredential(t *testing.T) {
	if !ConsoleBuilt() {
		t.Skip("no console in this build")
	}
	rec := consoleGet(t, &Server{}, "/admin")
	body := rec.Body.String()
	for _, forbidden := range []string{"ADMIN_TOKEN", "JWT_SECRET", "COOKIE_SECRET", "sk-"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the console index contains %q — this file is served unauthenticated", forbidden)
		}
	}
}

// The console must render in degraded mode. That is the state it exists for.
//
// ⚠️ This did not hold when it was written. degradedRoutes listed
// "/api/admin/" with the comment "the console" — true while the console WAS
// only an API, and quietly false the day a page appeared at /admin. The whole
// point of degraded boot is that somebody can open the console and read what is
// wrong; refusing the console with the same 503 as everything else would make
// the design self-defeating.
func TestTheConsoleItselfSurvivesDegradedMode(t *testing.T) {
	if !degradedAllowed("/admin") {
		t.Error("/admin is refused in degraded mode — the console cannot render in the one state it is for")
	}
	if !degradedAllowed("/admin/assets/index.js") {
		t.Error("console assets are refused in degraded mode, so the page loads without its script")
	}
	// And the rest is still refused, or degraded mode means nothing.
	for _, p := range []string{"/api/plan", "/api/chat/threads", "/"} {
		if degradedAllowed(p) {
			t.Errorf("%s is served in degraded mode", p)
		}
	}
}
