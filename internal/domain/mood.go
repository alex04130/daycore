// Package domain holds the core entities and the storage-agnostic repository
// contracts. Nothing in this package may import a concrete database driver —
// it is the "interface" half of the database/interface separation.
package domain

import "time"

// MoodCheckin records a single mood entry and the AI's response.
type MoodCheckin struct {
	ID                string    `json:"id"`
	SessionID         string    `json:"sessionId"`
	Mood              string    `json:"mood"`
	AIResponse        *string   `json:"aiResponse,omitempty"`
	ExerciseOffered   *string   `json:"exerciseOffered,omitempty"`
	ExerciseCompleted bool      `json:"exerciseCompleted"`
	Theme             *string   `json:"theme,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`

	// Source is MoodSourceUser or MoodSourceAgent. An agent check-in is an
	// inference from something the user said; a user one is them pressing a
	// button. Both are real, but they do not weigh the same and the user has to
	// be able to see which is which (EXPERIENCE_CORE §12.1).
	//
	// Rows written before this column existed carry "" and are read as user
	// check-ins: the agent could not record one yet, so that is what they are.
	Source string `json:"source,omitempty"`

	// Note is whatever the user wanted to add — how the day actually went, what
	// they got half-done. Never asked for; offered.
	Note string `json:"note,omitempty"`
}
