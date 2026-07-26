package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

type feedbackLogDoc struct {
	ID        string    `bson:"_id"`
	SessionID string    `bson:"session_id"`
	MessageID string    `bson:"message_id"`
	Reason    string    `bson:"reason"`
	Useful    bool      `bson:"useful"`
	CreatedAt time.Time `bson:"created_at"`
}

type feedbackRepo struct{ *Store }

func (r feedbackRepo) Add(ctx context.Context, f *domain.FeedbackLog) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	f.CreatedAt = time.Now().UTC()
	_, err := r.c("feedback_logs").InsertOne(ctx, feedbackLogDoc{
		ID:        f.ID,
		SessionID: f.SessionID,
		MessageID: f.MessageID,
		Reason:    f.Reason,
		Useful:    f.Useful,
		CreatedAt: f.CreatedAt,
	})
	return err
}

func (r feedbackRepo) Stats(ctx context.Context) (useful, total int, err error) {
	t, err := r.c("feedback_logs").CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, 0, err
	}
	u, err := r.c("feedback_logs").CountDocuments(ctx, bson.M{"useful": true})
	if err != nil {
		return 0, 0, err
	}
	return int(u), int(t), nil
}
