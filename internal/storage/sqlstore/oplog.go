package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type opLogRepo struct{ *Store }

func (r opLogRepo) Add(ctx context.Context, l *domain.OperationLog) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	l.Summary = domain.ClampOpLogSummary(l.Summary)
	if l.Domain == "" {
		l.Domain = domain.OpDomainOf(l.Action)
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO operation_logs (id, session_id, actor, action, domain, target_id, date, summary, detail, status, request_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.SessionID, l.Actor, l.Action, l.Domain, l.TargetID, l.Date, l.Summary, l.Detail, l.Status, l.RequestID, now)
	if err != nil {
		return err
	}
	l.CreatedAt = fromMillis(now)
	return nil
}

func (r opLogRepo) Get(ctx context.Context, sessionID, id string) (*domain.OperationLog, error) {
	row := r.queryRow(ctx,
		`SELECT id, session_id, actor, action, domain, target_id, date, summary, detail, status, request_id, created_at
		 FROM operation_logs WHERE id = ? AND session_id = ?`, id, sessionID)
	var (
		l         domain.OperationLog
		createdAt int64
	)
	err := row.Scan(&l.ID, &l.SessionID, &l.Actor, &l.Action, &l.Domain, &l.TargetID, &l.Date,
		&l.Summary, &l.Detail, &l.Status, &l.RequestID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Rows written before the column existed carry ''; derive rather than
	// backfill, so the log stays append-only.
	if l.Domain == "" {
		l.Domain = domain.OpDomainOf(l.Action)
	}
	l.CreatedAt = fromMillis(createdAt)
	return &l, nil
}

func (r opLogRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.OperationLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.query(ctx,
		`SELECT id, session_id, actor, action, domain, target_id, date, summary, detail, status, request_id, created_at
		 FROM operation_logs WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.OperationLog{}
	for rows.Next() {
		var (
			l         domain.OperationLog
			createdAt int64
		)
		if err := rows.Scan(&l.ID, &l.SessionID, &l.Actor, &l.Action, &l.Domain, &l.TargetID, &l.Date,
			&l.Summary, &l.Detail, &l.Status, &l.RequestID, &createdAt); err != nil {
			return nil, err
		}
		if l.Domain == "" {
			l.Domain = domain.OpDomainOf(l.Action)
		}
		l.CreatedAt = fromMillis(createdAt)
		out = append(out, l)
	}
	return out, rows.Err()
}
