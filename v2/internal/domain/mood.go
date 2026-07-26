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
}
