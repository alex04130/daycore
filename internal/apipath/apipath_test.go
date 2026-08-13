package apipath

import (
	"strconv"
	"strings"
	"testing"

	"daycore/internal/version"
)

func TestPrefixDerivesFromVersion(t *testing.T) {
	if Prefix != "/api/v"+strconv.Itoa(version.APIVersion) {
		t.Fatalf("Prefix %q must derive from version.APIVersion %d", Prefix, version.APIVersion)
	}
	if !strings.HasPrefix(Prefix, "/api/v") {
		t.Fatalf("Prefix %q must live under /api/v", Prefix)
	}
}

func TestPathIdempotentAndVersioned(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/api/plan", Prefix + "/plan"},
		{"/api/plan?date=2026-01-01", Prefix + "/plan?date=2026-01-01"},
		{Prefix + "/plan", Prefix + "/plan"}, // already versioned: idempotent
		{Prefix, Prefix},
		// Exactly "/api" is not a route — nothing is mounted there, and the
		// rewrite only fires on paths WITH a trailing segment.
		{"/api", "/api"},
		{"/not-api/x", "/not-api/x"},
	}
	for _, tc := range cases {
		if got := Path(tc.in); got != tc.want {
			t.Errorf("Path(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUnversionedPathsStayPut(t *testing.T) {
	// Discovery and liveness must not move on a major bump, and the OAuth
	// callback is a registered redirect target, not a client call.
	for _, p := range []string{"/api/version", "/api/healthz"} {
		if Path(p) != p {
			t.Errorf("Path(%q) must stay unversioned, got %q", p, Path(p))
		}
	}
	// The callback is matched by shape: prefix /api/auth/oauth/ and suffix /callback.
	if got := Path("/api/auth/oauth/google/callback"); got != "/api/auth/oauth/google/callback" {
		t.Errorf("oauth callback must stay unversioned, got %q", got)
	}
	// …but the initiating redirect IS versioned — a client chooses when to
	// send somebody there.
	if got := Path("/api/auth/oauth/google"); got != Prefix+"/auth/oauth/google" {
		t.Errorf("the oauth initiating redirect must be versioned, got %q", got)
	}
}

func TestPatternCarriesMethod(t *testing.T) {
	if got := Pattern("GET /api/plan"); got != "GET "+Prefix+"/plan" {
		t.Errorf("Pattern with method = %q", got)
	}
	if got := Pattern("/api/plan"); got != Prefix+"/plan" {
		t.Errorf("Pattern without method = %q", got)
	}
	// Idempotent on patterns too: a doubled rewrite must not produce /v2/v2/…
	once := Pattern("POST " + Prefix + "/x")
	if got := Pattern(once); got != once {
		t.Errorf("Pattern must be idempotent: %q → %q", once, got)
	}
}

func TestIsUnversioned(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"/api/version", true},
		{"/api/healthz", true},
		{"/api/auth/oauth/google/callback", true},
		{"/api/auth/oauth/google", false},
		{"/api/auth/oauth/callback", true},
		{"/api/plan", false},
		{"/api/auth/oauth/google/callback/extra", false},
	}
	for _, tc := range cases {
		if got := IsUnversioned(tc.in); got != tc.want {
			t.Errorf("IsUnversioned(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
