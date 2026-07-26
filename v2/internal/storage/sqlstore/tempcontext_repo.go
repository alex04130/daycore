package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type tempContextRepo struct{ *Store }

func (r tempContextRepo) Get(ctx context.Context, sid, key string) (*domain.TempContext, error) {
	row := r.queryRow(ctx,
		`SELECT id, session_id, key, payload, ttl, created_at FROM temp_contexts
		 WHERE session_id = ? AND key = ? AND ttl > ?`, sid, key, nowMillis())
	var (
		tc  domain.TempContext
		ttl int64
		ca  int64
	)
	err := row.Scan(&tc.ID, &tc.SessionID, &tc.Key, &tc.Payload, &ttl, &ca)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	tc.TTL = fromMillis(ttl)
	tc.CreatedAt = fromMillis(ca)
	return &tc, nil
}

func (r tempContextRepo) Set(ctx context.Context, tc *domain.TempContext) error {
	// UPDATE-then-INSERT keyed on (session_id, key) — cross-dialect safe (MySQL
	// has no ON CONFLICT) and, crucially, does NOT collide on an empty id, which
	// previously collapsed the whole table to a single global row.
	if tc.ID == "" {
		tc.ID = uuid.NewString()
	}
	if tc.CreatedAt.IsZero() {
		tc.CreatedAt = time.Now()
	}
	res, err := r.exec(ctx,
		`UPDATE temp_contexts SET payload = ?, ttl = ? WHERE session_id = ? AND key = ?`,
		tc.Payload, toMillis(tc.TTL), tc.SessionID, tc.Key)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = r.exec(ctx,
		`INSERT INTO temp_contexts (id, session_id, key, payload, ttl, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tc.ID, tc.SessionID, tc.Key, tc.Payload, toMillis(tc.TTL), toMillis(tc.CreatedAt))
	return err
}

func (r tempContextRepo) Delete(ctx context.Context, sid, key string) error {
	_, err := r.exec(ctx,
		`DELETE FROM temp_contexts WHERE session_id = ? AND key = ?`, sid, key)
	return err
}

func (r tempContextRepo) Expire(ctx context.Context) (int, error) {
	res, err := r.exec(ctx,
		`DELETE FROM temp_contexts WHERE ttl < ?`, nowMillis())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
