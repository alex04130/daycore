package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

// ── leases ──────────────────────────────────────────────────────────────────

type leaseRepo struct{ *Store }

// Acquire is deliberately UPDATE-first-then-INSERT with the whole condition
// inside the UPDATE. A SELECT to see whether the lease is free, followed by an
// UPDATE to take it, has a window between the two where a second instance can
// do the same — and this package has no transactions to close it with. One
// conditional statement is the only mutual exclusion available on all four
// backends.
func (r leaseRepo) Acquire(ctx context.Context, name, holder string, ttl time.Duration, now time.Time) (*domain.Lease, bool, error) {
	// A holder must identify one PROCESS. Two instances passing the same string
	// both match the renewal UPDATE, so both keep coming back true forever and
	// the fence never moves — the one failure it exists to catch becomes
	// undetectable. An empty holder is the degenerate case and the only one this
	// layer can see, so it is refused here; uniqueness itself is the caller's
	// contract (a per-process value, never configured — a pod restarting with the
	// same hostname must not be able to renew its predecessor's lease).
	if holder == "" {
		return nil, false, errors.New("lease: holder must be a per-process identifier, not empty")
	}
	nowMS := toMillis(now)
	expires := toMillis(now.Add(ttl))

	// Renewal: we already hold it and it has not lapsed. The fence does NOT
	// move — it counts hand-overs, not heartbeats, so a holder can use it to
	// tell "still mine" from "mine again after someone else had it".
	res, err := r.exec(ctx,
		`UPDATE leases SET expires_at = ? WHERE lease_name = ? AND holder = ? AND expires_at > ?`,
		expires, name, holder, nowMS)
	if err != nil {
		return nil, false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return r.confirm(ctx, name, holder, now)
	}

	// Take-over: unheld or expired. Bumping the fence in the same statement is
	// what makes it a hand-over rather than two writes that could interleave.
	res, err = r.exec(ctx,
		`UPDATE leases SET holder = ?, acquired_at = ?, expires_at = ?, fence = fence + 1
		 WHERE lease_name = ? AND expires_at <= ?`,
		holder, nowMS, expires, name, nowMS)
	if err != nil {
		return nil, false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return r.confirm(ctx, name, holder, now)
	}

	// First time this lease has ever been claimed. A concurrent insert loses on
	// the primary key, which is the right answer — it simply did not win.
	_, err = r.exec(ctx,
		`INSERT INTO leases (lease_name, holder, acquired_at, expires_at, fence) VALUES (?, ?, ?, ?, 1)`,
		name, holder, nowMS, expires)
	if err == nil {
		return r.confirm(ctx, name, holder, now)
	}
	// The insert failed. Distinguishing "somebody inserted first" from "the
	// database is unreachable" matters: swallowing both as a lost election means
	// an outage looks like an ordinary hand-over, the worker goes quiet, and
	// nothing anywhere says why. Reading the row back is dialect-free — a row
	// that now exists means we simply lost.
	if l, _, gerr := r.get(ctx, name); gerr == nil {
		return l, l.Held(holder, now), nil
	}
	return nil, false, err
}

// confirm re-reads the row after a statement claimed to have won it, and reports
// ok only if the row still names us.
//
// The post-image cannot come from the acquiring statement on every engine, so it
// comes from a follow-up SELECT — and in the gap between the two, an instance
// whose clock runs fast can see our fresh expires_at as already lapsed and take
// the lease. Returning the row unchecked would then hand US the row that says
// somebody ELSE is the holder, with THEIR fence, so we would record their fence
// as our own and conclude nothing had changed. That is the one failure the fence
// exists to catch (domain/coordination.go), so it cannot be defeated here.
func (r leaseRepo) confirm(ctx context.Context, name, holder string, now time.Time) (*domain.Lease, bool, error) {
	l, _, err := r.get(ctx, name)
	if err != nil {
		return nil, false, err
	}
	return l, l.Held(holder, now), nil
}

func (r leaseRepo) Release(ctx context.Context, name, holder string) error {
	// Expire it rather than delete it, so the fence survives. A deleted row
	// would restart the count and a stalled ex-holder could mistake a fresh 1
	// for the value it remembered.
	_, err := r.exec(ctx,
		`UPDATE leases SET expires_at = 0 WHERE lease_name = ? AND holder = ?`, name, holder)
	return err
}

func (r leaseRepo) Get(ctx context.Context, name string) (*domain.Lease, error) {
	l, _, err := r.get(ctx, name)
	return l, err
}

func (r leaseRepo) get(ctx context.Context, name string) (*domain.Lease, bool, error) {
	row := r.queryRow(ctx,
		`SELECT lease_name, holder, acquired_at, expires_at, fence FROM leases WHERE lease_name = ?`, name)
	var (
		l          domain.Lease
		acquiredAt int64
		expiresAt  int64
	)
	err := row.Scan(&l.Name, &l.Holder, &acquiredAt, &expiresAt, &l.Fence)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, domain.ErrNotFound
	}
	if err != nil {
		return nil, false, err
	}
	l.AcquiredAt = fromMillis(acquiredAt)
	l.ExpiresAt = fromMillis(expiresAt)
	return &l, true, nil
}

// ── job runs ────────────────────────────────────────────────────────────────

type jobRunRepo struct{ *Store }

// Claim races on the unique index over (session_id, job_name, run_key). The
// INSERT is the claim: it either succeeds, and this instance owns the
// occurrence, or it violates the index and somebody else already does.
//
// That ordering — write first, work second — is the entire mechanism. Recording
// the run afterwards would leave open exactly the window this exists to close.
func (r jobRunRepo) Claim(ctx context.Context, run *domain.JobRun) (bool, error) {
	// The id is a CLAIM TOKEN, not an identity the caller chooses, so it is
	// always freshly generated. The usual "caller-supplied id wins" convention
	// would make a reused struct collide on the primary key instead of on the
	// occurrence index — and Claim would then report "somebody else owns this"
	// for an occurrence nobody owns.
	run.ID = uuid.NewString()
	now := nowMillis()
	run.Status = domain.JobRunning
	run.StartedAt = fromMillis(now)
	if run.Attempts == 0 {
		run.Attempts = 1
	}

	_, err := r.exec(ctx,
		`INSERT INTO job_runs (id, session_id, job_name, run_key, status, instance, started_at, ended_at, attempts, error_text)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, NULL)`,
		run.ID, run.SessionID, run.Job, run.RunKey, string(run.Status), run.Instance, now, run.Attempts)
	if err == nil {
		return true, nil
	}

	// The insert lost. Two cases may be taken over:
	//
	//   - a claim stuck in "running" long past when its owner could still be
	//     working (JobStaleAfter is twenty times the longest job timeout), i.e.
	//     the instance holding it died mid-flight
	//   - a "failed" attempt with retries left, so one transient error does not
	//     cost the user the whole day's brief
	//
	// Rotating the row's id to OUR id in the same statement is what makes the
	// takeover safe: the previous owner, if it wakes up after all, calls
	// Finish with an id that no longer matches anything and its late write is a
	// harmless no-op rather than a verdict on our run.
	stale := toMillis(fromMillis(now).Add(-domain.JobStaleAfter))
	res, uerr := r.exec(ctx,
		`UPDATE job_runs SET id = ?, status = ?, instance = ?, started_at = ?, ended_at = NULL,
			attempts = attempts + 1, error_text = NULL
		 WHERE session_id = ? AND job_name = ? AND run_key = ?
		   AND ((status = ? AND started_at < ? AND attempts < ?)
			 OR (status = ? AND attempts < ?))`,
		run.ID, string(domain.JobRunning), run.Instance, now,
		run.SessionID, run.Job, run.RunKey,
		string(domain.JobRunning), stale, domain.JobMaxCrashAttempts,
		string(domain.JobFailed), domain.JobMaxAttempts)
	if uerr != nil {
		return false, uerr
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Neither the insert nor the takeover landed. That is normally "somebody
		// else owns this occurrence" — but it is also what a transient write
		// failure looks like, and reporting an outage as a lost race leaves the
		// worker quiet with nothing to explain it. A row for the occurrence means
		// we genuinely lost; no row means the insert error was real.
		var one int
		qerr := r.queryRow(ctx,
			`SELECT 1 FROM job_runs WHERE session_id = ? AND job_name = ? AND run_key = ?`,
			run.SessionID, run.Job, run.RunKey).Scan(&one)
		if errors.Is(qerr, sql.ErrNoRows) {
			return false, err
		}
		return false, nil
	}
	// The UPDATE incremented attempts in the database; read it back so the
	// caller's struct agrees with the row. Leaving it at 1 would tell a caller
	// that checks "was this the last try?" the wrong thing forever, and the
	// Mongo implementation returns the real count — two backends answering the
	// same call differently is worse than either answer.
	if err := r.queryRow(ctx, `SELECT attempts FROM job_runs WHERE id = ?`, run.ID).Scan(&run.Attempts); err != nil {
		return true, nil // we hold it; the count is cosmetic next to that
	}
	return true, nil
}

func (r jobRunRepo) Finish(ctx context.Context, id string, status domain.JobStatus, errText string, at time.Time) error {
	_, err := r.exec(ctx,
		`UPDATE job_runs SET status = ?, ended_at = ?, error_text = ? WHERE id = ?`,
		string(status), toMillis(at), errText, id)
	return err
}

func (r jobRunRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.JobRun, error) {
	rows, err := r.query(ctx,
		`SELECT id, session_id, job_name, run_key, status, instance, started_at, ended_at, attempts,
			COALESCE(error_text, '')
		 FROM job_runs WHERE session_id = ? ORDER BY started_at DESC`+limitClause(limit, 50, 500), sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.JobRun{}
	for rows.Next() {
		var (
			j         domain.JobRun
			status    string
			startedAt int64
			endedAt   sql.NullInt64
		)
		if err := rows.Scan(&j.ID, &j.SessionID, &j.Job, &j.RunKey, &status, &j.Instance,
			&startedAt, &endedAt, &j.Attempts, &j.Error); err != nil {
			return nil, err
		}
		j.Status = domain.JobStatus(status)
		j.StartedAt = fromMillis(startedAt)
		j.EndedAt = ptrMillis(endedAt)
		out = append(out, j)
	}
	return out, rows.Err()
}

// Prune only touches finished rows. A "running" row older than the cutoff is
// evidence of a crash, and deleting it would erase the only trace of a job that
// never came back.
func (r jobRunRepo) Prune(ctx context.Context, before time.Time) (int, error) {
	res, err := r.exec(ctx,
		`DELETE FROM job_runs WHERE status <> ? AND started_at < ?`,
		string(domain.JobRunning), toMillis(before))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
