package domain

import "testing"

func TestOpDomainOf(t *testing.T) {
	cases := []struct{ action, want string }{
		// schedule — anything that lands on the timeline
		{"plan_add", OpDomainSchedule},
		{"plan_update", OpDomainSchedule},
		{"plan_remove", OpDomainSchedule},
		{"plan_upsert", OpDomainSchedule},
		{"plan_bulk", OpDomainSchedule},
		{"plan_lock", OpDomainSchedule},
		{"autoplan", OpDomainSchedule},

		// habit — recurring patterns
		{"rule_create", OpDomainHabit},
		{"rule_update", OpDomainHabit},
		{"rule_delete", OpDomainHabit},
		{"rule_batch", OpDomainHabit},

		// archive — the filing cabinet
		{"memory_add", OpDomainArchive},
		{"memory_delete", OpDomainArchive},
		{"memory_clear", OpDomainArchive},
		{"material_create", OpDomainArchive},
		{"wish_create", OpDomainArchive},
		{"assignment_upsert", OpDomainArchive},

		// care — how the user is doing
		{"mood_record", OpDomainCare},
		{"protector_nudge", OpDomainCare},

		// system — plumbing, never scored
		{"theme_create", OpDomainSystem},
		{"import_canvas", OpDomainSystem},
		{"session_update", OpDomainSystem},
		{"", OpDomainSystem},
		{"something_nobody_wrote_yet", OpDomainSystem},
	}
	for _, c := range cases {
		if got := OpDomainOf(c.action); got != c.want {
			t.Errorf("OpDomainOf(%q) = %q, want %q", c.action, got, c.want)
		}
	}
}

// An unknown action must land in system rather than in a scored domain — a new
// action added elsewhere should never silently move someone's rapport score.
func TestOpDomainOfUnknownIsNotScored(t *testing.T) {
	scored := map[string]bool{
		OpDomainSchedule: true, OpDomainHabit: true,
		OpDomainArchive: true, OpDomainCare: true,
	}
	for _, action := range []string{"brand_new_thing", "xyz", "plan", "rule", "mood"} {
		if scored[OpDomainOf(action)] {
			t.Errorf("unknown action %q was scored as %q", action, OpDomainOf(action))
		}
	}
}
