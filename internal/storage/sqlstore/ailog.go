package sqlstore

import (
	"context"
	"strings"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

const aiLogCols = `id, session_id, endpoint, model, prompt_tokens, comp_tokens, duration_ms, status, error, request_id, created_at`

type aiLogRepo struct{ *Store }

func (r aiLogRepo) Add(ctx context.Context, l *domain.AICallLog) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO ai_call_logs (id, session_id, endpoint, model, prompt_tokens, comp_tokens, duration_ms, status, error, request_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.SessionID, l.Endpoint, l.Model, l.PromptTokens, l.CompTokens, l.DurationMs, l.Status, l.Error, l.RequestID, now)
	if err != nil {
		return err
	}
	l.CreatedAt = fromMillis(now)
	return nil
}

// aiLogWhere renders the filter.
//
// ⚠️ Every narrowing value is BOUND, never interpolated — including the ones
// that look like closed vocabularies (status is "ok" or "error", endpoint is
// one of eleven constants). They arrive from a query string, and "the console
// only sends these" is a property of one client rather than of this function.
func aiLogWhere(f domain.AILogFilter) ([]string, []any) {
	where := []string{"1 = 1"}
	args := []any{}
	add := func(clause string, v any) {
		where = append(where, clause)
		args = append(args, v)
	}
	if f.SessionID != "" {
		add("session_id = ?", f.SessionID)
	}
	if f.Endpoint != "" {
		add("endpoint = ?", f.Endpoint)
	}
	if f.Model != "" {
		add("model = ?", f.Model)
	}
	if f.Status != "" {
		add("status = ?", f.Status)
	}
	if !f.Since.IsZero() {
		add("created_at >= ?", toMillis(f.Since))
	}
	// Backward keyset paging. The id breaks the tie, and it must — created_at is
	// stored to the millisecond and a burst of calls lands several rows inside
	// one. Without it a page boundary that falls mid-millisecond either repeats
	// a row or drops it, and dropping is the one nobody notices.
	if !f.Before.CreatedAt.IsZero() {
		ms := toMillis(f.Before.CreatedAt)
		where = append(where, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, ms, ms, f.Before.ID)
	}
	return where, args
}

func (r aiLogRepo) List(ctx context.Context, f domain.AILogFilter, limit int) ([]domain.AICallLog, error) {
	where, args := aiLogWhere(f)
	rows, err := r.query(ctx,
		`SELECT `+aiLogCols+` FROM ai_call_logs WHERE `+strings.Join(where, " AND ")+
			` ORDER BY created_at DESC, id DESC`+limitClause(limit, domain.AILogListDefault, domain.AILogListMax),
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AICallLog{}
	for rows.Next() {
		var l domain.AICallLog
		var created int64
		if err := rows.Scan(&l.ID, &l.SessionID, &l.Endpoint, &l.Model, &l.PromptTokens,
			&l.CompTokens, &l.DurationMs, &l.Status, &l.Error, &l.RequestID, &created); err != nil {
			return nil, err
		}
		l.CreatedAt = fromMillis(created)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r aiLogRepo) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.exec(ctx, `DELETE FROM ai_call_logs WHERE created_at < ?`, toMillis(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r aiLogRepo) Stats(ctx context.Context) (*domain.AdminStats, error) {
	row := r.queryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM users), (SELECT COUNT(*) FROM sessions),
		        (SELECT COUNT(*) FROM ai_call_logs), (SELECT COALESCE(SUM(comp_tokens), 0) FROM ai_call_logs)`)
	var s domain.AdminStats
	if err := row.Scan(&s.Users, &s.Sessions, &s.AICalls, &s.TokenUsed); err != nil {
		return nil, err
	}
	return &s, nil
}
