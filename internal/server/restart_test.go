package server

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// The button answers BEFORE it restarts, and it refuses without shutting down.
//
// Both halves matter and they fail in opposite directions:
//
//   - Restarting first and answering after means the console's connection dies
//     mid-response, so the operator sees a network error and cannot tell a
//     restart that worked from one that never started.
//   - Answering 200 and only then discovering the process cannot restart means
//     the process has already stopped serving with nothing to bring it back.
//     That is the one failure this whole feature exists to avoid, so the
//     preflight runs synchronously and its refusal is the response.
func TestRestartAnswersBeforeItActsAndRefusesWithoutStopping(t *testing.T) {
	s := adminServer(t)

	// No hook installed: 501, and nothing was called.
	rec := adminReq(t, s, http.MethodPost, "/api/admin/restart", "", withRootHeader(s))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("with no restarter installed: %d, want 501", rec.Code)
	}

	// A hook whose preflight fails: 503, and the caller is told why.
	s.SetRestarter(func() error { return errors.New("the binary is gone") })
	rec = adminReq(t, s, http.MethodPost, "/api/admin/restart", "", withRootHeader(s))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("a failed preflight answered %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "the binary is gone") {
		t.Errorf("the refusal does not carry the reason: %s", rec.Body)
	}

	// A hook that succeeds: 200, called exactly once, and the body says what to
	// expect — a console that just says "ok" leaves somebody staring at a
	// refused connection wondering whether they broke it.
	calls := 0
	s.SetRestarter(func() error { calls++; return nil })
	rec = adminReq(t, s, http.MethodPost, "/api/admin/restart", "", withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("restart answered %d: %s", rec.Code, rec.Body)
	}
	if calls != 1 {
		t.Errorf("the restarter was called %d times", calls)
	}
	if !strings.Contains(rec.Body.String(), "message") || len(rec.Body.String()) < 60 {
		t.Errorf("the response does not explain what happens next: %s", rec.Body)
	}
}

// Restarting needs its own permission, and config.write is not it.
//
// They fail differently: a bad config value is fixed from the same screen, and
// a restart that does not come back needs somebody with shell access on the
// machine. Merging them would look like tidying — two things about "changing
// how the server runs" — and nothing would fail.
func TestRestartIsNotBehindConfigWrite(t *testing.T) {
	s := adminServer(t)
	s.SetRestarter(func() error { return nil })
	makeAdminUser(t, s, "tuner", []string{PermConfigRead, PermConfigWrite})
	makeAdminUser(t, s, "operator", []string{PermRestart})

	if rec := adminReq(t, s, http.MethodPost, "/api/admin/restart", "", withAdminCookie(t, s, "tuner")); rec.Code != http.StatusForbidden {
		t.Errorf("config.write restarted the process: %d", rec.Code)
	}
	if rec := adminReq(t, s, http.MethodPost, "/api/admin/restart", "", withAdminCookie(t, s, "operator")); rec.Code != http.StatusOK {
		t.Errorf("server.restart could not restart: %d %s", rec.Code, rec.Body)
	}
}

// A degraded process can still restart itself.
//
// ⚠️ This is the case the button exists for: storage is down, the operator has
// just fixed DB_DSN in .env, and a restart is the only way to pick it up. A
// restart path that touched the store would stop working exactly when it is
// needed — the same trap degraded boot was designed around for the admin login.
func TestADegradedProcessCanStillRestart(t *testing.T) {
	s := degradedServer(t)
	called := false
	s.SetRestarter(func() error { called = true; return nil })

	rec := adminReq(t, s, http.MethodPost, "/api/admin/restart", "", withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("a degraded process refused to restart: %d %s", rec.Code, rec.Body)
	}
	if !called {
		t.Error("the restarter was not reached in degraded mode")
	}
	// And the store really is absent, so this proved what it claims.
	if s.store != nil {
		t.Fatal("degradedServer has a store; this test is not exercising degraded mode")
	}
}
