package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type dayPlanDoc struct {
	ID         string             `bson:"_id"`
	SessionID  string             `bson:"session_id"`
	Date       string             `bson:"date"`
	Blocks     []domain.TimeBlock `bson:"blocks"`
	SourceType string             `bson:"source_type"`
	Note       *string            `bson:"note,omitempty"`
	CreatedAt  time.Time          `bson:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at"`
}

func (d dayPlanDoc) toDomain() *domain.DayPlan {
	blocks := d.Blocks
	if blocks == nil {
		blocks = []domain.TimeBlock{}
	}
	return &domain.DayPlan{
		ID: d.ID, SessionID: d.SessionID, Date: d.Date, Blocks: blocks,
		SourceType: d.SourceType, Note: d.Note, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type dayPlanRepo struct{ *Store }

func (r dayPlanRepo) Get(ctx context.Context, sessionID, date string) (*domain.DayPlan, error) {
	var d dayPlanDoc
	if err := r.c("day_plans").FindOne(ctx, bson.M{"session_id": sessionID, "date": date}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r dayPlanRepo) Upsert(ctx context.Context, plan *domain.DayPlan) (*domain.DayPlan, error) {
	if plan.Blocks == nil {
		plan.Blocks = []domain.TimeBlock{}
	}
	sourceType := plan.SourceType
	if sourceType == "" {
		sourceType = "text"
	}
	now := time.Now().UTC()
	_, err := r.c("day_plans").UpdateOne(ctx,
		bson.M{"session_id": plan.SessionID, "date": plan.Date},
		bson.M{
			"$set":         bson.M{"blocks": plan.Blocks, "source_type": sourceType, "note": plan.Note, "updated_at": now},
			"$setOnInsert": bson.M{"_id": uuid.NewString(), "created_at": now},
		},
		options.Update().SetUpsert(true))
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, plan.SessionID, plan.Date)
}

func (r dayPlanRepo) Range(ctx context.Context, sessionID, from, to string) ([]domain.DayPlan, error) {
	cur, err := r.c("day_plans").Find(ctx,
		bson.M{"session_id": sessionID, "date": bson.M{"$gte": from, "$lte": to}},
		options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []domain.DayPlan
	for cur.Next(ctx) {
		var d dayPlanDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}
