package sqlstore

import (
	"context"
	"database/sql"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type memoryRepo struct{ *Store }

func (r memoryRepo) ListFacts(ctx context.Context, sessionID string) ([]domain.MemoryFact, error) {
	rows, err := r.query(ctx,
		`SELECT id, session_id, fact, source, COALESCE(type,''), created_at FROM memory_facts
		 WHERE session_id = ? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.MemoryFact{}
	for rows.Next() {
		var f domain.MemoryFact
		var createdAt int64
		if err := rows.Scan(&f.ID, &f.SessionID, &f.Fact, &f.Source, &f.Type, &createdAt); err != nil {
			return nil, err
		}
		f.CreatedAt = fromMillis(createdAt)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r memoryRepo) AddFact(ctx context.Context, f *domain.MemoryFact) (*domain.MemoryFact, error) {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	if f.Source == "" {
		f.Source = "chat"
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO memory_facts (id, session_id, fact, source, type, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		f.ID, f.SessionID, f.Fact, f.Source, f.Type, now)
	if err != nil {
		return nil, err
	}
	f.CreatedAt = fromMillis(now)
	return f, nil
}

func (r memoryRepo) DeleteFact(ctx context.Context, sessionID, id string) error {
	res, err := r.exec(ctx, `DELETE FROM memory_facts WHERE session_id = ? AND id = ?`, sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r memoryRepo) ClearFacts(ctx context.Context, sessionID string) (int, error) {
	res, err := r.exec(ctx, `DELETE FROM memory_facts WHERE session_id = ?`, sessionID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r memoryRepo) AddImport(ctx context.Context, rec *domain.ImportRecord) (*domain.ImportRecord, error) {
	if rec.ID == "" {
		rec.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO import_history (id, session_id, source, items, summary, payload, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.SessionID, rec.Source, rec.Items, rec.Summary, rec.Payload, now)
	if err != nil {
		return nil, err
	}
	rec.CreatedAt = fromMillis(now)
	return rec, nil
}

func (r memoryRepo) ListImports(ctx context.Context, sessionID string, limit int) ([]domain.ImportRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.query(ctx,
		`SELECT id, session_id, source, items, summary, created_at FROM import_history
		 WHERE session_id = ? ORDER BY created_at DESC LIMIT `+itoa(limit), sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.ImportRecord{}
	for rows.Next() {
		var rec domain.ImportRecord
		var summary sql.NullString
		var createdAt int64
		if err := rows.Scan(&rec.ID, &rec.SessionID, &rec.Source, &rec.Items, &summary, &createdAt); err != nil {
			return nil, err
		}
		if summary.Valid {
			rec.Summary = summary.String
		}
		rec.CreatedAt = fromMillis(createdAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

// itoa avoids fmt for a trivially-safe int.
func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	buf := [8]byte{}
	i := len(buf)
	for n > 0 && i > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
