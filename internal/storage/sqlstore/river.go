package sqlstore

import (
	"context"
	"time"

	"daycore/internal/domain"
)

type riverRepo struct{ *Store }

// Days returns the last `days` UTC days inclusive of today, oldest first, with
// each day's operation count and mood emoji.
//
// # Day boundaries are computed in Go, once
//
// created_at is epoch milliseconds and the three SQL dialects each spell date
// extraction differently, so the boundary for "a day" is computed here rather
// than in the query — the same rule as the AI usage rollup (see ai_usage.go).
// That keeps one definition of "a day" shared with every reader through
// domain.UTCDay.
//
// # One query for moods, one COUNT per day for operations
//
// Mood check-ins are low volume, so the whole window is read in one query and
// bucketed in Go (newest-first, so the first resolvable id of a day is its
// newest). Operations can be high volume, so each day's total is a COUNT over
// the (session_id, created_at) range rather than materialising every row.
func (r riverRepo) Days(ctx context.Context, sessionID string, days int) ([]domain.RiverDay, error) {
	today := domain.UTCDay(time.Now())
	start, err := domain.StartOfUTCDay(today)
	if err != nil {
		return nil, err
	}
	from := start.AddDate(0, 0, -(days - 1))
	end := start.AddDate(0, 0, 1) // exclusive

	moods, err := r.moodsByDay(ctx, sessionID, from, end)
	if err != nil {
		return nil, err
	}

	out := make([]domain.RiverDay, 0, days)
	for d := from; d.Before(end); d = d.AddDate(0, 0, 1) {
		day := domain.UTCDay(d)
		row := domain.RiverDay{Date: day, Mood: moods[day]}
		lo, hi := toMillis(d), toMillis(d.AddDate(0, 0, 1))
		if err := r.queryRow(ctx,
			`SELECT COUNT(*) FROM operation_logs WHERE session_id = ? AND created_at >= ? AND created_at < ?`,
			sessionID, lo, hi).Scan(&row.Count); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// moodsByDay folds one window of check-ins into day → emoji, newest resolvable
// winning. Unresolvable ids are skipped, not stored: a day whose only check-in
// is an id the registry no longer knows reads as "" rather than as a raw id.
func (r riverRepo) moodsByDay(ctx context.Context, sessionID string, from, end time.Time) (map[string]string, error) {
	rows, err := r.query(ctx,
		`SELECT mood, created_at FROM mood_checkins WHERE session_id = ? AND created_at >= ? AND created_at < ? ORDER BY created_at DESC`,
		sessionID, toMillis(from), toMillis(end))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var moodID string
		var created int64
		if err := rows.Scan(&moodID, &created); err != nil {
			return nil, err
		}
		day := domain.UTCDay(fromMillis(created))
		if _, set := out[day]; set {
			continue // a newer check-in already won this day
		}
		if kind, ok := domain.MoodKindByID(moodID); ok {
			out[day] = kind.Emoji
		}
	}
	return out, rows.Err()
}
