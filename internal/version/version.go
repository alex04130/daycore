// Package version is the single source of truth for the app version, shown in
// the settings screen and returned by /api/healthz.
//
// Versioning scheme (v2 beta line):
//   - Format: 2.<minor>.<patch>, Channel "beta" → displayed as "v2.2.0-beta".
//   - minor +1 for each feature milestone (a new screen, a new subsystem such
//     as memory or themes); patch +1 for fixes and small tweaks.
//   - The -beta channel suffix stays until the production release, which drops
//     it without changing the number.
package version

// Version is the semantic version of this build.
const Version = "2.2.0"

// Channel marks the release channel; "beta" while v2 is in test.
const Channel = "beta"

// APIVersion is the API contract major version, independent of the build
// version above. Bump ONLY on breaking API changes — clients hard-fail on a
// mismatch (served by GET /api/version).
const APIVersion = 1

// APIMinor bumps on additive, backward-compatible contract changes (new
// endpoints or fields). Clients may gate optional features on it.
const APIMinor = 5

// MinClient is the oldest client build this server considers compatible.
// Advisory: clients show an update hint when their own build is older.
const MinClient = "2.2.0"

// Full returns the display string, e.g. "2.1.0-beta".
func Full() string {
	if Channel == "" {
		return Version
	}
	return Version + "-" + Channel
}
