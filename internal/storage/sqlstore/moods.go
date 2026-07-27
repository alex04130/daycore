package sqlstore

import (
	"context"
	"database/sql"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type moodRepo struct{ *Store }

func (r moodRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.MoodCheckin, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.query(ctx,
		`SELECT id, session_id, mood, ai_response, exercise_offered, exercise_completed, theme, source, note, created_at
		 FROM mood_checkins WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.MoodCheckin{}
	for rows.Next() {
		var (
			m         domain.MoodCheckin
			aiResp    sql.NullString
			offered   sql.NullString
			completed int
			theme     sql.NullString
			source    sql.NullString
			note      sql.NullString
			createdAt int64
		)
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Mood, &aiResp, &offered, &completed, &theme, &source, &note, &createdAt); err != nil {
			return nil, err
		}
		m.AIResponse = ptrString(aiResp)
		m.ExerciseOffered = ptrString(offered)
		m.ExerciseCompleted = completed != 0
		m.Theme = ptrString(theme)
		m.Source = source.String
		m.Note = note.String
		m.CreatedAt = fromMillis(createdAt)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r moodRepo) Create(ctx context.Context, m *domain.MoodCheckin) (*domain.MoodCheckin, error) {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO mood_checkins (id, session_id, mood, ai_response, exercise_offered, exercise_completed, theme, source, note, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.SessionID, m.Mood, nullString(m.AIResponse), nullString(m.ExerciseOffered),
		boolToInt(m.ExerciseCompleted), nullString(m.Theme), m.Source, m.Note, now)
	if err != nil {
		return nil, err
	}
	m.CreatedAt = fromMillis(now)
	return m, nil
}

func (r moodRepo) MarkExerciseCompleted(ctx context.Context, sessionID, id string) error {
	_, err := r.exec(ctx, `UPDATE mood_checkins SET exercise_completed = 1 WHERE id = ? AND session_id = ?`, id, sessionID)
	return err
}
