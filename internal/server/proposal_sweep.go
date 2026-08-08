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

// # Delivery scheduling lives in proposal_delivery.go
//
// This file said, one commit ago, that a delivery scheduler was deliberately not
// built because nothing queued. That was true of every producer at the time and
// stopped being true the moment one fired on a clock instead of in response to
// something the user had just done — the Protector, at four in the morning.
//
// The rule that survives is the one about mechanisms with no callers. The
// conclusion drawn from it was premature, and it is recorded here rather than
// quietly deleted because "we decided not to build X" is exactly the kind of
// note that outlives its reason.
