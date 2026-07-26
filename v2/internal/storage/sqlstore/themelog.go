package sqlstore

import (
	"context"

	"github.com/google/uuid"
)

type themeLogRepo struct{ *Store }

func (r themeLogRepo) Add(ctx context.Context, sessionID, theme string) error {
	_, err := r.exec(ctx,
		`INSERT INTO theme_switch_log (id, session_id, theme, switched_at) VALUES (?, ?, ?, ?)`,
		uuid.NewString(), sessionID, theme, nowMillis())
	return err
}
