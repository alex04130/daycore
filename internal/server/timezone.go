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

// normalizePlanBlocks fills UTC anchors for every block, in place, before the
// plan is persisted or returned. Idempotent, so it also backfills legacy blocks
// that predate the UTCTime field.
func (s *Server) normalizePlanBlocks(blocks []domain.TimeBlock, planDate, fallbackTZ string) {
	for i := range blocks {
		fillBlockUTC(&blocks[i], planDate, fallbackTZ)
	}
}
