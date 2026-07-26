package mongostore

import (
	"context"
	"regexp"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type materialDoc struct {
	ID         string    `bson:"_id"`
	SessionID  string    `bson:"session_id"`
	Category   string    `bson:"category"`
	Title      string    `bson:"title"`
	Summary    string    `bson:"summary"`
	Body       string    `bson:"body"`
	Source     string    `bson:"source"`
	MimeType   string    `bson:"mime_type"`
	StorageRef string    `bson:"storage_ref"`
	Tags       []string  `bson:"tags"`
	CreatedAt  time.Time `bson:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at"`
}

func (d materialDoc) toDomain() *domain.Material {
	tags := d.Tags
	if tags == nil {
		tags = []string{}
	}
	return &domain.Material{
		ID: d.ID, SessionID: d.SessionID, Category: d.Category,
		Title: d.Title, Summary: d.Summary, Body: d.Body,
		Source: d.Source, MimeType: d.MimeType, StorageRef: d.StorageRef,
		Tags: tags, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type materialRepo struct{ *Store }

func (r materialRepo) List(ctx interface{}, sessionID, category, query string, limit, offset int) ([]domain.Material, error) {
	c := ctx.(context.Context)
	filter := bson.M{"session_id": sessionID}
	if category != "" {
		filter["category"] = category
	}
	if query != "" {
		// Escape the user input so it's matched literally — a raw pattern like
		// "(a+)+$" would cause catastrophic backtracking (ReDoS).
		q := regexp.QuoteMeta(query)
		filter["$or"] = []bson.M{
			{"title": bson.M{"$regex": q, "$options": "i"}},
			{"summary": bson.M{"$regex": q, "$options": "i"}},
			{"body": bson.M{"$regex": q, "$options": "i"}},
		}
	}
	opts := options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	if offset > 0 {
		opts.SetSkip(int64(offset))
	}
	cur, err := r.c("materials").Find(c, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(c)
	out := []domain.Material{}
	for cur.Next(c) {
		var d materialDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}

func (r materialRepo) Get(ctx interface{}, sessionID, id string) (*domain.Material, error) {
	var d materialDoc
	if err := r.c("materials").FindOne(ctx.(context.Context), bson.M{"session_id": sessionID, "_id": id}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r materialRepo) Create(ctx interface{}, m *domain.Material) (*domain.Material, error) {
	c := ctx.(context.Context)
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now
	_, err := r.c("materials").InsertOne(c, materialDoc{
		ID: m.ID, SessionID: m.SessionID, Category: m.Category,
		Title: m.Title, Summary: m.Summary, Body: m.Body,
		Source: m.Source, MimeType: m.MimeType, StorageRef: m.StorageRef,
		Tags: m.Tags, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r materialRepo) Update(ctx interface{}, sessionID, id string, m *domain.Material) (*domain.Material, error) {
	c := ctx.(context.Context)
	if m.Tags == nil {
		m.Tags = []string{}
	}
	now := time.Now().UTC()
	res, err := r.c("materials").UpdateOne(c,
		bson.M{"session_id": sessionID, "_id": id},
		bson.M{"$set": bson.M{
			"category":    m.Category,
			"title":       m.Title,
			"summary":     m.Summary,
			"body":        m.Body,
			"source":      m.Source,
			"mime_type":   m.MimeType,
			"storage_ref": m.StorageRef,
			"tags":        m.Tags,
			"updated_at":  now,
		}},
	)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, domain.ErrNotFound
	}
	return r.Get(ctx, sessionID, id)
}

func (r materialRepo) Delete(ctx interface{}, sessionID, id string) error {
	res, err := r.c("materials").DeleteOne(ctx.(context.Context), bson.M{"session_id": sessionID, "_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}
