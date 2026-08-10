package domain

import "time"

// The AI spend rollup: what this deployment has used, kept forever.
//
// # Why a rollup exists at all
//
// ai_call_logs is pruned (90 days), and AdminStats used to COUNT and SUM that
// table — so "AI 调用" and "Token 消耗" on the console were window figures
// presented as totals, and they went DOWN as the window slid. A number that
// silently changes meaning is worse than no number: nobody can act on it and
// nobody can tell it has stopped being what it says.
//
// Keeping every raw row forever to compute a sum is the expensive way to store
// a number. One row per (day, model, endpoint) is the cheap way, and it answers
// strictly more questions than a running counter would.
//
// # The database does the folding, not us
//
// The rollup is produced by ONE server-side statement per closed day —
// `INSERT … SELECT … GROUP BY` on SQL, an aggregation pipeline with `$merge`
// on Mongo. Not by incrementing a counter on every AI call.
//
// The per-call increment was the first design and it was worse in three ways
// at once: an extra write on the hot path, a read-modify-write race between
// instances (needing the UPDATE-then-INSERT-then-retry dance), and a counter
// that could not be checked against anything. Folding server-side has none of
// those, and it costs one statement per day instead of one per call.
//
// ⚠️ Stored procedures and triggers were the other candidate and are refused:
// a self-hosted mongod has no triggers at all (Atlas Triggers is a cloud
// product), so that half would come back to Go anyway — one rule with two
// implementations, living BELOW the layer storagetest can reach. And
// procedural DDL needs a versioned-migration track this repository does not
// have; ColumnMigration only ever adds columns.
//
// # Idempotent by construction, so it needs no bookkeeping
//
// A CLOSED day (strictly before today, UTC) can no longer receive ledger rows,
// so folding it is a pure function of the ledger. The job walks the days the
// ledger still holds that the rollup does not have, and writes each one whole.
// Running it twice, or after a crash halfway through, produces the same rows —
// there is no "have I already counted this" state to keep, and therefore none
// to get wrong.
//
// ⚠️ ORDER IS LOAD-BEARING: roll up BEFORE pruning. The prune deletes the rows
// the rollup is computed from, so a prune that ran first would delete a day
// that was never folded, and nothing anywhere would report the loss. They are
// deliberately in one job function for that reason.
//
// # One granularity, and it is the day
//
// Not hour+day+month. Two reasons, and the second decides it:
//
//   - Anything coarser is DERIVABLE by summing days, so storing a monthly total
//     as well would be two facts that can disagree — and the one that disagrees
//     is always the one somebody is reading.
//   - Anything finer is already answered by the raw ledger inside its
//     retention, and outside it "which hour of a Tuesday in March" is not a
//     question anybody asks. Hourly would be 24× the rows for that.
//
// If the table ever grows enough to matter — roughly (models × endpoints) rows
// per day, a few tens of thousands a year — the lever is compacting days older
// than a year into months, NOT dropping to a coarser grain now.
//
// # UTC days, deployment-wide
//
// The day key is UTC. Sessions have their own timezones and the planner cares
// about those, but this is a cost figure for the deployment as a whole, and
// "spend on the 3rd" has to mean one thing regardless of who was awake.
//
// # ⚠️ Deliberately NOT keyed by session
//
// Per-user attribution would make the row count grow with users (users ×
// models × endpoints × days), which is exactly what this table exists to
// avoid. Within the ledger's retention the raw rows answer per-user questions
// precisely; beyond it they cannot, and that is accepted. A hosted tier that
// bills per account needs metering keyed by account with its own retention — a
// billing concern, not an operations one, and a different table.

// AIUsageDay is one day's spend on one model through one endpoint.
type AIUsageDay struct {
	// Day is YYYY-MM-DD in UTC.
	Day          string `json:"day"`
	Model        string `json:"model"`
	Endpoint     string `json:"endpoint"`
	Calls        int64  `json:"calls"`
	Errors       int64  `json:"errors"`
	PromptTokens int64  `json:"promptTokens"`
	CompTokens   int64  `json:"compTokens"`
}

// AIUsageTotals is a sum over a window, with the window's real start.
type AIUsageTotals struct {
	Calls        int64 `json:"calls"`
	Errors       int64 `json:"errors"`
	PromptTokens int64 `json:"promptTokens"`
	CompTokens   int64 `json:"compTokens"`
	// FirstDay is the oldest day the rollup actually holds, or "" when empty.
	//
	// ⚠️ This is what keeps a "total" honest. The rollup can only ever be built
	// from ledger rows that still exist, so on a deployment upgrading into this
	// feature it begins wherever the ledger begins — NOT at the beginning of
	// time. Rendering the figure without this date would be the same lie the
	// rollup exists to fix, moved one level up.
	FirstDay string `json:"firstDay,omitempty"`
}

// UTCDay is the rollup's day key for an instant. One function, so the folder
// and every reader cannot disagree about where a day starts.
func UTCDay(t time.Time) string { return t.UTC().Format("2006-01-02") }

// StartOfUTCDay is the instant a day key begins, which is what turns a day into
// the millisecond range the ledger is queried by.
//
// Ranges rather than a date function in SQL: created_at is stored as epoch
// milliseconds, and every engine spells date extraction differently. Computing
// the boundary in Go keeps one definition of "a day" and keeps the query
// portable.
func StartOfUTCDay(day string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", day, time.UTC)
}
