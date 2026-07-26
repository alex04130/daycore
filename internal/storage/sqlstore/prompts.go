package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"
)

type promptRepo struct{ *Store }

func (r promptRepo) Get(ctx context.Context, key, locale string) (*domain.Prompt, error) {
	row := r.queryRow(ctx,
		`SELECT prompt_key, locale, content, updated_at FROM prompt_overrides
		 WHERE prompt_key = ? AND locale = ?`, key, locale)
	var (
		p         domain.Prompt
		updatedAt int64
	)
	err := row.Scan(&p.Key, &p.Locale, &p.Content, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.UpdatedAt = fromMillis(updatedAt)
	return &p, nil
}

func (r promptRepo) Set(ctx context.Context, key, locale, content string) error {
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE prompt_overrides SET content = ?, updated_at = ? WHERE prompt_key = ? AND locale = ?`,
		content, now, key, locale)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = r.exec(ctx,
			`INSERT INTO prompt_overrides (prompt_key, locale, content, updated_at) VALUES (?, ?, ?, ?)`,
			key, locale, content, now)
	}
	return err
}

func (r promptRepo) List(ctx context.Context) ([]domain.Prompt, error) {
	rows, err := r.query(ctx,
		`SELECT prompt_key, locale, content, updated_at FROM prompt_overrides ORDER BY prompt_key, locale`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Prompt{}
	for rows.Next() {
		var (
			p         domain.Prompt
			updatedAt int64
		)
		if err := rows.Scan(&p.Key, &p.Locale, &p.Content, &updatedAt); err != nil {
			return nil, err
		}
		p.UpdatedAt = fromMillis(updatedAt)
		out = append(out, p)
	}
	return out, rows.Err()
}
