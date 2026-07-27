package rhythm

import (
	"time"
)

// Jobs are the three scheduled moments the Daemon derives from a rhythm, as
// local "HH:MM". They are computed from Wake and Sleep rather than stored, so
// learning a person's rhythm moves all three together.
type Jobs struct {
	// PlanAt is when auto-plan runs: deep in the quiet window, shortly before
	// they get up, so tomorrow's plan is waiting rather than stale.
	PlanAt string `json:"planAt"`
	// BriefAt is the habitual wake time itself — §5 wants the morning brief
	// ready and pushed then.
	BriefAt string `json:"briefAt"`
	// ReviewAt is the evening review, placed in the window before bed.
	ReviewAt string `json:"reviewAt"`
}

// Schedule derives the three job times. With Cold and DefaultConfig this
// returns exactly the design's fallbacks: 04:00 / 07:30 / 21:00 (§5).
//
// PlanAt has a guard. Wake − PlanBefore normally lands in the middle of the
// night, but for someone who sleeps under four hours it would land *before*
// they went to bed — auto-plan would run while they were still awake, editing
// the day underneath them. When the window is that tight, PlanAt collapses to
// the midpoint of whatever sleep there is: still the quietest moment available,
// and never a moment they are known to be using.
func Schedule(p Profile, cfg Config) Jobs {
	wake, errW := parseHM(p.Wake)
	sleep, errS := parseHM(p.Sleep)
	if errW != nil || errS != nil {
		return Schedule(Cold(), cfg)
	}

	// Length of the night, wrapping midnight (sleep 22:30 → wake 07:30 is 9h).
	night := wake - sleep
	if night <= 0 {
		night += 24 * time.Hour
	}

	planOffset := cfg.PlanBefore
	if planOffset >= night {
		planOffset = night / 2
	}
	return Jobs{
		PlanAt:   hm(wake - planOffset),
		BriefAt:  hm(wake),
		ReviewAt: hm(sleep - cfg.ReviewBefore),
	}
}

// hm renders a duration-since-midnight as local "HH:MM", wrapping in both
// directions so 22:30 − 90m and 07:30 − 3h30m both come out right.
func hm(d time.Duration) string {
	m := int(d.Minutes()) % (24 * 60)
	if m < 0 {
		m += 24 * 60
	}
	return pad2(m/60) + ":" + pad2(m%60)
}

func pad2(v int) string {
	if v < 10 {
		return "0" + string(rune('0'+v))
	}
	return string(rune('0'+v/10)) + string(rune('0'+v%10))
}

// Run is an unbroken stretch of being awake, as read off the signals.
type Run struct {
	Since time.Time // first signal after the last long gap
	Last  time.Time // most recent signal
	// Continuous is how long they have been up: now − Since, not Last − Since.
	// Someone who last touched the app forty minutes ago has still been awake
	// for those forty minutes; measuring to the last signal would let the
	// counter stall exactly when someone is too tired to keep tapping.
	Continuous time.Duration
}

// CurrentRun finds how long the user has been continuously awake. signals need
// not be sorted. A zero Run (Since zero) means there is nothing recent enough
// to say — no signals at all, or the last one is older than IdleBreak, which
// reads as "they went to sleep" rather than "they have been up forever".
func CurrentRun(signals []Signal, now time.Time, cfg Config) Run {
	awake := make([]time.Time, 0, len(signals))
	for _, s := range signals {
		if s.Kind.Awake() && !s.At.After(now) {
			awake = append(awake, s.At)
		}
	}
	if len(awake) == 0 {
		return Run{}
	}
	sortTimes(awake)

	last := awake[len(awake)-1]
	if now.Sub(last) >= cfg.IdleBreak {
		return Run{} // the run ended; they are presumably asleep
	}
	since := last
	for i := len(awake) - 1; i > 0; i-- {
		if awake[i].Sub(awake[i-1]) >= cfg.IdleBreak {
			break
		}
		since = awake[i-1]
	}
	return Run{Since: since, Last: last, Continuous: now.Sub(since)}
}

// NeedsProtector reports whether this run has crossed the threshold where §5
// wants the Daemon to speak up — "你快 20 小时没合眼了，上午的安排我先帮你顺延，
// 去睡一会？"
//
// It says nothing about whether to actually send one. That depends on the push
// budget, the attention ladder, and whether one already went out for this run;
// the caller owns all three. A pure predicate here means the threshold can be
// tested without a scheduler.
func (r Run) NeedsProtector(cfg Config) bool {
	return !r.Since.IsZero() && r.Continuous >= cfg.ProtectAfter
}

func sortTimes(ts []time.Time) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && ts[j].Before(ts[j-1]); j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}
