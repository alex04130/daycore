package server

import (
	"daycore/internal/domain"
	"daycore/internal/timeutil"
)

// fillBlockUTC anchors a fixed/local block to an absolute instant by computing
// its UTCTime from the wall-clock (date, time, timezone), so the same moment can
// be displayed correctly in any timezone. A floating block follows the local
// wall clock and carries no UTC anchor (UTCTime stays nil). planDate supplies the
// date when the block itself doesn't set one; fallbackTZ is used when the block
// has no timezone (UTC if that is empty too).
//
// The floating/fixed/local distinction:
//   - floating: no anchor — "9:00" is 9:00 wherever you view it (default).
//   - fixed:    anchored to an instant — an airport pickup stays the same moment.
//   - local:    anchored too, but meant to track the user's *current* local zone
//     (circadian items like sleep); anchor + display zone is the
//     frontend's job, we just record the instant.
func fillBlockUTC(b *domain.TimeBlock, planDate, fallbackTZ string) {
	if b.TimeMode != domain.TimeFixed && b.TimeMode != domain.TimeLocal {
		b.UTCTime = nil // floating / unset: wall-clock only
		return
	}
	date := b.Date
	if date == "" {
		date = planDate
	}
	if b.Time == nil || *b.Time == "" || date == "" {
		return
	}
	tz := b.Timezone
	if tz == "" {
		tz = fallbackTZ
	}
	if tz == "" {
		tz = "UTC"
	}
	utc, err := timeutil.ToUTC(date, *b.Time, tz)
	if err != nil {
		return // leave the anchor unset rather than storing a wrong instant
	}
	b.UTCTime = &utc
	if b.Timezone == "" {
		b.Timezone = tz
	}
}

// normalizePlanBlocks fills UTC anchors and derives locks for every block, in
// place, before the plan is persisted or returned. Idempotent, so it also
// backfills legacy blocks that predate either field.
//
// Lock derivation lives here because this is the one place every write already
// walks every block. Before it did, domain.DeriveLock had no production caller
// at all: every block in the database carried an empty lockLevel, so the gate
// in plan_guard.go guarded a field nothing ever set, and a class imported from
// a timetable was as movable as a note to self.
//
// DeriveLock only fills an underived lock — a level the user or the agent set
// by hand is a decision, not a guess, and re-deriving over it would make the
// unlock button do nothing that survives a refresh.
func (s *Server) normalizePlanBlocks(blocks []domain.TimeBlock, planDate, fallbackTZ, locale string) {
	for i := range blocks {
		fillBlockUTC(&blocks[i], planDate, fallbackTZ)
		domain.DeriveLock(&blocks[i], locale)
	}
}

// localizeLockReasons rewrites the reason on DERIVED locks into the reader's
// language, on the way out.
//
// The stored reason is whatever locale was current when the block was written,
// and a user picks two languages and switches between them — so a reason frozen
// at write time shows up in the wrong one. Re-resolving from the level costs a
// map lookup and makes the switch work.
//
// User- and agent-set reasons are passed through untouched. Those are somebody's
// own words ("别动，答应了室友"), not a rendering of a level, and translating
// them would be putting words in their mouth.
func localizeLockReasons(blocks []domain.TimeBlock, locale string) {
	for i := range blocks {
		// A derived reason is a rendering of the level, so it follows the reader.
		// So does an EMPTY one on any source: a user who pinned a block without
		// typing an explanation still wants the card to say something, and the
		// only honest something is the default for that level.
		if blocks[i].LockSource == domain.LockSourceDerived || blocks[i].LockReason == "" {
			blocks[i].LockReason = domain.DefaultLockReason(blocks[i].LockLevel, locale)
		}
	}
}
