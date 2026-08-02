// Package rhythm learns when a person is usually awake, so that scheduled work
// can go where it belongs: auto-plan in the quiet hours, the morning brief at
// the hour they actually get up, the evening review before they turn in. The
// alternative is a fixed 07:00 brief, which is a small daily insult to anyone
// who does not get up at 07:00.
//
// Status: this package computes the profile; the Worker does not read it yet
// and still schedules from WORKER_DEFAULT_TZ. Signals are being recorded now
// (see server/awake.go) precisely because they cannot be backfilled — the
// learning needs MinDays of history before it can answer anything at all.
//
// Everything here is a pure function of a slice of signals plus a clock — no
// storage, no timers, no background state. Like petrification and rapport, a
// wrong parameter costs the next read, not a migration.
//
// EXPERIENCE_CORE §5 layer 3.
package rhythm

import (
	"fmt"
	"sort"
	"time"
)

// SignalKind is what produced an observation. Only genuine interaction counts
// as being awake (§5): an intent the user typed, something they tapped, a
// foreground heartbeat from an open app.
//
// What is deliberately excluded is anything that can happen while they sleep —
// a browser extension pushing Canvas data on a timer, a message arriving on a
// bound QQ channel. Counting those would teach the Daemon that someone whose
// phone buzzes at 03:00 is a night owl, and it would then schedule their
// morning brief accordingly.
type SignalKind string

const (
	KindIntent    SignalKind = "intent"    // typed or spoke something
	KindUI        SignalKind = "ui"        // tapped, dragged, checked off
	KindHeartbeat SignalKind = "heartbeat" // app in the foreground
)

// Awake reports whether a kind counts as evidence of being awake. Unknown kinds
// are false: a new signal source has to opt in deliberately, because the failure
// mode of opting in by accident is silent and takes weeks to notice.
func (k SignalKind) Awake() bool {
	switch k {
	case KindIntent, KindUI, KindHeartbeat:
		return true
	}
	return false
}

// Signal is one observation that the user was awake at a moment.
type Signal struct {
	At   time.Time
	Kind SignalKind
}

// Config holds the tunables. None of these come from the design original except
// the three cold-start times, which Cold reproduces exactly.
type Config struct {
	// DayCutHour is where one rhythm day ends and the next begins, in local
	// hours. It is NOT midnight: someone still working at 02:00 is having a
	// long Tuesday, not a very early Wednesday, and splitting their session at
	// midnight would record it as two short days and learn nothing from either.
	DayCutHour int

	// MinDays is how many rhythm days with usable signals are needed before the
	// learned times are trusted over the cold-start defaults. Below it the
	// profile still reports what it has seen — the footprint page shows it —
	// but Source stays "default" and the scheduler keeps the fallbacks.
	MinDays int

	// WindowDays bounds how far back learning looks. A rhythm from a year ago
	// is not this person's rhythm.
	WindowDays int

	// IdleBreak is the gap in signals that counts as having slept. Deliberately
	// long: a quiet afternoon at work is not a nap, and mistaking one for the
	// other resets the continuous-activity counter and silences the Protector
	// exactly when it is needed. Erring long means the Protector fires rarely
	// and means it — under-triggering is the right failure for a feature whose
	// whole tone is concern rather than nagging.
	IdleBreak time.Duration

	// ProtectAfter is how long a person can be continuously active before the
	// Protector speaks up (§5, "≈20 小时").
	ProtectAfter time.Duration

	// PlanBefore places auto-plan this long before the habitual wake — deep
	// enough into the night that nothing is running, close enough that the plan
	// is fresh when they open it.
	PlanBefore time.Duration

	// ReviewBefore places the evening review this long before habitual sleep.
	ReviewBefore time.Duration
}

// DefaultConfig reproduces the design's cold-start times exactly: with the cold
// profile below, PlanAt is 04:00, BriefAt is 07:30 and ReviewAt is 21:00 (§5).
//
// The three job times are derived from two body times plus two offsets rather
// than being stored as three independent numbers, so learning a person's wake
// and sleep moves all three together and they cannot drift into an arrangement
// that makes no sense (a review scheduled after bedtime, say).
func DefaultConfig() Config {
	return Config{
		DayCutHour:   4,
		MinDays:      5,
		WindowDays:   21,
		IdleBreak:    3 * time.Hour,
		ProtectAfter: 20 * time.Hour,
		PlanBefore:   3*time.Hour + 30*time.Minute,
		ReviewBefore: 90 * time.Minute,
	}
}

// Source says where a profile's times came from. It is shown to the user on the
// footprint page — "learned from 12 days" reads very differently from "this is
// just the default", and the difference is the whole reason the page exists.
type Source string

const (
	SourceDefault Source = "default" // not enough evidence yet
	SourceLearned Source = "learned" // median of observed days
	SourcePinned  Source = "pinned"  // the user said so
)

// Profile is a person's rhythm: two body times, and where they came from.
type Profile struct {
	// Wake and Sleep are local wall clock "HH:MM". Sleep may be past midnight
	// ("02:15") — that is the normal case for a night owl and is exactly why
	// rhythm days are cut at DayCutHour rather than at midnight.
	Wake  string `json:"wake"`
	Sleep string `json:"sleep"`

	Source Source `json:"source"`
	// Days is how many rhythm days of evidence stand behind Wake and Sleep. It
	// is reported even when Source is "default", so the footprint page can show
	// progress towards being learned.
	Days int `json:"days"`
}

// Cold is where everyone starts: up at 07:30, asleep at 22:30. With
// DefaultConfig that puts auto-plan at 04:00 and the evening review at 21:00 —
// the three fallback times §5 specifies.
func Cold() Profile {
	return Profile{Wake: "07:30", Sleep: "22:30", Source: SourceDefault}
}

// Pin fixes a rhythm by hand — "我就是夜猫子，别管我". A pinned profile is never
// overwritten by learning; Learn returns it unchanged.
func Pin(wake, sleep string) (Profile, error) {
	if _, err := parseHM(wake); err != nil {
		return Profile{}, fmt.Errorf("rhythm: wake: %w", err)
	}
	if _, err := parseHM(sleep); err != nil {
		return Profile{}, fmt.Errorf("rhythm: sleep: %w", err)
	}
	return Profile{Wake: wake, Sleep: sleep, Source: SourcePinned}, nil
}

// Learn folds raw observations into a profile. signals need not be sorted.
//
// This is the reference path — a one-off recompute, and what the tests drive.
// The live path aggregates as it goes and calls LearnDays; both go through the
// same core so the two cannot drift.
func Learn(prev Profile, signals []Signal, now time.Time, loc *time.Location, cfg Config) Profile {
	return LearnDays(prev, DaysFrom(signals, now, loc, cfg), cfg)
}

// LearnDays folds pre-aggregated rhythm days into a profile — what the storage
// layer calls, since it keeps one row per day rather than one per signal.
//
// A pinned profile is returned untouched. That check is here rather than at
// every call site so there is one place to be right about it.
func LearnDays(prev Profile, days []Day, cfg Config) Profile {
	if prev.Source == SourcePinned {
		return prev
	}
	usable := make([]Day, 0, len(days))
	for _, d := range days {
		if d.Usable() {
			usable = append(usable, d)
		}
	}
	if len(usable) < cfg.MinDays {
		// Report progress towards being learned, but keep the scheduler on the
		// defaults — two days of evidence is an anecdote.
		cold := Cold()
		cold.Days = len(usable)
		return cold
	}

	wakes := make([]int, 0, len(usable))
	sleeps := make([]int, 0, len(usable))
	for _, d := range usable {
		wakes = append(wakes, d.FirstMin)
		// Sleep is expressed as minutes from the day cut so that 02:00 sorts
		// after 23:00 instead of before it. Taking a median of raw wall-clock
		// minutes would put a night owl's bedtime at lunchtime.
		sleeps = append(sleeps, d.LastMin)
	}
	return Profile{
		Wake:   hmFromCutOffset(medianInt(wakes), cfg.DayCutHour),
		Sleep:  hmFromCutOffset(medianInt(sleeps), cfg.DayCutHour),
		Days:   len(usable),
		Source: SourceLearned,
	}
}

// DayKey names the rhythm day an instant belongs to, as YYYY-MM-DD of the day
// it *started*. Anything before the cut belongs to the previous day.
func DayKey(local time.Time, cutHour int) string {
	if local.Hour() < cutHour {
		local = local.AddDate(0, 0, -1)
	}
	return local.Format("2006-01-02")
}

// minutesFromCut measures a local time as minutes since that rhythm day's cut,
// so an ordering exists across midnight: with a 04:00 cut, 23:00 is 1140 and
// 02:00 the next morning is 1320.
func minutesFromCut(local time.Time, cutHour int) int {
	m := local.Hour()*60 + local.Minute() - cutHour*60
	if m < 0 {
		m += 24 * 60
	}
	return m
}

func hmFromCutOffset(off, cutHour int) string {
	m := (off + cutHour*60) % (24 * 60)
	if m < 0 {
		m += 24 * 60
	}
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

func medianInt(v []int) int {
	s := append([]int(nil), v...)
	sort.Ints(s)
	n := len(s)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return s[n/2]
	}
	// The lower of the two middles rather than their mean: an average of 23:00
	// and 02:00 is 00:30, which is nobody's bedtime, whereas picking an
	// observed value always is.
	return s[n/2-1]
}

func parseHM(hm string) (time.Duration, error) {
	var h, m int
	if _, err := fmt.Sscanf(hm, "%d:%d", &h, &m); err != nil {
		return 0, fmt.Errorf("%q is not HH:MM", hm)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("%q is out of range", hm)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}
