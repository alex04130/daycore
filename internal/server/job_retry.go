package server

import (
	"context"
	"time"

	"daycore/internal/domain"
)

// Re-driving a failed occurrence.
//
// # ⚠️ The retry machinery existed; the caller did not
//
// `Claim` has always taken over a row whose status is `failed` and whose
// attempts are under the cap — that branch is in every backend and pinned by the
// conformance suite. So "retry a failed occurrence" is already spelled "call
// Claim again with the same run key". Nothing ever did.
//
// The consequence, concretely: a morning brief whose model call fails at 07:31
// writes `failed / attempts=1`, and the cron entry that produced it next fires
// tomorrow with a different day key. The row sits there until Prune deletes it
// two weeks later. **JobMaxAttempts describes what Claim permits, and two of the
// three attempts were never reachable by anything.** One failed brief was one
// morning with no brief.
//
// # ⚠️ Re-driving means RUNNING THE JOB AGAIN, not resending a stored result
//
// The job body re-reads preferences, re-checks Do Not Disturb, and rebuilds its
// content from the current plan. A sweeper that went straight to the send path
// with what the row remembers would skip every one of those — and the worst
// case is specific: somebody whose brief failed at 07:31, who then turned the
// brief off or turned Do Not Disturb on, gets it pushed at 09:00 anyway.
//
// So this dispatches back through the ordinary job body. The suppression gates
// are inside it, ahead of the claim, exactly where they already are.
//
// # ⚠️ Lateness is a per-job question, not a global one
//
// Re-driving without a bound turns a silent failure into a more annoying one: a
// brief that failed at 07:31 and arrives at 15:00 is a brief for a morning that
// is over. But `rhythm_learn` has no such problem — nobody can tell what time it
// ran — and re-driving it hours later is pure gain.
//
// Hence a per-job deadline rather than one number. A job with no entry here is
// not re-driven at all, which is the safe default: `rolling_replan`'s key is a
// slot that has moved on by design, `deadline_warning` and `protector` self-heal
// on their own cadence, and `theme_backfill` has its own sweeper.

// jobRetryEvery is how often the sweeper looks. Five minutes matches the other
// leader-gated sweeps.
const jobRetryEvery = 5 * time.Minute

// jobRetryBatch bounds one pass.
//
// ⚠️ Small on purpose. Each entry re-runs a real job — some of them cost a model
// call — so a backlog is worked through over several passes rather than in one
// burst that looks like a stampede to whatever is downstream.
const jobRetryBatch = 20

// retryDeadlines is how late each job may still be re-driven, measured from the
// original attempt.
//
// ⚠️ A job absent from this map is NEVER re-driven. Opting in per job is the
// direction that fails safe: a new job added later gets today's behaviour until
// somebody decides what "too late" means for it, rather than silently inheriting
// a number chosen for briefs.
var retryDeadlines = map[string]time.Duration{
	// Two hours. A morning brief is about the day ahead, and one that arrives
	// mid-afternoon is about a day that mostly happened.
	domain.JobMorningBrief: 2 * time.Hour,
	// Same shape, and the evening has the same problem in the other direction.
	domain.JobEveningReview: 2 * time.Hour,
	// Until the next day cut. Nothing about the learner is time-of-day visible —
	// it reads yesterday's rows and writes a profile — so the only real deadline
	// is "before the next one runs and makes this occurrence moot".
	domain.JobRhythmLearn: 20 * time.Hour,
	// The gap offer is for the day about to start; re-driving it during that day
	// is still useful, but not after it.
	domain.JobWishFill: 12 * time.Hour,
	// A habit is a claim about weeks. A day late changes nothing.
	domain.JobHabitScan: 20 * time.Hour,
}

// StartJobRetry re-drives failed occurrences until their attempts run out.
//
// ⚠️ Leader-gated, and it still goes through Claim. Unlike the proposal sweep —
// which is idempotent, so a second instance running it costs only work — a
// re-drive SENDS THINGS. The lease keeps two instances from sweeping at once;
// the row's claim is what keeps one sweep from racing the cron entry that owns
// the same occurrence.
func (s *Server) StartJobRetry() {
	// everyTick, not everyTickNow: unlike the proposal sweep there is no backlog
	// a restart creates. A row that failed before the restart is just as failed
	// five minutes later, and running immediately at boot would put a burst of
	// re-drives — some of them model calls — into the busiest moment a process
	// has.
	s.everyTick("job retry", jobRetryEvery, func(ctx context.Context) {
		if !s.LeadsWorker() {
			return
		}
		s.sweepJobRetries(ctx)
	})
}

// sweepJobRetries is the body, extracted so a test can drive it directly.
func (s *Server) sweepJobRetries(ctx context.Context) {
	if s.store == nil || s.worker == nil {
		return
	}
	now := time.Now()
	// ⚠️ The query carries `attempts < JobMaxAttempts` so exhausted rows stop
	// being selected. Without it, a row that failed three times would be picked
	// on every sweep for the fortnight it survives, and each pick would call
	// Claim only to be refused — harmless, and exactly the shape of "a column
	// recording that something failed seven times, that nobody reads".
	runs, err := s.store.JobRuns().ListRetryable(ctx, now.Add(-maxRetryWindow), jobRetryBatch)
	if err != nil {
		s.log.Warn("job retry: could not list retryable runs", "err", err)
		return
	}
	for _, run := range runs {
		deadline, ok := retryDeadlines[run.Job]
		if !ok {
			continue // not a job anybody decided to re-drive
		}
		if now.Sub(run.StartedAt) > deadline {
			// ⚠️ Logged rather than silently skipped: "it failed and was too late
			// to retry" is a different fact from "it failed", and an operator
			// looking at a missing brief should be able to tell them apart.
			s.log.Info("job retry: past its usefulness, leaving it failed",
				"job", run.Job, "session", run.SessionID, "age", now.Sub(run.StartedAt))
			continue
		}
		s.worker.redrive(run)
	}
}

// maxRetryWindow bounds how far back the query looks.
//
// ⚠️ Not the same as the deadlines above, and it is the cheaper guard: the
// deadlines are per-job policy, this keeps the QUERY from scanning a fortnight
// of rows on every sweep to find the handful that are still eligible. It has to
// be at least as long as the longest deadline or that policy becomes unreachable.
const maxRetryWindow = 24 * time.Hour

// redrive runs one failed occurrence's job body again.
//
// ⚠️ The dispatch table is names → job bodies, and it exists because cron
// entries are closures that captured (sid, tz) and are reachable only through
// the scheduler's own map. A row carries a session id and a job name, so getting
// from one to the other needs this.
//
// The timezone is re-resolved rather than remembered: somebody who moved should
// get their retry in the zone they are in now.
func (w *Worker) redrive(run domain.JobRun) {
	tz := w.s.sessionTimezone(context.Background(), run.SessionID)
	switch run.Job {
	case domain.JobMorningBrief:
		w.runBrief(run.SessionID, tz, "morning")
	case domain.JobEveningReview:
		w.runBrief(run.SessionID, tz, "evening")
	case domain.JobRhythmLearn:
		w.runRhythmLearn(run.SessionID, tz)
	case domain.JobWishFill:
		w.runWishFill(run.SessionID, tz)
	case domain.JobHabitScan:
		w.runHabitScan(run.SessionID, tz)
	default:
		// Unreachable while retryDeadlines is the gate, and left explicit: a job
		// added to that map without a case here would otherwise be selected,
		// dispatched to nothing, and look like a retry that ran.
		w.log.Error("job retry: no dispatch for job", "job", run.Job)
	}
}
