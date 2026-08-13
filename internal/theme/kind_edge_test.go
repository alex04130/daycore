package theme

import (
	"strings"
	"testing"
)

// MaxValueLen is enforced by BYTE length, while the error says characters —
// locked as the current behaviour so the CJK discrepancy cannot drift
// silently either way.
func TestMaxValueLenBoundary(t *testing.T) {
	if err := CharacterFloor(strings.Repeat("a", 256)); err != nil {
		t.Errorf("exactly 256 bytes must pass, got %v", err)
	}
	if err := CharacterFloor(strings.Repeat("a", 257)); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("257 bytes must be refused, got %v", err)
	}
	// 90 CJK characters are 270 bytes: refused, despite being 90 characters.
	if err := CharacterFloor(strings.Repeat("字", 90)); err == nil {
		t.Error("90 CJK characters (270 bytes) must be refused — the limit is bytes (locked)")
	}
}

func TestCharacterFloorForbiddenAndWhitespace(t *testing.T) {
	for _, bad := range []string{"url(", ";", "}", "{", "<", ">", "\\", "/*", "*/", "\n", "\r"} {
		if err := CharacterFloor("a" + bad + "b"); err == nil {
			t.Errorf("value containing %q must be refused", bad)
		}
	}
	// Whitespace padding is tolerated by the floor; the kind validator trims.
	if err := CharacterFloor("  #fff  "); err != nil {
		t.Errorf("the floor must not reject padding, got %v", err)
	}
	// A tab is NOT in the forbidden list — locked asymmetry with the kind
	// expression charset, which does refuse it.
	if err := CharacterFloor("a\tb"); err != nil {
		t.Errorf("a tab currently passes the floor (locked), got %v", err)
	}
}

func TestPlainKindNameBounds(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"color", true},
		{"tab-bar-h", true},
		{"", false},
		{"Color", false},
		{"1color", false},
		{"-color", false},
		{"one-of[a,b]", false},
		{"café", false},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
	}
	for _, tc := range cases {
		if got := PlainKindName(tc.in); got != tc.want {
			t.Errorf("PlainKindName(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestOriginOfAndBaseOrigin(t *testing.T) {
	r := NewRegistry()
	// The embedded floor reports its origin; an unknown name reports none.
	if got := r.OriginOf("color"); got != OriginEmbedded {
		t.Errorf("OriginOf(color) = %q", got)
	}
	if got := r.OriginOf("no-such-kind"); got != "" {
		t.Errorf("OriginOf(unknown) = %q, want empty", got)
	}
	if got := r.BaseOrigin("color"); got != OriginEmbedded {
		t.Errorf("BaseOrigin(color) = %q", got)
	}
	if got := r.BaseOrigin("no-such-kind"); got != "" {
		t.Errorf("BaseOrigin(unknown) = %q, want empty", got)
	}
	// A DB row that shadows the floor: OriginOf says db, BaseOrigin says the
	// layer it is redefining.
	k := Kind{Name: "color", Pattern: "#....", Origin: OriginDB, Description: "db color"}
	r.SetDBKinds([]Kind{k})
	if got := r.OriginOf("color"); got != OriginDB {
		t.Errorf("OriginOf(color) after shadowing = %q", got)
	}
	if got := r.BaseOrigin("color"); got != OriginEmbedded {
		t.Errorf("BaseOrigin(color) after shadowing = %q", got)
	}
}

func TestCompileCheck(t *testing.T) {
	if _, err := CompileCheck(strings.Repeat("a", 513)); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("an over-long pattern must be refused, got %v", err)
	}
	if _, err := CompileCheck("["); err == nil {
		t.Error("an invalid regex must be refused")
	}
	if _, err := CompileCheck("abc"); err != nil {
		t.Errorf("a plain pattern must compile, got %v", err)
	}
	// The anchor wrapper: a pattern is compiled anchored, so it must match
	// the whole value.
	if _, err := CompileCheck("a"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTrimsAndEmpty(t *testing.T) {
	r := NewRegistry()
	// Padded values are trimmed before the kind check and pass.
	if err := r.Validate("color", "  #fff  "); err != nil {
		t.Errorf("padding must be trimmed, got %v", err)
	}
	// An empty value fails the kind check (the floor lets it through).
	if err := r.Validate("color", ""); err == nil {
		t.Error("an empty value must be refused by the kind check")
	}
	// An unknown kind is refused even for a floor-clean value.
	if err := r.Validate("no-such-kind", "x"); err == nil {
		t.Error("an unknown kind must be refused")
	}
}
