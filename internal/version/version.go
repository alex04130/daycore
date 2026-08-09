// Package version is the single source of truth for the app version, shown in
// the settings screen and returned by /api/healthz.
//
// # The build version is a BATCH MARKER, not a release number
//
// This is the thing that is easy to get wrong, because the string looks like
// semver and is not being used as semver (2026-08-07, stated by the author):
//
//	2.2 → 2.3 is not "some features were added". It means A WHOLE PLAN was
//	implemented — the vNext roadmap's batches, end to end.
//
// So the number does NOT move per feature, per batch, or per merge. Everything
// from ζ through κ lands under 2.2.0-beta, and 2.3.0-beta is what the finished
// roadmap is called. Bumping it early would spend the only marker there is for
// "that plan is done" on a Tuesday.
//
//   - Format: 2.<minor>.<patch>, Channel "beta" → displayed as "v2.2.0-beta".
//   - minor +1 when a planned body of work is COMPLETE, not when it starts
//     paying off; patch +1 for fixes shipped between those.
//   - The -beta suffix stays until public beta. The release plan is
//     v2 (beta) → 小范围内测 → v2 continues → 公测 → v3 正式版, so v3 is a
//     product milestone rather than a breaking-change marker.
//
// # This is NOT the API contract version
//
// APIVersion/APIMinor below move on their own schedule and much more often —
// they are what the four frontends negotiate against, and they must change the
// moment the contract surface does. Do not tie them to the build version; the
// two answer different questions and the whole reason there are three version
// layers in this repo is that somebody once did tie them together.
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
const APIMinor = 14

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
