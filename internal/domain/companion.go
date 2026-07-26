package domain

import "time"

// Role constants for Message.Role (history blob).
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

// Message is a single turn in the conversation history (companion memory blob).
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// CompanionMemory is the persisted chat history + extracted facts per session.
type CompanionMemory struct {
	ID                  string    `json:"id"`
	SessionID           string    `json:"sessionId"`
	KeyFacts            []string  `json:"keyFacts"`
	ConversationHistory []Message `json:"conversationHistory"`
	UpdatedAt           time.Time `json:"updatedAt"`
}
