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
