package domain

import (
	"context"
	"time"
)

// WeeklyLetter is the Sunday-evening prose letter: a reading of the week's
// river, moods and regrets, written by the model and stored for the app to
// show. It is deliberately NOT a chart or an aggregate — the weekly review is a
// letter, which is the whole reason the model writes it rather than the server
// folding numbers (see design-ui/liuli/app/cj-drawers.jsx, "散文，不是饼图").
type WeeklyLetter struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	// WeekStart is the YYYY-MM-DD of the Monday that began the week the letter
	// covers; WeekEnd is the Sunday that closed it. The worker fires on Sunday
	// evening, so it writes the seven days that just ended.
	WeekStart string `json:"weekStart"`
	WeekEnd   string `json:"weekEnd"`
	// Body is the prose itself. It carries no structure — the model writes it.
	Body      string    `json:"body"`
	Locale    string    `json:"locale"` // "zh-CN" | "en-US", the language the letter was written in
	CreatedAt time.Time `json:"createdAt"`
}

// WeeklyLetterRepository stores one prose letter per session per week. The
// worker's job_runs claim is what keeps the Sunday job at-once-per-week; the
// repository itself imposes no uniqueness so that an on-demand regeneration
// simply adds a newer letter and Latest returns it.
type WeeklyLetterRepository interface {
	// Latest returns the most recent letter for the session, or ErrNotFound.
	Latest(ctx context.Context, sessionID string) (*WeeklyLetter, error)
	// Get returns one letter by id, scoped to the session.
	Get(ctx context.Context, sessionID, id string) (*WeeklyLetter, error)
	// List returns letters newest-first, capped by ListLimit.
	List(ctx context.Context, sessionID string, limit int) ([]WeeklyLetter, error)
	// Create inserts a letter, minting an id when one is empty.
	Create(ctx context.Context, l *WeeklyLetter) (*WeeklyLetter, error)
	// Delete removes one letter. It exists to undo an on-demand generation.
	Delete(ctx context.Context, sessionID, id string) error
}
