package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Every admin route actually consults the permission it declares.
//
// The gate this replaced was `if !s.adminAuthorized(r)`, and the failure that
// made this repository build gates like this one was: deleting that line from a
// handler left the whole suite green while the endpoint answered 200 with no
// credential. The route-level check makes that particular deletion impossible —
// so the question this asks is the next one along: does the permission SEPARATE
// anything, or does any admin credential open every door?
//
// It asks it of every route at once, because that is the only way to catch the
// route that was added without the question being asked at all.
func TestEveryAdminRouteChecksItsOwnPermission(t *testing.T) {
	s := adminServer(t)
	// Somebody with a valid console session and one permission that is not the
	// one any of these routes want. Not zero permissions: a person with none
	// cannot log in at all, so testing with none would exercise a state the
	// product does not produce.
	makeAdminUser(t, s, "narrow", []string{PermModelsTest})
	as := withAdminCookie(t, s, "narrow")

	checked := 0
	for _, rt := range RouteTable(s) {
		method, path, ok := strings.Cut(rt.Pattern, " ")
		if !ok || !isAdminPattern(rt.Pattern) {
			continue
		}
		perm, declared := PermissionFor(rt.Pattern)
		if !declared {
			continue // the forward gate in permissions_test.go owns this case
		}
		switch perm {
		case permOpen, permAnyCredential, PermModelsTest:
			continue
		}
		// Wildcards filled with something syntactically valid; the request must
		// be refused before anything looks at them.
		path = strings.NewReplacer("{name}", "x", "{key}", "x", "{id}", "x").Replace(path)
		checked++
		rec := adminReq(t, s, method, path, "{}", as)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s answered %d to a session holding only %q, want 403 — %s",
				rt.Pattern, rec.Code, PermModelsTest, rt.File)
		}
	}
	// Without this the loop passes on an empty route table and asserts nothing —
	// the failure mode of every table-walking check.
	if checked < 15 {
		t.Fatalf("only exercised %d admin routes; this gate is not looking at what it thinks it is", checked)
	}
}

// A route the gate does not know about is refused, not passed through.
//
// The author's decision was "忘标的话就是默认拒绝". This is the behavioural half
// of it — permissions_test.go catches the omission at build time, and this
// catches what happens if it ever gets past that.
func TestAnUndeclaredAdminRouteIsRefused(t *testing.T) {
	s := adminServer(t)
	mux := http.NewServeMux()
	reached := false
	adminGate{s: s, mux: mux}.HandleFunc("GET /api/admin/invented-yesterday",
		func(w http.ResponseWriter, r *http.Request) { reached = true })

	req := httptest.NewRequest(http.MethodGet, "/api/admin/invented-yesterday", nil)
	req.Header.Set("X-Admin-Token", s.cfg.AdminToken) // even the root credential
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if reached {
		t.Error("an undeclared admin route ran its handler — forgetting to mark one is supposed to close it, not open it")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("an undeclared admin route answered %d, want 403", rec.Code)
	}
}

// Non-admin routes are handed through untouched.
//
// Worth asserting because the prefix match is the only thing separating the two
// worlds: a bug that widened it would put a permission check on /api/plan and
// lock every user out of the product.
func TestTheGateOnlyTouchesTheAdminSurface(t *testing.T) {
	s := adminServer(t)
	mux := http.NewServeMux()
	reached := false
	adminGate{s: s, mux: mux}.HandleFunc("GET /api/plan/today",
		func(w http.ResponseWriter, r *http.Request) { reached = true })

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/plan/today", nil))
	if !reached {
		t.Error("the admin gate intercepted an ordinary route")
	}

	for _, p := range []string{"GET /api/admins", "GET /api/administration/x", "POST /admin/thing"} {
		if isAdminPattern(p) {
			t.Errorf("%q was treated as part of the admin surface", p)
		}
	}
	for _, p := range []string{"GET /api/admin/config", "DELETE /api/admin/db/table/{name}/{id}"} {
		if !isAdminPattern(p) {
			t.Errorf("%q was NOT treated as part of the admin surface", p)
		}
	}
}

// Handler() registers routes THROUGH the gate.
//
// A behavioural test cannot see this: if somebody replaces the wrapper with the
// bare mux, every admin route becomes reachable by anyone, and the tests that
// would notice are the ones that would have been deleted along with it. So this
// reads the source.
//
// The same shape as the other structural gates in this repository (the i18n
// catalog, the runtime snapshot, the //go:embed placement): the thing being
// protected is a line that must exist, and a line that must exist cannot be
// guarded by exercising code.
func TestHandlerRegistersThroughTheAdminGate(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var handler *ast.FuncDecl
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if ok && fn.Name.Name == "Handler" && fn.Recv != nil {
			handler = fn
		}
		return true
	})
	if handler == nil {
		t.Fatal("server.go has no Handler method; this gate is not looking at what it thinks it is")
	}

	// Find every `g.register(...)` call and check what it is handed.
	found := 0
	ast.Inspect(handler, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "register" || len(call.Args) != 2 {
			return true
		}
		found++
		lit, ok := call.Args[1].(*ast.CompositeLit)
		if !ok {
			t.Errorf("Handler registers routes with %T rather than an adminGate — "+
				"every /api/admin/ route would be published with no permission check at all", call.Args[1])
			return true
		}
		if id, ok := lit.Type.(*ast.Ident); !ok || id.Name != "adminGate" {
			t.Errorf("Handler registers routes into %v, not adminGate", lit.Type)
		}
		return true
	})
	if found == 0 {
		t.Fatal("no route registration found in Handler; this gate is not looking at what it thinks it is")
	}
}
