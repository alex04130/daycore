package server

import (
	"strings"
	"testing"

	"daycore/internal/version"
)

// The versioned surface, and the three things deliberately outside it.
func TestTheVersionedSurface(t *testing.T) {
	if APIPrefix != "/api/v"+itoaTest(version.APIVersion) {
		t.Fatalf("APIPrefix is %q but the build's API major is %d", APIPrefix, version.APIVersion)
	}

	for _, c := range []struct{ in, want string }{
		{"/api/plan", APIPrefix + "/plan"},
		{"/api/admin/stats", APIPrefix + "/admin/stats"},
		{"/api/proposals/{id}/respond", APIPrefix + "/proposals/{id}/respond"},
		// ⚠️ Discovery cannot be behind the thing it discovers.
		{"/api/version", "/api/version"},
		// ⚠️ Liveness is configured by people who do not track this contract.
		{"/api/healthz", "/api/healthz"},
		// ⚠️ A redirect target registered in somebody else's console. Nothing
		// calls it; a browser is sent to it by this server.
		{"/api/auth/oauth/google/callback", "/api/auth/oauth/google/callback"},
		{"/api/auth/oauth/{provider}/callback", "/api/auth/oauth/{provider}/callback"},
		// …but STARTING the flow is a client-facing call, so it moves.
		{"/api/auth/oauth/{provider}", APIPrefix + "/auth/oauth/{provider}"},
		// Not our surface at all.
		{"/admin/", "/admin/"},
		{"/", "/"},
	} {
		if got := versionPath(c.in); got != c.want {
			t.Errorf("versionPath(%q) = %q, want %q", c.in, got, c.want)
		}
		// ⚠️ Idempotent: callers hold either view, and a double application
		// yields /api/v2/v2/… , which 404s with nothing explaining why.
		if twice := versionPath(versionPath(c.in)); twice != c.want {
			t.Errorf("versionPath is not idempotent for %q: %q", c.in, twice)
		}
	}

	if got := versionPattern("PUT /api/plan"); got != "PUT "+APIPrefix+"/plan" {
		t.Errorf("versionPattern kept the method wrong: %q", got)
	}
}

// Every route the server serves is under the prefix, except the three named
// above — and the table says both things about each.
func TestEveryRouteIsVersionedExceptTheDiscoveryPair(t *testing.T) {
	s := adminServer(t)
	table := RouteTable(s)
	if len(table) < 100 {
		t.Fatalf("only %d routes; this test is not looking at the real surface", len(table))
	}
	exempt := 0
	for _, rt := range table {
		_, path, _ := strings.Cut(rt.Pattern, " ")
		_, logical, _ := strings.Cut(rt.Logical, " ")
		// The console UI and the SPA fallback are not the API surface — they are
		// pages this server happens to serve.
		if !strings.HasPrefix(logical, "/api/") {
			continue
		}
		if isUnversioned(logical) {
			exempt++
			if path != logical {
				t.Errorf("%s should not have moved, but serves at %s", rt.Logical, rt.Pattern)
			}
			continue
		}
		if !strings.HasPrefix(path, APIPrefix+"/") {
			t.Errorf("%s serves at %s, outside the versioned surface", rt.Logical, rt.Pattern)
		}
		// ⚠️ Logical stays version-free. The permission table and the admin gate
		// key on it, and a gate that stopped recognising an admin route would
		// fail OPEN.
		if strings.Contains(logical, "/v"+itoaTest(version.APIVersion)+"/") {
			t.Errorf("%s carries the version in its logical form", rt.Logical)
		}
	}
	// version (GET+POST), healthz, and the oauth callback.
	if exempt < 3 {
		t.Errorf("only %d exempt routes; the discovery pair should be there", exempt)
	}
}
