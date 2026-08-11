package mongostore

import (
	"context"
	"errors"
	"time"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type familyDoc struct {
	ID            string             `bson:"_id"`
	DisplayName   string             `bson:"display_name,omitempty"`
	Tokens        []domain.TokenSpec `bson:"tokens"`
	Rules         string             `bson:"rules,omitempty"`
	RulesAccepted bool               `bson:"rules_accepted"`
	Pinned        bool               `bson:"pinned"`
	CreatedAt     int64              `bson:"created_at"`
	UpdatedAt     int64              `bson:"updated_at"`
}

func (d familyDoc) toDomain() domain.FrontendFamily {
	tokens := d.Tokens
	if tokens == nil {
		tokens = []domain.TokenSpec{}
	}
	return domain.FrontendFamily{
		ID: d.ID, DisplayName: d.DisplayName, Tokens: tokens,
		Rules: d.Rules, RulesAccepted: d.RulesAccepted, Pinned: d.Pinned,
		CreatedAt: fromMillis(d.CreatedAt), UpdatedAt: fromMillis(d.UpdatedAt),
	}
}

type buildDoc struct {
	BuildHash   string `bson:"_id"`
	FamilyID    string `bson:"family_id"`
	DisplayName string `bson:"display_name,omitempty"`
	Version     string `bson:"build_version,omitempty"`
	MinAPI      int    `bson:"min_api,omitempty"`
	FirstSeenAt int64  `bson:"first_seen_at"`
	LastSeenAt  int64  `bson:"last_seen_at"`
}

type frontendRepo struct{ *Store }

func (r frontendRepo) ListFamilies(ctx context.Context) ([]domain.FrontendFamily, error) {
	cur, err := r.c("frontend_families").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.FrontendFamily{}
	for cur.Next(ctx) {
		var d familyDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, d.toDomain())
	}
	return out, cur.Err()
}

func (r frontendRepo) GetFamily(ctx context.Context, id string) (*domain.FrontendFamily, error) {
	if id == "" {
		return nil, domain.ErrNotFound
	}
	var d familyDoc
	err := r.c("frontend_families").FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	f := d.toDomain()
	return &f, nil
}

func (r frontendRepo) UpsertFamily(ctx context.Context, f domain.FrontendFamily) error {
	if f.ID == "" {
		return domain.ErrMissingUpsertKey
	}
	tokens := f.Tokens
	if tokens == nil {
		tokens = []domain.TokenSpec{}
	}
	now := nowMillis()
	_, err := r.c("frontend_families").UpdateOne(ctx, bson.M{"_id": f.ID}, bson.M{
		"$set": bson.M{
			"display_name": f.DisplayName, "tokens": tokens, "rules": f.Rules,
			"rules_accepted": f.RulesAccepted, "pinned": f.Pinned, "updated_at": now,
		},
		"$setOnInsert": bson.M{"created_at": now},
	}, options.Update().SetUpsert(true))
	return err
}

func (r frontendRepo) DeleteFamily(ctx context.Context, id string) error {
	_, err := r.c("frontend_families").DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r frontendRepo) GetBuild(ctx context.Context, hash string) (*domain.FrontendBuild, error) {
	if hash == "" {
		return nil, domain.ErrNotFound
	}
	var d buildDoc
	err := r.c("frontend_builds").FindOne(ctx, bson.M{"_id": hash}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &domain.FrontendBuild{
		BuildHash: d.BuildHash, FamilyID: d.FamilyID, DisplayName: d.DisplayName,
		Version: d.Version, MinAPI: d.MinAPI,
		FirstSeenAt: fromMillis(d.FirstSeenAt), LastSeenAt: fromMillis(d.LastSeenAt),
	}, nil
}

func (r frontendRepo) ListBuilds(ctx context.Context) ([]domain.FrontendBuild, error) {
	cur, err := r.c("frontend_builds").Find(ctx, bson.M{},
		options.Find().SetSort(bson.D{{Key: "family_id", Value: 1}, {Key: "first_seen_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.FrontendBuild{}
	for cur.Next(ctx) {
		var d buildDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, domain.FrontendBuild{
			BuildHash: d.BuildHash, FamilyID: d.FamilyID, DisplayName: d.DisplayName,
			Version: d.Version, MinAPI: d.MinAPI,
			FirstSeenAt: fromMillis(d.FirstSeenAt), LastSeenAt: fromMillis(d.LastSeenAt),
		})
	}
	return out, cur.Err()
}

// SeenBuild records a handshake.
//
// ⚠️ family_id is in $setOnInsert, NOT $set. An operator who moved a build into
// another family would otherwise have that undone by the build's next startup,
// silently, with the themes following it.
//
// The last-seen write is conditional on staleness, so a frontend handshaking on
// every page load does not turn that into a write every time. Two updates
// rather than one because the conditional filter cannot also create the row.
func (r frontendRepo) SeenBuild(ctx context.Context, b domain.FrontendBuild, staleBefore time.Time) error {
	if b.BuildHash == "" || b.FamilyID == "" {
		return domain.ErrMissingUpsertKey
	}
	now := nowMillis()
	res, err := r.c("frontend_builds").UpdateOne(ctx,
		bson.M{"_id": b.BuildHash, "last_seen_at": bson.M{"$lte": staleBefore.UnixMilli()}},
		bson.M{"$set": bson.M{
			"display_name": b.DisplayName, "build_version": b.Version,
			"min_api": b.MinAPI, "last_seen_at": now,
		}})
	if err != nil {
		return err
	}
	if res.MatchedCount > 0 {
		return nil
	}
	// Not stale, or not there. Upsert-on-insert-only settles both without a
	// read: an existing fresh row matches and nothing in $setOnInsert applies.
	_, err = r.c("frontend_builds").UpdateOne(ctx, bson.M{"_id": b.BuildHash}, bson.M{
		"$setOnInsert": bson.M{
			"family_id": b.FamilyID, "display_name": b.DisplayName,
			"build_version": b.Version, "min_api": b.MinAPI,
			"first_seen_at": now, "last_seen_at": now,
		},
	}, options.Update().SetUpsert(true))
	return err
}

func (r frontendRepo) SetBuildFamily(ctx context.Context, buildHash, familyID string) error {
	res, err := r.c("frontend_builds").UpdateOne(ctx, bson.M{"_id": buildHash},
		bson.M{"$set": bson.M{"family_id": familyID}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}
