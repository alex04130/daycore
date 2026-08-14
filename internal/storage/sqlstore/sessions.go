package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

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
		 VALUES (?, 0, 0, '', 'sky', ?, ?)`, id, now, now)
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

// persona_prompt and preferences are TEXT and therefore nullable — MySQL
// rejects a literal DEFAULT on TEXT, so they cannot be NOT NULL DEFAULT ”
// there (dialect.go). COALESCE keeps the scan targets plain strings and works
// on databases created before and after that change.
const sessionSelect = `SELECT id, user_id, interaction_count, sign_in_prompted, assistant_name,
	current_theme, language, import_token, COALESCE(persona_prompt, ''), COALESCE(preferences, ''),
	usage_fast_calls, usage_fast_tokens, usage_fast_start,
	usage_slow_calls, usage_slow_tokens, usage_slow_start,
	usage_calls, usage_prompt_tokens, usage_comp_tokens,
	created_at, updated_at FROM sessions`

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
	var fastStart, slowStart int64
	err := row.Scan(&s.ID, &userID, &s.InteractionCount, &signIn, &s.AssistantName, &s.CurrentTheme,
		&s.Language, &s.ImportToken, &s.PersonaPrompt, &s.Preferences,
		&s.Usage.FastCalls, &s.Usage.FastTokens, &fastStart,
		&s.Usage.SlowCalls, &s.Usage.SlowTokens, &slowStart,
		&s.Usage.TotalCalls, &s.Usage.TotalPromptTokens, &s.Usage.TotalCompTokens,
		&createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.UserID = ptrString(userID)
	s.SignInPrompted = signIn != 0
	// Zero means "no window has ever opened", which is not the epoch — a stored
	// zero rendered as 1970 would make every fresh session look permanently
	// expired AND permanently ancient on the console.
	if fastStart > 0 {
		s.Usage.FastWindowStart = fromMillis(fastStart)
	}
	if slowStart > 0 {
		s.Usage.SlowWindowStart = fromMillis(slowStart)
	}
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

// AddUsage folds one call into the three counters, in one statement.
//
// # Why the window reset is arithmetic and not a read-modify-write
//
// Two instances handling two calls for the same account at the same moment is
// ordinary, and a read-modify-write would lose one of them — silently, into a
// number a quota is meant to rest on. Expressing the rollover as CASE keeps the
// whole thing inside one UPDATE, which every engine applies atomically to the
// row. The same shape rhythm_days.Observe uses, for the same reason.
//
// Read it as: if the window opened longer ago than its length, this call STARTS
// a new window (counter = this call, stamp = now); otherwise it adds to the
// current one. A stamp of 0 means no window has ever opened, and `0 <= cutoff`
// is true, so the first call opens one without a special case.
func (r sessionRepo) AddUsage(ctx context.Context, sessionID string, promptTokens, compTokens int, now time.Time) error {
	if sessionID == "" {
		return domain.ErrMissingUpsertKey
	}
	tokens := int64(promptTokens + compTokens)
	nowMs := toMillis(now)
	fastCut := toMillis(now.Add(-domain.UsageFastWindow))
	slowCut := toMillis(now.Add(-domain.UsageSlowWindow))
	_, err := r.exec(ctx,
		`UPDATE sessions SET
			usage_fast_calls  = CASE WHEN usage_fast_start <= ? THEN 1 ELSE usage_fast_calls + 1 END,
			usage_fast_tokens = CASE WHEN usage_fast_start <= ? THEN ? ELSE usage_fast_tokens + ? END,
			usage_fast_start  = CASE WHEN usage_fast_start <= ? THEN ? ELSE usage_fast_start END,
			usage_slow_calls  = CASE WHEN usage_slow_start <= ? THEN 1 ELSE usage_slow_calls + 1 END,
			usage_slow_tokens = CASE WHEN usage_slow_start <= ? THEN ? ELSE usage_slow_tokens + ? END,
			usage_slow_start  = CASE WHEN usage_slow_start <= ? THEN ? ELSE usage_slow_start END,
			usage_calls = usage_calls + 1,
			usage_prompt_tokens = usage_prompt_tokens + ?,
			usage_comp_tokens = usage_comp_tokens + ?,
			updated_at = ?
		 WHERE id = ?`,
		fastCut, fastCut, tokens, tokens, fastCut, nowMs,
		slowCut, slowCut, tokens, tokens, slowCut, nowMs,
		int64(promptTokens), int64(compTokens), nowMs, sessionID)
	return err
}
