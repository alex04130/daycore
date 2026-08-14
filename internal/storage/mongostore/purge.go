package mongostore

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
)

// sessionCollections are the collections whose documents carry a session_id
// field — everything one session owns. Mirrors sqlstore's purgeSessionDeletes
// one for one.
var sessionCollections = []string{
	"day_plans",
	"mood_checkins",
	"feedback_logs",
	"proposals",
	"job_runs",
	"rhythm_days",
	"companion_memory",
	"theme_switch_log",
	"operation_logs",
	"schedule_rules",
	"courses",
	"assignments",
	"custom_themes",
	"memory_facts",
	"import_history",
	"chat_threads",
	"chat_messages",
	"attachments",
	"ai_call_logs",
	"channel_bindings",
	"materials",
	"wishes",
	"temp_contexts",
}

// PurgeSession deletes one session and every row it owns. See
// domain.Store.PurgeSession for the best-effort contract.
func (s *Store) PurgeSession(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, nil
	}
	var firstErr error
	for _, coll := range sessionCollections {
		if _, err := s.c(coll).DeleteMany(ctx, bson.M{"session_id": sessionID}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// Two derived caches key their _id on the session id itself rather than a
	// session_id field, mirroring the SQL side where session_id is the PK.
	for _, coll := range []string{"rapport_states", "rhythm_profiles"} {
		if _, err := s.c(coll).DeleteOne(ctx, bson.M{"_id": sessionID}); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	res, err := s.c("sessions").DeleteOne(ctx, bson.M{"_id": sessionID})
	if err != nil && firstErr == nil {
		firstErr = err
	}
	purged := 0
	if err == nil && res.DeletedCount > 0 {
		purged = 1
	}
	return purged, firstErr
}
