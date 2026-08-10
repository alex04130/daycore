package rapport

import "daycore/internal/domain"

// FoldVersion identifies the rules that produced a cached reading.
//
// The cache is only allowed to exist because it is reproducible: re-folding the
// same ledger must give the same numbers. That guarantee is what makes the
// ledger authoritative and the cache disposable. It holds only while the rules
// stay put — so when they move, every stored reading becomes a number nobody
// can reproduce or explain, and the honest thing is to throw it away rather
// than carry it forward.
//
// # What has to bump this
//
// The rule is not "bump when you edit this package". It is exactly: **bump when
// the function from (ledger) to (scores) changes.** Its inputs are
//
//   - DeltaAccept, DeltaReject, DeltaUndo
//   - Floor, Ceiling
//   - the Domains set (adding one gives it a cold-start value that was never
//     folded; removing one orphans a stored key)
//   - Cold, the starting point every fold begins from
//   - which actions Fold scores, and how a revert is attributed
//
// # What must NOT bump it
//
// ApprenticeBelow is not an input. It reads a score; it does not produce one.
// Moving the phase line changes what the agent does with a 0.4 — it does not
// make a stored 0.4 wrong, and rebuilding every cache to arrive at the same
// number is pure cost. Same for anything else that only interprets the output:
// Score.Phase, Scores.Sorted, the gate the server eventually puts in front of
// proactivity.
//
// # How a forgotten bump gets caught
//
// Not by discipline. testdata/fold-golden.json pins a fixed ledger against the
// scores each version produces, and TestFoldGolden fails in both directions —
// change a delta without bumping and the recorded scores no longer match; bump
// without recording and there is no entry to check against. The failure message
// says which of the two happened.
const FoldVersion = 1

// Resume returns the folder and the cursor a catch-up should start from, given
// whatever was cached.
//
// A stored reading from another version is not repaired, and neither is one
// whose cursor is missing: both come back as a cold folder and a zero cursor,
// which is to say "re-fold the whole ledger". Discarding the cursor along with
// the scores is the load-bearing half. Keeping it would tell the caller to fold
// forward from the middle over scores of zero, so everything before that point
// is skipped forever and the rebuilt reading is permanently low — and because
// it would then be stamped with the current version, the next read takes the
// same path and it never heals.
//
// resolve is passed through in every case, including the cold one. A full
// rebuild and an incremental catch-up are then literally the same loop, which
// is the only way "same version + same ledger ⇒ same scores" can be true: a
// rebuild that folded with a nil Origin would skip reverts whose originals sort
// before them, and disagree with the catch-up it is supposed to reproduce.
func Resume(state *domain.RapportState, resolve Origin) (*Folder, domain.LogCursor) {
	if state == nil || state.FoldVersion != FoldVersion || state.Cursor.ID == "" {
		return NewFolderFrom(Cold(), resolve), domain.LogCursor{}
	}
	return NewFolderFrom(FromStored(state.Scores), resolve), state.Cursor
}

// FromStored and Snapshot convert between this package's Scores and the shape
// the storage layer carries. The duplication exists so that domain stays free
// of rapport and rapport stays free of storage (derived.go says as much); the
// price of pointing the dependency one way is these two functions, and having
// them in one place is what keeps the price from being paid at every call site
// in a slightly different way.
func FromStored(m map[string]domain.RapportScore) Scores {
	out := make(Scores, len(m))
	for d, v := range m {
		out[d] = Score{Domain: d, Value: v.Value, Evidence: v.Evidence}
	}
	return out
}

// Snapshot packages a folded result for storage. The cursor is the caller's to
// choose and must come from domain.AdvanceCursor rather than from the last row
// read — see the hazard documented there.
func Snapshot(sessionID string, s Scores, cursor domain.LogCursor) *domain.RapportState {
	m := make(map[string]domain.RapportScore, len(s))
	for d, v := range s {
		m[d] = domain.RapportScore{Value: v.Value, Evidence: v.Evidence}
	}
	return &domain.RapportState{
		SessionID:   sessionID,
		Scores:      m,
		Cursor:      cursor,
		FoldVersion: FoldVersion,
	}
}
