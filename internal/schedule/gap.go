package schedule

import (
	"sort"
	"time"

	"daycore/internal/domain"
)

// Gap is a stretch of a day nothing claims.
type Gap struct {
	Start time.Time
	End   time.Time
}

// Minutes is how long the gap is.
func (g Gap) Minutes() int { return int(g.End.Sub(g.Start) / time.Minute) }

// Gaps finds the empty stretches of a day inside [dayStart, dayEnd).
//
// The mirror image of Overlaps, and deliberately the same kind of thing: a
// QUERY, not a decision. It reports where nothing is; whether anything should go
// there — and what — belongs to the caller.
//
// # ⚠️ What counts as claimed is WIDER here than what counts as a clash
//
// Overlaps ignores blocks with no time because a loose to-do cannot clash with
// anything. This ignores them too, for the opposite reason and with the same
// result: a task with no slot is not occupying one.
//
// But tombstones diverge. Overlaps skips a hidden occurrence because the user
// said it is not happening, so it cannot fight anything. Gaps skips it too — and
// that is the RIGHT answer for a different reason: an occurrence the reader
// dismissed leaves its time genuinely free.
//
// # ⚠️ The bounds are half-open and the caller owns them
//
// dayStart/dayEnd are passed in rather than derived from the date, because "the
// part of the day worth filling" is a product question with a real answer
// elsewhere: the learned rhythm's wake and sleep times. Deriving midnight-to-
// midnight here would offer somebody a 03:00 slot, which is exactly the kind of
// suggestion that teaches people to stop reading suggestions.
//
// # ⚠️ Blocks that run past dayEnd still shorten the gap before them
//
// A block is clipped to the window rather than dropped, so an evening thing that
// spills past bedtime does not leave a phantom gap where it actually is. The
// same for one starting before dayStart.
func Gaps(blocks []domain.TimeBlock, planDate string, loc *time.Location, dayStart, dayEnd time.Time) []Gap {
	if !dayEnd.After(dayStart) {
		return nil
	}
	type span struct{ start, end time.Time }
	spans := make([]span, 0, len(blocks))
	for _, b := range blocks {
		if b.Hidden {
			continue
		}
		start, end, ok := b.Span(planDate, loc)
		if !ok || !end.After(start) {
			continue
		}
		// Clip to the window; drop anything entirely outside it.
		if start.Before(dayStart) {
			start = dayStart
		}
		if end.After(dayEnd) {
			end = dayEnd
		}
		if !end.After(start) {
			continue
		}
		spans = append(spans, span{start: start, end: end})
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })

	var out []Gap
	cursor := dayStart
	for _, sp := range spans {
		// ⚠️ Overlapping blocks must not reopen a gap behind the cursor. Two
		// things at 09:00–10:00 and 09:30–11:00 leave no gap at all, and a naive
		// "gap = this.start - previous.end" would report a negative-length one
		// between them — which downstream reads as an enormous free stretch.
		if sp.start.After(cursor) {
			out = append(out, Gap{Start: cursor, End: sp.start})
		}
		if sp.end.After(cursor) {
			cursor = sp.end
		}
	}
	if dayEnd.After(cursor) {
		out = append(out, Gap{Start: cursor, End: dayEnd})
	}
	return out
}

// LongestGap returns the widest gap of at least minMinutes, and whether one
// exists.
//
// ⚠️ Ties go to the EARLIER gap. A day with two equal openings should be offered
// the one that comes first — a suggestion for later today has more chance of
// being overtaken by events before it arrives.
func LongestGap(gaps []Gap, minMinutes int) (Gap, bool) {
	var best Gap
	found := false
	for _, g := range gaps {
		if g.Minutes() < minMinutes {
			continue
		}
		if !found || g.Minutes() > best.Minutes() {
			best, found = g, true
		}
	}
	return best, found
}
