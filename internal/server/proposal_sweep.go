package server

import (
	"context"
	"time"
)

// The proposal lifecycle sweeps — the half of ζ½ that turns `proposals` from a
// table that only ever grows into one with a lifecycle.
//
// # What was missing
//
// Three repository methods had zero production callers: Expire, Supersede and
// Prune. The consequence was not visible from the outside, which is what made it
// dangerous:
//
//   - A lapsed card stayed `pending` forever. Prune only deletes terminal rows
//     (correctly — a pending row past the cutoff is one Expire has not reached,
//     and deleting it would drop a card the user was still owed), so nothing
//     ever deleted anything. The table grew without bound.
//   - Nobody noticed, because the stack query filters on expires_at: the user
//     saw the right cards the whole time. The only symptom was disk.
//   - `Resolution` was never written for a lapse, so "they never answered" and
//     "we never asked properly" were indistinguishable in the ledger — and
//     consensus 15 rests on being able to tell silence from a no.
//
// # Why this is leader-gated but not claim-gated
//
// These are cross-session sweeps, not per-session occurrences: there is no
// (session, job, run_key) to claim, and running one twice is idempotent — Expire
// moves rows that are still pending, Prune deletes rows that are still there.
// The lease is throttling, which is all that is needed when the work is
// naturally idempotent. Compare the briefs, where running twice sends two
// pushes and the occurrence row is doing real work.

// Sweep cadence and retention.
const (
	// proposalSweepEvery bounds how long a lapsed card sits in the pool before
	// its terminal state is written. It does not affect what the user sees —
	// Deliverable and the stack query both check expires_at directly, so an
	// expired card is invisible the instant it lapses whatever this is.
	proposalSweepEvery = 5 * time.Minute

	// ProposalRetention is how long a settled proposal is kept.
	//
	// Long enough to answer "what did it suggest last week and what did I say",
	// which is the footprint page's whole job. Pending rows are never pruned at
	// any age: one older than the cutoff is a card the user is still owed.
	ProposalRetention = 90 * 24 * time.Hour
)

// StartProposalSweep runs the expiry and retention passes.
func (s *Server) StartProposalSweep() {
	// everyTickNow, not everyTick: the first pass has to happen at boot. A
	// restart is exactly when there is a backlog — decision cards orphaned by the
	// process that died are already lapsed and waiting to be settled — and a
	// plain ticker would leave them pending for a whole interval.
	s.everyTickNow("proposal sweep", proposalSweepEvery, s.sweepProposals)
}

// sweepProposals is one pass. Separate from the loop so a test can run it
// directly rather than racing a ticker — the leader gate is the property worth
// asserting and it is invisible from outside otherwise.
func (s *Server) sweepProposals(parent context.Context) {
	if !s.LeadsWorker() {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	// Expiry first, then retention: a card that lapses in this pass becomes
	// eligible for pruning ProposalRetention later, not immediately, so the order
	// only matters for tidiness — but doing it the other way would mean a row
	// waits a whole extra sweep for no reason.
	if n, err := s.store.Proposals().Expire(ctx, time.Now()); err != nil {
		s.log.Warn("proposal expiry sweep failed", "err", err)
	} else if n > 0 {
		s.log.Info("proposals lapsed", "count", n)
	}
	if n, err := s.store.Proposals().Prune(ctx, time.Now().Add(-ProposalRetention)); err != nil {
		s.log.Warn("proposal prune failed", "err", err)
	} else if n > 0 {
		s.log.Info("pruned settled proposals", "count", n)
	}
}

// # Delivery scheduling is deliberately NOT built yet
//
// Proposal carries DeliverAfter and DeliveredAt, and consensus 15 describes a
// pool where generation is unthrottled and delivery is the gate. The scheduler
// that would move a queued card into the stack does not exist, and building it
// now would be the mistake this repo keeps making — a mechanism with no
// producer, written and tested and never called, which is what leases,
// job_runs, the file bus, internal/rhythm and four other things were.
//
// Nothing queues today: every producer stamps DeliveredAt at creation, because
// each is a response to something the user just did (a decision card in a turn,
// a conflict they asked about, a care nudge at hour twenty). A card that is on
// screen the moment it is made needs no scheduler.
//
// Build it when the first producer genuinely queues — the daemon chain
// (habit scan → Rule proposal, wish-pool gap filling) is the one EXPERIENCE_CORE
// §12.2 describes, and it produces cards nobody asked for at a moment nobody
// chose. That is when "when should this be shown" becomes a real question, and
// the answer will need the reordering, merging and back-pressure of consensus 15
// rather than a timestamp comparison.

// # Why there is NO special sweep for decision cards
//
// A KindDecision card is the only kind bound to a live goroutine: the agent turn
// that created it waits on a process-local channel. After a restart that channel
// is gone, so the obvious worry is a row that says pending forever while the
// client shows a card that unblocks nobody.
//
// It does not happen, and the reason is worth writing down because the fix for
// it was very nearly a new cross-session repository method on four backends:
//
//	persistDecision sets ExpiresAt to the agent's own wait budget — 45 seconds
//	synchronous, 90 asynchronous. An orphaned decision card is therefore ALREADY
//	LAPSED by the time anything could look at it.
//
// Lapsed means Deliverable() is false and the stack query excludes it, so the
// user never sees it; and the expiry pass below settles it to expired/silence
// like any other lapse. StartProposalSweep runs its first pass at boot rather
// than one interval in, so the window between a restart and that settlement is
// as close to zero as a sweep can make it.
//
// ⚠️ The thing that would break this: giving decision cards a long TTL. If a
// card is ever allowed to outlive the turn that made it, it stops being
// self-settling and this comment stops being true — that change needs a real
// cross-session sweep, and ProposalFilter cannot express one (session_id is
// unconditional in the filter builder, deliberately: "a filter never spans
// users"). It would have to be a new repository method, four backends and a
// conformance case.
