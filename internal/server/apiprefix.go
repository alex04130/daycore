package server

import "daycore/internal/apipath"

// The versioned surface. The RULE lives in internal/apipath because the openapi
// bundler needs the same answer — see that package for why the version is in
// the path at all, and which three routes stay outside it.

// APIPrefix is where the versioned surface is mounted, e.g. "/api/v2".
var APIPrefix = apipath.Prefix

var (
	versionPath    = apipath.Path
	versionPattern = apipath.Pattern
	isUnversioned  = apipath.IsUnversioned
)

// versionedMux applies the prefix on the way to the real mux.
//
// ⚠️ Handler() uses it; RouteTable records both views itself (see Route). They
// must not be able to disagree about what this server serves — RouteTable is
// what docs/API_SURFACE.md and the openapi cross-check are built from, so a
// table describing paths the mux does not serve would make every one of those
// gates green about the wrong thing.
type versionedMux struct{ mux Mux }

func (v versionedMux) HandleFunc(pattern string, h handlerFunc) {
	v.mux.HandleFunc(versionPattern(pattern), h)
}
