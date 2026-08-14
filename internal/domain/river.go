package domain

import "context"

// RiverDay is one day of the activity river: how much happened and how the
// user felt, reduced to the two things a colour band can show.
//
// The river is the "far view" of the footprint (EXPERIENCE_CORE §九): the
// ledger up close, a band of activity from afar. A day with no operations and
// no check-in still appears as a row — the band is continuous, and an empty day
// is a fact ("nothing happened") rather than a hole in the timeline.
type RiverDay struct {
	// Date is the UTC day, YYYY-MM-DD.
	Date string `json:"date"`
	// Count is how many operation_logs rows were created on that UTC day.
	Count int64 `json:"count"`
	// Mood is the emoji of the day's mood check-in, or "" when there is none.
	//
	// Resolution: the NEWEST check-in of the day whose mood id is in the
	// registry wins. A check-in whose id the registry no longer knows is skipped
	// rather than rendered as a raw id, so an older resolvable one still shows;
	// a day whose every check-in is unresolvable reads as "".
	Mood string `json:"mood"`
}

// RiverRepository folds the operation log and the mood check-ins into the
// per-day band the river renders.
type RiverRepository interface {
	// Days returns the last `days` days inclusive of today, oldest first — one
	// row per day, even when nothing happened. `days` is 1..90; the handler is
	// the place that clamps the request parameter, so this trusts the caller.
	Days(ctx context.Context, sessionID string, days int) ([]RiverDay, error)
}
