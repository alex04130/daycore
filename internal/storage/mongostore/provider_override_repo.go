package mongostore

import (
	"context"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// _id is "<kind>:<id>", the same composite the SQL side spells as a two-column
// primary key. Composing it into _id rather than adding a unique index is what
// makes the upsert below correct with no second index to keep in step — the
// same reasoning settingDoc uses for its bare key.
//
// The separator is ':' and neither half may contain it; the loader rejects such
// ids, because a kind/id pair that can be spelled two ways is a row that can be
// written twice and read once.
type providerOverrideDoc struct {
	Key         string            `bson:"_id"`
	Kind        string            `bson:"kind"`
	ID          string            `bson:"provider_id"`
	Enabled     *bool             `bson:"enabled,omitempty"`
	BaseURL     string            `bson:"base_url,omitempty"`
	Description map[string]string `bson:"description,omitempty"`
	DescHash    string            `bson:"description_hash,omitempty"`
	Approved    bool              `bson:"approved"`
	UpdatedAt   int64             `bson:"updated_at"`
}

func providerOverrideKey(kind, id string) string { return kind + ":" + id }

type providerOverrideRepo struct{ *Store }

func (r providerOverrideRepo) All(ctx context.Context) ([]domain.ProviderOverride, error) {
	cur, err := r.c("provider_overrides").Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.ProviderOverride{}
	for cur.Next(ctx) {
		var d providerOverrideDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.ProviderOverride{
			Kind: d.Kind, ID: d.ID, Enabled: d.Enabled, BaseURL: d.BaseURL,
			Description: d.Description, DescriptionHash: d.DescHash,
			Approved: d.Approved, UpdatedAt: fromMillis(d.UpdatedAt),
		})
	}
	return out, cur.Err()
}

func (r providerOverrideRepo) Set(ctx context.Context, o domain.ProviderOverride) error {
	if o.Kind == "" || o.ID == "" {
		return domain.ErrMissingUpsertKey
	}
	// $set every field rather than replacing the document: an override written
	// by an older build that carried a field this one does not know about stays
	// intact. The SQL side gets this for free from its column list.
	set := bson.M{
		"kind": o.Kind, "provider_id": o.ID,
		"approved": o.Approved, "updated_at": nowMillis(),
		"description_hash": o.DescriptionHash,
		"base_url":         o.BaseURL,
	}
	unset := bson.M{}
	// Absent rather than null for the two optional fields: All decodes a missing
	// key as the zero value, and for *bool that is nil — "no opinion" — which is
	// exactly the distinction Enabled exists to carry. Writing an explicit null
	// would decode the same way today and stop doing so the moment anybody adds
	// a projection.
	if o.Enabled != nil {
		set["enabled"] = *o.Enabled
	} else {
		unset["enabled"] = ""
	}
	if len(o.Description) > 0 {
		set["description"] = o.Description
	} else {
		unset["description"] = ""
	}
	update := bson.M{"$set": set}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	_, err := r.c("provider_overrides").UpdateOne(ctx,
		bson.M{"_id": providerOverrideKey(o.Kind, o.ID)}, update, options.Update().SetUpsert(true))
	return err
}

func (r providerOverrideRepo) Delete(ctx context.Context, kind, id string) error {
	_, err := r.c("provider_overrides").DeleteOne(ctx, bson.M{"_id": providerOverrideKey(kind, id)})
	return err
}
