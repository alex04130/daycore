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

// StartOfDay returns midnight at the start of now's calendar day in loc.
func StartOfDay(now time.Time, loc *time.Location) time.Time {
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
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
