// Package domain holds the core entities and the storage-agnostic repository
// contracts. Nothing in this package may import a concrete database driver —
// it is the "interface" half of the database/interface separation.
package domain

import "time"

// ─── Block & message value types (ported from v1 daycore.ts) ────────────────

// BlockType is the category of a time block.
type BlockType string

const (
	BlockTask        BlockType = "task"
	BlockAppointment BlockType = "appointment"
	BlockBreak       BlockType = "break"
	BlockRelax       BlockType = "relax"
	BlockMeal        BlockType = "meal"
)

// TimeMode controls how a block is anchored across timezones.
type TimeMode string

const (
	TimeFloating TimeMode = "floating" // follows the local wall clock
	TimeFixed    TimeMode = "fixed"    // anchored to an absolute instant
	TimeLocal    TimeMode = "local"    // scheduled in the session's local timezone
)

// Origin constants for TimeBlock.Origin.
const (
	OriginAuto   = "auto"
	OriginManual = "manual"
	OriginRule   = "rule"
)

// LockLevel says how firmly a block's time slot is fixed. Some hours are simply
// not the user's to move — a lecture is set by the timetable, a meeting by the
// other party — and the planner has to know that before it reshuffles a day.
//
// The zero value is deliberately NOT "unlocked": it means "never derived yet",
// which is a third state the design prototype expressed as `undefined` (see
// design-ui/HANDOFF/01-core-contract.md §5). A block whose level was cleared by
// the user is LockNone, and that must survive a re-read — otherwise DeriveLock
// would helpfully lock the user's class right back up every time it is loaded.
// LockSource is what keeps those two apart.
type LockLevel string

const (
	LockUnset LockLevel = ""     // not derived yet; DeriveLock will fill it in
	LockNone  LockLevel = "none" // derived, and the slot is free to move
	LockSoft  LockLevel = "soft" // agreed with someone else; move needs confirmation
	LockHard  LockLevel = "hard" // set by a timetable; the user cannot move it at all
)

// LockSource records who decided the lock, so a derived guess never overwrites a
// deliberate choice.
const (
	LockSourceUnset   = ""        // never derived
	LockSourceDerived = "derived" // inferred from type + title; safe to recompute
	LockSourceUser    = "user"    // the user set it by hand; never recompute
	LockSourceAgent   = "agent"   // the agent set it; never recompute
)

// TimeBlock is a single scheduled item inside a day plan. The JSON shape is the
// wire contract shared with the AI prompts and the frontend, so the tags must
// stay stable.
type TimeBlock struct {
	ID            string    `json:"id"`
	Date          string    `json:"date,omitempty"` // YYYY-MM-DD; which day the block belongs to
	Time          *string   `json:"time"`           // HH:MM, or null when unscheduled
	Title         string    `json:"title"`
	Type          BlockType `json:"type"`
	DurationMin   *int      `json:"duration_min"` // no omitempty: revert must be able to restore a nil (cleared) duration
	TimeMode      TimeMode  `json:"time_mode"`
	Timezone      string    `json:"timezone"`
	UTCTime       *string   `json:"utc_time,omitempty"`
	OffsetMin     *int      `json:"offset_min,omitempty"`
	OffsetRef     string    `json:"offset_ref,omitempty"`
	Completed     bool      `json:"completed"`         // no omitempty: revert of "mark as done" needs the false to be present
	IsAchievement bool      `json:"isAchievement"`     // no omitempty: same reason as Completed
	RuleID        string    `json:"rule_id,omitempty"` // set when the block was expanded from a ScheduleRule
	Origin        string    `json:"origin,omitempty"`  // "auto" | "manual" | "rule"; empty = legacy/manual
	Hidden        bool      `json:"hidden,omitempty"`  // tombstone: user removed this rule occurrence for the day

	// Note is whatever the user felt like writing when they checked the block
	// off — how it went, what was left half-done. Always optional: we never ask
	// for it, we just keep a place for it, and the agent reads it to know the
	// user better.
	Note string `json:"note"` // no omitempty: revert must be able to restore a cleared note

	// Lock fields. All three are no-omitempty because revertPlanUpdate rebuilds
	// changes key-by-key from the "before" snapshot — a key missing from the
	// JSON is a key revert cannot restore.
	LockLevel  LockLevel `json:"lock_level"`
	LockReason string    `json:"lock_reason"`
	LockSource string    `json:"lock_source"`

	// Re-fishing: an unfinished block whose time was never the point (exercise,
	// reading) gets offered a new slot instead of quietly rotting in the past.
	// Bookings do not — a lecture you missed is missed. RescheduledFrom chains
	// each retry back to the original so RescheduleCount can stop the offers
	// once it is clear the user simply does not want to do this.
	RescheduledFrom string `json:"rescheduled_from,omitempty"`
	RescheduleCount int    `json:"reschedule_count,omitempty"`
}

// DayPlan is the set of time blocks for one session on one date.
type DayPlan struct {
	ID         string      `json:"id"`
	SessionID  string      `json:"sessionId"`
	Date       string      `json:"date"` // YYYY-MM-DD
	Blocks     []TimeBlock `json:"blocks"`
	SourceType string      `json:"sourceType"` // "text" | "image" | "auto"
	Note       *string     `json:"note"`
	CreatedAt  time.Time   `json:"createdAt"`
	UpdatedAt  time.Time   `json:"updatedAt"`
}
