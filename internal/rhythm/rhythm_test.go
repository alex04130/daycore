package rhythm

import (
	"testing"
	"time"
)

var shanghai = mustLoad("Asia/Shanghai")

func mustLoad(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// at builds a signal at a local wall time on a given day.
func at(day int, hour, min int) Signal {
	return Signal{At: time.Date(2026, 7, day, hour, min, 0, 0, shanghai), Kind: KindUI}
}

// The cold profile plus the default config must reproduce the three fallback
// times the design specifies (EXPERIENCE_CORE §5): 04:00 / 07:30 / 21:00. This
// is the test that keeps the derivation honest — the offsets exist to produce
// these numbers, so if someone retunes one, this fails.
func TestColdStartMatchesDesignFallbacks(t *testing.T) {
	j := Schedule(Cold(), DefaultConfig())
	if j.PlanAt != "04:00" || j.BriefAt != "07:30" || j.ReviewAt != "21:00" {
		t.Errorf("cold jobs = %+v, want 04:00 / 07:30 / 21:00", j)
	}
}

func TestScheduleWrapsMidnight(t *testing.T) {
	// A night owl: bed at 02:30, up at 10:00.
	p := Profile{Wake: "10:00", Sleep: "02:30", Source: SourceLearned}
	j := Schedule(p, DefaultConfig())
	if j.PlanAt != "06:30" {
		t.Errorf("PlanAt = %s, want 06:30 (wake − 3h30m)", j.PlanAt)
	}
	if j.ReviewAt != "01:00" {
		t.Errorf("ReviewAt = %s, want 01:00 (sleep − 90m, wrapping back past midnight)", j.ReviewAt)
	}
}

// Someone sleeping under four hours would otherwise have auto-plan scheduled
// before they went to bed — it would rewrite the day while they were still
// looking at it.
func TestScheduleGuardsAgainstShortNights(t *testing.T) {
	p := Profile{Wake: "05:00", Sleep: "03:00", Source: SourceLearned} // 2h night
	j := Schedule(p, DefaultConfig())
	if j.PlanAt != "04:00" {
		t.Errorf("PlanAt = %s, want the midpoint 04:00 rather than a time before bed", j.PlanAt)
	}
}

func TestScheduleFallsBackOnGarbage(t *testing.T) {
	j := Schedule(Profile{Wake: "nope", Sleep: "??"}, DefaultConfig())
	if j.BriefAt != "07:30" {
		t.Errorf("an unparseable profile must not take the scheduler down, got %+v", j)
	}
}

// A rhythm day is cut at 04:00, not midnight: someone still working at 02:00 is
// having a long Tuesday. Cutting at midnight would record two short days and
// learn nothing from either.
func TestRhythmDayCutsAtFour(t *testing.T) {
	cases := []struct {
		hour, min int
		day       int
		want      string
	}{
		{9, 0, 14, "2026-07-14"},
		{23, 59, 14, "2026-07-14"},
		{0, 30, 15, "2026-07-14"}, // still Tuesday
		{3, 59, 15, "2026-07-14"}, // still Tuesday
		{4, 0, 15, "2026-07-15"},  // Wednesday begins
		{12, 0, 15, "2026-07-15"},
	}
	for _, c := range cases {
		got := DayKey(time.Date(2026, 7, c.day, c.hour, c.min, 0, 0, shanghai), 4)
		if got != c.want {
			t.Errorf("%02d-%02d %02d:%02d → %s, want %s", 7, c.day, c.hour, c.min, got, c.want)
		}
	}
}

func TestLearnMedianOfObservedDays(t *testing.T) {
	cfg := DefaultConfig()
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, shanghai)
	var sigs []Signal
	// Six days, up around 08:00 and down around 23:00, with one outlier day
	// where they were up at 05:00 and awake to 03:00.
	for d := 14; d <= 18; d++ {
		sigs = append(sigs, at(d, 8, 0), at(d, 23, 0))
	}
	sigs = append(sigs, at(19, 5, 0), Signal{At: time.Date(2026, 7, 20, 3, 0, 0, 0, shanghai), Kind: KindUI})

	p := Learn(Profile{}, sigs, now, shanghai, cfg)
	if p.Source != SourceLearned {
		t.Fatalf("source = %s with %d days", p.Source, p.Days)
	}
	if p.Days != 6 {
		t.Errorf("days = %d, want 6", p.Days)
	}
	// The median must shrug off the all-nighter.
	if p.Wake != "08:00" || p.Sleep != "23:00" {
		t.Errorf("learned %s/%s, want 08:00/23:00 — one outlier day must not move the median", p.Wake, p.Sleep)
	}
}

// The median has to be taken in rhythm-day space. In wall-clock minutes a
// bedtime of 02:00 sorts before one of 22:00, and the median of a night owl's
// week comes out at lunchtime.
func TestLearnHandlesPastMidnightBedtimes(t *testing.T) {
	cfg := DefaultConfig()
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, shanghai)
	var sigs []Signal
	for d := 15; d <= 19; d++ {
		sigs = append(sigs,
			at(d, 11, 0),
			Signal{At: time.Date(2026, 7, d+1, 2, 0, 0, 0, shanghai), Kind: KindUI},
		)
	}
	p := Learn(Profile{}, sigs, now, shanghai, cfg)
	if p.Wake != "11:00" || p.Sleep != "02:00" {
		t.Errorf("learned %s/%s, want 11:00/02:00", p.Wake, p.Sleep)
	}
}

func TestLearnNeedsEvidence(t *testing.T) {
	cfg := DefaultConfig()
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, shanghai)
	sigs := []Signal{at(18, 6, 0), at(18, 22, 0), at(19, 6, 0), at(19, 22, 0)}

	p := Learn(Profile{}, sigs, now, shanghai, cfg)
	if p.Source != SourceDefault {
		t.Errorf("two days is an anecdote; source = %s", p.Source)
	}
	if p.Wake != Cold().Wake {
		t.Errorf("times should still be the fallbacks, got %s", p.Wake)
	}
	// ...but the progress is still reported, so the footprint page can show it.
	if p.Days != 2 {
		t.Errorf("days = %d, want the observed 2", p.Days)
	}
}

// Signals that can arrive while the user is asleep must never teach the Daemon
// anything — a QQ message at 03:00 is not evidence of being a night owl.
func TestOnlyRealInteractionCounts(t *testing.T) {
	if SignalKind("channel").Awake() || SignalKind("extension").Awake() || SignalKind("").Awake() {
		t.Error("unknown kinds must not count as awake")
	}
	for _, k := range []SignalKind{KindIntent, KindUI, KindHeartbeat} {
		if !k.Awake() {
			t.Errorf("%s should count", k)
		}
	}
	cfg := DefaultConfig()
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, shanghai)
	var sigs []Signal
	for d := 14; d <= 19; d++ {
		sigs = append(sigs, at(d, 9, 0), at(d, 21, 0),
			Signal{At: time.Date(2026, 7, d, 3, 0, 0, 0, shanghai), Kind: SignalKind("channel")})
	}
	p := Learn(Profile{}, sigs, now, shanghai, cfg)
	if p.Wake != "09:00" {
		t.Errorf("wake = %s — a 03:00 channel message must not count as waking up", p.Wake)
	}
}

// A single check-in says nothing about when the day started or ended.
func TestSingleSignalDayIsNotEvidence(t *testing.T) {
	cfg := DefaultConfig()
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, shanghai)
	sigs := []Signal{at(15, 15, 0), at(16, 15, 0), at(17, 15, 0), at(18, 15, 0), at(19, 15, 0)}
	p := Learn(Profile{}, sigs, now, shanghai, cfg)
	if p.Source != SourceDefault {
		t.Errorf("five one-signal days are not five days of evidence; got %s with %d days", p.Source, p.Days)
	}
}

func TestPinIsNeverOverwritten(t *testing.T) {
	pinned, err := Pin("11:00", "03:00")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, shanghai)
	var sigs []Signal
	for d := 14; d <= 19; d++ {
		sigs = append(sigs, at(d, 6, 0), at(d, 20, 0))
	}
	got := Learn(pinned, sigs, now, shanghai, cfg)
	if got != pinned {
		t.Errorf("learning overwrote a pinned rhythm: %+v", got)
	}
	if _, err := Pin("25:00", "03:00"); err == nil {
		t.Error("Pin should reject an out-of-range hour")
	}
	if _, err := Pin("11:00", "nope"); err == nil {
		t.Error("Pin should reject an unparseable time")
	}
}

func TestCurrentRun(t *testing.T) {
	cfg := DefaultConfig()
	base := time.Date(2026, 7, 20, 8, 0, 0, 0, shanghai)
	mk := func(offsets ...time.Duration) []Signal {
		var out []Signal
		for _, o := range offsets {
			out = append(out, Signal{At: base.Add(o), Kind: KindUI})
		}
		return out
	}

	// Awake since 08:00, still poking at 03:00 the next morning. Signals are
	// spaced under IdleBreak so the run stays unbroken — a 3h silence would
	// legitimately read as a nap and reset it.
	now := base.Add(19 * time.Hour)
	var offsets []time.Duration
	for h := 0; h <= 18; h += 2 {
		offsets = append(offsets, time.Duration(h)*time.Hour)
	}
	sigs := mk(offsets...)
	r := CurrentRun(sigs, now, cfg)
	if !r.Since.Equal(base) {
		t.Errorf("run started at %v, want %v", r.Since, base)
	}
	if r.Continuous != 19*time.Hour {
		t.Errorf("continuous = %v, want 19h", r.Continuous)
	}
	if r.NeedsProtector(cfg) {
		t.Error("19h should not trip a 20h threshold")
	}
	if !CurrentRun(sigs, base.Add(20*time.Hour+time.Minute), cfg).NeedsProtector(cfg) {
		t.Error("past 20h the Protector should be due")
	}
}

// The counter measures to now, not to the last signal: someone too tired to
// keep tapping is exactly who this feature is for.
func TestRunMeasuresToNowNotLastSignal(t *testing.T) {
	cfg := DefaultConfig()
	base := time.Date(2026, 7, 20, 8, 0, 0, 0, shanghai)
	var sigs []Signal
	for h := 0; h <= 19; h += 2 {
		sigs = append(sigs, Signal{At: base.Add(time.Duration(h) * time.Hour), Kind: KindUI})
	}
	// Last signal at +18h, but it is now +20h30m and they have not slept: the
	// silence is under IdleBreak, so the run is still running.
	r := CurrentRun(sigs, base.Add(20*time.Hour+30*time.Minute), cfg)
	if r.Continuous != 20*time.Hour+30*time.Minute {
		t.Errorf("continuous = %v, want 20h30m measured to now", r.Continuous)
	}
	if !r.NeedsProtector(cfg) {
		t.Error("should be due")
	}
}

func TestRunBreaksOnSleep(t *testing.T) {
	cfg := DefaultConfig()
	base := time.Date(2026, 7, 19, 8, 0, 0, 0, shanghai)
	sigs := []Signal{
		{At: base, Kind: KindUI},                     // yesterday morning
		{At: base.Add(14 * time.Hour), Kind: KindUI}, // yesterday 22:00
		{At: base.Add(24 * time.Hour), Kind: KindUI}, // today 08:00, after a 10h gap
		{At: base.Add(25 * time.Hour), Kind: KindUI}, // today 09:00
	}
	now := base.Add(26 * time.Hour)
	r := CurrentRun(sigs, now, cfg)
	if !r.Since.Equal(base.Add(24 * time.Hour)) {
		t.Errorf("run should start after the night's gap, got %v", r.Since)
	}
	if r.Continuous != 2*time.Hour {
		t.Errorf("continuous = %v, want 2h", r.Continuous)
	}
}

// Nothing recent means we cannot say anything — not that they have been up
// forever. A stale last signal reads as "asleep", which is the safe default for
// a feature that would otherwise wake someone to tell them to sleep.
func TestRunIsEmptyWhenStale(t *testing.T) {
	cfg := DefaultConfig()
	base := time.Date(2026, 7, 20, 8, 0, 0, 0, shanghai)
	sigs := []Signal{{At: base, Kind: KindUI}, {At: base.Add(time.Hour), Kind: KindUI}}
	r := CurrentRun(sigs, base.Add(9*time.Hour), cfg)
	if !r.Since.IsZero() || r.NeedsProtector(cfg) {
		t.Errorf("stale signals should produce an empty run, got %+v", r)
	}
	if r2 := CurrentRun(nil, base, cfg); !r2.Since.IsZero() || r2.NeedsProtector(cfg) {
		t.Errorf("no signals at all should produce an empty run, got %+v", r2)
	}
}
