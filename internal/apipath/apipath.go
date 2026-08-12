// Package apipath decides where the HTTP API is mounted.
//
// ⚠️ Its own package because TWO builds need the same answer: the server, which
// mounts the routes, and api/spec/bundle, which writes the paths into
// openapi.yaml. A copy of the rule in the generator would be a second source
// for one fact — and it would be the copy that kept saying v2 after the server
// moved on, producing a contract file that describes endpoints nobody serves.
package apipath

import (
	"strconv"
	"strings"

	"daycore/internal/version"
)

// The API lives under a version in its path: /api/v2/…
//
// # Why the version is in the path
//
// A frontend is deployed separately and released on its own schedule
// (docs/ROADMAP.md 阶段 κ). When the contract's MAJOR changes, an old build has
// to keep working against a new server for as long as its users take to update
// — and the only way an old build can keep working is if the endpoints it was
// written against are still there. /api/v2/… and /api/v3/… can be served side
// by side; /api/… cannot be two things at once.
//
// # ⚠️ Three paths stay OUTSIDE the prefix, and each for a different reason
//
//	/api/version   discovery. A client that had to know the version to ask what
//	               the version is has a chicken-and-egg problem — this is the
//	               one endpoint whose whole job is answering that question, and
//	               it is also where the handshake lives.
//	/api/healthz   liveness. Configured in infrastructure — a load balancer, a
//	               container orchestrator, an uptime monitor — by people who are
//	               not tracking this project's contract version. Moving it on a
//	               major bump would silently mark every deployment unhealthy.
//	/api/auth/oauth/…/callback
//	               a redirect target registered in Google's or GitHub's console,
//	               not an endpoint any client calls. Versioning it would mean
//	               every deployment reconfigures its OAuth app on every major
//	               bump, in exchange for nothing: there is no "old client" here,
//	               only a browser following a redirect the server itself issued.
//
// # ⚠️ It derives from version.APIVersion rather than being written down
//
// There is one version in this repository (see the root CLAUDE.md). A literal
// "v2" here would be a second one, and it would be the one that stayed at 2
// after somebody bumped the real one — serving v2 paths from a v3 build, with
// nothing anywhere reporting the mismatch.

// Prefix is where the versioned surface is mounted, e.g. "/api/v2".
var Prefix = "/api/v" + strconv.Itoa(version.APIVersion)

// unversioned lists the paths that must NOT move when the major changes. See
// the note above — each is a different argument, not one rule applied three
// times.
var unversioned = []string{
	"/api/version",
	"/api/healthz",
}

// isUnversioned reports whether a route path stays at /api/… .
func IsUnversioned(path string) bool {
	for _, p := range unversioned {
		if path == p {
			return true
		}
	}
	// The OAuth callback, matched by shape because the provider name is a path
	// variable. The initiating redirect (/api/auth/oauth/{provider}) IS
	// versioned — a client chooses when to send somebody there, so it is a
	// client-facing call; the callback is not.
	return strings.HasPrefix(path, "/api/auth/oauth/") && strings.HasSuffix(path, "/callback")
}

// versionPath rewrites one route path into the versioned surface.
//
// ⚠️ IDEMPOTENT. Callers legitimately hold either view — the admin gate walks
// RouteTable and has wire paths; a test names a resource and has a logical one —
// and a rewrite that fired twice would produce /api/v2/v2/… , which 404s. A
// path that is already versioned is already right.
func Path(path string) string {
	if !strings.HasPrefix(path, "/api/") || IsUnversioned(path) {
		return path
	}
	if strings.HasPrefix(path, Prefix+"/") || path == Prefix {
		return path
	}
	return Prefix + strings.TrimPrefix(path, "/api")
}

// versionPattern rewrites a ServeMux pattern, which may carry a method.
func Pattern(pattern string) string {
	method, path, found := strings.Cut(pattern, " ")
	if !found {
		return Path(pattern)
	}
	return method + " " + Path(path)
}
