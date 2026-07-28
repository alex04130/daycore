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
		return r.get(ctx, name)
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
		return r.get(ctx, name)
	}

	// First time this lease has ever been claimed. A concurrent insert loses on
	// the primary key, which is the right answer — it simply did not win.
	_, err = r.exec(ctx,
		`INSERT INTO leases (lease_name, holder, acquired_at, expires_at, fence) VALUES (?, ?, ?, ?, 1)`,
		name, holder, nowMS, expires)
	if err != nil {
		// Somebody inserted first, or it is held and unexpired. Either way we
		// do not have it; report that rather than the driver's error, which
		// differs per engine and says nothing useful.
		return nil, false, nil
	}
	return r.get(ctx, name)
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
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
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
		   AND ((status = ? AND started_at < ?) OR (status = ? AND attempts < ?))`,
		run.ID, string(domain.JobRunning), run.Instance, now,
		run.SessionID, run.Job, run.RunKey,
		string(domain.JobRunning), stale,
		string(domain.JobFailed), domain.JobMaxAttempts)
	if uerr != nil {
		return false, uerr
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	return false, nil
}

func (r jobRunRepo) Finish(ctx context.Context, id string, status domain.JobStatus, errText string, at time.Time) error {
	_, err := r.exec(ctx,
		`UPDATE job_runs SET status = ?, ended_at = ?, error_text = ? WHERE id = ?`,
		string(status), toMillis(at), errText, id)
	return err
}

func (r jobRunRepo) List(ctx context.Context, sessionID string, limit int) ([]domain.JobRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.query(ctx,
		`SELECT id, session_id, job_name, run_key, status, instance, started_at, ended_at, attempts,
			COALESCE(error_text, '')
		 FROM job_runs WHERE session_id = ? ORDER BY started_at DESC LIMIT `+itoa(limit), sessionID)
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
