package server

import (
	"context"
	"time"

	"daycore/internal/domain"
)

// Delivery: turning a queued proposal into one the user is actually shown.
//
// # The distinction, and why it is not bookkeeping
//
// Generation is unthrottled and delivery is the gate (consensus 15). A proposal
// exists from the moment something decided to make it; it becomes something the
// user has been SHOWN only when DeliveredAt is stamped, and the stack query
// requires that stamp. Those are two different facts and the gap between them is
// where the product's manners live.
//
// The Protector is the case that makes it concrete. It fires at four in the
// morning, because that is when somebody has been up for twenty hours. Stamping
// the card delivered right then means it is "shown" at 4am — read at noon, when
// it is a remark about yesterday. Queued instead, it waits for the user to
// actually open the app, and if it lapses before they do, it is voided having
// never been shown. Which is the honest outcome: the moment it was about has
// passed.
//
// ⚠️ This corrects a judgement made one commit earlier. proposal_sweep.go said
// delivery scheduling was deliberately not built because "nothing queues today"
// — true of every producer at the time, and wrong the moment a producer fires on
// a clock rather than in response to something the user just did. The Protector
// is that producer. The rule that survives is the one about mechanisms with no
// callers; the conclusion drawn from it was premature.
//
// # What still delivers at creation, and why
//
//	decision card   the agent turn is blocked on it right now
//	conflict card   the user asked, in this request, about that clash
//
// Both are responses to something happening at that instant. A card that is on
// screen because the user just did something needs no queue.
//
// # Channel pushes are a different medium and are NOT gated on this
//
// Delivery is about what appears in the app. A push to QQ is how somebody who is
// not in the app finds out at all, so it goes out when the card is made. For the
// Protector that is defensible on its own terms: the premise of the nudge is
// that this person is demonstrably awake and using something.

// deliverQueued stamps deliverable, undelivered proposals as delivered.
//
// Called from the awake path, so "the user opened the app" is the trigger — the
// same admission gate the rhythm signals ride, and for the same reason: an
// authenticated data request from a client IS the app being open, while the
// extension's import path authenticates differently and correctly does not
// count.
//
// Best-effort. A card that fails to deliver stays queued and is picked up on the
// next request, which is a better failure than a request that errors because a
// suggestion could not be shown.
func (s *Server) deliverQueued(ctx context.Context, sid string) {
	if s == nil || s.store == nil || sid == "" {
		return
	}
	now := time.Now()
	queued, err := s.store.Proposals().List(ctx, domain.ProposalFilter{
		SessionID: sid,
		State:     domain.ProposalPending,
		// Undelivered AND still deliverable: DeliverableAt additionally excludes
		// what has lapsed in the pool and what is held back for later. Asking for
		// "undelivered" alone would hand the user cards that expired unseen —
		// exactly the ones being queued is supposed to void.
		Delivered:     domain.PresenceUnset,
		DeliverableAt: now,
	})
	if err != nil {
		s.log.Debug("proposal delivery: could not read the queue", "sid", sid, "err", err)
		return
	}
	for i := range queued {
		p := queued[i]
		// Only the newest of a merge group is shown. Superseding at delivery
		// rather than at creation is what lets a producer keep producing while
		// the user is away without stacking up N copies of the same nudge — and
		// it is why Supersede spares delivered cards: retiring one already on
		// screen would make it vanish mid-read.
		if p.MergeKey != "" {
			if _, err := s.store.Proposals().Supersede(ctx, sid, p.MergeKey, p.ID); err != nil {
				s.log.Debug("proposal delivery: supersede failed", "id", p.ID, "err", err)
			}
		}
		p.DeliveredAt = &now
		if err := s.store.Proposals().Update(ctx, &p); err != nil {
			// A conflict means somebody settled it in between; anything else is
			// worth a line but not worth abandoning the rest.
			s.log.Debug("proposal delivery: could not stamp", "id", p.ID, "err", err)
			continue
		}
		s.log.Debug("proposal delivered", "sid", sid, "id", p.ID, "origin", p.Origin)
	}
}
