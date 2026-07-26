package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type wishRepo struct{ *Store }

const wishSelect = `SELECT id, session_id, title, note, effort_min, status, created_at, updated_at FROM wishes`

func (r wishRepo) List(ctx interface{}, sid, status string) ([]domain.Wish, error) {
	c := ctx.(context.Context)
	query := wishSelect + ` WHERE session_id = ?`
	args := []any{sid}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := r.query(c, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Wish{}
	for rows.Next() {
		w, err := scanWish(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func (r wishRepo) Get(ctx interface{}, sid, id string) (*domain.Wish, error) {
	c := ctx.(context.Context)
	row := r.queryRow(c, wishSelect+` WHERE session_id = ? AND id = ?`, sid, id)
	return scanWish(row.Scan)
}

func (r wishRepo) Create(ctx interface{}, w *domain.Wish) (*domain.Wish, error) {
	c := ctx.(context.Context)
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.exec(c,
		`INSERT INTO wishes (id, session_id, title, note, effort_min, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.SessionID, w.Title, w.Note, w.EffortMin, w.Status, now, now)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, w.SessionID, w.ID)
}

func (r wishRepo) Update(ctx interface{}, sid, id string, w *domain.Wish) (*domain.Wish, error) {
	c := ctx.(context.Context)
	now := nowMillis()
	_, err := r.exec(c,
		`UPDATE wishes SET title = ?, note = ?, effort_min = ?, status = ?, updated_at = ?
		 WHERE session_id = ? AND id = ?`,
		w.Title, w.Note, w.EffortMin, w.Status, now, sid, id)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, sid, id)
}

func (r wishRepo) Delete(ctx interface{}, sid, id string) error {
	c := ctx.(context.Context)
	res, err := r.exec(c, `DELETE FROM wishes WHERE session_id = ? AND id = ?`, sid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanWish(scan func(dest ...any) error) (*domain.Wish, error) {
	var (
		w         domain.Wish
		createdAt int64
		updatedAt int64
	)
	err := scan(&w.ID, &w.SessionID, &w.Title, &w.Note, &w.EffortMin, &w.Status, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	w.CreatedAt = fromMillis(createdAt)
	w.UpdatedAt = fromMillis(updatedAt)
	return &w, nil
}
