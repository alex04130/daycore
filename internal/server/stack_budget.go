package server

import (
	"context"

	"daycore/internal/domain"
)

// 「L2 同时最多堆叠 3 条」, enforced.
//
// # ⚠️ It cost nothing until today, and that is why it was never written
//
// The ceiling has been in EXPERIENCE_CORE since the start and had no
// implementation, because there was exactly one producer of queued cards (the
// Protector) and it merges on its own run key — one card, never a stack. Three
// daemon producers landing in the same batch is the first time the number is
// load-bearing: a reader who has not opened the app for a day could otherwise
// come back to a gap suggestion, a habit offer, and two clash cards, all at
// once, on a morning they were already behind.
//
// The failure this prevents is not clutter. It is that a pile of suggestions
// reads as a list of things you have failed to do — which is the one thing the
// product's 底色 forbids.
//
// # ⚠️ Why it is checked at PRODUCTION and not at delivery
//
// Filtering at delivery would leave the extra cards in the table, pending and
// invisible, to surface later out of context — a suggestion about last Tuesday
// arriving on Thursday. Refusing to create them means the ones that exist are
// the ones somebody will see.
//
// # ⚠️ Why this is not the push budget
//
// domain.PushBudgetWindow counts L3 PUSHES per day — how often the assistant is
// allowed to interrupt. This counts L2 cards WAITING at once — how much can be
// asked of somebody in one sitting. A day may legitimately hold several pushes
// and still only ever show three cards, and a quiet day may show three cards
// with no push at all.
//
// # ⚠️ The known race, stated
//
// This is a read followed by a write, with nothing between them. Two producers
// on two instances can both see two cards and both create a third. That is
// exactly the shape domain/proposal.go warns about for the push budget, and the
// remedy it names is the same: an insert-or-fail claim like job_runs, not a
// count.
//
// It is accepted here for now because the two cheap mitigations already hold —
// every producer claims its own occurrence first (so the same producer cannot
// race itself), and the producers run at different times of day (the day cut,
// the quiet hour, and :07 past). The overshoot available is one card, and the
// cost of one extra card is not the cost this ceiling exists to prevent.
// ⬜ Replace with a claim when a fourth producer lands.

// stackCeiling is how many L2 cards may be waiting at once.
const stackCeiling = 3

// stackHasRoom reports whether another L2 card may be created for this session.
//
// ⚠️ Counts PENDING cards, delivered or not. A queued card is one the reader has
// not answered and will be shown; counting only delivered ones would let a night
// of producers queue five and call it room.
//
// ⚠️ Unknown means NO. A count that failed is not evidence of space, and the
// direction of the guess matters: guessing "there is room" adds to a pile the
// ceiling exists to prevent, while guessing "there is not" costs one suggestion
// that will be offered again tomorrow.
func (s *Server) stackHasRoom(ctx context.Context, sid string) bool {
	if s.store == nil {
		return false
	}
	n, err := s.store.Proposals().Count(ctx, domain.ProposalFilter{
		SessionID: sid,
		State:     domain.ProposalPending,
		Level:     domain.LevelL2,
	})
	if err != nil {
		s.log.Warn("stack budget: could not count pending cards; holding back", "session", sid, "err", err)
		return false
	}
	return n < stackCeiling
}
