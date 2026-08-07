package server

import (
	"context"
	"os"
	"sync"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

// Leader election for the background worker.
//
// # Why two mechanisms and not one
//
// The lease elects one instance to run the scheduled jobs. The job-run row
// (domain.JobRun) records one occurrence of one job for one session and is the
// thing that actually makes "the morning brief goes out once" true. They are
// not redundant, and it matters which one carries which promise:
//
//	the job row   correctness. A unique index on (session, job, run_key) is the
//	              only mutual exclusion all four backends share, and sqlstore has
//	              no transactions at all.
//	the lease     throttling. It stops N instances from each waking up, building
//	              context, calling a model, and then having N−1 of them discover
//	              they lost.
//
// Correctness must not rest on the lease, because a lease rests on clocks and
// clocks on different machines disagree. Anything that would be wrong if two
// instances both believed they held the lease has to be defended by the row.
//
// # What the fence is and is not used for
//
// Lease.Fence rises on every acquisition and never on a renewal, so a holder
// that stalls past its expiry can tell "still mine" from "mine again after
// somebody else had it". We record it and log handovers. We deliberately do NOT
// stamp writes with it: the occurrence row already rotates its claim id on
// takeover, so a zombie's late Finish lands on nothing, and adding a second,
// weaker guard on top of a working one is cargo cult rather than defence.
//
// # What this does not give you
//
// Retries. domain.JobMaxAttempts and JobMaxCrashAttempts describe what Claim
// would allow, but nothing re-drives a failed occurrence: each job is triggered
// by exactly one cron firing, and by the next firing the run key has moved on.
// A morning brief that fails is a morning with no brief, which is what the
// domain comment already warned about. Fixing it needs a sweeper that re-drives
// failed rows, and that is a separate piece of work with its own failure modes
// (a job that fails at 07:31 and is retried at 09:00 is a brief arriving at a
// time the user did not ask for).

// workerLease is the process's view of whether it currently leads.
type workerLease struct {
	mu        sync.Mutex
	holdUntil time.Time
	fence     int64
	// everHeld distinguishes "we have never held it" from "we lost it", which
	// is the difference between an ordinary startup and a handover worth logging.
	everHeld bool
}

// InstanceID identifies this process for leases and job claims.
//
// Generated in-process, never from configuration. A restarted pod that kept its
// name would otherwise inherit its own previous lease and its own previous
// claims — the holder string has to mean "this process", not "this deployment
// slot", or the whole mechanism silently becomes a no-op across restarts.
func (s *Server) InstanceID() string {
	s.instanceOnce.Do(func() {
		host, err := os.Hostname()
		if err != nil || host == "" {
			host = "unknown"
		}
		s.instanceID = host + "/" + uuid.NewString()
	})
	return s.instanceID
}

// StartWorkerLease begins renewing the worker lease, starting immediately.
//
// everyTickNow rather than everyTick: a ticker's first tick lands one whole
// interval in, so a plain loop would leave every restart with no leader for
// LeaseRenewIn — twenty seconds in which a single-instance deployment does
// nothing at all.
func (s *Server) StartWorkerLease() {
	s.everyTickNow("worker lease", domain.LeaseRenewIn, s.renewWorkerLease)
}

// renewWorkerLease is one renewal attempt. Three outcomes, and they must stay
// three: collapsing the last two is how a deployment ends up with two leaders.
func (s *Server) renewWorkerLease(parent context.Context) {
	if s.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	l, ok, err := s.store.Leases().Acquire(ctx, domain.LeaseWorker, s.InstanceID(), domain.LeaseTTL, time.Now())
	switch {
	case err != nil:
		// Keep whatever hold we had. LeaseRenewIn is a third of LeaseTTL exactly
		// so that one slow query or one GC pause does not hand the lease away —
		// dropping it here would make a flaky database into a leadership
		// flip-flop, which is worse than a stale leader for one more interval.
		s.log.Warn("worker lease renewal failed; keeping the current hold until it expires on its own",
			"err", err)
	case !ok:
		// We know we lost. Stop leading now rather than at expiry: this is the
		// normal state for N−1 instances, so it is Debug, not Warn.
		s.lease.mu.Lock()
		wasLeader := s.lease.holdUntil.After(time.Now())
		s.lease.holdUntil = time.Time{}
		s.lease.mu.Unlock()
		if wasLeader {
			s.log.Info("lost the worker lease; this instance stops running scheduled jobs")
		} else {
			s.log.Debug("worker lease is held elsewhere")
		}
	default:
		s.lease.mu.Lock()
		handover := s.lease.everHeld && l.Fence != s.lease.fence
		s.lease.holdUntil = l.ExpiresAt
		s.lease.fence = l.Fence
		s.lease.everHeld = true
		s.lease.mu.Unlock()
		if handover {
			// The fence moved while we thought we held it: somebody else had the
			// lease in between. Nothing to repair — job ownership is per
			// occurrence — but it is the only visible sign of a split.
			s.log.Info("took the worker lease after a handover", "fence", l.Fence)
		}
	}
}

// LeadsWorker reports whether this instance may run scheduled jobs.
//
// A deployment with no lease repository behaves as a leader: the single-instance
// case must not be switched off by a mechanism that exists for the multi-instance
// one.
func (s *Server) LeadsWorker() bool {
	if s == nil || s.store == nil {
		return true
	}
	s.lease.mu.Lock()
	defer s.lease.mu.Unlock()
	return s.lease.holdUntil.After(time.Now())
}

// ReleaseWorkerLease gives the lease up on the way out, turning a full TTL of
// nothing happening into nothing at all.
//
// Ordering matters and is easy to get backwards: StopTicks has to run FIRST, or
// the renewal loop still in flight will re-acquire what this just released and
// the next instance waits a whole TTL for a leader that has already exited.
func (s *Server) ReleaseWorkerLease() {
	if s == nil || s.store == nil {
		return
	}
	s.lease.mu.Lock()
	held := s.lease.holdUntil.After(time.Now())
	s.lease.holdUntil = time.Time{}
	s.lease.mu.Unlock()
	if !held {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.store.Leases().Release(ctx, domain.LeaseWorker, s.InstanceID()); err != nil {
		s.log.Warn("could not release the worker lease; the next leader waits out its TTL", "err", err)
	}
}

// StartJobRunPrune keeps the job_runs table from growing without bound. It takes
// the most rows of the six coordination tables — one per session per job per
// occurrence — and nothing else deletes from it.
//
// Only the leader prunes, for the same reason only the leader runs jobs: N
// instances deleting the same rows is N−1 wasted round trips, not an error.
func (s *Server) StartJobRunPrune() {
	s.everyTick("job run prune", JobRunPruneEvery, func(parent context.Context) {
		if !s.LeadsWorker() {
			return
		}
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		n, err := s.store.JobRuns().Prune(ctx, time.Now().Add(-JobRunRetention))
		if err != nil {
			s.log.Warn("job run prune failed", "err", err)
			return
		}
		if n > 0 {
			s.log.Info("pruned finished job runs", "count", n)
		}
	})
}

// Retention for job_runs.
//
// Two weeks because the question this table answers is "did my morning brief
// actually go out, and if not why" — asked days later, about a specific morning,
// and never about last month. Running rows are never pruned (the store refuses):
// a row stuck in "running" is the only trace a crash leaves.
const (
	JobRunRetention  = 14 * 24 * time.Hour
	JobRunPruneEvery = 6 * time.Hour
)
