package sqlstore

import (
	"context"
	"encoding/json"

	"daycore/internal/domain"
)

type roleRepo struct{ *Store }

// The column is role_name, not name, in role_members — and permissions_json
// rather than permissions.
//
// Both follow the precedent this package set with setting_key and rows_json:
// pick a spelling that is unreserved on all three engines and unambiguous in a
// composite key, rather than quoting an identifier that is only quotable one
// way per dialect.
const roleCols = `name, description, permissions_json, created_at, updated_at`

func (r roleRepo) ListRoles(ctx context.Context) ([]domain.Role, error) {
	rows, err := r.query(ctx, `SELECT `+roleCols+` FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Role{}
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

func (r roleRepo) GetRole(ctx context.Context, name string) (*domain.Role, error) {
	rows, err := r.query(ctx, `SELECT `+roleCols+` FROM roles WHERE name = ?`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, domain.ErrNotFound
	}
	role, err := scanRole(rows)
	if err != nil {
		return nil, err
	}
	return &role, rows.Err()
}

type scanner interface{ Scan(dest ...any) error }

func scanRole(s scanner) (domain.Role, error) {
	var role domain.Role
	var permJSON string
	var created, updated int64
	if err := s.Scan(&role.Name, &role.Description, &permJSON, &created, &updated); err != nil {
		return role, err
	}
	// A permission list that will not parse yields an EMPTY set, not an error.
	//
	// Empty is the safe direction and it is also the honest one: a role whose
	// definition cannot be read grants nothing, which is what "we do not know
	// what this permits" should mean. Failing the whole read instead would take
	// every other role down with it, and the first thing an operator would try
	// is the console — which needs the role list to render.
	role.Permissions = []string{}
	if permJSON != "" {
		_ = json.Unmarshal([]byte(permJSON), &role.Permissions)
		if role.Permissions == nil {
			role.Permissions = []string{}
		}
	}
	role.CreatedAt, role.UpdatedAt = fromMillis(created), fromMillis(updated)
	return role, nil
}

func (r roleRepo) UpsertRole(ctx context.Context, role domain.Role) error {
	if role.Name == "" {
		return domain.ErrMissingUpsertKey
	}
	perms := role.Permissions
	if perms == nil {
		perms = []string{}
	}
	b, err := json.Marshal(perms)
	if err != nil {
		return err
	}
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE roles SET description = ?, permissions_json = ?, updated_at = ? WHERE name = ?`,
		role.Description, string(b), now, role.Name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.exec(ctx, `INSERT INTO roles (`+roleCols+`) VALUES (?, ?, ?, ?, ?)`,
		role.Name, role.Description, string(b), now, now)
	return err
}

// DeleteRole removes the definition AND the membership.
//
// Both, and in this order, because the alternative leaves membership rows that
// grant nothing until somebody creates a role with the same name again — at
// which point a set of people silently regain a permission set that was
// deleted. There is no transaction in this package, so the order matters: drop
// the definition first, so that a failure between the two statements leaves
// rows that grant nothing rather than a role whose membership is unknown.
func (r roleRepo) DeleteRole(ctx context.Context, name string) error {
	if _, err := r.exec(ctx, `DELETE FROM roles WHERE name = ?`, name); err != nil {
		return err
	}
	_, err := r.exec(ctx, `DELETE FROM role_members WHERE role_name = ?`, name)
	return err
}

func (r roleRepo) RolesOf(ctx context.Context, userID string) ([]string, error) {
	return r.names(ctx, `SELECT role_name FROM role_members WHERE user_id = ? ORDER BY role_name`, userID)
}

func (r roleRepo) MembersOf(ctx context.Context, name string) ([]string, error) {
	return r.names(ctx, `SELECT user_id FROM role_members WHERE role_name = ? ORDER BY user_id`, name)
}

func (r roleRepo) names(ctx context.Context, q string, arg string) ([]string, error) {
	rows, err := r.query(ctx, q, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// AddMember is idempotent: two consoles clicking the same button is ordinary.
//
// UPDATE-then-INSERT rather than INSERT-and-swallow-the-duplicate, which is the
// shape every other upsert in this package uses. The reason is portability: the
// three engines report a duplicate key with three different error types, and a
// helper that recognises all three is a helper that will one day recognise two.
// A no-op UPDATE that reports a row is the same signal, spelled identically
// everywhere.
func (r roleRepo) AddMember(ctx context.Context, name, userID string) error {
	if name == "" || userID == "" {
		return domain.ErrMissingUpsertKey
	}
	res, err := r.exec(ctx, `UPDATE role_members SET role_name = ? WHERE role_name = ? AND user_id = ?`,
		name, name, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.exec(ctx, `INSERT INTO role_members (role_name, user_id, created_at) VALUES (?, ?, ?)`,
		name, userID, nowMillis())
	return err
}

func (r roleRepo) RemoveMember(ctx context.Context, name, userID string) error {
	_, err := r.exec(ctx, `DELETE FROM role_members WHERE role_name = ? AND user_id = ?`, name, userID)
	return err
}
