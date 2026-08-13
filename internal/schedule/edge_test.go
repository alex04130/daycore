package schedule

import (
	"testing"
	"time"

	"daycore/internal/domain"
)

func TestExpandInvalidRangeIsNil(t *testing.T) {
	if got := Expand(nil, "bad-date", "2026-01-10"); got != nil {
		t.Errorf("a bad from date must yield nil, got %v", got)
	}
	if got := Expand(nil, "2026-01-10", "bad-date"); got != nil {
		t.Errorf("a bad to date must yield nil, got %v", got)
	}
	// An inverted range is a caller error, not something to silently reorder.
	if got := Expand(nil, "2026-01-10", "2026-01-01"); got != nil {
		t.Errorf("an inverted range must yield nil, got %v", got)
	}
}

func TestExpandClampsTheAbuseGuard(t *testing.T) {
	r := domain.ScheduleRule{ID: "r", Active: true, Kind: domain.RuleOnce, Date: strptr("2026-01-15")}
	got := Expand([]domain.ScheduleRule{r}, "2026-01-01", "2027-01-01")
	// The range is clamped to maxExpandDays from the start; the once-rule at
	// Jan 15 is inside, so exactly one occurrence comes back.
	if len(got) != 1 {
		t.Fatalf("clamped expansion must still find the rule, got %d blocks", len(got))
	}
	got = Expand([]domain.ScheduleRule{r}, "2026-01-01", "2027-06-01")
	if len(got) != 1 || got[0].Date != "2026-01-15" {
		t.Fatalf("got %+v", got)
	}
}

func strptr(s string) *string { return &s }

func TestOccursEdgeMatrix(t *testing.T) {
	d := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC) // a Thursday
	rec := func(freq string) domain.ScheduleRule {
		return domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: freq}
	}
	cases := []struct {
		name string
		r    domain.ScheduleRule
		want bool
	}{
		{"inactive never occurs", domain.ScheduleRule{Active: false, Kind: domain.RuleRecurring, Freq: domain.FreqDaily}, false},
		{"once with nil date", domain.ScheduleRule{Active: true, Kind: domain.RuleOnce}, false},
		{"once wrong date", domain.ScheduleRule{Active: true, Kind: domain.RuleOnce, Date: strptr("2026-01-16")}, false},
		{"once right date", domain.ScheduleRule{Active: true, Kind: domain.RuleOnce, Date: strptr("2026-01-15")}, true},
		{"daily", rec(domain.FreqDaily), true},
		{"daily before start", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqDaily, StartDate: "2026-02-01"}, false},
		{"daily after until", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqDaily, Until: strptr("2026-01-10")}, false},
		{"bad start date", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqDaily, StartDate: "garbage"}, false},
		{"bad until", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqDaily, Until: strptr("garbage")}, false},
		{"every-n-days without anchor", rec(domain.FreqEveryNDays), false},
		{"every-2-days in phase", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqEveryNDays, Interval: 2, StartDate: "2026-01-15"}, true},
		{"every-2-days out of phase", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqEveryNDays, Interval: 2, StartDate: "2026-01-16"}, false},
		{"weekly on the right day", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqWeekly, ByWeekday: []int{int(time.Thursday)}}, true},
		{"weekly wrong day", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqWeekly, ByWeekday: []int{int(time.Friday)}}, false},
		{"weekly no weekdays, no anchor", rec(domain.FreqWeekly), false},
		{"weekly interval>1 without anchor", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqWeekly, Interval: 2}, false},
		{"monthly without anchor", rec(domain.FreqMonthly), false},
		{"monthly right day", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqMonthly, StartDate: "2026-01-15"}, true},
		{"monthly wrong day", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqMonthly, StartDate: "2026-01-16"}, false},
		{"monthly every 2 months in phase", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqMonthly, Interval: 2, StartDate: "2026-01-15"}, true},
		{"monthly every 2 months out of phase", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqMonthly, Interval: 2, StartDate: "2026-02-15"}, false},
		{"interval below 1 becomes 1", domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqEveryNDays, Interval: -3, StartDate: "2026-01-15"}, true},
		{"unknown freq", rec("HOURLY"), false},
		// Occurs never checks Kind against RuleRecurring: anything that is not
		// RuleOnce falls into the Freq switch. Locked as the current behaviour
		// — validation belongs to the rule write path, not the expander.
		{"unknown kind with a valid freq still occurs", domain.ScheduleRule{Active: true, Kind: "weird", Freq: domain.FreqDaily}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Occurs(&tc.r, d); got != tc.want {
				t.Errorf("Occurs = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMonthlySkipsShortMonths(t *testing.T) {
	// The 31st of a month that does not have one is skipped, not wrapped.
	r := domain.ScheduleRule{Active: true, Kind: domain.RuleRecurring, Freq: domain.FreqMonthly, StartDate: "2026-01-31"}
	if Occurs(&r, time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)) {
		t.Error("Feb has no 31st; the occurrence must be skipped")
	}
	if !Occurs(&r, time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)) {
		t.Error("Mar 31 must occur")
	}
}

func TestMergeEmptyRuleIDOccurrence(t *testing.T) {
	// Stored blocks only mark non-empty RuleIDs as seen, so a virtual
	// occurrence with an empty RuleID can never be deduped against anything.
	stored := []domain.TimeBlock{{ID: "manual", Title: "手工块"}}
	occ := []domain.TimeBlock{{ID: "rule-x-2026-01-15", Title: "虚拟块"}}
	got := Merge(stored, occ)
	if len(got) != 2 {
		t.Fatalf("an occurrence with an empty RuleID is added alongside, got %d", len(got))
	}
	// With a RuleID, the stored block wins and the occurrence is dropped.
	stored = []domain.TimeBlock{{ID: "b", RuleID: "r1", Title: "已存"}}
	occ = []domain.TimeBlock{{ID: "rule-r1-2026-01-15", RuleID: "r1", Title: "虚拟"}}
	got = Merge(stored, occ)
	if len(got) != 1 || got[0].Title != "已存" {
		t.Errorf("the stored block must win, got %+v", got)
	}
}

func TestVisibleKeepsNilInputEmpty(t *testing.T) {
	// Visible(nil) comes back as an empty non-nil slice — callers range over
	// it either way; what must hold is zero length.
	if got := Visible(nil); len(got) != 0 {
		t.Errorf("Visible(nil) must have zero length, got %v", got)
	}
	if got := Visible([]domain.TimeBlock{}); len(got) != 0 {
		t.Errorf("Visible(empty) must stay empty, got %v", got)
	}
}
