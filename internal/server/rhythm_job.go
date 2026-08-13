package server

import (
	"context"
	"errors"
	"time"

	"daycore/internal/domain"
	"daycore/internal/rhythm"
)

// The nightly rhythm learning job — the thing that turns the rows markAwake has
// been writing into a profile anybody reads.
//
// Before this, `internal/rhythm` was 515 lines of pure functions with **two**
// production callers, both in awake.go and both only used to compute a day key.
// `Learn`, `LearnDays`, `Schedule`, `Cold`, `Pin`, `Profile` and the whole of
// schedule.go had none. The Rhythm repository's Get/Save/Touch/Days/PruneDays
// were equally unreached — only Observe was live. That is the sixth time this
// repo has shipped a package that was written, tested, and never called.
//
// It also means the claim in ARCHITECTURE.md that the three job times are
// "derived from the rhythm" was true only numerically: Schedule(Cold()) happens
// to produce 04:00 / 07:30 / 21:00, which is exactly what the cron specs had
// hard-coded, so nobody could tell that learning something else would change
// nothing.

// rhythmConfig is the single source of the rhythm parameters.
//
// Both ends must agree: markAwake writes day keys with DayCutHour and the
// learner reads them with DayCutHour. Two independent DefaultConfig() calls
// would work today and silently mis-bucket every row the day somebody changes
// one of them — the rows would still be written, still be read, and mean a
// different day.
func rhythmConfig() rhythm.Config { return rhythm.DefaultConfig() }

// profileFor loads a session's stored profile, or the zero value when there is
// no row.
//
// Zero rather than Cold(): the caller needs to distinguish "we have never
// learned anything" from "we learned the defaults", and Cold() is indisting-
// uishable from a real learned result that happens to match.
func (w *Worker) profileFor(ctx context.Context, sid string) rhythm.Profile {
	if w.s == nil || w.s.store == nil {
		return rhythm.Profile{}
	}
	p, err := w.s.store.Rhythm().Get(ctx, sid)
	if err != nil || p == nil {
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			w.log.Debug("rhythm: could not load profile", "sid", sid, "err", err)
		}
		return rhythm.Profile{}
	}
	return rhythm.Profile{
		Wake: p.Wake, Sleep: p.Sleep, Source: rhythm.Source(p.Source), Days: p.Days,
	}
}

// jobsFor is when this session's scheduled work should happen.
//
// Schedule falls back to Cold() on its own for an empty profile, so there is no
// second fallback here — one place to be right about it.
func (w *Worker) jobsFor(ctx context.Context, sid string) rhythm.Jobs {
	return rhythm.Schedule(w.profileFor(ctx, sid), rhythmConfig())
}

// runRhythmLearn recomputes one session's profile from its recent days.
//
// Runs at the day cut (04:00 local by default), because that is the moment the
// rhythm day just closed and yesterday's row is final. The alternative — just
// before PlanAt — would make the job's own trigger time depend on the thing it
// computes, and a bootstrapping loop is a bad property for the piece everything
// else is scheduled from.
//
// The cost of this choice, stated because it is real: for somebody who wakes
// before 04:00 the new profile lands after today's PlanAt, so a change takes
// effect a day later than it could.
func (w *Worker) runRhythmLearn(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg := rhythmConfig()
	loc := resolveLocation(tz)
	now := time.Now()
	todayKey := rhythm.DayKey(now.In(loc), cfg.DayCutHour)

	// One occurrence per session per rhythm day. Claimed before the read because
	// unlike the briefs there is no suppression gate here — the job either runs
	// or somebody else already ran it.
	run, ok := w.claim(ctx, sid, domain.JobRhythmLearn, todayKey)
	if !ok {
		return
	}
	var jobErr error
	defer func() { w.finish(ctx, run, jobErr) }()

	prev := w.profileFor(ctx, sid)
	if prev.Source == rhythm.SourcePinned {
		// "我就是夜猫子，别管我". LearnDays would return it untouched anyway; not
		// reading the rows at all is the same answer for less work, and it keeps
		// the pin visible here rather than only inside the pure function.
		return
	}

	windowStart := rhythm.DayKey(now.In(loc).AddDate(0, 0, -cfg.WindowDays), cfg.DayCutHour)
	rows, err := w.s.store.Rhythm().Days(ctx, sid, cfg.WindowDays)
	if err != nil {
		jobErr = err
		w.log.Warn("rhythm: could not read days", "sid", sid, "err", err)
		return
	}

	days := rhythmWindow(rows, windowStart, todayKey)
	next := mergeLearned(prev, rhythm.LearnDays(prev, days, cfg))

	if next == prev {
		return // nothing moved; do not churn updated_at
	}
	if err := w.s.store.Rhythm().Save(ctx, &domain.RhythmProfile{
		SessionID: sid, Wake: next.Wake, Sleep: next.Sleep,
		Source: string(next.Source), Days: next.Days,
	}); err != nil {
		jobErr = err
		w.log.Warn("rhythm: could not save profile", "sid", sid, "err", err)
		return
	}
	w.log.Info("rhythm learned", "sid", sid, "wake", next.Wake, "sleep", next.Sleep,
		"source", next.Source, "days", next.Days)

	// The learned times feed the cron specs, so the entries have to be rebuilt
	// or the profile is a value nobody acts on.
	//
	// ⚠️ RescheduleUser, not ScheduleUser. The latter returns immediately when
	// the session is already scheduled at this timezone — which it always is
	// here, because what moved is the WAKE TIME, not the zone. The comment above
	// described the intent and the call defeated it: a learned 06:40 did nothing
	// until the process restarted.
	if changed := prev.Wake != next.Wake || prev.Sleep != next.Sleep; changed && w.s.worker != nil {
		w.s.worker.RescheduleUser(sid, tz)
	}

	// Pruning is bounded by the same window the learner reads, so a row that
	// still matters is never dropped. A failure here does NOT fail the job: the
	// learning succeeded, and marking the occurrence failed would spend a retry
	// on housekeeping.
	if n, err := w.s.store.Rhythm().PruneDays(ctx, sid, windowStart); err != nil {
		w.log.Warn("rhythm: could not prune days", "sid", sid, "err", err)
	} else if n > 0 {
		w.log.Debug("rhythm: pruned old days", "sid", sid, "count", n)
	}
}

// rhythmWindow narrows stored rows to the days that may be learned from.
//
// Two filters, and both are load-bearing:
//
//   - `>= windowStart`. rhythm.LearnDays does NOT apply the window — the window
//     lives in DaysFrom, which is the signal-list path, not the stored-rows
//     path. An intermittent user's most recent 21 ROWS can span six months, and
//     feeding those in means learning today's rhythm from last winter. Silent,
//     no error, nobody would ever notice.
//   - `< todayKey`. Today's row is still growing: LastMin keeps rising until the
//     user goes to bed, so including it teaches an ever-earlier bedtime. At
//     04:00 today's row is almost always absent and this looks like a no-op —
//     until somebody triggers the job by hand.
//
// The `limit` passed to Days is a ROW COUNT, not a date filter, so it cannot do
// either of these.
func rhythmWindow(rows []domain.RhythmDay, windowStart, todayKey string) []rhythm.Day {
	out := make([]rhythm.Day, 0, len(rows))
	for _, r := range rows {
		if r.Day < windowStart || r.Day >= todayKey {
			continue
		}
		out = append(out, rhythm.Day{
			Key: r.Day, FirstMin: r.FirstMin, LastMin: r.LastMin, Signals: r.Signals,
		})
	}
	return out
}

// mergeLearned decides what to keep when the evidence thins out.
//
// rhythm.LearnDays answers one question — "what do THESE days say" — and when
// there are too few usable ones it answers Cold(), which is correct for that
// question. Writing that answer straight back is not: three weeks away would
// throw out three months of learning and move somebody's morning brief back to
// 07:30 overnight, with no explanation and nothing they did.
//
// So the policy lives here rather than in the pure function: keep the learned
// times, report the honest (lower) day count. The profile stops gaining
// confidence, it does not lose what it had.
//
// The trade: a user whose rhythm genuinely changed during a long gap keeps the
// old times until they accumulate MinDays of new evidence. That is a few days of
// being slightly wrong, against the alternative of being reset to a default that
// was never right for them. It also keeps `Source` honest — "learned" continues
// to mean "learned from this person", which is what the footprint page shows.
func mergeLearned(prev, next rhythm.Profile) rhythm.Profile {
	if next.Source != rhythm.SourceDefault || prev.Source != rhythm.SourceLearned {
		return next
	}
	kept := prev
	kept.Days = next.Days
	return kept
}
