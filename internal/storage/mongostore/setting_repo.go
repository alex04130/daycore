package mongostore

import (
	"context"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// _id is the setting key: it is unique by nature, and using it means the upsert
// below needs no second index to be correct.
type settingDoc struct {
	Key       string `bson:"_id"`
	Value     string `bson:"value"`
	UpdatedAt int64  `bson:"updated_at"`
}

type settingRepo struct{ *Store }

func (r settingRepo) All(ctx context.Context) ([]domain.Setting, error) {
	cur, err := r.c("settings").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.Setting{}
	for cur.Next(ctx) {
		var d settingDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.Setting{Key: d.Key, Value: d.Value, UpdatedAt: fromMillis(d.UpdatedAt)})
	}
	return out, cur.Err()
}

func (r settingRepo) Set(ctx context.Context, key, value string) error {
	if key == "" {
		return domain.ErrMissingUpsertKey
	}
	_, err := r.c("settings").UpdateOne(ctx,
		bson.M{"_id": key},
		bson.M{"$set": bson.M{"value": value, "updated_at": nowMillis()}},
		options.Update().SetUpsert(true))
	return err
}

func (r settingRepo) Delete(ctx context.Context, key string) error {
	_, err := r.c("settings").DeleteOne(ctx, bson.M{"_id": key})
	return err
}
