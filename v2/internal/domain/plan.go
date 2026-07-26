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
	Completed     bool      `json:"completed"`     // no omitempty: revert of "mark as done" needs the false to be present
	IsAchievement bool      `json:"isAchievement"` // no omitempty: same reason as Completed
	RuleID        string    `json:"rule_id,omitempty"` // set when the block was expanded from a ScheduleRule
	Origin        string    `json:"origin,omitempty"`  // "auto" | "manual" | "rule"; empty = legacy/manual
	Hidden        bool      `json:"hidden,omitempty"`  // tombstone: user removed this rule occurrence for the day
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
