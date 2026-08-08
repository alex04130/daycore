package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daycore/internal/ai"
	"daycore/internal/auth"
	"daycore/internal/config"
)

func adminServer(t *testing.T) *Server {
	t.Helper()
	s, _ := newAgentTestServer(t)
	s.cfg.AdminToken = "the-real-token"
	s.tokens = auth.NewTokenIssuer("admin-test-secret", time.Hour)
	// The admin endpoint used below needs one; the gate under test does not.
	ps, err := ai.NewPromptService(nil)
	if err != nil {
		t.Fatal(err)
	}
	s.prompts = ps
	return s
}

func adminReq(t *testing.T, s *Server, method, path, body string, mut func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if mut != nil {
		mut(req)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// The hole this batch closes: an unset ADMIN_TOKEN used to mean the whole
// configuration API was unauthenticated on any non-production deployment — every
// dev box, every staging instance, every self-host whose owner never set
// APP_ENV. It is also the branch that would have been most dangerous under the
// planned degraded boot, whose entire premise is serving the console while
// storage is down.
func TestNoConfigurationLeavesTheAdminAPIOpen(t *testing.T) {
	s := adminServer(t)
	s.cfg.AdminToken = ""
	s.cfg.Env = "development"

	rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("an unconfigured dev deployment answered %d — the admin API is open", rec.Code)
	}

	// And Load never produces that state: it invents one rather than opening.
	t.Setenv("APP_ENV", "development")
	t.Setenv("ADMIN_TOKEN", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdminToken == "" {
		t.Fatal("Load left AdminToken empty")
	}
	if !cfg.GeneratedAdminToken {
		t.Error("a generated token was not flagged, so nothing would print it — a credential nobody is told about is the same as none")
	}
	if len(cfg.AdminToken) < 32 {
		t.Errorf("generated token is %d chars; it is the only credential on the admin API", len(cfg.AdminToken))
	}

	// Production refuses to invent one: a token nobody wrote down is a token
	// nobody can rotate.
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "x")
	t.Setenv("COOKIE_SECRET", "y")
	if _, err := config.Load(); err == nil {
		t.Error("production booted with no ADMIN_TOKEN")
	}
}

func TestAdminLoginExchangesTheTokenForACookie(t *testing.T) {
	s := adminServer(t)

	if rec := adminReq(t, s, http.MethodPost, "/api/admin/session", `{"token":"wrong"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a wrong token got %d", rec.Code)
	}
	rec := adminReq(t, s, http.MethodPost, "/api/admin/session", `{"token":"the-real-token"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}

	var admin *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == adminCookie {
			admin = c
		}
	}
	if admin == nil {
		t.Fatal("login set no admin cookie")
	}
	// Each of these is the fix for a specific hole, so each is asserted.
	if !admin.HttpOnly {
		t.Error("the admin cookie is readable from JavaScript — an XSS on the console takes it, which is the exact problem this replaced")
	}
	if admin.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict — nobody should arrive at the console by following a link", admin.SameSite)
	}
	if admin.Path != "/api/admin" {
		t.Errorf("cookie path %q — it should not be sent to endpoints that do not need it", admin.Path)
	}
	if admin.MaxAge <= 0 || admin.MaxAge > int(auth.AdminTokenTTL.Seconds()) {
		t.Errorf("MaxAge = %d; the TTL is the only revocation this token has", admin.MaxAge)
	}
	// The raw token must never come back in the response body.
	if strings.Contains(rec.Body.String(), "the-real-token") {
		t.Error("the login response echoed the raw admin token")
	}

	// The cookie now authenticates.
	ok := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: adminCookie, Value: admin.Value})
	})
	if ok.Code != http.StatusOK {
		t.Errorf("the admin cookie did not authenticate: %d %s", ok.Code, ok.Body.String())
	}
}

// A user session JWT is signed with the same key and is otherwise valid. Without
// the scope claim it would authenticate against the admin API — and a user token
// is something anybody can obtain by signing up.
func TestAUserTokenIsNotAnAdminToken(t *testing.T) {
	s := adminServer(t)
	userTok, err := s.tokens.Issue("some-user", 0)
	if err != nil {
		t.Fatal(err)
	}
	rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: adminCookie, Value: userTok})
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("a user session token authenticated against the admin API: %d", rec.Code)
	}
	// And the reverse, so the boundary is guarded from both sides.
	adminTok, err := s.tokens.IssueAdmin("console")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.tokens.Parse(adminTok); err == nil {
		t.Error("an admin token parsed as a user session")
	}
}

// A cookie rides along on cross-site requests, so httpOnly buys confidentiality
// and not authority. SameSite=Strict is the first guard; this is the second.
func TestAdminCookieRefusesACrossOriginRequest(t *testing.T) {
	s := adminServer(t)
	tok, err := s.tokens.IssueAdmin("console")
	if err != nil {
		t.Fatal(err)
	}
	withCookie := func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: adminCookie, Value: tok})
	}

	rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", func(r *http.Request) {
		withCookie(r)
		r.Header.Set("Origin", "https://evil.example")
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("a cross-origin request with the cookie was accepted: %d", rec.Code)
	}

	// Same origin is fine, and so is no Origin at all — curl and CI do not send
	// one, and they are exactly who the header path serves.
	same := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", func(r *http.Request) {
		withCookie(r)
		r.Header.Set("Origin", "http://"+r.Host)
	})
	if same.Code != http.StatusOK {
		t.Errorf("a same-origin request was refused: %d", same.Code)
	}

	// The header path is CSRF-immune by construction (a custom header forces a
	// preflight), so an Origin must not break it.
	hdr := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", func(r *http.Request) {
		r.Header.Set("X-Admin-Token", "the-real-token")
		r.Header.Set("Origin", "https://evil.example")
	})
	if hdr.Code != http.StatusOK {
		t.Errorf("the machine path was broken by an Origin header: %d", hdr.Code)
	}
}

// Logging out clears the cookie, and must work when there is nothing to clear —
// a 401 on logout leaves somebody unable to drop a session they cannot use.
func TestAdminLogoutClearsAndIsAlwaysAvailable(t *testing.T) {
	s := adminServer(t)
	rec := adminReq(t, s, http.MethodDelete, "/api/admin/session", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout with no session: %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == adminCookie && c.MaxAge >= 0 {
			t.Errorf("logout did not expire the cookie: MaxAge=%d", c.MaxAge)
		}
	}
}
