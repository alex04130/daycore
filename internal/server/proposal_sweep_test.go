package server

import (
	"context"
	"testing"
	"time"

	"daycore/internal/domain"
)

func sweepServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, sid := newAgentTestServer(t)
	s.renewWorkerLease(context.Background())
	return s, sid
}

func seedProposal(t *testing.T, s *Server, p *domain.Proposal) *domain.Proposal {
	t.Helper()
	if p.Title == "" {
		p.Title = "seed"
	}
	if p.Level == "" {
		p.Level = domain.LevelL2
	}
	if p.Kind == "" {
		p.Kind = domain.KindCard
	}
	if p.Origin == "" {
		p.Origin = domain.OriginDaemon
	}
	if p.TTLPolicy == "" {
		p.TTLPolicy = domain.TTLSilenceRejects
	}
	if p.ExpiresAt.IsZero() {
		p.ExpiresAt = time.Now().Add(time.Hour)
	}
	if p.State == "" {
		p.State = domain.ProposalPending
	}
	if err := s.store.Proposals().Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

// Expire, Supersede and Prune all had zero production callers. The symptom was
// invisible from outside — the stack query filters on expires_at, so the user
// saw the right cards the whole time. What it cost was the ledger: a lapsed card
// stayed `pending` with no Resolution, so "they never answered" and "we never
// asked" were the same row, and Prune (correctly) spares pending, so nothing was
// ever deleted and the table grew without bound.
func TestSweepExpiresLapsedProposals(t *testing.T) {
	s, sid := sweepServer(t)
	ctx := context.Background()

	lapsedAsk := seedProposal(t, s, &domain.Proposal{
		SessionID: sid, Title: "asked", TTLPolicy: domain.TTLSilenceRejects,
		ExpiresAt: time.Now().Add(-time.Minute),
	})
	lapsedAct := seedProposal(t, s, &domain.Proposal{
		SessionID: sid, Title: "acted", TTLPolicy: domain.TTLSilenceAccepts,
		// An act-first card must name what it already did — the store refuses one
		// that cannot say, because "silence closes the undo window" is meaningless
		// without something to undo.
		AppliedOpIDs: []string{"op-1"},
		ExpiresAt:    time.Now().Add(-time.Minute),
	})
	live := seedProposal(t, s, &domain.Proposal{SessionID: sid, Title: "still live"})

	n, err := s.store.Proposals().Expire(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expired %d, want 2", n)
	}

	// TTL asymmetry: an act-first card lands accepted (the work is already in the
	// ledger), an ask-first one lands expired — nobody said no.
	got, _ := s.store.Proposals().Get(ctx, sid, lapsedAsk.ID)
	if got.State != domain.ProposalExpired || got.Resolution != domain.ResolutionSilence {
		t.Errorf("ask-first lapsed to %s/%s, want expired/silence", got.State, got.Resolution)
	}
	got, _ = s.store.Proposals().Get(ctx, sid, lapsedAct.ID)
	if got.State != domain.ProposalAccepted || got.Resolution != domain.ResolutionSilence {
		t.Errorf("act-first lapsed to %s/%s, want accepted/silence", got.State, got.Resolution)
	}
	got, _ = s.store.Proposals().Get(ctx, sid, live.ID)
	if got.State != domain.ProposalPending {
		t.Errorf("a card that has not lapsed was expired: %s", got.State)
	}
}

// A decision card orphaned by a restart settles itself, and this is the test
// that keeps that true.
//
// Its TTL is the agent's own wait budget (45s sync, 90s async), so it is already
// lapsed by the time anything looks — the expiry pass settles it like any other
// lapse, and no cross-session sweep is needed. Give decision cards a long TTL
// and this stops holding: they would sit pending with a dead waiter, and
// ProposalFilter cannot express a cross-session sweep (session_id is
// unconditional, deliberately).
func TestOrphanedDecisionCardSettlesItself(t *testing.T) {
	s, sid := sweepServer(t)
	ctx := context.Background()

	// What persistDecision writes, for a turn whose process then died.
	orphan := seedProposal(t, s, &domain.Proposal{
		SessionID: sid, Title: "which one?", Kind: domain.KindDecision,
		Origin: domain.OriginAgent, ExpiresAt: time.Now().Add(-time.Second),
	})
	if orphan.Deliverable(time.Now()) {
		t.Fatal("an orphaned decision card is still deliverable — the user would be shown a card that unblocks nobody")
	}

	live := seedProposal(t, s, &domain.Proposal{
		SessionID: sid, Title: "a nudge", Kind: domain.KindCard, Origin: domain.OriginProtector,
	})

	if _, err := s.store.Proposals().Expire(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, _ := s.store.Proposals().Get(ctx, sid, orphan.ID)
	if got.State != domain.ProposalExpired || got.Resolution != domain.ResolutionSilence {
		t.Errorf("orphan settled to %s/%s, want expired/silence — nobody said no", got.State, got.Resolution)
	}
	// And the pool survives: expiring everything on restart would mean a deploy
	// silently cancelled every suggestion the user had not answered yet.
	got, _ = s.store.Proposals().Get(ctx, sid, live.ID)
	if got.State != domain.ProposalPending {
		t.Errorf("a card with time left was settled by the sweep: %s", got.State)
	}
}

// Only terminal rows may be pruned. A pending row older than the cutoff is one
// Expire has not reached, and deleting it would drop a card the user is owed.
func TestPruneKeepsWhatTheUserIsStillOwed(t *testing.T) {
	s, sid := sweepServer(t)
	ctx := context.Background()

	old := seedProposal(t, s, &domain.Proposal{SessionID: sid, Title: "answered long ago"})
	oldPending := seedProposal(t, s, &domain.Proposal{SessionID: sid, Title: "never answered"})

	old.State, old.Resolution = domain.ProposalAccepted, domain.ResolutionUser
	if err := s.store.Proposals().Update(ctx, old); err != nil {
		t.Fatal(err)
	}

	n, err := s.store.Proposals().Prune(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	if _, err := s.store.Proposals().Get(ctx, sid, oldPending.ID); err != nil {
		t.Error("a pending card was pruned; the user was still owed it")
	}
}

// The sweep is leader-gated. It is not claim-gated, because a cross-session
// sweep has no (session, job, run_key) and running it twice is idempotent — the
// lease is throttling, which is all that is needed when the work is naturally
// safe to repeat.
func TestProposalSweepIsLeaderGated(t *testing.T) {
	s, sid := sweepServer(t)
	ctx := context.Background()
	seedProposal(t, s, &domain.Proposal{
		SessionID: sid, Title: "lapsed", ExpiresAt: time.Now().Add(-time.Minute),
	})

	follower := &Server{store: s.store, cfg: s.cfg, log: discardLogger()}
	follower.renewWorkerLease(ctx)
	if follower.LeadsWorker() {
		t.Fatal("setup: the follower took the lease")
	}
	follower.sweepProposals(ctx)
	if got, _ := s.store.Proposals().List(ctx, domain.ProposalFilter{SessionID: sid}); len(got) != 1 || got[0].State != domain.ProposalPending {
		t.Errorf("a follower swept: %+v", got)
	}

	// And the leader does the work, so the gate is not just switching it off.
	s.sweepProposals(ctx)
	got, _ := s.store.Proposals().List(ctx, domain.ProposalFilter{SessionID: sid})
	if len(got) != 1 || got[0].State != domain.ProposalExpired {
		t.Errorf("the leader did not sweep: %+v", got)
	}
}

// A card made at four in the morning is not a card the user was shown at four in
// the morning. Queued means it appears on their next visit, and if it lapses
// before that it is voided having never been seen — which is honest, because the
// moment it was about has passed.
func TestQueuedProposalIsDeliveredOnTheNextVisit(t *testing.T) {
	s, sid := sweepServer(t)
	ctx := context.Background()

	queued := seedProposal(t, s, &domain.Proposal{
		SessionID: sid, Title: "要不要眯一会？", Origin: domain.OriginProtector,
	})
	if queued.DeliveredAt != nil {
		t.Fatal("the fixture was created already delivered")
	}
	// Not in the stack yet: the stack requires the stamp.
	stack, err := s.store.Proposals().List(ctx, domain.ProposalFilter{
		SessionID: sid, State: domain.ProposalPending,
		Delivered: domain.PresenceSet, DeliverableAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stack) != 0 {
		t.Fatalf("a queued card is already in the stack: %+v", stack)
	}

	s.deliverQueued(ctx, sid)

	got, _ := s.store.Proposals().Get(ctx, sid, queued.ID)
	if got.DeliveredAt == nil {
		t.Fatal("opening the app did not deliver the queued card")
	}
	// Idempotent: a second request must not re-stamp (which would move the
	// "when were you shown this" answer every time they refreshed).
	first := *got.DeliveredAt
	s.deliverQueued(ctx, sid)
	got, _ = s.store.Proposals().Get(ctx, sid, queued.ID)
	if !got.DeliveredAt.Equal(first) {
		t.Errorf("delivery stamp moved on a second visit: %v → %v", first, *got.DeliveredAt)
	}
}

// Queued and already lapsed: voided without ever having been shown.
func TestQueuedProposalThatLapsedIsNeverDelivered(t *testing.T) {
	s, sid := sweepServer(t)
	ctx := context.Background()

	lapsed := seedProposal(t, s, &domain.Proposal{
		SessionID: sid, Title: "昨天的事", Origin: domain.OriginProtector,
		ExpiresAt: time.Now().Add(-time.Minute),
	})

	s.deliverQueued(ctx, sid)

	got, _ := s.store.Proposals().Get(ctx, sid, lapsed.ID)
	if got.DeliveredAt != nil {
		t.Error("a card that lapsed before the user opened the app was still shown to them")
	}
}

// Only the newest of a merge group survives delivery. Superseding here rather
// than at creation is what lets a producer keep producing while the user is away
// without stacking N copies of the same nudge.
func TestDeliverySupersedesOlderCardsInTheSameGroup(t *testing.T) {
	s, sid := sweepServer(t)
	ctx := context.Background()

	older := seedProposal(t, s, &domain.Proposal{
		SessionID: sid, Title: "第一次", Origin: domain.OriginProtector, MergeKey: "protector:run:x",
	})
	// Force a distinct, later creation instant: Supersede breaks ties on
	// created_at, and two rows in the same millisecond would make this a
	// coin flip rather than a test.
	time.Sleep(2 * time.Millisecond)
	newer := seedProposal(t, s, &domain.Proposal{
		SessionID: sid, Title: "第二次", Origin: domain.OriginProtector, MergeKey: "protector:run:x",
	})

	s.deliverQueued(ctx, sid)

	gotOld, _ := s.store.Proposals().Get(ctx, sid, older.ID)
	gotNew, _ := s.store.Proposals().Get(ctx, sid, newer.ID)
	if gotNew.DeliveredAt == nil {
		t.Error("the newest card in the group was not delivered")
	}
	if gotOld.State == domain.ProposalPending && gotOld.DeliveredAt != nil {
		t.Errorf("both cards in one merge group were delivered: %s / %s", gotOld.Title, gotNew.Title)
	}
}
