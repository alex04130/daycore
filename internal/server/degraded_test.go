package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daycore/internal/ai"
	"daycore/internal/auth"
	"daycore/internal/config"
	"daycore/internal/i18n"
)

// A Server with no store at all — which is what degraded boot produces.
func degradedServer(t *testing.T) *Server {
	t.Helper()
	ps, err := ai.NewPromptService(nil)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Deps{
		Config: &config.Config{
			AdminToken:     "the-real-token",
			DBType:         "postgres",
			DefaultLocales: i18n.Pair{Primary: "zh-CN", Secondary: "en-US"},
		},
		Logger:  discardLogger(),
		Prompts: ps,
		Tokens:  auth.NewTokenIssuer("degraded-test", time.Hour),
		// A real degraded process HAS a cookie signer: main builds it from the
		// config before it ever touches storage. Leaving it nil here made this
		// fixture describe a process that cannot exist, and the first test to
		// sign a cookie panicked in the helper rather than finding out anything
		// about the server.
		Cookies: auth.NewCookieSigner("degraded-test-cookie-secret"),
	})
	s.EnterDegraded("open db (postgres): dial tcp 10.0.0.5:5432: connect: connection refused")
	return s
}

func hit(t *testing.T, s *Server, method, path string, mut func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if mut != nil {
		mut(req)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// The point of degraded boot: a store that will not open used to take the
// process with it, which under any supervisor is a crash loop — the only
// evidence a log line somebody has to know to look for, and no way to reach the
// screen that would let you fix the configuration, because that screen is served
// by the process that keeps dying.
func TestDegradedServesTheConsoleAndRefusesEverythingElse(t *testing.T) {
	s := degradedServer(t)

	// Nothing that reads rows. And it must be a clean 503, not a panic caught by
	// recoverMW — the store is nil, so anything reaching a handler would
	// dereference it.
	for _, path := range []string{"/api/plan", "/api/chat/threads", "/api/proposals", "/api/moods", "/api/files"} {
		rec := hit(t, s, http.MethodGet, path, nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s answered %d, want 503", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "degraded") {
			t.Errorf("%s did not say why: %s", path, rec.Body.String())
		}
	}

	// Contract negotiation reads no rows, so a client can still find out what
	// this deployment is.
	if rec := hit(t, s, http.MethodGet, "/api/version", nil); rec.Code != http.StatusOK {
		t.Errorf("/api/version answered %d in degraded mode", rec.Code)
	}

	// The console's login path was built DB-free (F4a) precisely so it works
	// here. This is the assertion that keeps the two designs tied together.
	login := httptest.NewRequest(http.MethodPost, "/api/admin/session", strings.NewReader(`{"token":"the-real-token"}`))
	login.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, login)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin login failed with no database: %d %s — F4a's whole constraint was that it must not need one", rec.Code, rec.Body.String())
	}
}

// 503 with a status, and NO detail: this endpoint is unauthenticated and a
// driver error routinely carries the DSN, which routinely carries a password.
func TestDegradedHealthSaysSoWithoutLeaking(t *testing.T) {
	s := degradedServer(t)
	rec := hit(t, s, http.MethodGet, "/api/healthz", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("healthz answered %d in degraded mode, want 503 so a load balancer stops routing here", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", body["status"])
	}
	for _, leak := range []string{"10.0.0.5", "connection refused", "dial tcp"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("healthz leaked driver detail %q to an unauthenticated caller: %s", leak, rec.Body.String())
		}
	}
	// The operator still gets it — through the process, not the wire.
	if !strings.Contains(s.DegradedReason(), "10.0.0.5") {
		t.Error("the reason was not kept for the operator")
	}
}

// Degraded is entered once and never left. Watching the database and flipping
// back would need answers for a request mid-flip, for how many failures count,
// and for whether to re-run migrations on a live server — and that last one is
// exactly the operation that wants a human.
func TestDegradedIsOneWay(t *testing.T) {
	s := degradedServer(t)
	if !s.Degraded() {
		t.Fatal("setup")
	}
	// There is deliberately no exit. If somebody adds one, this is where they
	// will have to argue with the design.
	if _, ok := any(s).(interface{ LeaveDegraded() }); ok {
		t.Error("a way out of degraded mode was added; read degraded.go first — getting out is a restart")
	}
}

// A healthy server must be entirely unaffected: the middleware is on the hot
// path of every request.
func TestHealthyServerIsUntouched(t *testing.T) {
	s, sid := newAgentTestServer(t)
	if s.Degraded() {
		t.Fatal("a server with a working store reported degraded")
	}
	rec := hit(t, s, http.MethodGet, "/api/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("healthz = %d on a healthy server", rec.Code)
	}
	_ = sid
}

// A degraded process must survive a request from somebody who is logged in.
//
// ⚠️ It did not. userMW runs OUTSIDE degradedMW — it has to, so the identity
// middlewares cannot panic before the refusal is written — and it dereferenced
// s.store, which is nil in a degraded process. Any request carrying a VALID
// dc_auth cookie hit that nil and recoverMW turned it into a 500 that said
// nothing.
//
// Four things answered 500, and the list is what makes this bad rather than
// untidy: GET /api/version (the four frontends' handshake, listed in
// degradedRoutes precisely because it reads no rows), GET /api/healthz (how an
// orchestrator and an operator both ask what is wrong), GET /api/admin/health,
// and /admin — the console itself. Somebody whose browser held a login could
// not reach the one surface degraded boot exists to serve.
//
// The earlier probe missed it because an invalid cookie value fails to parse
// and never reaches the store. It takes a real token.
func TestDegradedSurvivesALoggedInBrowser(t *testing.T) {
	s := degradedServer(t)
	tok, err := s.tokens.Issue("u-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	// Expected codes, not "not 500": /api/healthz answers 503 in a degraded
	// process on purpose — that is a READINESS answer and it is the whole
	// signal. Asserting "< 500" would pass on a 503 that came from a panic
	// somewhere else, which is the shape of assertion this test exists because
	// of.
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/version", http.StatusOK},
		{"/api/healthz", http.StatusServiceUnavailable},
		{"/admin", http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.AddCookie(&http.Cookie{Name: authCookie, Value: tok})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s with a valid login cookie returned %d, want %d — a degraded process must still answer this",
				tc.path, rec.Code, tc.want)
		}
	}

	// And the admin surface, which is the whole reason the process is up.
	req := httptest.NewRequest(http.MethodGet, "/api/admin/health", nil)
	req.AddCookie(&http.Cookie{Name: authCookie, Value: tok})
	req.Header.Set("X-Admin-Token", "the-real-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("the admin health screen returned %d for a logged-in browser holding the admin token", rec.Code)
	}

	// A request that carries a VALID session cookie reaches requestLocale with a
	// real sid, and requestLocale reads the store.
	//
	// That is the second nil-store path and it is worse than it looks, because
	// it is called from the FIRST line of handlers whose own degraded check is
	// three lines below — handleAdminConfigPut is exactly that shape. So a
	// handler that had carefully considered degraded mode still panicked before
	// reaching the consideration.
	signed := s.cookies.Sign("sess-1")
	put := httptest.NewRequest(http.MethodPut, "/api/admin/config", strings.NewReader(`{"settings":{"MaxUploadBytes":"1"}}`))
	put.AddCookie(&http.Cookie{Name: sessionCookie, Value: signed})
	put.Header.Set("X-Admin-Token", "the-real-token")
	put.Header.Set("Content-Type", "application/json")
	recPut := httptest.NewRecorder()
	h.ServeHTTP(recPut, put)
	if recPut.Code != http.StatusServiceUnavailable {
		t.Errorf("PUT /api/admin/config in degraded mode with a session cookie returned %d, want 503 — "+
			"the handler checks for degraded on line 4 and calls requestLocale on line 1", recPut.Code)
	}

	// A session cookie takes a different path (cookies.Verify, no store), but
	// assert it too rather than reasoning about it.
	req2 := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	req2.AddCookie(&http.Cookie{Name: sessionCookie, Value: "anything"})
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("/api/version with a session cookie returned %d, want 200", rec2.Code)
	}
}

// The proactive worker does not schedule anything in a degraded process.
//
// It already did not, but by accident: cron entries come from ScheduleUser,
// which runs off markAwake, which sits behind requireSession — and degradedMW
// refuses every route that requires a session. The worker was idle because a
// middleware three layers away kept requests from reaching it.
//
// That accident has an expiry date written into docs/ROADMAP.md: enumerating
// sessions at boot is a stated future want, and on the day somebody adds it the
// worker would start running jobs against a nil store. This makes the property
// explicit rather than emergent, and this test is what keeps it that way.
func TestTheWorkerStaysIdleWhileDegraded(t *testing.T) {
	s := degradedServer(t)
	w := NewWorker(s, nil)
	w.Start()
	defer w.Stop()

	if n := len(w.cron.Entries()); n != 0 {
		t.Errorf("a degraded process scheduled %d cron entries; every job they run reads rows", n)
	}
}
