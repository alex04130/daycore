// Package version is the single source of truth for the app version, shown in
// the settings screen and returned by /api/healthz and /api/version.
//
// # One number, not three (2026-08-09, author's decision)
//
// There used to be an API contract version moving on its own schedule, on the
// theory that four independent frontends would negotiate against it. That
// separation was paid for on every batch — a second thing to remember to bump,
// a lock file to keep in step, three paragraphs of documentation explaining
// which was which — and it bought nothing, because **there are zero frontends
// negotiating anything today.**
//
// So APIVersion and APIMinor are now DERIVED from the build version. There is
// one number to move, and `GET /api/version` still reports the same fields, so
// nothing on the wire changed shape.
//
// # What it costs, said plainly
//
// A frontend can no longer say "I need a contract at least this new" separately
// from "I need a build at least this new". Those become the same statement, so
// a purely internal change that bumps the build also looks, to a client, like a
// contract change. That is a real loss of precision and it is accepted on
// purpose: it costs nothing while nobody is negotiating, and the moment it
// starts costing something — a frontend in its own repository, released on its
// own schedule — splitting the number back out is a smaller job than keeping
// two of them aligned by hand for months first.
//
// # The version is a BATCH MARKER, not a release number
//
// This is the part that is easy to get wrong, because the string looks like
// semver and is not being used as semver:
//
//	2.2 → 2.3 is not "some features were added". It means A WHOLE PLAN was
//	implemented — the vNext roadmap's batches, end to end.
//
//   - Format: 2.<minor>.<patch>, Channel "beta" → displayed as "v2.3.0-beta".
//   - minor +1 when a planned body of work is COMPLETE; patch +1 for fixes
//     shipped between those.
//   - The -beta suffix stays until public beta. The release plan is
//     v2 (beta) → 小范围内测 → v2 continues → 公测 → v3 正式版, so v3 is a
//     product milestone rather than a breaking-change marker.
package version

import "strconv"

// Version is the semantic version of this build, and the only number to move.
const Version = "2.3.0"

// Channel marks the release channel; "beta" while v2 is in test.
const Channel = "beta"

// MinClient is the oldest client build this server considers compatible.
// Advisory: clients show an update hint when their own build is older.
const MinClient = "2.2.0"

// APIVersion is the contract major, derived from the build version.
//
// Still reported by GET /api/version and still the field a client compares
// against, so the handshake did not change shape — only where the number comes
// from. A client that hard-fails on a mismatch keeps working exactly as before.
var APIVersion = majorOf(Version)

// APIMinor is the contract minor, derived from the build version.
var APIMinor = minorOf(Version)

// Full returns the display string, e.g. "2.3.0-beta".
func Full() string {
	if Channel == "" {
		return Version
	}
	return Version + "-" + Channel
}

// majorOf and minorOf parse the two leading components.
//
// Deliberately total rather than validating: Version is a compile-time constant
// in this file, so a malformed one is a typo caught by the test below and never
// something a running process has to survive. Returning 0 keeps the parse from
// being a second place that can fail at startup.
func majorOf(v string) int { return partOf(v, 0) }
func minorOf(v string) int { return partOf(v, 1) }

func partOf(v string, idx int) int {
	start := 0
	for i := 0; i <= idx; i++ {
		end := start
		for end < len(v) && v[end] != '.' {
			end++
		}
		if i == idx {
			n, _ := strconv.Atoi(v[start:end])
			return n
		}
		if end >= len(v) {
			return 0
		}
		start = end + 1
	}
	return 0
}
