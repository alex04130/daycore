// Package schedule turns ScheduleRules (recurring & long-term one-off items)
// into concrete TimeBlocks for a date range, and merges those occurrences into
// stored day plans. Pure functions, no storage or HTTP dependencies.
package schedule

import (
	"fmt"
	"time"

	"daycore/internal/domain"
)

const dateLayout = "2006-01-02"

// maxExpandDays caps how many days a single expansion walks, as an abuse guard.
const maxExpandDays = 366

// BlockID is the deterministic id of a rule occurrence, so a materialized copy
// and a virtual expansion of the same occurrence always collide (and dedupe).
func BlockID(ruleID, date string) string {
	return fmt.Sprintf("rule-%s-%s", ruleID, date)
}

// Expand materializes every active rule occurrence within [from, to]
// (inclusive, YYYY-MM-DD). Blocks come back ordered by date then time.
// Invalid dates or an inverted range yield nil.
func Expand(rules []domain.ScheduleRule, from, to string) []domain.TimeBlock {
	start, err1 := time.Parse(dateLayout, from)
	end, err2 := time.Parse(dateLayout, to)
	if err1 != nil || err2 != nil || end.Before(start) {
		return nil
	}
	if end.Sub(start) > maxExpandDays*24*time.Hour {
		end = start.AddDate(0, 0, maxExpandDays)
	}

	var out []domain.TimeBlock
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		date := d.Format(dateLayout)
		for i := range rules {
			r := &rules[i]
			if Occurs(r, d) {
				out = append(out, occurrenceBlock(r, date))
			}
		}
	}
	return out
}

// Occurs reports whether rule r has an occurrence on day d (a parsed
// YYYY-MM-DD, time components zero).
func Occurs(r *domain.ScheduleRule, d time.Time) bool {
	if !r.Active {
		return false
	}
	date := d.Format(dateLayout)

	if r.Kind == domain.RuleOnce {
		return r.Date != nil && *r.Date == date
	}

	// Recurring: bounded by [StartDate, Until].
	var start time.Time
	if r.StartDate != "" {
		t, err := time.Parse(dateLayout, r.StartDate)
		if err != nil {
			return false
		}
		start = t
		if d.Before(start) {
			return false
		}
	}
	if r.Until != nil && *r.Until != "" {
		u, err := time.Parse(dateLayout, *r.Until)
		if err != nil || d.After(u) {
			return false
		}
	}

	interval := r.Interval
	if interval < 1 {
		interval = 1
	}

	switch r.Freq {
	case domain.FreqDaily:
		return true
	case domain.FreqEveryNDays:
		if start.IsZero() {
			return false // needs an anchor; validation enforces StartDate
		}
		days := int(d.Sub(start).Hours() / 24)
		return days%interval == 0
	case domain.FreqWeekly:
		weekdays := r.ByWeekday
		if len(weekdays) == 0 {
			if start.IsZero() {
				return false
			}
			weekdays = []int{int(start.Weekday())}
		}
		if !containsInt(weekdays, int(d.Weekday())) {
			return false
		}
		if interval > 1 {
			if start.IsZero() {
				return false
			}
			// Weeks are anchored to the Sunday of StartDate's week (0=Sunday
			// encoding), so "every 2 weeks on Mon/Wed" stays in phase.
			weeks := int(weekStart(d).Sub(weekStart(start)).Hours()/24) / 7
			return weeks%interval == 0
		}
		return true
	case domain.FreqMonthly:
		if start.IsZero() {
			return false
		}
		if d.Day() != start.Day() {
			return false // months lacking the day (e.g. the 31st) are skipped
		}
		months := (d.Year()-start.Year())*12 + int(d.Month()) - int(start.Month())
		return months%interval == 0
	}
	return false
}

func occurrenceBlock(r *domain.ScheduleRule, date string) domain.TimeBlock {
	return domain.TimeBlock{
		ID:          BlockID(r.ID, date),
		Date:        date,
		Time:        r.Time,
		Title:       r.Title,
		Type:        r.Type,
		DurationMin: r.DurationMin,
		TimeMode:    r.TimeMode,
		Timezone:    r.Timezone,
		RuleID:      r.ID,
		Origin:      domain.OriginRule,
	}
}

// Merge combines stored plan blocks with virtual rule occurrences for one day.
// A stored block with the same RuleID wins (it may carry completion state or a
// tombstone), so occurrences are only added when not yet represented.
func Merge(stored, occurrences []domain.TimeBlock) []domain.TimeBlock {
	seen := map[string]bool{}
	for _, b := range stored {
		if b.RuleID != "" {
			seen[b.RuleID] = true
		}
	}
	out := append([]domain.TimeBlock{}, stored...)
	for _, b := range occurrences {
		if !seen[b.RuleID] {
			out = append(out, b)
		}
	}
	return out
}

// Visible filters out tombstoned rule blocks (removed by the user but kept so
// the occurrence does not resurface on the next read).
func Visible(blocks []domain.TimeBlock) []domain.TimeBlock {
	out := make([]domain.TimeBlock, 0, len(blocks))
	for _, b := range blocks {
		if !b.Hidden {
			out = append(out, b)
		}
	}
	return out
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// weekStart returns the Sunday beginning d's week.
func weekStart(d time.Time) time.Time {
	return d.AddDate(0, 0, -int(d.Weekday()))
}
