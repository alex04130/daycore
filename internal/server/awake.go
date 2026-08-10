package server

import (
	"context"
	"sync"
	"time"

	"daycore/internal/rhythm"
)

// Awake-signal recording: the input half of rhythm learning.
//
// The learning job and the Protector are later work (they do not exist yet), but
// recording is wired now on purpose. `Config.MinDays` is 5 and `WindowDays` is
// 21, so the learner needs weeks of history before it can say anything — and
// **a signal cannot be backfilled**. Every day this is not recording is a day
// the learner will not have when it lands. Recording is append-only, forward-
// only, and nothing reads it yet, so switching it on early costs nothing and
// buys the only input that has to be collected in real time.
//
// What counts as awake is decided by *where this is called from*, not by a list
// of paths. It hangs off `requireSession`, and the two machine-push endpoints
// (Canvas and ICS) authenticate through `importSession` instead — so "a plugin
// pushed data at 04:00" cannot claim the user was up, and neither can an inbound
// channel message, which never passes through an HTTP handler at all. A path
// allowlist would express the same rule and then drift away from it.

const (
	// awakeThrottle is how often one session may write a signal. Reads are the
	// common request and a write per read would put a row update on the hot path
	// for no gain: the day row only records the first and last minute seen, so
	// two signals a minute apart are indistinguishable from one.
	//
	// Five minutes also sets the resolution of "continuously active", which is
	// compared against IdleBreak (3h) and ProtectAfter (20h) — three orders of
	// magnitude of headroom.
	awakeThrottle = 5 * time.Minute

	// awakeTrackerCap bounds the in-memory throttle map. It is a cache, not
	// state: dropping an entry causes one extra write, never a lost signal.
	awakeTrackerCap = 20000
)

type awakeTracker struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newAwakeTracker() *awakeTracker {
	return &awakeTracker{last: make(map[string]time.Time)}
}

// admit reports whether this session's signal should be written, and records
// that it was. Per-instance: two instances each writing once per throttle window
// is fine, because `Observe` widens the day's bounds rather than counting.
func (t *awakeTracker) admit(sid string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if prev, ok := t.last[sid]; ok && now.Sub(prev) < awakeThrottle {
		return false
	}
	if len(t.last) >= awakeTrackerCap {
		// Drop the whole map rather than scanning for the oldest entries: this is
		// a throttle, and the penalty for a cold map is one extra write per
		// active session. A cap that is never enforced is the real hazard —
		// sessions accumulate for the life of the process.
		t.last = make(map[string]time.Time, awakeTrackerCap/2)
	}
	t.last[sid] = now
	return true
}

// markAwake records that this session was doing something at this moment.
//
// Best-effort by design: a request must not fail because a rhythm row could not
// be written. It is also deliberately not on the request's own context — the
// write outlives the response and a client that disconnects mid-request was
// still awake.
func (s *Server) markAwake(sid string) {
	if s == nil || s.store == nil || sid == "" || s.awake == nil {
		return
	}
	now := time.Now()
	if !s.awake.admit(sid, now) {
		return
	}
	// Schedule this session's proactive jobs if nobody has yet. Riding the same
	// admission gate means the first request from a session after boot schedules
	// it, and later requests cost nothing; ScheduleUser is idempotent anyway.
	if f := s.scheduleOnUse.Load(); f != nil {
		(*f)(sid)
	}
	// The session's own zone (ζ-4). This used to be the deployment-wide
	// WORKER_DEFAULT_TZ, which drew a user in another zone's day boundary in the
	// wrong place — and the day boundary is the whole unit the learner works in.
	//
	// It reads the session on the hot path only once per awakeThrottle (five
	// minutes), because markAwake has already passed the admission gate by the
	// time it gets here.
	loc := s.sessionLocation(context.Background(), sid)
	// Shared with the learner: two independent DefaultConfig() calls would
	// silently mis-bucket every row the day one of them changed. See rhythm_job.go.
	cfg := rhythmConfig()
	day, minute := rhythm.DayOf(now, loc, cfg)

	s.GoTracked(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 5*time.Second)
		defer cancel()
		if err := s.store.Rhythm().Observe(ctx, sid, day, minute); err != nil {
			s.log.Debug("rhythm: observe failed", "sid", sid, "day", day, "err", err)
		}
		s.foldAwakeMark(ctx, sid, now, cfg)
		// The user is in the app. Anything queued for them — a Protector nudge
		// made at four in the morning, a daemon suggestion — becomes visible
		// now rather than at the moment it was made. See proposal_delivery.go.
		s.deliverQueued(ctx, sid)
	})
}

// SetScheduleOnUse installs the callback that gives a session its cron entries
// the first time it is seen.
//
// It is a callback rather than a direct Worker call because the Worker is built
// after the Server (it needs it), and because a Server without one — every test,
// and a future degraded boot — must still serve requests. Atomic rather than
// mutex-guarded: it is written once at startup and read on every request.
func (s *Server) SetScheduleOnUse(f func(sid string)) {
	if f == nil {
		return
	}
	s.scheduleOnUse.Store(&f)
}

// foldAwakeMark advances the O(1) "how long have they been up" state.
//
// # Why this had to exist before the Protector could
//
// rhythm_profiles.run_since and last_signal_at had NO production writer. The
// day rows were being written (Observe, above) and the two live marks were not,
// so rhythm.Live.Run read a zero RunSince — which it correctly interprets as
// "they are asleep" — and NeedsProtector was permanently false.
//
// Wiring the Protector without this would have produced a feature that compiles,
// whose tests pass on fabricated rows, and that never once fires in production
// with no error and no log line. That is the shape this repo has shipped six
// times now, and it is the reason the input side goes in first.
//
// # Why Touch and not Save
//
// The row has two writers on wildly different cadences: this one, on every
// admitted signal, and the nightly learner, which writes wake/sleep/source/days.
// A whole-row Save from the learner would carry a snapshot of the marks taken
// before it started and write it back minutes later, erasing a stretch that
// began in between — and a zero RunSince reads as asleep, so the Protector would
// forget somebody had been up for nine hours. Touch writes only the two marks,
// and only forward.
//
// Best-effort, like everything else on this path: a request must not fail
// because a rhythm mark could not be written.
func (s *Server) foldAwakeMark(ctx context.Context, sid string, now time.Time, cfg rhythm.Config) {
	var live rhythm.Live
	// Both stores return ErrNotFound rather than (nil, nil) for a missing row.
	// Treating any error as fatal here would mean the very first signal never
	// seeds the row, silently.
	if p, err := s.store.Rhythm().Get(ctx, sid); err == nil && p != nil {
		live = rhythm.Live{RunSince: p.RunSince, LastSignalAt: p.LastSignalAt}
	}
	next := live.Observe(now, cfg)
	if next == live {
		return // an out-of-order signal; Observe already declined it
	}
	if err := s.store.Rhythm().Touch(ctx, sid, next.RunSince, next.LastSignalAt); err != nil {
		s.log.Debug("rhythm: touch failed", "sid", sid, "err", err)
	}
}
