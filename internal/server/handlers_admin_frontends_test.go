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

// The two flags the handshake honours can now be SET, which is the whole point
// of this screen.
//
// ⚠️ They shipped one batch earlier with nothing able to set them — a switch
// that exists, is read, and has no hand on it. That is the same defect as a
// permission nothing checks, seen from the other side.
func TestPinningAndApprovingAreReachable(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	handshake(t, s, `{"familyId":"liuli","buildHash":"web-1","theme":{
		"tokens":[{"name":"--primary","kind":"color"}],"rules":"玻璃质感…"}}`)

	// Pinning freezes the token space — and the handshake honours it, which is
	// what makes this control mean anything.
	rec := adminReq(t, s, http.MethodPut, "/api/admin/frontends/families/liuli", `{"pinned":true}`, withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("pin: %d %s", rec.Code, rec.Body)
	}
	if hs, _ := handshake(t, s, `{"familyId":"liuli","theme":{"tokens":[{"name":"--new","kind":"color"}]}}`); hs.Code != http.StatusConflict {
		t.Errorf("after pinning, a new token was still accepted: %d", hs.Code)
	}

	// Approving lets the frontend's own text reach the model.
	rec = adminReq(t, s, http.MethodPut, "/api/admin/frontends/families/liuli", `{"rulesAccepted":true}`, withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if !fam.RulesAccepted {
		t.Error("approval did not stick")
	}
	// …and the handshake reports it, so the frontend knows which prompt it is
	// getting.
	if _, out := handshake(t, s, `{"familyId":"liuli","theme":{"tokens":[{"name":"--primary","kind":"color"}]}}`); out["rulesAccepted"] != true {
		t.Errorf("the handshake still reports rulesAccepted=%v", out["rulesAccepted"])
	}
}

// Omitted fields mean "leave alone".
//
// ⚠️ Two-state booleans would silently unpin a family every time somebody
// renamed one — and nothing would report it, because unpinning is not an error,
// it is just a family anybody can widen again.
func TestOmittedFieldsDoNotResetTheOthers(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	handshake(t, s, `{"familyId":"liuli","theme":{"tokens":[{"name":"--a","kind":"color"}]}}`)
	adminReq(t, s, http.MethodPut, "/api/admin/frontends/families/liuli", `{"pinned":true,"rulesAccepted":true}`, withRootHeader(s))

	// A rename that says nothing about the flags.
	rec := adminReq(t, s, http.MethodPut, "/api/admin/frontends/families/liuli", `{"displayName":"琉璃"}`, withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	fam, _ := s.store.Frontends().GetFamily(ctx, "liuli")
	if !fam.Pinned || !fam.RulesAccepted {
		t.Errorf("a rename reset the flags: pinned=%v accepted=%v", fam.Pinned, fam.RulesAccepted)
	}
	if fam.DisplayName != "琉璃" {
		t.Errorf("the rename did not apply: %q", fam.DisplayName)
	}
}

// Moving a build is what makes a new platform inherit a family's themes, and
// the target has to exist.
func TestMovingABuildRequiresARealTarget(t *testing.T) {
	s := adminServer(t)
	handshake(t, s, `{"familyId":"liuli","buildHash":"app-1","theme":{"tokens":[{"name":"--a","kind":"color"}]}}`)
	handshake(t, s, `{"familyId":"ting","theme":{"tokens":[{"name":"--b","kind":"color"}]}}`)

	// ⚠️ A family that does not exist is refused: the build would vanish from
	// every family's list and show up only among the orphans, with nothing
	// saying why.
	rec := adminReq(t, s, http.MethodPut, "/api/admin/frontends/builds/app-1/family", `{"familyId":"nowhere"}`, withRootHeader(s))
	if rec.Code != http.StatusNotFound {
		t.Errorf("moving into a non-existent family answered %d, want 404", rec.Code)
	}
	rec = adminReq(t, s, http.MethodPut, "/api/admin/frontends/builds/app-1/family", `{"familyId":"ting"}`, withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	if _, out := handshake(t, s, `{"familyId":"liuli","buildHash":"app-1","theme":{"tokens":[{"name":"--a","kind":"color"}]}}`); out["assignedFamilyId"] != "ting" {
		t.Errorf("the move did not survive the build's next handshake: %v", out["assignedFamilyId"])
	}
}

// A family with builds still pointing at it is not deleted.
func TestAFamilyInUseIsNotDeleted(t *testing.T) {
	s := adminServer(t)
	handshake(t, s, `{"familyId":"liuli","buildHash":"web-1","theme":{"tokens":[{"name":"--a","kind":"color"}]}}`)

	rec := adminReq(t, s, http.MethodDelete, "/api/admin/frontends/families/liuli", "", withRootHeader(s))
	if rec.Code != http.StatusConflict {
		t.Errorf("deleting a family in use answered %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "web-1") {
		t.Errorf("the refusal does not name what is still pointing at it: %s", rec.Body)
	}
	// Move the build away, and now it goes.
	handshake(t, s, `{"familyId":"ting","theme":{"tokens":[{"name":"--b","kind":"color"}]}}`)
	adminReq(t, s, http.MethodPut, "/api/admin/frontends/builds/web-1/family", `{"familyId":"ting"}`, withRootHeader(s))
	if rec := adminReq(t, s, http.MethodDelete, "/api/admin/frontends/families/liuli", "", withRootHeader(s)); rec.Code != http.StatusOK {
		t.Errorf("deleting an unused family answered %d: %s", rec.Code, rec.Body)
	}
}

// A theme is judged against the token space of the build that asked.
//
// ⚠️ And a request naming NO build falls back to the built-in space, so the
// current frontend — which does not handshake — behaves exactly as it did.
func TestThemesAreJudgedAgainstTheCallersFamily(t *testing.T) {
	s, sid := newAgentTestServer(t)
	s.cookies = auth.NewCookieSigner("frontend-test-secret")
	handshake(t, s, `{"familyId":"liuli","buildHash":"web-1","theme":{"tokens":[
		{"name":"--glass-alpha","kind":"ratio"},
		{"name":"--primary","kind":"color"}]}}`)

	post := func(build, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/themes", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Session-Token", s.cookies.Sign(sid))
		if build != "" {
			req.Header.Set(frontendBuildHeader, build)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	// A ratio token this family declares and the built-in space does not.
	body := `{"name":"t","variables":{"--glass-alpha":"0.72"}}`
	if rec := post("web-1", body); rec.Code != http.StatusOK {
		t.Errorf("a token the caller's family declares was refused: %d %s", rec.Code, rec.Body)
	}
	// The same request without the header falls back, where that token does not
	// exist.
	if rec := post("", body); rec.Code != http.StatusBadRequest {
		t.Errorf("the fallback space accepted a token it does not declare: %d %s", rec.Code, rec.Body)
	}
	// And a value of the WRONG KIND for that token is refused even though it
	// would be a fine colour — the kind is per token, not per family.
	if rec := post("web-1", `{"name":"t","variables":{"--glass-alpha":"#ffffff"}}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a colour was accepted for a ratio token: %d %s", rec.Code, rec.Body)
	}
	// An unknown build hash falls back rather than failing: a build the operator
	// deleted must still be able to read and write themes.
	if rec := post("never-seen", `{"name":"t2","variables":{"--primary":"#fff"}}`); rec.Code != http.StatusOK {
		t.Errorf("an unknown build could not write a theme against the fallback space: %d %s", rec.Code, rec.Body)
	}
}

// Reading the list and changing it are separate permissions.
func TestFrontendReadAndManageAreSeparate(t *testing.T) {
	s := adminServer(t)
	handshake(t, s, `{"familyId":"liuli","theme":{"tokens":[{"name":"--a","kind":"color"}]}}`)
	makeAdminUser(t, s, "watcher", []string{PermFrontendsRead})

	rec := adminReq(t, s, http.MethodGet, "/api/admin/frontends", "", withAdminCookie(t, s, "watcher"))
	if rec.Code != http.StatusOK {
		t.Fatalf("frontends.read could not list: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Families []struct {
			ID     string             `json:"id"`
			Tokens []domain.TokenSpec `json:"tokens"`
		} `json:"families"`
		Kinds            []map[string]string `json:"kinds"`
		FallbackFamilyID string              `json:"fallbackFamilyId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Families) != 1 || len(body.Families[0].Tokens) != 1 {
		t.Errorf("the list is not carrying the token space: %+v", body.Families)
	}
	// The kinds travel with it, or an operator reading `--x: ratio` has no way
	// to know what a ratio may be.
	if len(body.Kinds) < 6 {
		t.Errorf("only %d kinds were reported; the embedded floor is six", len(body.Kinds))
	}
	if body.FallbackFamilyID == "" {
		t.Error("the fallback family is not named; the console cannot say which space applies to a frontend that never handshakes")
	}

	if rec := adminReq(t, s, http.MethodPut, "/api/admin/frontends/families/liuli", `{"pinned":true}`, withAdminCookie(t, s, "watcher")); rec.Code != http.StatusForbidden {
		t.Errorf("frontends.read pinned a family: %d", rec.Code)
	}
}
