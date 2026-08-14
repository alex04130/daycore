package sqlstore

import (
	"context"
)

// purgeSessionDeletes is the cascade behind DELETE /api/admin/users/{id}. Every
// table here carries a session_id column and is owned by one session: when the
// session goes, its rows go with it.
//
// ⚠️ Written as explicit backtick literals rather than a loop over a name list
// on purpose: TestEveryTableUsedBySQLIsCreated scans exactly these literals, so
// a typo in one of these names is a failing test instead of a runtime "no such
// table" on some engine the author was not running.
var purgeSessionDeletes = []string{
	`DELETE FROM day_plans WHERE session_id = ?`,
	`DELETE FROM mood_checkins WHERE session_id = ?`,
	`DELETE FROM feedback_logs WHERE session_id = ?`,
	`DELETE FROM proposals WHERE session_id = ?`,
	`DELETE FROM job_runs WHERE session_id = ?`,
	`DELETE FROM rapport_states WHERE session_id = ?`,
	`DELETE FROM rhythm_profiles WHERE session_id = ?`,
	`DELETE FROM rhythm_days WHERE session_id = ?`,
	`DELETE FROM companion_memory WHERE session_id = ?`,
	`DELETE FROM theme_switch_log WHERE session_id = ?`,
	`DELETE FROM operation_logs WHERE session_id = ?`,
	`DELETE FROM schedule_rules WHERE session_id = ?`,
	`DELETE FROM courses WHERE session_id = ?`,
	`DELETE FROM assignments WHERE session_id = ?`,
	`DELETE FROM custom_themes WHERE session_id = ?`,
	`DELETE FROM memory_facts WHERE session_id = ?`,
	`DELETE FROM import_history WHERE session_id = ?`,
	`DELETE FROM chat_threads WHERE session_id = ?`,
	`DELETE FROM chat_messages WHERE session_id = ?`,
	`DELETE FROM attachments WHERE session_id = ?`,
	`DELETE FROM ai_call_logs WHERE session_id = ?`,
	`DELETE FROM channel_bindings WHERE session_id = ?`,
	`DELETE FROM materials WHERE session_id = ?`,
	`DELETE FROM wishes WHERE session_id = ?`,
	`DELETE FROM temp_contexts WHERE session_id = ?`,
}

// PurgeSession deletes one session and every row it owns. See
// domain.Store.PurgeSession for the best-effort contract.
func (s *Store) PurgeSession(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, nil
	}
	var firstErr error
	for _, q := range purgeSessionDeletes {
		if _, err := s.exec(ctx, q, sessionID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	res, err := s.exec(ctx, `DELETE FROM sessions WHERE id = ?`, sessionID)
	if err != nil && firstErr == nil {
		firstErr = err
	}
	purged := 0
	if err == nil {
		if n, rerr := res.RowsAffected(); rerr == nil && n > 0 {
			purged = 1
		}
	}
	return purged, firstErr
}
