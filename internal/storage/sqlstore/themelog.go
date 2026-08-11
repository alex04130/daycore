package sqlstore

import (
	"context"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type themeLogRepo struct{ *Store }

func (r themeLogRepo) Add(ctx context.Context, sessionID, theme, familyID string) error {
	if familyID == "" {
		familyID = domain.FallbackFamilyID
	}
	_, err := r.exec(ctx,
		`INSERT INTO theme_switch_log (id, session_id, theme, family_id, switched_at) VALUES (?, ?, ?, ?, ?)`,
		uuid.NewString(), sessionID, theme, familyID, nowMillis())
	return err
}
