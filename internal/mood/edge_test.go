package mood

import (
	"testing"
	"time"

	"daycore/internal/domain"
)

func mk(t time.Time, mood string, source string) domain.MoodCheckin {
	return domain.MoodCheckin{CreatedAt: t, Mood: mood, Source: source}
}

// The three thresholds are EXACT boundaries: -0.75/0.75 for tone, TrendDelta
// equality counts as a direction, StaleAfter/MinWeight compare with strict
// inequalities. Each comparison direction is a one-character decision that a
// rewrite can silently flip.
func TestToneBoundaryValues(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cases := []struct {
		name    string
		valence int
		want    Tone
	}{
		{"just at gentle line", -1, ToneGentle},
		{"just at bright line", 2, ToneBright},
		{"mid", 0, ToneNeutral},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			moodID := ""
			for _, k := range domain.MoodKinds() {
				if k.Valence == tc.valence {
					moodID = k.ID
					break
				}
			}
			if moodID == "" {
				t.Skipf("no mood kind with valence %d", tc.valence)
			}
			w := Read([]domain.MoodCheckin{mk(now.Add(-time.Hour), moodID, domain.MoodSourceUser)}, now, cfg)
			if w.Tone() != tc.want {
				t.Errorf("Tone() = %v, want %v (score %v)", w.Tone(), tc.want, w.Score)
			}
		})
	}
}

func TestTrendDeltaEquality(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	// A trend needs one sample in each span. Use a neutral mood and a bright
	// one; delta 2 > TrendDelta (0.5) → improving. Equality at exactly
	// TrendDelta is covered by shrinking the delta via valences 0 and +1?
	// That is delta 1 — still above. To hit EXACTLY 0.5 use averages… the
	// registry has integer valences, so equality is exercised with a custom
	// TrendDelta equal to the delta instead.
	good, bad := "", ""
	for _, k := range domain.MoodKinds() {
		if k.Valence == 1 {
			good = k.ID
		}
		if k.Valence == -1 {
			bad = k.ID
		}
	}
	if good == "" || bad == "" {
		t.Skip("registry lacks ±1 valences")
	}
	cfg.TrendDelta = 2.0
	w := Read([]domain.MoodCheckin{
		mk(now.Add(-90*time.Hour), bad, domain.MoodSourceUser),
		mk(now.Add(-time.Hour), good, domain.MoodSourceUser),
	}, now, cfg)
	if w.Trend != TrendImproving {
		t.Errorf("a delta exactly at TrendDelta must count as improving (>=), got %v", w.Trend)
	}
	// And just below it is flat.
	cfg.TrendDelta = 2.5
	w = Read([]domain.MoodCheckin{
		mk(now.Add(-90*time.Hour), bad, domain.MoodSourceUser),
		mk(now.Add(-time.Hour), good, domain.MoodSourceUser),
	}, now, cfg)
	if w.Trend != TrendFlat {
		t.Errorf("a delta below TrendDelta must be flat, got %v", w.Trend)
	}
}

func TestExactBoundariesStaleAndKnown(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	mid := ""
	for _, k := range domain.MoodKinds() {
		if k.Valence == 0 {
			mid = k.ID
			break
		}
	}
	if mid == "" {
		t.Skip("no neutral mood kind")
	}
	// A check-in EXACTLY StaleAfter old is not stale (strict >).
	w := Read([]domain.MoodCheckin{mk(now.Add(-cfg.StaleAfter), mid, domain.MoodSourceUser)}, now, cfg)
	if w.Stale {
		t.Error("exactly StaleAfter old must not be stale (strict >)")
	}
	// A total weight EXACTLY at MinWeight is Known (strict < on the reject).
	// One fresh check-in weighs 1.0; set MinWeight to exactly 1.0.
	cfg.MinWeight = 1.0
	w = Read([]domain.MoodCheckin{mk(now, mid, domain.MoodSourceUser)}, now, cfg)
	if !w.Known {
		t.Error("weight exactly at MinWeight must be Known (strict <)")
	}
	cfg.MinWeight = 1.01
	w = Read([]domain.MoodCheckin{mk(now, mid, domain.MoodSourceUser)}, now, cfg)
	if w.Known {
		t.Error("weight below MinWeight must not be Known")
	}
}

func TestDecayDegenerateConfig(t *testing.T) {
	// A zero-value Config must not divide by zero or NaN the whole window:
	// with halfLife <= 0 everything keeps full weight, and a non-positive
	// age decays to 1.
	if got := decay(10*time.Hour, 0); got != 1 {
		t.Errorf("halfLife 0 must yield 1, got %v", got)
	}
	if got := decay(10*time.Hour, -time.Hour); got != 1 {
		t.Errorf("negative halfLife must yield 1, got %v", got)
	}
	if got := decay(-time.Hour, 72*time.Hour); got != 1 {
		t.Errorf("non-positive age must yield 1, got %v", got)
	}
	if got := decay(0, 72*time.Hour); got != 1 {
		t.Errorf("age 0 must yield 1, got %v", got)
	}
}

func TestAgentWeightZeroStillCountsSample(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cfg := DefaultConfig()
	cfg.AgentWeight = 0
	good := ""
	for _, k := range domain.MoodKinds() {
		if k.Valence == 1 {
			good = k.ID
			break
		}
	}
	if good == "" {
		t.Skip()
	}
	w := Read([]domain.MoodCheckin{mk(now, good, domain.MoodSourceAgent)}, now, cfg)
	// The sample exists but carries zero weight: it is counted, not scored.
	if w.Samples != 1 {
		t.Errorf("Samples = %d, want 1", w.Samples)
	}
	if w.Known {
		t.Error("a zero-weight sample cannot make the window Known")
	}
}
