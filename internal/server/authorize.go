package server

import (
	"context"
	"crypto/subtle"
	"net/http"
	"sort"
)

// Who may call an admin endpoint.
//
// # Three ways in, and they are not three tiers
//
//	1. X-Admin-Token   the root credential. Passes everything, including routes
//	                   nobody remembered to mark. It is the environment
//	                   variable, it cannot be changed from any interface, and it
//	                   is the recovery path when every other way is gone.
//	2. dc_admin (root) a short-lived cookie minted from that token. Same power,
//	                   browser-shaped, so it can be stolen in ways an
//	                   environment variable cannot — hence the thirty-minute TTL.
//	3. dc_admin (user) the same cookie minted from a person's own login. Their
//	                   permissions are read from the database on every request.
//
// # Why a person's login cookie is not accepted directly
//
// The author asked for administrators to reach the console "with their own
// login token", and this is that — but the credential CARRIED to /api/admin/*
// is still the admin cookie, obtained by exchanging their login once. The login
// action is theirs; the credential shape is not.
//
// The difference is not ceremony. dc_auth is long-lived and Path=/, so
// accepting it here would put a permanent, broadly-scoped cookie on every admin
// request — and cookies ride along on cross-site requests, which is why
// dc_admin is SameSite=Strict with Path=/api/admin and an Origin check. Making
// the admin surface reachable with dc_auth would hand that whole arrangement
// back.
//
// # Forgetting to mark a route denies it (author's decision)
//
// An /api/admin/ route with no declared permission is refused for everybody
// except root. Not open, not "admin only" — refused. So the failure mode of
// forgetting is an endpoint nobody can reach, which somebody notices in a day,
// rather than one everybody can reach, which nobody notices at all.
//
// The gate in permissions_test.go turns "forgot" into a red test, so the deny
// is a second line rather than the only one.

// principal is who is making an admin request.
type principal struct {
	// root means the ADMIN_TOKEN, directly or through a cookie minted from it.
	root bool
	// userID is set when a person's own login is behind the cookie.
	userID string
}

// authorize answers whether this request may do this thing.
//
// It replaced adminAuthorized, which answered a bool about the request alone
// and therefore could not express "this person, but only for these things".
//
// ⚠️ Call sites do not name permissions. adminGate derives the permission from
// the route pattern, so "the handler checks a different permission from the one
// the route declares" is not a mistake that can be made — see admin_gate.go.
// The few handlers that call this directly are tightening a route-level
// permission, never standing in for one.
func (s *Server) authorize(r *http.Request, perm string) bool {
	p, ok := s.principalFrom(r)
	if !ok {
		return false
	}
	if p.root {
		return true
	}
	switch perm {
	case permRoot:
		// Root already returned above, so reaching here means somebody who is
		// not root asked for a root-only route.
		return false
	case permOpen:
		// An unmarked route arrives with "" and is refused, which is the
		// decision above. permOpen routes never reach this function: the gate
		// does not wrap them at all, because they are how a credential is
		// obtained in the first place.
		return false
	case permAnyCredential:
		return true
	}
	return s.userHasPermission(r.Context(), p.userID, perm)
}

// principalFrom identifies the caller, or reports that there is none.
func (s *Server) principalFrom(r *http.Request) (principal, bool) {
	if s.cfg == nil || s.cfg.AdminToken == "" {
		// Unreachable via config.Load, which invents a token when none is set.
		// Refusing rather than opening is the right direction for a guard whose
		// precondition another file maintains.
		return principal{}, false
	}
	// Machines: a custom header, so cross-origin JavaScript cannot send it
	// without a preflight it will not get.
	if hdr := r.Header.Get("X-Admin-Token"); hdr != "" {
		if subtle.ConstantTimeCompare([]byte(hdr), []byte(s.cfg.AdminToken)) == 1 {
			return principal{root: true}, true
		}
		// A header that was sent and did not match is a decision: do NOT fall
		// through to the cookie. Otherwise a wrong token silently succeeds
		// whenever the browser happens to hold a console session, and the
		// operator never learns their token is wrong — they learn it the day
		// they run the same script from a machine with no browser.
		return principal{}, false
	}
	// Humans: the console's httpOnly cookie. Cookies ride along on cross-site
	// requests, so this path needs the Origin check that the header path does
	// not — SameSite=Strict is the first guard and this is the second.
	c, err := r.Cookie(adminCookie)
	if err != nil || c.Value == "" {
		return principal{}, false
	}
	if !s.sameOriginRequest(r) {
		return principal{}, false
	}
	if s.tokens == nil {
		return principal{}, false
	}
	userID, root, err := s.tokens.ParseAdmin(c.Value)
	if err != nil {
		return principal{}, false
	}
	if root {
		return principal{root: true}, true
	}
	return principal{userID: userID}, true
}

// userHasPermission resolves a person's effective permissions and asks whether
// one of them is this.
//
// # Read fresh, every time, and it costs nothing
//
// The obvious alternative — permissions in the token — was considered and buys
// nothing here: userMW already reads the user row on every authenticated
// request to check TokenVersion, so a row read is happening either way. Putting
// permissions in the token would save no query and would buy a revocation
// window the length of the TTL. Reading them here means **taking somebody's
// access away takes effect on their next request.**
//
// The owner mark short-circuits, which is the entire reason it is a mark rather
// than a role holding every permission: a permission defined today is held by
// every owner today, with no data change and no migration.
func (s *Server) userHasPermission(ctx context.Context, userID, perm string) bool {
	if s.store == nil || userID == "" || perm == "" {
		// Degraded: there is no database to read roles from. Only the root
		// credential works in that state, which is the arrangement degraded boot
		// was built around — the console's login was deliberately made
		// database-free so that it still functions when nothing else does.
		return false
	}
	u, err := s.store.Users().GetByID(ctx, userID)
	if err != nil || u == nil {
		return false
	}
	if u.IsOwner {
		return true
	}
	for _, p := range s.effectivePermissions(ctx, userID) {
		if p == perm {
			return true
		}
	}
	return false
}

// effectivePermissions is the union of the permissions of every role a user
// belongs to, sorted and de-duplicated.
//
// A union and nothing else: there are no deny entries, no inheritance and no
// per-user grants, so this is the whole computation. That is what makes "who
// can export the database" answerable with one membership query rather than a
// closure over a graph.
func (s *Server) effectivePermissions(ctx context.Context, userID string) []string {
	if s.store == nil || userID == "" {
		return nil
	}
	names, err := s.store.Roles().RolesOf(ctx, userID)
	if err != nil || len(names) == 0 {
		return nil
	}
	roles, err := s.store.Roles().ListRoles(ctx)
	if err != nil {
		return nil
	}
	byName := map[string][]string{}
	for _, role := range roles {
		byName[role.Name] = role.Permissions
	}
	set := map[string]bool{}
	for _, n := range names {
		for _, p := range byName[n] {
			set[p] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// principalView is what the console is told about itself so it can hide
// sections it cannot open.
//
// ⚠️ Hiding is a courtesy, never a gate. Every endpoint checks for itself, and
// this exists so a person is not shown a screen that will 403 — not so the
// server can stop checking.
type principalView struct {
	Root        bool     `json:"root"`
	UserID      string   `json:"userId,omitempty"`
	Owner       bool     `json:"owner"`
	Permissions []string `json:"permissions"`
}

func (s *Server) principalView(r *http.Request) (principalView, bool) {
	p, ok := s.principalFrom(r)
	if !ok {
		return principalView{}, false
	}
	if p.root {
		// Root holds everything by definition, so the console is handed the
		// whole list rather than a special flag it would have to interpret in
		// every screen.
		return principalView{Root: true, Permissions: allPermissionIDs()}, true
	}
	view := principalView{UserID: p.userID, Permissions: []string{}}
	if s.store == nil {
		return view, true
	}
	if u, err := s.store.Users().GetByID(r.Context(), p.userID); err == nil && u != nil && u.IsOwner {
		view.Owner = true
		view.Permissions = allPermissionIDs()
		return view, true
	}
	if perms := s.effectivePermissions(r.Context(), p.userID); len(perms) > 0 {
		view.Permissions = perms
	}
	return view, true
}

func allPermissionIDs() []string {
	all := Permissions()
	out := make([]string, 0, len(all))
	for _, p := range all {
		out = append(out, p.ID)
	}
	return out
}
