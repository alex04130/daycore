package schedule

import (
	"testing"

	"daycore/internal/domain"
)

func strPtr(s string) *string { return &s }

func baseRule(id string) domain.ScheduleRule {
	return domain.ScheduleRule{
		ID: id, SessionID: "s1", Title: "r-" + id, Type: domain.BlockTask,
		Timezone: "UTC", TimeMode: domain.TimeFloating, Active: true, Interval: 1,
	}
}

func datesOf(blocks []domain.TimeBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, b.Date)
	}
	return out
}

func TestExpand(t *testing.T) {
	cases := []struct {
		name     string
		rule     func() domain.ScheduleRule
		from, to string
		want     []string // expected occurrence dates
	}{
		{
			name: "once inside range",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleOnce
				r.Date = strPtr("2026-07-10")
				return r
			},
			from: "2026-07-06", to: "2026-07-12",
			want: []string{"2026-07-10"},
		},
		{
			name: "once outside range",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleOnce
				r.Date = strPtr("2026-08-15")
				return r
			},
			from: "2026-07-06", to: "2026-07-12",
			want: nil,
		},
		{
			name: "every 3 days from anchor",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleRecurring
				r.Freq = domain.FreqEveryNDays
				r.Interval = 3
				r.StartDate = "2026-07-06"
				return r
			},
			from: "2026-07-06", to: "2026-07-14",
			want: []string{"2026-07-06", "2026-07-09", "2026-07-12"},
		},
		{
			name: "every_n_days without anchor never fires",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleRecurring
				r.Freq = domain.FreqEveryNDays
				r.Interval = 2
				return r
			},
			from: "2026-07-06", to: "2026-07-10",
			want: nil,
		},
		{
			name: "weekly mon/wed/fri",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleRecurring
				r.Freq = domain.FreqWeekly
				r.ByWeekday = []int{1, 3, 5}
				r.StartDate = "2026-07-06" // a Monday
				return r
			},
			from: "2026-07-06", to: "2026-07-12",
			want: []string{"2026-07-06", "2026-07-08", "2026-07-10"},
		},
		{
			name: "biweekly stays in phase across the off week",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleRecurring
				r.Freq = domain.FreqWeekly
				r.Interval = 2
				r.ByWeekday = []int{1} // Mondays
				r.StartDate = "2026-07-06"
				return r
			},
			from: "2026-07-06", to: "2026-07-27",
			want: []string{"2026-07-06", "2026-07-20"},
		},
		{
			name: "weekly defaults to the start date's weekday",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleRecurring
				r.Freq = domain.FreqWeekly
				r.StartDate = "2026-07-07" // a Tuesday
				return r
			},
			from: "2026-07-06", to: "2026-07-19",
			want: []string{"2026-07-07", "2026-07-14"},
		},
		{
			name: "daily bounded by until",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleRecurring
				r.Freq = domain.FreqDaily
				r.StartDate = "2026-07-08"
				r.Until = strPtr("2026-07-10")
				return r
			},
			from: "2026-07-06", to: "2026-07-14",
			want: []string{"2026-07-08", "2026-07-09", "2026-07-10"},
		},
		{
			name: "monthly on the anchor's day-of-month",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleRecurring
				r.Freq = domain.FreqMonthly
				r.StartDate = "2026-05-15"
				return r
			},
			from: "2026-07-01", to: "2026-08-31",
			want: []string{"2026-07-15", "2026-08-15"},
		},
		{
			name: "monthly on the 31st skips short months",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleRecurring
				r.Freq = domain.FreqMonthly
				r.StartDate = "2026-05-31"
				return r
			},
			from: "2026-06-01", to: "2026-07-31",
			want: []string{"2026-07-31"},
		},
		{
			name: "inactive rule never fires",
			rule: func() domain.ScheduleRule {
				r := baseRule("a")
				r.Kind = domain.RuleRecurring
				r.Freq = domain.FreqDaily
				r.Active = false
				return r
			},
			from: "2026-07-06", to: "2026-07-08",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Expand([]domain.ScheduleRule{tc.rule()}, tc.from, tc.to)
			gotDates := datesOf(got)
			if len(gotDates) != len(tc.want) {
				t.Fatalf("dates = %v, want %v", gotDates, tc.want)
			}
			for i := range tc.want {
				if gotDates[i] != tc.want[i] {
					t.Fatalf("dates = %v, want %v", gotDates, tc.want)
				}
			}
			for _, b := range got {
				if b.RuleID != "a" || b.Origin != domain.OriginRule {
					t.Fatalf("block %+v missing rule metadata", b)
				}
				if b.ID != BlockID("a", b.Date) {
					t.Fatalf("block id %q not deterministic", b.ID)
				}
			}
		})
	}
}

func TestMergeStoredWins(t *testing.T) {
	stored := []domain.TimeBlock{
		{ID: BlockID("a", "2026-07-06"), RuleID: "a", Title: "edited", Completed: true},
		{ID: "manual-1", Title: "manual"},
	}
	occ := []domain.TimeBlock{
		{ID: BlockID("a", "2026-07-06"), RuleID: "a", Title: "fresh"},
		{ID: BlockID("b", "2026-07-06"), RuleID: "b", Title: "new rule"},
	}
	got := Merge(stored, occ)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for _, b := range got {
		if b.RuleID == "a" && (b.Title != "edited" || !b.Completed) {
			t.Fatalf("stored block should win, got %+v", b)
		}
	}
}

func TestVisibleFiltersTombstones(t *testing.T) {
	blocks := []domain.TimeBlock{
		{ID: "1", Hidden: true},
		{ID: "2"},
	}
	got := Visible(blocks)
	if len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("Visible = %+v", got)
	}
}
