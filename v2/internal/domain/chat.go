// Package domain holds the core entities and the storage-agnostic repository
// contracts. Nothing in this package may import a concrete database driver —
// it is the "interface" half of the database/interface separation.
package domain

import "time"

// ChatThread groups messages into a named conversation.
type ChatThread struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"` // accumulated sliding-window summary
	Archived  bool      `json:"archived"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ChatMessage status values (async companion turns). Rows written before the
// field existed have "" — readers must treat empty as done.
const (
	MsgStatusPending = "pending"
	MsgStatusDone    = "done"
	MsgStatusError   = "error"
)

// ChatMessage is one message (user/assistant/system/tool) in a thread.
type ChatMessage struct {
	ID         string    `json:"id"`
	ThreadID   string    `json:"threadId"`
	SessionID  string    `json:"sessionId"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	ToolEvents string    `json:"toolEvents,omitempty"` // JSON array of SSE v2 frames (tool/decision cards) for replay
	Status     string    `json:"status,omitempty"`     // ""≡done | pending | done | error
	CreatedAt  time.Time `json:"createdAt"`
}
