package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"daycore/internal/domain"
)

type sessionRepo struct{ *Store }

func (r sessionRepo) GetOrCreate(ctx context.Context, id string) (*domain.Session, error) {
	if s, err := r.Get(ctx, id); err == nil {
		return s, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO sessions (id, interaction_count, sign_in_prompted, assistant_name, current_theme, created_at, updated_at)
		 VALUES (?, 0, 0, 'Leo', 'sky', ?, ?)`, id, now, now)
	if err != nil {
		if s, e := r.Get(ctx, id); e == nil {
			return s, nil
		}
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r sessionRepo) Get(ctx context.Context, id string) (*domain.Session, error) {
	row := r.queryRow(ctx, sessionSelect+` WHERE id = ?`, id)
	return scanSession(row)
}

func (r sessionRepo) GetByImportToken(ctx context.Context, token string) (*domain.Session, error) {
	if token == "" {
		return nil, domain.ErrNotFound
	}
	row := r.queryRow(ctx, sessionSelect+` WHERE import_token = ?`, token)
	return scanSession(row)
}

const sessionSelect = `SELECT id, user_id, interaction_count, sign_in_prompted, assistant_name,
	current_theme, language, import_token, persona_prompt, preferences, created_at, updated_at FROM sessions`

func (r sessionRepo) Update(ctx context.Context, id string, upd domain.SessionUpdate) (*domain.Session, error) {
	set := []string{}
	args := []any{}
	if upd.UserID != nil {
		set = append(set, "user_id = ?")
		args = append(args, *upd.UserID)
	}
	if upd.AssistantName != nil {
		set = append(set, "assistant_name = ?")
		args = append(args, *upd.AssistantName)
	}
	if upd.CurrentTheme != nil {
		set = append(set, "current_theme = ?")
		args = append(args, *upd.CurrentTheme)
	}
	if upd.SignInPrompted != nil {
		set = append(set, "sign_in_prompted = ?")
		args = append(args, boolToInt(*upd.SignInPrompted))
	}
	if upd.Language != nil {
		set = append(set, "language = ?")
		args = append(args, *upd.Language)
	}
	if upd.PersonaPrompt != nil {
		set = append(set, "persona_prompt = ?")
		args = append(args, *upd.PersonaPrompt)
	}
	if upd.ImportToken != nil {
		set = append(set, "import_token = ?")
		args = append(args, *upd.ImportToken)
	}
	if upd.Preferences != nil {
		set = append(set, "preferences = ?")
		args = append(args, *upd.Preferences)
	}
	if len(set) == 0 {
		return r.Get(ctx, id)
	}
	set = append(set, "updated_at = ?")
	args = append(args, nowMillis())
	query := "UPDATE sessions SET " + joinComma(set) + " WHERE id = ?"
	args = append(args, id)
	if _, err := r.exec(ctx, query, args...); err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r sessionRepo) IncrementInteraction(ctx context.Context, id string) error {
	_, err := r.exec(ctx,
		`UPDATE sessions SET interaction_count = interaction_count + 1, updated_at = ? WHERE id = ?`,
		nowMillis(), id)
	return err
}

func scanSession(row *sql.Row) (*domain.Session, error) {
	var (
		s         domain.Session
		userID    sql.NullString
		signIn    int
		createdAt int64
		updatedAt int64
	)
	err := row.Scan(&s.ID, &userID, &s.InteractionCount, &signIn, &s.AssistantName, &s.CurrentTheme,
		&s.Language, &s.ImportToken, &s.PersonaPrompt, &s.Preferences, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.UserID = ptrString(userID)
	s.SignInPrompted = signIn != 0
	s.CreatedAt = fromMillis(createdAt)
	s.UpdatedAt = fromMillis(updatedAt)
	return &s, nil
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
