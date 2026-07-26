package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type ruleRepo struct{ *Store }

const ruleSelect = `SELECT id, session_id, title, block_type, start_time, duration_min, timezone,
	time_mode, kind, date, freq, interval_n, by_weekday, start_date, until_date, active, source,
	note, created_at, updated_at FROM schedule_rules`

func (r ruleRepo) Get(ctx context.Context, sessionID, id string) (*domain.ScheduleRule, error) {
	row := r.queryRow(ctx, ruleSelect+` WHERE session_id = ? AND id = ?`, sessionID, id)
	return scanRule(row.Scan)
}

func (r ruleRepo) List(ctx context.Context, sessionID string) ([]domain.ScheduleRule, error) {
	rows, err := r.query(ctx, ruleSelect+` WHERE session_id = ? ORDER BY created_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.ScheduleRule{}
	for rows.Next() {
		rule, err := scanRule(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *rule)
	}
	return out, rows.Err()
}

func (r ruleRepo) Create(ctx context.Context, rule *domain.ScheduleRule) (*domain.ScheduleRule, error) {
	if rule.ID == "" {
		rule.ID = uuid.NewString()
	}
	if rule.Interval < 1 {
		rule.Interval = 1
	}
	now := nowMillis()
	_, err := r.exec(ctx,
		`INSERT INTO schedule_rules (id, session_id, title, block_type, start_time, duration_min,
			timezone, time_mode, kind, date, freq, interval_n, by_weekday, start_date, until_date,
			active, source, note, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.SessionID, rule.Title, string(rule.Type), nullString(rule.Time), nullInt(rule.DurationMin),
		rule.Timezone, string(rule.TimeMode), rule.Kind, nullString(rule.Date), rule.Freq, rule.Interval,
		marshalJSON(weekdaysOrEmpty(rule.ByWeekday)), rule.StartDate, nullString(rule.Until),
		boolToInt(rule.Active), rule.Source, nullString(rule.Note), now, now)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, rule.SessionID, rule.ID)
}

func (r ruleRepo) Update(ctx context.Context, sessionID, id string, upd domain.ScheduleRuleUpdate) (*domain.ScheduleRule, error) {
	set := []string{}
	args := []any{}
	add := func(col string, v any) {
		set = append(set, col+" = ?")
		args = append(args, v)
	}
	if upd.Title != nil {
		add("title", *upd.Title)
	}
	if upd.Type != nil {
		add("block_type", string(*upd.Type))
	}
	if upd.HasTime {
		if upd.Time == nil || *upd.Time == "" {
			add("start_time", nil)
		} else {
			add("start_time", *upd.Time)
		}
	}
	if upd.DurationMin != nil {
		add("duration_min", *upd.DurationMin)
	}
	if upd.Timezone != nil {
		add("timezone", *upd.Timezone)
	}
	if upd.TimeMode != nil {
		add("time_mode", string(*upd.TimeMode))
	}
	if upd.Kind != nil {
		add("kind", *upd.Kind)
	}
	if upd.Date != nil {
		add("date", *upd.Date)
	}
	if upd.Freq != nil {
		add("freq", *upd.Freq)
	}
	if upd.Interval != nil {
		n := *upd.Interval
		if n < 1 {
			n = 1
		}
		add("interval_n", n)
	}
	if upd.ByWeekday != nil {
		add("by_weekday", marshalJSON(weekdaysOrEmpty(*upd.ByWeekday)))
	}
	if upd.StartDate != nil {
		add("start_date", *upd.StartDate)
	}
	if upd.HasUntil {
		if upd.Until == nil || *upd.Until == "" {
			add("until_date", nil)
		} else {
			add("until_date", *upd.Until)
		}
	}
	if upd.Active != nil {
		add("active", boolToInt(*upd.Active))
	}
	if upd.Note != nil {
		add("note", *upd.Note)
	}
	if len(set) == 0 {
		return r.Get(ctx, sessionID, id)
	}
	add("updated_at", nowMillis())
	query := "UPDATE schedule_rules SET " + joinComma(set) + " WHERE session_id = ? AND id = ?"
	args = append(args, sessionID, id)
	res, err := r.exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Distinguish "not found" from "no-op update" with a read.
		if _, err := r.Get(ctx, sessionID, id); err != nil {
			return nil, err
		}
	}
	return r.Get(ctx, sessionID, id)
}

func (r ruleRepo) Delete(ctx context.Context, sessionID, id string) error {
	res, err := r.exec(ctx, `DELETE FROM schedule_rules WHERE session_id = ? AND id = ?`, sessionID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanRule(scan func(dest ...any) error) (*domain.ScheduleRule, error) {
	var (
		rule        domain.ScheduleRule
		blockType   string
		startTime   sql.NullString
		durationMin sql.NullInt64
		timeMode    string
		date        sql.NullString
		byWeekday   string
		until       sql.NullString
		active      int
		note        sql.NullString
		createdAt   int64
		updatedAt   int64
	)
	err := scan(&rule.ID, &rule.SessionID, &rule.Title, &blockType, &startTime, &durationMin,
		&rule.Timezone, &timeMode, &rule.Kind, &date, &rule.Freq, &rule.Interval, &byWeekday,
		&rule.StartDate, &until, &active, &rule.Source, &note, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rule.Type = domain.BlockType(blockType)
	rule.Time = ptrString(startTime)
	if durationMin.Valid {
		v := int(durationMin.Int64)
		rule.DurationMin = &v
	}
	rule.TimeMode = domain.TimeMode(timeMode)
	rule.Date = ptrString(date)
	if e := json.Unmarshal([]byte(byWeekday), &rule.ByWeekday); e != nil {
		rule.ByWeekday = nil
	}
	rule.Until = ptrString(until)
	rule.Active = active != 0
	rule.Note = ptrString(note)
	rule.CreatedAt = fromMillis(createdAt)
	rule.UpdatedAt = fromMillis(updatedAt)
	return &rule, nil
}

// nullInt returns *int as a driver arg (nil → SQL NULL).
func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// weekdaysOrEmpty normalizes nil to an empty slice so by_weekday stores "[]".
func weekdaysOrEmpty(w []int) []int {
	if w == nil {
		return []int{}
	}
	return w
}
