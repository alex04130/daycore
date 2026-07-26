package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type themeRepo struct{ *Store }

const themeSelect = `SELECT id, session_id, name, base, dark, variables, created_at, updated_at
	FROM custom_themes`

func (r themeRepo) Get(ctx context.Context, sessionID, id string) (*domain.CustomTheme, error) {
	row := r.queryRow(ctx, themeSelect+` WHERE session_id = ? AND id = ?`, sessionID, id)
	return scanTheme(row.Scan)
}

func (r themeRepo) List(ctx context.Context, sessionID string) ([]domain.CustomTheme, error) {
	rows, err := r.query(ctx, themeSelect+` WHERE session_id = ? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.CustomTheme{}
	for rows.Next() {
		t, err := scanTheme(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r themeRepo) Create(ctx context.Context, t *domain.CustomTheme) (*domain.CustomTheme, error) {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	if t.Variables == nil {
		t.Variables = map[string]string{}
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO custom_themes (id, session_id, name, base, dark, variables, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.SessionID, t.Name, t.Base, boolToInt(t.Dark), marshalJSON(t.Variables), now, now)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, t.SessionID, t.ID)
}

func (r themeRepo) Update(ctx context.Context, sessionID, id string, upd domain.CustomThemeUpdate) (*domain.CustomTheme, error) {
	set := []string{}
	args := []any{}
	if upd.Name != nil {
		set = append(set, "name = ?")
		args = append(args, *upd.Name)
	}
	if upd.Dark != nil {
		set = append(set, "dark = ?")
		args = append(args, boolToInt(*upd.Dark))
	}
	if upd.Variables != nil {
		set = append(set, "variables = ?")
		args = append(args, marshalJSON(*upd.Variables))
	}
	if len(set) == 0 {
		return r.Get(ctx, sessionID, id)
	}
	set = append(set, "updated_at = ?")
	args = append(args, nowMillis(), sessionID, id)
	res, err := r.exec(ctx,
		"UPDATE custom_themes SET "+joinComma(set)+" WHERE session_id = ? AND id = ?", args...)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, err := r.Get(ctx, sessionID, id); err != nil {
			return nil, err
		}
	}
	return r.Get(ctx, sessionID, id)
}

func (r themeRepo) Delete(ctx context.Context, sessionID, id string) error {
	res, err := r.exec(ctx, `DELETE FROM custom_themes WHERE session_id = ? AND id = ?`, sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanTheme(scan func(dest ...any) error) (*domain.CustomTheme, error) {
	var (
		t         domain.CustomTheme
		dark      int
		variables string
		createdAt int64
		updatedAt int64
	)
	err := scan(&t.ID, &t.SessionID, &t.Name, &t.Base, &dark, &variables, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Dark = dark != 0
	if e := json.Unmarshal([]byte(variables), &t.Variables); e != nil || t.Variables == nil {
		t.Variables = map[string]string{}
	}
	t.CreatedAt = fromMillis(createdAt)
	t.UpdatedAt = fromMillis(updatedAt)
	return &t, nil
}
