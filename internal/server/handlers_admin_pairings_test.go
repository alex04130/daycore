package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/domain"
)

// issuePairing creates one through the API and returns the key it was shown.
func issuePairing(t *testing.T, s *Server, body string) (key string, id string) {
	t.Helper()
	rec := adminReq(t, s, http.MethodPost, "/api/admin/pairings", body, withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("issue pairing: %d %s", rec.Code, rec.Body)
	}
	var out struct {
		Key     string `json:"key"`
		Pairing struct {
			ID string `json:"id"`
		} `json:"pairing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Key, out.Pairing.ID
}

func withPairingKey(key string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(pairingHeader, key) }
}

// An attached console authenticates, and gets exactly what its groups grant.
//
// This is the whole feature: a key this backend issued, carried to another
// program, resolving through the SAME roles a person's permissions resolve
// through. A pairing that got its own permission model would be a second place
// for "what does this role grant" to mean something slightly different.
func TestAnAttachedConsoleGetsItsGroupsAndNothingElse(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	if err := s.store.Roles().UpsertRole(ctx, domain.Role{
		Name: "watcher", Permissions: []string{PermOverview},
	}); err != nil {
		t.Fatal(err)
	}

	key, id := issuePairing(t, s, `{"name":"cluster-eu","roles":["watcher"]}`)
	if !strings.HasPrefix(key, domain.PairingKeyPrefix+"_"+id+"_") {
		t.Fatalf("the key does not carry its id in the clear: %q", key)
	}

	// It opens what its group grants…
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", withPairingKey(key)); rec.Code != http.StatusOK {
		t.Errorf("the attached console could not read the overview: %d %s", rec.Code, rec.Body)
	}
	// …and nothing else. Without this the feature is "a second root token".
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", withPairingKey(key)); rec.Code != http.StatusForbidden {
		t.Errorf("the attached console read the prompts with only overview.read: %d", rec.Code)
	}
	// Nor the root-only route, whatever it holds.
	if rec := adminReq(t, s, http.MethodPut, "/api/admin/users/x/owner", `{"owner":true}`, withPairingKey(key)); rec.Code != http.StatusForbidden {
		t.Errorf("an attached console reached the owner mark: %d", rec.Code)
	}

	// Changing its groups changes what it can do, on the next request.
	if err := s.store.Pairings().SetRoles(ctx, id, nil); err != nil {
		t.Fatal(err)
	}
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", withPairingKey(key)); rec.Code != http.StatusForbidden {
		t.Errorf("a pairing stripped of its groups still worked: %d", rec.Code)
	}
}

// The key is shown once, is never retrievable, and is not stored.
//
// ⚠️ Three separate claims, because each fails differently: a key echoed by the
// list endpoint leaks to anybody with pairings.read; a key stored in plaintext
// leaks with any backup; and a key that can be re-fetched makes "did it leak
// from us" a live question forever.
func TestThePairingKeyIsShownOnceAndNeverStored(t *testing.T) {
	s := adminServer(t)
	key, id := issuePairing(t, s, `{"name":"once"}`)
	secret := strings.SplitN(key, "_", 3)[2]

	// Not in the list.
	rec := adminReq(t, s, http.MethodGet, "/api/admin/pairings", "", withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Errorf("the list endpoint echoed the secret: %s", rec.Body)
	}
	// Nor the hash. It is a verifier rather than the thing itself, but it is
	// still the only object standing between a leaked row and an attached
	// console, and no operational question is answered by showing it.
	if strings.Contains(rec.Body.String(), hashPairingSecret(secret)) {
		t.Errorf("the list endpoint echoed the stored verifier: %s", rec.Body)
	}
	// Not in the row either — what is stored is a hash, and it is not the
	// secret.
	p, err := s.store.Pairings().Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if p.SecretHash == secret || p.SecretHash == "" {
		t.Errorf("the stored verifier is the secret itself (or empty): %q", p.SecretHash)
	}
	if p.SecretHash != hashPairingSecret(secret) {
		t.Error("the stored verifier does not match the hash of the issued secret")
	}
	// And the database browser will not serve it either.
	browsed := adminReq(t, s, http.MethodGet, "/api/admin/db/table/pairings", "", withRootHeader(s))
	if strings.Contains(browsed.Body.String(), p.SecretHash) {
		t.Errorf("the database browser served the secret hash: %s", browsed.Body)
	}
}

// A wrong or revoked key opens nothing, and a wrong one does not fall through.
func TestABadPairingKeyOpensNothing(t *testing.T) {
	s := adminServer(t)
	key, id := issuePairing(t, s, `{"name":"doomed"}`)

	for _, bad := range []string{
		"garbage",
		"dcp_" + id,            // no secret half
		"dcp_" + id + "_wrong", // right pairing, wrong secret
		"dcp_00000000-0000-0000-0000-000000000000_x", // no such pairing
		strings.Replace(key, "dcp_", "xxx_", 1),      // wrong prefix
	} {
		if rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", withPairingKey(bad)); rec.Code != http.StatusUnauthorized {
			t.Errorf("key %q answered %d, want 401", bad, rec.Code)
		}
	}

	// ⚠️ A WRONG pairing key must not fall through to another credential. The
	// admin header does the same thing for the same reason: otherwise a bad key
	// silently succeeds whenever the caller also holds something else, and
	// nobody ever learns the key is wrong.
	rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", func(r *http.Request) {
		r.Header.Set(pairingHeader, "dcp_"+id+"_wrong")
		r.Header.Set("X-Admin-Token", s.cfg.AdminToken)
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("a wrong pairing key fell through to the admin token: %d", rec.Code)
	}

	// Revoked is gone immediately, on the next request.
	if rec := adminReq(t, s, http.MethodDelete, "/api/admin/pairings/"+id, "", withRootHeader(s)); rec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body)
	}
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", withPairingKey(key)); rec.Code != http.StatusUnauthorized {
		t.Errorf("a revoked key still authenticated: %d", rec.Code)
	}
}

// Attaching a console to a group that carries permissions needs roles.edit.
//
// Same boundary as putting a PERSON into an admin group, and it matters more
// here: the thing being granted runs on somebody else's machine.
func TestAttachingToAnAdminGroupNeedsRolesEdit(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	for _, r := range []domain.Role{
		{Name: "plain"},
		{Name: "powerful", Permissions: []string{PermDBExport}},
	} {
		if err := s.store.Roles().UpsertRole(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	makeAdminUser(t, s, "pairer", []string{PermPairingsRead, PermPairingsManage})
	as := withAdminCookie(t, s, "pairer")

	if rec := adminReq(t, s, http.MethodPost, "/api/admin/pairings", `{"name":"a","roles":["plain"]}`, as); rec.Code != http.StatusOK {
		t.Errorf("pairings.manage could not attach a console to a plain group: %d %s", rec.Code, rec.Body)
	}
	rec := adminReq(t, s, http.MethodPost, "/api/admin/pairings", `{"name":"b","roles":["powerful"]}`, as)
	if rec.Code != http.StatusForbidden {
		t.Errorf("pairings.manage alone attached a console to a group carrying db.export: %d", rec.Code)
	}
	// And the same on the way in through the roles endpoint.
	_, id := issuePairing(t, s, `{"name":"c"}`)
	rec = adminReq(t, s, http.MethodPut, "/api/admin/pairings/"+id+"/roles", `{"roles":["powerful"]}`, as)
	if rec.Code != http.StatusForbidden {
		t.Errorf("pairings.manage alone granted an existing pairing db.export: %d", rec.Code)
	}
	// An unknown group is refused rather than stored and silently granting
	// nothing.
	rec = adminReq(t, s, http.MethodPost, "/api/admin/pairings", `{"name":"d","roles":["no-such-group"]}`, withRootHeader(s))
	if rec.Code != http.StatusNotFound {
		t.Errorf("an unknown group was accepted: %d %s", rec.Code, rec.Body)
	}
}

// Reading the list and issuing a key are separate permissions.
//
// The list names every external system with access — which somebody auditing
// needs and somebody attaching a new one does not.
func TestPairingReadAndManageAreSeparate(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "auditor", []string{PermPairingsRead})

	if rec := adminReq(t, s, http.MethodGet, "/api/admin/pairings", "", withAdminCookie(t, s, "auditor")); rec.Code != http.StatusOK {
		t.Errorf("pairings.read could not list: %d %s", rec.Code, rec.Body)
	}
	if rec := adminReq(t, s, http.MethodPost, "/api/admin/pairings", `{"name":"x"}`, withAdminCookie(t, s, "auditor")); rec.Code != http.StatusForbidden {
		t.Errorf("pairings.read issued a key: %d", rec.Code)
	}
}

// A degraded process accepts no pairing at all.
//
// ⚠️ Deliberate, and it is the reason ADMIN_TOKEN cannot be replaced by this:
// pairings live in the database, so when storage is down they cannot be read.
// Only the root credential works there — the arrangement degraded boot exists
// for.
func TestADegradedProcessAcceptsNoPairing(t *testing.T) {
	s := degradedServer(t)
	rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", withPairingKey("dcp_anything_atall"))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("a degraded process accepted a pairing key: %d %s", rec.Code, rec.Body)
	}
	// …and the root credential still works, which is the other half of the
	// claim.
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", withRootHeader(s)); rec.Code != http.StatusOK {
		t.Errorf("the root credential stopped working in degraded mode: %d %s", rec.Code, rec.Body)
	}
}

// A full pairing is root for AUTHORISATION, and only root can make one.
//
// This is the author's decision — "配对和集群按理来说应该有 admin token 的
// 权限的，不然集群管理就会乱掉" — and the reason it is GRANTED rather than
// automatic is the other half of the same argument: a key on somebody else's
// machine that is unconditionally root leaves no way to attach a console that
// merely watches, which is the most common reason to attach one at all.
func TestAFullPairingIsRootAndOnlyRootCanMakeOne(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	key, id := issuePairing(t, s, `{"name":"cluster"}`)

	// It starts with nothing.
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/health", "", withPairingKey(key)); rec.Code != http.StatusForbidden {
		t.Fatalf("a fresh pairing already had access: %d", rec.Code)
	}

	// ⚠️ Nothing short of root may grant it — not the permission that manages
	// pairings, not the one that is owner-equivalent for people.
	makeAdminUser(t, s, "manager", []string{PermPairingsRead, PermPairingsManage, PermRolesEdit})
	rec := adminReq(t, s, http.MethodPut, "/api/admin/pairings/"+id+"/full", `{"full":true}`, withAdminCookie(t, s, "manager"))
	if rec.Code != http.StatusForbidden {
		t.Errorf("pairings.manage + roles.edit granted root-equivalence: %d — the set of things that can "+
			"do everything now grows without anybody deciding it should", rec.Code)
	}
	// An owner cannot either, exactly as an owner cannot mint another owner.
	owner := makeAdminUser(t, s, "boss", nil)
	if err := s.store.Users().SetOwner(ctx, owner.ID, true); err != nil {
		t.Fatal(err)
	}
	if rec := adminReq(t, s, http.MethodPut, "/api/admin/pairings/"+id+"/full", `{"full":true}`, withAdminCookie(t, s, owner.ID)); rec.Code != http.StatusForbidden {
		t.Errorf("an owner granted root-equivalence to a pairing: %d", rec.Code)
	}

	// Root can.
	if rec := adminReq(t, s, http.MethodPut, "/api/admin/pairings/"+id+"/full", `{"full":true}`, withRootHeader(s)); rec.Code != http.StatusOK {
		t.Fatalf("root could not grant it: %d %s", rec.Code, rec.Body)
	}

	// And now the console really is root: it passes an ordinary permission it
	// was never granted, AND the route no permission reaches.
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", withPairingKey(key)); rec.Code != http.StatusOK {
		t.Errorf("a full pairing was refused an ordinary route: %d %s", rec.Code, rec.Body)
	}
	target := makeAdminUser(t, s, "somebody", nil)
	if rec := adminReq(t, s, http.MethodPut, "/api/admin/users/"+target.ID+"/owner", `{"owner":true}`, withPairingKey(key)); rec.Code != http.StatusOK {
		t.Errorf("a full pairing could not reach the root-only route: %d %s — a cluster console that "+
			"cannot reach them manages nothing, which is what this exists for", rec.Code, rec.Body)
	}

	// Revoking the mark takes effect on the next request, like every other
	// permission change.
	if rec := adminReq(t, s, http.MethodPut, "/api/admin/pairings/"+id+"/full", `{"full":false}`, withRootHeader(s)); rec.Code != http.StatusOK {
		t.Fatalf("root could not revoke it: %d %s", rec.Code, rec.Body)
	}
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/prompts", "", withPairingKey(key)); rec.Code != http.StatusForbidden {
		t.Errorf("a pairing stripped of root-equivalence still passed: %d", rec.Code)
	}
	// A missing field is a 400, not "false" — "grant root" and "send an empty
	// body" must not be the same request.
	if rec := adminReq(t, s, http.MethodPut, "/api/admin/pairings/"+id+"/full", `{}`, withRootHeader(s)); rec.Code != http.StatusBadRequest {
		t.Errorf("an empty body answered %d, want 400", rec.Code)
	}
}

// Root for authorisation is NOT root for disclosure.
//
// ⚠️ The distinction is deliberate and easy to erase. isRootCredential gates the
// one thing shown only to ADMIN_TOKEN — a driver error carrying the DSN
// password — and its rule is "things the holder could read anyway by looking at
// the same file". That is true of whoever holds the environment variable and
// FALSE of a console on another machine, however powerful it is otherwise.
func TestAFullPairingIsNotRootForDisclosure(t *testing.T) {
	s := adminServer(t)
	key, id := issuePairing(t, s, `{"name":"cluster"}`)
	if rec := adminReq(t, s, http.MethodPut, "/api/admin/pairings/"+id+"/full", `{"full":true}`, withRootHeader(s)); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, versionPath("/api/admin/health"), nil)
	req.Header.Set(pairingHeader, key)
	if s.isRootCredential(req) {
		t.Error("a full pairing satisfies isRootCredential — it would be shown a DSN password it " +
			"cannot read from the machine it runs on")
	}
	// The header holder still does.
	req2 := httptest.NewRequest(http.MethodGet, versionPath("/api/admin/health"), nil)
	req2.Header.Set("X-Admin-Token", s.cfg.AdminToken)
	if !s.isRootCredential(req2) {
		t.Error("the admin token stopped satisfying isRootCredential")
	}
}
