package server

import (
	"strings"
	"testing"
)

// Every /api/admin/ route declares a permission, and every permission is used.
//
// # Why this gate and not a code review
//
// The failure it catches is **a line that does not exist**: somebody adds an
// admin endpoint and does not think about who may call it. That is invisible to
// review of the diff, because the diff looks complete — a handler, a route, a
// test. It is only visible by asking the same question of every route at once.
//
// This repository has measured the cost of not having such a gate. Thirteen
// admin routes were exempted in auth_surface_test.go with the annotation
// "X-Admin-Token"; the annotation was true and the exemption was wrong, and
// deleting an authorisation check from one of them left the entire suite green
// while the endpoint answered 200 to a request with no credential.
//
// The three directions are not decoration:
//
//	forward   a route with no declaration → denied, and this says so loudly
//	backward  a permission no route uses → a switch in the console that does nothing
//	quality   a damage line too short to decide from → a switch nobody can judge
func TestEveryAdminRouteDeclaresAPermission(t *testing.T) {
	routes := RouteTable(&Server{})
	seen := 0
	used := map[string]bool{}

	for _, r := range routes {
		if !strings.Contains(r.Pattern, "/api/admin/") {
			continue
		}
		seen++
		perm, declared := PermissionFor(r.Pattern)
		if !declared {
			t.Errorf("%s (group %q) declares no permission.\n"+
				"  An undeclared admin route is DENIED for everybody except the root credential, so this is\n"+
				"  not a security hole — it is an endpoint nobody can reach. Add it to routePermissions with\n"+
				"  the permission whose CONSEQUENCE matches, or with \"\" and a comment saying why it needs none.",
				r.Pattern, r.Group)
			continue
		}
		if perm == "" {
			continue // explicitly exempt; the map comment carries the reason
		}
		if !PermissionExists(perm) {
			t.Errorf("%s requires %q, which is not a registered permission — a typo here is a route nobody can ever reach", r.Pattern, perm)
		}
		used[perm] = true
	}

	// Without this the gate passes on an empty route table and asserts nothing —
	// the exact failure mode of every table-walking check.
	if seen < 15 {
		t.Fatalf("only saw %d admin routes; this gate is not looking at what it thinks it is", seen)
	}

	for _, p := range Permissions() {
		if !used[p.ID] {
			t.Errorf("permission %q is registered but no route requires it.\n"+
				"  That is a switch in the console that grants nothing — either wire it to the route it was\n"+
				"  meant for, or delete it. A permission nobody checks is worse than none: it reads like\n"+
				"  protection.", p.ID)
		}
	}
}

// A permission's damage line has to be something an operator can decide from.
//
// "manage config" is not. The person granting is deciding what somebody else
// can break, and they are usually deciding it quickly — so the sentence has to
// name the consequence, not the surface.
func TestEveryPermissionSaysWhatItCosts(t *testing.T) {
	perms := Permissions()
	if len(perms) < 10 {
		t.Fatalf("only %d permissions registered; this gate is not looking at what it thinks it is", len(perms))
	}
	for _, p := range perms {
		if n := len([]rune(p.Damage)); n < 12 {
			t.Errorf("%s: damage is %d characters (%q) — too short to grant from", p.ID, n, p.Damage)
		}
		for _, empty := range []string{"管理", "配置管理", "manage", "admin access"} {
			if strings.TrimSpace(p.Damage) == empty {
				t.Errorf("%s: %q says nothing about consequence", p.ID, p.Damage)
			}
		}
	}
}

// The two permissions whose separation IS the escalation boundary must stay
// separate.
//
// Holding both "change what a role can do" and "put a user in a group" is
// equivalent to holding everything: make a group all-powerful, then join it.
// Split, the commercial operation (moving a customer between tiers) can never
// leave the person performing it with more than they had.
//
// This test exists because merging them would look like tidying up — two
// permissions about groups, surely one is enough — and nothing about the merged
// version would fail.
func TestTheEscalationBoundaryStaysSplit(t *testing.T) {
	if PermRolesEdit == PermUsersAssign {
		t.Fatal("the two group permissions were merged; holding both is equivalent to owner")
	}
	// Neither is registered yet — their endpoints do not exist, and the reverse
	// gate refuses a permission no route uses. The constants exist because the
	// SHAPE is decided (docs/AUTH.md); registration lands with the routes.
	//
	// ⚠️ When they do land, the damage line for roles.edit must say it is
	// owner-equivalent, or somebody grants it thinking it is one admin
	// permission among many. That assertion moves here on that day.
	if PermissionExists(PermRolesEdit) != PermissionExists(PermUsersAssign) {
		t.Error("the two group permissions landed separately; they are one batch — " +
			"shipping assign without edit leaves nothing to assign, and edit without assign " +
			"is owner-equivalence with no commercial operation behind it")
	}
}

// The database browser is split, because the author's decision that user
// content is *assignable* only means something if it can be assigned on its
// own.
func TestUserContentIsItsOwnPermission(t *testing.T) {
	if PermDBOperational == PermDBUserContent {
		t.Fatal("browsing operational tables and browsing user content are one permission")
	}
	// db.user_content is defined but deliberately not registered yet: its
	// handler-level split does not exist, and the reverse gate above refuses a
	// permission no route uses — for the good reason that a console switch which
	// grants nothing reads like protection.
	if PermissionExists(PermDBUserContent) {
		t.Error("db.user_content is registered but its table-level split is not implemented; " +
			"a switch that grants nothing is worse than no switch")
	}
}
