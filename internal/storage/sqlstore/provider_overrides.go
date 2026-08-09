package sqlstore

import (
	"context"
	"encoding/json"

	"daycore/internal/domain"
)

type providerOverrideRepo struct{ *Store }

// The column is provider_id, not id.
//
// Not reserved anywhere, but this table's key is composite (kind, provider_id)
// and a bare `id` in a two-column key reads as "the row's id" in every query
// that joins or debugs against it. The naming cost is one word; the confusion
// cost is somebody writing WHERE id = ... and getting two rows.
const providerOverrideCols = `kind, provider_id, enabled, description_json, description_hash, approved, updated_at`

func (r providerOverrideRepo) All(ctx context.Context) ([]domain.ProviderOverride, error) {
	rows, err := r.query(ctx, `SELECT `+providerOverrideCols+` FROM provider_overrides ORDER BY kind, provider_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ProviderOverride{}
	for rows.Next() {
		var o domain.ProviderOverride
		var enabled *bool
		var descJSON string
		var ua int64
		if err := rows.Scan(&o.Kind, &o.ID, &enabled, &descJSON, &o.DescriptionHash, &o.Approved, &ua); err != nil {
			return nil, err
		}
		o.Enabled = enabled
		if descJSON != "" {
			// A row whose description will not parse is a row somebody wrote by
			// hand or an older shape. Dropping just the description keeps the
			// enabled/approved bits, which is the half that changes behaviour —
			// failing the whole read would take down every other source's
			// settings over one bad string.
			_ = json.Unmarshal([]byte(descJSON), &o.Description)
		}
		o.UpdatedAt = fromMillis(ua)
		out = append(out, o)
	}
	return out, rows.Err()
}

// Set is an upsert without a transaction, the UPDATE-then-INSERT shape the rest
// of this package uses: the composite primary key makes the race harmless — a
// concurrent insert loses on the key and the caller's retry updates.
func (r providerOverrideRepo) Set(ctx context.Context, o domain.ProviderOverride) error {
	if o.Kind == "" || o.ID == "" {
		return domain.ErrMissingUpsertKey
	}
	desc := ""
	if len(o.Description) > 0 {
		b, err := json.Marshal(o.Description)
		if err != nil {
			return err
		}
		desc = string(b)
	}
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE provider_overrides SET enabled = ?, description_json = ?, description_hash = ?, approved = ?, updated_at = ?
		 WHERE kind = ? AND provider_id = ?`,
		o.Enabled, desc, o.DescriptionHash, o.Approved, now, o.Kind, o.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.exec(ctx, `INSERT INTO provider_overrides (`+providerOverrideCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		o.Kind, o.ID, o.Enabled, desc, o.DescriptionHash, o.Approved, now)
	return err
}

// Delete restores whatever providers.yaml says. Deleting a row that is not
// there is not an error: "reset this to the file" is idempotent by nature, and
// the console cannot know whether a row exists without a round trip that would
// itself be a race.
func (r providerOverrideRepo) Delete(ctx context.Context, kind, id string) error {
	_, err := r.exec(ctx, `DELETE FROM provider_overrides WHERE kind = ? AND provider_id = ?`, kind, id)
	return err
}
