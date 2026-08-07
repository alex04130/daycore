package server

import (
	"log/slog"
	"strings"
	"testing"
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
