package domain

import (
	"context"
	"time"
)

// The two things in this file are caches of read-time derivations, and both
// carry the same warning: the row is not the fact. Rapport is a reading of the
// operation log; a rhythm is a reading of when someone was awake. Either row
// can be deleted and rebuilt, and if a row and the evidence behind it ever
// disagree, the evidence wins.
//
// They are stored at all because the derivation is not free — replaying a
// year's ledger on every card the agent considers is not a thing to do — and
// because being able to resume from a cursor is what makes an append-only log
// affordable to read.

// ── rapport ─────────────────────────────────────────────────────────────────

// RapportState is a session's cached rapport scores plus the point in the
// operation log they were folded up to.
//
// Scores is stored as JSON rather than as four pairs of typed columns. The
// domain list is closed today, but a column per domain would make adding one a
// three-dialect migration, and there is no query that reads rapport across
// sessions — nothing would benefit from the columns being queryable. The
// precedent is day_plans.blocks.
type RapportState struct {
	SessionID string `json:"sessionId"`
	// Scores maps a domain to {value, evidence}. The rapport package owns the
	// shape; this layer only carries it, so that internal/rapport stays free of
	// storage and internal/domain stays free of rapport.
	Scores map[string]RapportScore `json:"scores"`
	// Cursor is how far into the ledger this reading goes. A catch-up scans
	// from here rather than from the beginning — see OperationLogRepository.Scan.
	Cursor OpLogCursor `json:"cursor"`
	// FoldVersion is the version of the folding rules the row was produced by.
	// When the deltas or the domain list change, an old row is not wrong so
	// much as meaningless, and comparing this against the current version is
	// how a deployment knows to throw it away and re-fold rather than carry a
	// number nobody can explain.
	FoldVersion int       `json:"foldVersion"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// RapportScore is one domain's cached standing. It mirrors rapport.Score; the
// duplication is the price of keeping the dependency pointing one way.
type RapportScore struct {
	Value    float64 `json:"value"`
	Evidence int     `json:"evidence"`
}

type RapportRepository interface {
	// Get returns the cached reading, or ErrNotFound when there is none — which
	// is not an error condition, just an instruction to fold from the start.
	Get(ctx context.Context, sessionID string) (*RapportState, error)
	Save(ctx context.Context, s *RapportState) error
	// Reset drops the cache so the next read re-folds the whole ledger. Used
	// when the fold version moves and when a user clears their history.
	Reset(ctx context.Context, sessionID string) error
}

// ── rhythm ──────────────────────────────────────────────────────────────────

// RhythmProfile is a session's learned or pinned rhythm, plus the two marks
// that track being awake right now.
//
// Wake/Sleep/Source/Days mirror rhythm.Profile. RunSince/LastSignalAt mirror
// rhythm.Live and are updated on every signal, which is why they live on this
// row rather than being derived: deriving them would mean keeping every signal.
type RhythmProfile struct {
	SessionID string `json:"sessionId"`
	Wake      string `json:"wake"`  // local "HH:MM"
	Sleep     string `json:"sleep"` // local "HH:MM"; may be past midnight
	Source    string `json:"source"`
	Days      int    `json:"days"`

	// RunSince and LastSignalAt are the O(1) continuous-activity state the
	// Protector reads. Zero means "no current stretch", which reads as asleep.
	RunSince     time.Time `json:"runSince,omitempty"`
	LastSignalAt time.Time `json:"lastSignalAt,omitempty"`

	UpdatedAt time.Time `json:"updatedAt"`
}

// RhythmDay is one rhythm day's bounds — one row per person per day, not one
// per signal. A foreground heartbeat once a minute would be roughly 1,400 rows
// a day for a derivation whose entire output is four numbers; the first and
// last awake moment is everything the median actually reads.
//
// Minutes are measured from that day's cut, not from midnight, so a bedtime of
// 02:00 sorts after one of 23:00. Sorting them as wall-clock minutes would put
// a night owl's median bedtime at lunchtime.
type RhythmDay struct {
	SessionID string `json:"sessionId"`
	Day       string `json:"day"` // YYYY-MM-DD of the day it STARTED
	FirstMin  int    `json:"firstMin"`
	LastMin   int    `json:"lastMin"`
	Signals   int    `json:"signals"`
}

type RhythmRepository interface {
	Get(ctx context.Context, sessionID string) (*RhythmProfile, error)
	// Save writes the LEARNED half — wake, sleep, source, days. It deliberately
	// does not touch the two live marks.
	//
	// The row has two owners on wildly different cadences: the nightly learn job
	// writes the profile, and every awake signal moves the marks. With one
	// whole-row Save, the learn job's Get→compute→Save would carry a snapshot of
	// the marks from before it started and write it back minutes later, erasing a
	// stretch that began in between — and a zero RunSince reads as "asleep", so
	// the Protector would forget somebody had been up for nine hours. Splitting
	// the writers is cheaper and more obviously correct than versioning the row.
	Save(ctx context.Context, p *RhythmProfile) error
	// Touch writes only the two live marks, and only forward: a signal older
	// than the one already recorded is a retry or a clock that stepped back, and
	// letting it move the marks would shorten a stretch that in fact continued.
	//
	// This is the every-signal path, so it is one statement.
	Touch(ctx context.Context, sessionID string, runSince, lastSignalAt time.Time) error
	// Observe folds one awake moment into the day's row: widening its bounds if
	// it already exists, creating it if not. Called on every signal, so it has
	// to be one statement's worth of work.
	Observe(ctx context.Context, sessionID, day string, minute int) error
	// Days returns the rows learning reads, newest day first, bounded by limit.
	Days(ctx context.Context, sessionID string, limit int) ([]RhythmDay, error)
	// PruneDays drops rows for days before the given key, which is what bounds
	// this table — the learning window is three weeks and nothing reads past it.
	PruneDays(ctx context.Context, sessionID, before string) (int, error)
}

// ── locale overrides ────────────────────────────────────────────────────────

// LocaleOverride is one translated string, edited from the console. It is the
// database layer of the message catalog (i18n.Catalog): database over files
// over the strings compiled into the binary.
//
// Unlike almost everything else here it is NOT per session. A translation is an
// operator-level fact about the installation, the same way a prompt override
// is.
type LocaleOverride struct {
	Key       string    `json:"key"`
	Locale    string    `json:"locale"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type LocaleRepository interface {
	// All returns every override, for handing to i18n.Catalog.SetOverrides —
	// which replaces the layer wholesale, so a partial read would silently
	// delete translations.
	All(ctx context.Context) ([]LocaleOverride, error)
	Set(ctx context.Context, key, locale, content string) error
	Delete(ctx context.Context, key, locale string) error
	// DeleteLocale removes a whole language — the console's "uninstall this
	// translation" button.
	DeleteLocale(ctx context.Context, locale string) (int, error)
}
