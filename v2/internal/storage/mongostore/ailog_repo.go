package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type aiCallLogDoc struct {
	ID           string `bson:"_id"`
	SessionID    string `bson:"session_id"`
	Endpoint     string `bson:"endpoint"`
	Model        string `bson:"model"`
	PromptTokens int    `bson:"prompt_tokens"`
	CompTokens   int    `bson:"comp_tokens"`
	DurationMs   int64  `bson:"duration_ms"`
	Status       string `bson:"status"`
	Error        string `bson:"error,omitempty"`
	RequestID    string `bson:"request_id,omitempty"`
	CreatedAt    int64  `bson:"created_at"`
}

type aiLogRepo struct{ *Store }

func (r aiLogRepo) Add(ctx context.Context, l *domain.AICallLog) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.c("ai_call_logs").InsertOne(ctx, aiCallLogDoc{l.ID, l.SessionID, l.Endpoint, l.Model, l.PromptTokens, l.CompTokens, l.DurationMs, l.Status, l.Error, l.RequestID, now})
	if err != nil {
		return err
	}
	l.CreatedAt = fromMillis(now)
	return nil
}

func (r aiLogRepo) Stats(ctx context.Context) (*domain.AdminStats, error) {
	var s domain.AdminStats
	uc, _ := r.c("users").CountDocuments(ctx, bson.M{})
	sc, _ := r.c("sessions").CountDocuments(ctx, bson.M{})
	ac, _ := r.c("ai_call_logs").CountDocuments(ctx, bson.M{})
	s.Users = int(uc)
	s.Sessions = int(sc)
	s.AICalls = int(ac)
	// Sum comp_tokens via aggregation
	pipe := mongo.Pipeline{{
		{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "total", Value: bson.D{{Key: "$sum", Value: "$comp_tokens"}}}}},
	}}
	cur, err := r.c("ai_call_logs").Aggregate(ctx, pipe)
	if err == nil {
		defer cur.Close(ctx)
		if cur.Next(ctx) {
			var result struct{ Total int64 }
			cur.Decode(&result)
			s.TokenUsed = result.Total
		}
	}
	return &s, nil
}

func nowMillis() int64              { return time.Now().UnixMilli() }
func fromMillis(ms int64) time.Time { return time.UnixMilli(ms) }
