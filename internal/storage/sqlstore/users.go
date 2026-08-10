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
		`SELECT id, email, name, avatar_url, data_session_id, token_version, is_owner, created_at, updated_at FROM users WHERE id = ?`, id)
	return scanUser(row.Scan)
}

func (r userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.queryRow(ctx,
		`SELECT id, email, name, avatar_url, data_session_id, token_version, is_owner, created_at, updated_at FROM users WHERE email = ?`, email)
	return scanUser(row.Scan)
}

func (r userRepo) SetDataSession(ctx context.Context, userID, sessionID string) error {
	_, err := r.exec(ctx, `UPDATE users SET data_session_id = ? WHERE id = ? AND (data_session_id = '' OR data_session_id = ?)`, sessionID, userID, sessionID)
	return err
}

// SetOwner is a NARROW setter, and the narrowness is the point.
//
// Upsert takes a whole *domain.User, and the OAuth callback builds one out of
// what the provider returned before calling it. So the moment is_owner appears
// in Upsert's column list, logging in through Google becomes a write that can
// set the super-administrator flag from data this deployment does not control.
// TokenVersion and DataSessionID are protected by the same arrangement; unlike
// them, this one has a conformance case across all four back ends, because a
// convention nothing tests is a convention somebody tidies away.
func (r userRepo) SetOwner(ctx context.Context, userID string, owner bool) error {
	v := 0
	if owner {
		v = 1
	}
	_, err := r.exec(ctx, `UPDATE users SET is_owner = ?, updated_at = ? WHERE id = ?`, v, nowMillis(), userID)
	return err
}

// List returns users newest first, capped.
//
// Newest first because the console's question is usually about somebody who
// just signed up or was just changed. Capped because this table grows without
// bound and a console that tries to render all of it stops being usable exactly
// when the deployment gets interesting.
func (r userRepo) List(ctx context.Context, limit int) ([]domain.User, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.query(ctx, `SELECT id, email, name, avatar_url, data_session_id, token_version, is_owner, created_at, updated_at
		FROM users ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.User{}
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
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
		// The three dialects store this as INTEGER / INTEGER / TINYINT(1) — the
		// house style for a boolean here, set by reminders_off. Scanning into an
		// int and converting is what keeps one scan function working on all
		// three; scanning into a bool works on some drivers and not others.
		owner int
	)
	err := scan(&u.ID, &email, &name, &avatar, &u.DataSessionID, &u.TokenVersion, &owner, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Email = ptrString(email)
	u.Name = ptrString(name)
	u.AvatarURL = ptrString(avatar)
	u.IsOwner = owner != 0
	u.CreatedAt = fromMillis(createdAt)
	u.UpdatedAt = fromMillis(updatedAt)
	return &u, nil
}
