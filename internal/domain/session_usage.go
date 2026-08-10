package domain

import "time"

// Per-account AI usage: three scales, on the session row, no table.
//
// # Why this is on the session and not in a table
//
// The deployment-wide rollup (ai_usage.go) is keyed by day and folded from the
// ledger, and it deliberately does NOT carry a session key — that would make the
// row count grow with users, which is what it exists to avoid.
//
// The per-account answer therefore has to come from somewhere else, and the
// cheapest somewhere is the row that already exists. Three counters on the
// session are nine columns and zero new rows, against a (users × days) table
// that grows forever to answer a question that is almost always asked about
// NOW.
//
// ⚠️ The session, not the user, and the difference matters:
//
//   - Anonymous sessions are the majority on a self-hosted deployment and have
//     no user row at all. Counters on `users` would silently measure a
//     minority.
//   - A signed-in user's calls already land on their CANONICAL data session
//     (User.DataSessionID; sessionIDFrom resolves to it), so "on the session"
//     IS "on the user" for anybody who has one.
//
// # Tumbling windows, and what that costs
//
// A window is a counter plus the instant it opened. When a call arrives after
// the window has expired, the counter RESETS to that call rather than sliding.
//
// The honest cost: just after a reset the fast figure reads as one call even if
// the account was busy four minutes earlier. A true sliding window needs
// per-event rows, which is the table this design exists to not have.
//
// It is the right approximation for what these are for. A quota that resets is
// what a quota IS ("400 calls a week"), and a rate limiter reading a
// just-reset window under-counts in the permissive direction for at most one
// window — which is a decision to make where the limit is set, not here.
//
// # ⚠️ This is a HOOK, not a billing system
//
// It answers "is this account using a lot, right now / this week / ever". A
// hosted tier that bills needs three things this deliberately does not have:
// per-model attribution (the price differs per model), an immutable record per
// period (these counters are overwritten), and a link to an account rather than
// a session (sessions can be merged at sign-in). Each is a real addition; none
// of them is needed to answer the operational question, and building them now
// would be building a billing system nobody is billing with.
//
// What it DOES give a future billing system is the shape of the question and a
// place to hang from — see docs/ROADMAP.md.

// Usage window lengths.
//
// One definition each, in domain, because the writer resets the window and every
// reader labels it: two places with two numbers is a screen that says "3 小时"
// over a counter that resets every four.
const (
	// UsageFastWindow is "right now" — long enough to survive a conversation,
	// short enough that a runaway loop shows up while it is still running.
	UsageFastWindow = 3 * time.Hour
	// UsageSlowWindow is the quota-shaped one. A week rather than a month
	// because a month-long window spends most of its life reading as "not much
	// yet", which is not a number anybody acts on.
	UsageSlowWindow = 7 * 24 * time.Hour
)

// SessionUsage is one account's spend at three scales.
//
// The windows carry COMBINED tokens and the lifetime total keeps the split.
// That is not an oversight: the windows are read to decide "is this too much",
// where the prompt/completion split changes no decision, and the total is read
// to answer "what did this cost", where it is the whole question. Four columns
// saved on the busiest table in the schema, and every remaining one is used.
type SessionUsage struct {
	// FastCalls / FastTokens cover UsageFastWindow, since FastWindowStart.
	FastCalls       int64     `json:"fastCalls"`
	FastTokens      int64     `json:"fastTokens"`
	FastWindowStart time.Time `json:"fastWindowStart,omitempty"`

	// SlowCalls / SlowTokens cover UsageSlowWindow, since SlowWindowStart.
	SlowCalls       int64     `json:"slowCalls"`
	SlowTokens      int64     `json:"slowTokens"`
	SlowWindowStart time.Time `json:"slowWindowStart,omitempty"`

	// Lifetime, never reset.
	TotalCalls        int64 `json:"totalCalls"`
	TotalPromptTokens int64 `json:"totalPromptTokens"`
	TotalCompTokens   int64 `json:"totalCompTokens"`
}

// FastExpired reports whether the fast window has rolled over at `now`, so a
// reader can tell "no recent usage" from "the window is stale and about to
// reset on the next call".
//
// Readers need this because the stored counter is NOT zeroed when a window
// expires — nothing runs on expiry; the reset happens on the next write. A
// reader that ignored the stamp would report yesterday's burst as current.
func (u SessionUsage) FastExpired(now time.Time) bool {
	return u.FastWindowStart.IsZero() || now.Sub(u.FastWindowStart) >= UsageFastWindow
}

// SlowExpired is the same question for the slow window.
func (u SessionUsage) SlowExpired(now time.Time) bool {
	return u.SlowWindowStart.IsZero() || now.Sub(u.SlowWindowStart) >= UsageSlowWindow
}

// Live returns the counters as a reader should understand them at `now`: an
// expired window reads as zero rather than as its stale contents.
//
// ⚠️ Use this, never the raw fields, anywhere a number reaches a person or a
// decision. The raw fields are what is stored; this is what is true.
func (u SessionUsage) Live(now time.Time) SessionUsage {
	out := u
	if u.FastExpired(now) {
		out.FastCalls, out.FastTokens, out.FastWindowStart = 0, 0, time.Time{}
	}
	if u.SlowExpired(now) {
		out.SlowCalls, out.SlowTokens, out.SlowWindowStart = 0, 0, time.Time{}
	}
	return out
}
