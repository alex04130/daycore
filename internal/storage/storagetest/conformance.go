// Package storagetest is one behavioural test suite that every domain.Store
// implementation runs.
//
// It exists because the two stores that already shipped disagreed with each
// other in about fifteen places, and nothing anywhere noticed. sqlstore had
// sixty tests and mongostore had none, so "the two backends behave the same" was
// an assumption held up by review alone — and an adversarial review pass found
// that assumption false on the type an agent tool asserts on, on what a lease
// returns when it loses, on whether a takeover orphans a zombie's verdict, on
// whether a nullable timestamp survives a round trip. Every one of those is a
// service-layer bug waiting for whichever backend the author did not run.
//
// So the invariants live here, once, stated against the interface. A new backend
// — the planned HTTP conversion layer among them — is finished when this passes,
// and not before. That is also the only thing that makes a fifth backend
// affordable: pairwise divergence grows as the square of the number of stores,
// and a shared suite is what flattens it.
//
// The suite is deliberately behavioural. dialect_parity_test.go already compares
// the three SQL dialects' DDL strings statically; that cannot see any of this.
package storagetest

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"daycore/internal/domain"
)

// Harness is what a backend supplies to run the suite: a fresh store, plus the
// two things the suite cannot do through domain.Store.
//
// Both extras are about MOVING STORED TIME. Several invariants only appear once a
// claim has gone stale or two cards were created in the same instant, and waiting
// ten real minutes or racing the millisecond clock would make the suite slow and
// flaky. Only the backend knows how to reach behind its own storage, so it hands
// that reach in rather than the suite guessing.
type Harness interface {
	// Store is fresh, empty and migrated. Each call to the factory must be
	// isolated from every other — a suite that shared state between cases would
	// pass or fail depending on test order.
	Store() domain.Store
	// BackdateJobRuns shifts every job_run in a session back by d, simulating a
	// claimant that died without waiting out JobStaleAfter.
	BackdateJobRuns(t *testing.T, sessionID string, d time.Duration)
	// ForceProposalCreatedAt sets every proposal in a session to one instant, so
	// the same-millisecond tie two daemons hit routinely is deterministic here.
	ForceProposalCreatedAt(t *testing.T, sessionID string, at time.Time)
	// BackdateProposals moves updated_at, which is the column retention is
	// measured from — a record is kept for so long after it SETTLED, not after
	// it was created. Separate from ForceProposalCreatedAt because the two
	// columns answer different questions and a helper that quietly moved both
	// would let a Prune test pass without pruning anything.
	BackdateProposals(t *testing.T, sessionID string, at time.Time)
}

// Factory returns a fresh Harness.
type Factory func(t *testing.T) Harness

// Run executes the whole suite. Call it from each backend's own test file:
//
//	func TestConformance(t *testing.T) {
//	    storagetest.Run(t, func(t *testing.T) storagetest.Harness { … })
//	}
func Run(t *testing.T, newHarness Factory) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			c.fn(t, h)
		})
	}
}

type suiteCase struct {
	name string
	fn   func(t *testing.T, h Harness)
}

var cases = []suiteCase{
	{"Lease/OneHolderAndFence", leaseOneHolder},
	{"Lease/RefusesEmptyHolder", leaseEmptyHolder},
	{"Lease/NeverClaimsAnotherHoldersRow", leaseNeverClaimsOther},
	{"Lease/ReleaseHandsOverImmediately", leaseRelease},
	{"JobRun/ClaimIsExclusive", jobClaimExclusive},
	{"JobRun/FailedRetriesToTheCap", jobFailedRetries},
	{"JobRun/CrashTakeoverIsBoundedAndReportsAttempts", jobCrashTakeover},
	{"JobRun/TakeoverOrphansTheZombiesFinish", jobZombieFinish},
	{"JobRun/PruneKeepsRunning", jobPrune},
	{"Proposal/RoundTrip", proposalRoundTrip},
	{"Proposal/NilAndEmptyAreDistinctFromMissing", proposalNilEmpty},
	{"Proposal/OpArgsComeBackAsJSONTypes", proposalArgTypes},
	{"Proposal/ValidatedOnCreateAndUpdate", proposalValidate},
	{"Proposal/UpdateIsCompareAndSet", proposalCAS},
	{"Proposal/ExpiryIsAsymmetric", proposalExpiry},
	{"Proposal/SupersedeKeepsExactlyOneEvenOnATie", proposalSupersedeTie},
	{"Proposal/SupersedeSparesDelivered", proposalSupersedeDelivered},
	{"Proposal/SupersedeMissingKeeperIsANonEvent", proposalSupersedeMissing},
	{"Proposal/DeliverableExcludesLapsedAndHeldBack", proposalDeliverable},
	{"Proposal/UnmarshalableOpsFailTheWrite", proposalBadOps},
	{"Rapport/RoundTripWithCursor", rapportRoundTrip},
	{"Rhythm/SaveDoesNotClobberLiveMarks", rhythmSaveVsTouch},
	{"Rhythm/TouchOnlyMovesForward", rhythmTouchForward},
	{"Rhythm/ObserveWidensBounds", rhythmObserve},
	{"Rhythm/ObserveConcurrentFirstWrite", rhythmObserveRace},
	{"Locale/RoundTripAndDeleteLocale", localeRoundTrip},
	{"Locale/ConcurrentFirstWrite", localeRace},
	{"OpLog/RevertedByIsExact", opLogRevertedBy},
	{"OpLog/ScanIsOldestFirstFromACursor", opLogScan},
	{"Upsert/EmptyCanvasIDIsRefused", upsertEmptyKey},
	{"List/LimitDefaultAndCeilingAgree", listLimitsAgree},
	{"Delete/ScopedToSessionAndReportsAbsence", deleteScopeAndAbsence},
	{"Proposal/FilterDimensionsAndCount", proposalFilterDimensions},
	{"Proposal/StackIsAConjunctionNotAStamp", proposalStackIsAConjunction},
	{"Proposal/PruneSparesPending", proposalPruneSparesPending},
	{"Proposal/OwnerInstanceIsQueryable", proposalOwnerInstanceIsQueryable},
}

// ── helpers ─────────────────────────────────────────────────────────────────

func bg() context.Context { return context.Background() }

func pending(sid, title string) *domain.Proposal {
	return &domain.Proposal{
		SessionID: sid, Title: title, Level: domain.LevelL2, Kind: domain.KindCard,
		TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: time.Now().Add(time.Hour),
		Origin: domain.OriginDaemon,
	}
}

func mustCreate(t *testing.T, s domain.Store, p *domain.Proposal) {
	t.Helper()
	if err := s.Proposals().Create(bg(), p); err != nil {
		t.Fatalf("create %q: %v", p.Title, err)
	}
}

// ── lease ───────────────────────────────────────────────────────────────────

func leaseOneHolder(t *testing.T, h Harness) {
	s := h.Store()
	now := time.Now()
	l, ok, err := s.Leases().Acquire(bg(), domain.LeaseWorker, "a", time.Minute, now)
	if err != nil || !ok {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}
	if l.Fence != 1 {
		t.Errorf("fence = %d on the first claim, want 1", l.Fence)
	}
	if _, ok, _ := s.Leases().Acquire(bg(), domain.LeaseWorker, "b", time.Minute, now); ok {
		t.Fatal("a second instance took a live lease")
	}
	// Renewal must not move the fence: it counts hand-overs, not heartbeats, so
	// a holder can tell "still mine" from "mine again after someone else had it".
	l2, ok, err := s.Leases().Acquire(bg(), domain.LeaseWorker, "a", time.Minute, now.Add(20*time.Second))
	if err != nil || !ok {
		t.Fatalf("renewal: ok=%v err=%v", ok, err)
	}
	if l2.Fence != 1 {
		t.Errorf("fence moved on a renewal: %d", l2.Fence)
	}
	later := now.Add(2 * time.Minute)
	l3, ok, err := s.Leases().Acquire(bg(), domain.LeaseWorker, "b", time.Minute, later)
	if err != nil || !ok {
		t.Fatalf("takeover after expiry: ok=%v err=%v", ok, err)
	}
	if l3.Fence != 2 {
		t.Errorf("fence = %d after a hand-over, want 2", l3.Fence)
	}
	if l3.Held("a", later) {
		t.Error("the old holder still reads as the holder")
	}
}

// Two instances sharing a holder string both match the renewal predicate, so
// both keep coming back true forever and the fence never moves — the one failure
// the fence exists to catch. Uniqueness is the caller's contract; the empty case
// is the only one this layer can see.
func leaseEmptyHolder(t *testing.T, h Harness) {
	s := h.Store()
	if _, ok, err := s.Leases().Acquire(bg(), domain.LeaseWorker, "", time.Minute, time.Now()); err == nil || ok {
		t.Errorf("an empty holder should be refused, got ok=%v err=%v", ok, err)
	}
}

// The post-image cannot come from the acquiring statement on every engine, and in
// the gap an instance whose clock runs fast can take the lease. Returning that
// row unchecked hands US the row naming somebody ELSE as holder, with THEIR
// fence — so we would record their fence as ours and conclude nothing changed.
func leaseNeverClaimsOther(t *testing.T, h Harness) {
	s := h.Store()
	base := time.Now()
	if _, ok, err := s.Leases().Acquire(bg(), domain.LeaseWorker, "a", time.Minute, base); err != nil || !ok {
		t.Fatalf("acquire: %v", err)
	}
	// B's clock runs two minutes fast, so it sees A's fresh lease as lapsed.
	if _, ok, err := s.Leases().Acquire(bg(), domain.LeaseWorker, "b", time.Minute, base.Add(2*time.Minute)); err != nil || !ok {
		t.Fatalf("skewed takeover: %v", err)
	}
	l, ok, err := s.Leases().Acquire(bg(), domain.LeaseWorker, "a", time.Minute, base.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("A came back holding a lease that B owns")
	}
	if l != nil && l.Holder != "b" {
		t.Errorf("the row should name B, got %q", l.Holder)
	}
}

func leaseRelease(t *testing.T, h Harness) {
	s := h.Store()
	now := time.Now()
	if _, ok, _ := s.Leases().Acquire(bg(), domain.LeaseWorker, "a", time.Minute, now); !ok {
		t.Fatal("acquire")
	}
	if err := s.Leases().Release(bg(), domain.LeaseWorker, "a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Leases().Acquire(bg(), domain.LeaseWorker, "b", time.Minute, now); !ok {
		t.Error("after a release the next instance should not have to wait out the TTL")
	}
	// A release by somebody who is not the holder must not evict the holder.
	if err := s.Leases().Release(bg(), domain.LeaseWorker, "not-the-holder"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Leases().Acquire(bg(), domain.LeaseWorker, "c", time.Minute, now); ok {
		t.Error("a release by a non-holder evicted the real holder")
	}
}

// ── job runs ────────────────────────────────────────────────────────────────

func job(sid, name, key, inst string) *domain.JobRun {
	return &domain.JobRun{SessionID: sid, Job: name, RunKey: key, Instance: inst}
}

func jobClaimExclusive(t *testing.T, h Harness) {
	s := h.Store()
	first := job("s1", domain.JobMorningBrief, "2026-07-27", "a")
	if ok, err := s.JobRuns().Claim(bg(), first); err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	if ok, err := s.JobRuns().Claim(bg(), job("s1", domain.JobMorningBrief, "2026-07-27", "b")); err != nil || ok {
		t.Fatalf("second claim on the same occurrence: ok=%v err=%v", ok, err)
	}
	// A different day, and a different session, are different occurrences.
	if ok, err := s.JobRuns().Claim(bg(), job("s1", domain.JobMorningBrief, "2026-07-28", "b")); err != nil || !ok {
		t.Fatalf("next day should be claimable: ok=%v err=%v", ok, err)
	}
	if ok, err := s.JobRuns().Claim(bg(), job("s2", domain.JobMorningBrief, "2026-07-27", "b")); err != nil || !ok {
		t.Fatalf("another session should be claimable: ok=%v err=%v", ok, err)
	}

	if err := s.JobRuns().Finish(bg(), first.ID, domain.JobDone, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	// A completed occurrence stays claimed: re-running the morning brief because
	// a process restarted is what this table prevents.
	if ok, _ := s.JobRuns().Claim(bg(), job("s1", domain.JobMorningBrief, "2026-07-27", "c")); ok {
		t.Error("a completed occurrence was claimed again")
	}
	runs, err := s.JobRuns().List(bg(), "s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("listed %d runs for s1, want 2", len(runs))
	}
	var done *domain.JobRun
	for i := range runs {
		if runs[i].RunKey == "2026-07-27" {
			done = &runs[i]
		}
	}
	if done == nil || done.Status != domain.JobDone || done.EndedAt == nil {
		t.Errorf("the finished run: %+v", done)
	}
}

func jobFailedRetries(t *testing.T, h Harness) {
	s := h.Store()
	for attempt := 1; attempt <= domain.JobMaxAttempts; attempt++ {
		run := job("s1", domain.JobMorningBrief, "k", "a")
		ok, err := s.JobRuns().Claim(bg(), run)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("attempt %d refused; the cap is %d", attempt, domain.JobMaxAttempts)
		}
		if err := s.JobRuns().Finish(bg(), run.ID, domain.JobFailed, "boom", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := s.JobRuns().Claim(bg(), job("s1", domain.JobMorningBrief, "k", "a")); err != nil || ok {
		t.Errorf("past the cap it must stop retrying: ok=%v err=%v", ok, err)
	}
}

func jobZombieFinish(t *testing.T, h Harness) {
	s := h.Store()
	zombie := job("s1", domain.JobRollingReplan, "k", "died")
	if ok, err := s.JobRuns().Claim(bg(), zombie); err != nil || !ok {
		t.Fatalf("claim: %v", err)
	}
	zombieID := zombie.ID
	h.BackdateJobRuns(t, "s1", domain.JobStaleAfter+time.Minute)

	fresh := job("s1", domain.JobRollingReplan, "k", "alive")
	if ok, err := s.JobRuns().Claim(bg(), fresh); err != nil || !ok {
		t.Fatalf("takeover: ok=%v err=%v", ok, err)
	}
	if fresh.ID == zombieID {
		t.Fatal("the takeover kept the old claim id; a late Finish from the dead instance would land on this row")
	}
	// The zombie finally comes back and reports failure.
	if err := s.JobRuns().Finish(bg(), zombieID, domain.JobFailed, "died", time.Now()); err != nil {
		t.Fatal(err)
	}
	runs, _ := s.JobRuns().List(bg(), "s1", 10)
	if len(runs) != 1 {
		t.Fatalf("%d rows, want 1 — a takeover must reuse the occurrence, not add one", len(runs))
	}
	if runs[0].Status != domain.JobRunning || runs[0].Instance != "alive" {
		t.Errorf("the zombie's late verdict landed on the live run: %+v", runs[0])
	}
}

func jobCrashTakeover(t *testing.T, h Harness) {
	s := h.Store()
	if ok, err := s.JobRuns().Claim(bg(), job("s1", domain.JobAutoPlan, "k", "i0")); err != nil || !ok {
		t.Fatalf("claim: %v", err)
	}
	steals := 0
	for i := 0; i < domain.JobMaxCrashAttempts+3; i++ {
		h.BackdateJobRuns(t, "s1", domain.JobStaleAfter+time.Minute)
		run := job("s1", domain.JobAutoPlan, "k", "next")
		ok, err := s.JobRuns().Claim(bg(), run)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		steals++
		// The caller's struct must carry the ROW's attempt count: a caller
		// deciding "was this the last try, should I tell the user the
		// integration is broken" reads it.
		if run.Attempts != steals+1 {
			t.Errorf("steal %d reported attempts=%d, want %d", steals, run.Attempts, steals+1)
		}
	}
	if steals == 0 {
		t.Error("a genuinely dead claim should be recoverable at least once")
	}
	if steals >= domain.JobMaxCrashAttempts+3 {
		t.Errorf("crash takeover never stopped: %d steals — a job that OOMs its instance would loop forever", steals)
	}
}

func jobPrune(t *testing.T, h Harness) {
	s := h.Store()
	done := job("s1", domain.JobAutoPlan, "old", "a")
	if _, err := s.JobRuns().Claim(bg(), done); err != nil {
		t.Fatal(err)
	}
	if err := s.JobRuns().Finish(bg(), done.ID, domain.JobDone, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	stuck := job("s1", domain.JobAutoPlan, "stuck", "a")
	if _, err := s.JobRuns().Claim(bg(), stuck); err != nil {
		t.Fatal(err)
	}
	h.BackdateJobRuns(t, "s1", 90*24*time.Hour)

	n, err := s.JobRuns().Prune(bg(), time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want only the finished one", n)
	}
	runs, _ := s.JobRuns().List(bg(), "s1", 10)
	// A "running" row older than the cutoff is the only trace of a job that
	// crashed and never came back, so Prune must leave it alone.
	if len(runs) != 1 || runs[0].RunKey != "stuck" {
		t.Errorf("the crashed run should survive the prune: %+v", runs)
	}
}

// ── proposals ───────────────────────────────────────────────────────────────

func proposalRoundTrip(t *testing.T, h Harness) {
	s := h.Store()
	dur := 45
	expires := time.Now().Add(2 * time.Hour).Truncate(time.Millisecond)
	in := &domain.Proposal{
		SessionID: "s1", Level: domain.LevelL1, Kind: domain.KindTimed,
		Title: "去跑步", Summary: "早上八点有个空档", Reason: "你前三天都在这个点跑",
		Evidence: "op:123", Date: "2026-07-27", Start: "08:00", Dur: &dur,
		BType: domain.BlockAppointment, LockLevel: domain.LockSoft, LockReason: "和别人约好的时间",
		Rows: []domain.ProposalRow{
			{ID: "r1", Label: "八点跑步", State: domain.ProposalPending},
			{ID: "r2", Label: "九点复习", State: domain.ProposalPending},
		},
		MergeKey: "morning-gap", TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: expires,
		Origin: domain.OriginDaemon, ThreadID: "th1",
		Ops: []domain.ProposalOp{{Tool: "plan_add", Args: map[string]any{"title": "跑步"}}},
	}
	mustCreate(t, s, in)

	got, err := s.Proposals().Get(bg(), "s1", in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != in.Title || got.Start != in.Start || got.Date != in.Date || got.Evidence != in.Evidence {
		t.Errorf("scalars: %+v", got)
	}
	if got.Dur == nil || *got.Dur != dur {
		t.Errorf("dur = %v, want %d", got.Dur, dur)
	}
	if got.LockLevel != domain.LockSoft || got.LockReason != in.LockReason {
		t.Errorf("lock: %q / %q", got.LockLevel, got.LockReason)
	}
	if len(got.Rows) != 2 || got.Rows[1].Label != "九点复习" {
		t.Errorf("rows: %+v", got.Rows)
	}
	if len(got.Ops) != 1 || got.Ops[0].Tool != "plan_add" {
		t.Errorf("ops: %+v", got.Ops)
	}
	if got.State != domain.ProposalPending || got.Rev != 1 {
		t.Errorf("state/rev = %q/%d", got.State, got.Rev)
	}
	if !got.ExpiresAt.Equal(expires.UTC()) {
		t.Errorf("expiresAt = %v, want %v", got.ExpiresAt, expires.UTC())
	}
	// Identity fields survive.
	if got.Level != domain.LevelL1 || got.Kind != domain.KindTimed || got.Origin != domain.OriginDaemon || got.ThreadID != "th1" {
		t.Errorf("identity: level=%q kind=%q origin=%q thread=%q", got.Level, got.Kind, got.Origin, got.ThreadID)
	}
}

func proposalNilEmpty(t *testing.T, h Harness) {
	s := h.Store()
	p := pending("s1", "什么都没带")
	mustCreate(t, s, p)
	got, err := s.Proposals().Get(bg(), "s1", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A nil slice comes back empty, not nil — the write path normalises it so no
	// caller needs a nil check the round trip promised to remove.
	if got.Rows == nil || len(got.Rows) != 0 {
		t.Errorf("Rows = %v, want an empty slice", got.Rows)
	}
	if got.Ops == nil || got.AppliedOpIDs == nil || got.AcceptOpIDs == nil {
		t.Errorf("slices came back nil: ops=%v applied=%v accept=%v", got.Ops, got.AppliedOpIDs, got.AcceptOpIDs)
	}
	// The three nullable timestamps stay nil. DeliveredAt nil is what "still in
	// the pool, never shown" means, so a spurious epoch would take the card out
	// of the outbox for good.
	if got.DeliveredAt != nil || got.PushedAt != nil || got.DeliverAfter != nil {
		t.Errorf("nil timestamps came back set: %+v", got)
	}

	// And a pointer to the ZERO time counts as absent, not as a value.
	p2 := pending("s1", "零值时间指针")
	var zero time.Time
	p2.DeliveredAt = &zero
	mustCreate(t, s, p2)
	back, _ := s.Proposals().Get(bg(), "s1", p2.ID)
	if back.DeliveredAt != nil && !back.DeliveredAt.IsZero() {
		t.Errorf("a zero-time pointer became %v", back.DeliveredAt)
	}
	pool, err := s.Proposals().List(bg(), domain.ProposalFilter{SessionID: "s1", Delivered: domain.PresenceUnset})
	if err != nil {
		t.Fatal(err)
	}
	if len(pool) != 2 {
		t.Errorf("the undelivered pool has %d of 2 — a zero delivered_at took a card out of it", len(pool))
	}
}

// The one an adversarial pass found. ProposalOp.Args is map[string]any, so its
// values are re-typed by whichever serialiser a backend uses: 45 comes back as
// float64 through encoding/json and as int32 through BSON, a list as
// []interface{} versus primitive.A. An agent tool doing
// args["minutes"].(float64) is correct on one store and a failed assertion on
// the other, and the service layer is written against whichever one the author
// happened to run.
func proposalArgTypes(t *testing.T, h Harness) {
	s := h.Store()
	p := pending("s1", "带参数的 op")
	p.Ops = []domain.ProposalOp{{Tool: "plan_add", Args: map[string]any{
		"minutes": 45,
		"nested":  map[string]any{"title": "跑步"},
		"list":    []any{1, "two"},
		"flag":    true,
		"text":    "字符串",
	}}}
	mustCreate(t, s, p)
	got, err := s.Proposals().Get(bg(), "s1", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Ops) != 1 {
		t.Fatalf("ops: %+v", got.Ops)
	}
	args := got.Ops[0].Args
	if v, ok := args["minutes"].(float64); !ok || v != 45 {
		t.Errorf("minutes came back as %T (%v), want float64(45) — the type every backend must agree on", args["minutes"], args["minutes"])
	}
	if _, ok := args["list"].([]any); !ok {
		t.Errorf("list came back as %T, want []any", args["list"])
	}
	nested, ok := args["nested"].(map[string]any)
	if !ok || nested["title"] != "跑步" {
		t.Errorf("nested came back as %T = %v", args["nested"], args["nested"])
	}
	if v, ok := args["flag"].(bool); !ok || !v {
		t.Errorf("flag came back as %T", args["flag"])
	}
	if v, ok := args["text"].(string); !ok || v != "字符串" {
		t.Errorf("text came back as %T (%v)", args["text"], args["text"])
	}
}

func proposalValidate(t *testing.T, h Harness) {
	s := h.Store()
	// Act-first with nothing to undo is the load-bearing invariant: a card whose
	// silence counts as consent must already have logged operations to reverse.
	act := pending("s1", "已经帮你挪好了")
	act.TTLPolicy = domain.TTLSilenceAccepts
	if err := s.Proposals().Create(bg(), act); !errors.Is(err, domain.ErrActFirstNeedsUndo) {
		t.Errorf("Create: want ErrActFirstNeedsUndo, got %v", err)
	}
	// No deadline: the first sweep would resolve it as silence, so it is born
	// dead and refused at construction.
	noTTL := pending("s1", "没有死线")
	noTTL.ExpiresAt = time.Time{}
	if err := s.Proposals().Create(bg(), noTTL); !errors.Is(err, domain.ErrNoExpiry) {
		t.Errorf("Create: want ErrNoExpiry, got %v", err)
	}

	// And the same rules hold on Update. Without that, one under-populated call
	// blanks ttl_policy — and a proposal whose policy is "" matches neither arm
	// of the expiry sweep, so it stays pending forever, invisible to the
	// mechanism meant to retire it.
	ok := pending("s1", "一张正常的卡")
	mustCreate(t, s, ok)
	live, err := s.Proposals().Get(bg(), "s1", ok.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		mut  func(*domain.Proposal)
	}{
		{"blank ttl policy", func(p *domain.Proposal) { p.TTLPolicy = "" }},
		{"blank title", func(p *domain.Proposal) { p.Title = "" }},
		{"zero expiry", func(p *domain.Proposal) { p.ExpiresAt = time.Time{} }},
		{"act-first with nothing to undo", func(p *domain.Proposal) {
			p.TTLPolicy, p.AppliedOpIDs = domain.TTLSilenceAccepts, nil
		}},
	} {
		bad := *live
		c.mut(&bad)
		if err := s.Proposals().Update(bg(), &bad); err == nil {
			t.Errorf("Update accepted %s", c.name)
		}
	}
	after, _ := s.Proposals().Get(bg(), "s1", ok.ID)
	if after.Rev != 1 || after.TTLPolicy != domain.TTLSilenceRejects || after.Title == "" {
		t.Errorf("a refused Update still wrote something: %+v", after)
	}
}

func proposalCAS(t *testing.T, h Harness) {
	s := h.Store()
	p := pending("s1", "两行卡")
	p.Rows = []domain.ProposalRow{{ID: "r1", Label: "一"}, {ID: "r2", Label: "二"}}
	mustCreate(t, s, p)
	a, _ := s.Proposals().Get(bg(), "s1", p.ID)
	b, _ := s.Proposals().Get(bg(), "s1", p.ID)

	a.Rows[0].State = domain.ProposalAccepted
	if err := s.Proposals().Update(bg(), a); err != nil {
		t.Fatal(err)
	}
	if a.Rev != 2 {
		t.Errorf("rev = %d after a successful write, want 2", a.Rev)
	}
	b.Rows[1].State = domain.ProposalRejected
	if err := s.Proposals().Update(bg(), b); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("the stale write returned %v, want ErrConflict", err)
	}
	fresh, _ := s.Proposals().Get(bg(), "s1", p.ID)
	if fresh.Rows[0].State != domain.ProposalAccepted || fresh.Rows[1].State == domain.ProposalRejected {
		t.Errorf("the losing write clobbered the winner: %+v", fresh.Rows)
	}
}

// Silence means yes for work already done and no for work not yet done. Getting
// this backwards either executes unattended work or silently discards an undo
// window.
func proposalExpiry(t *testing.T, h Harness) {
	s := h.Store()
	past := time.Now().Add(-time.Minute)
	ask := pending("s1", "要不要挪")
	ask.ExpiresAt = past
	mustCreate(t, s, ask)
	act := pending("s1", "已经挪好了")
	act.TTLPolicy, act.AppliedOpIDs, act.ExpiresAt = domain.TTLSilenceAccepts, []string{"op:1"}, past
	mustCreate(t, s, act)
	live := pending("s1", "还没到期")
	mustCreate(t, s, live)

	n, err := s.Proposals().Expire(bg(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("expired %d, want 2", n)
	}
	got, _ := s.Proposals().Get(bg(), "s1", ask.ID)
	if got.State != domain.ProposalExpired || got.Resolution != domain.ResolutionSilence {
		t.Errorf("ask-first lapsed to %s/%s, want expired/silence", got.State, got.Resolution)
	}
	got, _ = s.Proposals().Get(bg(), "s1", act.ID)
	if got.State != domain.ProposalAccepted || got.Resolution != domain.ResolutionSilence {
		t.Errorf("act-first lapsed to %s/%s, want accepted/silence", got.State, got.Resolution)
	}
	got, _ = s.Proposals().Get(bg(), "s1", live.ID)
	if got.State != domain.ProposalPending {
		t.Errorf("a live proposal was expired: %s", got.State)
	}
}

// Two daemons reacting to one trigger land in the same millisecond routinely.
// Retiring "everything that is not me" makes them annihilate each other and the
// user sees nothing; a strict created_at comparison leaves both alive. Exactly
// one must survive, whichever daemon calls first.
func proposalSupersedeTie(t *testing.T, h Harness) {
	s := h.Store()
	a := pending("s1", "daemon A 的卡")
	b := pending("s1", "daemon B 的卡")
	a.MergeKey, b.MergeKey = "gap", "gap"
	// Identical creation instants, which is what the millisecond clock produces
	// for two instances reacting to one trigger.
	at := time.Now().Truncate(time.Millisecond)
	a.CreatedAt, b.CreatedAt = at, at
	mustCreate(t, s, a)
	mustCreate(t, s, b)
	h.ForceProposalCreatedAt(t, "s1", at)

	if _, err := s.Proposals().Supersede(bg(), "s1", "gap", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Proposals().Supersede(bg(), "s1", "gap", b.ID); err != nil {
		t.Fatal(err)
	}
	live, err := s.Proposals().List(bg(), domain.ProposalFilter{SessionID: "s1", State: domain.ProposalPending})
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 {
		var titles []string
		for _, p := range live {
			titles = append(titles, p.Title)
		}
		t.Fatalf("%d survivors (%v), want exactly 1 even when the timestamps tie", len(live), titles)
	}
}

func proposalSupersedeDelivered(t *testing.T, h Harness) {
	s := h.Store()
	old := pending("s1", "旧的")
	old.MergeKey = "gap"
	mustCreate(t, s, old)
	shown := pending("s1", "已经给用户看过了")
	now := time.Now()
	shown.MergeKey, shown.DeliveredAt = "gap", &now
	mustCreate(t, s, shown)
	fresh := pending("s1", "新的")
	fresh.MergeKey = "gap"
	mustCreate(t, s, fresh)

	if _, err := s.Proposals().Supersede(bg(), "s1", "gap", fresh.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Proposals().Get(bg(), "s1", shown.ID)
	// Consensus 15 throttles delivery; it does not retract what was delivered.
	if got.State != domain.ProposalPending {
		t.Error("a card the user has already seen must not vanish from under them")
	}
}

// A keeper that no longer exists is nothing to merge against — the same
// non-event as an empty merge key. Both stores must say so identically, or a
// caller that aborts the delivery pass on ErrNotFound aborts on one backend and
// proceeds on the other for identical data.
func proposalSupersedeMissing(t *testing.T, h Harness) {
	s := h.Store()
	p := pending("s1", "孤零零一张")
	p.MergeKey = "gap"
	mustCreate(t, s, p)
	n, err := s.Proposals().Supersede(bg(), "s1", "gap", "an-id-that-never-existed")
	if err != nil {
		t.Errorf("want (0, nil), got err %v", err)
	}
	if n != 0 {
		t.Errorf("retired %d against a keeper that does not exist", n)
	}
}

// An unset delivered stamp on its own is not the outbox: it includes cards that lapsed in
// the pool and cards held back for later, neither of which the user may see. The
// filter has to exclude them in the query, because a limit applied before a
// caller-side filter can return an empty page while live cards wait behind it.
func proposalDeliverable(t *testing.T, h Harness) {
	s := h.Store()
	now := time.Now()
	live := pending("s1", "可投递")
	lapsed := pending("s1", "池子里过期了")
	lapsed.ExpiresAt = now.Add(-time.Minute)
	later := pending("s1", "压后再投")
	after := now.Add(time.Hour)
	later.DeliverAfter = &after
	for _, p := range []*domain.Proposal{live, lapsed, later} {
		mustCreate(t, s, p)
	}
	pool, err := s.Proposals().List(bg(), domain.ProposalFilter{
		SessionID: "s1", State: domain.ProposalPending, Delivered: domain.PresenceUnset, DeliverableAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pool) != 1 || pool[0].ID != live.ID {
		var titles []string
		for _, p := range pool {
			titles = append(titles, p.Title)
		}
		t.Errorf("deliverable set = %v, want just the live one", titles)
	}
	for _, p := range pool {
		if !p.Deliverable(now) {
			t.Errorf("%q came back from the query but Deliverable() says no", p.Title)
		}
	}
}

// A card whose ops will not serialise must fail the write. Storing it would mean
// a live-looking card that claims work and holds none: the user accepts it and
// nothing happens.
func proposalBadOps(t *testing.T, h Harness) {
	s := h.Store()
	p := pending("s1", "参数没法序列化")
	p.Ops = []domain.ProposalOp{{Tool: "plan_add", Args: map[string]any{"bad": math.NaN()}}}
	if err := s.Proposals().Create(bg(), p); err == nil {
		t.Error("want an error rather than a card that claims work it does not hold")
	}
	got, err := s.Proposals().List(bg(), domain.ProposalFilter{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("the failed write left %d rows behind", len(got))
	}
}

// ── rapport ─────────────────────────────────────────────────────────────────

func rapportRoundTrip(t *testing.T, h Harness) {
	s := h.Store()
	if _, err := s.Rapport().Get(bg(), "s1"); !errors.Is(err, domain.ErrNotFound) {
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
	if err := s.Rapport().Save(bg(), in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Rapport().Get(bg(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Scores[domain.OpDomainSchedule].Value != 0.34 || got.Scores[domain.OpDomainArchive].Evidence != 12 {
		t.Errorf("scores: %+v", got.Scores)
	}
	// The cursor is what makes the cache resumable rather than a number nobody
	// can rebuild. Losing it means re-folding the whole ledger every time.
	if !got.Cursor.CreatedAt.Equal(cursorAt.UTC()) || got.Cursor.ID != "op-99" {
		t.Errorf("cursor = %+v, want %v/op-99", got.Cursor, cursorAt.UTC())
	}
	if got.FoldVersion != 1 {
		t.Errorf("foldVersion = %d", got.FoldVersion)
	}

	in.Scores[domain.OpDomainSchedule] = domain.RapportScore{Value: 0.37, Evidence: 6}
	if err := s.Rapport().Save(bg(), in); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Rapport().Get(bg(), "s1")
	if got.Scores[domain.OpDomainSchedule].Evidence != 6 {
		t.Errorf("the second save did not overwrite: %+v", got.Scores)
	}
	if err := s.Rapport().Reset(bg(), "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rapport().Get(bg(), "s1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("after Reset the cache should be gone, got %v", err)
	}
}

// ── rhythm ──────────────────────────────────────────────────────────────────

// The row has two writers on wildly different cadences: the nightly learn job
// writes the profile, and every awake signal moves the live marks. A whole-row
// Save from the learn job would carry a snapshot taken before it started and
// erase a stretch that began in between — and a zero RunSince reads as "asleep",
// so the Protector would forget somebody had been up for nine hours.
func rhythmSaveVsTouch(t *testing.T, h Harness) {
	s := h.Store()
	stale := &domain.RhythmProfile{SessionID: "s1", Wake: "07:30", Sleep: "22:30", Source: "default"}
	if err := s.Rhythm().Save(bg(), stale); err != nil {
		t.Fatal(err)
	}
	runSince := time.Now().Add(-9 * time.Hour).Truncate(time.Millisecond)
	last := time.Now().Truncate(time.Millisecond)
	if err := s.Rhythm().Touch(bg(), "s1", runSince, last); err != nil {
		t.Fatal(err)
	}
	// The learn job finishes and writes what it computed, from the struct it read
	// before the signal landed.
	stale.Wake, stale.Sleep, stale.Source, stale.Days = "10:00", "02:30", "learned", 12
	if err := s.Rhythm().Save(bg(), stale); err != nil {
		t.Fatal(err)
	}
	got, err := s.Rhythm().Get(bg(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Wake != "10:00" || got.Sleep != "02:30" || got.Days != 12 || got.Source != "learned" {
		t.Errorf("the learned half did not land: %+v", got)
	}
	if !got.RunSince.Equal(runSince.UTC()) || !got.LastSignalAt.Equal(last.UTC()) {
		t.Errorf("the learn job erased the live stretch: runSince=%v want %v", got.RunSince, runSince.UTC())
	}
}

func rhythmTouchForward(t *testing.T, h Harness) {
	s := h.Store()
	runSince := time.Now().Add(-3 * time.Hour).Truncate(time.Millisecond)
	last := time.Now().Truncate(time.Millisecond)
	if err := s.Rhythm().Touch(bg(), "s1", runSince, last); err != nil {
		t.Fatal(err)
	}
	// A late-arriving older signal is a retry or a clock that stepped back;
	// letting it win would shorten a stretch that in fact continued.
	older := last.Add(-30 * time.Minute)
	if err := s.Rhythm().Touch(bg(), "s1", older, older); err != nil {
		t.Fatal(err)
	}
	got, err := s.Rhythm().Get(bg(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastSignalAt.Equal(last.UTC()) || !got.RunSince.Equal(runSince.UTC()) {
		t.Errorf("an out-of-order signal moved the marks: %+v", got)
	}
	// Touch on a session with no row seeds one — and does NOT invent body times.
	// The cold-start defaults live in internal/rhythm; a storage layer that wrote
	// them here would be a second place where "07:30" is recorded.
	if err := s.Rhythm().Touch(bg(), "s2", runSince, last); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.Rhythm().Get(bg(), "s2")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Wake != "" || fresh.Source != "" {
		t.Errorf("the seed row invented body times: %+v", fresh)
	}
	if !fresh.LastSignalAt.Equal(last.UTC()) {
		t.Errorf("the seed row lost the mark: %+v", fresh)
	}
}

func rhythmObserve(t *testing.T, h Harness) {
	s := h.Store()
	for _, m := range []int{600, 400, 900, 700} {
		if err := s.Rhythm().Observe(bg(), "s1", "2026-07-26", m); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Rhythm().Observe(bg(), "s1", "2026-07-27", 500); err != nil {
		t.Fatal(err)
	}
	days, err := s.Rhythm().Days(bg(), "s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("got %d days, want 2", len(days))
	}
	if days[0].Day != "2026-07-27" {
		t.Errorf("Days must be newest first, got %+v", days)
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
	// Minute zero is a MEANINGFUL value — midnight at the day cut — not an
	// absence, so it must widen the lower bound.
	if err := s.Rhythm().Observe(bg(), "s1", "2026-07-26", 0); err != nil {
		t.Fatal(err)
	}
	days, _ = s.Rhythm().Days(bg(), "s1", 10)
	for _, d := range days {
		if d.Day == "2026-07-26" && d.FirstMin != 0 {
			t.Errorf("minute 0 did not widen the bound: %+v", d)
		}
	}
	n, err := s.Rhythm().PruneDays(bg(), "s1", "2026-07-27")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want 1", n)
	}
}

// Two tabs sending the first signal of a new day at the same moment both find no
// row to widen. The loser must fold into the winner's row rather than surface a
// constraint error and drop the signal.
func rhythmObserveRace(t *testing.T, h Harness) {
	s := h.Store()
	const tabs = 6
	errs := make(chan error, tabs)
	start := make(chan struct{})
	for i := 0; i < tabs; i++ {
		go func(i int) {
			<-start
			errs <- s.Rhythm().Observe(bg(), "s1", "2026-07-27", 400+i)
		}(i)
	}
	close(start)
	for i := 0; i < tabs; i++ {
		if err := <-errs; err != nil {
			t.Errorf("Observe failed on a concurrent first write: %v", err)
		}
	}
	days, err := s.Rhythm().Days(bg(), "s1", 10)
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

// ── locale overrides ────────────────────────────────────────────────────────

func localeRoundTrip(t *testing.T, h Harness) {
	s := h.Store()
	for _, c := range [][3]string{
		{"mood.happy", "ja-JP", "うれしい"},
		{"mood.calm", "ja-JP", "おだやか"},
		{"mood.happy", "en-US", "Cheerful"},
	} {
		if err := s.Locales().Set(bg(), c[0], c[1], c[2]); err != nil {
			t.Fatal(err)
		}
	}
	// Setting the same key twice updates rather than duplicating.
	if err := s.Locales().Set(bg(), "mood.happy", "ja-JP", "うれしい！"); err != nil {
		t.Fatal(err)
	}
	all, err := s.Locales().All(bg())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d overrides, want 3: %+v", len(all), all)
	}
	for _, o := range all {
		if o.Key == "mood.happy" && o.Locale == "ja-JP" && o.Content != "うれしい！" {
			t.Errorf("the second Set did not overwrite: %q", o.Content)
		}
	}
	if err := s.Locales().Delete(bg(), "mood.happy", "en-US"); err != nil {
		t.Fatal(err)
	}
	n, err := s.Locales().DeleteLocale(bg(), "ja-JP")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("uninstalling a language removed %d rows, want 2", n)
	}
	all, _ = s.Locales().All(bg())
	if len(all) != 0 {
		t.Errorf("leftovers: %+v", all)
	}
}

func localeRace(t *testing.T, h Harness) {
	s := h.Store()
	const clicks = 4
	errs := make(chan error, clicks)
	start := make(chan struct{})
	for i := 0; i < clicks; i++ {
		go func() {
			<-start
			errs <- s.Locales().Set(bg(), "mood.happy", "ja-JP", "うれしい")
		}()
	}
	close(start)
	for i := 0; i < clicks; i++ {
		if err := <-errs; err != nil {
			t.Errorf("Set failed on a concurrent first write: %v", err)
		}
	}
	all, _ := s.Locales().All(bg())
	if len(all) != 1 || all[0].Content != "うれしい" {
		t.Errorf("got %+v, want exactly one row", all)
	}
}

// ── operation log ───────────────────────────────────────────────────────────

// The double-revert guard used to scan the most recent 200 entries, so a session
// busy enough to push a revert past the 200th row could undo the same operation
// twice — and a compensation is not idempotent.
func opLogRevertedBy(t *testing.T, h Harness) {
	s := h.Store()
	orig := &domain.OperationLog{
		SessionID: "s1", Actor: domain.ActorAgent, Action: "plan_add",
		Domain: domain.OpDomainSchedule, TargetID: "block-1", Summary: "加了一个块",
	}
	if err := s.OpLogs().Add(bg(), orig); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpLogs().RevertedBy(bg(), "s1", orig.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("nothing has reverted it yet, want ErrNotFound, got %v", err)
	}
	rev := &domain.OperationLog{
		SessionID: "s1", Actor: domain.ActorSystem, Action: "revert", TargetID: orig.ID,
	}
	if err := s.OpLogs().Add(bg(), rev); err != nil {
		t.Fatal(err)
	}
	got, err := s.OpLogs().RevertedBy(bg(), "s1", orig.ID)
	if err != nil {
		t.Fatalf("the revert should be found however long the log is: %v", err)
	}
	if got.ID != rev.ID {
		t.Errorf("found %q, want %q", got.ID, rev.ID)
	}
	// Another session's revert of the same target id must not count.
	if _, err := s.OpLogs().RevertedBy(bg(), "s2", orig.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("a revert leaked across sessions: %v", err)
	}
}

// Scan is the replay path: rapport is defined as a reading of the ledger, so its
// cache must be rebuildable by re-folding every operation in the order it
// happened. List cannot do that — it is newest-first and capped.
func opLogScan(t *testing.T, h Harness) {
	s := h.Store()
	for i := 0; i < 5; i++ {
		if err := s.OpLogs().Add(bg(), &domain.OperationLog{
			SessionID: "s1", Actor: domain.ActorUser, Action: "plan_add",
			Domain: domain.OpDomainSchedule, Summary: "op",
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // distinct created_at values
	}
	all, err := s.OpLogs().Scan(bg(), "s1", domain.OpLogCursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("scanned %d, want 5", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].CreatedAt.Before(all[i-1].CreatedAt) {
			t.Fatalf("Scan is not oldest-first: %v then %v", all[i-1].CreatedAt, all[i].CreatedAt)
		}
	}
	// Resuming from a cursor returns strictly what follows it, with no repeats
	// and no gaps — which is what makes an append-only log affordable to fold.
	cursor := domain.OpLogCursor{CreatedAt: all[1].CreatedAt, ID: all[1].ID}
	rest, err := s.OpLogs().Scan(bg(), "s1", cursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest) != 3 {
		t.Fatalf("resumed with %d, want 3", len(rest))
	}
	if rest[0].ID != all[2].ID {
		t.Errorf("resume started at %q, want %q", rest[0].ID, all[2].ID)
	}
}

// An empty canvas id is not "no key" — courses and assignments both carry a
// UNIQUE index on (session_id, canvas_id), so every keyless row wants the same
// slot. Without a guard the upsert finds the previous keyless row and overwrites
// it in place, keeping its id: two manual entries collapse into one, the first
// is gone, and nothing anywhere returns an error. Measured before the guard
// existed, that is exactly what happened.
//
// The four backends have to agree because the failure is invisible from above:
// a caller that got away with it on Mongo would lose rows the day someone moved
// the deployment to Postgres.
func upsertEmptyKey(t *testing.T, h Harness) {
	s := h.Store()
	if _, err := s.Assignments().UpsertByCanvasID(bg(), &domain.Assignment{
		SessionID: "s1", Title: "读第三章", Source: "manual",
	}); !errors.Is(err, domain.ErrMissingUpsertKey) {
		t.Errorf("assignment upsert with no canvas id: got %v, want ErrMissingUpsertKey", err)
	}
	if _, err := s.Courses().UpsertByCanvasID(bg(), &domain.Course{
		SessionID: "s1", Name: "软件设计",
	}); !errors.Is(err, domain.ErrMissingUpsertKey) {
		t.Errorf("course upsert with no canvas id: got %v, want ErrMissingUpsertKey", err)
	}
	// The refusal must not have written anything on its way out.
	list, err := s.Assignments().List(bg(), "s1", domain.AssignmentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("a refused upsert still wrote %d row(s)", len(list))
	}
}

// Every list method resolves its limit through domain.ListLimit, so the default
// page size and the ceiling are the same number on every backend.
//
// Both halves had gone wrong in the obvious ways. ListImports defaulted to 20
// rows on SQL and 50 on Mongo, so the same call returned different history
// depending on the deployment. And the ceiling — whose reason sqlstore's own
// limitClause comment states plainly, "an unbounded LIMIT reachable from a
// query parameter is a way to ask the server to materialise a whole table" —
// was applied to the four lists no query parameter reaches and to none of the
// three that a request drives straight through (/api/ops, /api/mood and the
// chat history endpoint all pass ?limit= down untouched).
//
// Seeding past the ceiling is what makes the second half of this test able to
// fail at all. An earlier draft seeded a dozen rows and asserted "no more than
// 500 came back", which is true whether or not the clamp exists — the assertion
// could not distinguish the fix from the bug. Rows beyond the ceiling are the
// only way to see it, so the suite pays for them.
func listLimitsAgree(t *testing.T, h Harness) {
	s := h.Store()
	const over = domain.MoodListMax + 1
	for i := 0; i < over; i++ {
		if _, err := s.Moods().Create(bg(), &domain.MoodCheckin{
			SessionID: "s1", Mood: "calm", Source: domain.MoodSourceUser,
		}); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	// Zero means "the usual page", and the usual page is the same everywhere.
	got, err := s.Moods().List(bg(), "s1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != domain.MoodListDefault {
		t.Errorf("default page = %d rows, want %d", len(got), domain.MoodListDefault)
	}
	// Over the ceiling is capped, not refused and not honoured: a backend that
	// forgot the clamp hands back all `over` rows here.
	got, err = s.Moods().List(bg(), "s1", domain.MoodListMax*1000)
	if err != nil {
		t.Fatalf("an over-large limit must be capped, not rejected: %v", err)
	}
	if len(got) != domain.MoodListMax {
		t.Errorf("asked for %d of %d stored rows and got %d back, ceiling is %d",
			domain.MoodListMax*1000, over, len(got), domain.MoodListMax)
	}
	// A limit inside the range is honoured verbatim — the clamp must not become
	// a fixed page size.
	if got, err = s.Moods().List(bg(), "s1", 2); err != nil || len(got) != 2 {
		t.Errorf("limit=2 returned %d rows (err=%v)", len(got), err)
	}
}

// Delete on moods and assignments is what makes the four capture tools
// undoable: reverting an agent-recorded check-in or an agent-created deadline
// means the row goes away, not that it acquires a "dismissed" state a user
// would then see.
//
// Three properties, and every one of them is somewhere the two stores could
// have drifted without anything noticing:
//
//   - Session scope. Both take (sessionID, id) and both must put the session in
//     the predicate. A delete that keys on id alone works identically in every
//     test that uses one session, and lets one user erase another's row in
//     production.
//   - ErrNotFound for a row that is not there, rather than nil. The revert path
//     ignores this error today, but "gone" and "never existed" are different
//     answers and the next caller may care.
//   - Idempotence in the sense that a second delete does not resurrect, corrupt
//     or panic — it just reports the same absence.
func deleteScopeAndAbsence(t *testing.T, h Harness) {
	s := h.Store()
	mine, err := s.Moods().Create(bg(), &domain.MoodCheckin{
		SessionID: "s1", Mood: "calm", Source: domain.MoodSourceAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Another session's row with the same shape — the one a mis-scoped delete
	// would take out.
	theirs, err := s.Moods().Create(bg(), &domain.MoodCheckin{
		SessionID: "s2", Mood: "calm", Source: domain.MoodSourceAgent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Moods().Delete(bg(), "s2", mine.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("deleting another session's id: got %v, want ErrNotFound", err)
	}
	if rows, _ := s.Moods().List(bg(), "s1", 10); len(rows) != 1 {
		t.Errorf("a cross-session delete removed the row anyway: %d left", len(rows))
	}
	if err := s.Moods().Delete(bg(), "s1", mine.ID); err != nil {
		t.Fatalf("deleting own row: %v", err)
	}
	if err := s.Moods().Delete(bg(), "s1", mine.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second delete: got %v, want ErrNotFound", err)
	}
	if rows, _ := s.Moods().List(bg(), "s2", 10); len(rows) != 1 || rows[0].ID != theirs.ID {
		t.Errorf("the other session lost its row")
	}

	// Same three properties for assignments, whose delete backs
	// revertAssignmentUpsert's create branch.
	a, err := s.Assignments().UpsertByCanvasID(bg(), &domain.Assignment{
		SessionID: "s1", CanvasID: "manual:x", Title: "读第三章", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Assignments().Delete(bg(), "s2", a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cross-session assignment delete: got %v, want ErrNotFound", err)
	}
	if err := s.Assignments().Delete(bg(), "s1", a.ID); err != nil {
		t.Fatalf("deleting own assignment: %v", err)
	}
	if err := s.Assignments().Delete(bg(), "s1", a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second assignment delete: got %v, want ErrNotFound", err)
	}
	if rows, _ := s.Assignments().List(bg(), "s1", domain.AssignmentFilter{}); len(rows) != 0 {
		t.Errorf("%d assignment(s) survived the delete", len(rows))
	}
}

// The three dimensions the attention ladder needed and the filter could not
// express: which rung a proposal is on, whether a nullable stamp is set, and
// how many match rather than how many fit on a page.
//
// Every call here carries SessionID because both backends put it in the
// predicate unconditionally — a filter that leaves it out matches nothing, and
// that is the intended behaviour rather than an oversight to work around.
func proposalFilterDimensions(t *testing.T, h Harness) {
	s := h.Store()
	now := time.Now()
	mk := func(title string, level domain.ProposalLevel) *domain.Proposal {
		p := pending("s1", title)
		p.Level = level
		return p
	}
	l3a, l3b, l2 := mk("推送甲", domain.LevelL3), mk("推送乙", domain.LevelL3), mk("卡片", domain.LevelL2)
	for _, p := range []*domain.Proposal{l3a, l3b, l2} {
		mustCreate(t, s, p)
	}
	// Another session's L3, to catch a predicate that forgot the session.
	other := mk("别人的推送", domain.LevelL3)
	other.SessionID = "s2"
	mustCreate(t, s, other)

	byLevel, err := s.Proposals().List(bg(), domain.ProposalFilter{SessionID: "s1", Level: domain.LevelL3})
	if err != nil {
		t.Fatal(err)
	}
	if len(byLevel) != 2 {
		t.Errorf("Level=L3 returned %d, want 2", len(byLevel))
	}
	n, err := s.Proposals().Count(bg(), domain.ProposalFilter{SessionID: "s1", Level: domain.LevelL3})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("Count(Level=L3) = %d, want 2", n)
	}
	if n, _ := s.Proposals().Count(bg(), domain.ProposalFilter{SessionID: "s1"}); n != 3 {
		t.Errorf("Count(session) = %d, want 3 — the other session leaked in", n)
	}

	// Count answers "how many match", not "how many fit on a page": a budget
	// that saturates at the page size is not a budget.
	if n, _ := s.Proposals().Count(bg(), domain.ProposalFilter{SessionID: "s1", Limit: 1}); n != 3 {
		t.Errorf("Count with Limit=1 = %d, want 3 — Limit must not reach Count", n)
	}

	// Push one of them and check all three answers about the stamp.
	pushed := now.Add(-time.Hour)
	l3a.PushedAt = &pushed
	if err := s.Proposals().Update(bg(), l3a); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		f    domain.ProposalFilter
		want int
	}{
		{"any", domain.ProposalFilter{SessionID: "s1", Level: domain.LevelL3}, 2},
		{"set", domain.ProposalFilter{SessionID: "s1", Level: domain.LevelL3, Pushed: domain.PresenceSet}, 1},
		{"unset", domain.ProposalFilter{SessionID: "s1", Level: domain.LevelL3, Pushed: domain.PresenceUnset}, 1},
	} {
		got, err := s.Proposals().Count(bg(), c.f)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("Pushed=%s: count %d, want %d", c.name, got, c.want)
		}
	}

	// The window is half-open, and it is what the budget actually counts —
	// presence alone would count every push this session has ever received,
	// because pushed_at is never cleared.
	start, end := domain.PushBudgetWindow(now, time.UTC)
	inWindow, err := s.Proposals().Count(bg(), domain.ProposalFilter{
		SessionID: "s1", PushedSince: start, PushedBefore: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	if inWindow != 1 {
		t.Errorf("pushes inside today's window = %d, want 1", inWindow)
	}
	// A window that ended before the push began must exclude it — the half-open
	// end is exclusive.
	if n, _ := s.Proposals().Count(bg(), domain.ProposalFilter{
		SessionID: "s1", PushedSince: start, PushedBefore: pushed,
	}); n != 0 {
		t.Errorf("PushedBefore is inclusive: got %d, want 0", n)
	}
}

// delivered_at is a permanent stamp, so "how many cards are in the stack" is a
// conjunction — pending, delivered, and still deliverable — not a question
// about the stamp alone. Asking only about presence would let a user see three
// cards in their life and then never again, which is why this case exists
// alongside the dimensions above rather than inside them.
func proposalStackIsAConjunction(t *testing.T, h Harness) {
	s := h.Store()
	now := time.Now()
	delivered := now.Add(-time.Hour)

	live := pending("s1", "还在堆叠里")
	live.DeliveredAt = &delivered
	mustCreate(t, s, live)

	answered := pending("s1", "早就答过了")
	answered.DeliveredAt = &delivered
	answered.State = domain.ProposalAccepted
	answered.Resolution = domain.ResolutionUser
	mustCreate(t, s, answered)

	lapsed := pending("s1", "投递过但已过期")
	lapsed.DeliveredAt = &delivered
	lapsed.ExpiresAt = now.Add(-time.Minute)
	mustCreate(t, s, lapsed)

	all, err := s.Proposals().Count(bg(), domain.ProposalFilter{
		SessionID: "s1", Delivered: domain.PresenceSet,
	})
	if err != nil {
		t.Fatal(err)
	}
	if all != 3 {
		t.Errorf("ever delivered = %d, want 3", all)
	}
	stack, err := s.Proposals().Count(bg(), domain.ProposalFilter{
		SessionID: "s1", State: domain.ProposalPending,
		Delivered: domain.PresenceSet, DeliverableAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stack != 1 {
		t.Errorf("in the stack right now = %d, want 1 (presence alone would say %d)", stack, all)
	}
}

// Prune bounds the one new table that had no way to shrink. Settled rows go;
// pending rows stay at any age, because an old pending row is one Expire has
// not reached yet and deleting it would drop a card the user was still owed.
func proposalPruneSparesPending(t *testing.T, h Harness) {
	s := h.Store()
	old := time.Now().Add(-72 * time.Hour)

	stillWaiting := pending("s1", "老但仍待答")
	mustCreate(t, s, stillWaiting)
	settled := pending("s1", "老且已答")
	settled.State = domain.ProposalRejected
	settled.Resolution = domain.ResolutionUser
	mustCreate(t, s, settled)
	fresh := pending("s1", "刚答的")
	fresh.State = domain.ProposalAccepted
	fresh.Resolution = domain.ResolutionUser
	mustCreate(t, s, fresh)

	// Backdate everything, then bring the recent one back to now through the
	// ordinary write path. Doing it in that order rather than backdating a
	// subset keeps the helper dumb (it has no filter) and still produces the
	// only arrangement that can distinguish a working Prune from a no-op: one
	// old pending, one old settled, one recent settled.
	h.BackdateProposals(t, "s1", old)
	again, err := s.Proposals().Get(bg(), "s1", fresh.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Proposals().Update(bg(), again); err != nil {
		t.Fatalf("refresh updated_at: %v", err)
	}

	n, err := s.Proposals().Prune(bg(), time.Now().Add(-48*time.Hour))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	left, err := s.Proposals().List(bg(), domain.ProposalFilter{SessionID: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]bool{}
	for _, p := range left {
		byTitle[p.Title] = true
	}
	if !byTitle["老但仍待答"] {
		t.Errorf("prune took a pending proposal — %d deleted, %d left", n, len(left))
	}
	if !byTitle["刚答的"] {
		t.Errorf("prune took a recently settled proposal")
	}
	if byTitle["老且已答"] {
		t.Errorf("prune left an old settled proposal behind — it deleted %d rows", n)
	}
	if n != 1 {
		t.Errorf("prune reported %d deletions, want 1", n)
	}
}

// OwnerInstance was stored by both backends and queryable by neither, so the
// crash sweep it exists for could not be written.
func proposalOwnerInstanceIsQueryable(t *testing.T, h Harness) {
	s := h.Store()
	mine, theirs := pending("s1", "本实例持有"), pending("s1", "别的实例持有")
	mine.OwnerInstance = "inst-a"
	theirs.OwnerInstance = "inst-b"
	mustCreate(t, s, mine)
	mustCreate(t, s, theirs)
	orphan := pending("s1", "无主")
	mustCreate(t, s, orphan)

	got, err := s.Proposals().List(bg(), domain.ProposalFilter{SessionID: "s1", OwnerInstance: "inst-b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "别的实例持有" {
		t.Fatalf("OwnerInstance filter returned %d rows: %+v", len(got), got)
	}
	// An empty OwnerInstance means "don't care", not "unowned" — otherwise the
	// console's view of one session would quietly become "the unclaimed ones".
	if n, _ := s.Proposals().Count(bg(), domain.ProposalFilter{SessionID: "s1"}); n != 3 {
		t.Errorf("empty OwnerInstance filtered instead of matching any: %d", n)
	}
}
