package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"daycore/internal/domain"
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
		switch perm {
		case permOpen, permAnyCredential, permRoot:
			// A route marker rather than a permission. Each is named and
			// documented next to routePermissions, and registerPerm refuses to
			// make any of them grantable.
			continue
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

	// The reverse direction has to look further than the route table.
	//
	// db.user_content is required by a HANDLER, not by a route: the pattern
	// `GET /api/admin/db/table/{name}` cannot say whether {name} is
	// operation_logs or chat_messages, so the route carries the weaker
	// permission and handlers_admin_db.go raises it per table. A gate that only
	// read routePermissions would have called that permission unused and
	// pushed somebody to delete the split.
	//
	// So: consulted = named by a route, OR mentioned anywhere in this package's
	// real source outside permissions.go. Coarse on purpose — the question is
	// "does anything look at this at all", and any mention in shipped code is
	// evidence that something does. It cannot be satisfied by adding a line to a
	// table, which is what makes it worth having.
	consulted := permissionsMentionedInSource(t)
	for _, p := range Permissions() {
		if used[p.ID] || consulted[p.ID] {
			continue
		}
		t.Errorf("permission %q is registered and nothing consults it.\n"+
			"  That is a switch in the console that grants nothing — either wire it to the route or the\n"+
			"  handler it was meant for, or delete it. A permission nobody checks is worse than none: it\n"+
			"  reads like protection.", p.ID)
	}
}

// permissionsMentionedInSource returns the permission ids whose Perm* constant
// is referenced somewhere in this package's non-test source, other than in
// permissions.go itself (where every one of them is defined and registered).
func permissionsMentionedInSource(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && fi.Name() != "permissions.go"
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Constant name → id, built from the registry so this cannot drift.
	byName := map[string]string{
		"PermOverview": PermOverview, "PermConfigRead": PermConfigRead, "PermConfigWrite": PermConfigWrite,
		"PermProvidersRead": PermProvidersRead, "PermProvidersWrite": PermProvidersWrite,
		"PermModelsRead": PermModelsRead, "PermModelsTest": PermModelsTest, "PermOAuthRead": PermOAuthRead,
		"PermPromptsRead": PermPromptsRead, "PermPromptsWrite": PermPromptsWrite, "PermAILogsRead": PermAILogsRead,
		"PermUsersRead": PermUsersRead, "PermUsersDelete": PermUsersDelete, "PermUsersAssign": PermUsersAssign,
		"PermRolesEdit": PermRolesEdit, "PermDBOperational": PermDBOperational,
		"PermRestart":        PermRestart,
		"PermPairingsRead":   PermPairingsRead,
		"PermPairingsManage": PermPairingsManage,
		"PermDBUserContent":  PermDBUserContent, "PermDBDeleteRow": PermDBDeleteRow,
		"PermDBExport": PermDBExport, "PermDBImport": PermDBImport,
	}
	// A permission added to the registry but not to the map above would be
	// silently exempt from the check, which is the failure this whole gate is
	// about. So the map is checked against the registry too.
	for _, p := range Permissions() {
		found := false
		for _, id := range byName {
			if id == p.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("permission %q is not in permissionsMentionedInSource's name map, so the reverse "+
				"gate silently exempts it — add it there in the same edit that registers it", p.ID)
		}
	}

	out := map[string]bool{}
	files := 0
	for _, pkg := range pkgs {
		for range pkg.Files {
			files++
		}
		ast.Inspect(pkg, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if permID, isPerm := byName[id.Name]; isPerm {
				out[permID] = true
			}
			return true
		})
	}
	if files < 30 {
		t.Fatalf("only parsed %d source files; this gate is not looking at what it thinks it is", files)
	}
	return out
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
	if !PermissionExists(PermRolesEdit) || !PermissionExists(PermUsersAssign) {
		t.Fatal("the two group permissions are one batch — shipping assign without edit leaves " +
			"nothing to assign, and edit without assign is owner-equivalence with no commercial " +
			"operation behind it")
	}
	// roles.edit has to SAY it is owner-equivalent, next to the switch, or
	// somebody grants it thinking it is one admin permission among many. It is
	// the one line in the list where the damage is not what the name suggests.
	var damage string
	for _, p := range Permissions() {
		if p.ID == PermRolesEdit {
			damage = p.Damage
		}
	}
	if !strings.Contains(damage, "超级管理员") && !strings.Contains(damage, "owner") {
		t.Errorf("roles.edit's damage line does not say it is owner-equivalent: %q\n"+
			"  Whoever can edit a role's permissions can put themselves in an all-powerful one.\n"+
			"  Granting it IS making somebody an owner, and the sentence beside the switch is the\n"+
			"  only place that gets said.", damage)
	}
}

// The two routes whose separation carries that boundary have to stay separate
// too. A permission split means nothing if one endpoint does both jobs.
func TestTheEscalationBoundaryIsTwoRoutes(t *testing.T) {
	edit, editOK := PermissionFor("PUT /api/admin/roles/{name}")
	assign, assignOK := PermissionFor("PUT /api/admin/users/{id}/roles")
	if !editOK || !assignOK {
		t.Fatal("the role-edit and user-assign routes are not both declared")
	}
	if edit != PermRolesEdit || assign != PermUsersAssign {
		t.Fatalf("the boundary routes require %q and %q; expected %q and %q",
			edit, assign, PermRolesEdit, PermUsersAssign)
	}
}

// The owner mark is set with the root credential and nothing else.
//
// Not owner-only and not roles.edit: it is the bootstrap (the author ruled out
// "first to register wins") and the recovery path, so it must be reachable from
// the environment alone and unreachable by anything the database can grant.
func TestTheOwnerMarkIsRootOnly(t *testing.T) {
	perm, ok := PermissionFor("PUT /api/admin/users/{id}/owner")
	if !ok {
		t.Fatal("the owner route declares nothing")
	}
	if perm != permRoot {
		t.Fatalf("setting the owner mark requires %q; it must be the root credential only — "+
			"a permission that reaches it is a permission that can mint break-glass holders", perm)
	}
}

// The database browser is split, because the author's decision that user
// content is *assignable* only means something if it can be assigned on its
// own.
func TestUserContentIsItsOwnPermission(t *testing.T) {
	if PermDBOperational == PermDBUserContent {
		t.Fatal("browsing operational tables and browsing user content are one permission")
	}
	if !PermissionExists(PermDBUserContent) {
		t.Fatal("db.user_content is not registered, so the split it names cannot be granted separately")
	}
	// And the split is REAL: the two classes must map to the two permissions,
	// and both classes must actually be populated. A catalogue where every table
	// is operational would satisfy the mapping and grant everybody everything.
	ops, content := 0, 0
	for _, tb := range domain.Tables {
		switch permForTable(tb) {
		case PermDBOperational:
			ops++
		case PermDBUserContent:
			content++
		default:
			t.Errorf("%s maps to %q, which is neither browse permission", tb.Name, permForTable(tb))
		}
	}
	if ops < 5 || content < 5 {
		t.Errorf("the catalogue is %d operational and %d user-content tables; one of the two classes is "+
			"empty or nearly so, which makes the split decorative", ops, content)
	}
	// The three the author named by hand must be on the user-content side. They
	// are the reason the split exists, so they are the ones worth pinning.
	for _, name := range []string{"chat_messages", "mood_checkins", "memory_facts"} {
		tb, ok := domain.TableByName(name)
		if !ok {
			t.Errorf("%s is not in the catalogue at all", name)
			continue
		}
		if permForTable(tb) != PermDBUserContent {
			t.Errorf("%s is classified %q — the author named this table specifically as the kind of "+
				"thing that must be assignable on its own", name, tb.Class)
		}
	}
}
