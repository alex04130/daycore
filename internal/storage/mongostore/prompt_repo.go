package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type promptDoc struct {
	ID        string    `bson:"_id"` // "<key>:<locale>"
	Key       string    `bson:"prompt_key"`
	Locale    string    `bson:"locale"`
	Content   string    `bson:"content"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type promptRepo struct{ *Store }

func (r promptRepo) Get(ctx context.Context, key, locale string) (*domain.Prompt, error) {
	var d promptDoc
	if err := r.c("prompt_overrides").FindOne(ctx, bson.M{"prompt_key": key, "locale": locale}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.Prompt{Key: d.Key, Locale: d.Locale, Content: d.Content, UpdatedAt: d.UpdatedAt}, nil
}

func (r promptRepo) Set(ctx context.Context, key, locale, content string) error {
	_, err := r.c("prompt_overrides").UpdateOne(ctx, bson.M{"prompt_key": key, "locale": locale}, bson.M{
		"$set":         bson.M{"content": content, "updated_at": time.Now().UTC()},
		"$setOnInsert": bson.M{"_id": key + ":" + locale, "prompt_key": key, "locale": locale},
	}, options.Update().SetUpsert(true))
	return err
}

func (r promptRepo) List(ctx context.Context) ([]domain.Prompt, error) {
	cur, err := r.c("prompt_overrides").Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "prompt_key", Value: 1}, {Key: "locale", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Prompt{}
	for cur.Next(ctx) {
		var d promptDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.Prompt{Key: d.Key, Locale: d.Locale, Content: d.Content, UpdatedAt: d.UpdatedAt})
	}
	return out, cur.Err()
}
