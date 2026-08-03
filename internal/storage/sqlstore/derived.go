package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"daycore/internal/domain"
)

// ── rapport ─────────────────────────────────────────────────────────────────

type rapportRepo struct{ *Store }

func (r rapportRepo) Get(ctx context.Context, sessionID string) (*domain.RapportState, error) {
	row := r.queryRow(ctx,
		`SELECT session_id, COALESCE(scores_json, '{}'), cursor_created_at, cursor_id, fold_version, updated_at
		 FROM rapport_states WHERE session_id = ?`, sessionID)
	var (
		s               domain.RapportState
		scoresJSON      string
		cursorCreatedAt int64
		updatedAt       int64
	)
	err := row.Scan(&s.SessionID, &scoresJSON, &cursorCreatedAt, &s.Cursor.ID, &s.FoldVersion, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if e := json.Unmarshal([]byte(scoresJSON), &s.Scores); e != nil || s.Scores == nil {
		// A cache that will not parse is not an error to propagate — it is a
		// cache to throw away. The caller re-folds from the ledger, which is
		// where the truth was all along.
		//
		// Throwing it away has to include the cursor. Keeping the cursor while
		// dropping the scores tells the caller "fold forward from here" over
		// scores of zero, so every operation before that point is skipped
		// forever and the rebuilt reading permanently understates. Returning
		// early is the only way to be sure the restore below cannot undo this.
		return &domain.RapportState{
			SessionID: s.SessionID,
			Scores:    map[string]domain.RapportScore{},
			UpdatedAt: fromMillis(updatedAt),
		}, nil
	}
	if cursorCreatedAt > 0 {
		s.Cursor.CreatedAt = fromMillis(cursorCreatedAt)
	}
	s.UpdatedAt = fromMillis(updatedAt)
	return &s, nil
}

func (r rapportRepo) Save(ctx context.Context, s *domain.RapportState) error {
	if s.Scores == nil {
		s.Scores = map[string]domain.RapportScore{}
	}
	now := nowMillis()
	scores := marshalJSON(s.Scores)
	var cursorAt int64
	if !s.Cursor.CreatedAt.IsZero() {
		cursorAt = toMillis(s.Cursor.CreatedAt)
	}
	res, err := r.exec(ctx,
		`UPDATE rapport_states SET scores_json = ?, cursor_created_at = ?, cursor_id = ?,
			fold_version = ?, updated_at = ? WHERE session_id = ?`,
		scores, cursorAt, s.Cursor.ID, s.FoldVersion, now, s.SessionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = r.exec(ctx,
			`INSERT INTO rapport_states (session_id, scores_json, cursor_created_at, cursor_id, fold_version, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			s.SessionID, scores, cursorAt, s.Cursor.ID, s.FoldVersion, now)
		if err != nil {
			// Somebody inserted the row between our UPDATE and our INSERT. The
			// UPDATE that follows lands on their row and our value wins, which
			// is what the caller asked for; the duplicate-key error was never
			// theirs to see. leaseRepo.Acquire and jobRunRepo.Claim have always
			// treated a losing INSERT this way — these three did not, which was
			// the inconsistency.
			if res, rerr := r.exec(ctx,
				`UPDATE rapport_states SET scores_json = ?, cursor_created_at = ?, cursor_id = ?,
					fold_version = ?, updated_at = ? WHERE session_id = ?`,
				scores, cursorAt, s.Cursor.ID, s.FoldVersion, now, s.SessionID); rerr == nil {
				if n, _ := res.RowsAffected(); n > 0 {
					err = nil
				}
			}
		}
	}
	if err == nil {
		s.UpdatedAt = fromMillis(now)
	}
	return err
}

func (r rapportRepo) Reset(ctx context.Context, sessionID string) error {
	_, err := r.exec(ctx, `DELETE FROM rapport_states WHERE session_id = ?`, sessionID)
	return err
}

// ── rhythm ──────────────────────────────────────────────────────────────────

type rhythmRepo struct{ *Store }

func (r rhythmRepo) Get(ctx context.Context, sessionID string) (*domain.RhythmProfile, error) {
	row := r.queryRow(ctx,
		`SELECT session_id, wake_hm, sleep_hm, source, learned_days, run_since, last_signal_at, updated_at
		 FROM rhythm_profiles WHERE session_id = ?`, sessionID)
	var (
		p         domain.RhythmProfile
		runSince  sql.NullInt64
		lastAt    sql.NullInt64
		updatedAt int64
	)
	err := row.Scan(&p.SessionID, &p.Wake, &p.Sleep, &p.Source, &p.Days, &runSince, &lastAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if t := ptrMillis(runSince); t != nil {
		p.RunSince = *t
	}
	if t := ptrMillis(lastAt); t != nil {
		p.LastSignalAt = *t
	}
	p.UpdatedAt = fromMillis(updatedAt)
	return &p, nil
}

func (r rhythmRepo) Save(ctx context.Context, p *domain.RhythmProfile) error {
	now := nowMillis()
	var runSince, lastAt any
	if !p.RunSince.IsZero() {
		runSince = toMillis(p.RunSince)
	}
	if !p.LastSignalAt.IsZero() {
		lastAt = toMillis(p.LastSignalAt)
	}
	res, err := r.exec(ctx,
		`UPDATE rhythm_profiles SET wake_hm = ?, sleep_hm = ?, source = ?, learned_days = ?,
			updated_at = ? WHERE session_id = ?`,
		p.Wake, p.Sleep, p.Source, p.Days, now, p.SessionID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = r.exec(ctx,
			`INSERT INTO rhythm_profiles (session_id, wake_hm, sleep_hm, source, learned_days, run_since, last_signal_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			p.SessionID, p.Wake, p.Sleep, p.Source, p.Days, runSince, lastAt, now)
		if err != nil {
			if res, rerr := r.exec(ctx,
				`UPDATE rhythm_profiles SET wake_hm = ?, sleep_hm = ?, source = ?, learned_days = ?,
					updated_at = ? WHERE session_id = ?`,
				p.Wake, p.Sleep, p.Source, p.Days, now, p.SessionID); rerr == nil {
				if n, _ := res.RowsAffected(); n > 0 {
					err = nil
				}
			}
		}
	}
	if err == nil {
		p.UpdatedAt = fromMillis(now)
	}
	return err
}

// Touch moves the two live marks, and only forward. See RhythmRepository.Touch
// for why they are not part of Save.
func (r rhythmRepo) Touch(ctx context.Context, sessionID string, runSince, lastSignalAt time.Time) error {
	now := nowMillis()
	last := toMillis(lastSignalAt)
	res, err := r.exec(ctx,
		`UPDATE rhythm_profiles SET run_since = ?, last_signal_at = ?, updated_at = ?
		 WHERE session_id = ? AND (last_signal_at IS NULL OR last_signal_at < ?)`,
		toMillis(runSince), last, now, sessionID, last)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	// Either the row is not there yet, or the mark we hold is not newer than the
	// stored one. Try the insert; if THAT loses, the row exists with a newer mark
	// and there is nothing to do — which is the correct outcome for a stale
	// signal, not an error.
	// The seed row carries EMPTY body times, not the cold-start defaults: those
	// belong to internal/rhythm, and a storage layer that guessed them would be a
	// second place where "07:30" is written down. A caller that reads this row
	// sees Source "" and Wake "" and asks rhythm for the fallback, which is where
	// the answer lives.
	if _, ierr := r.exec(ctx,
		`INSERT INTO rhythm_profiles (session_id, wake_hm, sleep_hm, source, learned_days, run_since, last_signal_at, updated_at)
		 VALUES (?, '', '', '', 0, ?, ?, ?)`,
		sessionID, toMillis(runSince), last, now); ierr == nil {
		return nil
	}
	// The insert lost to a concurrent writer whose mark is newer than ours.
	// Nothing to do — a stale signal being ignored is the correct outcome, not an
	// error.
	return nil
}

// Observe widens a rhythm day's bounds, or creates the row. This runs on every
// awake signal, which is why the bounds are widened in SQL rather than read,
// compared and written back — that round trip would be a lost update whenever
// two tabs are open.
func (r rhythmRepo) Observe(ctx context.Context, sessionID, day string, minute int) error {
	res, err := r.exec(ctx,
		`UPDATE rhythm_days
		 SET first_min = CASE WHEN ? < first_min THEN ? ELSE first_min END,
			 last_min  = CASE WHEN ? > last_min  THEN ? ELSE last_min  END,
			 signals = signals + 1
		 WHERE session_id = ? AND day = ?`,
		minute, minute, minute, minute, sessionID, day)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.exec(ctx,
		`INSERT INTO rhythm_days (session_id, day, first_min, last_min, signals) VALUES (?, ?, ?, ?, 1)`,
		sessionID, day, minute, minute)
	if err == nil {
		return nil
	}
	// Two tabs sent the first signal of a new day at the same moment: both
	// UPDATEs matched nothing, both INSERTed, one lost on the primary key. The
	// loser folds into the winner's row instead of dropping the signal, which is
	// exactly what the widening UPDATE does.
	res, rerr := r.exec(ctx,
		`UPDATE rhythm_days
		 SET first_min = CASE WHEN ? < first_min THEN ? ELSE first_min END,
			 last_min  = CASE WHEN ? > last_min  THEN ? ELSE last_min  END,
			 signals = signals + 1
		 WHERE session_id = ? AND day = ?`,
		minute, minute, minute, minute, sessionID, day)
	if rerr != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	return err
}

func (r rhythmRepo) Days(ctx context.Context, sessionID string, limit int) ([]domain.RhythmDay, error) {
	rows, err := r.query(ctx,
		`SELECT session_id, day, first_min, last_min, signals FROM rhythm_days
		 WHERE session_id = ? ORDER BY day DESC`+limitClause(limit, domain.RhythmDaysDefault, domain.RhythmDaysMax), sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.RhythmDay{}
	for rows.Next() {
		var d domain.RhythmDay
		if err := rows.Scan(&d.SessionID, &d.Day, &d.FirstMin, &d.LastMin, &d.Signals); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r rhythmRepo) PruneDays(ctx context.Context, sessionID, before string) (int, error) {
	res, err := r.exec(ctx,
		`DELETE FROM rhythm_days WHERE session_id = ? AND day < ?`, sessionID, before)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ── locale overrides ────────────────────────────────────────────────────────

type localeRepo struct{ *Store }

func (r localeRepo) All(ctx context.Context) ([]domain.LocaleOverride, error) {
	rows, err := r.query(ctx,
		`SELECT message_key, locale, COALESCE(content, ''), updated_at FROM locale_overrides
		 ORDER BY message_key, locale`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.LocaleOverride{}
	for rows.Next() {
		var (
			o         domain.LocaleOverride
			updatedAt int64
		)
		if err := rows.Scan(&o.Key, &o.Locale, &o.Content, &updatedAt); err != nil {
			return nil, err
		}
		o.UpdatedAt = fromMillis(updatedAt)
		out = append(out, o)
	}
	return out, rows.Err()
}

// Set is UPDATE-then-INSERT rather than an upsert: MySQL has no ON CONFLICT and
// this package writes one statement per dialect. Same shape as promptRepo.Set,
// which is the precedent this whole table copies.
func (r localeRepo) Set(ctx context.Context, key, locale, content string) error {
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE locale_overrides SET content = ?, updated_at = ? WHERE message_key = ? AND locale = ?`,
		content, now, key, locale)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.exec(ctx,
		`INSERT INTO locale_overrides (message_key, locale, content, updated_at) VALUES (?, ?, ?, ?)`,
		key, locale, content, now)
	if err == nil {
		return nil
	}
	// A double-clicked console save on a brand-new key: both UPDATEs matched
	// nothing, both INSERTed, one lost. Our value still wins.
	res, rerr := r.exec(ctx,
		`UPDATE locale_overrides SET content = ?, updated_at = ? WHERE message_key = ? AND locale = ?`,
		content, now, key, locale)
	if rerr != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	return err
}

func (r localeRepo) Delete(ctx context.Context, key, locale string) error {
	_, err := r.exec(ctx,
		`DELETE FROM locale_overrides WHERE message_key = ? AND locale = ?`, key, locale)
	return err
}

func (r localeRepo) DeleteLocale(ctx context.Context, locale string) (int, error) {
	res, err := r.exec(ctx, `DELETE FROM locale_overrides WHERE locale = ?`, locale)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
