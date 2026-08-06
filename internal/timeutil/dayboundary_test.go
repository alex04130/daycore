package timeutil

import (
	"testing"
	"time"
)

// The zones that matter here are the ones whose transition is AT midnight, so
// the local day does not start at 00:00. America/New_York and Asia/Shanghai are
// in the table as controls: if the fix changed anything about a zone whose
// midnight exists, these fail.
//
// Every expectation below was measured against Go's own tzdata rather than
// derived from the rules, for the same reason ResolveWall's DST notes were: the
// standard library documents the gap case as "not guaranteed", so what it
// actually does is the contract the frontends have to match.
func TestStartOfDayWhenMidnightDoesNotExist(t *testing.T) {
	cases := []struct {
		zone string
		now  string // wall clock, parsed in zone
		want string // expected StartOfDay, as "2006-01-02 15:04 MST"
	}{
		// Havana springs forward 00:00 → 01:00 on 2026-03-08.
		{"America/Havana", "2026-03-08T23:30", "2026-03-08 01:00 CDT"},
		{"America/Havana", "2026-03-08T12:00", "2026-03-08 01:00 CDT"},
		// The day before and after are ordinary.
		{"America/Havana", "2026-03-07T12:00", "2026-03-07 00:00 CST"},
		{"America/Havana", "2026-03-09T12:00", "2026-03-09 00:00 CDT"},
		// Santiago does the same in September.
		{"America/Santiago", "2026-09-06T23:30", "2026-09-06 01:00 -03"},
		// Controls: midnight exists, so nothing moved.
		{"America/New_York", "2026-03-08T23:30", "2026-03-08 00:00 EST"},
		{"Asia/Shanghai", "2026-03-08T23:30", "2026-03-08 00:00 CST"},
	}
	for _, c := range cases {
		loc, err := time.LoadLocation(c.zone)
		if err != nil {
			t.Skipf("no tzdata for %s: %v", c.zone, err)
		}
		now, err := time.ParseInLocation("2006-01-02T15:04", c.now, loc)
		if err != nil {
			t.Fatal(err)
		}
		got := StartOfDay(now, loc)
		if g := got.Format("2006-01-02 15:04 MST"); g != c.want {
			t.Errorf("StartOfDay(%s %s) = %s, want %s", c.zone, c.now, g, c.want)
		}
		// The property that the naive version broke, stated directly: the start
		// of a day is inside that day, and is not after the instant asked about.
		if gy, gm, gd := got.Date(); gy != now.Year() || gm != now.Month() || gd != now.Day() {
			t.Errorf("StartOfDay(%s %s) landed on %v — a different day", c.zone, c.now, got)
		}
		if got.After(now) {
			t.Errorf("StartOfDay(%s %s) = %v is after now", c.zone, c.now, got)
		}
	}
}

func TestStartOfNextDayCrossesTheGap(t *testing.T) {
	cases := []struct {
		zone string
		now  string
		want string
	}{
		// The case AddDate gets wrong: adding a day to 2026-03-07 00:30 in
		// Havana lands on a 03-08 00:30 that never happened, which Go folds
		// back to 03-07 23:30 — the day it started from.
		{"America/Havana", "2026-03-07T00:30", "2026-03-08 01:00 CDT"},
		{"America/Havana", "2026-03-07T23:30", "2026-03-08 01:00 CDT"},
		{"America/Santiago", "2026-09-05T00:30", "2026-09-06 01:00 -03"},
		// Month and year rollover still work (the date arithmetic is time.Date's).
		{"Asia/Shanghai", "2026-12-31T22:00", "2027-01-01 00:00 CST"},
		{"America/New_York", "2026-03-08T23:30", "2026-03-09 00:00 EDT"},
	}
	for _, c := range cases {
		loc, err := time.LoadLocation(c.zone)
		if err != nil {
			t.Skipf("no tzdata for %s: %v", c.zone, err)
		}
		now, _ := time.ParseInLocation("2006-01-02T15:04", c.now, loc)
		got := StartOfNextDay(now, loc)
		if g := got.Format("2006-01-02 15:04 MST"); g != c.want {
			t.Errorf("StartOfNextDay(%s %s) = %s, want %s", c.zone, c.now, g, c.want)
		}
		if !got.After(now) {
			t.Errorf("StartOfNextDay(%s %s) = %v is not after now", c.zone, c.now, got)
		}
	}
}

// A day boundary that is not in its own day breaks every consumer at once, so
// the invariant is asserted across a whole year of dates in the awkward zones
// rather than only on the transition day someone remembered to list.
func TestDayBoundaryInvariantsHoldAllYear(t *testing.T) {
	for _, zone := range []string{
		"America/Havana", "America/Santiago", "Asia/Beirut", "Asia/Tehran",
		"America/New_York", "Europe/London", "Australia/Lord_Howe", "Asia/Shanghai",
	} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Skipf("no tzdata for %s: %v", zone, err)
		}
		day := time.Date(2026, 1, 1, 12, 0, 0, 0, loc)
		for i := 0; i < 366; i++ {
			now := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, loc)
			start := StartOfDay(now, loc)
			next := StartOfNextDay(now, loc)
			if sy, sm, sd := start.Date(); sy != now.Year() || sm != now.Month() || sd != now.Day() {
				t.Fatalf("%s %v: StartOfDay landed on %v", zone, now.Format("2006-01-02"), start)
			}
			if !start.Before(next) {
				t.Fatalf("%s %v: start %v not before next %v", zone, now.Format("2006-01-02"), start, next)
			}
			if !now.Before(next) || now.Before(start) {
				t.Fatalf("%s %v: now not inside [start, next)", zone, now.Format("2006-01-02"))
			}
			// The span of one day is 24h give or take one shift, never zero and
			// never two days — a fold would show up here as 0h or 47h.
			if d := next.Sub(start); d < 22*time.Hour || d > 26*time.Hour {
				t.Fatalf("%s %v: day is %v long", zone, now.Format("2006-01-02"), d)
			}
			day = day.AddDate(0, 0, 1)
		}
	}
}
