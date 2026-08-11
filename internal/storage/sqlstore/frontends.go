package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"daycore/internal/domain"
)

type frontendRepo struct{ *Store }

const familyCols = `id, display_name, tokens_json, rules, rules_accepted, pinned, created_at, updated_at`
const buildCols = `build_hash, family_id, display_name, build_version, min_api, first_seen_at, last_seen_at`

func scanFamily(s scanner) (domain.FrontendFamily, error) {
	var f domain.FrontendFamily
	var name, tokensJSON, rules sql.NullString
	var accepted, pinned int
	var created, updated int64
	if err := s.Scan(&f.ID, &name, &tokensJSON, &rules, &accepted, &pinned, &created, &updated); err != nil {
		return f, err
	}
	f.DisplayName, f.Rules = name.String, rules.String
	f.RulesAccepted, f.Pinned = accepted != 0, pinned != 0
	// A token space that will not parse yields an EMPTY one, the same call the
	// role and pairing repositories make: a family whose declarations cannot be
	// read declares nothing, so every theme write against it is refused with
	// "unknown variable" rather than accepted unvalidated.
	f.Tokens = []domain.TokenSpec{}
	if tokensJSON.Valid && tokensJSON.String != "" {
		_ = json.Unmarshal([]byte(tokensJSON.String), &f.Tokens)
		if f.Tokens == nil {
			f.Tokens = []domain.TokenSpec{}
		}
	}
	f.CreatedAt, f.UpdatedAt = fromMillis(created), fromMillis(updated)
	return f, nil
}

func (r frontendRepo) ListFamilies(ctx context.Context) ([]domain.FrontendFamily, error) {
	rows, err := r.query(ctx, `SELECT `+familyCols+` FROM frontend_families ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FrontendFamily{}
	for rows.Next() {
		f, err := scanFamily(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r frontendRepo) GetFamily(ctx context.Context, id string) (*domain.FrontendFamily, error) {
	if id == "" {
		return nil, domain.ErrNotFound
	}
	rows, err := r.query(ctx, `SELECT `+familyCols+` FROM frontend_families WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, domain.ErrNotFound
	}
	f, err := scanFamily(rows)
	if err != nil {
		return nil, err
	}
	return &f, rows.Err()
}

func (r frontendRepo) UpsertFamily(ctx context.Context, f domain.FrontendFamily) error {
	if f.ID == "" {
		return domain.ErrMissingUpsertKey
	}
	tokens := f.Tokens
	if tokens == nil {
		tokens = []domain.TokenSpec{}
	}
	b, err := json.Marshal(tokens)
	if err != nil {
		return err
	}
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE frontend_families SET display_name = ?, tokens_json = ?, rules = ?,
		 rules_accepted = ?, pinned = ?, updated_at = ? WHERE id = ?`,
		f.DisplayName, string(b), f.Rules, boolToInt(f.RulesAccepted), boolToInt(f.Pinned), now, f.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.exec(ctx, `INSERT INTO frontend_families (`+familyCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.DisplayName, string(b), f.Rules, boolToInt(f.RulesAccepted), boolToInt(f.Pinned), now, now)
	return err
}

func (r frontendRepo) DeleteFamily(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `DELETE FROM frontend_families WHERE id = ?`, id)
	return err
}

func (r frontendRepo) GetBuild(ctx context.Context, hash string) (*domain.FrontendBuild, error) {
	if hash == "" {
		return nil, domain.ErrNotFound
	}
	b, err := r.getBuild(ctx, hash)
	if err == sql.ErrNoRows {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r frontendRepo) ListBuilds(ctx context.Context) ([]domain.FrontendBuild, error) {
	rows, err := r.query(ctx, `SELECT `+buildCols+` FROM frontend_builds ORDER BY family_id, first_seen_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.FrontendBuild{}
	for rows.Next() {
		var b domain.FrontendBuild
		var name, version sql.NullString
		var first, last int64
		if err := rows.Scan(&b.BuildHash, &b.FamilyID, &name, &version, &b.MinAPI, &first, &last); err != nil {
			return nil, err
		}
		b.DisplayName, b.Version = name.String, version.String
		b.FirstSeenAt, b.LastSeenAt = fromMillis(first), fromMillis(last)
		out = append(out, b)
	}
	return out, rows.Err()
}

// SeenBuild records a handshake.
//
// ⚠️ THE UPDATE DOES NOT TOUCH family_id. An operator who moved a build into
// another family would otherwise have that undone by the build's next startup —
// silently, and with the themes following it. The move has its own method.
//
// The last-seen write is conditional in the statement, like a pairing's: a
// frontend that handshakes on every page load must not turn that into a write
// every time.
func (r frontendRepo) SeenBuild(ctx context.Context, b domain.FrontendBuild, staleBefore time.Time) error {
	if b.BuildHash == "" || b.FamilyID == "" {
		return domain.ErrMissingUpsertKey
	}
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE frontend_builds SET display_name = ?, build_version = ?, min_api = ?, last_seen_at = ?
		 WHERE build_hash = ? AND last_seen_at <= ?`,
		b.DisplayName, b.Version, b.MinAPI, now, b.BuildHash, toMillis(staleBefore))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	// Either the row is fresh (nothing to do) or it does not exist yet. Telling
	// them apart costs a read; inserting and tolerating the duplicate costs an
	// error type per engine. So: try the insert, and if the row was simply
	// fresh, the insert fails on the primary key and that is the right outcome.
	_, err = r.exec(ctx, `INSERT INTO frontend_builds (`+buildCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.BuildHash, b.FamilyID, b.DisplayName, b.Version, b.MinAPI, now, now)
	if err != nil {
		// A duplicate here means the row existed and was fresh — the ordinary
		// case for a frontend that handshakes often. Confirm rather than guess
		// at the driver's error type, which is the thing this package refuses to
		// do (three engines, three spellings).
		if _, gerr := r.getBuild(ctx, b.BuildHash); gerr == nil {
			return nil
		}
		return err
	}
	return nil
}

func (r frontendRepo) getBuild(ctx context.Context, hash string) (domain.FrontendBuild, error) {
	var b domain.FrontendBuild
	var name, version sql.NullString
	var first, last int64
	err := r.queryRow(ctx, `SELECT `+buildCols+` FROM frontend_builds WHERE build_hash = ?`, hash).
		Scan(&b.BuildHash, &b.FamilyID, &name, &version, &b.MinAPI, &first, &last)
	if err != nil {
		return b, err
	}
	b.DisplayName, b.Version = name.String, version.String
	b.FirstSeenAt, b.LastSeenAt = fromMillis(first), fromMillis(last)
	return b, nil
}

func (r frontendRepo) SetBuildFamily(ctx context.Context, buildHash, familyID string) error {
	res, err := r.exec(ctx, `UPDATE frontend_builds SET family_id = ? WHERE build_hash = ?`, familyID, buildHash)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
