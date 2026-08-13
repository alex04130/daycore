package server

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"daycore/internal/domain"

	"github.com/robfig/cron/v3"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(new(strings.Builder), nil))
}

// ScheduleUser had a guard that could never fire: it looked up w.jobs[sid+":"+tz]
// while the writes were w.jobs[key+":morning"] and friends. Nothing ever wrote
// the key it read, so "already scheduled" was always false.
//
// That is not a multi-instance problem — it is a bug that was costing users
// every day on a single instance. markAwake admits one session every five
// minutes (awakeThrottle) and calls ScheduleUser on each admission, so an hour
// of activity left twelve copies of every job: twelve rolling-replan runs per
// half-hour tick, each one a model call and a push. The comment at
// awake.go said "ScheduleUser is idempotent anyway".
//
// The property is invisible from outside a Worker, which is how it survived —
// hence EntryCount.
func TestScheduleUserIsIdempotent(t *testing.T) {
	w := NewWorker(&Server{log: discardLogger()}, nil)

	w.ScheduleUser("s1", "Asia/Shanghai")
	first := w.EntryCount()
	if first == 0 {
		t.Fatal("scheduling a session registered no cron entries at all")
	}

	for i := 0; i < 12; i++ { // an hour of markAwake admissions
		w.ScheduleUser("s1", "Asia/Shanghai")
	}
	if got := w.EntryCount(); got != first {
		t.Errorf("after 12 more calls: %d entries, want %d — every admission is adding another copy of every job", got, first)
	}

	// A second session is additive, not a no-op: a guard that refuses everything
	// passes the test above and switches the product off.
	w.ScheduleUser("s2", "Asia/Shanghai")
	if got := w.EntryCount(); got != first*2 {
		t.Errorf("after scheduling a second session: %d entries, want %d", got, first*2)
	}
}

// A session that changes timezone must not keep the old entries: the brief
// would fire twice, once per zone. This is the case per-session timezone (ζ)
// walks straight into.
func TestScheduleUserReplacesOnTimezoneChange(t *testing.T) {
	w := NewWorker(&Server{log: discardLogger()}, nil)

	w.ScheduleUser("s1", "Asia/Shanghai")
	one := w.EntryCount()

	w.ScheduleUser("s1", "America/Chicago")
	if got := w.EntryCount(); got != one {
		t.Errorf("after a timezone change: %d entries, want %d — the old zone's jobs are still armed", got, one)
	}

	w.UnscheduleUser("s1")
	if got := w.EntryCount(); got != 0 {
		t.Errorf("after unscheduling: %d entries, want 0", got)
	}
	// Unscheduling twice, or a session that was never scheduled, must be boring.
	w.UnscheduleUser("s1")
	w.UnscheduleUser("never-seen")
	if got := w.EntryCount(); got != 0 {
		t.Errorf("redundant unschedule left %d entries", got)
	}
}

// An unloadable timezone used to be a PARTIAL failure that looked like a
// success, which is the worst of the available outcomes: the two CRON_TZ jobs
// (morning brief, evening review) failed to register, the two plain ones
// succeeded, the session was recorded as scheduled — and nothing ever retried.
// That user silently never got a brief again.
//
// This assertion is here because the first version of it was vacuous: it
// checked "the second call still schedules something", which was true either
// way since the two un-prefixed jobs always registered. Counting the entries is
// what makes it able to fail.
func TestScheduleUserFallsBackOnAnUnknownTimezone(t *testing.T) {
	good := NewWorker(&Server{log: discardLogger()}, nil)
	good.ScheduleUser("s1", "Asia/Shanghai")
	want := good.EntryCount()

	bad := NewWorker(&Server{log: discardLogger()}, nil)
	bad.ScheduleUser("s1", "Not/AZone")
	if got := bad.EntryCount(); got != want {
		t.Errorf("an unknown timezone scheduled %d of %d jobs — the missing ones are the brief and the review, and nothing retries", got, want)
	}

	// The fallback must also be remembered, or every admission reschedules.
	bad.ScheduleUser("s1", "Not/AZone")
	if got := bad.EntryCount(); got != want {
		t.Errorf("rescheduling under the same unknown zone added entries: %d, want %d", got, want)
	}
}

// parseBlockTime accepted a date and ignored it: the day came from time.Now()
// whatever you passed. Both callers at the time happened to pass today, so the
// parameter was a lie nothing could catch — until the Protector became the first
// caller to reason about a different day and silently got yesterday's blocks
// treated as still ahead.
func TestParseBlockTimeUsesTheDateItIsGiven(t *testing.T) {
	loc := time.UTC
	got, err := parseBlockTime("2020-03-05", "09:30", loc)
	if err != nil {
		t.Fatal(err)
	}
	if y, m, d := got.Date(); y != 2020 || m != time.March || d != 5 {
		t.Errorf("parsed %v, want 2020-03-05 — the date argument is being ignored", got)
	}
	if got.Hour() != 9 || got.Minute() != 30 {
		t.Errorf("parsed %v, want 09:30", got)
	}

	// An empty or unparseable date still falls back to today, which is what the
	// existing callers rely on.
	today := time.Now().In(loc)
	for _, bad := range []string{"", "not-a-date"} {
		got, err := parseBlockTime(bad, "09:30", loc)
		if err != nil {
			t.Fatalf("parseBlockTime(%q): %v", bad, err)
		}
		if y, m, d := got.Date(); y != today.Year() || m != today.Month() || d != today.Day() {
			t.Errorf("parseBlockTime(%q) = %v, want today", bad, got)
		}
	}
}

// The fact-track ladder (STRATEGY §1.3).
//
// checkDeadlines used to re-list everything due within 48 hours every two hours
// and send the same message again — nothing recorded that an item had already
// been warned about, so a deadline three days out produced roughly two dozen
// identical messages, and the only way to stop them was DeadlineAlerts, which
// turns the whole fact track off.
func TestDeadlineRungs(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		due  time.Time
		want time.Duration
		in   bool
	}{
		{"three days out", now.Add(72 * time.Hour), 0, false},
		{"just over a day", now.Add(25 * time.Hour), 0, false},
		{"inside 24h", now.Add(20 * time.Hour), 24 * time.Hour, true},
		{"exactly 24h", now.Add(24 * time.Hour), 24 * time.Hour, true},
		{"inside 12h", now.Add(6 * time.Hour), 12 * time.Hour, true},
		{"inside 1h", now.Add(30 * time.Minute), time.Hour, true},
		{"overdue", now.Add(-time.Hour), 0, true},
	}
	for _, tc := range cases {
		got, in := domain.DeadlineRungFor(tc.due, now)
		if in != tc.in || got != tc.want {
			t.Errorf("%s: rung=%v in=%v, want %v/%v", tc.name, got, in, tc.want, tc.in)
		}
	}

	// Each rung is its own occurrence, so climbing produces three messages and
	// re-checking inside one produces none.
	seen := map[string]bool{}
	for _, left := range []time.Duration{20 * time.Hour, 18 * time.Hour, 6 * time.Hour, 5 * time.Hour, 30 * time.Minute} {
		rung, _ := domain.DeadlineRungFor(now.Add(left), now)
		seen[deadlineRunKey("a1", rung)] = true
	}
	if len(seen) != 3 {
		t.Errorf("five checks across the ladder produced %d occurrence keys, want 3: %v", len(seen), seen)
	}
	if deadlineRunKey("a1", 0) != "due:a1:overdue" {
		t.Errorf("the overdue key is %q; run keys are read by people too", deadlineRunKey("a1", 0))
	}
}

// The rhythm learner must be able to rebuild a session's entries when only the
// SPECS moved.
//
// ⚠️ This is the other side of TestScheduleUserIsIdempotent, and the two guards
// pull in opposite directions — which is exactly how the second one broke while
// the first stayed green.
//
// The idempotence guard exists because markAwake calls ScheduleUser on every
// admission. The rhythm learner calls it too, after learning a new wake time —
// and the guard sent it straight back out, because what changed was the wake
// time, not the timezone. rhythm_job.go's own comment says the entries "have to
// be rebuilt or the profile is a value nobody acts on"; that is precisely what
// happened. A learned 06:40 did nothing until the process restarted.
//
// Invisible from outside, again, and for the same reason: the only observable
// is the entry set.
func TestRescheduleUserRebuildsWhenOnlyTheSpecsMoved(t *testing.T) {
	w := NewWorker(&Server{log: discardLogger()}, nil)

	w.ScheduleUser("s1", "Asia/Shanghai")
	first := w.EntryCount()
	if first == 0 {
		t.Fatal("scheduling a session registered no cron entries at all")
	}
	ids := append([]cron.EntryID(nil), w.jobs["s1"]...)

	// Same session, same zone — what the learner does after a profile changes.
	w.RescheduleUser("s1", "Asia/Shanghai")

	if got := w.EntryCount(); got != first {
		t.Errorf("after a reschedule: %d entries, want %d — a rebuild must REPLACE, not add", got, first)
	}
	// ⚠️ Same count is not enough: ScheduleUser's early return also produces the
	// same count, and that is the bug. The entries have to be NEW ones.
	same := 0
	for i, id := range w.jobs["s1"] {
		if i < len(ids) && ids[i] == id {
			same++
		}
	}
	if same == len(ids) {
		t.Error("the cron entries were not rebuilt — every id is the one from before, " +
			"so a learned wake time would go on doing nothing")
	}

	// And the plain call must still be a no-op, or markAwake starts piling up
	// twelve copies an hour again.
	w.ScheduleUser("s1", "Asia/Shanghai")
	if got := w.EntryCount(); got != first {
		t.Errorf("plain ScheduleUser is no longer idempotent: %d entries, want %d", got, first)
	}
}
