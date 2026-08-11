package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/auth"
	"daycore/internal/domain"
)

// Two frontends, one session, two themes — and neither one retheming the other.
//
// ⚠️ This is the assertion the whole per-family split exists for. One current
// theme per session would have the phone and the desktop overwriting each other
// forever, and neither would be wrong.
func TestTwoFrontendsHoldTwoThemesAtOnce(t *testing.T) {
	s, sid := newAgentTestServer(t)
	s.cookies = auth.NewCookieSigner("theme-family-secret")
	handshake(t, s, `{"familyId":"liuli","buildHash":"web-1","theme":{"tokens":[{"name":"--primary","kind":"color"}]}}`)
	handshake(t, s, `{"familyId":"ting","buildHash":"app-1","theme":{"tokens":[{"name":"--primary","kind":"color"}]}}`)

	req := func(build, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Session-Token", s.cookies.Sign(sid))
		if build != "" {
			r.Header.Set(frontendBuildHeader, build)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, r)
		return rec
	}
	themeOf := func(build string) string {
		t.Helper()
		rec := req(build, http.MethodPost, "/api/session/init", "{}")
		if rec.Code != http.StatusOK {
			t.Fatalf("session init: %d %s", rec.Code, rec.Body)
		}
		var out struct {
			CurrentTheme string `json:"currentTheme"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.CurrentTheme
	}

	if rec := req("web-1", http.MethodPost, "/api/session/theme", `{"theme":"night"}`); rec.Code != http.StatusOK {
		t.Fatalf("liuli switch: %d %s", rec.Code, rec.Body)
	}
	if rec := req("app-1", http.MethodPost, "/api/session/theme", `{"theme":"nature"}`); rec.Code != http.StatusOK {
		t.Fatalf("ting switch: %d %s", rec.Code, rec.Body)
	}

	if got := themeOf("web-1"); got != "night" {
		t.Errorf("琉璃 is on %q, want night — the other frontend overwrote it", got)
	}
	if got := themeOf("app-1"); got != "nature" {
		t.Errorf("汀 is on %q, want nature", got)
	}
	// ⚠️ And the frontend that sends no header — the one shipping today — is
	// untouched by either of them.
	if got := themeOf(""); got == "night" || got == "nature" {
		t.Errorf("the fallback family is on %q; a handshaking frontend rethemed the one that never handshakes", got)
	}

	// PATCH settings takes the same route, or renaming the assistant on the
	// phone would write the desktop's theme.
	if rec := req("app-1", http.MethodPatch, "/api/session/settings", `{"currentTheme":"sunset"}`); rec.Code != http.StatusOK {
		t.Fatalf("settings patch: %d %s", rec.Code, rec.Body)
	}
	if got := themeOf("app-1"); got != "sunset" {
		t.Errorf("汀 is on %q after a settings patch, want sunset", got)
	}
	if got := themeOf("web-1"); got != "night" {
		t.Errorf("琉璃 became %q when 汀 patched its settings", got)
	}
}

// Themes are listed in the caller's family only, and deleting one resets only
// that family's current theme.
func TestThemesAreListedAndResetPerFamily(t *testing.T) {
	s, sid := newAgentTestServer(t)
	s.cookies = auth.NewCookieSigner("theme-family-secret")
	handshake(t, s, `{"familyId":"liuli","buildHash":"web-1","theme":{"tokens":[{"name":"--primary","kind":"color"}]}}`)

	req := func(build, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Session-Token", s.cookies.Sign(sid))
		if build != "" {
			r.Header.Set(frontendBuildHeader, build)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, r)
		return rec
	}
	list := func(build string) []domain.CustomTheme {
		t.Helper()
		rec := req(build, http.MethodGet, "/api/themes", "")
		var out struct {
			Themes   []domain.CustomTheme `json:"themes"`
			FamilyID string               `json:"familyId"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Themes
	}

	rec := req("web-1", http.MethodPost, "/api/themes", `{"name":"琉璃夜","variables":{"--primary":"#a78bfa"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var made domain.CustomTheme
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil {
		t.Fatal(err)
	}
	if made.FamilyID != "liuli" {
		t.Errorf("a theme made by a 琉璃 build landed in family %q", made.FamilyID)
	}

	if got := list("web-1"); len(got) != 1 {
		t.Errorf("琉璃 sees %d themes, want its own 1", len(got))
	}
	// ⚠️ The frontend that never handshakes must not suddenly see a theme built
	// against thirteen tokens it may not have.
	if got := list(""); len(got) != 0 {
		t.Errorf("the fallback family sees %d themes belonging to another family", len(got))
	}

	// On it, then delete it: only 琉璃 resets.
	req("web-1", http.MethodPost, "/api/session/theme", `{"theme":"`+made.ID+`"}`)
	req("", http.MethodPost, "/api/session/theme", `{"theme":"night"}`)

	rec = req("web-1", http.MethodDelete, "/api/themes/"+made.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	var del struct {
		Reset bool `json:"currentThemeReset"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &del)
	if !del.Reset {
		t.Error("deleting the theme 琉璃 was on did not reset it; the frontend now points at a dangling id")
	}
	sess, _ := s.store.Sessions().Get(context.Background(), sid)
	if sess.CurrentTheme != "night" {
		t.Errorf("the fallback family became %q when 琉璃 deleted a theme", sess.CurrentTheme)
	}
}
