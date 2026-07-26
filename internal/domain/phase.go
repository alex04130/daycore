package domain

import (
	"time"

	"daycore/internal/timeutil"
)

// Phase is where a block sits relative to now and to the petrify line. It is
// computed on read and never stored — there is no phase column, no nightly
// job, and therefore no way for a block's phase to be stale or to race.
type Phase string

const (
	PhaseFuture Phase = "future" // hasn't started
	PhaseNow    Phase = "now"    // running right now
	PhaseRecon  Phase = "recon"  // over, but still inside the window you can edit
	PhaseStone  Phase = "stone"  // frozen: a record, not a plan
)

// PhaseAt classifies a span. The order of the checks is the semantics: a block
// that ended before the line is stone even though it also ended before now.
//
// Boundary conventions are copied from the prototype (daycore-core.js phaseOf)
// and matter at the edges: a block that ends exactly at the line is NOT stone
// (`end < line`), and one that ends exactly now IS recon (`end <= now`).
func PhaseAt(start, end, now, line time.Time) Phase {
	switch {
	case end.Before(line):
		return PhaseStone
	case !end.After(now):
		return PhaseRecon
	case !start.After(now):
		return PhaseNow
	default:
		return PhaseFuture
	}
}

// Span resolves a block to an absolute start and end. ok is false for a block
// with no time — an unscheduled item has no position on the clock, so it never
// freezes and stays editable.
//
// A block anchored to an instant (UTCTime, written for fixed/local modes) is
// resolved from that instant; a floating block is resolved against loc, which
// is what makes "23:00 sleep" mean 23:00 wherever the user currently is.
// planDate covers blocks that inherit their date from the enclosing DayPlan.
func (b TimeBlock) Span(planDate string, loc *time.Location) (start, end time.Time, ok bool) {
	if loc == nil {
		loc = time.UTC
	}
	dur := time.Duration(0)
	if b.DurationMin != nil && *b.DurationMin > 0 {
		dur = time.Duration(*b.DurationMin) * time.Minute
	}

	if b.UTCTime != nil && *b.UTCTime != "" {
		t, err := time.Parse(time.RFC3339, *b.UTCTime)
		if err == nil {
			return t, t.Add(dur), true
		}
		// A malformed anchor falls through to the wall clock rather than
		// making the block un-phaseable.
	}

	if b.Time == nil || *b.Time == "" {
		return time.Time{}, time.Time{}, false
	}
	date := b.Date
	if date == "" {
		date = planDate
	}
	if date == "" {
		return time.Time{}, time.Time{}, false
	}
	t, err := timeutil.ResolveWall(date, *b.Time, loc)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	return t, t.Add(dur), true
}

// PhaseIn is the whole computation in one call: resolve the block, find the
// line, classify. An unscheduled block reports future — it has no past to
// freeze.
func (b TimeBlock) PhaseIn(planDate string, now time.Time, loc *time.Location, horizon time.Duration) Phase {
	start, end, ok := b.Span(planDate, loc)
	if !ok {
		return PhaseFuture
	}
	return PhaseAt(start, end, now, timeutil.PetrifyLine(now, loc, horizon))
}

// Frozen reports whether a block may no longer be rescheduled or re-marked.
// A frozen block still accepts a note and can be "rescheduled" — which leaves
// it in place and creates a new future block, because the past is not rewritten.
func (b TimeBlock) Frozen(planDate string, now time.Time, loc *time.Location, horizon time.Duration) bool {
	return b.PhaseIn(planDate, now, loc, horizon) == PhaseStone
}
