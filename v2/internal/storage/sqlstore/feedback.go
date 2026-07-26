package sqlstore

import (
	"context"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type feedbackRepo struct{ *Store }

func (r feedbackRepo) Add(ctx context.Context, f *domain.FeedbackLog) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO feedback_logs (id, session_id, message_id, reason, useful, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		f.ID, f.SessionID, f.MessageID, f.Reason, boolToInt(f.Useful), now)
	if err != nil {
		return err
	}
	f.CreatedAt = fromMillis(now)
	return nil
}

func (r feedbackRepo) Stats(ctx context.Context) (useful, total int, err error) {
	row := r.queryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(useful), 0) FROM feedback_logs`)
	err = row.Scan(&total, &useful)
	return
}
