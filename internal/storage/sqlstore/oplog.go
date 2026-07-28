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
	return scanOpLog(row.Scan)
}

// scanOpLog reads one row of the shared column list. Rows written before the
// domain column existed carry an empty string and are classified on read —
// the log is append-only, and OpDomainOf reproduces the same answer from the
// action.
func scanOpLog(scan func(dest ...any) error) (*domain.OperationLog, error) {
	var (
		l         domain.OperationLog
		createdAt int64
	)
	err := scan(&l.ID, &l.SessionID, &l.Actor, &l.Action, &l.Domain, &l.TargetID, &l.Date,
		&l.Summary, &l.Detail, &l.Status, &l.RequestID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if l.Domain == "" {
		l.Domain = domain.OpDomainOf(l.Action)
	}
	l.CreatedAt = fromMillis(createdAt)
	return &l, nil
}

// Scan walks the log oldest-first so derived state can be rebuilt by re-folding
// it. The keyset predicate (created_at, id) is what makes paging safe here:
// created_at is milliseconds, ties are ordinary, and OFFSET paging over an
// append-only table would drift as new rows land mid-replay.
func (r opLogRepo) Scan(ctx context.Context, sessionID string, after domain.OpLogCursor, limit int) ([]domain.OperationLog, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.query(ctx,
		`SELECT id, session_id, actor, action, domain, target_id, date, summary, detail, status, request_id, created_at
		 FROM operation_logs
		 WHERE session_id = ? AND (created_at > ? OR (created_at = ? AND id > ?))
		 ORDER BY created_at ASC, id ASC LIMIT ?`,
		sessionID, toMillis(after.CreatedAt), toMillis(after.CreatedAt), after.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOpLogs(rows)
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
	return scanOpLogs(rows)
}

func (r opLogRepo) RevertedBy(ctx context.Context, sessionID, targetID string) (*domain.OperationLog, error) {
	row := r.queryRow(ctx,
		`SELECT id, session_id, actor, action, domain, target_id, date, summary, detail, status, request_id, created_at
		 FROM operation_logs WHERE session_id = ? AND action = ? AND target_id = ?
		 ORDER BY created_at ASC LIMIT 1`, sessionID, "revert", targetID)
	return scanOpLog(row.Scan)
}

// scanOpLogs reads the shared SELECT column list. Rows written before the
// domain column existed carry an empty string and are classified on read — the
// log is append-only, and OpDomainOf reproduces the same answer from the action.
func scanOpLogs(rows *sql.Rows) ([]domain.OperationLog, error) {
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
