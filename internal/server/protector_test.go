package server

import (
	"context"
	"testing"
	"time"

	"daycore/internal/domain"
)

func protectorServer(t *testing.T) (*Server, *Worker, string) {
	t.Helper()
	s, sid := newAgentTestServer(t)
	s.cfg.WorkerDefaultTZ = "UTC"
	s.renewWorkerLease(context.Background())
	w := NewWorker(s, nil)
	s.SetWorker(w)
	return s, w, sid
}

// awake marks a session as having been up since `since`, still active `lastAgo`
// ago — the two columns markAwake now writes.
func awakeFor(t *testing.T, s *Server, sid string, since time.Duration, lastAgo time.Duration) {
	t.Helper()
	now := time.Now()
	if err := s.store.Rhythm().Touch(context.Background(), sid,
		now.Add(-since), now.Add(-lastAgo)); err != nil {
		t.Fatal(err)
	}
}

func protectorCards(t *testing.T, s *Server, sid string) []domain.Proposal {
	t.Helper()
	all, err := s.store.Proposals().List(context.Background(), domain.ProposalFilter{SessionID: sid})
	if err != nil {
		t.Fatal(err)
	}
	var out []domain.Proposal
	for _, p := range all {
		if p.Origin == domain.OriginProtector {
			out = append(out, p)
		}
	}
	return out
}

// The input side is what makes the whole feature real.
//
// rhythm_profiles.run_since and last_signal_at had NO production writer: the day
// rows were written and the two live marks were not, so rhythm.Live.Run read a
// zero RunSince — which it correctly reads as "asleep" — and NeedsProtector was
// permanently false. Wiring the Protector without this gives a feature that
// compiles, whose tests pass on fabricated rows, and that never fires in
// production with no error and no log line.
func TestMarkAwakeWritesTheLiveMarks(t *testing.T) {
	s, _, sid := protectorServer(t)
	ctx := context.Background()

	s.markAwake(sid)
	if err := s.WaitBackground(ctx); err != nil {
		t.Fatal(err)
	}

	p, err := s.store.Rhythm().Get(ctx, sid)
	if err != nil {
		t.Fatalf("markAwake wrote no rhythm row at all: %v", err)
	}
	if p.RunSince.IsZero() || p.LastSignalAt.IsZero() {
		t.Fatalf("run_since / last_signal_at are still unwritten (%+v) — Live.Run reads that as asleep and the Protector can never fire", p)
	}

	// A second signal within IdleBreak continues the same stretch rather than
	// starting a new one — otherwise "how long have they been up" resets on
	// every interaction and never reaches twenty hours.
	first := p.RunSince
	s.awake.last = map[string]time.Time{} // clear the 5-minute throttle
	// The two live marks are millisecond-granular on every backend (SQL
	// toMillis, BSON datetime), so a second signal that lands in the same
	// millisecond is indistinguishable from a stale retry and dropped. Sleep
	// long enough to guarantee a distinct millisecond — the throttle map, not
	// the clock, is what this test is about.
	time.Sleep(2 * time.Millisecond)
	s.markAwake(sid)
	if err := s.WaitBackground(ctx); err != nil {
		t.Fatal(err)
	}
	p2, _ := s.store.Rhythm().Get(ctx, sid)
	if !p2.RunSince.Equal(first) {
		t.Errorf("the second signal restarted the stretch: %v → %v", first, p2.RunSince)
	}
	if !p2.LastSignalAt.After(p.LastSignalAt) {
		t.Error("the second signal did not move last_signal_at")
	}
}

func TestProtectorFiresAfterTwentyHours(t *testing.T) {
	s, w, sid := protectorServer(t)
	cfg := rhythmConfig()

	// The second signal must land in a DIFFERENT millisecond from the first.
	// Touch is millisecond-granular on all four backends (SQL toMillis, BSON
	// datetime) and compares strictly (<), so two signals whose timestamps
	// truncate to the same millisecond read as a stale retry and the second
	// one is dropped — run_since never advances and NeedsProtector stays
	// false. Different lastAgo values (1min → 1s) keep the two writes
	// distinguishable without sleeping, which would only make the test
	// machine-speed-dependent in the other direction.
	awakeFor(t, s, sid, cfg.ProtectAfter-time.Hour, time.Minute)
	w.checkProtector(sid, "UTC")
	if got := protectorCards(t, s, sid); len(got) != 0 {
		t.Fatalf("fired at %v, before the threshold: %+v", cfg.ProtectAfter-time.Hour, got)
	}

	awakeFor(t, s, sid, cfg.ProtectAfter+time.Hour, time.Second)
	w.checkProtector(sid, "UTC")
	cards := protectorCards(t, s, sid)
	if len(cards) != 1 {
		t.Fatalf("%d cards after 21 hours awake, want 1", len(cards))
	}
	if cards[0].TTLPolicy != domain.TTLSilenceRejects {
		t.Errorf("TTL policy = %q — silence at 4am is somebody concentrating or asleep, not consent to move their morning", cards[0].TTLPolicy)
	}
	if len(cards[0].Rows) != 2 {
		t.Errorf("card has %d rows, want an accept and a decline", len(cards[0].Rows))
	}
}

// One nudge per stretch of being awake. The occurrence key is the START of the
// run: a day key makes a 30-hour stretch that crosses local midnight fire twice,
// and a slot key makes it fire every half hour.
func TestProtectorFiresOncePerStretch(t *testing.T) {
	s, w, sid := protectorServer(t)
	cfg := rhythmConfig()
	awakeFor(t, s, sid, cfg.ProtectAfter+time.Hour, time.Minute)

	for i := 0; i < 4; i++ { // two hours of half-hourly checks
		w.checkProtector(sid, "UTC")
	}
	if got := protectorCards(t, s, sid); len(got) != 1 {
		t.Fatalf("%d cards for one stretch, want 1", len(got))
	}

	// A NEW stretch, after they slept, may nudge again.
	awakeFor(t, s, sid, cfg.ProtectAfter+time.Hour, time.Minute)
	now := time.Now()
	if err := s.store.Rhythm().Touch(context.Background(), sid,
		now.Add(-cfg.ProtectAfter-2*time.Hour), now); err != nil {
		t.Fatal(err)
	}
	w.checkProtector(sid, "UTC")
	if got := protectorCards(t, s, sid); len(got) != 2 {
		t.Errorf("%d cards after a second stretch, want 2", len(got))
	}
}

// Reading a stale last-signal as "still awake" would mean waking somebody up in
// order to tell them to sleep.
func TestProtectorTreatsALongSilenceAsSleep(t *testing.T) {
	s, w, sid := protectorServer(t)
	cfg := rhythmConfig()

	awakeFor(t, s, sid, cfg.ProtectAfter+5*time.Hour, cfg.IdleBreak+time.Hour)
	w.checkProtector(sid, "UTC")
	if got := protectorCards(t, s, sid); len(got) != 0 {
		t.Errorf("nudged somebody whose last signal was %v ago: %+v", cfg.IdleBreak+time.Hour, got)
	}
}

// Do Not Disturb suppresses the whole nudge, not just the push: a care card that
// appears silently at 4am and is read at noon says "you were up too long" about
// a fact from yesterday.
func TestProtectorRespectsDoNotDisturb(t *testing.T) {
	s, w, sid := protectorServer(t)
	cfg := rhythmConfig()
	awakeFor(t, s, sid, cfg.ProtectAfter+time.Hour, time.Minute)

	if rec := patchPrefs(t, s, sid, `{"doNotDisturb":true}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	w.checkProtector(sid, "UTC")
	if got := protectorCards(t, s, sid); len(got) != 0 {
		t.Errorf("nudged through Do Not Disturb: %+v", got)
	}
}

// A follower must not nudge. The occurrence row keeps it to one card even if two
// instances both believe they lead, but the lease is what stops N of them each
// building the card first.
func TestProtectorFollowerDoesNothing(t *testing.T) {
	s, _, sid := protectorServer(t)
	ctx := context.Background()
	cfg := rhythmConfig()
	awakeFor(t, s, sid, cfg.ProtectAfter+time.Hour, time.Minute)

	follower := &Server{store: s.store, cfg: s.cfg, log: discardLogger()}
	follower.renewWorkerLease(ctx)
	if follower.LeadsWorker() {
		t.Fatal("setup: the follower took the lease")
	}
	NewWorker(follower, nil).checkProtector(sid, "UTC")
	if got := protectorCards(t, s, sid); len(got) != 0 {
		t.Errorf("a follower created a card: %+v", got)
	}
}

// The budget is a hard ceiling on interruption (consensus 24: ≤3/day). The card
// still exists in the app when it is spent — the budget bounds pushes, not care.
func TestPushBudgetIsBoundedAndDerived(t *testing.T) {
	s, w, sid := protectorServer(t)
	ctx := context.Background()
	now := time.Now()
	loc := time.UTC

	if !w.spendPushBudget(ctx, sid, now, loc) {
		t.Fatal("no budget on a fresh day")
	}
	for i := 0; i < domain.PushBudgetPerDay; i++ {
		pushed := now
		p := &domain.Proposal{
			SessionID: sid, State: domain.ProposalPending, Level: domain.LevelL2,
			Kind: domain.KindCard, Title: "seed", TTLPolicy: domain.TTLSilenceRejects,
			ExpiresAt: now.Add(time.Hour), Origin: domain.OriginDaemon,
			PushedAt: &pushed,
		}
		if err := s.store.Proposals().Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if w.spendPushBudget(ctx, sid, now, loc) {
		t.Errorf("budget of %d was exceeded", domain.PushBudgetPerDay)
	}
	// Another session's pushes are not this session's budget.
	if _, err := s.store.Sessions().GetOrCreate(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	if !w.spendPushBudget(ctx, "other", now, loc) {
		t.Error("one session's pushes consumed another session's budget")
	}
}

// The card offers to move only what may be moved: Refishable carries the
// reschedule cap that stops the same thing being pushed forward forever, and a
// locked block is one the timetable decides.
func TestPostponeOffersOnlyMovableMorningBlocks(t *testing.T) {
	s, w, sid := protectorServer(t)
	ctx := context.Background()
	loc := time.UTC
	// Pick an instant early enough that "this morning" still has room.
	now := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 5, 0, 0, 0, loc)
	date := now.Format("2006-01-02")

	hhmm := func(v string) *string { return &v }
	blocks := []domain.TimeBlock{
		{ID: "movable", Title: "复习", Time: hhmm("09:00")},
		{ID: "locked", Title: "课", Time: hhmm("10:00"), LockLevel: domain.LockHard},
		{ID: "afternoon", Title: "下午的事", Time: hhmm("15:00")},
		{ID: "done", Title: "做完了", Time: hhmm("08:00"), Completed: true},
		{ID: "past", Title: "已经过去", Time: hhmm("04:00")},
	}
	if _, err := s.store.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID: sid, Date: date, SourceType: "manual", Blocks: blocks,
	}); err != nil {
		t.Fatal(err)
	}

	got := w.morningBlocksToPostpone(ctx, sid, now, loc)
	if len(got) != 1 || got[0].ID != "movable" {
		ids := make([]string, len(got))
		for i, b := range got {
			ids[i] = b.ID
		}
		t.Errorf("offered %v, want only [movable]", ids)
	}
}
