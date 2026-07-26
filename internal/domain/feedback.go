package domain

import (
	"context"
	"time"
)

type FeedbackLog struct {
	ID        string
	SessionID string
	MessageID string
	Reason    string
	Useful    bool
	CreatedAt time.Time
}

type FeedbackLogRepository interface {
	Add(ctx context.Context, feedback *FeedbackLog) error
	// Stats returns how many feedback entries were marked useful vs the total,
	// so the collected feedback is actually surfaced (admin dashboard).
	Stats(ctx context.Context) (useful, total int, err error)
}
