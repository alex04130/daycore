package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type userDoc struct {
	ID            string    `bson:"_id"`
	Email         *string   `bson:"email,omitempty"`
	Name          *string   `bson:"name,omitempty"`
	AvatarURL     *string   `bson:"avatar_url,omitempty"`
	DataSessionID string    `bson:"data_session_id,omitempty"`
	TokenVersion  int       `bson:"token_version,omitempty"`
	IsOwner       bool      `bson:"is_owner,omitempty"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
}

func (d userDoc) toDomain() *domain.User {
	return &domain.User{ID: d.ID, Email: d.Email, Name: d.Name, AvatarURL: d.AvatarURL, DataSessionID: d.DataSessionID, TokenVersion: d.TokenVersion, IsOwner: d.IsOwner, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

type userRepo struct{ *Store }

func (r userRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	return r.findUser(ctx, bson.M{"_id": id})
}

func (r userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.findUser(ctx, bson.M{"email": email})
}

func (r userRepo) findUser(ctx context.Context, filter bson.M) (*domain.User, error) {
	var d userDoc
	if err := r.c("users").FindOne(ctx, filter).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return d.toDomain(), nil
}

func (r userRepo) Upsert(ctx context.Context, u *domain.User) (*domain.User, error) {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	// ⚠️ An explicit field list, and is_owner is deliberately absent.
	//
	// Replacing the document (or spreading the struct) would let the OAuth
	// callback — which builds a *domain.User out of what the provider returned —
	// set the super-administrator flag. The SQL side gets the same protection
	// from its column list. A conformance case pins both.
	// ⚠️ A nil email must be UNSET, not written as null.
	//
	// The users.email index is unique+sparse, and sparse skips documents where
	// the field is ABSENT — not documents where it is explicitly null. Writing
	// `{"email": null}` therefore puts every account-less user in the index
	// under the same key, and the second one fails with E11000. SQL has no such
	// rule (many NULLs are fine in a unique index), so this breaks on exactly
	// one back end, and only for anonymous users — which is most of them.
	set := bson.M{"name": u.Name, "avatar_url": u.AvatarURL, "updated_at": now}
	update := bson.M{"$set": set, "$setOnInsert": bson.M{"created_at": now}}
	if u.Email != nil {
		set["email"] = u.Email
	} else {
		update["$unset"] = bson.M{"email": ""}
	}
	_, err := r.c("users").UpdateOne(ctx, bson.M{"_id": u.ID}, update, options.Update().SetUpsert(true))
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, u.ID)
}

// SetOwner is the narrow setter for the super-administrator mark. See the note
// on Upsert for what it is narrow against.
func (r userRepo) SetOwner(ctx context.Context, userID string, owner bool) error {
	_, err := r.c("users").UpdateOne(ctx, bson.M{"_id": userID},
		bson.M{"$set": bson.M{"is_owner": owner, "updated_at": time.Now().UTC()}})
	return err
}

// List returns users newest first, capped — see the SQL implementation for why
// both halves of that matter.
func (r userRepo) List(ctx context.Context, limit int) ([]domain.User, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	cur, err := r.c("users").Find(ctx, bson.M{}, options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := []domain.User{}
	for cur.Next(ctx) {
		var d userDoc
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toDomain())
	}
	return out, cur.Err()
}

func (r userRepo) SetDataSession(ctx context.Context, userID, sessionID string) error {
	filter := bson.M{"_id": userID, "$or": []bson.M{{"data_session_id": ""}, {"data_session_id": bson.M{"$exists": false}}, {"data_session_id": sessionID}}}
	_, err := r.c("users").UpdateOne(ctx, filter, bson.M{"$set": bson.M{"data_session_id": sessionID}})
	return err
}

func (r userRepo) IncrementTokenVersion(ctx context.Context, userID string) error {
	_, err := r.c("users").UpdateOne(ctx, bson.M{"_id": userID}, bson.M{
		"$inc": bson.M{"token_version": 1},
		"$set": bson.M{"updated_at": time.Now().UTC()},
	})
	return err
}
