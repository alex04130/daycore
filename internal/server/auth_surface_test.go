package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"daycore/internal/config"
	"daycore/internal/storage"
	_ "daycore/internal/storage/sqlstore"
)

// publicRoutes is the documented list from docs/AUTH.md, plus the two token-
// authenticated rails that carry their own credential instead of a session.
//
// Anything not in here must answer an unauthenticated request with 401. The list
// is written out rather than derived so that making an endpoint public is an
// edit to this file — a reviewable act — instead of a side effect of forgetting
// a line in a handler.
var publicRoutes = map[string]string{
	"GET /api/healthz":       "liveness, deliberately open",
	"GET /api/version":       "contract negotiation happens before a session exists",
	"GET /api/models":        "capability discovery",
	"POST /api/session/init": "this is what mints the session",
	"POST /api/version": "the frontend handshake — a build must be able to introduce itself before it " +
		"has any credential, which is the premise of the two-layer identity. ⚠️ It WRITES (families, " +
		"build sightings), so the exposure is bounded deliberately rather than by a credential: a " +
		"family cap, an operator's pin freezing a token space, and a kind gate on every declaration. " +
		"No value it carries is ever used unapproved — see handlers_handshake.go.",

	"POST /api/auth/register": "no session yet by definition",
	"POST /api/auth/login":    "no session yet by definition",
	"POST /api/auth/logout":   "must work with an expired session",
	"DELETE /api/admin/session": "logging out must work with an expired or absent admin cookie — " +
		"401ing a logout leaves somebody unable to clear a session they cannot use, and " +
		"it clears a cookie rather than reading anything",
	"POST /api/admin/session":                 "this is what mints the admin session; the raw ADMIN_TOKEN in the body IS the credential",
	"GET /api/auth/providers":                 "shown on the signed-out screen",
	"GET /api/me":                             "answers {user:null} when anonymous",
	"GET /api/auth/oauth/{provider}":          "starts the redirect dance",
	"GET /api/auth/oauth/{provider}/callback": "the IdP calls this, not the browser session",

	// Own credential, not a session.
	"POST /api/channels/{channel}/verify": "the binding token in the body IS the credential — " +
		"the channel side (a bot) calls this, and it has no session by construction",
	// Static files, not data. Every byte of the console bundle is in the public
	// repository, so gating the HTML would only produce a login page that cannot
	// render its own login form. The credential check is on /api/admin/*, where
	// the data is.
	//
	// ⚠️ The consequence, and it is a real constraint on the console: nothing in
	// that bundle may carry a secret, a deployment-specific value, or anything
	// the API would refuse to serve unauthenticated.
	"GET /admin":  "static console bundle; the credential check is on /api/admin/*",
	"GET /admin/": "same",

	"POST /api/import/canvas": "X-Import-Token (browser extension push)",
	"POST /api/import/ics":    "X-Import-Token",
}

// ⚠️ 2026-08-09: thirteen /api/admin/* routes used to sit in the list above,
// each annotated "X-Admin-Token".
//
// That annotation was TRUE and the entry was WRONG, and the difference is the
// whole point of this file. "Carries its own credential instead of a session"
// is not the same statement as "answers an unauthenticated request" — and this
// list only means the second one. Every one of those thirteen DOES check
// adminAuthorized; listing them here said "and it is fine if one day it does
// not".
//
// It was not hypothetical. Deleting the four-line check from
// handleAdminDBExport left the ENTIRE suite green — `go test ./internal/server/`
// ok 15.7s — while `GET /api/admin/db/export` answered 200 to a request with no
// credential at all. Fifteen of twenty-three admin routes were in that state:
// a door with a sign reading "locked" and no lock behind it.
//
// The import rails above stay, and the difference is worth naming: they answer
// 401 to an unauthenticated request too, but their credential is a header this
// test cannot mint, so exempting them keeps the test honest rather than
// blinding it. Admin routes have exactly the same shape — which is why they are
// now checked with an empty request instead of exempted.

// Every non-public route must reject a request that carries no credentials.
//
// This exists because three endpoints did not. `POST /api/ai/plan-text`,
// `plan-image` and `extract-schedule-image` checked the rate limiter and nothing
// else — so the two most expensive operations in the product (both vision) were
// reachable by anyone who could reach the host, with an IP bucket as the only
// brake on the operator's model spend. Nothing declared that: openapi's global
// security applies to them and AUTH.md's public list never included them.
//
// A per-handler review would not have caught it, because the missing line is
// invisible; only asking every route the same question does.
func TestUnauthenticatedRequestsAreRejected(t *testing.T) {
	store, err := storage.Open("sqlite", "file:"+t.TempDir()+"/auth.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := New(Deps{
		Config: &config.Config{RateLimitPerMin: 0, AuthRateLimitPerMin: 0},
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(new(strings.Builder), nil)),
	})
	h := s.Handler()

	var leaks []string
	for _, rt := range RouteTable(s) {
		if _, public := publicRoutes[rt.Pattern]; public {
			continue
		}
		method, path, ok := strings.Cut(rt.Pattern, " ")
		if !ok {
			continue
		}
		// Fill wildcards with something syntactically valid; the request must be
		// rejected before anything looks at them.
		concrete := wildcard.ReplaceAllString(path, "x")

		req := httptest.NewRequest(method, concrete, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			leaks = append(leaks, rt.Pattern+"  →  "+http.StatusText(rec.Code)+" ("+itoaSurface(rec.Code)+"), group "+rt.Group)
		}
	}
	sort.Strings(leaks)
	if len(leaks) > 0 {
		t.Errorf("%d routes answered an unauthenticated request with something other than 401.\n"+
			"Either add the missing credential check, or add the route to publicRoutes with a reason:\n  %s",
			len(leaks), strings.Join(leaks, "\n  "))
	}
}

// The reverse direction: an entry in publicRoutes that no longer matches a real
// route is a stale exemption, and a stale exemption is how a route becomes
// accidentally public later under a recycled name.
func TestPublicRouteListHasNoStaleEntries(t *testing.T) {
	served := map[string]bool{}
	for _, rt := range RouteTable(&Server{}) {
		served[rt.Pattern] = true
	}
	var stale []string
	for pattern := range publicRoutes {
		if !served[pattern] {
			stale = append(stale, pattern)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("publicRoutes exempts %d routes that are not served:\n  %s", len(stale), strings.Join(stale, "\n  "))
	}
}

// wildcard matches a ServeMux path segment like {id} or {provider}.
var wildcard = regexp.MustCompile("[{][^}]+[}]")
