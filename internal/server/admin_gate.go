package server

import (
	"net/http"
	"strings"

	"daycore/internal/i18n"
)

// The permission check is done by the router, not by the handlers.
//
// # What this makes impossible
//
// Every admin handler used to open with `if !s.adminAuthorized(r) { 401 }`.
// Twenty-two copies of one line, and permissions would have turned each of them
// into a copy that also names a permission — twenty-two chances to name the
// wrong one, and naming the wrong one is silent: the handler still refuses
// somebody, still returns 403, still looks correct in review. The only way to
// notice would be a person who holds config.read discovering they can read
// prompts.
//
// Deriving the permission from the ROUTE PATTERN removes the question. There is
// exactly one place the mapping lives (routePermissions), the gate reads it, and
// a handler physically cannot check something else — it never sees a permission
// id at all.
//
// # And what it does not
//
// Two things still need a handler's own judgement, and both TIGHTEN rather than
// replace:
//
//   - The database browser: whether a table is operational or user content
//     cannot be read off `GET /api/admin/db/table/{name}`, so the route carries
//     the weaker permission and the handler raises it per table.
//   - Role assignment: joining a group that carries permissions needs
//     roles.edit on top of users.assign, and which group is in the body.
//
// Both call s.authorize directly. Neither can weaken the route-level check,
// because the route-level check already ran.
//
// # Boundary: this wraps registration, not dispatch
//
// adminGate is a Mux, so it wraps handlers as they are registered rather than
// sitting in the middleware chain. That is deliberate: a middleware would have
// to re-derive the matched pattern from the URL, and re-deriving it means
// implementing ServeMux's precedence rules a second time, slightly differently.
// Here the pattern is the literal string the route was registered with.
//
// ⚠️ Which means every path into the mux must go through this wrapper. Handler()
// is the only one, and gate_test.go asserts that a bare mux would be caught.
type adminGate struct {
	s   *Server
	mux Mux
}

var (
	keyAdminNeedCredential = i18n.Reg("admin.gate.unauthorized", i18n.Text{
		"zh-CN": "需要管理凭据",
		"en-US": "This needs an admin credential",
	})
	keyAdminNeedPermission = i18n.Reg("admin.gate.forbidden", i18n.Text{
		"zh-CN": "你的账号没有这一项的权限",
		"en-US": "Your account does not have permission for this",
	})
	keyAdminNeedRoot = i18n.Reg("admin.gate.root_only", i18n.Text{
		"zh-CN": "这一项只有 ADMIN_TOKEN 能做",
		"en-US": "Only ADMIN_TOKEN can do this",
	})
	keyAdminUndeclared = i18n.Reg("admin.gate.undeclared", i18n.Text{
		"zh-CN": "这个端点还没有声明它需要什么权限，因此被拒绝",
		"en-US": "This endpoint has not declared what permission it needs, so it is refused",
	})
)

func (g adminGate) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	if !isAdminPattern(pattern) {
		g.mux.HandleFunc(pattern, handler)
		return
	}
	perm, declared := PermissionFor(pattern)
	switch {
	case !declared:
		// Refused, not passed through. The alternative — treat "not in the map"
		// as "root only" — is the same behaviour with none of the noise, and
		// noise is the point: this has to be discovered on the first request,
		// not six months later.
		g.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			g.s.writeErr(w, http.StatusForbidden, "route_undeclared",
				i18n.T(keyAdminUndeclared, g.s.requestLocale(r)))
		})
		return
	case perm == permOpen:
		// The credential exchange. Gating it would be a lock whose key is
		// inside; these handlers authenticate on their own terms.
		g.mux.HandleFunc(pattern, handler)
		return
	}
	g.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		locale := g.s.requestLocale(r)
		p, ok := g.s.principalFrom(r)
		if !ok {
			// 401, not 403: there is no credential here at all, and the
			// difference tells a caller whether to log in or to ask for access.
			g.s.writeErr(w, http.StatusUnauthorized, "unauthorized", i18n.T(keyAdminNeedCredential, locale))
			return
		}
		if !g.s.authorize(r, perm) {
			key := keyAdminNeedPermission
			if perm == permRoot && !p.root {
				key = keyAdminNeedRoot
			}
			g.s.writeErr(w, http.StatusForbidden, "forbidden", i18n.T(key, locale))
			return
		}
		handler(w, r)
	})
}

// isAdminPattern reports whether a route pattern is part of the admin surface.
//
// Matching on the path rather than on a registration-time opt-in, because an
// opt-in is a thing somebody can forget — and the whole point of this file is
// that forgetting should not be able to open anything. /api/admin/ is the
// prefix docs/API_SURFACE.md, the openapi tags and routePermissions all already
// agree on.
func isAdminPattern(pattern string) bool {
	// Patterns are "METHOD /path"; take everything from the first slash so a
	// method name can never contribute a match.
	if i := strings.IndexByte(pattern, '/'); i >= 0 {
		pattern = pattern[i:]
	}
	return strings.HasPrefix(pattern, "/api/admin/") || pattern == "/api/admin"
}
