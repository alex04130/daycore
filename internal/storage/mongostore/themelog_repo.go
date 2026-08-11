package mongostore

import (
	"context"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

type themeLogRepo struct{ *Store }

func (r themeLogRepo) Add(ctx context.Context, sessionID, theme, familyID string) error {
	if familyID == "" {
		familyID = domain.FallbackFamilyID
	}
	_, err := r.c("theme_switch_log").InsertOne(ctx, bson.M{
		"_id": uuid.NewString(), "session_id": sessionID, "theme": theme,
		"family_id": familyID, "switched_at": time.Now().UTC(),
	})
	return err
}
