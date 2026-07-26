package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type wishDoc struct {
	ID        string    `bson:"_id"`
	SessionID string    `bson:"session_id"`
	Title     string    `bson:"title"`
	Note      string    `bson:"note"`
	EffortMin int       `bson:"effort_min"`
	Status    string    `bson:"status"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func (d wishDoc) toDomain() *domain.Wish {
	return &domain.Wish{
		ID:        d.ID,
		SessionID: d.SessionID,
		Title:     d.Title,
		Note:      d.Note,
		EffortMin: d.EffortMin,
		Status:    d.Status,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
}

type wishRepo struct{ *Store }

func (r wishRepo) List(ctx interface{}, sid, status string) ([]domain.Wish, error) {
	c := ctx.(context.Context)
	filter := bson.M{"session_id": sid}
	if status != "" {
		filter["status"] = status
	}
	cur, err := r.c("wishes").Find(c, filter,
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(c)
	out := []domain.Wish{}
	for cur.Next(c) {
		var d wishDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}

func (r wishRepo) Get(ctx interface{}, sid, id string) (*domain.Wish, error) {
	c := ctx.(context.Context)
	var d wishDoc
	if err := r.c("wishes").FindOne(c, bson.M{"_id": id, "session_id": sid}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r wishRepo) Create(ctx interface{}, w *domain.Wish) (*domain.Wish, error) {
	c := ctx.(context.Context)
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	w.CreatedAt, w.UpdatedAt = now, now
	if _, err := r.c("wishes").InsertOne(c, wishDoc{
		ID:        w.ID,
		SessionID: w.SessionID,
		Title:     w.Title,
		Note:      w.Note,
		EffortMin: w.EffortMin,
		Status:    w.Status,
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}); err != nil {
		return nil, err
	}
	return r.Get(ctx, w.SessionID, w.ID)
}

func (r wishRepo) Update(ctx interface{}, sid, id string, w *domain.Wish) (*domain.Wish, error) {
	c := ctx.(context.Context)
	now := time.Now().UTC()
	set := bson.M{
		"title":      w.Title,
		"note":       w.Note,
		"effort_min": w.EffortMin,
		"status":     w.Status,
		"updated_at": now,
	}
	res, err := r.c("wishes").UpdateOne(c, bson.M{"_id": id, "session_id": sid}, bson.M{"$set": set})
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, domain.ErrNotFound
	}
	return r.Get(ctx, sid, id)
}

func (r wishRepo) Delete(ctx interface{}, sid, id string) error {
	c := ctx.(context.Context)
	res, err := r.c("wishes").DeleteOne(c, bson.M{"_id": id, "session_id": sid})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}
