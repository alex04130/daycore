package rhythm

import "time"

// Persisting rhythm the obvious way — one row per signal — would make it the
// largest table in the schema by an order of magnitude: a foreground heartbeat
// once a minute is ~1,400 rows per person per day, ~30,000 inside the 21-day
// learning window, for a package whose entire output is four numbers.
//
// It is also unnecessary. Nothing downstream wants the raw stream:
//
//   - Learn only reads each rhythm day's first and last awake moment, so one
//     row per day carries everything it uses.
//   - CurrentRun only needs where the present stretch of being awake began,
//     which two columns can track in O(1) as signals arrive.
//
// This file is that pair. The raw-signal entry points stay — they are the
// reference implementation, and tests hold the two in agreement — but storage
// keeps days and a running mark, not a firehose.

// Day is one rhythm day's bounds. Minutes are measured from that day's cut
// (Config.DayCutHour), not from midnight, so 02:00 sorts after 23:00 instead of
// before it — the ordering the whole median depends on.
type Day struct {
	Key      string `json:"key"` // YYYY-MM-DD of the day it STARTED
	FirstMin int    `json:"firstMin"`
	LastMin  int    `json:"lastMin"`
	Signals  int    `json:"signals"`
}

// DayOf places an instant: which rhythm day it belongs to, and how far into
// that day it is.
func DayOf(at time.Time, loc *time.Location, cfg Config) (key string, minute int) {
	local := at.In(loc)
	return DayKey(local, cfg.DayCutHour), minutesFromCut(local, cfg.DayCutHour)
}

// Extend widens a day with one more observation. The zero Day has no bounds
// yet, so the first observation sets both ends rather than widening from zero —
// otherwise every day would appear to start at its cut.
func (d Day) Extend(key string, minute int) Day {
	if d.Signals == 0 {
		return Day{Key: key, FirstMin: minute, LastMin: minute, Signals: 1}
	}
	out := d
	out.Key = key
	if minute < out.FirstMin {
		out.FirstMin = minute
	}
	if minute > out.LastMin {
		out.LastMin = minute
	}
	out.Signals++
	return out
}

// Usable reports whether a day says anything about when it began and ended. A
// single check-in does not: one tap at 15:00 is not evidence of waking at
// 15:00.
func (d Day) Usable() bool { return d.Signals > 1 && d.LastMin > d.FirstMin }

// Live is the O(1) half — enough state to answer "how long have they been up"
// without keeping the signals that answered it.
type Live struct {
	// RunSince is when the present stretch of being awake began: the first
	// signal after the last gap of at least IdleBreak.
	RunSince time.Time `json:"runSince"`
	// LastSignalAt is the most recent awake signal, which is what decides
	// whether the next one continues this stretch or starts a new one.
	LastSignalAt time.Time `json:"lastSignalAt"`
}

// Observe folds one signal in. A signal arriving after a long enough silence
// starts a new stretch; anything sooner continues the present one.
//
// Out-of-order arrivals are ignored rather than reordered. Signals come from a
// live client, so one that predates the last is a retry or a clock that stepped
// backwards, and letting it rewrite RunSince would shorten a stretch that in
// fact kept going.
func (l Live) Observe(at time.Time, cfg Config) Live {
	if !l.LastSignalAt.IsZero() && !at.After(l.LastSignalAt) {
		return l
	}
	if l.LastSignalAt.IsZero() || at.Sub(l.LastSignalAt) >= cfg.IdleBreak {
		return Live{RunSince: at, LastSignalAt: at}
	}
	return Live{RunSince: l.RunSince, LastSignalAt: at}
}

// Run reports the present stretch, or a zero Run when the last signal is old
// enough to read as "they went to sleep" — which is the safe answer for a
// feature that would otherwise wake someone up to tell them to sleep.
func (l Live) Run(now time.Time, cfg Config) Run {
	if l.LastSignalAt.IsZero() || now.Sub(l.LastSignalAt) >= cfg.IdleBreak || l.RunSince.IsZero() {
		return Run{}
	}
	if now.Before(l.RunSince) {
		return Run{}
	}
	return Run{Since: l.RunSince, Last: l.LastSignalAt, Continuous: now.Sub(l.RunSince)}
}

// DaysFrom aggregates raw signals into days — the bridge between the reference
// implementation and what storage keeps.
func DaysFrom(signals []Signal, now time.Time, loc *time.Location, cfg Config) []Day {
	byKey := map[string]Day{}
	cutoff := now.Add(-time.Duration(cfg.WindowDays) * 24 * time.Hour)
	for _, s := range signals {
		if !s.Kind.Awake() || s.At.Before(cutoff) || s.At.After(now) {
			continue
		}
		key, minute := DayOf(s.At, loc, cfg)
		byKey[key] = byKey[key].Extend(key, minute)
	}
	out := make([]Day, 0, len(byKey))
	for _, d := range byKey {
		out = append(out, d)
	}
	return out
}
