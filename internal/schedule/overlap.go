package schedule

import (
	"sort"
	"time"

	"daycore/internal/domain"
)

// Overlap is two blocks claiming the same minutes.
type Overlap struct {
	A, B  domain.TimeBlock
	Start time.Time // the contested span
	End   time.Time
}

// Overlaps finds every pair of blocks on a day that claim the same minutes.
//
// It is deliberately a query and not a rule. EXPERIENCE_CORE is explicit that a
// scheduling clash becomes a decision proposal rather than being resolved
// quietly — the system does not know which of the two matters more, and
// guessing is how an assistant ends up moving the thing the user cared about.
// So this reports; deciding is somebody else's job.
//
// What counts as a clash is narrower than "the numbers touch":
//
//   - Blocks that end exactly when another begins do not overlap. Back-to-back
//     is how a day is supposed to look.
//   - Tombstoned occurrences do not overlap anything: the user already said
//     that one is not happening.
//   - Unscheduled blocks (no time, or no duration) claim no minutes at all. A
//     to-do with no slot cannot clash with anything, and treating it as a
//     zero-length event at midnight would make every loose task fight the
//     others.
func Overlaps(blocks []domain.TimeBlock, planDate string, loc *time.Location) []Overlap {
	type span struct {
		b          domain.TimeBlock
		start, end time.Time
	}
	spans := make([]span, 0, len(blocks))
	for _, b := range blocks {
		if b.Hidden {
			continue
		}
		start, end, ok := b.Span(planDate, loc)
		if !ok || !end.After(start) {
			continue
		}
		spans = append(spans, span{b: b, start: start, end: end})
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })

	var out []Overlap
	for i := range spans {
		for j := i + 1; j < len(spans); j++ {
			// Sorted by start, so once one begins at or after this one ends,
			// every later one does too.
			if !spans[j].start.Before(spans[i].end) {
				break
			}
			start, end := spans[j].start, spans[i].end
			if spans[j].end.Before(end) {
				end = spans[j].end
			}
			out = append(out, Overlap{A: spans[i].b, B: spans[j].b, Start: start, End: end})
		}
	}
	return out
}
