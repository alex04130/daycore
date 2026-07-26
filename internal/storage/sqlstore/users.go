package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type userRepo struct{ *Store }

func (r userRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	row := r.queryRow(ctx,
		`SELECT id, email, name, avatar_url, data_session_id, token_version, created_at, updated_at FROM users WHERE id = ?`, id)
	return scanUser(row.Scan)
}

func (r userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.queryRow(ctx,
		`SELECT id, email, name, avatar_url, data_session_id, token_version, created_at, updated_at FROM users WHERE email = ?`, email)
	return scanUser(row.Scan)
}

func (r userRepo) SetDataSession(ctx context.Context, userID, sessionID string) error {
	_, err := r.exec(ctx, `UPDATE users SET data_session_id = ? WHERE id = ? AND (data_session_id = '' OR data_session_id = ?)`, sessionID, userID, sessionID)
	return err
}

func (r userRepo) IncrementTokenVersion(ctx context.Context, userID string) error {
	_, err := r.exec(ctx, `UPDATE users SET token_version = token_version + 1, updated_at = ? WHERE id = ?`, nowMillis(), userID)
	return err
}

func (r userRepo) Upsert(ctx context.Context, u *domain.User) (*domain.User, error) {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE users SET email = ?, name = ?, avatar_url = ?, updated_at = ? WHERE id = ?`,
		nullString(u.Email), nullString(u.Name), nullString(u.AvatarURL), now, u.ID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = r.exec(ctx,
			`INSERT INTO users (id, email, name, avatar_url, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			u.ID, nullString(u.Email), nullString(u.Name), nullString(u.AvatarURL), now, now)
		if err != nil {
			return nil, err
		}
	}
	return r.GetByID(ctx, u.ID)
}

func scanUser(scan func(dest ...any) error) (*domain.User, error) {
	var (
		u         domain.User
		email     sql.NullString
		name      sql.NullString
		avatar    sql.NullString
		createdAt int64
		updatedAt int64
	)
	err := scan(&u.ID, &email, &name, &avatar, &u.DataSessionID, &u.TokenVersion, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Email = ptrString(email)
	u.Name = ptrString(name)
	u.AvatarURL = ptrString(avatar)
	u.CreatedAt = fromMillis(createdAt)
	u.UpdatedAt = fromMillis(updatedAt)
	return &u, nil
}
