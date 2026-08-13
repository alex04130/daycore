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
	"fmt"
	"math"
	"strconv"
	"sync/atomic"
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
	// ForceAILogCreatedAt sets every AI call log in a session to one instant.
	//
	// Same purpose as ForceProposalCreatedAt and added for the same reason: the
	// paging cursor's whole job is to break a same-millisecond tie, and a test
	// that writes its rows milliseconds apart never produces one — so it passes
	// identically with the tie-break deleted. Measured: removing the `id` clause
	// from the SQL cursor left the suite green.
	//
	// A burst of AI calls really does land several rows in one millisecond
	// (parallel tool calls in one agent round), so this is the ordinary case
	// rather than a contrived one.
	ForceAILogCreatedAt(t *testing.T, sessionID string, at time.Time)
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
	{"Attachment/RoundTripAndSessionScope", attachmentRoundTripAndScope},
	{"Attachment/BindIsExclusiveAndIdempotent", attachmentBindOwnership},
	{"Attachment/HydrateIsOrderedAndScoped", attachmentHydrate},
	{"Attachment/DeleteReturnsTheRowAndSparesBound", attachmentDeleteReturnsRows},
	{"Attachment/DeleteByThreadTakesItsBytes", attachmentDeleteByThread},
	{"Attachment/PruneReclaimsOnlyUnsentUploads", attachmentPruneUnbound},
	{"Setting/OverrideRoundTripAndReset", settingRoundTrip},
	{"ProviderOverride/TriStateEnabledAndApprovalRevoke", providerOverrideRoundTrip},
	{"User/UpsertNeverTouchesTheOwnerMark", upsertNeverTouchesOwner},
	{"Role/MembershipAndDeleteTakesItsMembers", roleMembership},
	{"AILog/FilterAndBackwardPaging", aiLogFilterAndPaging},
	{"AILog/PruneIsByAgeAndReportsCount", aiLogPrune},
	{"Browser/PagesRedactsAndRefusesUnknownTables", browserPagesAndRedacts},
	{"Browser/DeleteIsByKeyAndRefusesCompositeTables", browserDelete},
	{"AIUsage/RollUpIsIdempotentAndSkipsToday", aiUsageRollUp},
	{"AIUsage/FoldsSurviveThePrune", aiUsageSurvivesPrune},
	{"AIUsage/ConcurrentFoldsDoNotCollide", aiUsageConcurrentFold},
	{"SessionUsage/ThreeScalesAndTumblingWindows", sessionUsageScales},
	{"Pairing/RoundTripRolesAndThrottledLastSeen", pairingRoundTrip},
	{"Frontend/FamilyUnionAndBuildFamilyStickiness", frontendRoundTrip},
	{"Theme/ScopedToFamilyAndDefaultedOnWrite", themeFamilyScope},
	{"ThemeKind/ApprovalRoundTripAndPendingCount", themeKindRoundTrip},
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
		Cursor:      domain.LogCursor{CreatedAt: cursorAt, ID: "op-99"},
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
	all, err := s.OpLogs().Scan(bg(), "s1", domain.LogCursor{}, 10)
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
	cursor := domain.LogCursor{CreatedAt: all[1].CreatedAt, ID: all[1].ID}
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
	//
	// ⚠️ Derived from the WINDOW, not from `now`. It used to be
	// `now.Add(-time.Hour)`, and that is only inside today's window when local
	// time is at least an hour past UTC midnight — so in any zone west of
	// Greenwich this case failed for one hour every day and passed the other
	// twenty-three. It was caught at 19:06 CDT, which is 00:06 UTC.
	//
	// The window is computed in UTC (PushBudgetWindow's second argument) while
	// `now` is local; anchoring the stamp to `start` removes the mismatch
	// entirely rather than making it rarer.
	start0, _ := domain.PushBudgetWindow(now, time.UTC)
	pushed := start0.Add(time.Minute)
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

// ── attachments (ε) ─────────────────────────────────────────────────────────

// att builds one upload with an id that increases on every call, which is not
// cosmetic — see below.
//
// ⚠️ THE ID IS A MONOTONIC COUNTER, AND THE ORDERING ASSERTION DEPENDS ON IT.
//
// The documented order is `(created_at, id)`. created_at is stored to the
// millisecond, and four inserts over a network round-trip usually land in four
// different ones — but not always. When two collide, the tie-break decides, and
// with a repository-generated random UUID the tie-break is a coin toss. That is
// what made Attachment/HydrateIsOrderedAndScoped fail about one Mongo run in
// five: not a bug in any back end, a test asserting an order the data could not
// carry.
//
// Sortable ids make the assertion deterministic in BOTH cases, which is
// strictly more than it checked before — it now pins the tie-break itself
// rather than avoiding the tie.
//
// ⚠️ The production gap this leaves visible, deliberately: real uploads DO get
// a random uuid, so two attachments landing in the same millisecond (a
// multi-file drop uploads in parallel) come back in an arbitrary order. Nothing
// downstream depends on it today. Closing it means a monotonic id, not a
// stronger sort — sorting harder cannot recover an order the row never stored.
var attSeq atomic.Int64

func att(sid, name, mime string) *domain.Attachment {
	return &domain.Attachment{
		// Zero-padded because the sort is lexicographic on a string column.
		ID:        fmt.Sprintf("att-%06d", attSeq.Add(1)),
		SessionID: sid, Ref: "ref-" + name, MIME: mime,
		Kind: domain.AttachmentKindOf(mime), Size: 11, SHA256: "abc", Filename: name,
	}
}

func mustAttach(t *testing.T, s domain.Store, a *domain.Attachment) *domain.Attachment {
	t.Helper()
	out, err := s.Attachments().Create(bg(), a)
	if err != nil {
		t.Fatalf("create attachment %q: %v", a.Filename, err)
	}
	return out
}

// A row with no ref is an owner record for nothing: every later check passes and
// the failure surfaces as a 404 at read time, which reads like a missing file
// rather than a bug at write time. Session scoping is the other half — the id is
// the only thing a client sends, so a Get that trusted it would hand any
// authenticated caller any upload in the installation.
func attachmentRoundTripAndScope(t *testing.T, h Harness) {
	s := h.Store()
	a := mustAttach(t, s, att("s1", "a.png", "image/png"))
	if a.ID == "" || a.CreatedAt.IsZero() {
		t.Fatalf("create returned %+v, want an id and a timestamp", a)
	}
	if a.Kind != domain.AttachmentImage {
		t.Errorf("kind = %q, want %q", a.Kind, domain.AttachmentImage)
	}
	got, err := s.Attachments().Get(bg(), "s1", a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Ref != a.Ref || got.MIME != a.MIME || got.Size != a.Size ||
		got.SHA256 != a.SHA256 || got.Filename != a.Filename {
		t.Errorf("round trip lost fields:\n got %+v\nwant %+v", *got, *a)
	}
	if got.Bound() {
		t.Error("a fresh upload reads as bound")
	}
	if _, err := s.Attachments().Get(bg(), "s2", a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("another session read the attachment: err=%v", err)
	}
	if _, err := s.Attachments().Create(bg(), &domain.Attachment{SessionID: "s1"}); err == nil {
		t.Error("an attachment with no ref was accepted")
	}
	if _, err := s.Attachments().Create(bg(), &domain.Attachment{Ref: "r"}); err == nil {
		t.Error("an attachment with no session was accepted")
	}
}

// One attachment on two messages would make "delete the message, delete its
// bytes" ambiguous, and the ambiguity shows up as either a leaked blob or a
// message rendering a broken image. Resending the same message must still work,
// which is why the same (id, message) pair is a no-op rather than a conflict.
func attachmentBindOwnership(t *testing.T, h Harness) {
	s := h.Store()
	mine := mustAttach(t, s, att("s1", "a.png", "image/png"))
	theirs := mustAttach(t, s, att("s2", "b.png", "image/png"))

	if err := s.Attachments().Bind(bg(), "s1", "t1", "m1", []string{mine.ID}); err != nil {
		t.Fatalf("bind: %v", err)
	}
	got, _ := s.Attachments().Get(bg(), "s1", mine.ID)
	if got.MessageID != "m1" || got.ThreadID != "t1" {
		t.Errorf("after bind: thread=%q message=%q", got.ThreadID, got.MessageID)
	}
	// Idempotent: the client resent the same message.
	if err := s.Attachments().Bind(bg(), "s1", "t1", "m1", []string{mine.ID}); err != nil {
		t.Errorf("rebinding to the same message should be a no-op, got %v", err)
	}
	// A second message may not take it.
	if err := s.Attachments().Bind(bg(), "s1", "t1", "m2", []string{mine.ID}); !errors.Is(err, domain.ErrAttachmentBound) {
		t.Errorf("binding to a second message: err=%v, want ErrAttachmentBound", err)
	}
	if got, _ := s.Attachments().Get(bg(), "s1", mine.ID); got.MessageID != "m1" {
		t.Errorf("the refused bind moved the attachment anyway: message=%q", got.MessageID)
	}
	// Another session's upload is invisible, so it is missing rather than taken.
	if err := s.Attachments().Bind(bg(), "s1", "t1", "m3", []string{theirs.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("binding a foreign attachment: err=%v, want ErrNotFound", err)
	}
	if got, _ := s.Attachments().Get(bg(), "s2", theirs.ID); got.Bound() {
		t.Error("a foreign session bound someone else's upload")
	}
	if err := s.Attachments().Bind(bg(), "s1", "t1", "", []string{mine.ID}); err == nil {
		t.Error("bind accepted an empty message id")
	}
	// The cap is enforced here and not only in the handler, because the agent
	// pipeline writes messages too.
	many := make([]string, domain.AttachmentsPerMessage+1)
	for i := range many {
		many[i] = mustAttach(t, s, att("s1", "x.png", "image/png")).ID
	}
	if err := s.Attachments().Bind(bg(), "s1", "t1", "m9", many); !errors.Is(err, domain.ErrTooManyAttachments) {
		t.Errorf("over the per-message cap: err=%v, want ErrTooManyAttachments", err)
	}
}

// Hydrating a page of messages is one query, ordered by upload time so a
// message's files render in the order the user picked them.
func attachmentHydrate(t *testing.T, h Harness) {
	s := h.Store()
	first := mustAttach(t, s, att("s1", "1.png", "image/png"))
	second := mustAttach(t, s, att("s1", "2.pdf", "application/pdf"))
	other := mustAttach(t, s, att("s1", "3.png", "image/png"))
	foreign := mustAttach(t, s, att("s2", "4.png", "image/png"))

	if err := s.Attachments().Bind(bg(), "s1", "t1", "m1", []string{first.ID, second.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.Attachments().Bind(bg(), "s1", "t1", "m2", []string{other.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.Attachments().Bind(bg(), "s2", "t9", "m1", []string{foreign.ID}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Attachments().ListByMessages(bg(), "s1", []string{"m1", "m2", "", "m1"})
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("hydrate returned %d rows, want 3 (duplicate and empty ids must not multiply or widen the result): %+v", len(got), got)
	}
	if got[0].ID != first.ID || got[1].ID != second.ID {
		t.Errorf("attachments came back out of upload order: %q then %q", got[0].Filename, got[1].Filename)
	}
	for _, a := range got {
		if a.SessionID != "s1" {
			t.Errorf("hydrate crossed sessions: %+v", a)
		}
	}
	if got, err := s.Attachments().ListByMessages(bg(), "s1", nil); err != nil || len(got) != 0 {
		t.Errorf("hydrating no messages returned %d rows (err=%v)", len(got), err)
	}
	// The composer's view: uploads not yet sent, newest first, mine only.
	pending := mustAttach(t, s, att("s1", "5.png", "image/png"))
	un, err := s.Attachments().ListUnbound(bg(), "s1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(un) != 1 || un[0].ID != pending.ID {
		t.Errorf("unbound list = %+v, want just the pending upload", un)
	}
}

// Delete returns the row because the caller — and only the caller — has the file
// bus. A repository that dropped the row and said "ok" would leak the bytes
// forever with nothing left in the database to find them by.
func attachmentDeleteReturnsRows(t *testing.T, h Harness) {
	s := h.Store()
	loose := mustAttach(t, s, att("s1", "loose.png", "image/png"))
	sent := mustAttach(t, s, att("s1", "sent.png", "image/png"))
	if err := s.Attachments().Bind(bg(), "s1", "t1", "m1", []string{sent.ID}); err != nil {
		t.Fatal(err)
	}

	gone, err := s.Attachments().Delete(bg(), "s1", loose.ID)
	if err != nil {
		t.Fatalf("delete unbound: %v", err)
	}
	if gone == nil || gone.Ref != loose.Ref {
		t.Fatalf("delete returned %+v, want the row so its bytes can go too", gone)
	}
	if _, err := s.Attachments().Get(bg(), "s1", loose.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("deleted attachment still readable: %v", err)
	}
	// A bound attachment belongs to a message now; the door out is the message.
	if _, err := s.Attachments().Delete(bg(), "s1", sent.ID); !errors.Is(err, domain.ErrAttachmentBound) {
		t.Errorf("deleting a bound attachment: err=%v, want ErrAttachmentBound", err)
	}
	if _, err := s.Attachments().Get(bg(), "s1", sent.ID); err != nil {
		t.Errorf("the refused delete removed it anyway: %v", err)
	}
	if _, err := s.Attachments().Delete(bg(), "s2", sent.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("cross-session delete: err=%v, want ErrNotFound", err)
	}
}

// Deleting a conversation takes its bytes with it. Without this, the only record
// of those blobs disappears with the messages and nothing can ever reclaim them.
func attachmentDeleteByThread(t *testing.T, h Harness) {
	s := h.Store()
	a := mustAttach(t, s, att("s1", "a.png", "image/png"))
	b := mustAttach(t, s, att("s1", "b.png", "image/png"))
	elsewhere := mustAttach(t, s, att("s1", "c.png", "image/png"))
	foreign := mustAttach(t, s, att("s2", "d.png", "image/png"))
	if err := s.Attachments().Bind(bg(), "s1", "t1", "m1", []string{a.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.Attachments().Bind(bg(), "s1", "t1", "m2", []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.Attachments().Bind(bg(), "s1", "t2", "m3", []string{elsewhere.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.Attachments().Bind(bg(), "s2", "t1", "m4", []string{foreign.ID}); err != nil {
		t.Fatal(err)
	}

	gone, err := s.Attachments().DeleteByThread(bg(), "s1", "t1")
	if err != nil {
		t.Fatalf("delete by thread: %v", err)
	}
	if len(gone) != 2 {
		t.Fatalf("returned %d rows, want the 2 that were removed: %+v", len(gone), gone)
	}
	for _, a := range gone {
		if a.Ref == "" {
			t.Error("a returned row has no ref, so its bytes cannot be deleted")
		}
	}
	if _, err := s.Attachments().Get(bg(), "s1", elsewhere.ID); err != nil {
		t.Errorf("another thread's attachment was removed: %v", err)
	}
	if _, err := s.Attachments().Get(bg(), "s2", foreign.ID); err != nil {
		t.Errorf("another session's thread t1 was removed: %v", err)
	}
	if gone, err := s.Attachments().DeleteByThread(bg(), "s1", "t1"); err != nil || len(gone) != 0 {
		t.Errorf("deleting an already-empty thread returned %d rows (err=%v)", len(gone), err)
	}
}

// An upload nobody sent is reclaimable; a sent one never is. The cutoff is
// absolute rather than a duration so the sweeper and the test agree on "old"
// without either of them waiting.
func attachmentPruneUnbound(t *testing.T, h Harness) {
	s := h.Store()
	loose := mustAttach(t, s, att("s1", "loose.png", "image/png"))
	sent := mustAttach(t, s, att("s1", "sent.png", "image/png"))
	if err := s.Attachments().Bind(bg(), "s1", "t1", "m1", []string{sent.ID}); err != nil {
		t.Fatal(err)
	}

	// Nothing is older than an hour ago yet.
	if gone, err := s.Attachments().PruneUnbound(bg(), time.Now().Add(-time.Hour)); err != nil || len(gone) != 0 {
		t.Fatalf("prune with an old cutoff removed %d rows (err=%v) — the cutoff is not being applied", len(gone), err)
	}
	if _, err := s.Attachments().Get(bg(), "s1", loose.ID); err != nil {
		t.Fatalf("the no-op prune removed the upload anyway: %v", err)
	}

	gone, err := s.Attachments().PruneUnbound(bg(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(gone) != 1 || gone[0].ID != loose.ID {
		t.Fatalf("prune returned %+v, want exactly the unsent upload", gone)
	}
	if gone[0].Ref == "" {
		t.Error("the pruned row has no ref, so its bytes stay on disk forever")
	}
	if _, err := s.Attachments().Get(bg(), "s1", loose.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("pruned attachment still readable: %v", err)
	}
	if _, err := s.Attachments().Get(bg(), "s1", sent.ID); err != nil {
		t.Errorf("prune reclaimed an attachment that belongs to a message: %v", err)
	}
}

// ── settings (θ-F4b) ────────────────────────────────────────────────────────

// The runtime half of configuration layering. Small surface, and every part of
// it is load-bearing for the console:
//
//   - Set is an upsert, because "change this again" is the normal case and the
//     console has no idea whether a row exists.
//   - Delete restores the environment seed, and MUST be distinguishable from
//     setting the value to "": for a string knob those are different requests,
//     and a backend that conflated them would make "reset to default" silently
//     mean "set to empty".
//   - Deleting what is not there is not an error. Two consoles clicking reset is
//     ordinary, and so is clicking it on a knob that was never overridden.
func settingRoundTrip(t *testing.T, h Harness) {
	s := h.Store()
	ctx := bg()

	if got, err := s.Settings().All(ctx); err != nil || len(got) != 0 {
		t.Fatalf("a fresh store has %d overrides (err=%v); nil must come back as an empty slice", len(got), err)
	}
	if err := s.Settings().Set(ctx, "MaxUploadBytes", "1048576"); err != nil {
		t.Fatal(err)
	}
	if err := s.Settings().Set(ctx, "AgentMaxRounds", "3"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings().All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("All returned %d, want 2: %+v", len(got), got)
	}
	byKey := map[string]domain.Setting{}
	for _, x := range got {
		byKey[x.Key] = x
	}
	if byKey["MaxUploadBytes"].Value != "1048576" {
		t.Errorf("round trip lost the value: %+v", byKey["MaxUploadBytes"])
	}
	if byKey["AgentMaxRounds"].UpdatedAt.IsZero() {
		t.Error("no updated_at; the console shows when a setting last changed")
	}

	// Upsert, not insert-or-fail.
	if err := s.Settings().Set(ctx, "AgentMaxRounds", "6"); err != nil {
		t.Fatalf("re-setting an existing key failed: %v", err)
	}
	got, _ = s.Settings().All(ctx)
	if len(got) != 2 {
		t.Errorf("re-setting created a second row: %+v", got)
	}

	// An empty value is a value.
	if err := s.Settings().Set(ctx, "DefaultVisionModel", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Settings().All(ctx)
	var sawEmpty bool
	for _, x := range got {
		if x.Key == "DefaultVisionModel" {
			sawEmpty = true
		}
	}
	if !sawEmpty {
		t.Error("setting a knob to the empty string stored nothing — that is a different request from resetting it")
	}

	// Delete restores the seed, and is boring when repeated.
	if err := s.Settings().Delete(ctx, "AgentMaxRounds"); err != nil {
		t.Fatal(err)
	}
	if err := s.Settings().Delete(ctx, "AgentMaxRounds"); err != nil {
		t.Errorf("deleting an absent override errored: %v", err)
	}
	if err := s.Settings().Delete(ctx, "never-set"); err != nil {
		t.Errorf("resetting a knob that was never overridden errored: %v", err)
	}
	got, _ = s.Settings().All(ctx)
	for _, x := range got {
		if x.Key == "AgentMaxRounds" {
			t.Error("delete did not remove the override")
		}
	}

	// An empty key is a row nothing can ever read back.
	if err := s.Settings().Set(ctx, "", "x"); err == nil {
		t.Error("an empty key was accepted")
	}
}

// providerOverrideRoundTrip covers the console-editable half of a capability
// source. Three properties, and each one is a behaviour a back end could
// plausibly get wrong on its own:
//
//   - Enabled is TRI-state. nil ("no opinion, use the file"), false ("off") and
//     true are three different answers, and a back end that stores a Go bool
//     collapses the first two — which silently turns "I never touched this" into
//     "I turned it off" for every source in a fresh deployment.
//   - The key is composite. Two capabilities may each have a source called
//     "primary"; a back end keyed on id alone lets one overwrite the other.
//   - Editing a description must not carry its approval along. The hash is what
//     approval was granted against, and the whole gate rests on the store
//     round-tripping it faithfully.
func providerOverrideRoundTrip(t *testing.T, h Harness) {
	ctx := bg()
	repo := h.Store().ProviderOverrides()

	on, off := true, false
	if err := repo.Set(ctx, domain.ProviderOverride{
		Kind: "weather", ID: "primary", Enabled: &off,
		Description:     map[string]string{"zh-CN": "内网天气", "en-US": "intranet weather"},
		DescriptionHash: "h1", Approved: true,
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Same id, different capability: must be a separate row.
	if err := repo.Set(ctx, domain.ProviderOverride{Kind: "search", ID: "primary", Enabled: &on}); err != nil {
		t.Fatalf("set second kind: %v", err)
	}
	// No opinion at all — neither on nor off.
	if err := repo.Set(ctx, domain.ProviderOverride{Kind: "channel", ID: "qq"}); err != nil {
		t.Fatalf("set third: %v", err)
	}

	all, err := repo.All(ctx)
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("All returned %d rows, want 3 — (kind, id) is the key, so weather/primary and search/primary are different rows", len(all))
	}
	byKey := map[string]domain.ProviderOverride{}
	for _, o := range all {
		byKey[o.Kind+"/"+o.ID] = o
	}

	w := byKey["weather/primary"]
	if w.Enabled == nil || *w.Enabled {
		t.Errorf("weather/primary Enabled = %v, want an explicit false", w.Enabled)
	}
	if w.Description["zh-CN"] != "内网天气" || w.Description["en-US"] != "intranet weather" {
		t.Errorf("description did not round-trip: %v", w.Description)
	}
	if w.DescriptionHash != "h1" || !w.Approved {
		t.Errorf("approval did not round-trip: hash=%q approved=%v", w.DescriptionHash, w.Approved)
	}
	if e := byKey["search/primary"].Enabled; e == nil || !*e {
		t.Errorf("search/primary Enabled = %v, want true", e)
	}
	if e := byKey["channel/qq"].Enabled; e != nil {
		t.Errorf("channel/qq Enabled = %v, want nil — 'no opinion' and 'off' are different answers", *e)
	}

	// Editing the description must not carry the old approval with it. The store
	// only has to persist what it is given, but a back end that ignores a field
	// on update would leave a source approved against text nobody read.
	if err := repo.Set(ctx, domain.ProviderOverride{
		Kind: "weather", ID: "primary", Enabled: &off,
		Description:     map[string]string{"zh-CN": "改过了", "en-US": "edited"},
		DescriptionHash: "h2", Approved: false,
	}); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	all, _ = repo.All(ctx)
	if len(all) != 3 {
		t.Errorf("re-setting an existing (kind, id) inserted a row instead of updating: %d rows", len(all))
	}
	for _, o := range all {
		if o.Kind == "weather" && o.ID == "primary" {
			if o.Approved || o.DescriptionHash != "h2" || o.Description["en-US"] != "edited" {
				t.Errorf("the edit did not land: %+v", o)
			}
		}
	}

	// Clearing the opinion has to be expressible, or the console can never undo
	// a change it made.
	if err := repo.Set(ctx, domain.ProviderOverride{Kind: "search", ID: "primary"}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	all, _ = repo.All(ctx)
	for _, o := range all {
		if o.Kind == "search" && o.Enabled != nil {
			t.Errorf("Enabled could not be cleared back to nil: %v", *o.Enabled)
		}
	}

	if err := repo.Delete(ctx, "weather", "primary"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.Delete(ctx, "weather", "primary"); err != nil {
		t.Errorf("deleting an absent override is not idempotent: %v", err)
	}
	if err := repo.Set(ctx, domain.ProviderOverride{Kind: "", ID: "x"}); err == nil {
		t.Error("an override with no kind was accepted; it would be unreachable by every reader")
	}
	if all, _ = repo.All(ctx); len(all) != 2 {
		t.Errorf("after one delete: %d rows, want 2", len(all))
	}
}

// The OAuth callback builds a *domain.User out of what the provider returned and
// hands it to Upsert. So if is_owner ever appears in the column list — or if a
// back end starts replacing the whole document instead of $set-ing a field list
// — **logging in becomes a privilege escalation write**.
//
// This is a convention today (TokenVersion and DataSessionID are protected the
// same way, and nothing tests them). It gets a conformance case because the
// consequence is not "a field gets clobbered": it is that a stranger with a
// Google account can become the super-administrator, and because the Mongo
// variant of the mistake is invisible on SQL — the four back ends would not
// fail together.
func upsertNeverTouchesOwner(t *testing.T, h Harness) {
	s := h.Store()
	ctx := bg()
	repo := s.Users()

	created, err := repo.Upsert(ctx, &domain.User{Name: strPtr("someone")})
	if err != nil {
		t.Fatal(err)
	}
	if created.IsOwner {
		t.Fatal("a newly created user is an owner")
	}
	if err := repo.SetOwner(ctx, created.ID, true); err != nil {
		t.Fatalf("SetOwner: %v", err)
	}
	if got, _ := repo.GetByID(ctx, created.ID); got == nil || !got.IsOwner {
		t.Fatal("SetOwner did not stick")
	}

	// The escalation shape, both directions.
	//
	// A login carrying IsOwner:false must not CLEAR the mark (that is a
	// self-inflicted lockout: the last owner logs in through Google and stops
	// being an owner), and one carrying IsOwner:true must not SET it.
	if _, err := repo.Upsert(ctx, &domain.User{ID: created.ID, Name: strPtr("renamed"), IsOwner: false}); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.GetByID(ctx, created.ID)
	if after == nil || !after.IsOwner {
		t.Error("Upsert with IsOwner:false cleared the owner mark — a login would demote the last owner")
	}
	if after.Name == nil || *after.Name != "renamed" {
		t.Error("Upsert did not apply the fields it IS supposed to write")
	}

	other, err := repo.Upsert(ctx, &domain.User{Name: strPtr("stranger")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Upsert(ctx, &domain.User{ID: other.ID, Name: strPtr("stranger"), IsOwner: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetByID(ctx, other.ID)
	if got == nil || got.IsOwner {
		t.Error("Upsert with IsOwner:true made somebody an owner — this is the OAuth callback's shape exactly")
	}

	// And the list the console needs, with the mark on it.
	users, err := repo.List(ctx, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) < 2 {
		t.Fatalf("List returned %d users, want at least 2", len(users))
	}
	owners := 0
	for _, u := range users {
		if u.IsOwner {
			owners++
		}
	}
	if owners != 1 {
		t.Errorf("List reports %d owners, want exactly 1 — every 'is this the last owner' check reads this", owners)
	}
}

// Roles, membership, and the two properties that are only interesting because
// getting them wrong is silent.
func roleMembership(t *testing.T, h Harness) {
	s := h.Store()
	ctx := bg()
	repo := s.Roles()

	if got, err := repo.ListRoles(ctx); err != nil || len(got) != 0 {
		t.Fatalf("a fresh store has %d roles (err=%v); nil must come back as an empty slice", len(got), err)
	}
	if _, err := repo.GetRole(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetRole on a missing role returned %v, want ErrNotFound", err)
	}

	// A role with no permissions is a plain group — a commercial tier — and that
	// is a meaningful state, not an unfinished one. It has to round-trip as an
	// empty slice rather than nil, because IsAdminRole() is what the assignment
	// check reads.
	if err := repo.UpsertRole(ctx, domain.Role{Name: "tier-free", Description: "免费档"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertRole(ctx, domain.Role{
		Name: "support", Description: "客服", Permissions: []string{"users.read", "overview.read"},
	}); err != nil {
		t.Fatal(err)
	}
	roles, err := repo.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 || roles[0].Name != "support" || roles[1].Name != "tier-free" {
		t.Fatalf("ListRoles = %+v, want two sorted by name", roles)
	}
	if roles[1].IsAdminRole() {
		t.Error("a role with no permissions reports as an admin role — the assignment check reads exactly this")
	}
	if !roles[0].IsAdminRole() || len(roles[0].Permissions) != 2 {
		t.Errorf("permissions did not round-trip: %+v", roles[0])
	}

	// Membership, and its idempotence: two consoles clicking the same button is
	// ordinary and must not be an error.
	if err := repo.AddMember(ctx, "support", "u-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(ctx, "support", "u-1"); err != nil {
		t.Errorf("adding an existing member is not idempotent: %v", err)
	}
	if err := repo.AddMember(ctx, "support", "u-2"); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(ctx, "tier-free", "u-1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.MembersOf(ctx, "support"); len(got) != 2 || got[0] != "u-1" || got[1] != "u-2" {
		t.Errorf("MembersOf = %v, want [u-1 u-2] — 'who can export the database' is this query", got)
	}
	if got, _ := repo.RolesOf(ctx, "u-1"); len(got) != 2 || got[0] != "support" || got[1] != "tier-free" {
		t.Errorf("RolesOf = %v, want both roles sorted", got)
	}

	if err := repo.RemoveMember(ctx, "support", "u-2"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveMember(ctx, "support", "u-2"); err != nil {
		t.Errorf("removing an absent member is not idempotent: %v", err)
	}

	// Deleting a role takes its membership with it.
	//
	// Otherwise the rows survive granting nothing — until somebody creates a
	// role with the same name, at which point a set of people SILENTLY regain a
	// permission set that was deleted. Nothing reports that, and "support" is
	// exactly the kind of name that gets recreated.
	if err := repo.DeleteRole(ctx, "support"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteRole(ctx, "support"); err != nil {
		t.Errorf("deleting an absent role is not idempotent: %v", err)
	}
	if got, _ := repo.MembersOf(ctx, "support"); len(got) != 0 {
		t.Errorf("deleting a role left %v behind — recreating the name would silently restore them", got)
	}
	if got, _ := repo.RolesOf(ctx, "u-1"); len(got) != 1 || got[0] != "tier-free" {
		t.Errorf("RolesOf after the delete = %v, want only tier-free", got)
	}

	if err := repo.UpsertRole(ctx, domain.Role{Name: ""}); err == nil {
		t.Error("a role with no name was accepted; nothing could ever reference it")
	}
	if err := repo.AddMember(ctx, "tier-free", ""); err == nil {
		t.Error("a membership with no user was accepted")
	}
}

func strPtr(s string) *string { return &s }

// ── AI call ledger ──────────────────────────────────────────────────────────

// aiLog builds one ledger row. created_at is stamped by the repository, so the
// only way to get distinguishable timestamps is to write them apart in time —
// which the paging case does deliberately.
func aiLog(sid, endpoint, model, status string) *domain.AICallLog {
	return &domain.AICallLog{
		SessionID: sid, Endpoint: endpoint, Model: model, Status: status,
		PromptTokens: 100, CompTokens: 20, DurationMs: 1234,
	}
}

// Every filter dimension narrows, they combine with AND, the order is newest
// first, and the backward cursor pages without repeating or dropping a row.
//
// The dimensions are checked ONE AT A TIME and then together. A filter test
// that only ever sets one field passes just as well when the builder ignores
// every field but the first — which is precisely the bug this back end has
// shipped before (a second assignment to the same bson key silently dropping
// the first clause).
func aiLogFilterAndPaging(t *testing.T, h Harness) {
	s := h.Store()
	repo := s.AILogs()
	ctx := bg()

	// Written oldest-first with a real gap, because created_at is stored to the
	// millisecond and the assertion below is about ORDER.
	want := []*domain.AICallLog{
		aiLog("s1", "companion", "glm-5", "ok"),
		aiLog("s1", "brief", "glm-5", "error"),
		aiLog("s2", "companion", "deepseek-v4", "ok"),
		aiLog("s1", "companion", "glm-5", "ok"),
	}
	for _, l := range want {
		if err := repo.Add(ctx, l); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	all, err := repo.List(ctx, domain.AILogFilter{}, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("unfiltered list returned %d rows, want 4", len(all))
	}
	// Newest first. Without this the paging assertion below is meaningless.
	for i := 1; i < len(all); i++ {
		if all[i].CreatedAt.After(all[i-1].CreatedAt) {
			t.Fatalf("row %d is newer than row %d — the list is not newest-first", i, i-1)
		}
	}
	if all[0].ID != want[3].ID || all[3].ID != want[0].ID {
		t.Errorf("newest-first order is wrong: got %q first and %q last", all[0].Endpoint, all[3].Endpoint)
	}
	// Every field survives the round trip. A ledger that loses the model or the
	// token counts is a ledger nobody can bill or debug from.
	got := all[3]
	if got.SessionID != "s1" || got.Endpoint != "companion" || got.Model != "glm-5" ||
		got.Status != "ok" || got.PromptTokens != 100 || got.CompTokens != 20 || got.DurationMs != 1234 {
		t.Errorf("round trip lost something: %+v", got)
	}

	// Each dimension alone.
	for _, tc := range []struct {
		name string
		f    domain.AILogFilter
		want int
	}{
		{"session", domain.AILogFilter{SessionID: "s1"}, 3},
		{"endpoint", domain.AILogFilter{Endpoint: "companion"}, 3},
		{"model", domain.AILogFilter{Model: "deepseek-v4"}, 1},
		{"status", domain.AILogFilter{Status: "error"}, 1},
		{"no match", domain.AILogFilter{Model: "nothing-called-this"}, 0},
	} {
		rows, err := repo.List(ctx, tc.f, 0)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(rows) != tc.want {
			t.Errorf("filter by %s returned %d rows, want %d", tc.name, len(rows), tc.want)
		}
	}
	// And together — AND, not OR. s2's row is "companion" too, so an OR would
	// return four here and a builder that keeps only the last clause would
	// return three.
	if rows, _ := repo.List(ctx, domain.AILogFilter{SessionID: "s1", Endpoint: "companion", Status: "ok"}, 0); len(rows) != 2 {
		t.Errorf("three filters combined returned %d rows, want 2 — they must AND", len(rows))
	}

	// Since is a lower bound, inclusive of its own instant.
	if rows, _ := repo.List(ctx, domain.AILogFilter{Since: all[1].CreatedAt}, 0); len(rows) != 2 {
		t.Errorf("Since returned %d rows, want the newest 2", len(rows))
	}

	// Backward paging: two pages of two, no repeat, no gap. This is the
	// assertion the cursor exists for — a page boundary that only compared
	// timestamps would repeat or drop whenever two rows share a millisecond.
	first, err := repo.List(ctx, domain.AILogFilter{}, 2)
	if err != nil || len(first) != 2 {
		t.Fatalf("first page: %d rows, err=%v", len(first), err)
	}
	cursor := domain.LogCursor{CreatedAt: first[1].CreatedAt, ID: first[1].ID}
	second, err := repo.List(ctx, domain.AILogFilter{Before: cursor}, 2)
	if err != nil || len(second) != 2 {
		t.Fatalf("second page: %d rows, err=%v", len(second), err)
	}
	seen := map[string]bool{}
	for _, l := range append(append([]domain.AICallLog{}, first...), second...) {
		if seen[l.ID] {
			t.Errorf("row %s came back on both pages", l.ID)
		}
		seen[l.ID] = true
	}
	if len(seen) != 4 {
		t.Errorf("two pages of two covered %d distinct rows, want all 4", len(seen))
	}
	// Past the end is empty, not an error and not a wrap-around.
	last := second[len(second)-1]
	if rows, err := repo.List(ctx, domain.AILogFilter{Before: domain.LogCursor{CreatedAt: last.CreatedAt, ID: last.ID}}, 2); err != nil || len(rows) != 0 {
		t.Errorf("paging past the oldest row returned %d rows (err=%v)", len(rows), err)
	}

	// ── the tie the cursor exists for ──────────────────────────────────────
	//
	// Everything above is written milliseconds apart, so created_at alone
	// separates every row and the id clause is never consulted. Deleting that
	// clause leaves all of it green — measured. So: force all four rows to ONE
	// instant and page through them again.
	//
	// This is the ordinary case, not a contrived one. A single agent round
	// firing parallel tool calls writes several ledger rows inside one
	// millisecond.
	oneInstant := time.Now().Add(-time.Minute)
	h.ForceAILogCreatedAt(t, "s1", oneInstant)
	h.ForceAILogCreatedAt(t, "s2", oneInstant)

	tied, err := repo.List(ctx, domain.AILogFilter{}, 0)
	if err != nil || len(tied) != 4 {
		t.Fatalf("after forcing one instant: %d rows, err=%v", len(tied), err)
	}
	for i := 1; i < len(tied); i++ {
		if !tied[i].CreatedAt.Equal(tied[0].CreatedAt) {
			t.Fatalf("the harness did not put every row on one instant; this assertion proves nothing")
		}
		// With the timestamps equal, the id is the ONLY thing left to order by,
		// and the order must still be total and descending.
		if tied[i].ID >= tied[i-1].ID {
			t.Errorf("rows sharing a millisecond came back in id order %q then %q — the tie-break is not descending",
				tied[i-1].ID, tied[i].ID)
		}
	}
	seenTied := map[string]bool{}
	cur := domain.LogCursor{}
	for page := 0; page < 4; page++ {
		got, err := repo.List(ctx, domain.AILogFilter{Before: cur}, 1)
		if err != nil {
			t.Fatalf("tied page %d: %v", page, err)
		}
		if len(got) != 1 {
			t.Fatalf("tied page %d returned %d rows; four rows in one millisecond must still page one at a time", page, len(got))
		}
		if seenTied[got[0].ID] {
			t.Fatalf("tied page %d repeated %s — the cursor is comparing only the timestamp", page, got[0].ID)
		}
		seenTied[got[0].ID] = true
		cur = domain.LogCursor{CreatedAt: got[0].CreatedAt, ID: got[0].ID}
	}
	if len(seenTied) != 4 {
		t.Errorf("paging four same-millisecond rows one at a time saw %d of them", len(seenTied))
	}
	if rows, _ := repo.List(ctx, domain.AILogFilter{Before: cur}, 1); len(rows) != 0 {
		t.Errorf("paging past the last of four tied rows returned %d rows", len(rows))
	}
	// And the cursor composes with a filter rather than replacing it.
	fp, _ := repo.List(ctx, domain.AILogFilter{SessionID: "s1"}, 1)
	if len(fp) != 1 {
		t.Fatalf("filtered first page: %d rows", len(fp))
	}
	fq, _ := repo.List(ctx, domain.AILogFilter{
		SessionID: "s1",
		Before:    domain.LogCursor{CreatedAt: fp[0].CreatedAt, ID: fp[0].ID},
	}, 10)
	if len(fq) != 2 {
		t.Errorf("cursor plus filter returned %d rows, want 2 — the cursor dropped the filter", len(fq))
	}
	for _, l := range fq {
		if l.SessionID != "s1" {
			t.Errorf("cursor plus filter crossed sessions: %+v", l)
		}
	}
}

// Prune deletes by age and says how many went.
//
// The count is not decoration: it is what the leader logs, and a prune that
// silently reports zero while deleting rows is indistinguishable from a prune
// that is not running — which is how a table grows without bound while
// somebody believes it is being kept.
func aiLogPrune(t *testing.T, h Harness) {
	s := h.Store()
	repo := s.AILogs()
	ctx := bg()

	// Add stamps created_at itself, so an "old" row has to be written and then
	// pruned against a boundary in the future rather than backdated.
	for i := 0; i < 3; i++ {
		if err := repo.Add(ctx, aiLog("s1", "companion", "glm-5", "ok")); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(5 * time.Millisecond)
	boundary := time.Now()
	time.Sleep(5 * time.Millisecond)
	keep := aiLog("s1", "brief", "glm-5", "ok")
	if err := repo.Add(ctx, keep); err != nil {
		t.Fatal(err)
	}

	n, err := repo.Prune(ctx, boundary)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 3 {
		t.Errorf("prune reported %d rows, want 3", n)
	}
	rows, _ := repo.List(ctx, domain.AILogFilter{}, 0)
	if len(rows) != 1 || rows[0].ID != keep.ID {
		t.Errorf("prune left %+v, want only the row newer than the boundary", rows)
	}
	// Pruning again takes nothing and is not an error — it runs on a ticker.
	if n, err := repo.Prune(ctx, boundary); err != nil || n != 0 {
		t.Errorf("second prune reported %d rows (err=%v), want 0", n, err)
	}
}

// ── the console's table browser ─────────────────────────────────────────────

// Paging, ordering, redaction, counts, and the refusal that makes the whole
// thing an allowlist rather than a validation.
//
// The two back ends reach the same answers by genuinely different routes — SQL
// returns a fixed column list, Mongo returns documents whose keys it has to
// union — so this is exactly the kind of thing that drifts without a shared
// suite.
func browserPagesAndRedacts(t *testing.T, h Harness) {
	s := h.Store()
	b := s.Browser()
	ctx := bg()

	// A table nobody catalogued is refused, and the refusal is NOT "zero rows".
	// Zero is a real answer; conflating the two is how a typo in a URL becomes
	// "that table is empty".
	if _, err := b.CountRows(ctx, "sqlite_master"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("counting an uncatalogued table returned %v, want ErrNotFound", err)
	}
	if _, err := b.BrowseRows(ctx, "users; DROP TABLE users", 10, 0); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("browsing a name that is not in the catalogue returned %v, want ErrNotFound", err)
	}
	if _, err := b.DeleteRow(ctx, "not_a_table", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("deleting from an uncatalogued table returned %v, want ErrNotFound", err)
	}

	// Empty is empty, not an error, and the page still carries its shape.
	empty, err := b.BrowseRows(ctx, "ai_call_logs", 10, 0)
	if err != nil {
		t.Fatalf("browsing an empty table: %v", err)
	}
	if empty.Total != 0 || len(empty.Rows) != 0 {
		t.Errorf("empty table reported total=%d rows=%d", empty.Total, len(empty.Rows))
	}

	// Five rows, written in order, so "newest first" is checkable.
	for i := 0; i < 5; i++ {
		if err := s.AILogs().Add(ctx, aiLog("s-browse", "companion", "m"+strconv.Itoa(i), "ok")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	if n, err := b.CountRows(ctx, "ai_call_logs"); err != nil || n != 5 {
		t.Errorf("CountRows = %d (err=%v), want 5", n, err)
	}

	first, err := b.BrowseRows(ctx, "ai_call_logs", 2, 0)
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if first.Total != 5 {
		t.Errorf("Total = %d, want 5 — the pager needs the whole count, not the page size", first.Total)
	}
	if len(first.Rows) != 2 {
		t.Fatalf("first page had %d rows, want 2", len(first.Rows))
	}
	// Column names, not positions: the two back ends order columns differently
	// and the console reads them by name.
	col := func(p *domain.TableRowPage, name string) int {
		for i, c := range p.Columns {
			if c == name || (name == "id" && c == "_id") {
				return i
			}
		}
		t.Fatalf("page has no %q column; got %v", name, p.Columns)
		return -1
	}
	modelAt := col(first, "model")
	if first.Rows[0][modelAt] != "m4" || first.Rows[1][modelAt] != "m3" {
		t.Errorf("first page is %v then %v, want m4 then m3 — the browser is not newest-first",
			first.Rows[0][modelAt], first.Rows[1][modelAt])
	}
	// Offset paging: page two continues rather than restarting.
	second, err := b.BrowseRows(ctx, "ai_call_logs", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Rows) != 2 || second.Rows[0][col(second, "model")] != "m2" {
		t.Errorf("second page starts at %v, want m2", second.Rows[0][col(second, "model")])
	}
	// Past the end is an empty page with the real total, not an error — the
	// console renders "0 of 5 on this page" rather than an error card.
	past, err := b.BrowseRows(ctx, "ai_call_logs", 2, 99)
	if err != nil || len(past.Rows) != 0 || past.Total != 5 {
		t.Errorf("past the end: %d rows, total %d, err=%v", len(past.Rows), past.Total, err)
	}

	// Redaction. sessions.import_token is a bearer credential — whoever holds it
	// can push data into that session from anywhere — and the browser must never
	// serve it. This is the assertion that would otherwise silently stop being
	// true when somebody renames a column.
	sid := "s-secret"
	if _, err := s.Sessions().GetOrCreate(ctx, sid); err != nil {
		t.Fatal(err)
	}
	// The token has to be SET, not merely declared.
	//
	// Mongo omits empty fields (`bson:"import_token,omitempty"`), so a fresh
	// session has no such key at all and the browser's union-of-keys never sees
	// one — which made this whole assertion vacuous on that back end while
	// passing on SQL, where the column always exists. Caught by the real-machine
	// run, which is what it is for.
	token := "tok-do-not-serve-this"
	if _, err := s.Sessions().Update(ctx, sid, domain.SessionUpdate{ImportToken: &token}); err != nil {
		t.Fatal(err)
	}
	page, err := b.BrowseRows(ctx, "sessions", 50, 0)
	if err != nil {
		t.Fatalf("browse sessions: %v", err)
	}
	if len(page.Rows) == 0 {
		t.Fatal("no session rows came back; the redaction assertion below would prove nothing")
	}
	tokenAt := -1
	for i, c := range page.Columns {
		if c == "import_token" {
			tokenAt = i
		}
	}
	if tokenAt < 0 {
		t.Fatal("sessions has no import_token column in the browsed page — redaction is not being exercised")
	}
	for _, row := range page.Rows {
		if row[tokenAt] != domain.RedactedValue {
			t.Errorf("import_token came back as %v, want the redaction marker — this is a bearer credential", row[tokenAt])
		}
	}
	// And the value is nowhere else in the row either, in case a back end ever
	// returns the same field twice under two keys.
	for _, row := range page.Rows {
		for i, v := range row {
			if str, ok := v.(string); ok && str == token {
				t.Errorf("the import token was served in column %q", page.Columns[i])
			}
		}
	}
	// And the rest of the row is intact: redacting the whole table would make
	// the browser useless exactly where debugging needs it.
	idAt := -1
	for i, c := range page.Columns {
		if c == "id" || c == "_id" {
			idAt = i
		}
	}
	if idAt < 0 || page.Rows[0][idAt] == nil || page.Rows[0][idAt] == "" {
		t.Errorf("the session id did not survive alongside the redacted column: %v", page.Rows[0])
	}
}

// Deleting one row, and refusing to when "one row" has no meaning.
func browserDelete(t *testing.T, h Harness) {
	s := h.Store()
	b := s.Browser()
	ctx := bg()

	l := aiLog("s-del", "companion", "m", "ok")
	if err := s.AILogs().Add(ctx, l); err != nil {
		t.Fatal(err)
	}
	other := aiLog("s-del", "brief", "m", "ok")
	if err := s.AILogs().Add(ctx, other); err != nil {
		t.Fatal(err)
	}

	ok, err := b.DeleteRow(ctx, "ai_call_logs", l.ID)
	if err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}
	// Reporting whether a row was there is the difference between "deleted" and
	// "there was nothing to delete", and the console says different things.
	if ok, err := b.DeleteRow(ctx, "ai_call_logs", l.ID); err != nil || ok {
		t.Errorf("deleting the same row twice reported ok=%v err=%v, want false and no error", ok, err)
	}
	// It deleted ONE row, not the table.
	if n, _ := b.CountRows(ctx, "ai_call_logs"); n != 1 {
		t.Errorf("after deleting one of two rows the table has %d", n)
	}

	// A table whose key is composite refuses, and the refusal is its own error —
	// mapping it to ErrNotFound would tell an operator the row is gone when it
	// is still there.
	if _, err := b.DeleteRow(ctx, "roles", "anything"); !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("deleting from a composite-key table returned %v, want ErrUnsupported", err)
	}
	// roles in particular: DeleteRole removes the definition AND the membership,
	// and a raw delete of just the definition leaves rows that silently restore
	// a deleted permission set the moment the name is reused.
	if tb, _ := domain.TableByName("roles"); tb.Deletable() {
		t.Error("roles became deletable from the browser; that bypasses the delete-both invariant in domain/role.go")
	}
}

// ── the AI spend rollup ─────────────────────────────────────────────────────

// Folding closed days, leaving today alone, and producing the same rows when
// run again.
//
// Idempotence is the property the whole design rests on: there is no "have I
// already counted this day" bookkeeping anywhere, so if a second run
// double-counted, every figure on the console would drift upward by however
// many times the job happened to fire. Nothing would report that.
func aiUsageRollUp(t *testing.T, h Harness) {
	s := h.Store()
	repo := s.AILogs()
	ctx := bg()

	// Two closed days and one open one. Add stamps created_at itself, so the
	// rows are written now and then moved — the same trick the paging case uses,
	// for the same reason.
	write := func(sid, endpoint, model, status string) *domain.AICallLog {
		l := aiLog(sid, endpoint, model, status)
		l.PromptTokens, l.CompTokens = 10, 3
		if err := repo.Add(ctx, l); err != nil {
			t.Fatal(err)
		}
		return l
	}
	now := time.Now().UTC()
	todayKey := domain.UTCDay(now)
	d1 := now.AddDate(0, 0, -2)
	d2 := now.AddDate(0, 0, -1)

	write("day1", "companion", "glm-5", "ok")
	write("day1", "companion", "glm-5", "error")
	write("day1", "brief", "glm-5", "ok")
	h.ForceAILogCreatedAt(t, "day1", d1)

	write("day2", "companion", "deepseek-v4", "ok")
	h.ForceAILogCreatedAt(t, "day2", d2)

	// Today's row stays where it is.
	write("today", "companion", "glm-5", "ok")

	n, err := repo.RollUpUsage(ctx, todayKey)
	if err != nil {
		t.Fatalf("roll up: %v", err)
	}
	if n != 2 {
		t.Fatalf("folded %d days, want 2 (the two closed ones)", n)
	}

	byModel := func(rows []domain.AIUsageDay, model string) *domain.AIUsageDay {
		for i := range rows {
			if rows[i].Model == model {
				return &rows[i]
			}
		}
		return nil
	}

	// Today is NOT in the rollup. If it were, the console would double-count it
	// against the live figure it adds on top — and only on days somebody
	// happened to look after the job ran.
	days, err := repo.UsageDays(ctx, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range days {
		if d.Day == todayKey {
			t.Errorf("today (%s) was folded; only closed days may be", todayKey)
		}
	}
	if len(days) != 2 {
		t.Fatalf("the rollup has %d days, want 2: %+v", len(days), days)
	}

	// The counters are right, per day and across the fold.
	first := domain.UTCDay(d1)
	for _, d := range days {
		if d.Day != first {
			continue
		}
		if d.Calls != 3 || d.Errors != 1 || d.PromptTokens != 30 || d.CompTokens != 9 {
			t.Errorf("%s folded to %+v, want 3 calls / 1 error / 30 prompt / 9 comp", d.Day, d)
		}
	}

	totals, err := repo.UsageTotals(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if totals.Calls != 4 || totals.Errors != 1 || totals.PromptTokens != 40 || totals.CompTokens != 12 {
		t.Errorf("totals = %+v, want 4 calls / 1 error / 40 prompt / 12 comp", totals)
	}
	// FirstDay is what keeps the figure honest on screen: it says where the
	// number actually starts rather than implying "all time".
	if totals.FirstDay != first {
		t.Errorf("FirstDay = %q, want %q", totals.FirstDay, first)
	}

	// Per-model, folded ACROSS days.
	models, err := repo.UsageByModel(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("per-model fold returned %d rows, want 2: %+v", len(models), models)
	}
	if m := byModel(models, "glm-5"); m == nil || m.Calls != 3 {
		t.Errorf("glm-5 folded to %+v, want 3 calls", m)
	}
	if m := byModel(models, "deepseek-v4"); m == nil || m.Calls != 1 {
		t.Errorf("deepseek-v4 folded to %+v, want 1 call", m)
	}

	// ── run it again ────────────────────────────────────────────────────────
	//
	// Every figure is unchanged. A second fold that ADDED to the counters would
	// inflate the console by however often the job happened to fire, silently —
	// which is why there is no incrementing anywhere in this design.
	again, err := repo.RollUpUsage(ctx, todayKey)
	if err != nil {
		t.Fatal(err)
	}
	// Exactly ONE day is rewritten: the newest folded one, deliberately revisited
	// every run so a partially-written day cannot be skipped forever. Everything
	// older is left alone.
	if again != 1 {
		t.Errorf("a second fold wrote %d days; it must rewrite exactly the newest one", again)
	}
	after, err := repo.UsageTotals(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if after.Calls != totals.Calls || after.CompTokens != totals.CompTokens {
		t.Errorf("running the fold twice changed the totals: %+v then %+v", totals, after)
	}

	// A window narrows, and both bounds are inclusive.
	win, err := repo.UsageTotals(ctx, first, first)
	if err != nil {
		t.Fatal(err)
	}
	if win.Calls != 3 {
		t.Errorf("one-day window = %d calls, want 3 — the bounds must be inclusive", win.Calls)
	}
	if empty, _ := repo.UsageTotals(ctx, "2000-01-01", "2000-01-02"); empty.Calls != 0 || empty.FirstDay != "" {
		t.Errorf("an empty window returned %+v, want zeroes and no first day", empty)
	}
}

// A day survives the prune that deletes the rows it was folded from.
//
// ⚠️ This is the assertion behind the ordering rule. Roll up first, then prune:
// the prune deletes the ledger rows the fold reads, so a prune that ran first
// would silently drop a day from history forever, and the only symptom would be
// a total that is lower than it was yesterday.
func aiUsageSurvivesPrune(t *testing.T, h Harness) {
	s := h.Store()
	repo := s.AILogs()
	ctx := bg()

	old := aiLog("old", "companion", "glm-5", "ok")
	old.PromptTokens, old.CompTokens = 7, 5
	if err := repo.Add(ctx, old); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	longAgo := now.AddDate(0, 0, -100)
	h.ForceAILogCreatedAt(t, "old", longAgo)

	if _, err := repo.RollUpUsage(ctx, domain.UTCDay(now)); err != nil {
		t.Fatal(err)
	}
	before, err := repo.UsageTotals(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if before.Calls != 1 {
		t.Fatalf("the day was not folded before the prune: %+v", before)
	}

	// ⚠️ A row is left behind on a LATER day, deliberately. A live deployment
	// always has recent calls, and an empty ledger takes a short-circuit that
	// hides the case this is really about: the rollup's newest day being OLDER
	// than the ledger's oldest surviving row. Without the clamp to the ledger,
	// the fold would revisit that day, recompute it from rows that no longer
	// exist, and write zero over real history.
	recent := aiLog("recent", "companion", "glm-5", "ok")
	if err := repo.Add(ctx, recent); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Prune(ctx, now.AddDate(0, 0, -90)); err != nil {
		t.Fatal(err)
	}
	// The old ledger row is gone and only the recent one is left…
	rows, _ := repo.List(ctx, domain.AILogFilter{}, 0)
	if len(rows) != 1 {
		t.Fatalf("the prune left %d ledger rows, want just the recent one", len(rows))
	}
	// …and the history is not.
	after, err := repo.UsageTotals(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if after.Calls != 1 || after.PromptTokens != 7 || after.CompTokens != 5 {
		t.Errorf("pruning the ledger took the folded history with it: %+v", after)
	}
	// And a fold after the prune does not resurrect or zero that day — the loop
	// starts after the newest folded day, so a day whose rows are gone is never
	// revisited.
	if _, err := repo.RollUpUsage(ctx, domain.UTCDay(now)); err != nil {
		t.Fatalf("fold after prune: %v", err)
	}
	final, _ := repo.UsageTotals(ctx, "", "")
	if final.Calls != 1 || final.PromptTokens != 7 || final.CompTokens != 5 {
		t.Errorf("a fold run after the prune changed a day whose rows are gone: %+v", final)
	}
}

// Two instances folding the same day at the same moment.
//
// The lease means this should not happen, and the lease rests on clocks — which
// is exactly the reasoning leader.go already applies to every other job: nothing
// whose correctness matters may rest on the lease alone.
//
// # What is guaranteed, and what is not
//
// GUARANTEED: the day ends up correct, and it is never left empty. Whichever
// interleaving happens, the last INSERT to succeed wrote the whole day from the
// ledger, and the ledger did not change while they ran.
//
// NOT guaranteed: that both calls return nil. DELETE-then-INSERT is two
// statements, so on an engine with real write concurrency the two can interleave
// as DELETE/DELETE/INSERT/INSERT and the loser hits the primary key. ⚠️ Measured
// on real Postgres — SQLite serialises writers and hides it entirely, which is
// what this suite exists to stop.
//
// A losing fold is NOISY, NOT WRONG: it returns an error, and the job's
// fail-stop then skips the prune, so the day is simply refolded on the next run.
// Making both succeed would need a per-dialect upsert (three spellings of ON
// CONFLICT) to remove a collision the lease already makes vanishingly rare —
// paying a permanent complexity cost for a transient log line.

func aiUsageConcurrentFold(t *testing.T, h Harness) {
	s := h.Store()
	repo := s.AILogs()
	ctx := bg()

	for i := 0; i < 4; i++ {
		l := aiLog("race", "companion", "glm-5", "ok")
		l.PromptTokens, l.CompTokens = 6, 2
		if err := repo.Add(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	h.ForceAILogCreatedAt(t, "race", yesterday)
	today := domain.UTCDay(time.Now())

	errs := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := repo.RollUpUsage(ctx, today)
			errs <- err
		}()
	}
	close(start)
	failed := 0
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			failed++
		}
	}
	// At most one may lose. Both failing would mean neither wrote the day, and
	// the fail-stop would then keep skipping the prune forever.
	if failed > 1 {
		t.Errorf("both concurrent folds failed; nobody wrote the day")
	}

	// And the answer is right — not doubled, not missing, not empty — whichever
	// of them won. Idempotence is what makes "both ran" harmless; without it the
	// totals would depend on how many instances happened to be up.
	totals, err := repo.UsageTotals(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if totals.Calls != 4 || totals.PromptTokens != 24 || totals.CompTokens != 8 {
		t.Errorf("after two concurrent folds: %+v, want 4 calls / 24 prompt / 8 comp", totals)
	}
	days, _ := repo.UsageDays(ctx, "", "", 0)
	if len(days) != 1 {
		t.Errorf("two folds produced %d day rows, want 1", len(days))
	}
}

// Per-account usage: three scales on the session row, windows that tumble.
//
// The property that matters and is easy to lose: the LIFETIME total never
// resets while the two windows do. A change that reset all three — or none —
// would still look right on a screen for a whole window, and the number nobody
// notices is exactly the one a quota rests on.
func sessionUsageScales(t *testing.T, h Harness) {
	s := h.Store()
	repo := s.Sessions()
	ctx := bg()
	sid := "usage-sid"
	if _, err := repo.GetOrCreate(ctx, sid); err != nil {
		t.Fatal(err)
	}

	// A fresh session counts nothing and has no open window.
	fresh, err := repo.Get(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Usage.TotalCalls != 0 || !fresh.Usage.FastExpired(time.Now()) {
		t.Fatalf("a fresh session already has usage: %+v", fresh.Usage)
	}

	base := time.Now().UTC()
	if err := repo.AddUsage(ctx, sid, 100, 20, base); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddUsage(ctx, sid, 10, 5, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	u := got.Usage
	if u.TotalCalls != 2 || u.TotalPromptTokens != 110 || u.TotalCompTokens != 25 {
		t.Errorf("lifetime = %d calls / %d prompt / %d comp, want 2 / 110 / 25",
			u.TotalCalls, u.TotalPromptTokens, u.TotalCompTokens)
	}
	// The windows carry COMBINED tokens — the split is a lifetime-only fact,
	// because it changes no decision a window is read to make.
	if u.FastCalls != 2 || u.FastTokens != 135 {
		t.Errorf("fast window = %d calls / %d tokens, want 2 / 135", u.FastCalls, u.FastTokens)
	}
	if u.SlowCalls != 2 || u.SlowTokens != 135 {
		t.Errorf("slow window = %d calls / %d tokens, want 2 / 135", u.SlowCalls, u.SlowTokens)
	}
	// The window opened at the FIRST call, not the most recent one. A stamp that
	// moved with every call would be a window that never expires on an active
	// account — which is the one account anybody would want it to.
	if u.FastWindowStart.IsZero() || u.FastWindowStart.After(base.Add(time.Second)) {
		t.Errorf("fast window opened at %v, want the first call (%v)", u.FastWindowStart, base)
	}

	// ── the fast window tumbles, the slow one does not ──────────────────────
	later := base.Add(domain.UsageFastWindow + time.Minute)
	if err := repo.AddUsage(ctx, sid, 1, 1, later); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Get(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	u = got.Usage
	if u.FastCalls != 1 || u.FastTokens != 2 {
		t.Errorf("after the fast window expired: %d calls / %d tokens, want the new call alone (1 / 2)",
			u.FastCalls, u.FastTokens)
	}
	if u.SlowCalls != 3 || u.SlowTokens != 137 {
		t.Errorf("the slow window rolled with the fast one: %d calls / %d tokens, want 3 / 137",
			u.SlowCalls, u.SlowTokens)
	}
	if u.TotalCalls != 3 || u.TotalPromptTokens != 111 || u.TotalCompTokens != 26 {
		t.Errorf("the lifetime total reset with a window: %d / %d / %d, want 3 / 111 / 26",
			u.TotalCalls, u.TotalPromptTokens, u.TotalCompTokens)
	}
	// The new window opened at the call that started it.
	if u.FastWindowStart.Before(later.Add(-time.Second)) {
		t.Errorf("the fast window kept its old stamp (%v) after tumbling", u.FastWindowStart)
	}

	// ── and the slow one tumbles too, on its own schedule ───────────────────
	muchLater := base.Add(domain.UsageSlowWindow + time.Hour)
	if err := repo.AddUsage(ctx, sid, 2, 3, muchLater); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Get(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Usage.SlowCalls != 1 || got.Usage.SlowTokens != 5 {
		t.Errorf("after the slow window expired: %d calls / %d tokens, want 1 / 5",
			got.Usage.SlowCalls, got.Usage.SlowTokens)
	}
	if got.Usage.TotalCalls != 4 {
		t.Errorf("lifetime = %d, want 4 — it must never reset", got.Usage.TotalCalls)
	}

	// ── a stale window reads as zero without being written ─────────────────
	//
	// Nothing runs on expiry; the reset happens on the next WRITE. So a reader
	// that trusted the stored counter would report a burst from three hours ago
	// as current. Live() is what every reader must go through.
	// Four hours on: the fast window has rolled, the slow one has not. Both
	// directions in one assertion, because a Live() that zeroed everything would
	// pass a check that only looked at the expired one.
	soon := got.Usage.Live(muchLater.Add(domain.UsageFastWindow + time.Hour))
	if soon.FastCalls != 0 {
		t.Errorf("an expired fast window read as %d calls, want zero", soon.FastCalls)
	}
	if soon.SlowCalls != 1 || soon.SlowTokens != 5 {
		t.Errorf("Live() zeroed a slow window that is still open: %d calls / %d tokens, want 1 / 5",
			soon.SlowCalls, soon.SlowTokens)
	}
	// Eight days on, both are gone and the lifetime total is not.
	stale := got.Usage.Live(muchLater.Add(domain.UsageSlowWindow + time.Hour))
	if stale.FastCalls != 0 || stale.SlowCalls != 0 {
		t.Errorf("an expired window read as %d fast / %d slow, want zeroes", stale.FastCalls, stale.SlowCalls)
	}
	if stale.TotalCalls != 4 || stale.TotalPromptTokens == 0 {
		t.Errorf("Live() zeroed the lifetime total: %+v", stale)
	}

	// An empty session id is refused rather than updating nothing quietly.
	if err := repo.AddUsage(ctx, "", 1, 1, base); err == nil {
		t.Error("AddUsage accepted an empty session id")
	}
	// An unknown session is a no-op, not an error: the ledger row is written
	// even when the session has since been deleted, and failing here would turn
	// that into a logged error on every call.
	if err := repo.AddUsage(ctx, "no-such-session", 1, 1, base); err != nil {
		t.Errorf("AddUsage on an unknown session errored: %v", err)
	}
}

// ── pairings: external consoles attached to this deployment ─────────────────

// Round trip, role replacement, revocation, and the throttled last-seen.
//
// The throttle is the part worth a shared test: it is expressed as a CONDITION
// INSIDE the write on both back ends (a WHERE clause / a filter), because doing
// it in Go would be read-then-write on the authorisation path of the busiest
// external console. A back end that dropped the condition would still pass every
// functional check while writing on every single request.
func pairingRoundTrip(t *testing.T, h Harness) {
	s := h.Store()
	repo := s.Pairings()
	ctx := bg()

	if _, err := repo.Get(ctx, "nobody"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get on an unknown pairing returned %v, want ErrNotFound", err)
	}
	// An empty id is refused rather than matching whatever comes first — this is
	// on the authorisation path, so "no id" must never resolve to a credential.
	if _, err := repo.Get(ctx, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get on an empty id returned %v, want ErrNotFound", err)
	}

	p := &domain.Pairing{ID: "pair-1", Name: "运营台", Description: "the cluster console",
		SecretHash: "abc123", Roles: []string{"ops"}}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	// Refused without the two things that make it a credential at all.
	if err := repo.Create(ctx, &domain.Pairing{Name: "no id"}); err == nil {
		t.Error("a pairing with no id was accepted; nothing could ever present it")
	}
	if err := repo.Create(ctx, &domain.Pairing{ID: "x", Name: "no secret"}); err == nil {
		t.Error("a pairing with no secret hash was accepted; it would verify against nothing")
	}

	got, err := repo.Get(ctx, "pair-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "运营台" || got.SecretHash != "abc123" || got.Description != "the cluster console" {
		t.Errorf("round trip lost something: %+v", got)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "ops" {
		t.Errorf("roles came back as %v, want [ops]", got.Roles)
	}
	// A fresh pairing has never been seen. Zero must NOT read as the epoch — a
	// console showing "last used 1970" for something issued a minute ago is a
	// console nobody believes.
	if !got.LastSeenAt.IsZero() {
		t.Errorf("a new pairing reports lastSeen=%v, want the zero time", got.LastSeenAt)
	}

	// Roles are replaced wholesale, and an empty set is a real state: a pairing
	// that authenticates and can do nothing.
	if err := repo.SetRoles(ctx, "pair-1", []string{"ops", "readonly"}); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.Get(ctx, "pair-1"); len(got.Roles) != 2 {
		t.Errorf("SetRoles left %v", got.Roles)
	}
	if err := repo.SetRoles(ctx, "pair-1", nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.Get(ctx, "pair-1"); len(got.Roles) != 0 {
		t.Errorf("clearing the roles left %v", got.Roles)
	}
	if err := repo.SetRoles(ctx, "not-a-pairing", []string{"ops"}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetRoles on an unknown pairing returned %v, want ErrNotFound", err)
	}

	// ── the throttle ────────────────────────────────────────────────────────
	now := time.Now().UTC()
	wrote, err := repo.TouchLastSeen(ctx, "pair-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("the first touch wrote nothing; last-seen would never be set")
	}
	if got, _ = repo.Get(ctx, "pair-1"); got.LastSeenAt.IsZero() {
		t.Error("last-seen is still unset after a touch that reported a write")
	}
	// A second touch a moment later must NOT write. This is the assertion the
	// whole design rests on — without the condition inside the statement, every
	// request from every attached console becomes a write.
	if wrote, err = repo.TouchLastSeen(ctx, "pair-1", now.Add(time.Second)); err != nil || wrote {
		t.Errorf("a touch one second after the last wrote again (wrote=%v err=%v); the throttle is not "+
			"in the statement, so every request from an attached console is a write", wrote, err)
	}
	// …and one past the granularity must.
	if wrote, err = repo.TouchLastSeen(ctx, "pair-1", now.Add(domain.PairingLastSeenGranularity+time.Minute)); err != nil || !wrote {
		t.Errorf("a touch past the granularity did not write (wrote=%v err=%v); last-seen would freeze", wrote, err)
	}
	// Touching something that does not exist is not an error: it runs on the
	// authorisation path, and a revoked-mid-request pairing must not turn into a
	// logged failure.
	if _, err := repo.TouchLastSeen(ctx, "gone", now); err != nil {
		t.Errorf("touching an absent pairing errored: %v", err)
	}

	// ── root-equivalence ────────────────────────────────────────────────────
	//
	// A stored flag, and it must round-trip as one: a back end that dropped it
	// would silently downgrade a cluster console to whatever its groups happen
	// to grant, and the symptom would be a console that mostly works.
	if got, _ = repo.Get(ctx, "pair-1"); got.Full {
		t.Error("a new pairing is root-equivalent; it must start with nothing")
	}
	if err := repo.SetFull(ctx, "pair-1", true); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.Get(ctx, "pair-1"); !got.Full {
		t.Error("SetFull(true) did not stick")
	}
	if list, _ := repo.List(ctx); len(list) != 1 || !list[0].Full {
		t.Error("the flag survives Get but not List — the console would show it wrong")
	}
	if err := repo.SetFull(ctx, "pair-1", false); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.Get(ctx, "pair-1"); got.Full {
		t.Error("SetFull(false) did not revoke it")
	}
	if err := repo.SetFull(ctx, "not-a-pairing", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetFull on an unknown pairing returned %v, want ErrNotFound", err)
	}

	// ── revocation ──────────────────────────────────────────────────────────
	if err := repo.Delete(ctx, "pair-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "pair-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("a revoked pairing still resolves: %v", err)
	}
	// Revoking twice is not an error — two operators revoking the same key is
	// exactly the situation revocation exists for.
	if err := repo.Delete(ctx, "pair-1"); err != nil {
		t.Errorf("revoking an already-revoked pairing errored: %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("List returned %d after everything was revoked", len(list))
	}
}

// ── frontend families and builds ────────────────────────────────────────────

// Family round trip, and the one property a back end could silently drop:
// SeenBuild must NOT move family_id.
//
// ⚠️ That is the assertion worth the shared test. An operator moving a build
// into an existing family is how a new platform inherits its themes; a back end
// whose "seen" write included family_id would undo that on the build's next
// startup — silently, with the themes following it. Every functional check would
// still pass.
func frontendRoundTrip(t *testing.T, h Harness) {
	s := h.Store()
	repo := s.Frontends()
	ctx := bg()

	if _, err := repo.GetFamily(ctx, "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetFamily on an unknown id returned %v, want ErrNotFound", err)
	}
	if _, err := repo.GetFamily(ctx, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetFamily on an empty id returned %v, want ErrNotFound", err)
	}

	fam := domain.FrontendFamily{
		ID: "liuli", DisplayName: "琉璃",
		Tokens: []domain.TokenSpec{
			{Name: "--primary", Kind: "color", Description: "主色"},
			{Name: "--rail-width", Kind: "length"},
		},
		Rules: "玻璃质感…",
	}
	if err := repo.UpsertFamily(ctx, fam); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetFamily(ctx, "liuli")
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "琉璃" || got.Rules != "玻璃质感…" {
		t.Errorf("round trip lost something: %+v", got)
	}
	if len(got.Tokens) != 2 {
		t.Fatalf("tokens came back as %+v", got.Tokens)
	}
	// The kind and the description survive per token — they are what the
	// validator and the model each read, and a back end that kept only names
	// would leave both with nothing.
	if tok, ok := got.TokenByName("--primary"); !ok || tok.Kind != "color" || tok.Description != "主色" {
		t.Errorf("a token lost its kind or description: %+v", tok)
	}
	// rules_accepted and pinned are flags a back end could drop, and both mean
	// the opposite of safe when missing.
	if got.RulesAccepted || got.Pinned {
		t.Errorf("a fresh family reports accepted=%v pinned=%v; both must start false",
			got.RulesAccepted, got.Pinned)
	}
	fam.RulesAccepted, fam.Pinned = true, true
	if err := repo.UpsertFamily(ctx, fam); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.GetFamily(ctx, "liuli"); !got.RulesAccepted || !got.Pinned {
		t.Errorf("the flags did not stick: accepted=%v pinned=%v", got.RulesAccepted, got.Pinned)
	}

	// ⚠️ The backfill request is the only field here that costs money when it
	// is read wrong: a value that failed to round-trip reads as "nobody asked",
	// and the leader silently never spends — a feature that looks implemented
	// and does nothing. Millisecond precision because it is stored as an epoch.
	asked := time.Now().UTC().Truncate(time.Millisecond)
	fam.BackfillRequestedAt = &asked
	if err := repo.UpsertFamily(ctx, fam); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetFamily(ctx, "liuli")
	if got.BackfillRequestedAt == nil || !got.BackfillRequestedAt.Equal(asked) {
		t.Errorf("the backfill request round-tripped as %v, want %v", got.BackfillRequestedAt, asked)
	}
	// …and clearing it really clears it, or a finished backfill sweeps forever.
	//
	// ⚠️ nil, not a zero time: the field is a pointer because `omitempty` does
	// not omit a zero struct on this Go version, and a family that always
	// reported a timestamp made every console row read as "backfill running".
	fam.BackfillRequestedAt = nil
	if err := repo.UpsertFamily(ctx, fam); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.GetFamily(ctx, "liuli"); got.BackfillRequestedAt != nil {
		t.Errorf("clearing the backfill request left %v", *got.BackfillRequestedAt)
	}

	// ── builds ──────────────────────────────────────────────────────────────
	now := time.Now().UTC()
	b := domain.FrontendBuild{BuildHash: "web-1", FamilyID: "liuli", DisplayName: "琉璃 web", Version: "4.2.0", MinAPI: 1}
	if err := repo.SeenBuild(ctx, b, now); err != nil {
		t.Fatal(err)
	}
	builds, err := repo.ListBuilds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 1 || builds[0].FamilyID != "liuli" || builds[0].Version != "4.2.0" {
		t.Fatalf("build round trip: %+v", builds)
	}
	if builds[0].FirstSeenAt.IsZero() || builds[0].LastSeenAt.IsZero() {
		t.Error("a build was recorded with no timestamps")
	}
	// Refused without the two fields that make it a sighting at all.
	if err := repo.SeenBuild(ctx, domain.FrontendBuild{FamilyID: "liuli"}, now); err == nil {
		t.Error("a build with no hash was accepted")
	}
	if err := repo.SeenBuild(ctx, domain.FrontendBuild{BuildHash: "x"}, now); err == nil {
		t.Error("a build with no family was accepted")
	}

	// ⚠️ The operator moves it, and a handshake must not move it back.
	if err := repo.SetBuildFamily(ctx, "web-1", "ting"); err != nil {
		t.Fatal(err)
	}
	// The build starts again, still declaring the family it was built with, and
	// with a stale-enough last-seen that the update path really runs.
	if err := repo.SeenBuild(ctx, b, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	builds, _ = repo.ListBuilds(ctx)
	if len(builds) != 1 {
		t.Fatalf("a repeat sighting created a second row: %+v", builds)
	}
	if builds[0].FamilyID != "ting" {
		t.Errorf("a handshake moved the build back to %q; the operator's assignment must win",
			builds[0].FamilyID)
	}
	if err := repo.SetBuildFamily(ctx, "not-a-build", "ting"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("SetBuildFamily on an unknown build returned %v, want ErrNotFound", err)
	}

	// The last-seen write is throttled, like a pairing's: a frontend that
	// handshakes on every page load must not turn that into a write every time.
	before, _ := repo.ListBuilds(ctx)
	if err := repo.SeenBuild(ctx, b, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.ListBuilds(ctx)
	if !after[0].LastSeenAt.Equal(before[0].LastSeenAt) {
		t.Error("a sighting inside the staleness window still wrote; every page load is a write")
	}

	// ── GetBuild: one indexed read, because it is on the theme path ─────────
	//
	// ⚠️ Every theme read and write resolves the caller's build from a header.
	// Doing that by scanning ListBuilds would make each of those requests cost
	// the whole table, so the interface has a point read and all four backends
	// must implement it — including the ErrNotFound, which is what makes an
	// unknown build fall back to the built-in token space instead of failing.
	one, err := repo.GetBuild(ctx, "web-1")
	if err != nil {
		t.Fatal(err)
	}
	if one.FamilyID != "ting" || one.Version != "4.2.0" {
		t.Errorf("GetBuild disagrees with ListBuilds: %+v", one)
	}
	if _, err := repo.GetBuild(ctx, "never-built"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetBuild on an unknown build returned %v, want ErrNotFound", err)
	}

	// ── DeleteFamily ────────────────────────────────────────────────────────
	//
	// ⚠️ It does NOT cascade to the builds. The admin handler refuses while any
	// build still points at a family, so a delete that got this far means none
	// do — and if the store deleted builds anyway, a family deleted by mistake
	// would take the record of what was connecting to it down as well. The
	// orphan list in the console exists precisely because these rows outlive
	// their family.
	if err := repo.DeleteFamily(ctx, "liuli"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetFamily(ctx, "liuli"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetFamily after DeleteFamily returned %v, want ErrNotFound", err)
	}
	if left, _ := repo.ListBuilds(ctx); len(left) != 1 {
		t.Errorf("deleting a family took %d builds with it", 1-len(left))
	}
	// Deleting one that is not there is not an error: two operators clicking the
	// same button must not produce a red screen for the second.
	if err := repo.DeleteFamily(ctx, "never-a-family"); err != nil {
		t.Errorf("deleting an absent family reported %v", err)
	}
}

// A theme belongs to a token space, not to a session.
//
// ⚠️ The failure this pins is silent on three backends and different on the
// fourth: Mongo has no "NOT NULL DEFAULT backfills existing rows" rule, so a
// theme written before family_id existed simply has no field, and a query for
// `family_id: "default"` does not match it. Every theme anybody ever made would
// vanish from the list — not error, not warn, gone — on exactly one backend.
// mongostore.Migrate backfills explicitly for that reason; this case is what
// says so out loud.
func themeFamilyScope(t *testing.T, h Harness) {
	ctx := bg()
	s := h.Store()
	repo := s.Themes()

	mk := func(sid, fam, name string) *domain.CustomTheme {
		t.Helper()
		got, err := repo.Create(ctx, &domain.CustomTheme{
			SessionID: sid, FamilyID: fam, Name: name,
			Variables: map[string]string{"--primary": "#f472b6"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	liuli := mk("sid1", "liuli", "琉璃的")
	ting := mk("sid1", "ting", "汀的")
	// ⚠️ No family at all. It must land in the fallback rather than in a limbo
	// no List can reach — "the theme I just made is not in the list" is the
	// most confusing symptom this design can produce, so it is the one pinned.
	none := mk("sid1", "", "没说是哪个端的")
	if none.FamilyID != domain.FallbackFamilyID {
		t.Errorf("a theme written with no family got %q, want the fallback", none.FamilyID)
	}
	mk("sid2", "liuli", "别人的")

	for _, c := range []struct {
		fam  string
		want []string
	}{
		{"liuli", []string{liuli.ID}},
		{"ting", []string{ting.ID}},
		{domain.FallbackFamilyID, []string{none.ID}},
		{"", []string{none.ID}}, // empty means the fallback on read too
		{"never-existed", nil},
	} {
		got, err := repo.List(ctx, "sid1", c.fam)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, th := range got {
			ids = append(ids, th.ID)
		}
		if len(ids) != len(c.want) {
			t.Errorf("List(%q) returned %d themes, want %d", c.fam, len(ids), len(c.want))
			continue
		}
		for i := range ids {
			if ids[i] != c.want[i] {
				t.Errorf("List(%q)[%d] = %q, want %q", c.fam, i, ids[i], c.want[i])
			}
		}
	}

	// Get is NOT scoped to a family — an id is an id, and the caller already had
	// a reference. Scoping it would turn "you sent the wrong header" into "your
	// theme disappeared".
	if got, err := repo.Get(ctx, "sid1", liuli.ID); err != nil || got.FamilyID != "liuli" {
		t.Errorf("Get across families: %+v err=%v", got, err)
	}

	// The merge path takes every family's themes, or signing in on the phone
	// drops what the desktop made.
	all, err := repo.ListAcrossFamilies(ctx, "sid1")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("ListAcrossFamilies returned %d, want all 3", len(all))
	}
	for _, th := range all {
		if th.FamilyID == "" {
			t.Errorf("theme %q came back with no family; the column is not being read", th.Name)
		}
	}

	// The switch log carries it too — "switched to 深夜紫 at 23:40" means
	// something different on the phone than on the desktop.
	if err := s.ThemeLog().Add(ctx, "sid1", liuli.ID, "liuli"); err != nil {
		t.Fatal(err)
	}
	if err := s.ThemeLog().Add(ctx, "sid1", none.ID, ""); err != nil {
		t.Fatal(err)
	}

	// ── ScanFamily: deployment-wide, keyset-paged ───────────────────────────
	//
	// ⚠️ The backfill's only reader, and it walks EVERY SESSION — a family's
	// themes belong to thousands of people. A scan that stayed session-scoped
	// would fill one person's themes and report the family done.
	//
	// Keyset on id rather than OFFSET because the sweep WRITES to the rows it
	// is walking: under OFFSET, a row updated mid-scan can shift and be skipped
	// or repeated, and a skipped row is a theme that stays broken while the
	// console says the family is complete.
	seen := map[string]bool{}
	after, pages := "", 0
	for {
		page, err := repo.ScanFamily(ctx, "liuli", after, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		pages++
		if pages > 10 {
			t.Fatal("ScanFamily never returned an empty page; the cursor is not advancing")
		}
		for _, th := range page {
			if seen[th.ID] {
				t.Errorf("ScanFamily returned %q twice", th.ID)
			}
			seen[th.ID] = true
			if th.FamilyID != "liuli" {
				t.Errorf("ScanFamily(\"liuli\") returned a theme from family %q", th.FamilyID)
			}
			after = th.ID
		}
	}
	// sid1's 琉璃 theme and sid2's, across two sessions.
	if len(seen) != 2 {
		t.Errorf("ScanFamily saw %d themes across sessions, want 2", len(seen))
	}
	// A page size of 1 must actually be honoured, or the cap that keeps a
	// two-thousand-theme family from being read into memory at once is fiction.
	if page, _ := repo.ScanFamily(ctx, "liuli", "", 1); len(page) != 1 {
		t.Errorf("ScanFamily with limit 1 returned %d rows", len(page))
	}
	if page, _ := repo.ScanFamily(ctx, "never-a-family", "", 10); len(page) != 0 {
		t.Errorf("ScanFamily on an unknown family returned %d rows", len(page))
	}
}

// The third tier: a validation rule stored as data, and the approval that gates
// it.
//
// ⚠️ The field this case exists for is Approved. Only approved rows are merged
// into the running registry, so a backend that lost the flag on a round trip
// would either install a pattern nobody agreed to, or refuse to install one
// somebody did — and neither reports anything. Everything else here is ordinary
// round-tripping.
func themeKindRoundTrip(t *testing.T, h Harness) {
	ctx := bg()
	s := h.Store()
	repo := s.ThemeKinds()

	if got, err := repo.List(ctx); err != nil || len(got) != 0 {
		t.Fatalf("a fresh store has %d kinds (err=%v); want none", len(got), err)
	}
	if n, err := repo.CountPending(ctx); err != nil || n != 0 {
		t.Fatalf("CountPending on a fresh store: %d (err=%v)", n, err)
	}

	// A frontend proposes one. Unapproved, and attributed.
	proposed := domain.ThemeKind{
		Name: "spring", Pattern: `[0-9.]+ [0-9.]+`,
		Description: "两个数：刚度 和 阻尼", ProposedBy: "liuli",
	}
	if err := repo.Upsert(ctx, proposed); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "spring")
	if err != nil {
		t.Fatal(err)
	}
	if got.Pattern != proposed.Pattern || got.Description != proposed.Description {
		t.Errorf("round trip: %+v", got)
	}
	if got.Approved {
		t.Error("a proposed kind came back APPROVED; it would be installed without anybody agreeing")
	}
	if got.ProposedBy != "liuli" {
		t.Errorf("proposedBy round-tripped as %q — six months later this column is the only answer to \"why does this deployment have a spring kind\"", got.ProposedBy)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("a kind was stored with no timestamps")
	}

	// A second, so ordering and counting have something to be wrong about.
	if err := repo.Upsert(ctx, domain.ThemeKind{Name: "grain", Pattern: `[0-9]+`}); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountPending(ctx); n != 2 {
		t.Errorf("CountPending = %d, want 2 — the ceiling on unauthenticated proposals reads this", n)
	}

	// The operator approves one.
	got.Approved = true
	if err := repo.Upsert(ctx, *got); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.Get(ctx, "spring")
	if !after.Approved {
		t.Fatal("approval did not stick; the kind can never be installed")
	}
	if after.ProposedBy != "liuli" {
		t.Errorf("approving it lost the attribution: %q", after.ProposedBy)
	}
	if n, _ := repo.CountPending(ctx); n != 1 {
		t.Errorf("CountPending = %d after one approval, want 1", n)
	}

	// List returns both, ordered by name, so two deployments with the same rows
	// produce the same screen and the same merge order.
	all, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Name != "grain" || all[1].Name != "spring" {
		t.Fatalf("List is not name-ordered: %+v", all)
	}
	// ⚠️ List returns UNAPPROVED rows too. The console needs them — the pending
	// ones are the entire point of the screen — and the caller filters.
	approved := 0
	for _, k := range all {
		if k.Approved {
			approved++
		}
	}
	if approved != 1 {
		t.Errorf("%d of 2 rows are approved; List must not filter", approved)
	}

	// Revoking is an ordinary write of the same row, not a delete: the pattern
	// and its attribution survive so an operator can approve it again without
	// the frontend re-proposing it.
	after.Approved = false
	if err := repo.Upsert(ctx, *after); err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.Get(ctx, "spring"); again.Approved || again.Pattern != proposed.Pattern {
		t.Errorf("revoking lost the row's contents: %+v", again)
	}

	if err := repo.Delete(ctx, "spring"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "spring"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get after Delete returned %v, want ErrNotFound", err)
	}
	// Deleting one that is not there is not an error: two operators pressing the
	// same button must not give the second a red screen.
	if err := repo.Delete(ctx, "never-a-kind"); err != nil {
		t.Errorf("deleting an absent kind reported %v", err)
	}
	if _, err := repo.Get(ctx, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get(\"\") returned %v, want ErrNotFound", err)
	}
	if err := repo.Upsert(ctx, domain.ThemeKind{Pattern: `x`}); !errors.Is(err, domain.ErrMissingUpsertKey) {
		t.Errorf("Upsert with no name returned %v, want ErrMissingUpsertKey", err)
	}
}
