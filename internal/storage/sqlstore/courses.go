package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type courseRepo struct{ *Store }

const courseSelect = `SELECT id, session_id, canvas_id, name, course_code, current_score,
	current_grade, created_at, updated_at FROM courses`

func (r courseRepo) UpsertByCanvasID(ctx context.Context, c *domain.Course) (*domain.Course, error) {
	if c.CanvasID == "" {
		return nil, domain.ErrMissingUpsertKey
	}
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE courses SET name = ?, course_code = ?, current_score = ?, current_grade = ?, updated_at = ?
		 WHERE session_id = ? AND canvas_id = ?`,
		c.Name, c.CourseCode, nullFloat(c.CurrentScore), nullString(c.CurrentGrade), now,
		c.SessionID, c.CanvasID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		id := c.ID
		if id == "" {
			id = uuid.NewString()
		}
		_, err := r.exec(ctx,
			`INSERT INTO courses (id, session_id, canvas_id, name, course_code, current_score,
				current_grade, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, c.SessionID, c.CanvasID, c.Name, c.CourseCode,
			nullFloat(c.CurrentScore), nullString(c.CurrentGrade), now, now)
		if err != nil {
			return nil, err
		}
	}
	row := r.queryRow(ctx, courseSelect+` WHERE session_id = ? AND canvas_id = ?`, c.SessionID, c.CanvasID)
	return scanCourse(row.Scan)
}

func (r courseRepo) List(ctx context.Context, sessionID string) ([]domain.Course, error) {
	rows, err := r.query(ctx, courseSelect+` WHERE session_id = ? ORDER BY name`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Course{}
	for rows.Next() {
		c, err := scanCourse(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func scanCourse(scan func(dest ...any) error) (*domain.Course, error) {
	var (
		c         domain.Course
		score     sql.NullFloat64
		grade     sql.NullString
		createdAt int64
		updatedAt int64
	)
	err := scan(&c.ID, &c.SessionID, &c.CanvasID, &c.Name, &c.CourseCode, &score, &grade, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if score.Valid {
		v := score.Float64
		c.CurrentScore = &v
	}
	c.CurrentGrade = ptrString(grade)
	c.CreatedAt = fromMillis(createdAt)
	c.UpdatedAt = fromMillis(updatedAt)
	return &c, nil
}

// nullFloat returns *float64 as a driver arg (nil → SQL NULL).
func nullFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
