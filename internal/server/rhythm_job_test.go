package server

import (
	"context"
	"testing"
	"time"

	"daycore/internal/domain"
	"daycore/internal/rhythm"
)

// Seed one rhythm day. minute values are offsets from the day cut, which is what
// markAwake writes and what the learner reads.
func seedDay(t *testing.T, s *Server, sid, day string, first, last int) {
	t.Helper()
	ctx := context.Background()
	if err := s.store.Rhythm().Observe(ctx, sid, day, first); err != nil {
		t.Fatal(err)
	}
	if err := s.store.Rhythm().Observe(ctx, sid, day, last); err != nil {
		t.Fatal(err)
	}
}

func learnServer(t *testing.T) (*Server, *Worker, string) {
	t.Helper()
	s, sid := newAgentTestServer(t)
	s.cfg.WorkerDefaultTZ = "UTC"
	// Lead, so claim() lets the job run at all.
	s.renewWorkerLease(context.Background())
	w := NewWorker(s, nil)
	s.SetWorker(w)
	return s, w, sid
}

// The whole point: rows that markAwake has been writing become a profile.
//
// Before this job, internal/rhythm had two production callers, both in awake.go
// and both only computing a day key — Learn, LearnDays, Schedule, Cold and Pin
// had none, and neither did Get/Save/Days/PruneDays on the repository.
func TestRhythmLearnTurnsDaysIntoAProfile(t *testing.T) {
	s, w, sid := learnServer(t)
	ctx := context.Background()
	cfg := rhythmConfig()
	now := time.Now().UTC()

	// Seven usable days, all "up at 08:00, last seen at 23:00" in cut-offset
	// minutes (the cut is 04:00, so 08:00 is +240 and 23:00 is +1140).
	for i := 1; i <= 7; i++ {
		day := rhythm.DayKey(now.AddDate(0, 0, -i), cfg.DayCutHour)
		seedDay(t, s, sid, day, 240, 1140)
	}

	w.runRhythmLearn(sid, "UTC")

	p, err := s.store.Rhythm().Get(ctx, sid)
	if err != nil {
		t.Fatalf("no profile was written: %v", err)
	}
	if p.Source != string(rhythm.SourceLearned) {
		t.Errorf("source = %q, want learned — %d days of evidence should be enough (MinDays=%d)", p.Source, p.Days, cfg.MinDays)
	}
	if p.Wake != "08:00" || p.Sleep != "23:00" {
		t.Errorf("learned %s–%s, want 08:00–23:00", p.Wake, p.Sleep)
	}
	if p.Days != 7 {
		t.Errorf("days = %d, want 7", p.Days)
	}

	// And the schedule now follows it rather than the hard-coded literals.
	jobs := w.jobsFor(ctx, sid)
	if jobs.BriefAt != "08:00" {
		t.Errorf("brief at %s, want the learned wake time", jobs.BriefAt)
	}
	if jobs.ReviewAt != "21:30" { // sleep 23:00 − ReviewBefore 90m
		t.Errorf("review at %s, want 90 minutes before the learned bedtime", jobs.ReviewAt)
	}
}

// rhythm.LearnDays does NOT apply the window — the window lives in DaysFrom,
// which is the signal-list path. An intermittent user's most recent 21 ROWS can
// span six months, and `Days(ctx, sid, 21)` is a row limit, not a date filter.
func TestRhythmWindowExcludesStaleAndToday(t *testing.T) {
	cfg := rhythmConfig()
	rows := []domain.RhythmDay{
		{Day: "2026-08-07", FirstMin: 240, LastMin: 1140, Signals: 4}, // today
		{Day: "2026-08-06", FirstMin: 240, LastMin: 1140, Signals: 4},
		{Day: "2026-07-01", FirstMin: 600, LastMin: 1400, Signals: 9}, // older than the window
	}
	got := rhythmWindow(rows, "2026-07-17", "2026-08-07")
	if len(got) != 1 || got[0].Key != "2026-08-06" {
		t.Fatalf("window = %+v, want only 2026-08-06", got)
	}

	// Today's row is still growing: LastMin rises until the user goes to bed, so
	// including it teaches an ever-earlier bedtime.
	if len(rhythmWindow(rows[:1], "2026-07-17", "2026-08-07")) != 0 {
		t.Error("today's row was included")
	}
	// And a row older than the window teaches today's rhythm from last month.
	if len(rhythmWindow(rows[2:], "2026-07-17", "2026-08-07")) != 0 {
		t.Error("a row from outside the window was included")
	}
	_ = cfg
}

// Thin evidence must not throw away what was already learned.
//
// LearnDays answers "what do THESE days say", and Cold() is the right answer to
// that question when there are too few. Writing it back is not: three weeks away
// would move somebody's brief back to 07:30 overnight, with no explanation and
// nothing they did.
func TestThinEvidenceKeepsWhatWasLearned(t *testing.T) {
	learned := rhythm.Profile{Wake: "09:15", Sleep: "01:30", Source: rhythm.SourceLearned, Days: 14}
	thin := rhythm.Cold()
	thin.Days = 2

	got := mergeLearned(learned, thin)
	if got.Wake != "09:15" || got.Sleep != "01:30" {
		t.Errorf("thin evidence reset the times to %s–%s", got.Wake, got.Sleep)
	}
	if got.Source != rhythm.SourceLearned {
		t.Errorf("source fell back to %q — the footprint page would claim we never learned anything", got.Source)
	}
	if got.Days != 2 {
		t.Errorf("days = %d, want the honest lower count 2", got.Days)
	}

	// A real learned result replaces the old one outright.
	fresh := rhythm.Profile{Wake: "07:00", Sleep: "22:00", Source: rhythm.SourceLearned, Days: 9}
	if got := mergeLearned(learned, fresh); got != fresh {
		t.Errorf("a fresh learned profile was not adopted: %+v", got)
	}
	// And somebody who has never learned anything gets the cold defaults.
	if got := mergeLearned(rhythm.Profile{}, thin); got != thin {
		t.Errorf("a first run did not get the defaults: %+v", got)
	}
}

// A pinned rhythm is a decision, not a guess: "我就是夜猫子，别管我".
//
// ⚠️ The guard is rhythm.LearnDays, which returns a pinned profile untouched —
// the early return in runRhythmLearn only saves the read, and deleting it leaves
// this test green. Asserted as an OUTCOME on purpose; if the pure function ever
// stops honouring the pin, this is what notices.
func TestPinnedProfileIsNeverRelearned(t *testing.T) {
	s, w, sid := learnServer(t)
	ctx := context.Background()
	cfg := rhythmConfig()
	now := time.Now().UTC()

	if err := s.store.Rhythm().Save(ctx, &domain.RhythmProfile{
		SessionID: sid, Wake: "11:00", Sleep: "03:00", Source: string(rhythm.SourcePinned),
	}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10; i++ {
		seedDay(t, s, sid, rhythm.DayKey(now.AddDate(0, 0, -i), cfg.DayCutHour), 240, 1140)
	}

	w.runRhythmLearn(sid, "UTC")

	p, _ := s.store.Rhythm().Get(ctx, sid)
	if p.Wake != "11:00" || p.Sleep != "03:00" || p.Source != string(rhythm.SourcePinned) {
		t.Errorf("learning overwrote a pinned profile: %+v", p)
	}
}

// One occurrence per session per rhythm day, whoever runs it.
func TestRhythmLearnClaimsItsOccurrence(t *testing.T) {
	s, w, sid := learnServer(t)
	ctx := context.Background()
	cfg := rhythmConfig()
	now := time.Now().UTC()
	for i := 1; i <= 7; i++ {
		seedDay(t, s, sid, rhythm.DayKey(now.AddDate(0, 0, -i), cfg.DayCutHour), 240, 1140)
	}

	w.runRhythmLearn(sid, "UTC")
	w.runRhythmLearn(sid, "UTC") // a restart inside the same day

	runs, err := s.store.JobRuns().List(ctx, sid, 0)
	if err != nil {
		t.Fatal(err)
	}
	var learns int
	for _, r := range runs {
		if r.Job == domain.JobRhythmLearn {
			learns++
			if r.Status != domain.JobDone {
				t.Errorf("occurrence ended %q, want done", r.Status)
			}
		}
	}
	if learns != 1 {
		t.Errorf("%d rhythm_learn occurrences, want 1", learns)
	}
}

// A follower must not learn at all. The occurrence row makes "once" true even
// when two instances both think they lead, but the lease is what stops N of them
// each reading three weeks of rows and writing the same answer.
//
// This is the assertion that can actually fail when the claim guard is removed:
// counting job_runs cannot, because a second claim on the same occurrence
// returns false and writes no row either way.
func TestFollowerDoesNotLearn(t *testing.T) {
	s, _, sid := learnServer(t)
	ctx := context.Background()
	cfg := rhythmConfig()
	now := time.Now().UTC()
	for i := 1; i <= 7; i++ {
		seedDay(t, s, sid, rhythm.DayKey(now.AddDate(0, 0, -i), cfg.DayCutHour), 240, 1140)
	}

	// A second instance that never won the lease.
	follower := &Server{store: s.store, cfg: s.cfg, log: discardLogger()}
	follower.renewWorkerLease(ctx) // loses: s already holds it
	if follower.LeadsWorker() {
		t.Fatal("setup: the follower took the lease")
	}
	NewWorker(follower, nil).runRhythmLearn(sid, "UTC")

	if p, err := s.store.Rhythm().Get(ctx, sid); err == nil && p != nil && p.Source != "" {
		t.Errorf("a follower wrote a profile: %+v", p)
	}
}

// The learner and the recorder must agree on what a day is. Two independent
// DefaultConfig() calls work today and silently mis-bucket every row the moment
// one of them changes.
func TestRecorderAndLearnerShareTheConfig(t *testing.T) {
	if rhythmConfig() != rhythm.DefaultConfig() {
		t.Fatal("rhythmConfig has diverged from the package default")
	}
	// The recorder's day key and the learner's must be the same function of the
	// same instant.
	now := time.Date(2026, 8, 7, 2, 30, 0, 0, time.UTC) // before the 04:00 cut
	cfg := rhythmConfig()
	recorded, _ := rhythm.DayOf(now, time.UTC, cfg)
	if learned := rhythm.DayKey(now, cfg.DayCutHour); recorded != learned {
		t.Errorf("recorder writes %q, learner reads %q", recorded, learned)
	}
}

// An unparseable time must not drop the cron entry: a brief at the default hour
// beats no brief, which is the same trade resolveTZ makes.
func TestCronScheduleFromRhythmFallsBack(t *testing.T) {
	for _, bad := range []string{"", "nope", "25:00", "07:99", "7"} {
		if got := cronScheduleAtHM(bad, "UTC"); got != cronScheduleAt(7, 30, "UTC") {
			t.Errorf("cronScheduleAtHM(%q) = %q, want the cold-start fallback", bad, got)
		}
	}
	if got, want := cronScheduleAtHM("09:15", "UTC"), cronScheduleAt(9, 15, "UTC"); got != want {
		t.Errorf("cronScheduleAtHM(09:15) = %q, want %q", got, want)
	}
}
