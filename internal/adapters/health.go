package adapters

import (
	"sync"
	"time"
)

// Health tracks whether one source is usable, with hysteresis.
//
// # Why hysteresis is not tuning
//
// The set of usable sources becomes the `enum` of a tool parameter, and the
// tool band is the first thing rendered in a request — before the system
// prompt. The ephemeral cache breakpoint sits on the system block, so a tool
// band that changes bytes rewrites it AND the entire four-layer system prompt
// behind it. Not "fewer cache hits": that whole round recomputed.
//
// So a source that flaps between up and down once a minute would cost every
// conversation its prompt cache, for the entire deployment. Requiring N
// consecutive agreeing observations before flipping is what makes the band
// stable enough to cache at all. It is a precondition, not a preference.
//
// # Boundary: this state is per process, and deliberately not in the database
//
// Three reasons, and the second is the one that settles it:
//
//  1. It is THIS process's reachability. An adapter is often same-host, so what
//     instance A cannot reach, instance B may reach fine. A shared row would
//     have them overwriting each other's truth.
//  2. Degraded boot has no database — and a process whose storage is gone is
//     exactly when knowing which sources answer matters most.
//  3. "Assume healthy" is the right state after a restart. A persisted "down"
//     would keep a source that recovered hours ago out of the tool band until
//     somebody noticed and cleared it by hand.
//
// The cost is that each instance learns independently and the console shows one
// instance's view, so the admin response carries the instance id. Two consoles
// disagreeing is confusing; two consoles disagreeing with no way to tell which
// machine you are looking at is unexplainable.
type Health struct {
	mu sync.Mutex

	up        bool
	streak    int // consecutive observations agreeing AGAINST the current state
	reason    string
	lastCheck time.Time
	// backoffUntil suppresses probes after a failure. Without it a dead source
	// is retried on every single call, and each retry costs the caller the full
	// dial timeout — the outage becomes a latency problem for features that do
	// not even use that source.
	backoffUntil time.Time
	failures     int
}

// FlipAfter is how many consecutive disagreeing observations flip the state.
//
// Three, from docs/specs/transport.md. Two would flip on a single retry-able
// blip plus one unlucky sample; four delays recovery past the point where an
// operator watching the console starts doubting the console.
const FlipAfter = 3

// NewHealth starts optimistic.
//
// A source nobody has called yet is presumed usable, because the alternative —
// starting down and requiring a probe to come up — means a boot-time round of
// network calls before the first request can be served, and any adapter that is
// slow to start would be excluded from the tool band of every conversation in
// the first minutes after a deploy.
func NewHealth() *Health { return &Health{up: true} }

// Up reports the current state. Cheap: this is read while assembling a tool
// band, which happens once per conversation round.
func (h *Health) Up() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.up
}

// Snapshot is what the console shows.
type Snapshot struct {
	Up           bool      `json:"up"`
	Reason       string    `json:"reason,omitempty"`
	LastChecked  time.Time `json:"lastChecked,omitempty"`
	Failures     int       `json:"failures,omitempty"`
	BackoffUntil time.Time `json:"backoffUntil,omitempty"`
}

func (h *Health) Snapshot() Snapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Snapshot{Up: h.up, Reason: h.reason, LastChecked: h.lastCheck,
		Failures: h.failures, BackoffUntil: h.backoffUntil}
}

// Observe records the outcome of one real call.
//
// Real calls only — never a cache hit. A cached forecast proves nothing about
// whether the source still answers, and counting it as a success would let a
// source stay "up" for the whole cache TTL after it died. That is why the
// health wrapper sits OUTSIDE the cache and not inside it.
func (h *Health) Observe(err error, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastCheck = now
	ok := err == nil

	if ok {
		h.failures = 0
		h.backoffUntil = time.Time{}
	} else {
		h.failures++
		h.backoffUntil = now.Add(backoff(h.failures))
	}

	if ok == h.up {
		h.streak = 0
		if ok {
			h.reason = ""
		}
		return
	}
	h.streak++
	if !ok {
		// Keep the most recent reason even before flipping: an operator looking
		// at a source that is still "up" but failing wants to see why.
		h.reason = err.Error()
	}
	if h.streak >= FlipAfter {
		h.up = ok
		h.streak = 0
		if ok {
			h.reason = ""
		}
	}
}

// ShouldProbe reports whether a half-open attempt is allowed now.
//
// # Boundary: only the interactive agent path may probe
//
// The morning and evening briefs run twice a day and are the only weather calls
// in a deployment nobody is chatting with. Letting them probe means the cost of
// discovering that a dead source recovered is paid by the two calls that most
// need to be fast and most need to succeed — and a probe that fails delays a
// brief that was otherwise ready to send. The brief takes the first healthy
// source and never probes; recovery is discovered by somebody's conversation.
func (h *Health) ShouldProbe(now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.up && now.After(h.backoffUntil)
}

// backoff is 1s, 2s, 4s… capped at 30s, the same ladder docs/specs/transport.md
// specifies for subprocess restarts. One ladder, so an operator reading either
// document learns the same numbers.
func backoff(failures int) time.Duration {
	d := time.Second
	for i := 1; i < failures && d < 30*time.Second; i++ {
		d *= 2
	}
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}
