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
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
}

func (d userDoc) toDomain() *domain.User {
	return &domain.User{ID: d.ID, Email: d.Email, Name: d.Name, AvatarURL: d.AvatarURL, DataSessionID: d.DataSessionID, TokenVersion: d.TokenVersion, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
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
	_, err := r.c("users").UpdateOne(ctx, bson.M{"_id": u.ID}, bson.M{
		"$set":         bson.M{"email": u.Email, "name": u.Name, "avatar_url": u.AvatarURL, "updated_at": now},
		"$setOnInsert": bson.M{"created_at": now},
	}, options.Update().SetUpsert(true))
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, u.ID)
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
