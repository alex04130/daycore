package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type assignmentRepo struct{ *Store }

const assignmentSelect = `SELECT id, session_id, course_id, canvas_id, title, due_at, points_possible,
	submitted, graded, score, COALESCE(html_url, ''), source, status, created_at, updated_at FROM assignments`

func (r assignmentRepo) Get(ctx context.Context, sessionID, id string) (*domain.Assignment, error) {
	row := r.queryRow(ctx, assignmentSelect+` WHERE session_id = ? AND id = ?`, sessionID, id)
	return scanAssignment(row.Scan)
}

func (r assignmentRepo) List(ctx context.Context, sessionID string, f domain.AssignmentFilter) ([]domain.Assignment, error) {
	query := assignmentSelect + ` WHERE session_id = ?`
	args := []any{sessionID}
	if f.DueFrom != nil {
		query += ` AND due_at IS NOT NULL AND due_at >= ?`
		args = append(args, toMillis(*f.DueFrom))
	}
	if f.DueTo != nil {
		query += ` AND due_at IS NOT NULL AND due_at <= ?`
		args = append(args, toMillis(*f.DueTo))
	}
	if f.Status != "" {
		query += ` AND status = ?`
		args = append(args, f.Status)
	}
	query += ` ORDER BY due_at IS NULL, due_at, title`

	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Assignment{}
	for rows.Next() {
		a, err := scanAssignment(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r assignmentRepo) UpsertByCanvasID(ctx context.Context, a *domain.Assignment) (*domain.Assignment, error) {
	if a.CanvasID == "" {
		return nil, domain.ErrMissingUpsertKey
	}
	now := nowMillis()
	var dueAt any
	if a.DueAt != nil {
		dueAt = toMillis(*a.DueAt)
	}
	// Status is deliberately NOT refreshed: it tracks the local planner workflow
	// ("planned"/"done"/"dismissed"), which a re-import must not reset.
	res, err := r.exec(ctx,
		`UPDATE assignments SET course_id = ?, title = ?, due_at = ?, points_possible = ?,
			submitted = ?, graded = ?, score = ?, html_url = ?, source = ?, updated_at = ?
		 WHERE session_id = ? AND canvas_id = ?`,
		a.CourseID, a.Title, dueAt, nullFloat(a.PointsPossible),
		boolToInt(a.Submitted), boolToInt(a.Graded), nullFloat(a.Score), a.HTMLURL, a.Source, now,
		a.SessionID, a.CanvasID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		id := a.ID
		if id == "" {
			id = uuid.NewString()
		}
		status := a.Status
		if status == "" {
			status = domain.AssignmentPending
		}
		_, err := r.exec(ctx,
			`INSERT INTO assignments (id, session_id, course_id, canvas_id, title, due_at,
				points_possible, submitted, graded, score, html_url, source, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, a.SessionID, a.CourseID, a.CanvasID, a.Title, dueAt,
			nullFloat(a.PointsPossible), boolToInt(a.Submitted), boolToInt(a.Graded),
			nullFloat(a.Score), a.HTMLURL, a.Source, status, now, now)
		if err != nil {
			return nil, err
		}
	}
	row := r.queryRow(ctx, assignmentSelect+` WHERE session_id = ? AND canvas_id = ?`, a.SessionID, a.CanvasID)
	return scanAssignment(row.Scan)
}

func (r assignmentRepo) SetStatus(ctx context.Context, sessionID, id, status string) error {
	res, err := r.exec(ctx,
		`UPDATE assignments SET status = ?, updated_at = ? WHERE session_id = ? AND id = ?`,
		status, nowMillis(), sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r assignmentRepo) Delete(ctx context.Context, sessionID, id string) error {
	res, err := r.exec(ctx,
		`DELETE FROM assignments WHERE session_id = ? AND id = ?`, sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanAssignment(scan func(dest ...any) error) (*domain.Assignment, error) {
	var (
		a         domain.Assignment
		dueAt     sql.NullInt64
		points    sql.NullFloat64
		submitted int
		graded    int
		score     sql.NullFloat64
		createdAt int64
		updatedAt int64
	)
	err := scan(&a.ID, &a.SessionID, &a.CourseID, &a.CanvasID, &a.Title, &dueAt, &points,
		&submitted, &graded, &score, &a.HTMLURL, &a.Source, &a.Status, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if dueAt.Valid {
		t := fromMillis(dueAt.Int64)
		a.DueAt = &t
	}
	if points.Valid {
		v := points.Float64
		a.PointsPossible = &v
	}
	a.Submitted = submitted != 0
	a.Graded = graded != 0
	if score.Valid {
		v := score.Float64
		a.Score = &v
	}
	a.CreatedAt = fromMillis(createdAt)
	a.UpdatedAt = fromMillis(updatedAt)
	return &a, nil
}
