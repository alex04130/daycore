package sqlstore

import (
	"context"
	"database/sql"

	"daycore/internal/domain"
)

type themeKindRepo struct{ *Store }

const themeKindCols = `name, pattern, description, approved, proposed_by, created_at, updated_at`

func scanThemeKind(s scanner) (domain.ThemeKind, error) {
	var k domain.ThemeKind
	var desc, proposedBy sql.NullString
	var approved int
	var created, updated int64
	if err := s.Scan(&k.Name, &k.Pattern, &desc, &approved, &proposedBy, &created, &updated); err != nil {
		return k, err
	}
	k.Description, k.ProposedBy = desc.String, proposedBy.String
	k.Approved = approved != 0
	k.CreatedAt, k.UpdatedAt = fromMillis(created), fromMillis(updated)
	return k, nil
}

func (r themeKindRepo) List(ctx context.Context) ([]domain.ThemeKind, error) {
	// Ordered by name so two deployments with the same rows produce the same
	// screen, and so the registry's merge order is deterministic.
	rows, err := r.query(ctx, `SELECT `+themeKindCols+` FROM theme_kinds ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ThemeKind{}
	for rows.Next() {
		k, err := scanThemeKind(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r themeKindRepo) Get(ctx context.Context, name string) (*domain.ThemeKind, error) {
	if name == "" {
		return nil, domain.ErrNotFound
	}
	rows, err := r.query(ctx, `SELECT `+themeKindCols+` FROM theme_kinds WHERE name = ?`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, domain.ErrNotFound
	}
	k, err := scanThemeKind(rows)
	if err != nil {
		return nil, err
	}
	return &k, rows.Err()
}

func (r themeKindRepo) Upsert(ctx context.Context, k domain.ThemeKind) error {
	if k.Name == "" {
		return domain.ErrMissingUpsertKey
	}
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE theme_kinds SET pattern = ?, description = ?, approved = ?, proposed_by = ?, updated_at = ?
		 WHERE name = ?`,
		k.Pattern, k.Description, boolToInt(k.Approved), k.ProposedBy, now, k.Name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.exec(ctx,
		`INSERT INTO theme_kinds (`+themeKindCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		k.Name, k.Pattern, k.Description, boolToInt(k.Approved), k.ProposedBy, now, now)
	return err
}

func (r themeKindRepo) Delete(ctx context.Context, name string) error {
	_, err := r.exec(ctx, `DELETE FROM theme_kinds WHERE name = ?`, name)
	return err
}

// CountPending is the ceiling's counter.
//
// ⚠️ A COUNT rather than len(List()): the handshake calls it on every frontend
// connection to decide whether it may record a proposal, and reading every row
// to answer "are there fewer than 64" would put the whole table on a path that
// runs once per page load.
func (r themeKindRepo) CountPending(ctx context.Context) (int, error) {
	rows, err := r.query(ctx, `SELECT COUNT(*) FROM theme_kinds WHERE approved = 0`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var n int
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
	}
	return n, rows.Err()
}
