package timeutil

import (
	"fmt"
	"time"
)

// PetrifyHorizonDefault is how long a finished block stays editable before it
// freezes. Five hours is a product value, not a tuning knob picked for
// performance: it is what makes 1 a.m. still feel like "tonight" to someone
// filling in the evening they just had.
const PetrifyHorizonDefault = 5 * time.Hour

// StartOfDay returns the first instant of now's calendar day in loc.
//
// Usually that is midnight. It is not always: several zones move their clocks
// AT 00:00 rather than at 02:00 or 03:00, and on those dates the local day
// begins at 01:00 because 00:00 never happens. America/Havana and
// America/Santiago do this every year.
//
// Go normalises a wall-clock reading that never happened by folding it
// BACKWARDS (the same behaviour ResolveWall documents below), so the obvious
// time.Date(y, m, d, 0, 0, 0, 0, loc) returns 23:00 on the PREVIOUS day — a
// "start of day" that is not inside that day at all. Measured, not reasoned:
// StartOfDay(2026-03-08 23:30 America/Havana) used to return 2026-03-07 23:00.
//
// Three things were built on the naive version and all three were wrong on
// those dates:
//
//   - PetrifyLine, whose midnight half decides what is still editable.
//   - Proposal expiry, which caps an ask-first card at the day boundary. With a
//     boundary an hour in the past, a card created after 23:00 was BORN EXPIRED
//     (measured: 30 minutes of negative life).
//   - The mood tool's "did they already check in today", which swallowed the
//     previous evening's check-ins.
//
// api/testdata/petrify-vectors.json only ever exercised America/New_York, whose
// transition is at 02:00 — midnight always exists there, which is exactly why
// none of this showed up. The vectors now carry a midnightGap section.
func StartOfDay(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := now.In(loc).Date()
	t := time.Date(y, m, d, 0, 0, 0, 0, loc)
	if ty, tm, td := t.Date(); ty == y && tm == m && td == d {
		return t
	}
	// Midnight was skipped. Walk forward a minute at a time to the first
	// wall-clock reading that does land on this date. Offsets move by whole
	// hours or by 30/45 minutes, so a real zone resolves within the first two
	// hours; the full-day bound is a backstop that keeps the function total
	// rather than an expected cost.
	for min := 1; min <= 24*60; min++ {
		c := time.Date(y, m, d, 0, min, 0, 0, loc)
		if cy, cm, cd := c.Date(); cy == y && cm == m && cd == d {
			return c
		}
	}
	return t
}

// StartOfNextDay returns the first instant of the day after now's, in loc.
//
// It builds the next calendar date arithmetically rather than adding 24 hours
// or calling AddDate on the instant. Both of those can be folded by the same
// gap this file exists to handle: AddDate(0, 0, 1) on 2026-03-07 00:30 in
// America/Havana lands on a 2026-03-08 00:30 that never happens, which Go folds
// back to 2026-03-07 23:30 — the day before the one asked for. Noon is used as
// the anchor because no zone has ever shifted by twelve hours, so it exists on
// every date.
func StartOfNextDay(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := now.In(loc).Date()
	return StartOfDay(time.Date(y, m, d+1, 12, 0, 0, 0, loc), loc)
}

// PetrifyLine is the instant before which the past is read-only:
// min(now − horizon, midnight today). Which of the two wins is the whole
// design — during the day midnight wins, so nothing from today ever freezes
// while you are living it; after midnight the rolling window wins, so the
// evening you just had is still yours to edit at 1 a.m. By 5 a.m. the rolling
// window has caught up to midnight and yesterday is closed.
//
// This is a pure function of the clock. There is no table, no job, and nothing
// to migrate — get the parameter wrong and the next read is already correct.
func PetrifyLine(now time.Time, loc *time.Location, horizon time.Duration) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	if horizon <= 0 {
		horizon = PetrifyHorizonDefault
	}
	rolling := now.Add(-horizon)
	midnight := StartOfDay(now, loc)
	if rolling.Before(midnight) {
		return rolling
	}
	return midnight
}

// ResolveWall interprets a date ("2006-01-02") and a wall-clock time ("15:04"
// or "15:04:05") as an instant in loc.
//
// Two DST edges are worth knowing about, because they are Go's behaviour
// rather than ours and the frontends have to match. Both were measured, not
// reasoned about — the standard library documents the gap case as "not
// guaranteed", so the observed answer is the contract:
//
//   - Spring forward leaves a gap; a wall-clock time inside it never happens.
//     Go resolves it *backwards*, to the instant one hour before the nominal
//     reading: on 2026-03-08 in America/New_York (02:00→03:00), both 02:00 and
//     02:30 come back as 01:00 and 01:30 EST.
//   - Fall back makes an hour ambiguous. Go picks the first (pre-transition)
//     occurrence — 01:30 on 2026-11-01 is EDT, not EST.
//
// Both are recorded in api/testdata/petrify-vectors.json so a client that
// resolves them differently fails a test rather than quietly disagreeing about
// whether a block has frozen.
func ResolveWall(date, hm string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	layout := "2006-01-02 15:04"
	if len(hm) == 8 {
		layout = "2006-01-02 15:04:05"
	}
	t, err := time.ParseInLocation(layout, date+" "+hm, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("timeutil.ResolveWall: date=%q time=%q: %w", date, hm, err)
	}
	return t, nil
}
