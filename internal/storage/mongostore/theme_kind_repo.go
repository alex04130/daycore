package mongostore

import (
	"context"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type themeKindDoc struct {
	Name        string `bson:"_id"`
	Pattern     string `bson:"pattern"`
	Description string `bson:"description,omitempty"`
	Approved    bool   `bson:"approved"`
	ProposedBy  string `bson:"proposed_by,omitempty"`
	CreatedAt   int64  `bson:"created_at"`
	UpdatedAt   int64  `bson:"updated_at"`
}

func (d themeKindDoc) toDomain() domain.ThemeKind {
	return domain.ThemeKind{
		Name: d.Name, Pattern: d.Pattern, Description: d.Description,
		Approved: d.Approved, ProposedBy: d.ProposedBy,
		CreatedAt: fromMillis(d.CreatedAt), UpdatedAt: fromMillis(d.UpdatedAt),
	}
}

type themeKindRepo struct{ *Store }

func (r themeKindRepo) List(ctx context.Context) ([]domain.ThemeKind, error) {
	cur, err := r.c("theme_kinds").Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.ThemeKind{}
	for cur.Next(ctx) {
		var d themeKindDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, d.toDomain())
	}
	return out, cur.Err()
}

func (r themeKindRepo) Get(ctx context.Context, name string) (*domain.ThemeKind, error) {
	if name == "" {
		return nil, domain.ErrNotFound
	}
	var d themeKindDoc
	if err := r.c("theme_kinds").FindOne(ctx, bson.M{"_id": name}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	k := d.toDomain()
	return &k, nil
}

func (r themeKindRepo) Upsert(ctx context.Context, k domain.ThemeKind) error {
	if k.Name == "" {
		return domain.ErrMissingUpsertKey
	}
	now := nowMillis()
	_, err := r.c("theme_kinds").UpdateOne(ctx, bson.M{"_id": k.Name}, bson.M{
		"$set": bson.M{
			"pattern": k.Pattern, "description": k.Description,
			"approved": k.Approved, "proposed_by": k.ProposedBy, "updated_at": now,
		},
		"$setOnInsert": bson.M{"created_at": now},
	}, options.Update().SetUpsert(true))
	return err
}

func (r themeKindRepo) Delete(ctx context.Context, name string) error {
	_, err := r.c("theme_kinds").DeleteOne(ctx, bson.M{"_id": name})
	return err
}

func (r themeKindRepo) CountPending(ctx context.Context) (int, error) {
	n, err := r.c("theme_kinds").CountDocuments(ctx, bson.M{"approved": false})
	return int(n), err
}
