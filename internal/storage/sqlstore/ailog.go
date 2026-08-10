package sqlstore

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

const aiLogCols = `id, session_id, endpoint, model, prompt_tokens, comp_tokens, duration_ms, status, error, request_id, created_at`

type aiLogRepo struct{ *Store }

func (r aiLogRepo) Add(ctx context.Context, l *domain.AICallLog) error {
	if l.ID == "" {
		l.ID = uuid.NewString()
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO ai_call_logs (id, session_id, endpoint, model, prompt_tokens, comp_tokens, duration_ms, status, error, request_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.SessionID, l.Endpoint, l.Model, l.PromptTokens, l.CompTokens, l.DurationMs, l.Status, l.Error, l.RequestID, now)
	if err != nil {
		return err
	}
	l.CreatedAt = fromMillis(now)
	return nil
}

// aiLogWhere renders the filter.
//
// ⚠️ Every narrowing value is BOUND, never interpolated — including the ones
// that look like closed vocabularies (status is "ok" or "error", endpoint is
// one of eleven constants). They arrive from a query string, and "the console
// only sends these" is a property of one client rather than of this function.
func aiLogWhere(f domain.AILogFilter) ([]string, []any) {
	where := []string{"1 = 1"}
	args := []any{}
	add := func(clause string, v any) {
		where = append(where, clause)
		args = append(args, v)
	}
	if f.SessionID != "" {
		add("session_id = ?", f.SessionID)
	}
	if f.Endpoint != "" {
		add("endpoint = ?", f.Endpoint)
	}
	if f.Model != "" {
		add("model = ?", f.Model)
	}
	if f.Status != "" {
		add("status = ?", f.Status)
	}
	if !f.Since.IsZero() {
		add("created_at >= ?", toMillis(f.Since))
	}
	// Backward keyset paging. The id breaks the tie, and it must — created_at is
	// stored to the millisecond and a burst of calls lands several rows inside
	// one. Without it a page boundary that falls mid-millisecond either repeats
	// a row or drops it, and dropping is the one nobody notices.
	if !f.Before.CreatedAt.IsZero() {
		ms := toMillis(f.Before.CreatedAt)
		where = append(where, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, ms, ms, f.Before.ID)
	}
	return where, args
}

func (r aiLogRepo) List(ctx context.Context, f domain.AILogFilter, limit int) ([]domain.AICallLog, error) {
	where, args := aiLogWhere(f)
	rows, err := r.query(ctx,
		`SELECT `+aiLogCols+` FROM ai_call_logs WHERE `+strings.Join(where, " AND ")+
			` ORDER BY created_at DESC, id DESC`+limitClause(limit, domain.AILogListDefault, domain.AILogListMax),
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AICallLog{}
	for rows.Next() {
		var l domain.AICallLog
		var created int64
		if err := rows.Scan(&l.ID, &l.SessionID, &l.Endpoint, &l.Model, &l.PromptTokens,
			&l.CompTokens, &l.DurationMs, &l.Status, &l.Error, &l.RequestID, &created); err != nil {
			return nil, err
		}
		l.CreatedAt = fromMillis(created)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r aiLogRepo) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.exec(ctx, `DELETE FROM ai_call_logs WHERE created_at < ?`, toMillis(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r aiLogRepo) Stats(ctx context.Context) (*domain.AdminStats, error) {
	row := r.queryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM users), (SELECT COUNT(*) FROM sessions),
		        (SELECT COUNT(*) FROM ai_call_logs), (SELECT COALESCE(SUM(comp_tokens), 0) FROM ai_call_logs)`)
	var s domain.AdminStats
	if err := row.Scan(&s.Users, &s.Sessions, &s.AICalls, &s.TokenUsed); err != nil {
		return nil, err
	}
	return &s, nil
}

// ── the daily rollup ────────────────────────────────────────────────────────

const aiUsageCols = `day, model, endpoint, calls, errors, prompt_tokens, comp_tokens`

// RollUpUsage folds each closed day the ledger holds and the rollup does not.
//
// # One statement per day, and the engine does the work
//
// `INSERT … SELECT … GROUP BY` — the rows never leave the database. The
// alternative that was written first, incrementing a counter on every AI call,
// cost an extra write on the hot path and a read-modify-write race between
// instances; this costs one statement per day and has neither. The full
// argument is in domain/ai_usage.go.
//
// # Why a millisecond range instead of a date function
//
// created_at is epoch milliseconds and every engine spells date extraction
// differently (`strftime` / `to_timestamp` / `FROM_UNIXTIME`). Computing the
// day boundary in Go keeps ONE definition of where a day starts — shared with
// every reader through domain.UTCDay — and keeps this query identical on all
// three dialects.
//
// # Why DELETE-then-INSERT rather than an upsert
//
// A portable upsert of many rows at once does not exist: SQLite and Postgres
// spell it `ON CONFLICT`, MySQL spells it `ON DUPLICATE KEY`, and the
// UPDATE-then-INSERT pattern used elsewhere in this package is per row, which
// would put the round trips straight back. Deleting the day first makes the
// write a plain INSERT on every engine.
//
// The window where a day reads as zero is real and bounded: it lasts one
// statement, only for a day being folded for the FIRST time, and only on the
// leader. A day already in the rollup is skipped entirely, so steady state
// never re-writes anything.
func (r aiLogRepo) RollUpUsage(ctx context.Context, today string) (int, error) {
	// Where to start. The day after the newest one already folded; failing
	// that, the oldest day the ledger still has.
	var start string
	var newest sql.NullString
	if err := r.queryRow(ctx, `SELECT MAX(day) FROM ai_usage_daily`).Scan(&newest); err != nil {
		return 0, err
	}
	// ⚠️ START AT the newest folded day, not after it — but never before the
	// oldest row the ledger still has.
	//
	// Two failures, pulling in opposite directions, and both are silent:
	//
	//   MAX(day)+1 skips a PARTIAL day. Mongo's $merge writes documents one at
	//   a time, so a fold killed midway leaves a day with some of its groups;
	//   +1 never revisits it and the number stays quietly short forever.
	//
	//   MAX(day) alone destroys a PRUNED day. Re-folding a day whose ledger rows
	//   are gone computes zero and writes zero over real history — the rollup is
	//   the only copy by then, so that is permanent.
	//
	// Clamping to the ledger's oldest row resolves both: the newest day is
	// re-folded while its rows are still there, and stops being touched the
	// moment they are not. The same clamp bounds the loop on a deployment that
	// has been idle or whose leader was down — there is nothing to fold before
	// the ledger begins.
	var oldest sql.NullInt64
	if err := r.queryRow(ctx, `SELECT MIN(created_at) FROM ai_call_logs`).Scan(&oldest); err != nil {
		return 0, err
	}
	if !oldest.Valid {
		return 0, nil // nothing has ever been logged, or all of it is pruned
	}
	start = domain.UTCDay(fromMillis(oldest.Int64))
	if newest.Valid && newest.String > start {
		start = newest.String
	}

	from, err := domain.StartOfUTCDay(start)
	if err != nil {
		return 0, err
	}
	end, err := domain.StartOfUTCDay(today)
	if err != nil {
		return 0, err
	}

	written := 0
	for d := from; d.Before(end); d = d.AddDate(0, 0, 1) {
		day := domain.UTCDay(d)
		lo := toMillis(d)
		hi := toMillis(d.AddDate(0, 0, 1))
		// Clears a half-written day from an interrupted previous run. A day
		// already complete is not reached — the loop starts after the newest
		// folded day.
		if _, err := r.exec(ctx, `DELETE FROM ai_usage_daily WHERE day = ?`, day); err != nil {
			return written, err
		}
		// status is compared rather than counted with a CASE on every dialect's
		// own boolean spelling: SUM(CASE WHEN … THEN 1 ELSE 0 END) is the one
		// form all three agree on.
		if _, err := r.exec(ctx,
			`INSERT INTO ai_usage_daily (`+aiUsageCols+`, updated_at)
			 SELECT ?, model, endpoint,
			        COUNT(*),
			        SUM(CASE WHEN status = ? THEN 1 ELSE 0 END),
			        SUM(prompt_tokens), SUM(comp_tokens), ?
			 FROM ai_call_logs
			 WHERE created_at >= ? AND created_at < ?
			 GROUP BY model, endpoint`,
			day, domain.AICallStatusError, nowMillis(), lo, hi); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// aiUsageWindow renders the [from, to] day filter. Both bounds are optional and
// both are inclusive — a caller asking for one day passes it as both.
func aiUsageWindow(from, to string) (string, []any) {
	where, args := "1 = 1", []any{}
	if from != "" {
		where += " AND day >= ?"
		args = append(args, from)
	}
	if to != "" {
		where += " AND day <= ?"
		args = append(args, to)
	}
	return where, args
}

func (r aiLogRepo) UsageTotals(ctx context.Context, from, to string) (*domain.AIUsageTotals, error) {
	where, args := aiUsageWindow(from, to)
	var t domain.AIUsageTotals
	var calls, errs, pt, ct sql.NullInt64
	var first sql.NullString
	err := r.queryRow(ctx,
		`SELECT COALESCE(SUM(calls),0), COALESCE(SUM(errors),0),
		        COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(comp_tokens),0), MIN(day)
		 FROM ai_usage_daily WHERE `+where, args...).
		Scan(&calls, &errs, &pt, &ct, &first)
	if err != nil {
		return nil, err
	}
	t.Calls, t.Errors, t.PromptTokens, t.CompTokens = calls.Int64, errs.Int64, pt.Int64, ct.Int64
	if first.Valid {
		t.FirstDay = first.String
	}
	return &t, nil
}

func (r aiLogRepo) UsageDays(ctx context.Context, from, to string, limit int) ([]domain.AIUsageDay, error) {
	where, args := aiUsageWindow(from, to)
	rows, err := r.query(ctx,
		`SELECT day, SUM(calls), SUM(errors), SUM(prompt_tokens), SUM(comp_tokens)
		 FROM ai_usage_daily WHERE `+where+
			` GROUP BY day ORDER BY day DESC`+limitClause(limit, domain.UsageDaysDefault, domain.UsageDaysMax),
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AIUsageDay{}
	for rows.Next() {
		var d domain.AIUsageDay
		if err := rows.Scan(&d.Day, &d.Calls, &d.Errors, &d.PromptTokens, &d.CompTokens); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r aiLogRepo) UsageByModel(ctx context.Context, from, to string) ([]domain.AIUsageDay, error) {
	where, args := aiUsageWindow(from, to)
	rows, err := r.query(ctx,
		`SELECT model, SUM(calls), SUM(errors), SUM(prompt_tokens), SUM(comp_tokens)
		 FROM ai_usage_daily WHERE `+where+
			` GROUP BY model ORDER BY SUM(prompt_tokens) + SUM(comp_tokens) DESC, model ASC`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AIUsageDay{}
	for rows.Next() {
		var d domain.AIUsageDay
		if err := rows.Scan(&d.Model, &d.Calls, &d.Errors, &d.PromptTokens, &d.CompTokens); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
