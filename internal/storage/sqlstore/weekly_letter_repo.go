package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type weeklyLetterRepo struct{ *Store }

const weeklyLetterSelect = `SELECT id, session_id, week_start, week_end, body, locale, created_at FROM weekly_letters`

// Latest returns the most recent letter: the latest week, and within a week the
// most recent generation. week_start is a YYYY-MM-DD string, so lexicographic
// order is chronological.
func (r weeklyLetterRepo) Latest(ctx context.Context, sessionID string) (*domain.WeeklyLetter, error) {
	row := r.queryRow(ctx, weeklyLetterSelect+` WHERE session_id = ? ORDER BY week_start DESC, created_at DESC, id DESC LIMIT 1`, sessionID)
	return scanWeeklyLetter(row.Scan)
}

func (r weeklyLetterRepo) Get(ctx context.Context, sessionID, id string) (*domain.WeeklyLetter, error) {
	row := r.queryRow(ctx, weeklyLetterSelect+` WHERE session_id = ? AND id = ?`, sessionID, id)
	return scanWeeklyLetter(row.Scan)
}

func (r weeklyLetterRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.WeeklyLetter, error) {
	rows, err := r.query(ctx, weeklyLetterSelect+` WHERE session_id = ? ORDER BY week_start DESC, created_at DESC`+
		limitClause(limit, domain.WeeklyLetterListDefault, domain.WeeklyLetterListMax), sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WeeklyLetter{}
	for rows.Next() {
		l, err := scanWeeklyLetter(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

func (r weeklyLetterRepo) Create(ctx context.Context, l *domain.WeeklyLetter) (*domain.WeeklyLetter, error) {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	_, err := r.exec(ctx,
		`INSERT INTO weekly_letters (id, session_id, week_start, week_end, body, locale, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.SessionID, l.WeekStart, l.WeekEnd, l.Body, l.Locale, nowMillis())
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, l.SessionID, l.ID)
}

func (r weeklyLetterRepo) Delete(ctx context.Context, sessionID, id string) error {
	res, err := r.exec(ctx, `DELETE FROM weekly_letters WHERE session_id = ? AND id = ?`, sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanWeeklyLetter(scan func(dest ...any) error) (*domain.WeeklyLetter, error) {
	var (
		l         domain.WeeklyLetter
		createdAt int64
	)
	err := scan(&l.ID, &l.SessionID, &l.WeekStart, &l.WeekEnd, &l.Body, &l.Locale, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	l.CreatedAt = fromMillis(createdAt)
	return &l, nil
}
