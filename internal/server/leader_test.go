package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"daycore/internal/domain"
	"daycore/internal/storage"
)

// Two Servers sharing one database, which is exactly the deployment the lease
// exists for.
func twoInstances(t *testing.T) (*Server, *Server, domain.Store) {
	t.Helper()
	store, err := storage.Open("sqlite", "file:"+t.TempDir()+"/lease.db?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := &Server{store: store, log: discardLogger()}
	b := &Server{store: store, log: discardLogger()}
	return a, b, store
}

// The property the whole mechanism is for: of N instances, one leads.
func TestOnlyOneInstanceLeads(t *testing.T) {
	a, b, _ := twoInstances(t)
	ctx := context.Background()

	if a.LeadsWorker() {
		t.Error("an instance led before it had ever renewed — the first cron minute would run unguarded")
	}
	a.renewWorkerLease(ctx)
	b.renewWorkerLease(ctx)

	if !a.LeadsWorker() {
		t.Fatal("the first instance to acquire does not lead")
	}
	if b.LeadsWorker() {
		t.Fatal("both instances lead")
	}
	if a.InstanceID() == b.InstanceID() {
		t.Fatal("two processes share an instance id; the lease predicate cannot tell them apart")
	}

	// Renewal keeps it, and does not move the fence — the fence counts
	// handovers, so a holder can tell "still mine" from "mine again".
	fenceBefore := a.lease.fence
	a.renewWorkerLease(ctx)
	if !a.LeadsWorker() || a.lease.fence != fenceBefore {
		t.Errorf("renewal changed the hold: leads=%v fence %d→%d", a.LeadsWorker(), fenceBefore, a.lease.fence)
	}
}

// Releasing on the way out is what turns a full TTL of nothing happening into
// nothing at all.
func TestReleaseHandsOverImmediately(t *testing.T) {
	a, b, _ := twoInstances(t)
	ctx := context.Background()
	a.renewWorkerLease(ctx)
	b.renewWorkerLease(ctx)
	if b.LeadsWorker() {
		t.Fatal("b led while a held the lease")
	}

	a.ReleaseWorkerLease()
	if a.LeadsWorker() {
		t.Error("the releasing instance still believes it leads")
	}
	b.renewWorkerLease(ctx)
	if !b.LeadsWorker() {
		t.Error("after a release the other instance still cannot take the lease")
	}
	if b.lease.fence <= a.lease.fence {
		t.Errorf("fence did not move on handover: a=%d b=%d", a.lease.fence, b.lease.fence)
	}
}

// A storage error must NOT drop the hold. LeaseRenewIn is a third of LeaseTTL
// precisely so one slow query does not hand the lease away; dropping it here
// turns a flaky database into leadership flip-flop, which is worse than a stale
// leader for one more interval.
func TestRenewalErrorKeepsTheHold(t *testing.T) {
	a, _, store := twoInstances(t)
	ctx := context.Background()
	a.renewWorkerLease(ctx)
	if !a.LeadsWorker() {
		t.Fatal("setup: did not acquire")
	}

	a.store = failingLeases{Store: store}
	a.renewWorkerLease(ctx)
	if !a.LeadsWorker() {
		t.Error("one failed renewal gave the lease up; a single slow query must not do that")
	}

	// A clean "somebody else has it", by contrast, must drop it at once — the
	// stale-leader case: this instance still believes it leads while the row
	// says otherwise.
	a.store = store
	a.ReleaseWorkerLease() // free the row so the takeover is not a 60s wait
	other := &Server{store: store, log: discardLogger()}
	other.renewWorkerLease(ctx)
	if !other.LeadsWorker() {
		t.Fatal("setup: the other instance did not take the freed lease")
	}
	a.lease.mu.Lock()
	a.lease.holdUntil = time.Now().Add(time.Minute) // still believes it leads
	a.lease.mu.Unlock()
	a.renewWorkerLease(ctx)
	if a.LeadsWorker() {
		t.Error("an instance that was told it lost still believes it leads")
	}
}

// A deployment with no store behaves as a leader: single-instance must not be
// switched off by a mechanism built for the multi-instance case.
func TestNoStoreStillLeads(t *testing.T) {
	if !(&Server{}).LeadsWorker() {
		t.Error("an instance with no store refuses to run its own scheduled jobs")
	}
}

// The occurrence row, not the lease, is what makes "once" true. This is the case
// where both instances believe they lead — which the lease is supposed to
// prevent and clock skew makes possible anyway.
func TestOccurrenceIsClaimedOnceEvenIfBothLead(t *testing.T) {
	a, b, store := twoInstances(t)
	ctx := context.Background()
	a.renewWorkerLease(ctx)
	// Force the split: b is told it leads without ever winning the lease.
	b.lease.holdUntil = time.Now().Add(time.Minute)

	wa := NewWorker(a, nil)
	wb := NewWorker(b, nil)
	const key = "2026-08-06"

	runA, okA := wa.claim(ctx, "s1", domain.JobMorningBrief, key)
	_, okB := wb.claim(ctx, "s1", domain.JobMorningBrief, key)
	if !okA || okB {
		t.Fatalf("both instances claimed the same occurrence: a=%v b=%v", okA, okB)
	}

	// And after it finishes, nobody re-runs it — including the winner, which is
	// what a restart inside the same cron minute looks like.
	wa.finish(ctx, runA, nil)
	if _, again := wa.claim(ctx, "s1", domain.JobMorningBrief, key); again {
		t.Error("a finished occurrence was claimed again")
	}

	runs, err := store.JobRuns().List(ctx, "s1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != domain.JobDone {
		t.Errorf("job_runs = %+v, want exactly one done row", runs)
	}
}

// A follower must not claim at all: the row is the correctness mechanism, but
// the lease is what stops N instances from each building context first.
func TestFollowerDoesNotClaim(t *testing.T) {
	a, b, _ := twoInstances(t)
	ctx := context.Background()
	a.renewWorkerLease(ctx)
	b.renewWorkerLease(ctx)

	wb := NewWorker(b, nil)
	if _, ok := wb.claim(ctx, "s1", domain.JobMorningBrief, "2026-08-06"); ok {
		t.Error("a follower claimed an occurrence")
	}
}

// A storage error during Claim must skip the occurrence, not run it unguarded:
// a proactive message that does not go out is cheaper than one that goes twice.
func TestClaimErrorSkipsRatherThanDuplicates(t *testing.T) {
	a, _, store := twoInstances(t)
	ctx := context.Background()
	a.renewWorkerLease(ctx)
	a.store = failingJobRuns{Store: store}

	w := NewWorker(a, nil)
	if _, ok := w.claim(ctx, "s1", domain.JobMorningBrief, "2026-08-06"); ok {
		t.Error("a failed Claim was treated as ownership")
	}
}

// Occurrence keys. The daily one has to be the user's local date — a UTC key
// gives someone near the date line two morning briefs on one of their days and
// none on another.
func TestOccurrenceKeys(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("no tzdata")
	}
	// 23:30 UTC on the 5th is 07:30 on the 6th in Shanghai.
	utcEvening := time.Date(2026, 8, 5, 23, 30, 0, 0, time.UTC)
	if got := dayKey(utcEvening, shanghai); got != "2026-08-06" {
		t.Errorf("dayKey = %q, want the local date 2026-08-06", got)
	}
	if dayKey(utcEvening, time.UTC) == dayKey(utcEvening, shanghai) {
		t.Error("dayKey ignores the location, so the occurrence is not the user's day")
	}

	// Slots truncate down, so every firing inside one slot agrees.
	base := time.Date(2026, 8, 6, 9, 0, 0, 0, shanghai)
	for _, d := range []time.Duration{0, 5 * time.Minute, 29*time.Minute + 59*time.Second} {
		if got, want := slotKey(base.Add(d), shanghai, 30*time.Minute), "2026-08-06T0900"; got != want {
			t.Errorf("slotKey(+%v) = %q, want %q", d, got, want)
		}
	}
	if got, want := slotKey(base.Add(30*time.Minute), shanghai, 30*time.Minute), "2026-08-06T0930"; got != want {
		t.Errorf("the next slot = %q, want %q", got, want)
	}
	// No grace window: a late firing must NOT reach forward into a slot that has
	// not happened, which would steal an occurrence and make the real firing skip.
	if slotKey(base.Add(29*time.Minute), shanghai, 30*time.Minute) != "2026-08-06T0900" {
		t.Error("a late firing was pushed into the next slot")
	}
}

// ─── failing stores ─────────────────────────────────────────────────────────

type failingLeases struct{ domain.Store }

func (f failingLeases) Leases() domain.LeaseRepository { return brokenLease{} }

type brokenLease struct{}

func (brokenLease) Acquire(context.Context, string, string, time.Duration, time.Time) (*domain.Lease, bool, error) {
	return nil, false, errors.New("database is having a moment")
}
func (brokenLease) Release(context.Context, string, string) error { return nil }
func (brokenLease) Get(context.Context, string) (*domain.Lease, error) {
	return nil, errors.New("database is having a moment")
}

type failingJobRuns struct{ domain.Store }

func (f failingJobRuns) JobRuns() domain.JobRunRepository { return brokenJobRuns{} }

type brokenJobRuns struct{}

func (brokenJobRuns) Claim(context.Context, *domain.JobRun) (bool, error) {
	return false, errors.New("database is having a moment")
}
func (brokenJobRuns) Finish(context.Context, string, domain.JobStatus, string, time.Time) error {
	return nil
}
func (brokenJobRuns) List(context.Context, string, int) ([]domain.JobRun, error) { return nil, nil }
func (brokenJobRuns) Prune(context.Context, time.Time) (int, error)              { return 0, nil }
func (brokenJobRuns) ListRetryable(context.Context, time.Time, int) ([]domain.JobRun, error) {
	return nil, errors.New("database is having a moment")
}
