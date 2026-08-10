package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"daycore/internal/domain"
)

type pairingRepo struct{ *Store }

// roles_json rather than roles, and secret_hash rather than secret.
//
// The first follows the precedent this package set with permissions_json and
// rows_json: a name that is unreserved on all three engines. The second is
// there so nobody reading a query ever thinks the column holds the thing
// itself — it holds a verifier, and the difference is the whole design.
const pairingCols = `id, name, description, secret_hash, roles_json, last_seen_at, created_at, updated_at`

func scanPairing(s scanner) (domain.Pairing, error) {
	var p domain.Pairing
	var desc, rolesJSON sql.NullString
	var lastSeen, created, updated int64
	if err := s.Scan(&p.ID, &p.Name, &desc, &p.SecretHash, &rolesJSON, &lastSeen, &created, &updated); err != nil {
		return p, err
	}
	p.Description = desc.String
	// A role list that will not parse yields an EMPTY set, exactly as a role's
	// permission list does: a pairing whose grants cannot be read grants
	// nothing, which is what "we do not know what this permits" should mean.
	p.Roles = []string{}
	if rolesJSON.Valid && rolesJSON.String != "" {
		_ = json.Unmarshal([]byte(rolesJSON.String), &p.Roles)
		if p.Roles == nil {
			p.Roles = []string{}
		}
	}
	if lastSeen > 0 {
		p.LastSeenAt = fromMillis(lastSeen)
	}
	p.CreatedAt, p.UpdatedAt = fromMillis(created), fromMillis(updated)
	return p, nil
}

func (r pairingRepo) List(ctx context.Context) ([]domain.Pairing, error) {
	rows, err := r.query(ctx, `SELECT `+pairingCols+` FROM pairings ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Pairing{}
	for rows.Next() {
		p, err := scanPairing(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r pairingRepo) Get(ctx context.Context, id string) (*domain.Pairing, error) {
	if id == "" {
		return nil, domain.ErrNotFound
	}
	rows, err := r.query(ctx, `SELECT `+pairingCols+` FROM pairings WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, domain.ErrNotFound
	}
	p, err := scanPairing(rows)
	if err != nil {
		return nil, err
	}
	return &p, rows.Err()
}

func (r pairingRepo) Create(ctx context.Context, p *domain.Pairing) error {
	if p.ID == "" || p.SecretHash == "" {
		return domain.ErrMissingUpsertKey
	}
	roles := p.Roles
	if roles == nil {
		roles = []string{}
	}
	b, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	now := nowMillis()
	_, err = r.exec(ctx,
		`INSERT INTO pairings (`+pairingCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, p.SecretHash, string(b), int64(0), now, now)
	if err != nil {
		return err
	}
	p.CreatedAt, p.UpdatedAt = fromMillis(now), fromMillis(now)
	return nil
}

func (r pairingRepo) SetRoles(ctx context.Context, id string, roles []string) error {
	if roles == nil {
		roles = []string{}
	}
	b, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	res, err := r.exec(ctx, `UPDATE pairings SET roles_json = ?, updated_at = ? WHERE id = ?`,
		string(b), nowMillis(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// TouchLastSeen writes only when the stored value is already stale.
//
// ⚠️ The staleness test is IN THE STATEMENT, not around it. Doing it in Go
// would mean read-then-write, and this runs on the authorisation path of the
// busiest external console — two round trips per request to maintain a field
// nobody reads more than once a week, plus a race that would make two instances
// each write what the other just wrote.
func (r pairingRepo) TouchLastSeen(ctx context.Context, id string, now time.Time) (bool, error) {
	cutoff := toMillis(now.Add(-domain.PairingLastSeenGranularity))
	res, err := r.exec(ctx,
		`UPDATE pairings SET last_seen_at = ? WHERE id = ? AND last_seen_at <= ?`,
		toMillis(now), id, cutoff)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r pairingRepo) Delete(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `DELETE FROM pairings WHERE id = ?`, id)
	return err
}
