package domain

import "time"

// Session is the anonymous-first unit every piece of user data hangs off of.
type Session struct {
	ID               string    `json:"id"`
	UserID           *string   `json:"userId,omitempty"`
	InteractionCount int       `json:"interactionCount"`
	SignInPrompted   bool      `json:"signInPrompted"`
	AssistantName    string    `json:"assistantName"`
	CurrentTheme     string    `json:"currentTheme"`
	Language         string    // BCP-47 locale, e.g. "zh-CN" | "en-US"
	Preferences      string    `json:"preferences,omitempty"`
	PersonaPrompt    string    `json:"-"`
	ImportToken      string    `json:"-"` // secret for extension direct-push; never serialized
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// SessionUpdate carries optional fields for a partial session update. A nil
// pointer means "leave unchanged".
type SessionUpdate struct {
	UserID         *string
	AssistantName  *string
	CurrentTheme   *string
	SignInPrompted *bool
	Language       *string
	ImportToken    *string
	PersonaPrompt  *string
	Preferences    *string
}
