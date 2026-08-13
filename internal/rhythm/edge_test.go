package rhythm

import (
	"testing"
	"time"
)

func TestMedianIntEvenTakesLower(t *testing.T) {
	// The design rule: with two middle values the LOWER is taken, because the
	// mean of 23:00 and 02:00 is nobody's bedtime — an observed value always is.
	cases := []struct {
		in   []int
		want int
	}{
		{nil, 0},
		{[]int{}, 0},
		{[]int{5}, 5},
		{[]int{3, 1, 2}, 2},
		{[]int{10, 20}, 10},
		{[]int{20, 10}, 10},
		{[]int{1, 2, 3, 4}, 2},
	}
	for _, tc := range cases {
		if got := medianInt(tc.in); got != tc.want {
			t.Errorf("medianInt(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseHMTrailingGarbage(t *testing.T) {
	// Sscanf tolerates a single-digit minute and trailing junk — locked as the
	// current behaviour: the guard rejects out-of-range values but not shape.
	if d, err := parseHM("7:5"); err != nil || d != 7*time.Hour+5*time.Minute {
		t.Errorf("parseHM(\"7:5\") = (%v, %v)", d, err)
	}
	if _, err := parseHM("11:00x"); err != nil {
		t.Errorf("parseHM(\"11:00x\") = %v (trailing junk is tolerated today)", err)
	}
	for _, bad := range []string{"24:00", "00:60", "-1:00", "9", ""} {
		if _, err := parseHM(bad); err == nil {
			t.Errorf("parseHM(%q) must fail", bad)
		}
	}
}

func TestScheduleEqualWakeSleep(t *testing.T) {
	// wake == sleep: the night is the full 24 hours, and PlanBefore (3h30m)
	// no longer overflows into the previous evening.
	cfg := DefaultConfig()
	j := Schedule(Profile{Wake: "07:30", Sleep: "07:30"}, cfg)
	if j.BriefAt != "07:30" {
		t.Errorf("BriefAt = %q", j.BriefAt)
	}
	if j.PlanAt != "04:00" {
		t.Errorf("PlanAt = %q, want 04:00", j.PlanAt)
	}
	// planOffset == night: collapses to the midpoint of the sleep window.
	cfg.PlanBefore = 24 * time.Hour
	j = Schedule(Profile{Wake: "07:30", Sleep: "07:30"}, cfg)
	if j.PlanAt != "19:30" {
		t.Errorf("PlanAt = %q, want the midpoint 19:30", j.PlanAt)
	}
}

func TestScheduleGarbageFallsBackToCold(t *testing.T) {
	j := Schedule(Profile{Wake: "nope", Sleep: "nope"}, DefaultConfig())
	if j != Schedule(Cold(), DefaultConfig()) {
		t.Errorf("garbage body times must fall back to the cold schedule, got %+v", j)
	}
}

func TestDayKeyCutBoundary(t *testing.T) {
	loc := time.UTC
	// Before the cut (04:00) an instant belongs to the PREVIOUS day — the
	// 2 a.m. student is living a long Tuesday, not an early Wednesday.
	before := time.Date(2026, 6, 16, 3, 59, 0, 0, loc)
	if got := DayKey(before, 4); got != "2026-06-15" {
		t.Errorf("DayKey(03:59) = %q, want the previous day", got)
	}
	if got := DayKey(time.Date(2026, 6, 16, 4, 0, 0, 0, loc), 4); got != "2026-06-16" {
		t.Errorf("DayKey(04:00) = %q, want the same day", got)
	}
	if got := DayKey(time.Date(2026, 6, 16, 23, 59, 0, 0, loc), 4); got != "2026-06-16" {
		t.Errorf("DayKey(23:59) = %q, want the same day", got)
	}
}

func TestObserveEqualInstantIsIgnored(t *testing.T) {
	cfg := DefaultConfig()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	l := Live{RunSince: now, LastSignalAt: now}
	next := l.Observe(now, cfg)
	if next.LastSignalAt.Equal(now) && next != l {
		t.Error("an equal instant is a retry, not a new signal")
	}
	if !next.LastSignalAt.Equal(now) {
		t.Error("Observe must not rewind the mark")
	}
	// A genuinely earlier signal is ignored (clock stepped backwards).
	next = l.Observe(now.Add(-time.Minute), cfg)
	if !next.LastSignalAt.Equal(now) {
		t.Error("an out-of-order signal must not move the mark backwards")
	}
}
