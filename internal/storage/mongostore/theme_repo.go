package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type themeDoc struct {
	ID        string            `bson:"_id"`
	SessionID string            `bson:"session_id"`
	FamilyID  string            `bson:"family_id"`
	Name      string            `bson:"name"`
	Base      string            `bson:"base"`
	Dark      bool              `bson:"dark"`
	Variables map[string]string `bson:"variables"`
	CreatedAt time.Time         `bson:"created_at"`
	UpdatedAt time.Time         `bson:"updated_at"`
}

func (d themeDoc) toDomain() *domain.CustomTheme {
	vars := d.Variables
	if vars == nil {
		vars = map[string]string{}
	}
	return &domain.CustomTheme{
		ID: d.ID, SessionID: d.SessionID, FamilyID: d.FamilyID, Name: d.Name, Base: d.Base, Dark: d.Dark,
		Variables: vars, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

type themeRepo struct{ *Store }

func (r themeRepo) Get(ctx context.Context, sessionID, id string) (*domain.CustomTheme, error) {
	var d themeDoc
	if err := r.c("custom_themes").FindOne(ctx, bson.M{"_id": id, "session_id": sessionID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r themeRepo) List(ctx context.Context, sessionID, familyID string) ([]domain.CustomTheme, error) {
	if familyID == "" {
		familyID = domain.FallbackFamilyID
	}
	cur, err := r.c("custom_themes").Find(ctx, bson.M{"session_id": sessionID, "family_id": familyID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.CustomTheme{}
	for cur.Next(ctx) {
		var d themeDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}

func (r themeRepo) ListAcrossFamilies(ctx context.Context, sessionID string) ([]domain.CustomTheme, error) {
	cur, err := r.c("custom_themes").Find(ctx, bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.CustomTheme{}
	for cur.Next(ctx) {
		var d themeDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}

func (r themeRepo) Create(ctx context.Context, t *domain.CustomTheme) (*domain.CustomTheme, error) {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Variables == nil {
		t.Variables = map[string]string{}
	}
	// Same reason as the SQL side: a theme with no family is invisible to every
	// List, and "the theme I just made is not in the list" is the most
	// confusing possible symptom.
	if t.FamilyID == "" {
		t.FamilyID = domain.FallbackFamilyID
	}
	now := time.Now().UTC()
	t.CreatedAt, t.UpdatedAt = now, now
	if _, err := r.c("custom_themes").InsertOne(ctx, themeDoc{
		ID: t.ID, SessionID: t.SessionID, FamilyID: t.FamilyID, Name: t.Name, Base: t.Base, Dark: t.Dark,
		Variables: t.Variables, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}); err != nil {
		return nil, err
	}
	return r.Get(ctx, t.SessionID, t.ID)
}

func (r themeRepo) Update(ctx context.Context, sessionID, id string, upd domain.CustomThemeUpdate) (*domain.CustomTheme, error) {
	set := bson.M{"updated_at": time.Now().UTC()}
	if upd.Name != nil {
		set["name"] = *upd.Name
	}
	if upd.Dark != nil {
		set["dark"] = *upd.Dark
	}
	if upd.Variables != nil {
		set["variables"] = *upd.Variables
	}
	res, err := r.c("custom_themes").UpdateOne(ctx, bson.M{"_id": id, "session_id": sessionID}, bson.M{"$set": set})
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, domain.ErrNotFound
	}
	return r.Get(ctx, sessionID, id)
}

func (r themeRepo) Delete(ctx context.Context, sessionID, id string) error {
	res, err := r.c("custom_themes").DeleteOne(ctx, bson.M{"_id": id, "session_id": sessionID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}
