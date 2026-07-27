// Package mood derives a reading of how someone has been feeling from their
// check-in history.
//
// One check-in is not a mood; a run of them is (EXPERIENCE_CORE §12.6).
// "Anxious today" and "more anxious every day this week" are different facts,
// and only the second should change how the system behaves. A single low
// reading from three weeks ago should change nothing at all.
//
// Everything that wants to know about mood reads a Window — the companion's
// context injection, rapport's tone tier, the Protector's wording, the morning
// brief and evening review, proposal card tone, how full auto-plan makes a day.
// One derivation, not six: six would drift, and the user would meet a system
// that is gentle in one place and brisk in another about the same week.
//
// Read-time derived like petrification and rapport: no trend table, no cron, no
// stored aggregate. A wrong parameter costs the next read.
//
// # The boundary
//
// A Window is an input to tone and pacing. It is NOT an input to content or
// refusal. A locked block's 409 does not consult mood: a refusal has to be
// immediate, certain and testable, and someone who cannot drag a block needs to
// know why now, not after the system has considered how their week has gone.
package mood

import (
	"math"
	"sort"
	"time"

	"daycore/internal/domain"
)

// Config holds the tunables. None come from the design original; all are here
// rather than inline so the console can expose them.
type Config struct {
	// HalfLife is how long it takes a check-in to count half as much. Decay is
	// exponential and never reaches zero, which is deliberate: a hard cutoff
	// ("only the last 7 days") makes a persistent low mood vanish on day 8,
	// exactly when it has become the most important thing to know.
	HalfLife time.Duration

	// AgentWeight scales check-ins the agent recorded on the user's behalf.
	// Both kinds are real, but one is an inference from something they said and
	// the other is them pressing a button, and those do not carry the same
	// weight (§12.1).
	AgentWeight float64

	// StaleAfter is how long since the last check-in before the agent must stop
	// treating a stored mood as today's baseline. Staleness is a separate
	// question from decay: decay decides how much old data counts toward a
	// score, staleness decides whether the agent may bring mood up at all.
	//
	// Three days, because a check-in describes a day, and by the third the day
	// it describes is no longer recent. Past it the agent knows what it knows
	// and knows that it is old — which is a thing to ask about, not to assume.
	StaleAfter time.Duration

	// TrendSpan is the width of each half of the comparison: the mean of the
	// last TrendSpan against the mean of the TrendSpan before it.
	TrendSpan time.Duration

	// TrendDelta is how far those two means must differ, on the −2..+2 valence
	// scale, before the difference is called a direction rather than noise.
	TrendDelta float64

	// MinWeight is the total decayed weight needed before the window claims to
	// know anything. One three-week-old check-in should not produce a reading.
	MinWeight float64
}

// DefaultConfig is the starting point.
func DefaultConfig() Config {
	return Config{
		HalfLife:    72 * time.Hour,
		AgentWeight: 0.6,
		StaleAfter:  72 * time.Hour,
		TrendSpan:   72 * time.Hour,
		TrendDelta:  0.5,
		MinWeight:   0.25,
	}
}

// Trend is which way things have been going.
type Trend string

const (
	TrendUnknown   Trend = "unknown" // not enough on both sides to compare
	TrendFlat      Trend = "flat"
	TrendImproving Trend = "improving"
	TrendWorsening Trend = "worsening"
)

// Tone is the register the agent should speak in. It is the one thing a Window
// is allowed to change about a reply, alongside how readily the agent speaks up
// at all.
type Tone string

const (
	ToneNeutral Tone = "neutral"
	ToneGentle  Tone = "gentle" // low, or heading down
	ToneBright  Tone = "bright" // good, and heading up
)

// Window is a reading of someone's recent mood.
type Window struct {
	// Known is whether there is enough recent signal to say anything at all.
	// When false every other field is meaningless and the prompt branch is
	// "I don't know how they are" — not a neutral mood, an absent one.
	Known bool `json:"known"`

	// Score is the decay-weighted mean valence, −2..+2.
	Score float64 `json:"score"`

	Trend Trend `json:"trend"`

	// Staleness is how long since the last check-in, and is itself information
	// worth handing the agent (§12.6): "they have not said anything in twelve
	// days" is a fact about the relationship, not a gap in the data.
	Staleness time.Duration `json:"staleness"`

	// Stale is Staleness past the threshold. Known and Stale are both true when
	// there is history but it is too old to speak for today — the agent knows
	// they had a rough stretch last week and must not act as though that is
	// this morning.
	Stale bool `json:"stale"`

	// LastKind is the most recent check-in's mood id (not its label — labels
	// are a rendering concern, and storing one would make renaming a mood a
	// data migration).
	LastKind string    `json:"lastKind,omitempty"`
	LastAt   time.Time `json:"lastAt,omitempty"`

	// Samples is how many check-ins contributed; Weight is their total decayed
	// weight. Both are shown on the footprint page rather than hidden, because
	// "this is based on two check-ins" is the difference between a reading and
	// a guess.
	Samples int     `json:"samples"`
	Weight  float64 `json:"weight"`
}

// Read derives the window. checkins need not be sorted; anything after now is
// ignored. Unknown mood ids are skipped rather than scored as neutral — a mood
// the registry does not know has no valence, and treating it as 0 would drag
// every average toward the middle.
func Read(checkins []domain.MoodCheckin, now time.Time, cfg Config) Window {
	type sample struct {
		at      time.Time
		valence float64
		weight  float64
		kind    string
	}
	var samples []sample
	for _, c := range checkins {
		if c.CreatedAt.After(now) {
			continue
		}
		k, ok := domain.MoodKindByID(c.Mood)
		if !ok {
			continue
		}
		w := decay(now.Sub(c.CreatedAt), cfg.HalfLife)
		if c.Source == domain.MoodSourceAgent {
			w *= cfg.AgentWeight
		}
		samples = append(samples, sample{at: c.CreatedAt, valence: float64(k.Valence), weight: w, kind: c.Mood})
	}
	if len(samples) == 0 {
		return Window{Trend: TrendUnknown}
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].at.Before(samples[j].at) })

	var sum, total float64
	for _, s := range samples {
		sum += s.valence * s.weight
		total += s.weight
	}
	last := samples[len(samples)-1]
	w := Window{
		Samples:   len(samples),
		Weight:    total,
		LastKind:  last.kind,
		LastAt:    last.at,
		Staleness: now.Sub(last.at),
		Trend:     TrendUnknown,
	}
	w.Stale = w.Staleness > cfg.StaleAfter
	if total < cfg.MinWeight {
		// History exists but has decayed to nothing. Reporting Staleness is
		// still useful — that is the "they have been quiet a long time" signal.
		return w
	}
	w.Known = true
	w.Score = sum / total

	// The trend compares two adjacent spans. Within each span the mean is
	// unweighted: the question is "were they worse then than now", and applying
	// the decay curve inside the older span would answer a different one.
	recentCut := now.Add(-cfg.TrendSpan)
	priorCut := now.Add(-2 * cfg.TrendSpan)
	var rSum, pSum float64
	var rN, pN int
	for _, s := range samples {
		switch {
		case s.at.After(recentCut):
			rSum += s.valence
			rN++
		case s.at.After(priorCut):
			pSum += s.valence
			pN++
		}
	}
	if rN > 0 && pN > 0 {
		delta := rSum/float64(rN) - pSum/float64(pN)
		switch {
		case delta >= cfg.TrendDelta:
			w.Trend = TrendImproving
		case delta <= -cfg.TrendDelta:
			w.Trend = TrendWorsening
		default:
			w.Trend = TrendFlat
		}
	}
	return w
}

// Speakable reports whether the agent may voice a read on how the user is
// doing. False means the honest move is to ask or to say nothing — not to
// narrate a three-week-old low as though it were this morning.
//
// This is the predicate prompt templates branch on. Hardcoding "the user has
// been feeling low" is the failure it exists to prevent.
func (w Window) Speakable() bool { return w.Known && !w.Stale }

// Tone is the register to speak in. An unspeakable window is neutral: not
// knowing how someone is doing is not a reason to be gentle at them.
func (w Window) Tone() Tone {
	if !w.Speakable() {
		return ToneNeutral
	}
	switch {
	case w.Score <= -0.75 || w.Trend == TrendWorsening:
		return ToneGentle
	case w.Score >= 0.75 && w.Trend != TrendWorsening:
		return ToneBright
	default:
		return ToneNeutral
	}
}

// Restrained reports whether the day should be planned lighter than usual —
// auto-plan should not fill a week that has been going badly. It is
// deliberately narrower than ToneGentle: speaking softly is cheap and low-risk,
// whereas quietly planning someone less to do is a decision about their time,
// so it wants both a low reading and a direction.
func (w Window) Restrained() bool {
	return w.Speakable() && w.Score <= -0.75 && w.Trend != TrendImproving
}

// decay is exponential with the given half-life, and never reaches zero.
func decay(age, halfLife time.Duration) float64 {
	if age <= 0 {
		return 1
	}
	if halfLife <= 0 {
		return 1
	}
	return math.Pow(0.5, age.Seconds()/halfLife.Seconds())
}
