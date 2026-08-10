package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func adminHealth(t *testing.T, s *Server) (int, map[string]any) {
	t.Helper()
	rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// The reason exists for exactly one moment: storage is down and somebody needs
// to know why. Before this endpoint, DegradedReason() had one caller — a log
// line in main — while handlers_misc.go's comment promised the console could
// reach it. The promise was true of no code.
func TestDegradedHealthCarriesTheReasonTheConsoleWasPromised(t *testing.T) {
	s := degradedServer(t)
	s.EnterDegraded("open db (postgres): dial tcp 10.0.0.5:5432: connect: connection refused")

	code, out := adminHealth(t, s)
	if code != http.StatusOK {
		t.Fatalf("degraded health returned %d; the request itself succeeded and found something out", code)
	}
	if out["degraded"] != true {
		t.Error("degraded is not reported")
	}
	reason, _ := out["reason"].(string)
	if !strings.Contains(reason, "connection refused") {
		t.Errorf("reason = %q — this is the single most useful string in a broken deployment", reason)
	}
	// Degraded is one-way and somebody WILL look for a retry button. Saying so
	// is cheaper than the ten minutes they would spend looking.
	if rec, _ := out["recovery"].(string); !strings.Contains(rec, "restart") {
		t.Error("no recovery line; degraded does not clear itself and nothing says so")
	}
	if out["instance"] == "" || out["instance"] == nil {
		t.Error("no instance — health is per process, so two consoles disagree with no way to tell which machine")
	}
	if _, ok := out["uptimeSec"]; !ok {
		t.Error("no uptime; 'did this just restart' is how a crash loop is told from a config problem")
	}
}

// The public endpoint must stay terse. A driver error carries the DSN and a DSN
// carries a password — that is why the detailed one needs a credential.
func TestThePublicHealthEndpointStillSaysNothing(t *testing.T) {
	s := degradedServer(t)
	s.EnterDegraded("open db (postgres): password authentication failed for user \"daycore\" (pw=hunter2)")

	rec := adminReq(t, s, http.MethodGet, "/api/healthz", "", func(r *http.Request) {})
	body := rec.Body.String()
	if strings.Contains(body, "hunter2") || strings.Contains(body, "password authentication") {
		t.Errorf("the unauthenticated health endpoint leaked the driver error: %s", body)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("healthz in degraded mode returned %d, want 503 (a READINESS answer)", rec.Code)
	}
}

// Three states, not two. A process that booted fine and then lost its database
// is a state a boolean cannot express, and it is the one an operator is most
// likely to be staring at.
func TestHealthDistinguishesNeverOpenedFromWentAway(t *testing.T) {
	healthy := adminServer(t)
	_, out := adminHealth(t, healthy)
	if out["degraded"] != false || out["dbReachable"] != true {
		t.Errorf("a healthy server reports degraded=%v dbReachable=%v", out["degraded"], out["dbReachable"])
	}
	if _, has := out["reason"]; has {
		t.Error("a healthy server reported a reason")
	}

	down := degradedServer(t)
	down.EnterDegraded("open db: nope")
	_, out2 := adminHealth(t, down)
	if _, has := out2["dbReachable"]; has {
		t.Error("a degraded server reported dbReachable — there is no database to reach, and saying false implies one was tried")
	}
}

func TestAdminHealthNeedsACredential(t *testing.T) {
	rec := adminReq(t, adminServer(t), http.MethodGet, "/api/admin/health", "", func(r *http.Request) {})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401 — this endpoint says things healthz refuses to", rec.Code)
	}
}

// The driver error goes only to the root credential.
//
// ⚠️ This endpoint shipped without this distinction, and it was wrong for the
// reason its own doc comment states two paragraphs earlier and then walks past:
// a driver error routinely carries the DSN, and a DSN routinely carries a
// password. Withholding it from unauthenticated callers and handing it to every
// console user gets the boundary exactly half right.
//
// The root credential is ADMIN_TOKEN — an environment variable. Whoever holds
// it can read DB_DSN out of the same file, so there is nothing here they cannot
// already see. Everyone else gets the fact without the string.
func TestOnlyTheRootCredentialSeesTheDriverError(t *testing.T) {
	s := degradedServer(t)
	s.cfg.AdminToken = "the-real-token"
	s.EnterDegraded(`open db (postgres): password authentication failed for user "daycore" (pw=hunter2)`)

	// Root credential: sees it.
	rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
	})
	if !strings.Contains(rec.Body.String(), "hunter2") {
		t.Error("the root credential cannot see the driver error, which is the whole point of this endpoint")
	}

	// A console session — authorised, but not root.
	login := adminReq(t, s, http.MethodPost, "/api/admin/session", `{"token":"the-real-token"}`, nil)
	var cookie *http.Cookie
	for _, c := range login.Result().Cookies() {
		if c.Name == adminCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("login set no admin cookie")
	}
	rec2 := adminReq(t, s, http.MethodGet, "/api/admin/health", "", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: adminCookie, Value: cookie.Value})
	})
	if rec2.Code != http.StatusOK {
		t.Fatalf("a console session cannot read health at all: %d", rec2.Code)
	}
	body := rec2.Body.String()
	if strings.Contains(body, "hunter2") || strings.Contains(body, "password authentication") {
		t.Errorf("a non-root credential was given the driver error: %s", body)
	}
	// And it still learns the FACT, or the screen is useless.
	if !strings.Contains(body, "degraded") {
		t.Error("a non-root credential cannot even tell that the deployment is degraded")
	}
	if !strings.Contains(body, "reasonWithheld") {
		t.Error("nothing says the detail was withheld, so it reads as 'there is no reason'")
	}
}
