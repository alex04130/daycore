package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type credentialDoc struct {
	UserID       string    `bson:"_id"`
	PasswordHash string    `bson:"password_hash"`
	CreatedAt    time.Time `bson:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at"`
}

type oauthDoc struct {
	ID             string    `bson:"_id"`
	UserID         string    `bson:"user_id"`
	Provider       string    `bson:"provider"`
	ProviderUserID string    `bson:"provider_user_id"`
	CreatedAt      time.Time `bson:"created_at"`
}

type authRepo struct{ *Store }

func (r authRepo) GetCredentialByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	var d credentialDoc
	if err := r.c("credentials").FindOne(ctx, bson.M{"_id": userID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.Credential{UserID: d.UserID, PasswordHash: d.PasswordHash, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}, nil
}

func (r authRepo) UpsertCredential(ctx context.Context, c *domain.Credential) error {
	now := time.Now().UTC()
	_, err := r.c("credentials").UpdateOne(ctx, bson.M{"_id": c.UserID}, bson.M{
		"$set":         bson.M{"password_hash": c.PasswordHash, "updated_at": now},
		"$setOnInsert": bson.M{"created_at": now},
	}, options.Update().SetUpsert(true))
	return err
}

func (r authRepo) GetOAuthIdentity(ctx context.Context, provider, providerUserID string) (*domain.OAuthIdentity, error) {
	var d oauthDoc
	if err := r.c("oauth_identities").FindOne(ctx, bson.M{"provider": provider, "provider_user_id": providerUserID}).Decode(&d); err != nil {
		if notFound(err) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.OAuthIdentity{ID: d.ID, UserID: d.UserID, Provider: d.Provider, ProviderUserID: d.ProviderUserID, CreatedAt: d.CreatedAt}, nil
}

func (r authRepo) CreateOAuthIdentity(ctx context.Context, oi *domain.OAuthIdentity) error {
	if oi.ID == "" {
		oi.ID = uuid.NewString()
	}
	oi.CreatedAt = time.Now().UTC()
	_, err := r.c("oauth_identities").InsertOne(ctx, oauthDoc{
		ID: oi.ID, UserID: oi.UserID, Provider: oi.Provider, ProviderUserID: oi.ProviderUserID, CreatedAt: oi.CreatedAt,
	})
	return err
}
