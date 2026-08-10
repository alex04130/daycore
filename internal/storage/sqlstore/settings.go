package sqlstore

import (
	"context"

	"daycore/internal/domain"
)

type settingRepo struct{ *Store }

// The column is setting_key, not key.
//
// KEY is reserved in MySQL 8, and this repo has already paid for that once:
// temp_contexts.key ships with backticks around every mention, which then have
// to be right in the CREATE, both indexes, and every statement. Renaming the
// column is the same fix proposals used for rows_json and start_time — it costs
// one word and removes a class of parse error that only appears on one dialect.
const settingCols = `setting_key, value, updated_at`

func (r settingRepo) All(ctx context.Context) ([]domain.Setting, error) {
	rows, err := r.query(ctx, `SELECT `+settingCols+` FROM settings ORDER BY setting_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Setting{}
	for rows.Next() {
		var s domain.Setting
		var ua int64
		if err := rows.Scan(&s.Key, &s.Value, &ua); err != nil {
			return nil, err
		}
		s.UpdatedAt = fromMillis(ua)
		out = append(out, s)
	}
	return out, rows.Err()
}

// Set is an upsert without a transaction, the same UPDATE-then-INSERT shape the
// rest of this package uses: the unique primary key is what makes the race
// harmless — a concurrent insert loses on the key and the retry updates.
func (r settingRepo) Set(ctx context.Context, key, value string) error {
	if key == "" {
		return domain.ErrMissingUpsertKey
	}
	now := nowMillis()
	res, err := r.exec(ctx, `UPDATE settings SET value = ?, updated_at = ? WHERE setting_key = ?`, value, now, key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.exec(ctx, `INSERT INTO settings (`+settingCols+`) VALUES (?, ?, ?)`, key, value, now)
	return err
}

// Delete restores the environment seed. Deleting a key that is not there is not
// an error: "reset this to default" is idempotent by nature, and two consoles
// clicking it is ordinary.
func (r settingRepo) Delete(ctx context.Context, key string) error {
	_, err := r.exec(ctx, `DELETE FROM settings WHERE setting_key = ?`, key)
	return err
}
