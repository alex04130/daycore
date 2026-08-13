package server

import (
	"context"
	"errors"
	"time"

	"daycore/internal/domain"
	"daycore/internal/rapport"
)

// Reading a session's rapport.
//
// # ⚠️ internal/rapport had zero callers until this file
//
// 387 lines, a golden-sample suite, a repository implemented on four backends
// and covered by the conformance cases — and nothing anywhere read it. So did
// `store.Rapport()`. It is the largest instance in this repository of the
// pattern its own documentation keeps warning about: 「写完测过没人调用」.
//
// It is wired here because the habit scanner needs it. §8's first-cut rule is
// that the system may not volunteer a standing change to somebody it has no
// standing with, and rapport is the thing that answers "how much standing" —
// without it the scanner would either never speak or always speak, and both are
// wrong for the same reason: they are not about this reader.
//
// # The fold is cached, and the cache is a derived value
//
// Replaying the whole ledger on every read is affordable today and will not stay
// so, which is exactly when a cache stops being premature. The row carries a
// cursor and a fold version:
//
//	cursor       how far into the ledger this reading goes; a catch-up scans
//	             from there rather than from the beginning
//	foldVersion  which rules produced it. An old row is not WRONG, it is
//	             meaningless — the deltas or the domain list moved under it —
//	             so it is thrown away rather than carried forward.
//
// ⚠️ The version comparison is the whole reason FoldVersion exists, and the
// mistake it prevents is subtle: a cached number nobody can explain, that
// differs from what a replay would produce, and that nothing anywhere reports
// as stale.

// rapportFoldVersion is the version of the folding rules in internal/rapport.
//
// ⚠️ Bump it whenever the deltas, the domain list, or what counts as evidence
// changes. Not bumping it leaves every existing deployment carrying scores
// produced by rules that no longer exist — and because the numbers still look
// like numbers, nothing will report it.
const rapportFoldVersion = 1

// rapportPage is how many ledger entries one catch-up scan reads.
const rapportPage = 500

// rapportScores returns this session's reading, folding whatever the cache has
// not seen.
//
// ⚠️ Best-effort by design: a storage failure returns the cold-start baseline
// rather than an error. Every caller is deciding how forward to be, and the
// cold baseline is the cautious answer — an apprentice-phase score shows its
// evidence and clears a lower bar for volunteering nothing. Refusing to act
// because a cache read failed would make an unrelated outage look like the
// assistant going quiet.
func (s *Server) rapportScores(ctx context.Context, sid string) rapport.Scores {
	if s.store == nil {
		return rapport.Cold()
	}
	state, err := s.store.Rapport().Get(ctx, sid)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return rapport.Cold()
	}

	var folder *rapport.Folder
	cursor := domain.LogCursor{}
	if state != nil && state.FoldVersion == rapportFoldVersion {
		folder = rapport.NewFolderFrom(scoresFromState(state), s.rapportOrigin(ctx, sid))
		cursor = state.Cursor
	} else {
		// No row, or one produced by rules that have since moved. Either way the
		// answer is a full replay — and a full replay needs no Origin, because
		// every operation a revert can name has already passed through Fold.
		folder = rapport.NewFolder()
	}

	// ⚠️ Paged rather than one big read: the ledger is unbounded and a session
	// that has been running for a year has a lot of it. The loop stops when a
	// page comes back short, which is the only end condition Scan offers.
	for {
		page, err := s.store.OpLogs().Scan(ctx, sid, cursor, rapportPage)
		if err != nil {
			// Fold what was read and stop. A partial fold is a real reading of a
			// prefix of the ledger — less current than it could be, never wrong
			// about what it did see.
			break
		}
		for _, op := range page {
			folder.Fold(op)
		}
		// ⚠️ AdvanceCursor, not "the last row's stamp". The ledger's timestamps
		// are not ordered strongly enough to resume from the newest one — a row
		// written a moment ago may still be committing — so the cursor stops at
		// a safe horizon and the tail gets re-read next time. Folder.folded is
		// what makes that re-read harmless.
		next := domain.AdvanceCursor(cursor, page, time.Now())
		if len(page) < rapportPage || next == cursor {
			cursor = next
			break
		}
		cursor = next
	}

	scores := folder.Scores()
	// Best-effort write-back. A failure costs the next read a longer scan.
	_ = s.store.Rapport().Save(ctx, stateOf(sid, scores, cursor))
	return scores
}

// rapportOrigin lets a catch-up score a revert whose original is older than the
// window it read.
//
// ⚠️ Without it the cached number would drift from what a replay produces: a
// revert only says something about the agent when the thing reverted was the
// agent's doing, and a folder resuming from a cursor has never seen the
// original. rapport.Origin's own doc calls this "not optional in practice".
func (s *Server) rapportOrigin(ctx context.Context, sid string) rapport.Origin {
	return func(id string) (string, string, bool) {
		op, err := s.store.OpLogs().Get(ctx, sid, id)
		if err != nil || op == nil {
			return "", "", false
		}
		d := op.Domain
		if d == "" {
			d = domain.OpDomainOf(op.Action)
		}
		return op.Actor, d, true
	}
}

func scoresFromState(st *domain.RapportState) rapport.Scores {
	out := make(rapport.Scores, len(st.Scores))
	for d, s := range st.Scores {
		out[d] = rapport.Score{Domain: d, Value: s.Value, Evidence: s.Evidence}
	}
	return out
}

func stateOf(sid string, scores rapport.Scores, cursor domain.LogCursor) *domain.RapportState {
	out := &domain.RapportState{
		SessionID: sid, Cursor: cursor, FoldVersion: rapportFoldVersion,
		Scores: make(map[string]domain.RapportScore, len(scores)),
	}
	for d, s := range scores {
		out.Scores[d] = domain.RapportScore{Value: s.Value, Evidence: s.Evidence}
	}
	return out
}
