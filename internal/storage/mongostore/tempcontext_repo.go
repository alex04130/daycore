package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type tempContextDoc struct {
	ID        string    `bson:"_id"`
	SessionID string    `bson:"session_id"`
	Key       string    `bson:"key"`
	Payload   string    `bson:"payload"`
	TTL       time.Time `bson:"ttl"`
	CreatedAt time.Time `bson:"created_at"`
}

type tempContextRepo struct{ *Store }

func (r tempContextRepo) Get(ctx context.Context, sid, key string) (*domain.TempContext, error) {
	var d tempContextDoc
	if err := r.c("temp_contexts").FindOne(ctx, bson.M{"session_id": sid, "key": key, "ttl": bson.M{"$gt": time.Now().UTC()}}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.TempContext{
		ID:        d.ID,
		SessionID: d.SessionID,
		Key:       d.Key,
		Payload:   d.Payload,
		TTL:       d.TTL,
		CreatedAt: d.CreatedAt,
	}, nil
}

func (r tempContextRepo) Set(ctx context.Context, tc *domain.TempContext) error {
	// Upsert keyed on (session_id, key), not _id: an empty _id previously
	// collapsed the whole collection into one shared document.
	if tc.ID == "" {
		tc.ID = uuid.NewString()
	}
	if tc.CreatedAt.IsZero() {
		tc.CreatedAt = time.Now().UTC()
	}
	_, err := r.c("temp_contexts").UpdateOne(ctx,
		bson.M{"session_id": tc.SessionID, "key": tc.Key},
		bson.M{
			"$set":         bson.M{"payload": tc.Payload, "ttl": tc.TTL},
			"$setOnInsert": bson.M{"_id": tc.ID, "created_at": tc.CreatedAt},
		},
		options.Update().SetUpsert(true))
	return err
}

func (r tempContextRepo) Delete(ctx context.Context, sid, key string) error {
	_, err := r.c("temp_contexts").DeleteOne(ctx, bson.M{"session_id": sid, "key": key})
	return err
}

func (r tempContextRepo) Expire(ctx context.Context) (int, error) {
	res, err := r.c("temp_contexts").DeleteMany(ctx, bson.M{"ttl": bson.M{"$lt": time.Now().UTC()}})
	if err != nil {
		return 0, err
	}
	return int(res.DeletedCount), nil
}
