package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type authRepo struct{ *Store }

func (r authRepo) GetCredentialByUserID(ctx context.Context, userID string) (*domain.Credential, error) {
	row := r.queryRow(ctx,
		`SELECT user_id, password_hash, created_at, updated_at FROM credentials WHERE user_id = ?`, userID)
	var (
		c         domain.Credential
		createdAt int64
		updatedAt int64
	)
	err := row.Scan(&c.UserID, &c.PasswordHash, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.CreatedAt = fromMillis(createdAt)
	c.UpdatedAt = fromMillis(updatedAt)
	return &c, nil
}

func (r authRepo) UpsertCredential(ctx context.Context, c *domain.Credential) error {
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE credentials SET password_hash = ?, updated_at = ? WHERE user_id = ?`,
		c.PasswordHash, now, c.UserID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = r.exec(ctx,
			`INSERT INTO credentials (user_id, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			c.UserID, c.PasswordHash, now, now)
	}
	return err
}

func (r authRepo) GetOAuthIdentity(ctx context.Context, provider, providerUserID string) (*domain.OAuthIdentity, error) {
	row := r.queryRow(ctx,
		`SELECT id, user_id, provider, provider_user_id, created_at
		 FROM oauth_identities WHERE provider = ? AND provider_user_id = ?`, provider, providerUserID)
	var (
		oi        domain.OAuthIdentity
		createdAt int64
	)
	err := row.Scan(&oi.ID, &oi.UserID, &oi.Provider, &oi.ProviderUserID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	oi.CreatedAt = fromMillis(createdAt)
	return &oi, nil
}

func (r authRepo) CreateOAuthIdentity(ctx context.Context, oi *domain.OAuthIdentity) error {
	if oi.ID == "" {
		oi.ID = uuid.NewString()
	}
	_, err := r.exec(ctx,
		`INSERT INTO oauth_identities (id, user_id, provider, provider_user_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		oi.ID, oi.UserID, oi.Provider, oi.ProviderUserID, nowMillis())
	return err
}
