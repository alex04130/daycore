package domain

import "time"

// The ledger's timestamps are not commit-ordered, and a keyset cursor over them
// is therefore unsafe near the present.
//
// OperationLogRepository.Add stamps created_at in Go, before the row is written
// (there are no transactions anywhere in this package's implementations, so
// there is nowhere later to put it). Two writers that stamp microseconds apart
// can be committed in the other order — on SQLite the second one may sit on the
// write lock for up to busy_timeout, five seconds, with its timestamp already
// taken. A reader that advances its cursor to the newest row it can see will
// then skip everything that was stamped earlier and landed afterwards, and
// because the predicate is `created_at > cursor` it skips them FOREVER.
//
// This is not a rare interleaving. Measured on SQLite with eight concurrent
// writers and an incremental consumer: 34 to 39 rows out of 480 were never
// delivered, reproducibly — around eight per cent.
//
// It matters because rapport is defined as a reading of the ledger and is only
// allowed to be cached because the cache is reproducible by re-folding. A
// catch-up that silently drops eight per cent of operations makes the cache
// authoritative over the ledger, which is exactly backwards.
const OpLogVisibilityLag = 30 * time.Second

// SafeCursorHorizon is the newest instant a persisted cursor may point at.
// Anything at or after it may still have rows behind it in flight.
func SafeCursorHorizon(now time.Time) time.Time {
	return now.Add(-OpLogVisibilityLag)
}

// AdvanceCursor returns the cursor to persist after folding page.
//
// Rows newer than the horizon are still returned to the caller and must still
// be folded — they are real operations and dropping them would make the reading
// lag reality by half a minute. What they must not do is move the cursor, so
// the next pass reads them again along with whatever finally landed behind
// them.
//
// That re-read is the reason folding has to be idempotent per operation id.
// The two halves are a pair: hold the cursor back without deduplicating and
// every catch-up double-counts the tail; deduplicate without holding the cursor
// back and the losses above stay. rapport.Folder skips ids it has already
// folded for this reason.
//
// prev is returned unchanged when the page is empty or entirely inside the
// unsafe window, so a caller can loop on this without special cases.
func AdvanceCursor(prev LogCursor, page []OperationLog, now time.Time) LogCursor {
	horizon := SafeCursorHorizon(now)
	out := prev
	for _, l := range page {
		if !l.CreatedAt.Before(horizon) {
			break // page is oldest-first; everything after this is newer still
		}
		out = LogCursor{CreatedAt: l.CreatedAt, ID: l.ID}
	}
	return out
}
