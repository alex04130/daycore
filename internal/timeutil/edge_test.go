package timeutil

import (
	"strings"
	"testing"
	"time"
)

func TestToUTCWithSeconds(t *testing.T) {
	got, err := ToUTC("2026-06-15", "09:00:30", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-06-15T01:00:30Z" {
		t.Fatalf("seconds form: got %s, want 01:00:30Z", got)
	}
}

func TestToUTCRejectsGarbage(t *testing.T) {
	cases := [][2]string{
		{"not-a-date", "09:00"},
		{"2026-06-15", "25:00"},
		{"2026-06-15", "9"},
		{"2026-06-15", ""},
		{"2026-02-30", "09:00"},
	}
	for _, c := range cases {
		if _, err := ToUTC(c[0], c[1], "Asia/Shanghai"); err == nil {
			t.Errorf("ToUTC(%q, %q) must fail", c[0], c[1])
		}
	}
}

func TestFromUTCRejectsGarbage(t *testing.T) {
	if _, _, err := FromUTC("not-rfc3339", "Asia/Shanghai"); err == nil {
		t.Fatal("a non-RFC3339 stamp must fail")
	}
	if _, _, err := FromUTC("2026-06-15T01:00:00Z", "Mars/Phobos"); err == nil {
		t.Fatal("an unknown target zone must fail")
	}
}

// A wall-clock time inside a spring-forward gap never happens. Go resolves it
// BACKWARDS, and the frontends have to match — see ResolveWall. ToUTC uses
// the same ParseInLocation, so the observed answer is the contract.
func TestToUTCInDSTGap(t *testing.T) {
	// 2026-03-08 02:30 America/New_York does not exist (02:00→03:00).
	got, err := ToUTC("2026-03-08", "02:30", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-03-08T06:30:00Z" {
		t.Errorf("the gap must fold backwards to 01:30 EST (06:30Z), got %s", got)
	}
}

func TestResolveWallWithSeconds(t *testing.T) {
	loc := time.FixedZone("X", 0)
	got, err := ResolveWall("2026-06-15", "09:00:30", loc)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hour() != 9 || got.Minute() != 0 || got.Second() != 30 {
		t.Errorf("seconds form: got %v", got)
	}
	if _, err := ResolveWall("bad", "09:00", loc); err == nil {
		t.Error("a bad date must fail")
	}
	if _, err := ResolveWall("2026-06-15", "bad", loc); err == nil {
		t.Error("a bad time must fail")
	}
	if _, err := ResolveWall("2026-06-15", "09:00", nil); err != nil {
		t.Errorf("a nil location falls back to UTC, got %v", err)
	}
}

// The next-day anchor is built from calendar arithmetic, not AddDate —
// 2024 is a leap year and Feb 29 must land on Mar 1, not vanish or wrap.
func TestStartOfNextDayLeapYear(t *testing.T) {
	loc := time.UTC
	for _, in := range []string{
		"2024-02-28T10:00:00Z", // → Feb 29
		"2024-02-29T10:00:00Z", // → Mar 1
		"2024-12-31T10:00:00Z", // → 2025-01-01
	} {
		now, err := time.Parse(time.RFC3339, in)
		if err != nil {
			t.Fatal(err)
		}
		next := StartOfNextDay(now, loc)
		if next.Sub(now) <= 0 || next.Sub(now) > 48*time.Hour {
			t.Errorf("StartOfNextDay(%s) = %s, must be within the next day", in, next)
		}
		if next.Hour() != 0 || next.Minute() != 0 {
			t.Errorf("StartOfNextDay(%s) = %s, must be a day boundary", in, next)
		}
	}
}

func TestPetrifyLineDegenerateHorizonUsesDefault(t *testing.T) {
	// At noon the midnight half always wins (that is the design), so the
	// substitution is only observable in the small hours: at 02:00 the
	// rolling window with the DEFAULT horizon is 21:00 yesterday, but a
	// horizon of 0 or negative would leave "now − 0" past midnight and let
	// midnight win at 00:00 — 2h behind instead of 5h.
	now := time.Date(2026, 6, 15, 2, 0, 0, 0, time.UTC)
	for _, h := range []time.Duration{0, -time.Hour} {
		line := PetrifyLine(now, time.UTC, h)
		if got := now.Sub(line); got != PetrifyHorizonDefault {
			t.Errorf("horizon %v: line is %v behind now, want the default %v", h, got, PetrifyHorizonDefault)
		}
	}
	// A nil location falls back to UTC rather than panicking. At 02:00 with a
	// 1h horizon the rolling window (01:00) is still past midnight, so the
	// midnight half wins — the expected answer is today's midnight.
	if got := PetrifyLine(now, nil, time.Hour); !got.Equal(StartOfDay(now, time.UTC)) {
		t.Errorf("nil location: got %v", got)
	}
}

// StartOfDay with a nil location must not panic — several callers pass a
// session zone that may be empty.
func TestStartOfDayNilLocation(t *testing.T) {
	got := StartOfDay(time.Now(), nil)
	if got.Location() != time.UTC {
		t.Errorf("nil location must fall back to UTC, got %v", got.Location())
	}
}

func TestToUTCErrorsNameTheInput(t *testing.T) {
	_, err := ToUTC("bad", "09:00", "Asia/Shanghai")
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Errorf("the error must name the input that failed, got %v", err)
	}
}
