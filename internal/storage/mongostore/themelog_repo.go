package mongostore

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

type themeLogRepo struct{ *Store }

func (r themeLogRepo) Add(ctx context.Context, sessionID, theme string) error {
	_, err := r.c("theme_switch_log").InsertOne(ctx, bson.M{
		"_id": uuid.NewString(), "session_id": sessionID, "theme": theme, "switched_at": time.Now().UTC(),
	})
	return err
}
