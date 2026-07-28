package domain

import (
	"context"
	"time"
)

// This file holds the entities that make more than one instance of the server
// safe to run: a lease so only one of them does the background work, and a
// record of each job occurrence so that even if two of them try, the work
// happens once.
//
// The two are not redundant. The job record is the correctness mechanism — a
// unique index is the only mutual exclusion all four storage backends share,
// and sqlstore has no transactions at all. The lease is a throttle on top: it
// stops N instances from each waking up, building context and racing for a
// claim that only one can win. Correctness must not rest on the lease, because
// a lease rests on clocks, and clocks on different machines disagree.

// ── leases ──────────────────────────────────────────────────────────────────

// A Lease is a named, expiring claim. Acquisition is a single conditional
// UPDATE — never SELECT-then-UPDATE, which has a window between the read and
// the write where a second instance can win the same lease.
type Lease struct {
	// Name is the resource, not the holder: "worker" is a lease, an instance id
	// is not.
	Name       string    `json:"name"`
	Holder     string    `json:"holder"`
	AcquiredAt time.Time `json:"acquiredAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	// Fence rises by one on every acquisition, never on a renewal. A holder
	// that stalls past its expiry and comes back can compare the fence it
	// remembers with the current one and discover it is no longer the leader —
	// which a timestamp cannot tell it, because its own clock is exactly what
	// it cannot trust.
	Fence int64 `json:"fence"`
}

// LeaseWorker is the one lease that exists today: the right to run the
// background worker's scheduled jobs. More names can be added without a
// migration; names that nothing claims should not be invented in advance.
const LeaseWorker = "worker"

// Lease timing. The renewal interval is a third of the TTL so a single missed
// renewal — one slow query, one GC pause — does not hand the lease away.
const (
	LeaseTTL     = 60 * time.Second
	LeaseRenewIn = 20 * time.Second
)

// Held reports whether the lease is currently ours. It takes now explicitly
// because the caller's clock is the one that matters and pretending otherwise
// hides the assumption.
func (l Lease) Held(by string, now time.Time) bool {
	return l.Holder == by && l.ExpiresAt.After(now)
}

type LeaseRepository interface {
	// Acquire takes or renews the lease and reports whether we now hold it.
	//
	// It must be one conditional statement: take the row when it is unheld or
	// expired, or when we already hold it. Two instances calling this at the
	// same instant must not both come back true, and with no transactions
	// available that means the condition and the write have to be the same
	// statement.
	Acquire(ctx context.Context, name, holder string, ttl time.Duration, now time.Time) (*Lease, bool, error)
	// Release gives the lease up early — one UPDATE in the shutdown path, which
	// turns a full TTL of nothing happening into nothing at all.
	Release(ctx context.Context, name, holder string) error
	Get(ctx context.Context, name string) (*Lease, error)
}

// ── job runs ────────────────────────────────────────────────────────────────

// JobRun records one occurrence of one scheduled job for one session.
//
// The row is written BEFORE the work, not after. That ordering is the whole
// point: (session, job, run_key) is unique, so the insert either succeeds — and
// this instance owns the occurrence — or fails on the duplicate key, meaning
// somebody else already has it. Recording afterwards would leave the window it
// exists to close.
type JobRun struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Job       string `json:"job"`
	// RunKey names the occurrence: the local date for a daily job, date plus
	// slot for one that repeats within a day. It is a natural key rather than a
	// timestamp so that a job which moves — the morning brief follows a learned
	// wake time — is still recognisably the same occurrence.
	RunKey    string     `json:"runKey"`
	Status    JobStatus  `json:"status"`
	Instance  string     `json:"instance"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	Attempts  int        `json:"attempts"`
	Error     string     `json:"error,omitempty"`
}

type JobStatus string

const (
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobFailed  JobStatus = "failed"
)

// Job names.
const (
	JobMorningBrief  = "morning_brief"
	JobEveningReview = "evening_review"
	JobDeadlineWarn  = "deadline_warning"
	JobRollingReplan = "rolling_replan"
	JobAutoPlan      = "auto_plan"
	JobRhythmLearn   = "rhythm_learn"
	JobProtector     = "protector"
)

// JobStaleAfter is how long a row may sit in "running" before another instance
// may take the occurrence over. It has to clear the job's own context timeout
// with room to spare — those are 15 to 30 seconds — so ten minutes is roughly
// twenty times the longest, which is the right side to err on: stealing early
// means doing the work twice, and doing a morning brief twice is a push the
// user did not want.
const JobStaleAfter = 10 * time.Minute

// JobMaxAttempts bounds retries of a failed occurrence.
//
// Without a retry at all, one failed morning brief means no brief for the rest
// of the day — the next tick sees a row for today's occurrence and steps aside,
// unable to tell "somebody is working on it" from "it already went wrong".
// Without a bound, a job that fails every time (a revoked API key, say) retries
// on every tick and turns a broken integration into a stream of noise.
//
// Three is enough to ride out a transient failure and few enough that a
// permanent one stops being a story after a minute.
const JobMaxAttempts = 3

// JobMaxCrashAttempts bounds takeovers of an occurrence stuck in "running".
//
// A job that fails returns an error and is capped at JobMaxAttempts. A job that
// kills its instance — OOM on a large context, say — returns nothing, leaves the
// row "running", and gets taken over ten minutes later by the next instance,
// which OOMs the same way. Without a bound here that loop never ends, and the
// attempts column sits there recording that it has happened seven times while
// nothing reads it.
//
// Higher than JobMaxAttempts because a crash is likelier to be circumstantial
// than a returned error is, and because the ten-minute wait already throttles it
// hard.
const JobMaxCrashAttempts = 6

type JobRunRepository interface {
	// Claim tries to take ownership of one occurrence. ok is false when someone
	// else already holds it and their claim is neither finished nor stale, and
	// when the occurrence has used up its attempts (JobMaxAttempts after a
	// returned error, JobMaxCrashAttempts after a claim that never came back).
	//
	// On success run.ID and run.Attempts reflect the row, including after a
	// takeover — a caller that decides "this was the last try, tell the user the
	// integration is broken" reads Attempts, so it has to be the row's value and
	// not the one the caller passed in.
	//
	// ⚠️ Only the lease holder should call this. Staleness compares started_at,
	// written by the previous claimant's clock, against the caller's own — so a
	// caller whose clock runs more than JobStaleAfter fast can steal a claim that
	// is milliseconds old and run the occurrence twice. The lease is what keeps
	// two instances from reaching here at once; hosts still have to be within
	// JobStaleAfter of each other.
	//
	// A suppressed job — the user turned that toggle off, or Do Not Disturb is
	// on — should never reach here. Writing a row every half hour to record
	// that nothing happened would bury the rows that mean something.
	Claim(ctx context.Context, run *JobRun) (bool, error)
	// Finish closes a claimed occurrence. errText is empty on success.
	Finish(ctx context.Context, id string, status JobStatus, errText string, at time.Time) error
	// List returns a session's recent runs, newest first — the console's
	// "did the morning brief actually go out" view.
	List(ctx context.Context, sessionID string, limit int) ([]JobRun, error)
	// Prune deletes finished runs older than before, and is what keeps this
	// table from growing without bound (it takes the most rows of the six).
	Prune(ctx context.Context, before time.Time) (int, error)
}
