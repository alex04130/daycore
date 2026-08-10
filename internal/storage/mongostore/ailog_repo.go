package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

// aiLogFilterDoc renders the filter.
//
// ⚠️ Every narrowing value goes in as a VALUE, never spliced into a key or an
// expression. The SQL side's note applies here for the same reason: these
// arrive from a query string, and "the console only sends valid ones" is a
// property of one client rather than of this function.
func aiLogFilterDoc(f domain.AILogFilter) bson.M {
	q := bson.M{}
	if f.SessionID != "" {
		q["session_id"] = f.SessionID
	}
	if f.Endpoint != "" {
		q["endpoint"] = f.Endpoint
	}
	if f.Model != "" {
		q["model"] = f.Model
	}
	if f.Status != "" {
		q["status"] = f.Status
	}
	if !f.Since.IsZero() {
		q["created_at"] = bson.M{"$gte": f.Since.UnixMilli()}
	}
	if !f.Before.CreatedAt.IsZero() {
		ms := f.Before.CreatedAt.UnixMilli()
		// $or rather than folding into the created_at clause above, because both
		// bounds can be set at once and a second assignment to the same key would
		// silently drop Since. That is the shape of bug this back end has shipped
		// before: a filter that quietly stops filtering.
		q["$and"] = []bson.M{{"$or": []bson.M{
			{"created_at": bson.M{"$lt": ms}},
			{"created_at": ms, "_id": bson.M{"$lt": f.Before.ID}},
		}}}
	}
	return q
}

func (r aiLogRepo) List(ctx context.Context, f domain.AILogFilter, limit int) ([]domain.AICallLog, error) {
	limit = domain.ListLimit(limit, domain.AILogListDefault, domain.AILogListMax)
	cur, err := r.c("ai_call_logs").Find(ctx, aiLogFilterDoc(f),
		options.Find().
			SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).
			SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.AICallLog{}
	for cur.Next(ctx) {
		var d aiCallLogDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.AICallLog{
			ID: d.ID, SessionID: d.SessionID, Endpoint: d.Endpoint, Model: d.Model,
			PromptTokens: d.PromptTokens, CompTokens: d.CompTokens, DurationMs: d.DurationMs,
			Status: d.Status, Error: d.Error, RequestID: d.RequestID,
			CreatedAt: fromMillis(d.CreatedAt),
		})
	}
	return out, cur.Err()
}

func (r aiLogRepo) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.c("ai_call_logs").DeleteMany(ctx, bson.M{"created_at": bson.M{"$lt": before.UnixMilli()}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
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
