// Package domain holds the core entities and the storage-agnostic repository
// contracts. Nothing in this package may import a concrete database driver —
// it is the "interface" half of the database/interface separation.
package domain

import (
	"strings"
	"time"
)

// ─── Operation log actors ────────────────────────────────────────────────────

const (
	ActorUser   = "user"
	ActorAgent  = "agent"
	ActorSystem = "system"
)

// ─── Operation log statuses ──────────────────────────────────────────────────

const (
	OpStatusOK     = "ok"
	OpStatusFailed = "failed"
)

// ─── Operation domains ───────────────────────────────────────────────────────

// OpDomain groups an operation by the part of the user's life it touched. Two
// features read it: the river colours its bands by domain, and rapport (how much
// latitude the user has given the agent) is scored per domain — being trusted
// with the schedule says nothing about being trusted to archive things.
//
// Rapport is only kept for the first four; OpDomainSystem is plumbing and never
// earns or loses any.
const (
	OpDomainSchedule = "schedule" // plans, blocks, rules — anything on the timeline
	OpDomainHabit    = "habit"    // recurring patterns the agent inferred
	OpDomainArchive  = "archive"  // materials, memories, wishes — the filing cabinet
	OpDomainCare     = "care"     // moods, check-ins, the 20h protector
	OpDomainSystem   = "system"   // settings, themes, imports; not scored
)

// OpDomainOf maps an operation action to its domain. Unknown actions fall back
// to system so a new action never silently skews someone's rapport score.
func OpDomainOf(action string) string {
	switch {
	case strings.HasPrefix(action, "plan_"), strings.HasPrefix(action, "autoplan"):
		return OpDomainSchedule
	case strings.HasPrefix(action, "rule_"):
		return OpDomainHabit
	case strings.HasPrefix(action, "memory_"), strings.HasPrefix(action, "material_"),
		strings.HasPrefix(action, "wish_"), strings.HasPrefix(action, "assignment_"):
		return OpDomainArchive
	case strings.HasPrefix(action, "mood_"), strings.HasPrefix(action, "protector_"):
		return OpDomainCare
	default:
		return OpDomainSystem
	}
}

// ─── Summary limit ───────────────────────────────────────────────────────────

// OpLogSummaryLimit caps OperationLog.Summary; every write path truncates via
// ClampOpLogSummary. Detail is unbounded.
const OpLogSummaryLimit = 512

// ClampOpLogSummary truncates s to OpLogSummaryLimit runes.
func ClampOpLogSummary(s string) string {
	r := []rune(s)
	if len(r) <= OpLogSummaryLimit {
		return s
	}
	return string(r[:OpLogSummaryLimit])
}

// ─── Structs ─────────────────────────────────────────────────────────────────

// OperationLog is one row of the generic write-audit log. Summary is the short
// list-view line; Detail carries the full JSON payload, including before/after
// snapshots so destructive writes stay recoverable.
type OperationLog struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Actor     string    `json:"actor"`  // "user" | "agent" | "system"
	Action    string    `json:"action"` // e.g. "plan_upsert", "rule_delete"
	Domain    string    `json:"domain"` // see OpDomain*; derived via OpDomainOf when not set
	TargetID  string    `json:"targetId,omitempty"`
	Date      string    `json:"date,omitempty"` // YYYY-MM-DD the operation targets
	Summary   string    `json:"summary"`
	Detail    string    `json:"detail,omitempty"`
	Status    string    `json:"status"` // "ok" | "failed"
	RequestID string    `json:"requestId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// AICallLog is one model call record.
type AICallLog struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"sessionId"`
	Endpoint     string    `json:"endpoint"`
	Model        string    `json:"model"`
	PromptTokens int       `json:"promptTokens"`
	CompTokens   int       `json:"compTokens"`
	DurationMs   int64     `json:"durationMs"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	RequestID    string    `json:"requestId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// AdminStats is an admin stats snapshot (read-only, no persistence).
type AdminStats struct {
	Users          int   `json:"users"`
	Sessions       int   `json:"sessions"`
	AICalls        int   `json:"aiCalls"`
	TokenUsed      int64 `json:"tokenUsed"`
	FeedbackUseful int   `json:"feedbackUseful"`
	FeedbackTotal  int   `json:"feedbackTotal"`
}
