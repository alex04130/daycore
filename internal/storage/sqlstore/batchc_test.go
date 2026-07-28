package sqlstore

import (
	"context"
	"strings"
	"testing"
	"time"

	"daycore/internal/domain"
)

// These six tables carry mechanisms, not just columns: an optimistic lock, a
// lease race, a unique index standing in for a transaction. A round trip that
// only checks the fields came back would miss all of it, so each test below
// exercises the thing the table exists to guarantee.

func newStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	return newTestStore(t), context.Background()
}

func pending(sid, title string) *domain.Proposal {
	return &domain.Proposal{
		SessionID: sid, Title: title, Level: domain.LevelL2, Kind: domain.KindCard,
		TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: time.Now().Add(time.Hour),
		Origin: domain.OriginDaemon,
	}
}

func TestProposalRoundTrip(t *testing.T) {
	s, ctx := newStore(t)
	dur := 45
	in := &domain.Proposal{
		SessionID: "s1", Level: domain.LevelL1, Kind: domain.KindTimed,
		Title: "去跑步", Summary: "早上八点有个空档", Reason: "你前三天都在这个点跑",
		Evidence: "op:123,op:124", Date: "2026-07-27", Start: "08:00", Dur: &dur,
		BType: domain.BlockAppointment, LockLevel: domain.LockSoft, LockReason: "和别人约好的时间",
		Rows: []domain.ProposalRow{
			{ID: "r1", Label: "八点跑步", State: domain.ProposalPending},
			{ID: "r2", Label: "九点复习", State: domain.ProposalPending},
		},
		MergeKey:  "morning-gap",
		TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: time.Now().Add(2 * time.Hour).Truncate(time.Millisecond),
		Origin: domain.OriginDaemon, ThreadID: "th1",
		Ops: []domain.ProposalOp{{Tool: "plan_add", Args: map[string]any{"title": "跑步"}}},
	}
	if err := s.Proposals().Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Proposals().Get(ctx, "s1", in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != in.Title || got.Start != in.Start || got.Date != in.Date {
		t.Errorf("scalars: %+v", got)
	}
	if got.Dur == nil || *got.Dur != dur {
		t.Errorf("dur = %v, want %d", got.Dur, dur)
	}
	if len(got.Rows) != 2 || got.Rows[1].Label != "九点复习" {
		t.Errorf("rows: %+v", got.Rows)
	}
	if len(got.Ops) != 1 || got.Ops[0].Tool != "plan_add" {
		t.Errorf("ops: %+v", got.Ops)
	}
	if got.LockLevel != domain.LockSoft {
		t.Errorf("lockLevel = %q", got.LockLevel)
	}
	if got.State != domain.ProposalPending || got.Rev != 1 {
		t.Errorf("state/rev = %q/%d", got.State, got.Rev)
	}
	// Nil timestamps must come back nil, not as the zero time — DeliveredAt nil
	// is what "still in the pool, never shown" means.
	if got.DeliveredAt != nil || got.PushedAt != nil || got.DeliverAfter != nil {
		t.Errorf("nil timestamps came back set: %+v", got)
	}
	// An empty slice must not become nil, or every caller needs a nil check the
	// write path already promised it would not.
	empty := pending("s1", "no rows")
	if err := s.Proposals().Create(ctx, empty); err != nil {
		t.Fatal(err)
	}
	back, _ := s.Proposals().Get(ctx, "s1", empty.ID)
	if back.Rows == nil || len(back.Rows) != 0 {
		t.Errorf("rows = %v, want an empty slice", back.Rows)
	}
}

// The act-first invariant is enforced at the boundary, not at runtime: a card
// whose silence counts as consent must already have something to undo.
func TestProposalCreateRejectsActFirstWithNothingToUndo(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "已经帮你挪好了")
	p.TTLPolicy = domain.TTLSilenceAccepts
	if err := s.Proposals().Create(ctx, p); err == nil {
		t.Fatal("want ErrActFirstNeedsUndo")
	}
	p.AppliedOpIDs = []string{"op:1"}
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatalf("with an op to undo it should be accepted: %v", err)
	}
}

// Optimistic concurrency is what stands in for a transaction. Two clients
// accepting different rows of one compound card both read the same rows JSON;
// without the rev check the second write would erase the first.
func TestProposalUpdateIsCompareAndSet(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "两行卡")
	p.Rows = []domain.ProposalRow{{ID: "r1", Label: "一"}, {ID: "r2", Label: "二"}}
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Proposals().Get(ctx, "s1", p.ID)
	b, _ := s.Proposals().Get(ctx, "s1", p.ID)

	a.Rows[0].State = domain.ProposalAccepted
	if err := s.Proposals().Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	if a.Rev != 2 {
		t.Errorf("rev = %d after a successful write, want 2", a.Rev)
	}

	b.Rows[1].State = domain.ProposalRejected
	if err := s.Proposals().Update(ctx, b); err != domain.ErrConflict {
		t.Fatalf("stale write returned %v, want ErrConflict", err)
	}
	// And the first writer's change survived.
	fresh, _ := s.Proposals().Get(ctx, "s1", p.ID)
	if fresh.Rows[0].State != domain.ProposalAccepted || fresh.Rows[1].State == domain.ProposalRejected {
		t.Errorf("the losing write clobbered the winner: %+v", fresh.Rows)
	}
}

// Silence means yes for work already done and no for work not yet done
// (consensus 14). Getting this backwards either executes unattended work or
// silently discards an undo window.
func TestProposalExpiryIsAsymmetric(t *testing.T) {
	s, ctx := newStore(t)
	past := time.Now().Add(-time.Minute)

	ask := pending("s1", "要不要挪")
	ask.ExpiresAt = past
	if err := s.Proposals().Create(ctx, ask); err != nil {
		t.Fatal(err)
	}
	act := pending("s1", "已经挪好了")
	act.TTLPolicy = domain.TTLSilenceAccepts
	act.AppliedOpIDs = []string{"op:1"}
	act.ExpiresAt = past
	if err := s.Proposals().Create(ctx, act); err != nil {
		t.Fatal(err)
	}
	live := pending("s1", "还没到期")
	if err := s.Proposals().Create(ctx, live); err != nil {
		t.Fatal(err)
	}

	n, err := s.Proposals().Expire(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expired %d, want 2", n)
	}
	got, _ := s.Proposals().Get(ctx, "s1", ask.ID)
	if got.State != domain.ProposalExpired || got.Resolution != domain.ResolutionSilence {
		t.Errorf("ask-first lapsed to %s/%s, want expired/silence", got.State, got.Resolution)
	}
	got, _ = s.Proposals().Get(ctx, "s1", act.ID)
	if got.State != domain.ProposalAccepted || got.Resolution != domain.ResolutionSilence {
		t.Errorf("act-first lapsed to %s/%s, want accepted/silence", got.State, got.Resolution)
	}
	got, _ = s.Proposals().Get(ctx, "s1", live.ID)
	if got.State != domain.ProposalPending {
		t.Errorf("a live proposal was expired: %s", got.State)
	}
}

// Merging retires older offers in the pool. A card the user has already SEEN
// must survive — delivery is throttled, not retracted.
func TestProposalSupersedeSparesDeliveredCards(t *testing.T) {
	s, ctx := newStore(t)
	old := pending("s1", "旧的")
	old.MergeKey = "gap"
	if err := s.Proposals().Create(ctx, old); err != nil {
		t.Fatal(err)
	}
	shown := pending("s1", "已经给用户看过了")
	shown.MergeKey = "gap"
	now := time.Now()
	shown.DeliveredAt = &now
	if err := s.Proposals().Create(ctx, shown); err != nil {
		t.Fatal(err)
	}
	fresh := pending("s1", "新的")
	fresh.MergeKey = "gap"
	if err := s.Proposals().Create(ctx, fresh); err != nil {
		t.Fatal(err)
	}

	n, err := s.Proposals().Supersede(ctx, "s1", "gap", fresh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("superseded %d, want only the undelivered one", n)
	}
	got, _ := s.Proposals().Get(ctx, "s1", shown.ID)
	if got.State != domain.ProposalPending {
		t.Error("a card the user has already seen must not vanish from under them")
	}
	got, _ = s.Proposals().Get(ctx, "s1", old.ID)
	if got.Resolution != domain.ResolutionSuperseded {
		t.Errorf("old resolution = %q", got.Resolution)
	}
}

func TestProposalListFilters(t *testing.T) {
	s, ctx := newStore(t)
	a := pending("s1", "今天")
	a.Date = "2026-07-27"
	b := pending("s1", "明天")
	b.Date = "2026-07-28"
	seen := pending("s1", "看过了")
	now := time.Now()
	seen.DeliveredAt = &now
	other := pending("s2", "别人的")
	for _, p := range []*domain.Proposal{a, b, seen, other} {
		if err := s.Proposals().Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := s.Proposals().List(ctx, domain.ProposalFilter{SessionID: "s1"})
	if len(all) != 3 {
		t.Errorf("session filter returned %d, want 3 (and never s2's)", len(all))
	}
	day, _ := s.Proposals().List(ctx, domain.ProposalFilter{SessionID: "s1", Date: "2026-07-27"})
	if len(day) != 1 || day[0].Title != "今天" {
		t.Errorf("date filter: %+v", day)
	}
	pool, _ := s.Proposals().List(ctx, domain.ProposalFilter{SessionID: "s1", Undelivered: true})
	if len(pool) != 2 {
		t.Errorf("the outbox is undelivered-only; got %d", len(pool))
	}
}

// The lease is a race by construction, so the test races it: two holders, one
// winner, and the loser must not believe otherwise.
func TestLeaseOnlyOneHolder(t *testing.T) {
	s, ctx := newStore(t)
	now := time.Now()

	l, ok, err := s.Leases().Acquire(ctx, domain.LeaseWorker, "instance-a", time.Minute, now)
	if err != nil || !ok {
		t.Fatalf("first acquire: %v %v", ok, err)
	}
	if l.Fence != 1 {
		t.Errorf("fence = %d on first claim, want 1", l.Fence)
	}

	if _, ok, _ := s.Leases().Acquire(ctx, domain.LeaseWorker, "instance-b", time.Minute, now); ok {
		t.Fatal("a second instance took a live lease")
	}

	// Renewal keeps the fence still — it counts hand-overs, not heartbeats.
	l2, ok, err := s.Leases().Acquire(ctx, domain.LeaseWorker, "instance-a", time.Minute, now.Add(20*time.Second))
	if err != nil || !ok {
		t.Fatalf("renewal: %v %v", ok, err)
	}
	if l2.Fence != 1 {
		t.Errorf("fence moved on a renewal: %d", l2.Fence)
	}

	// Once it lapses the other instance may take it, and the fence moves —
	// which is how the old holder can discover it is no longer the leader
	// without trusting its own clock.
	later := now.Add(2 * time.Minute)
	l3, ok, err := s.Leases().Acquire(ctx, domain.LeaseWorker, "instance-b", time.Minute, later)
	if err != nil || !ok {
		t.Fatalf("takeover after expiry: %v %v", ok, err)
	}
	if l3.Fence != 2 {
		t.Errorf("fence = %d after a hand-over, want 2", l3.Fence)
	}
	if l3.Held("instance-a", later) {
		t.Error("the old holder still believes it holds the lease")
	}
}

// Releasing on shutdown turns a full TTL of nothing happening into nothing.
func TestLeaseReleaseHandsOverImmediately(t *testing.T) {
	s, ctx := newStore(t)
	now := time.Now()
	if _, ok, _ := s.Leases().Acquire(ctx, domain.LeaseWorker, "a", time.Minute, now); !ok {
		t.Fatal("acquire")
	}
	if err := s.Leases().Release(ctx, domain.LeaseWorker, "a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Leases().Acquire(ctx, domain.LeaseWorker, "b", time.Minute, now); !ok {
		t.Error("after a release the next instance should get it without waiting out the TTL")
	}
	// Somebody else's release must not be able to evict us.
	if err := s.Leases().Release(ctx, domain.LeaseWorker, "not-the-holder"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Leases().Acquire(ctx, domain.LeaseWorker, "c", time.Minute, now); ok {
		t.Error("a release by a non-holder evicted the real holder")
	}
}

// The unique index IS the mutual exclusion — this package has no transactions,
// and a failed insert is the only "somebody else got there first" signal all
// four backends share.
func TestJobRunClaimIsExclusive(t *testing.T) {
	s, ctx := newStore(t)
	mk := func(instance string) *domain.JobRun {
		return &domain.JobRun{
			SessionID: "s1", Job: domain.JobMorningBrief, RunKey: "2026-07-27", Instance: instance,
		}
	}
	first := mk("a")
	ok, err := s.JobRuns().Claim(ctx, first)
	if err != nil || !ok {
		t.Fatalf("first claim: %v %v", ok, err)
	}
	if ok, err := s.JobRuns().Claim(ctx, mk("b")); err != nil || ok {
		t.Fatalf("second claim on the same occurrence: ok=%v err=%v", ok, err)
	}
	// A different day is a different occurrence.
	other := mk("b")
	other.RunKey = "2026-07-28"
	if ok, err := s.JobRuns().Claim(ctx, other); err != nil || !ok {
		t.Fatalf("next day should be claimable: %v %v", ok, err)
	}

	if err := s.JobRuns().Finish(ctx, first.ID, domain.JobDone, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	runs, err := s.JobRuns().List(ctx, "s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("listed %d runs, want 2", len(runs))
	}
	var done *domain.JobRun
	for i := range runs {
		if runs[i].ID == first.ID {
			done = &runs[i]
		}
	}
	if done == nil || done.Status != domain.JobDone || done.EndedAt == nil {
		t.Errorf("finished run: %+v", done)
	}
	// A finished occurrence stays claimed. Re-running the morning brief because
	// the process restarted is exactly what this table prevents.
	if ok, _ := s.JobRuns().Claim(ctx, mk("c")); ok {
		t.Error("a completed occurrence was claimed again")
	}
}

// A crash leaves a row saying "running" forever. It must be recoverable — but
// only after long enough that the original cannot still be working, because
// stealing early means sending the same push twice.
func TestJobRunStaleClaimIsRecoverable(t *testing.T) {
	s, ctx := newStore(t)
	run := &domain.JobRun{SessionID: "s1", Job: domain.JobEveningReview, RunKey: "2026-07-27", Instance: "crashed"}
	if ok, err := s.JobRuns().Claim(ctx, run); err != nil || !ok {
		t.Fatal(err)
	}
	// Fresh: not stealable.
	if ok, _ := s.JobRuns().Claim(ctx, &domain.JobRun{
		SessionID: "s1", Job: domain.JobEveningReview, RunKey: "2026-07-27", Instance: "other"}); ok {
		t.Fatal("a running job was stolen while it could still be running")
	}
	// Backdate the claim past the staleness threshold.
	stale := toMillis(time.Now().Add(-domain.JobStaleAfter - time.Minute))
	if _, err := s.exec(ctx, `UPDATE job_runs SET started_at = ? WHERE id = ?`, stale, run.ID); err != nil {
		t.Fatal(err)
	}
	taken := &domain.JobRun{SessionID: "s1", Job: domain.JobEveningReview, RunKey: "2026-07-27", Instance: "other"}
	if ok, err := s.JobRuns().Claim(ctx, taken); err != nil || !ok {
		t.Fatalf("a long-dead claim should be recoverable: %v %v", ok, err)
	}
	runs, _ := s.JobRuns().List(ctx, "s1", 10)
	if len(runs) != 1 {
		t.Errorf("taking over should reuse the row, not add one: %d rows", len(runs))
	}
	if runs[0].Attempts != 2 {
		t.Errorf("attempts = %d, want 2", runs[0].Attempts)
	}
}

// Prune must not erase a "running" row: it is the only trace of a job that
// crashed and never came back.
func TestJobRunPruneKeepsRunning(t *testing.T) {
	s, ctx := newStore(t)
	done := &domain.JobRun{SessionID: "s1", Job: domain.JobAutoPlan, RunKey: "old", Instance: "a"}
	if _, err := s.JobRuns().Claim(ctx, done); err != nil {
		t.Fatal(err)
	}
	if err := s.JobRuns().Finish(ctx, done.ID, domain.JobDone, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	stuck := &domain.JobRun{SessionID: "s1", Job: domain.JobAutoPlan, RunKey: "stuck", Instance: "a"}
	if _, err := s.JobRuns().Claim(ctx, stuck); err != nil {
		t.Fatal(err)
	}
	old := toMillis(time.Now().Add(-90 * 24 * time.Hour))
	if _, err := s.exec(ctx, `UPDATE job_runs SET started_at = ?`, old); err != nil {
		t.Fatal(err)
	}

	n, err := s.JobRuns().Prune(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want only the finished one", n)
	}
	runs, _ := s.JobRuns().List(ctx, "s1", 10)
	if len(runs) != 1 || runs[0].RunKey != "stuck" {
		t.Errorf("the crashed run should survive: %+v", runs)
	}
}

func TestRapportStateRoundTrip(t *testing.T) {
	s, ctx := newStore(t)
	if _, err := s.Rapport().Get(ctx, "s1"); err != domain.ErrNotFound {
		t.Fatalf("a missing cache is ErrNotFound, not %v", err)
	}
	cursorAt := time.Now().Truncate(time.Millisecond)
	in := &domain.RapportState{
		SessionID: "s1",
		Scores: map[string]domain.RapportScore{
			domain.OpDomainSchedule: {Value: 0.34, Evidence: 5},
			domain.OpDomainArchive:  {Value: 0.61, Evidence: 12},
		},
		Cursor:      domain.OpLogCursor{CreatedAt: cursorAt, ID: "op-99"},
		FoldVersion: 1,
	}
	if err := s.Rapport().Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Rapport().Get(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Scores[domain.OpDomainSchedule].Value != 0.34 || got.Scores[domain.OpDomainArchive].Evidence != 12 {
		t.Errorf("scores: %+v", got.Scores)
	}
	// The cursor is what makes the cache resumable rather than a number nobody
	// can rebuild. Losing it would mean re-folding the whole ledger every time.
	if !got.Cursor.CreatedAt.Equal(cursorAt.UTC()) || got.Cursor.ID != "op-99" {
		t.Errorf("cursor = %+v, want %v/op-99", got.Cursor, cursorAt.UTC())
	}

	in.Scores[domain.OpDomainSchedule] = domain.RapportScore{Value: 0.37, Evidence: 6}
	if err := s.Rapport().Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Rapport().Get(ctx, "s1")
	if got.Scores[domain.OpDomainSchedule].Evidence != 6 {
		t.Errorf("second save did not overwrite: %+v", got.Scores)
	}

	if err := s.Rapport().Reset(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rapport().Get(ctx, "s1"); err != domain.ErrNotFound {
		t.Errorf("after Reset the cache should be gone, got %v", err)
	}
}

func TestRhythmProfileRoundTrip(t *testing.T) {
	s, ctx := newStore(t)
	if _, err := s.Rhythm().Get(ctx, "s1"); err != domain.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	runSince := time.Now().Add(-9 * time.Hour).Truncate(time.Millisecond)
	last := time.Now().Add(-3 * time.Minute).Truncate(time.Millisecond)
	in := &domain.RhythmProfile{
		SessionID: "s1", Wake: "10:00", Sleep: "02:30", Source: "learned", Days: 12,
		RunSince: runSince, LastSignalAt: last,
	}
	if err := s.Rhythm().Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Rhythm().Get(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	// A bedtime past midnight is the normal case for a night owl and must
	// survive the round trip as written, not be normalised into the next day.
	if got.Wake != "10:00" || got.Sleep != "02:30" || got.Days != 12 {
		t.Errorf("profile: %+v", got)
	}
	if !got.RunSince.Equal(runSince.UTC()) || !got.LastSignalAt.Equal(last.UTC()) {
		t.Errorf("run marks: %v / %v", got.RunSince, got.LastSignalAt)
	}

	// A profile with no current run stores zero times, and they must come back
	// zero — a zero RunSince reads as "asleep", and a spurious epoch timestamp
	// would read as "awake since 1970".
	quiet := &domain.RhythmProfile{SessionID: "s2", Wake: "07:30", Sleep: "22:30", Source: "default"}
	if err := s.Rhythm().Save(ctx, quiet); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.Rhythm().Get(ctx, "s2")
	if !got2.RunSince.IsZero() || !got2.LastSignalAt.IsZero() {
		t.Errorf("an idle profile came back with run marks: %+v", got2)
	}
}

// Observe has to widen bounds in the statement itself. A read-compare-write
// would lose an update whenever two tabs are open, which is the common case.
func TestRhythmObserveWidensBounds(t *testing.T) {
	s, ctx := newStore(t)
	for _, m := range []int{600, 400, 900, 700} {
		if err := s.Rhythm().Observe(ctx, "s1", "2026-07-26", m); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Rhythm().Observe(ctx, "s1", "2026-07-27", 500); err != nil {
		t.Fatal(err)
	}
	days, err := s.Rhythm().Days(ctx, "s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("got %d days, want 2", len(days))
	}
	// Newest first.
	if days[0].Day != "2026-07-27" {
		t.Errorf("order: %+v", days)
	}
	var d26 domain.RhythmDay
	for _, d := range days {
		if d.Day == "2026-07-26" {
			d26 = d
		}
	}
	if d26.FirstMin != 400 || d26.LastMin != 900 || d26.Signals != 4 {
		t.Errorf("bounds = %d..%d over %d signals, want 400..900 over 4", d26.FirstMin, d26.LastMin, d26.Signals)
	}

	n, err := s.Rhythm().PruneDays(ctx, "s1", "2026-07-27")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want 1", n)
	}
}

func TestLocaleOverridesRoundTrip(t *testing.T) {
	s, ctx := newStore(t)
	if err := s.Locales().Set(ctx, "mood.happy", "ja-JP", "うれしい"); err != nil {
		t.Fatal(err)
	}
	if err := s.Locales().Set(ctx, "mood.calm", "ja-JP", "おだやか"); err != nil {
		t.Fatal(err)
	}
	if err := s.Locales().Set(ctx, "mood.happy", "en-US", "Cheerful"); err != nil {
		t.Fatal(err)
	}
	// Setting the same key twice updates rather than duplicating — the whole
	// table is keyed on (key, locale).
	if err := s.Locales().Set(ctx, "mood.happy", "ja-JP", "うれしい！"); err != nil {
		t.Fatal(err)
	}

	all, err := s.Locales().All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d overrides, want 3: %+v", len(all), all)
	}
	for _, o := range all {
		if o.Key == "mood.happy" && o.Locale == "ja-JP" && o.Content != "うれしい！" {
			t.Errorf("second Set did not overwrite: %q", o.Content)
		}
	}

	if err := s.Locales().Delete(ctx, "mood.happy", "en-US"); err != nil {
		t.Fatal(err)
	}
	n, err := s.Locales().DeleteLocale(ctx, "ja-JP")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("uninstalling a language removed %d rows, want 2", n)
	}
	all, _ = s.Locales().All(ctx)
	if len(all) != 0 {
		t.Errorf("leftovers: %+v", all)
	}
}

// ── regressions for what the adversarial review actually broke ───────────────
//
// Each of these reproduced against the first version of this code. They are
// separated from the tests above because their value is historical: they say
// "this specific thing went wrong", and a future simplification that reverts one
// of the fixes will fail here rather than in production.

// Two daemons each produce a card for the same merge key and each supersede with
// their own id. Retiring "everything that is not me" made them annihilate each
// other and the user saw nothing — the one outcome consensus 15 forbids, since
// throttling delivery is not cancelling it.
func TestSupersedeIsNotMutualAnnihilation(t *testing.T) {
	s, ctx := newStore(t)
	a := pending("s1", "daemon A 的卡")
	a.MergeKey = "gap"
	if err := s.Proposals().Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	// Force distinct creation stamps; both daemons act within the same tick in
	// production, but the ordering only has to be *defined*, not large.
	if _, err := s.exec(ctx, `UPDATE proposals SET created_at = created_at - 1000 WHERE id = ?`, a.ID); err != nil {
		t.Fatal(err)
	}
	b := pending("s1", "daemon B 的卡")
	b.MergeKey = "gap"
	if err := s.Proposals().Create(ctx, b); err != nil {
		t.Fatal(err)
	}

	// Both call supersede with their own id, in either order.
	if _, err := s.Proposals().Supersede(ctx, "s1", "gap", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Proposals().Supersede(ctx, "s1", "gap", b.ID); err != nil {
		t.Fatal(err)
	}

	live, err := s.Proposals().List(ctx, domain.ProposalFilter{SessionID: "s1", State: domain.ProposalPending})
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 {
		t.Fatalf("%d survivors, want exactly 1 — the newest", len(live))
	}
	if live[0].ID != b.ID {
		t.Errorf("survivor is %q, want the newer card", live[0].Title)
	}
}

// The outbox is not "undelivered": that set includes cards that lapsed in the
// pool and cards held back for later, neither of which the user may see. The
// filter has to exclude them in SQL, because a LIMIT applied before a Go-side
// filter can return an empty page while deliverable cards wait behind it.
func TestDeliverableFilterExcludesLapsedAndHeldBack(t *testing.T) {
	s, ctx := newStore(t)
	now := time.Now()

	live := pending("s1", "可投递")
	lapsed := pending("s1", "池子里过期了")
	lapsed.ExpiresAt = now.Add(-time.Minute)
	later := pending("s1", "压后再投")
	after := now.Add(time.Hour)
	later.DeliverAfter = &after
	for _, p := range []*domain.Proposal{live, lapsed, later} {
		if err := s.Proposals().Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	pool, err := s.Proposals().List(ctx, domain.ProposalFilter{
		SessionID: "s1", State: domain.ProposalPending, Undelivered: true, DeliverableAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pool) != 1 || pool[0].ID != live.ID {
		var got []string
		for _, p := range pool {
			got = append(got, p.Title)
		}
		t.Errorf("deliverable set = %v, want just the live one", got)
	}
	// And every row it returns must agree with the in-memory predicate.
	for _, p := range pool {
		if !p.Deliverable(now) {
			t.Errorf("%q came back from the query but Deliverable() says no", p.Title)
		}
	}
}

// One transient failure must not cost the user the whole day's brief, and a
// permanently failing job must not retry forever.
func TestJobRunFailedRetriesUpToTheCap(t *testing.T) {
	s, ctx := newStore(t)
	mk := func() *domain.JobRun {
		return &domain.JobRun{SessionID: "s1", Job: domain.JobMorningBrief, RunKey: "2026-07-27", Instance: "a"}
	}
	for attempt := 1; attempt <= domain.JobMaxAttempts; attempt++ {
		run := mk()
		ok, err := s.JobRuns().Claim(ctx, run)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("attempt %d was refused; the cap is %d", attempt, domain.JobMaxAttempts)
		}
		if err := s.JobRuns().Finish(ctx, run.ID, domain.JobFailed, "boom", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := s.JobRuns().Claim(ctx, mk()); err != nil || ok {
		t.Errorf("past the cap the occurrence must stop retrying: ok=%v err=%v", ok, err)
	}
	// A succeeded occurrence is never retried, whatever the attempt count.
	done := &domain.JobRun{SessionID: "s1", Job: domain.JobAutoPlan, RunKey: "k", Instance: "a"}
	if _, err := s.JobRuns().Claim(ctx, done); err != nil {
		t.Fatal(err)
	}
	if err := s.JobRuns().Finish(ctx, done.ID, domain.JobDone, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.JobRuns().Claim(ctx, &domain.JobRun{
		SessionID: "s1", Job: domain.JobAutoPlan, RunKey: "k", Instance: "b"}); ok {
		t.Error("a finished occurrence was claimed again")
	}
}

// A stale takeover rotates the row's id, so the previous owner waking up late
// calls Finish with an id that matches nothing. Without the rotation its stale
// verdict would land on the new claimant's run and mark a healthy job failed.
func TestJobRunTakeoverOrphansTheZombiesFinish(t *testing.T) {
	s, ctx := newStore(t)
	zombie := &domain.JobRun{SessionID: "s1", Job: domain.JobRollingReplan, RunKey: "k", Instance: "died"}
	if _, err := s.JobRuns().Claim(ctx, zombie); err != nil {
		t.Fatal(err)
	}
	zombieID := zombie.ID
	stale := toMillis(time.Now().Add(-domain.JobStaleAfter - time.Minute))
	if _, err := s.exec(ctx, `UPDATE job_runs SET started_at = ? WHERE id = ?`, stale, zombieID); err != nil {
		t.Fatal(err)
	}

	fresh := &domain.JobRun{SessionID: "s1", Job: domain.JobRollingReplan, RunKey: "k", Instance: "alive"}
	if ok, err := s.JobRuns().Claim(ctx, fresh); err != nil || !ok {
		t.Fatalf("takeover: %v %v", ok, err)
	}
	if fresh.ID == zombieID {
		t.Fatal("the takeover kept the old id; a late Finish from the dead instance would hit this row")
	}

	// The zombie finally comes back and reports failure.
	if err := s.JobRuns().Finish(ctx, zombieID, domain.JobFailed, "died", time.Now()); err != nil {
		t.Fatal(err)
	}
	runs, _ := s.JobRuns().List(ctx, "s1", 10)
	if len(runs) != 1 {
		t.Fatalf("%d rows, want 1", len(runs))
	}
	if runs[0].Status != domain.JobRunning || runs[0].Instance != "alive" {
		t.Errorf("the zombie's late verdict landed on the live run: %+v", runs[0])
	}
}

// Discarding an unreadable rapport cache has to discard its cursor too. Keeping
// the cursor tells the caller "fold forward from here" over scores of zero, so
// every operation before that point is skipped forever and the rebuilt reading
// never recovers.
func TestRapportDiscardsCursorWithAnUnreadableCache(t *testing.T) {
	s, ctx := newStore(t)
	if err := s.Rapport().Save(ctx, &domain.RapportState{
		SessionID: "s1",
		Scores:    map[string]domain.RapportScore{domain.OpDomainSchedule: {Value: 0.5, Evidence: 3}},
		Cursor:    domain.OpLogCursor{CreatedAt: time.Now(), ID: "op-99"},
	}); err != nil {
		t.Fatal(err)
	}
	// marshalJSON writes the literal "null" whenever json.Marshal fails, so this
	// row shape is reachable through the code's own write path.
	if _, err := s.exec(ctx, `UPDATE rapport_states SET scores_json = 'null' WHERE session_id = ?`, "s1"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Rapport().Get(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Scores) != 0 {
		t.Errorf("scores = %+v, want empty", got.Scores)
	}
	if !got.Cursor.CreatedAt.IsZero() || got.Cursor.ID != "" {
		t.Errorf("cursor = %+v, want entirely zero so the caller re-folds the whole ledger", got.Cursor)
	}
}

// Two tabs sending the first signal of a new day at the same moment both find no
// row to widen and both insert. The loser must fold into the winner's row rather
// than surface a constraint error and drop the signal.
func TestObserveConcurrentFirstWriteDoesNotDropSignals(t *testing.T) {
	s, ctx := newStore(t)
	const tabs = 6
	errs := make(chan error, tabs)
	start := make(chan struct{})
	for i := 0; i < tabs; i++ {
		go func(i int) {
			<-start
			errs <- s.Rhythm().Observe(ctx, "s1", "2026-07-27", 400+i)
		}(i)
	}
	close(start)
	for i := 0; i < tabs; i++ {
		if err := <-errs; err != nil {
			t.Errorf("Observe failed on a concurrent first write: %v", err)
		}
	}
	days, err := s.Rhythm().Days(ctx, "s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 {
		t.Fatalf("%d rows for one day", len(days))
	}
	if days[0].Signals != tabs {
		t.Errorf("signals = %d, want %d — a signal was dropped", days[0].Signals, tabs)
	}
	if days[0].FirstMin != 400 || days[0].LastMin != 400+tabs-1 {
		t.Errorf("bounds = %d..%d, want 400..%d", days[0].FirstMin, days[0].LastMin, 400+tabs-1)
	}
}

// Same race, on the console's save button: a double-click on a brand-new key.
func TestLocaleSetConcurrentFirstWrite(t *testing.T) {
	s, ctx := newStore(t)
	const clicks = 4
	errs := make(chan error, clicks)
	start := make(chan struct{})
	for i := 0; i < clicks; i++ {
		go func() {
			<-start
			errs <- s.Locales().Set(ctx, "mood.happy", "ja-JP", "うれしい")
		}()
	}
	close(start)
	for i := 0; i < clicks; i++ {
		if err := <-errs; err != nil {
			t.Errorf("Set failed on a concurrent first write: %v", err)
		}
	}
	all, _ := s.Locales().All(ctx)
	if len(all) != 1 || all[0].Content != "うれしい" {
		t.Errorf("got %+v, want exactly one row", all)
	}
}

// MySQL reports rows CHANGED rather than MATCHED unless clientFoundRows is set,
// and twenty-three call sites in this package branch on RowsAffected — including
// three that turn a zero into ErrNotFound, so on MySQL saving a form without
// altering a value would return "not found". The dialect forces the flag rather
// than documenting it, because the DSN is operator-supplied.
func TestMySQLDSNForcesClientFoundRows(t *testing.T) {
	d := mysqlDialect{}
	for _, c := range []struct{ in, want string }{
		{"u:p@tcp(h:3306)/db", "u:p@tcp(h:3306)/db?clientFoundRows=true"},
		{"u:p@tcp(h:3306)/db?parseTime=true", "u:p@tcp(h:3306)/db?parseTime=true&clientFoundRows=true"},
		{"u:p@tcp(h:3306)/db?clientFoundRows=true", "u:p@tcp(h:3306)/db?clientFoundRows=true"},
		{"u:p@tcp(h:3306)/db?clientFoundRows=false", "u:p@tcp(h:3306)/db?clientFoundRows=false"},
	} {
		if got := d.NormalizeDSN(c.in); got != c.want {
			t.Errorf("NormalizeDSN(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := (postgresDialect{}).NormalizeDSN("whatever"); got != "whatever" {
		t.Errorf("postgres rewrote the DSN: %q", got)
	}
}

// SQLite allows one writer at a time, so without busy_timeout a second writer
// fails immediately rather than waiting — which breaks every UPDATE-then-INSERT
// here and rhythmRepo.Observe, which runs on every awake signal. Both pragmas
// are in the documented DSN, and that is the problem: the DSN is
// operator-supplied.
func TestSQLiteDSNForcesPragmas(t *testing.T) {
	d := sqliteDialect{}
	got := d.NormalizeDSN("file:/tmp/x.db")
	for _, want := range []string{"busy_timeout(5000)", "journal_mode(WAL)"} {
		if !strings.Contains(got, want) {
			t.Errorf("NormalizeDSN = %q, missing %s", got, want)
		}
	}
	// An operator who set their own value keeps it.
	custom := d.NormalizeDSN("file:/tmp/x.db?_pragma=busy_timeout(20000)")
	if strings.Count(custom, "busy_timeout") != 1 || !strings.Contains(custom, "busy_timeout(20000)") {
		t.Errorf("an explicit busy_timeout was overridden or duplicated: %q", custom)
	}
	if got := d.NormalizeDSN(""); got != "" {
		t.Errorf("an empty DSN should stay empty, got %q", got)
	}
}

// Postgres needs a migration lock; IF NOT EXISTS is not a concurrency primitive
// there. The others must not claim one they cannot hold across a pooled
// connection.
func TestOnlyPostgresTakesAMigrationLock(t *testing.T) {
	if acq, rel := (postgresDialect{}).MigrationLock(); len(acq) != 1 || len(rel) != 1 {
		t.Errorf("postgres should acquire and release exactly one lock, got %v / %v", acq, rel)
	}
	for _, d := range []Dialect{sqliteDialect{}, mysqlDialect{}} {
		if acq, rel := d.MigrationLock(); len(acq) != 0 || len(rel) != 0 {
			t.Errorf("%s should need no migration lock, got %v / %v", d.Name(), acq, rel)
		}
	}
}
