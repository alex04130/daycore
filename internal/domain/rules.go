package domain

import "time"

// ─── Schedule rule constants ────────────────────────────────────────────────

// RuleKind distinguishes one-off long-term items from recurring ones.
const (
	RuleOnce      = "once"      // a single occurrence on Date (supports far-future dates)
	RuleRecurring = "recurring" // repeats according to Freq/Interval/ByWeekday
)

// Rule frequencies (Kind == RuleRecurring).
const (
	FreqDaily      = "daily"        // every single day
	FreqWeekly     = "weekly"       // on ByWeekday weekdays, every Interval weeks
	FreqMonthly    = "monthly"      // on StartDate's day-of-month, every Interval months
	FreqEveryNDays = "every_n_days" // every Interval days counted from StartDate
)

// ─── Schedule rule entities ─────────────────────────────────────────────────

// ScheduleRule is a standing schedule commitment — the "long-term memory" form
// of a schedule: weekly course meetings, "water the plants every 3 days", or a
// precise far-future appointment. Rules are expanded into TimeBlocks (with
// RuleID set) when plans are read or generated.
type ScheduleRule struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"sessionId"`
	Title       string    `json:"title"`
	Type        BlockType `json:"type"`
	Time        *string   `json:"time"` // HH:MM, or null when unscheduled
	DurationMin *int      `json:"duration_min,omitempty"`
	Timezone    string    `json:"timezone"`
	TimeMode    TimeMode  `json:"time_mode"`
	Kind        string    `json:"kind"`                 // "once" | "recurring"
	Date        *string   `json:"date,omitempty"`       // YYYY-MM-DD; required when Kind == "once"
	Freq        string    `json:"freq,omitempty"`       // "daily" | "weekly" | "monthly" | "every_n_days"
	Interval    int       `json:"interval,omitempty"`   // every N days/weeks/months (>= 1)
	ByWeekday   []int     `json:"by_weekday,omitempty"` // 0=Sunday … 6=Saturday; used by "weekly"
	StartDate   string    `json:"start_date,omitempty"` // YYYY-MM-DD; first eligible date
	Until       *string   `json:"until,omitempty"`      // YYYY-MM-DD inclusive; nil = forever
	Active      bool      `json:"active"`
	Source      string    `json:"source"` // "user" | "chat" | "ics" | "image" | "canvas"
	Note        *string   `json:"note,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ScheduleRuleUpdate carries optional fields for a partial rule update.
type ScheduleRuleUpdate struct {
	Title       *string
	Type        *BlockType
	Time        *string // set to empty string to clear
	HasTime     bool    // true when Time carries a change (distinguishes nil from "clear")
	DurationMin *int
	Timezone    *string
	TimeMode    *TimeMode
	Kind        *string
	Date        *string
	Freq        *string
	Interval    *int
	ByWeekday   *[]int
	StartDate   *string
	Until       *string
	HasUntil    bool
	Active      *bool
	Note        *string
}
