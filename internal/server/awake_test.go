package server

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"daycore/internal/config"
	"daycore/internal/storage"
	_ "daycore/internal/storage/sqlstore"
)

func TestAwakeThrottleAdmitsOncePerWindow(t *testing.T) {
	tr := newAwakeTracker()
	base := time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC)

	if !tr.admit("s1", base) {
		t.Fatal("first signal must be admitted — otherwise a session is never scheduled either")
	}
	if tr.admit("s1", base.Add(awakeThrottle-time.Second)) {
		t.Error("admitted twice inside the window")
	}
	if !tr.admit("s1", base.Add(awakeThrottle)) {
		t.Error("did not admit after the window elapsed")
	}
	// Sessions are independent: one busy user must not throttle everyone else.
	if !tr.admit("s2", base.Add(time.Second)) {
		t.Error("one session's throttle blocked another's")
	}
	// A clock that steps backwards must not lock a session out. now.Sub(prev) goes
	// negative, which is < awakeThrottle, so it correctly refuses — but the next
	// forward signal must still work rather than the entry being stuck in the
	// future.
	if tr.admit("s1", base.Add(-time.Hour)) {
		t.Error("a backwards clock produced an extra write")
	}
	if !tr.admit("s1", base.Add(2*awakeThrottle)) {
		t.Error("a backwards clock left the session locked out")
	}
}

func TestAwakeTrackerCapIsEnforced(t *testing.T) {
	tr := newAwakeTracker()
	now := time.Now()
	for i := 0; i < awakeTrackerCap+10; i++ {
		tr.admit(string(rune('a'+i%26))+strings.Repeat("x", i%7)+itoaSurface(i), now)
	}
	tr.mu.Lock()
	n := len(tr.last)
	tr.mu.Unlock()
	if n > awakeTrackerCap {
		t.Errorf("map grew past the cap: %d entries — sessions would accumulate for the life of the process", n)
	}
}

// The end-to-end wiring: a session-bearing request records a rhythm day row, and
// a machine push does not.
func TestRequireSessionRecordsAwakeSignal(t *testing.T) {
	store, err := storage.Open("sqlite", "file:"+t.TempDir()+"/awake.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	s := New(Deps{
		Config: &config.Config{WorkerDefaultTZ: "UTC"},
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(new(strings.Builder), nil)),
	})
	const sid = "awake-sid"
	if _, err := store.Sessions().GetOrCreate(ctx, sid); err != nil {
		t.Fatal(err)
	}

	if days, err := store.Rhythm().Days(ctx, sid, 10); err != nil {
		t.Fatal(err)
	} else if len(days) != 0 {
		t.Fatalf("expected no rhythm days before any signal, got %d", len(days))
	}

	// markAwake dispatches the write on a tracked goroutine, so wait for it the
	// same way shutdown does.
	s.markAwake(sid)
	if err := s.WaitBackground(ctx); err != nil {
		t.Fatal(err)
	}

	days, err := store.Rhythm().Days(ctx, sid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 {
		t.Fatalf("expected exactly one rhythm day after one signal, got %d", len(days))
	}
	if days[0].FirstMin < 0 || days[0].FirstMin > 24*60 {
		t.Errorf("minute out of range: %d", days[0].FirstMin)
	}

	// A second signal inside the throttle window must not widen anything, because
	// it must not even be written.
	before := days[0]
	s.markAwake(sid)
	if err := s.WaitBackground(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := store.Rhythm().Days(ctx, sid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].LastMin != before.LastMin {
		t.Errorf("throttled signal still reached the store: %+v → %+v", before, after)
	}
}

// scheduleOnUse must fire for the first request of a session and be harmless
// when nobody installed it (every test, and any future degraded boot).
func TestScheduleOnUseFiresOnFirstUse(t *testing.T) {
	s := New(Deps{
		Config: &config.Config{},
		Logger: slog.New(slog.NewTextHandler(new(strings.Builder), nil)),
	})
	// No store: markAwake must return before touching it rather than panicking.
	s.markAwake("nobody")

	var got []string
	s.SetScheduleOnUse(func(sid string) { got = append(got, sid) })
	s.SetScheduleOnUse(nil) // must not clear the installed callback

	s.awake = newAwakeTracker()
	s.markAwake("nobody") // still no store, so it returns before scheduling
	if len(got) != 0 {
		t.Errorf("scheduled despite there being no store: %v", got)
	}
}
