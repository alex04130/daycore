package mongostore

import (
	"context"
	"errors"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type opLogDoc struct {
	ID        string    `bson:"_id"`
	SessionID string    `bson:"session_id"`
	Actor     string    `bson:"actor"`
	Action    string    `bson:"action"`
	TargetID  string    `bson:"target_id"`
	Date      string    `bson:"date"`
	Summary   string    `bson:"summary"`
	Detail    string    `bson:"detail"`
	Status    string    `bson:"status"`
	RequestID string    `bson:"request_id"`
	CreatedAt time.Time `bson:"created_at"`
}

type opLogRepo struct{ *Store }

func (r opLogRepo) Add(ctx context.Context, l *domain.OperationLog) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	l.Summary = domain.ClampOpLogSummary(l.Summary)
	l.CreatedAt = time.Now().UTC()
	_, err := r.c("operation_logs").InsertOne(ctx, opLogDoc{
		ID: l.ID, SessionID: l.SessionID, Actor: l.Actor, Action: l.Action,
		TargetID: l.TargetID, Date: l.Date, Summary: l.Summary, Detail: l.Detail,
		Status: l.Status, RequestID: l.RequestID, CreatedAt: l.CreatedAt,
	})
	return err
}

func (r opLogRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.OperationLog, error) {
	if limit <= 0 {
		limit = 50
	}
	cur, err := r.c("operation_logs").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.OperationLog{}
	for cur.Next(ctx) {
		var d opLogDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.OperationLog{
			ID: d.ID, SessionID: d.SessionID, Actor: d.Actor, Action: d.Action,
			TargetID: d.TargetID, Date: d.Date, Summary: d.Summary, Detail: d.Detail,
			Status: d.Status, RequestID: d.RequestID, CreatedAt: d.CreatedAt,
		})
	}
	return out, cur.Err()
}

func (r opLogRepo) Get(ctx context.Context, sessionID, id string) (*domain.OperationLog, error) {
	var d opLogDoc
	if err := r.c("operation_logs").FindOne(ctx, bson.M{"_id": id, "session_id": sessionID}).Decode(&d); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.OperationLog{
		ID: d.ID, SessionID: d.SessionID, Actor: d.Actor, Action: d.Action,
		TargetID: d.TargetID, Date: d.Date, Summary: d.Summary, Detail: d.Detail,
		Status: d.Status, RequestID: d.RequestID, CreatedAt: d.CreatedAt,
	}, nil
}
