package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"daycore/internal/domain"
)

// A person, a group, and a console cookie minted from their own login.
func makeAdminUser(t *testing.T, s *Server, id string, perms []string) *domain.User {
	t.Helper()
	ctx := context.Background()
	u, err := s.store.Users().Upsert(ctx, &domain.User{ID: id, Name: &id})
	if err != nil {
		t.Fatal(err)
	}
	if perms != nil {
		role := "role-" + id
		if err := s.store.Roles().UpsertRole(ctx, domain.Role{Name: role, Permissions: perms}); err != nil {
			t.Fatal(err)
		}
		if err := s.store.Roles().AddMember(ctx, role, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	return u
}

// withAdminCookie puts a console session for this user on a request.
func withAdminCookie(t *testing.T, s *Server, userID string) func(*http.Request) {
	t.Helper()
	tok, err := s.tokens.IssueAdminUser(userID)
	if err != nil {
		t.Fatal(err)
	}
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: adminCookie, Value: tok}) }
}

func withRootCookie(t *testing.T, s *Server) func(*http.Request) {
	t.Helper()
	tok, err := s.tokens.IssueAdminRoot()
	if err != nil {
		t.Fatal(err)
	}
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: adminCookie, Value: tok}) }
}

func withRootHeader(s *Server) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("X-Admin-Token", s.cfg.AdminToken) }
}

// A permission grants its own route and nothing else.
//
// This is the assertion the whole batch exists for, and it is deliberately
// written against two routes rather than one: a check that only ever tests the
// allowed direction passes just as well when authorize returns true
// unconditionally.
func TestAPermissionOpensOneDoorAndNotTheNext(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "reader", []string{PermConfigRead})
	as := withAdminCookie(t, s, "reader")

	if rec := adminReq(t, s, http.MethodGet, "/api/admin/config", "", as); rec.Code != http.StatusOK {
		t.Errorf("config.read did not open GET /api/admin/config: %d %s", rec.Code, rec.Body)
	}
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", as); rec.Code != http.StatusForbidden {
		t.Errorf("config.read opened GET /api/admin/prompts (prompts.read): %d — permissions are not separating anything", rec.Code)
	}
	// 403 rather than 401: the difference tells a caller whether to log in or to
	// ask somebody for access, and a console that cannot tell them apart shows a
	// login form to somebody who is already logged in.
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("no credential answered %d, want 401", rec.Code)
	}
}

// Taking a permission away takes effect on the NEXT request, not when the
// console token expires.
//
// This is why permissions are read from the database rather than carried in the
// token, and it is the one property that would silently disappear if somebody
// "optimised" the lookup into the JWT — the tests would all still pass, because
// nothing else here re-reads within a session.
func TestRevokingAPermissionTakesEffectImmediately(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "temp", []string{PermPromptsRead})
	as := withAdminCookie(t, s, "temp")

	if rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", as); rec.Code != http.StatusOK {
		t.Fatalf("setup: prompts.read did not work: %d", rec.Code)
	}
	if err := s.store.Roles().UpsertRole(context.Background(), domain.Role{Name: "role-temp", Permissions: []string{}}); err != nil {
		t.Fatal(err)
	}
	// Same cookie, same TTL, no logout.
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", as); rec.Code != http.StatusForbidden {
		t.Errorf("a revoked permission still worked (%d) — the token is carrying authority the database has withdrawn", rec.Code)
	}
}

// The owner mark is a mark, not a role holding every permission.
func TestOwnerHoldsEverythingWithoutBeingInAnyGroup(t *testing.T) {
	s := adminServer(t)
	u := makeAdminUser(t, s, "boss", nil)
	if err := s.store.Users().SetOwner(context.Background(), u.ID, true); err != nil {
		t.Fatal(err)
	}
	as := withAdminCookie(t, s, u.ID)
	for _, path := range []string{"/api/admin/prompts", "/api/admin/config", "/api/admin/roles"} {
		if rec := adminReq(t, s, http.MethodGet, path, "", as); rec.Code != http.StatusOK {
			t.Errorf("owner was refused %s: %d %s", path, rec.Code, rec.Body)
		}
	}
	// And the mark is the ONLY thing doing it — no membership row exists.
	names, err := s.store.Roles().RolesOf(context.Background(), u.ID)
	if err != nil || len(names) != 0 {
		t.Fatalf("the owner is in %v; this test is not proving what it claims", names)
	}
}

// Root passes everything, including the route only it may call.
func TestRootCredentialPassesEverything(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "someone", []string{PermUsersRead})

	for _, mut := range []func(*http.Request){withRootHeader(s), withRootCookie(t, s)} {
		if rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", mut); rec.Code != http.StatusOK {
			t.Errorf("root was refused the prompt list: %d %s", rec.Code, rec.Body)
		}
		rec := adminReq(t, s, http.MethodPut, "/api/admin/users/someone/owner", `{"owner":true}`, mut)
		if rec.Code != http.StatusOK {
			t.Errorf("root could not set the owner mark: %d %s", rec.Code, rec.Body)
		}
	}
}

// Nothing short of the root credential reaches the owner mark — not even an
// owner, and not the permission that is owner-equivalent in effect.
//
// The bootstrap ("the first administrator is set from the console, not by
// registering first") and the recovery path both depend on this being
// unreachable from anything the database can grant.
func TestNobodyButRootSetsTheOwnerMark(t *testing.T) {
	s := adminServer(t)
	target := makeAdminUser(t, s, "target", nil)

	// Somebody holding the most powerful grantable permission.
	makeAdminUser(t, s, "editor", []string{PermRolesEdit, PermUsersAssign, PermUsersRead})
	rec := adminReq(t, s, http.MethodPut, "/api/admin/users/"+target.ID+"/owner", `{"owner":true}`, withAdminCookie(t, s, "editor"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("roles.edit reached the owner mark: %d — break-glass holders can now be minted from inside the model", rec.Code)
	}

	// And an existing owner cannot mint another.
	owner := makeAdminUser(t, s, "owner1", nil)
	if err := s.store.Users().SetOwner(context.Background(), owner.ID, true); err != nil {
		t.Fatal(err)
	}
	rec = adminReq(t, s, http.MethodPut, "/api/admin/users/"+target.ID+"/owner", `{"owner":true}`, withAdminCookie(t, s, owner.ID))
	if rec.Code != http.StatusForbidden {
		t.Errorf("an owner minted another owner: %d", rec.Code)
	}
}

// users.assign moves people between plain groups and cannot touch a group that
// carries any permission.
//
// Without this the split between users.assign and roles.edit is decorative:
// support staff could not change what "support" means, but could join it.
func TestAssignAloneCannotJoinAnAdminGroup(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	makeAdminUser(t, s, "support", []string{PermUsersAssign, PermUsersRead})
	target := makeAdminUser(t, s, "customer", nil)
	if err := s.store.Roles().UpsertRole(ctx, domain.Role{Name: "tier-pro"}); err != nil {
		t.Fatal(err)
	}
	if err := s.store.Roles().UpsertRole(ctx, domain.Role{Name: "ops", Permissions: []string{PermDBExport}}); err != nil {
		t.Fatal(err)
	}
	as := withAdminCookie(t, s, "support")

	// The commercial operation: a group with no permissions.
	rec := adminReq(t, s, http.MethodPut, "/api/admin/users/"+target.ID+"/roles", `{"roles":["tier-pro"]}`, as)
	if rec.Code != http.StatusOK {
		t.Errorf("users.assign could not move a customer between tiers: %d %s", rec.Code, rec.Body)
	}
	// The escalation: a group that grants something.
	rec = adminReq(t, s, http.MethodPut, "/api/admin/users/"+target.ID+"/roles", `{"roles":["tier-pro","ops"]}`, as)
	if rec.Code != http.StatusForbidden {
		t.Errorf("users.assign joined somebody to a group carrying db.export: %d — the split is decorative", rec.Code)
	}
	// Including themselves, which is the attack rather than the accident.
	rec = adminReq(t, s, http.MethodPut, "/api/admin/users/support/roles", `{"roles":["role-support","ops"]}`, as)
	if rec.Code != http.StatusForbidden {
		t.Errorf("support granted itself db.export: %d", rec.Code)
	}
	// Taking somebody OUT of an admin group is a change to who can do things,
	// and must need the same permission as putting them in.
	if err := s.store.Roles().AddMember(ctx, "ops", target.ID); err != nil {
		t.Fatal(err)
	}
	rec = adminReq(t, s, http.MethodPut, "/api/admin/users/"+target.ID+"/roles", `{"roles":["tier-pro"]}`, as)
	if rec.Code != http.StatusForbidden {
		t.Errorf("users.assign removed somebody from an admin group: %d", rec.Code)
	}
}

// Leaving an admin membership untouched is not an escalation, so it must not
// need roles.edit — otherwise support staff cannot change a customer's tier
// once anybody has ever been put in an admin group, which is the ordinary case.
func TestAssignCanEditTiersOfSomebodyWhoIsAlsoAnAdmin(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	makeAdminUser(t, s, "support2", []string{PermUsersAssign})
	target := makeAdminUser(t, s, "dualrole", nil)
	for _, r := range []domain.Role{
		{Name: "tier-free"}, {Name: "tier-paid"},
		{Name: "ops2", Permissions: []string{PermDBExport}},
	} {
		if err := s.store.Roles().UpsertRole(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"tier-free", "ops2"} {
		if err := s.store.Roles().AddMember(ctx, n, target.ID); err != nil {
			t.Fatal(err)
		}
	}
	rec := adminReq(t, s, http.MethodPut, "/api/admin/users/"+target.ID+"/roles",
		`{"roles":["tier-paid","ops2"]}`, withAdminCookie(t, s, "support2"))
	if rec.Code != http.StatusOK {
		t.Errorf("changing a tier while an unrelated admin membership stayed put was refused: %d %s", rec.Code, rec.Body)
	}
}

// An unknown permission id is refused rather than stored.
func TestARoleCannotGrantAPermissionThatDoesNotExist(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "editor2", []string{PermRolesEdit})
	rec := adminReq(t, s, http.MethodPut, "/api/admin/roles/ghost",
		`{"permissions":["config.raed"]}`, withAdminCookie(t, s, "editor2"))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a misspelled permission was accepted (%d) — the role now grants less than its definition claims", rec.Code)
	}
}

// Exchanging your own login for a console session, and being refused when there
// is nothing to exchange it for.
func TestAdminSessionFromYourOwnLogin(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "ops-person", []string{PermAILogsRead})
	makeAdminUser(t, s, "civilian", nil)

	login := func(userID string) func(*http.Request) {
		tok, err := s.tokens.Issue(userID, 0)
		if err != nil {
			t.Fatal(err)
		}
		return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: authCookie, Value: tok}) }
	}

	rec := adminReq(t, s, http.MethodPost, "/api/admin/session", "{}", login("ops-person"))
	if rec.Code != http.StatusOK {
		t.Fatalf("an admin could not exchange their own login: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Root        bool     `json:"root"`
		UserID      string   `json:"userId"`
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Root || body.UserID != "ops-person" || len(body.Permissions) != 1 {
		t.Errorf("the session says root=%v user=%q perms=%v", body.Root, body.UserID, body.Permissions)
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), adminCookie+"=") {
		t.Error("no console cookie was set")
	}

	// Somebody with no permissions gets refused rather than a credential that
	// can do nothing — a useless credential makes every later 403 look like a
	// console bug, and hands anybody who signs up an admin-scope token to probe
	// with.
	if rec := adminReq(t, s, http.MethodPost, "/api/admin/session", "{}", login("civilian")); rec.Code != http.StatusForbidden {
		t.Errorf("an ordinary user was given a console session: %d", rec.Code)
	}
}

// The whoami endpoint needs a credential and no permission.
func TestWhoAmINeedsACredentialAndNothingElse(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "nobody-much", []string{PermOverview})

	if rec := adminReq(t, s, http.MethodGet, "/api/admin/session", "", nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("whoami answered %d with no credential", rec.Code)
	}
	rec := adminReq(t, s, http.MethodGet, "/api/admin/session", "", withAdminCookie(t, s, "nobody-much"))
	if rec.Code != http.StatusOK {
		t.Fatalf("whoami refused a valid session: %d %s", rec.Code, rec.Body)
	}
	var view principalView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Permissions) != 1 || view.Permissions[0] != PermOverview {
		t.Errorf("whoami reported %v, want just %q", view.Permissions, PermOverview)
	}
}

// A console token minted for a user that no longer exists resolves to nothing.
func TestAConsoleTokenForADeletedUserGrantsNothing(t *testing.T) {
	s := adminServer(t)
	as := withAdminCookie(t, s, "never-existed")
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", as); rec.Code != http.StatusForbidden {
		t.Errorf("a token naming a non-existent user answered %d", rec.Code)
	}
}
