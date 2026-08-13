package version

import (
	"testing"
)

// majorOf/minorOf are deliberately total: a malformed Version is a compile-
// time typo, never something a running process must survive, and returning 0
// keeps the parse from being a second place that can fail at startup.
func TestMajorMinorOf(t *testing.T) {
	cases := []struct {
		in           string
		major, minor int
	}{
		{"2.3.0", 2, 3},
		{"0.1.0", 0, 1},
		{"10.22.33", 10, 22},
		{"2", 2, 0},
		{"2.3", 2, 3},
		{"", 0, 0},
		{".", 0, 0},
		{"..", 0, 0},
		{"v2.3.0", 0, 3},
		{"2.x.0", 2, 0},
		{"2.-3.0", 2, -3},
	}
	for _, tc := range cases {
		if got := majorOf(tc.in); got != tc.major {
			t.Errorf("majorOf(%q) = %d, want %d", tc.in, got, tc.major)
		}
		if got := minorOf(tc.in); got != tc.minor {
			t.Errorf("minorOf(%q) = %d, want %d", tc.in, got, tc.minor)
		}
	}
}

func TestDerivedAPIVersionMatchesVersion(t *testing.T) {
	// The one-number decision: APIVersion/APIMinor are DERIVED, so they must
	// agree with the build Version by construction.
	if APIVersion != majorOf(Version) {
		t.Errorf("APIVersion %d must equal majorOf(Version) %d", APIVersion, majorOf(Version))
	}
	if APIMinor != minorOf(Version) {
		t.Errorf("APIMinor %d must equal minorOf(Version) %d", APIMinor, minorOf(Version))
	}
}

func TestFull(t *testing.T) {
	// Full joins Version and Channel (both compile-time constants). The empty-
	// channel branch in Full exists for release builds that compile without
	// one; with Channel a constant it is unreachable here, so the join is all
	// this test can pin.
	if got := Full(); got != Version+"-"+Channel {
		t.Errorf("Full() = %q, want %q", got, Version+"-"+Channel)
	}
}
