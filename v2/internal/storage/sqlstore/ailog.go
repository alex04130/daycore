package sqlstore

import (
	"context"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

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
