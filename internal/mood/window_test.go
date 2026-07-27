package mood

import (
	"math"
	"testing"
	"time"

	"daycore/internal/domain"
)

var now = time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

func ci(daysAgo float64, kind string) domain.MoodCheckin {
	return domain.MoodCheckin{
		Mood:      kind,
		CreatedAt: now.Add(-time.Duration(daysAgo * float64(24*time.Hour))),
	}
}

func agentCi(daysAgo float64, kind string) domain.MoodCheckin {
	c := ci(daysAgo, kind)
	c.Source = domain.MoodSourceAgent
	return c
}

// Nothing at all is not a neutral mood — it is an absent one, and the prompt
// branch has to be able to tell the difference.
func TestNoCheckinsIsNotNeutral(t *testing.T) {
	w := Read(nil, now, DefaultConfig())
	if w.Known || w.Speakable() {
		t.Errorf("got %+v, want an unknown window", w)
	}
	if w.Trend != TrendUnknown {
		t.Errorf("trend = %s", w.Trend)
	}
	if w.Tone() != ToneNeutral {
		t.Errorf("tone = %s — not knowing is not a reason to be gentle at someone", w.Tone())
	}
}

// The case §12.6 is written against: one low reading three weeks ago must not
// become today's baseline. The history is still reported — how long they have
// been quiet is itself worth knowing — but it may not be spoken as a mood.
func TestThreeWeekOldLowIsKnownButNotSpeakable(t *testing.T) {
	w := Read([]domain.MoodCheckin{ci(21, "down")}, now, DefaultConfig())
	if w.Speakable() {
		t.Error("a three-week-old check-in must not be worn as today's mood")
	}
	if !w.Stale {
		t.Error("should be flagged stale")
	}
	if w.LastKind != "down" || int(w.Staleness.Hours()/24) != 21 {
		t.Errorf("the fact itself should still be reported, got %+v", w)
	}
	// Decayed to essentially nothing — seven half-lives.
	if w.Known {
		t.Errorf("weight %v should have fallen below MinWeight", w.Weight)
	}
}

// Decay is exponential and never truly reaches zero. A hard cutoff would make a
// persistent low vanish the day it aged out, which is exactly when it matters.
func TestDecayHasNoCliff(t *testing.T) {
	cfg := DefaultConfig()
	if got := decay(cfg.HalfLife, cfg.HalfLife); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("one half-life = %v, want 0.5", got)
	}
	if got := decay(30*24*time.Hour, cfg.HalfLife); got <= 0 {
		t.Errorf("30 days = %v, want small but positive", got)
	}
	// A run of low days that continues across the boundary keeps the window
	// alive — there is no day on which it disappears.
	var run []domain.MoodCheckin
	for d := 0; d < 14; d++ {
		run = append(run, ci(float64(d), "down"))
	}
	w := Read(run, now, cfg)
	if !w.Speakable() || w.Score > -1 {
		t.Errorf("a two-week run of low days should read low and speakable, got %+v", w)
	}
}

func TestTrend(t *testing.T) {
	cfg := DefaultConfig()
	cases := []struct {
		name string
		in   []domain.MoodCheckin
		want Trend
	}{
		{"getting worse", []domain.MoodCheckin{
			ci(5, "happy"), ci(4, "calm"), ci(2, "anxious"), ci(0.5, "down"),
		}, TrendWorsening},
		{"getting better", []domain.MoodCheckin{
			ci(5, "down"), ci(4, "stressed"), ci(2, "calm"), ci(0.5, "happy"),
		}, TrendImproving},
		{"steady", []domain.MoodCheckin{
			ci(5, "calm"), ci(4, "neutral"), ci(2, "calm"), ci(0.5, "neutral"),
		}, TrendFlat},
		// Nothing on one side of the comparison: unknown, not flat. "I cannot
		// tell" and "it is not moving" are different answers.
		{"only recent", []domain.MoodCheckin{ci(1, "calm"), ci(0.5, "happy")}, TrendUnknown},
	}
	for _, c := range cases {
		if got := Read(c.in, now, cfg).Trend; got != c.want {
			t.Errorf("%s: trend = %s, want %s", c.name, got, c.want)
		}
	}
}

// An agent check-in is an inference from something the user said; a user one is
// them pressing a button. Both count, not equally (§12.1).
func TestAgentCheckinsWeighLess(t *testing.T) {
	cfg := DefaultConfig()
	byUser := Read([]domain.MoodCheckin{ci(0.5, "down"), ci(0.4, "happy")}, now, cfg)
	// Same two readings, but the low one was inferred rather than pressed.
	mixed := Read([]domain.MoodCheckin{agentCi(0.5, "down"), ci(0.4, "happy")}, now, cfg)
	if !(mixed.Score > byUser.Score) {
		t.Errorf("discounting the inferred low should raise the score: %v vs %v", mixed.Score, byUser.Score)
	}
}

// A mood the registry does not know has no valence. Scoring it as neutral would
// drag every average toward the middle.
func TestUnknownMoodIdsAreSkipped(t *testing.T) {
	w := Read([]domain.MoodCheckin{ci(0.5, "no-such-mood")}, now, DefaultConfig())
	if w.Known || w.Samples != 0 {
		t.Errorf("got %+v, want the unknown id ignored entirely", w)
	}
}

func TestToneTiers(t *testing.T) {
	cfg := DefaultConfig()
	low := Read([]domain.MoodCheckin{ci(1, "down"), ci(0.5, "anxious")}, now, cfg)
	if low.Tone() != ToneGentle {
		t.Errorf("a low window = %s, want gentle", low.Tone())
	}
	high := Read([]domain.MoodCheckin{ci(1, "happy"), ci(0.5, "excited")}, now, cfg)
	if high.Tone() != ToneBright {
		t.Errorf("a good window = %s, want bright", high.Tone())
	}
	mid := Read([]domain.MoodCheckin{ci(1, "neutral"), ci(0.5, "calm")}, now, cfg)
	if mid.Tone() != ToneNeutral {
		t.Errorf("an ordinary window = %s, want neutral", mid.Tone())
	}
	// Stale beats everything: an old low is not a reason to speak softly today.
	stale := Read([]domain.MoodCheckin{ci(5, "down"), ci(4.5, "down")}, now, cfg)
	if stale.Tone() != ToneNeutral {
		t.Errorf("a stale window = %s, want neutral", stale.Tone())
	}
}

// Planning someone less to do is a decision about their time, so it needs more
// than a soft voice does: both a low reading and a direction that is not up.
func TestRestraintIsNarrowerThanTone(t *testing.T) {
	cfg := DefaultConfig()
	sinking := Read([]domain.MoodCheckin{
		ci(5, "calm"), ci(4, "neutral"), ci(2, "down"), ci(0.5, "down"),
	}, now, cfg)
	if !sinking.Restrained() {
		t.Errorf("a week going downhill should plan lighter: %+v", sinking)
	}
	recovering := Read([]domain.MoodCheckin{
		ci(5, "down"), ci(4, "down"), ci(2, "down"), ci(0.5, "calm"),
	}, now, cfg)
	if recovering.Restrained() {
		t.Error("someone on the way back up should not have their day quietly emptied")
	}
	fine := Read([]domain.MoodCheckin{ci(1, "calm"), ci(0.5, "happy")}, now, cfg)
	if fine.Restrained() {
		t.Error("a good week should not be restrained")
	}
}

// Known and Stale are independent: there can be real history that is simply too
// old to speak for today.
func TestKnownAndStaleAreSeparate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StaleAfter = 24 * time.Hour // check-ins go stale after a day
	w := Read([]domain.MoodCheckin{ci(2, "down"), ci(1.9, "anxious")}, now, cfg)
	if !w.Known {
		t.Fatalf("two-day-old check-ins still carry weight: %+v", w)
	}
	if !w.Stale || w.Speakable() {
		t.Errorf("got %+v, want known but not speakable", w)
	}
}

func TestFutureCheckinsIgnored(t *testing.T) {
	w := Read([]domain.MoodCheckin{{Mood: "happy", CreatedAt: now.Add(time.Hour)}}, now, DefaultConfig())
	if w.Samples != 0 {
		t.Errorf("a check-in from the future should be ignored, got %+v", w)
	}
}

// Input order must not change the answer — the caller reads newest-first from
// the store, and a future one might not.
func TestOrderIndependent(t *testing.T) {
	cfg := DefaultConfig()
	in := []domain.MoodCheckin{ci(5, "happy"), ci(1, "down"), ci(3, "calm"), ci(0.2, "anxious")}
	rev := make([]domain.MoodCheckin, len(in))
	for i, c := range in {
		rev[len(in)-1-i] = c
	}
	a, b := Read(in, now, cfg), Read(rev, now, cfg)
	if a.Score != b.Score || a.Trend != b.Trend || a.LastKind != b.LastKind {
		t.Errorf("order changed the result: %+v vs %+v", a, b)
	}
}
