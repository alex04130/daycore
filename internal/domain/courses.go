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
	// RemindersOff silences the deadline ladder for this one item without
	// changing what it is.
	//
	// Distinct from Status="dismissed" on purpose, and the difference is the
	// whole reason this field exists: dismissed means "I am not doing this", and
	// using it to mean "stop reminding me" would quietly delete the thing from
	// every count and every plan. Somebody who has the deadline under control
	// and does not want three more messages about it is not abandoning it.
	//
	// The fact track can only be turned off one item at a time (STRATEGY §1.3):
	// the global DeadlineAlerts toggle exists and is a blunt instrument, and a
	// user who silences everything because one item annoyed them is exactly the
	// failure "missing it has a real cost" is supposed to prevent.
	RemindersOff bool      `json:"remindersOff,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// DeadlineRungs is the fact-track escalation ladder: how long before something
// is due each warning goes out.
//
// STRATEGY §1.3 draws the line between the two tracks. The suggestion track is
// bounded by a daily push budget because ignoring a suggestion is always safe.
// The fact track is not: missing a deadline has a real cost, so it is bounded by
// STRUCTURE instead — three rungs per item, ever, and nothing else.
//
// That is what makes "it does not touch the ≤3/day budget" honest rather than a
// loophole: an assignment can produce at most three messages in its lifetime,
// and the number of assignments is the user's own, not something the system
// generates.
//
// ASCENDING, and the loop takes the first match — the TIGHTEST rung that still
// contains the item. Descending looks equally reasonable and is wrong: six hours
// left is `<= 24h`, so it would report the 24h rung forever and the 12h and 1h
// warnings would never fire.
var DeadlineRungs = []time.Duration{time.Hour, 12 * time.Hour, 24 * time.Hour}

// DeadlineRungFor returns the rung an assignment currently sits in, and whether
// it is in one at all.
//
// The rung is named by its own duration so that an item which crossed 24h while
// the process was down still gets exactly one warning when it comes back — the
// one for the rung it is in NOW, not a backlog of the ones it slept through.
// Occurrence keys are per (assignment, rung), so each fires at most once.
func DeadlineRungFor(due, now time.Time) (time.Duration, bool) {
	left := due.Sub(now)
	if left < 0 {
		// Overdue is its own rung: one message, not one per check.
		return 0, true
	}
	for _, r := range DeadlineRungs {
		if left <= r {
			return r, true
		}
	}
	return 0, false
}
