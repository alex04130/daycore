package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type dayPlanRepo struct{ *Store }

func (r dayPlanRepo) Get(ctx context.Context, sessionID, date string) (*domain.DayPlan, error) {
	row := r.queryRow(ctx,
		`SELECT id, session_id, date, blocks, source_type, note, created_at, updated_at
		 FROM day_plans WHERE session_id = ? AND date = ?`, sessionID, date)
	return scanDayPlan(row.Scan)
}

func (r dayPlanRepo) Upsert(ctx context.Context, plan *domain.DayPlan) (*domain.DayPlan, error) {
	if plan.Blocks == nil {
		plan.Blocks = []domain.TimeBlock{}
	}
	blocksJSON := marshalJSON(plan.Blocks)
	sourceType := plan.SourceType
	if sourceType == "" {
		sourceType = "text"
	}
	now := nowMillis()

	res, err := r.exec(ctx,
		`UPDATE day_plans SET blocks = ?, source_type = ?, note = ?, updated_at = ?
		 WHERE session_id = ? AND date = ?`,
		blocksJSON, sourceType, nullString(plan.Note), now, plan.SessionID, plan.Date)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		id := plan.ID
		if id == "" {
			id = uuid.NewString()
		}
		_, err := r.exec(ctx,
			`INSERT INTO day_plans (id, session_id, date, blocks, source_type, note, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, plan.SessionID, plan.Date, blocksJSON, sourceType, nullString(plan.Note), now, now)
		if err != nil {
			return nil, err
		}
	}
	return r.Get(ctx, plan.SessionID, plan.Date)
}

func (r dayPlanRepo) Range(ctx context.Context, sessionID, from, to string) ([]domain.DayPlan, error) {
	rows, err := r.query(ctx,
		`SELECT id, session_id, date, blocks, source_type, note, created_at, updated_at
		 FROM day_plans WHERE session_id = ? AND date >= ? AND date <= ? ORDER BY date`,
		sessionID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.DayPlan
	for rows.Next() {
		p, err := scanDayPlan(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// scanDayPlan works for both *sql.Row.Scan and *sql.Rows.Scan via the scan func.
func scanDayPlan(scan func(dest ...any) error) (*domain.DayPlan, error) {
	var (
		p          domain.DayPlan
		blocksJSON string
		note       sql.NullString
		createdAt  int64
		updatedAt  int64
	)
	err := scan(&p.ID, &p.SessionID, &p.Date, &blocksJSON, &p.SourceType, &note, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if e := json.Unmarshal([]byte(blocksJSON), &p.Blocks); e != nil || p.Blocks == nil {
		p.Blocks = []domain.TimeBlock{}
	}
	p.Note = ptrString(note)
	p.CreatedAt = fromMillis(createdAt)
	p.UpdatedAt = fromMillis(updatedAt)
	return &p, nil
}
