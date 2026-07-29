package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

type proposalRepo struct{ *Store }

// Column names diverge from the Go field names in two places on purpose: ROWS
// and START are reserved words in MySQL 8, and an unquoted reserved word is a
// parse error that Rebind cannot fix (temp_contexts.key shipped exactly that
// bug). rows_json and start_time avoid it without needing quoting anywhere.
// Every nullable column is COALESCEd, not just the JSON ones. title, summary,
// reason, evidence and lock_reason are TEXT — which this schema requires to be
// nullable in all three dialects, because MySQL refuses a literal DEFAULT on
// TEXT — and they scan into plain strings. A single NULL would fail the scan and
// take down not just that card but the whole session's List.
const proposalCols = `id, session_id, state, level, kind,
	COALESCE(title, ''), COALESCE(summary, ''), COALESCE(reason, ''), COALESCE(evidence, ''),
	date, start_time, duration_min, block_type, lock_level, COALESCE(lock_reason, ''),
	COALESCE(rows_json, '[]'), COALESCE(ops_json, '[]'),
	COALESCE(applied_op_ids, '[]'), COALESCE(accept_op_ids, '[]'),
	merge_key, deliver_after, delivered_at, pushed_at,
	ttl_policy, expires_at, resolution, origin, thread_id, owner_instance,
	rev, created_at, updated_at`

func (r proposalRepo) Create(ctx context.Context, p *domain.Proposal) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.State == "" {
		p.State = domain.ProposalPending
	}
	rowsJSON, err := marshalStrict(orEmptyRows(p.Rows))
	if err != nil {
		return err
	}
	opsJSON, err := marshalStrict(orEmptyOps(p.Ops))
	if err != nil {
		return err
	}
	now := nowMillis()
	p.CreatedAt, p.UpdatedAt = fromMillis(now), fromMillis(now)
	p.Rev = 1
	_, err = r.exec(ctx,
		`INSERT INTO proposals (id, session_id, state, level, kind, title, summary, reason, evidence,
			date, start_time, duration_min, block_type, lock_level, lock_reason,
			rows_json, ops_json, applied_op_ids, accept_op_ids,
			merge_key, deliver_after, delivered_at, pushed_at,
			ttl_policy, expires_at, resolution, origin, thread_id, owner_instance,
			rev, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.SessionID, string(p.State), string(p.Level), string(p.Kind), p.Title, p.Summary, p.Reason, p.Evidence,
		p.Date, p.Start, nullInt(p.Dur), string(p.BType), string(p.LockLevel), p.LockReason,
		rowsJSON, opsJSON,
		marshalJSON(orEmptyStrings(p.AppliedOpIDs)), marshalJSON(orEmptyStrings(p.AcceptOpIDs)),
		p.MergeKey, nullMillis(p.DeliverAfter), nullMillis(p.DeliveredAt), nullMillis(p.PushedAt),
		string(p.TTLPolicy), toMillis(p.ExpiresAt), string(p.Resolution), string(p.Origin), p.ThreadID, p.OwnerInstance,
		p.Rev, now, now)
	return err
}

func (r proposalRepo) Get(ctx context.Context, sessionID, id string) (*domain.Proposal, error) {
	row := r.queryRow(ctx,
		`SELECT `+proposalCols+` FROM proposals WHERE session_id = ? AND id = ?`, sessionID, id)
	return scanProposal(row.Scan)
}

func (r proposalRepo) List(ctx context.Context, f domain.ProposalFilter) ([]domain.Proposal, error) {
	where := []string{"session_id = ?"}
	args := []any{f.SessionID}
	if f.State != "" {
		where = append(where, "state = ?")
		args = append(args, string(f.State))
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, string(f.Kind))
	}
	if f.Date != "" {
		where = append(where, "date = ?")
		args = append(args, f.Date)
	}
	if f.Undelivered {
		where = append(where, "delivered_at IS NULL")
	}
	if !f.DeliverableAt.IsZero() {
		// Both halves of Deliverable that a query can express. Doing this in Go
		// after the fact would apply the LIMIT before the filter, so a pool of
		// lapsed cards could return an empty page while live ones waited behind.
		ms := toMillis(f.DeliverableAt)
		where = append(where, "expires_at > ?", "(deliver_after IS NULL OR deliver_after <= ?)")
		args = append(args, ms, ms)
	}
	rows, err := r.query(ctx,
		`SELECT `+proposalCols+` FROM proposals WHERE `+strings.Join(where, " AND ")+
			` ORDER BY created_at DESC`+limitClause(f.Limit, 100, 1000), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Proposal{}
	for rows.Next() {
		p, err := scanProposal(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Update is a compare-and-set on rev. Two clients accepting different rows of
// the same compound card would otherwise read the same rows JSON, each write
// their own line into it, and the second write would erase the first.
func (r proposalRepo) Update(ctx context.Context, p *domain.Proposal) error {
	// Validate on the way out as well as on the way in. Without it a caller can
	// blank the title, blank ttl_policy and zero the expiry — and a proposal
	// whose ttl_policy is "" matches neither arm of the expiry sweep, so it
	// stays pending forever, invisible to the mechanism meant to retire it.
	if err := p.Validate(); err != nil {
		return err
	}
	rowsJSON, err := marshalStrict(orEmptyRows(p.Rows))
	if err != nil {
		return err
	}
	opsJSON, err := marshalStrict(orEmptyOps(p.Ops))
	if err != nil {
		return err
	}
	now := nowMillis()
	res, err := r.exec(ctx,
		`UPDATE proposals SET state = ?, title = ?, summary = ?, reason = ?, evidence = ?,
			date = ?, start_time = ?, duration_min = ?, block_type = ?, lock_level = ?, lock_reason = ?,
			rows_json = ?, ops_json = ?, applied_op_ids = ?, accept_op_ids = ?,
			merge_key = ?, deliver_after = ?, delivered_at = ?, pushed_at = ?,
			ttl_policy = ?, expires_at = ?, resolution = ?, owner_instance = ?,
			rev = rev + 1, updated_at = ?
		 WHERE id = ? AND session_id = ? AND rev = ?`,
		string(p.State), p.Title, p.Summary, p.Reason, p.Evidence,
		p.Date, p.Start, nullInt(p.Dur), string(p.BType), string(p.LockLevel), p.LockReason,
		rowsJSON, opsJSON,
		marshalJSON(orEmptyStrings(p.AppliedOpIDs)), marshalJSON(orEmptyStrings(p.AcceptOpIDs)),
		p.MergeKey, nullMillis(p.DeliverAfter), nullMillis(p.DeliveredAt), nullMillis(p.PushedAt),
		string(p.TTLPolicy), toMillis(p.ExpiresAt), string(p.Resolution), p.OwnerInstance,
		now, p.ID, p.SessionID, p.Rev)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Either it is gone or somebody else wrote first. The caller re-reads
		// and decides; guessing which here would be guessing.
		return domain.ErrConflict
	}
	p.Rev++
	p.UpdatedAt = fromMillis(now)
	return nil
}

// Expire runs the two TTL policies as two statements, because they resolve
// differently: an act-first card lands accepted (the work was done and is still
// in the ledger), an ask-first one lands expired — nobody said no.
func (r proposalRepo) Expire(ctx context.Context, now time.Time) (int, error) {
	ms := toMillis(now)
	total := 0
	for _, c := range []struct {
		policy domain.ProposalTTL
		state  domain.ProposalState
	}{
		{domain.TTLSilenceAccepts, domain.ProposalAccepted},
		{domain.TTLSilenceRejects, domain.ProposalExpired},
	} {
		res, err := r.exec(ctx,
			`UPDATE proposals SET state = ?, resolution = ?, rev = rev + 1, updated_at = ?
			 WHERE state = ? AND ttl_policy = ? AND expires_at <= ?`,
			string(c.state), string(domain.ResolutionSilence), ms,
			string(domain.ProposalPending), string(c.policy), ms)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	return total, nil
}

func (r proposalRepo) Supersede(ctx context.Context, sessionID, mergeKey, keepID string) (int, error) {
	if mergeKey == "" {
		return 0, nil // no key, nothing to merge with
	}
	// Read the keeper's timestamp in its own statement. Referencing proposals
	// inside an UPDATE of proposals is ER_UPDATE_TABLE_USED (1093) on MySQL, and
	// the classic derived-table workaround does not survive MySQL 8 — derived_merge
	// is on by default, merges the derived table back into the enclosing
	// subquery, and re-exposes the target table. created_at is immutable, so
	// reading it separately opens no lost-update window and needs no transaction.
	var keepAt int64
	err := r.queryRow(ctx, `SELECT created_at FROM proposals WHERE session_id = ? AND id = ?`,
		sessionID, keepID).Scan(&keepAt)
	if errors.Is(err, sql.ErrNoRows) {
		// A keeper that is gone is nothing to merge against — the same non-event
		// as an empty merge key, and the same answer the Mongo side gives.
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	now := nowMillis()
	// Retire what is OLDER than the keeper, not merely "not the keeper". Two
	// daemons each calling this with their own id would otherwise annihilate each
	// other and leave the user with nothing. The order has to be total: on a
	// same-millisecond tie neither is older, so the tie breaks on id — arbitrary,
	// but identical from either caller's side.
	//
	// Only undelivered ones, either way. A card the user has already seen must
	// not vanish from under them because a newer one arrived: consensus 15
	// throttles delivery, it does not retract what was delivered.
	res, err := r.exec(ctx,
		`UPDATE proposals SET state = ?, resolution = ?, rev = rev + 1, updated_at = ?
		 WHERE session_id = ? AND merge_key = ? AND id <> ? AND state = ? AND delivered_at IS NULL
		   AND (created_at < ? OR (created_at = ? AND id < ?))`,
		string(domain.ProposalExpired), string(domain.ResolutionSuperseded), now,
		sessionID, mergeKey, keepID, string(domain.ProposalPending), keepAt, keepAt, keepID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanProposal(scan func(dest ...any) error) (*domain.Proposal, error) {
	var (
		p                                          domain.Proposal
		state, level, kind, btype, lockLevel       string
		ttlPolicy, resolution, origin              string
		rowsJSON, opsJSON, appliedJSON, acceptJSON string
		dur                                        sql.NullInt64
		deliverAfter, deliveredAt, pushedAt        sql.NullInt64
		expiresAt, createdAt, updatedAt            int64
	)
	err := scan(&p.ID, &p.SessionID, &state, &level, &kind, &p.Title, &p.Summary, &p.Reason, &p.Evidence,
		&p.Date, &p.Start, &dur, &btype, &lockLevel, &p.LockReason,
		&rowsJSON, &opsJSON, &appliedJSON, &acceptJSON,
		&p.MergeKey, &deliverAfter, &deliveredAt, &pushedAt,
		&ttlPolicy, &expiresAt, &resolution, &origin, &p.ThreadID, &p.OwnerInstance,
		&p.Rev, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.State = domain.ProposalState(state)
	p.Level = domain.ProposalLevel(level)
	p.Kind = domain.ProposalKind(kind)
	p.BType = domain.BlockType(btype)
	p.LockLevel = domain.LockLevel(lockLevel)
	p.TTLPolicy = domain.ProposalTTL(ttlPolicy)
	p.Resolution = domain.ProposalResolution(resolution)
	p.Origin = domain.ProposalOrigin(origin)
	if dur.Valid {
		d := int(dur.Int64)
		p.Dur = &d
	}
	if e := json.Unmarshal([]byte(rowsJSON), &p.Rows); e != nil {
		p.Rows = nil
	}
	if e := json.Unmarshal([]byte(opsJSON), &p.Ops); e != nil {
		p.Ops = nil
	}
	if e := json.Unmarshal([]byte(appliedJSON), &p.AppliedOpIDs); e != nil {
		p.AppliedOpIDs = nil
	}
	if e := json.Unmarshal([]byte(acceptJSON), &p.AcceptOpIDs); e != nil {
		p.AcceptOpIDs = nil
	}
	p.DeliverAfter = ptrMillis(deliverAfter)
	p.DeliveredAt = ptrMillis(deliveredAt)
	p.PushedAt = ptrMillis(pushedAt)
	p.ExpiresAt = fromMillis(expiresAt)
	p.CreatedAt = fromMillis(createdAt)
	p.UpdatedAt = fromMillis(updatedAt)
	return &p, nil
}

// The JSON columns are nullable everywhere (MySQL refuses a literal DEFAULT on
// TEXT, and a column that is nullable on one engine and not another is its own
// bug — see dialect_parity_test.go), so writes normalise nil to an empty
// collection and reads COALESCE. A round trip must never turn [] into null.
func orEmptyRows(v []domain.ProposalRow) []domain.ProposalRow {
	if v == nil {
		return []domain.ProposalRow{}
	}
	return v
}

func orEmptyOps(v []domain.ProposalOp) []domain.ProposalOp {
	if v == nil {
		return []domain.ProposalOp{}
	}
	return v
}

func orEmptyStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}
