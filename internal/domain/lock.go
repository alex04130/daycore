package domain

import (
	"regexp"
	"strings"

	"daycore/internal/i18n"
)

// ClassTitlePattern decides whether an appointment is a class. It is copied
// byte-for-byte from the design prototype (design-ui/core/daycore-core.js
// deriveLock) and is mirrored in api/lock-rules.json so the four frontends can
// apply the same rule without drifting. Treat it as a product value, not an
// implementation detail — changing it changes which blocks the user can move.
const ClassTitlePattern = `（课）|课$|课程|讲座|实验课|导论|Lecture|Lab\s*课`

// The pattern is wrapped in a non-capturing group before (?i) is applied. In
// RE2 a flag group extends to the end of the enclosing group, and relying on
// that to reach across every `|` branch is a subtlety not worth betting on —
// the explicit group makes case-insensitivity unambiguous. ClassTitlePattern
// itself stays verbatim so the contract fixture can compare it to the JS source.
var classTitleRe = regexp.MustCompile(`(?i)(?:` + ClassTitlePattern + `)`)

// Two deliberate divergences from the JavaScript original, both recorded in
// api/lock-rules.json:
//
//   - RE2's \s is ASCII-only; JavaScript's also matches U+00A0 and U+3000. A
//     title like "Lab　课" (ideographic space) is hard in the browser and soft
//     here. The server is authoritative and corrects the client within one
//     round-trip, so the pattern is left alone rather than special-cased.
//   - Titles are trimmed before matching (the prototype does not). "数学课 "
//     with a trailing space should still be a class; the fixture requires the
//     TypeScript side to trim too.

// IsClassTitle reports whether a title looks like a class, lecture, or lab.
func IsClassTitle(title string) bool {
	return classTitleRe.MatchString(strings.TrimSpace(title))
}

// defaultLockReasons mirrors api/lock-rules.json's defaultReason map, which the
// TypeScript side reads. Keep the two in step: the fixture is the contract, this
// is one implementation of it.
//
// The en-US strings are placeholders pending final copy (the fixture marks them
// the same way).
var defaultLockReasons = map[LockLevel]i18n.Text{
	LockHard: {
		"zh-CN": "课程时间由课表决定",
		"en-US": "Set by your class timetable",
	},
	LockSoft: {
		"zh-CN": "和别人约好的时间",
		"en-US": "Time you agreed with someone else",
	},
}

// LockReasonKey is the catalog key for a derived lock reason.
func LockReasonKey(level LockLevel) string { return "lock.reason." + string(level) }

func init() {
	for lvl, t := range defaultLockReasons {
		i18n.Register(LockReasonKey(lvl), t)
	}
}

// DefaultLockReason is the reason shown when the lock was inferred rather than
// set by hand. Derived reasons are not the user's words, so they are looked up
// per request locale instead of being frozen into the block at write time —
// that is part of why LockSource exists.
func DefaultLockReason(level LockLevel, locale string) string {
	if _, ok := defaultLockReasons[level]; !ok {
		return "" // an unlocked block carries no derived reason
	}
	return i18n.T(LockReasonKey(level), locale)
}

// InferLock returns the lock level a block's type and title imply, ignoring
// whatever is currently stored on it. Appointments that read as a class are
// hard-locked (the timetable owns that hour), other appointments are soft
// (someone else is involved), and everything else is free.
func InferLock(blockType BlockType, title string) LockLevel {
	if blockType != BlockAppointment {
		return LockNone
	}
	if IsClassTitle(title) {
		return LockHard
	}
	return LockSoft
}

// DeriveLock fills in a block's lock if it has never been derived, and leaves
// it alone otherwise. This is the idempotent form: safe to call on every read
// path, and it will not undo a lock the user or the agent set deliberately.
//
// It reports whether it changed anything, so write paths can skip a needless
// store round-trip.
func DeriveLock(b *TimeBlock, locale string) bool {
	if b == nil || b.LockSource != LockSourceUnset {
		return false
	}
	b.LockLevel = InferLock(b.Type, b.Title)
	b.LockSource = LockSourceDerived
	if b.LockReason == "" {
		b.LockReason = DefaultLockReason(b.LockLevel, locale)
	}
	return true
}

// RederiveLock recomputes a lock that was previously inferred, so that editing
// a block's type or title moves its lock with it — an appointment turned into
// a task should stop being locked. Locks with a source of user or agent are
// left untouched: those are decisions, not guesses.
//
// This is deliberately separate from DeriveLock. The prototype derives once and
// never again (design-ui/core/daycore-core.js:41 returns early whenever
// lockLevel is already set), so re-deriving is a behavioural change and the
// caller has to opt into it.
func RederiveLock(b *TimeBlock, locale string) bool {
	if b == nil {
		return false
	}
	if b.LockSource == LockSourceUnset {
		return DeriveLock(b, locale)
	}
	if b.LockSource != LockSourceDerived {
		return false
	}
	level := InferLock(b.Type, b.Title)
	reason := DefaultLockReason(level, locale)
	if b.LockLevel == level && b.LockReason == reason {
		return false
	}
	b.LockLevel, b.LockReason = level, reason
	return true
}

// Movable reports whether the given actor may change a block's time. The agent
// plans around locks rather than being stopped by them — it needs to know a
// class is immovable, not to be refused — and the system must be able to move
// anything, or reverting a lock change would be impossible.
//
// A soft lock is movable only once the user has confirmed; hard is never
// movable by the user. The caller supplies confirmed from the request, not from
// the block, so a confirmation cannot be persisted and silently reused.
func (b TimeBlock) Movable(actor string, confirmed bool) bool {
	if actor == ActorAgent || actor == ActorSystem {
		return true
	}
	switch b.LockLevel {
	case LockHard:
		return false
	case LockSoft:
		return confirmed
	default:
		return true
	}
}
