package domain

import "time"

// MemoryFact is one durable fact about the user, injected into agent prompts.
type MemoryFact struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Fact      string    `json:"fact"`
	Source    string    `json:"source"`         // "chat" | "user" | "import" | "auto"
	Type      string    `json:"type,omitempty"` // "" = fact, "open_loop" = unresolved item
	CreatedAt time.Time `json:"createdAt"`
}

// ImportRecord is one append-only archive entry.
type ImportRecord struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Source    string    `json:"source"` // "canvas" | "ics" | "image"
	Items     int       `json:"items"`
	Summary   string    `json:"summary"`
	Payload   string    `json:"-"` // raw upload, archived not listed
	CreatedAt time.Time `json:"createdAt"`
}
