// Package domain holds the core entities and the storage-agnostic repository
// contracts. Nothing in this package may import a concrete database driver —
// it is the "interface" half of the database/interface separation.
package domain

import "time"

// ─── Canvas LMS materials (imported by the browser extension) ────────────────

// Course is one Canvas course with its current grade snapshot.
type Course struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"sessionId"`
	CanvasID     string    `json:"canvasId"`
	Name         string    `json:"name"`
	CourseCode   string    `json:"courseCode,omitempty"`
	CurrentScore *float64  `json:"currentScore,omitempty"` // 0–100
	CurrentGrade *string   `json:"currentGrade,omitempty"` // letter grade, e.g. "A-"
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Assignment statuses (planner workflow, not Canvas submission state).
const (
	AssignmentPending   = "pending"   // not yet planned
	AssignmentPlanned   = "planned"   // auto-plan placed work blocks for it
	AssignmentDone      = "done"      // user marked finished
	AssignmentDismissed = "dismissed" // user hid it from planning
)

// Assignment is one Canvas assignment (or a manually added deadline).
type Assignment struct {
	ID             string     `json:"id"`
	SessionID      string     `json:"sessionId"`
	CourseID       string     `json:"courseId,omitempty"` // our Course.ID, not the Canvas id
	CanvasID       string     `json:"canvasId,omitempty"`
	Title          string     `json:"title"`
	DueAt          *time.Time `json:"dueAt,omitempty"`
	PointsPossible *float64   `json:"pointsPossible,omitempty"`
	Submitted      bool       `json:"submitted"`
	Graded         bool       `json:"graded"`
	Score          *float64   `json:"score,omitempty"`
	HTMLURL        string     `json:"htmlUrl,omitempty"`
	Source         string     `json:"source"` // "canvas" | "manual"
	Status         string     `json:"status"` // "pending" | "planned" | "done" | "dismissed"
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}
