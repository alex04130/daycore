package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type factDoc struct {
	ID        string    `bson:"_id"`
	SessionID string    `bson:"session_id"`
	Fact      string    `bson:"fact"`
	Source    string    `bson:"source"`
	Type      string    `bson:"type"`
	CreatedAt time.Time `bson:"created_at"`
}

type importDoc struct {
	ID        string    `bson:"_id"`
	SessionID string    `bson:"session_id"`
	Source    string    `bson:"source"`
	Items     int       `bson:"items"`
	Summary   string    `bson:"summary"`
	Payload   string    `bson:"payload"`
	CreatedAt time.Time `bson:"created_at"`
}

type memoryRepo struct{ *Store }

func (r memoryRepo) ListFacts(ctx context.Context, sessionID string) ([]domain.MemoryFact, error) {
	cur, err := r.c("memory_facts").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.MemoryFact{}
	for cur.Next(ctx) {
		var d factDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.MemoryFact{ID: d.ID, SessionID: d.SessionID, Fact: d.Fact, Source: d.Source, Type: d.Type, CreatedAt: d.CreatedAt})
	}
	return out, cur.Err()
}

func (r memoryRepo) AddFact(ctx context.Context, f *domain.MemoryFact) (*domain.MemoryFact, error) {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	if f.Source == "" {
		f.Source = "chat"
	}
	f.CreatedAt = time.Now().UTC()
	_, err := r.c("memory_facts").InsertOne(ctx, factDoc{
		ID: f.ID, SessionID: f.SessionID, Fact: f.Fact, Source: f.Source, Type: f.Type, CreatedAt: f.CreatedAt,
	})
	return f, err
}

func (r memoryRepo) DeleteFact(ctx context.Context, sessionID, id string) error {
	res, err := r.c("memory_facts").DeleteOne(ctx, bson.M{"_id": id, "session_id": sessionID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r memoryRepo) ClearFacts(ctx context.Context, sessionID string) (int, error) {
	res, err := r.c("memory_facts").DeleteMany(ctx, bson.M{"session_id": sessionID})
	if err != nil {
		return 0, err
	}
	return int(res.DeletedCount), nil
}

func (r memoryRepo) AddImport(ctx context.Context, rec *domain.ImportRecord) (*domain.ImportRecord, error) {
	if rec.ID == "" {
		rec.ID = uuid.NewString()
	}
	rec.CreatedAt = time.Now().UTC()
	_, err := r.c("import_history").InsertOne(ctx, importDoc{
		ID: rec.ID, SessionID: rec.SessionID, Source: rec.Source, Items: rec.Items,
		Summary: rec.Summary, Payload: rec.Payload, CreatedAt: rec.CreatedAt,
	})
	return rec, err
}

func (r memoryRepo) ListImports(ctx context.Context, sessionID string, limit int) ([]domain.ImportRecord, error) {
	limit = domain.ListLimit(limit, domain.ImportListDefault, domain.ImportListMax)
	cur, err := r.c("import_history").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)).
			SetProjection(bson.M{"payload": 0}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.ImportRecord{}
	for cur.Next(ctx) {
		var d importDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.ImportRecord{ID: d.ID, SessionID: d.SessionID, Source: d.Source,
			Items: d.Items, Summary: d.Summary, CreatedAt: d.CreatedAt})
	}
	return out, cur.Err()
}
