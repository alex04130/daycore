// Package domain holds the core entities and the storage-agnostic repository
// contracts. Nothing in this package may import a concrete database driver —
// it is the "interface" half of the database/interface separation.
package domain

import "time"

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
