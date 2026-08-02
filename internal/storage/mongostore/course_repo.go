package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type courseDoc struct {
	ID           string    `bson:"_id"`
	SessionID    string    `bson:"session_id"`
	CanvasID     string    `bson:"canvas_id"`
	Name         string    `bson:"name"`
	CourseCode   string    `bson:"course_code"`
	CurrentScore *float64  `bson:"current_score,omitempty"`
	CurrentGrade *string   `bson:"current_grade,omitempty"`
	CreatedAt    time.Time `bson:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at"`
}

func (d courseDoc) toDomain() *domain.Course {
	return &domain.Course{
		ID: d.ID, SessionID: d.SessionID, CanvasID: d.CanvasID, Name: d.Name,
		CourseCode: d.CourseCode, CurrentScore: d.CurrentScore, CurrentGrade: d.CurrentGrade,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type courseRepo struct{ *Store }

func (r courseRepo) UpsertByCanvasID(ctx context.Context, c *domain.Course) (*domain.Course, error) {
	if c.CanvasID == "" {
		return nil, domain.ErrMissingUpsertKey
	}
	now := time.Now().UTC()
	filter := bson.M{"session_id": c.SessionID, "canvas_id": c.CanvasID}
	_, err := r.c("courses").UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{
			"name": c.Name, "course_code": c.CourseCode,
			"current_score": c.CurrentScore, "current_grade": c.CurrentGrade, "updated_at": now,
		},
		"$setOnInsert": bson.M{"_id": uuid.NewString(), "created_at": now},
	}, options.Update().SetUpsert(true))
	if err != nil {
		return nil, err
	}
	var d courseDoc
	if err := r.c("courses").FindOne(ctx, filter).Decode(&d); err != nil {
		return nil, err
	}
	return d.toDomain(), nil
}

func (r courseRepo) List(ctx context.Context, sessionID string) ([]domain.Course, error) {
	cur, err := r.c("courses").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Course{}
	for cur.Next(ctx) {
		var d courseDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}
