package domain

// Row limits for the list methods, defined here rather than in each store so
// the backends cannot disagree about them.
//
// Two things go wrong when these live inside an implementation:
//
//   - The pair drifts. ListImports defaulted to 20 rows on SQL and 50 on Mongo,
//     so the same call returned different history depending on which database
//     the deployment happened to use. Nothing failed; the answer was just
//     quietly different.
//   - Only some of them get a ceiling. sqlstore's limitClause already says why
//     one is needed — "an unbounded LIMIT reachable from a query parameter is a
//     way to ask the server to materialise a whole table" — but it was applied
//     to the four lists no query parameter reaches and to none of the three
//     that a request can drive straight through (/api/ops, /api/mood, and the
//     chat history endpoint all pass ?limit= down untouched). The rule was
//     right and its coverage was exactly backwards.
//
// Every list method resolves its caller-supplied limit through ListLimit with
// the pair named here. storagetest asserts both halves against every backend.
const (
	// The three a request can drive directly. Ceilings are generous — a real
	// client paging through history never approaches them — and exist only so
	// that a hostile or buggy ?limit= cannot turn one request into a full scan.
	OpLogListDefault, OpLogListMax = 50, 500
	MoodListDefault, MoodListMax   = 10, 500
	ChatListDefault, ChatListMax   = 50, 500

	// The database browser's page. Small on purpose: every column of every row
	// is shipped, so a wide table (proposals has 32 columns) makes a page of 50
	// a large response — and nobody reads a thousand raw rows in a browser.
	BrowseListDefault, BrowseListMax = 50, 200

	// The AI ledger, driven straight from ?limit= on the console's log screen.
	// A page of 50 fills a screen; the ceiling is lower than the others because
	// this is the biggest table in the database and a page of it is the widest
	// row — a 500-row page is a real amount of JSON to build and ship.
	AILogListDefault, AILogListMax = 50, 200

	// The usage rollup's day series. Two years of days, so a chart can ask for
	// "everything" without the ceiling being the thing that truncates it.
	UsageDaysDefault, UsageDaysMax = 90, 800

	// Internal lists. These already had ceilings on SQL; the numbers are kept
	// as they were so this change alters no SQL behaviour beyond the two that
	// were wrong.
	ProposalListDefault, ProposalListMax         = 100, 1000
	RhythmDaysDefault, RhythmDaysMax             = 30, 400
	JobRunListDefault, JobRunListMax             = 50, 500
	ImportListDefault, ImportListMax             = 20, 500
	WeeklyLetterListDefault, WeeklyLetterListMax = 20, 500
)

// ListLimit resolves a caller-supplied row limit: zero or negative means "give
// me the usual page", and anything above the ceiling is capped rather than
// refused. Capping rather than erroring keeps an over-eager client working —
// it gets a short page instead of a 400 — while still bounding the query.
func ListLimit(limit, def, max int) int {
	if limit <= 0 {
		return def
	}
	if limit > max {
		return max
	}
	return limit
}
