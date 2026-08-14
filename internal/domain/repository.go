package domain

import (
	"context"
	"time"
)

type Store interface {
	Sessions() SessionRepository
	DayPlans() DayPlanRepository
	Moods() MoodRepository
	Companion() CompanionRepository
	ThemeLog() ThemeLogRepository
	OpLogs() OperationLogRepository
	Users() UserRepository
	Auth() AuthRepository
	Prompts() PromptRepository
	Chats() ChatRepository
	AILogs() AICallLogRepository
	Rules() RuleRepository
	Courses() CourseRepository
	Feedback() FeedbackLogRepository
	Assignments() AssignmentRepository
	Themes() ThemeRepository
	Memory() MemoryRepository
	Materials() MaterialRepository
	Wishes() WishRepository
	WeeklyLetters() WeeklyLetterRepository
	River() RiverRepository
	TempContexts() TempContextRepository

	ChannelBindings() ChannelBindingRepository

	// Added with the multi-instance work (batch C). Proposals replaces the
	// process-local decision-card map; Leases and JobRuns make two servers safe
	// to run at once; Rapport and Rhythm cache read-time derivations; Locales is
	// the database layer of the message catalog.
	Proposals() ProposalRepository
	Leases() LeaseRepository
	JobRuns() JobRunRepository
	Rapport() RapportRepository
	Rhythm() RhythmRepository
	Locales() LocaleRepository

	// ThemeKinds is the third tier of the theme kind system: a validation rule
	// that arrived as data. See theme_kind.go for what approval gates and, more
	// importantly, what it does not.
	ThemeKinds() ThemeKindRepository

	// Settings is the runtime half of the configuration layering: the boot half
	// stays in the environment because it built something already. See
	// setting.go and internal/config/layer.go.
	Settings() SettingRepository

	// ProviderOverrides is the console-editable half of an external capability
	// source. The other half lives in config/providers.yaml and is boot-layer —
	// see provider.go for the line between them and why the console never
	// writes the file.
	ProviderOverrides() ProviderOverrideRepository

	// Roles is the console permission model: named permission sets, and who is
	// in them. A role with no permissions is a plain user group — which is what
	// a commercial tier is. See role.go.
	Roles() RoleRepository

	// Attachments is the ownership half of the file bus: internal/blob maps refs
	// to bytes and knows nothing about sessions, so these rows are what makes a
	// ref safe to resolve. See attachment.go.
	Attachments() AttachmentRepository

	// Frontends is the two-layer frontend identity: families own theme token
	// spaces, builds are sightings of one. See frontend.go.
	Frontends() FrontendRepository

	// Pairings is how an external console attaches to this deployment — a
	// cluster manager, or somebody else's console. It resolves permissions
	// through the SAME roles a person does; see pairing.go for why the backend
	// issues the key rather than the console presenting one.
	Pairings() PairingRepository

	// Browser is the operations console's window onto the raw tables. It models
	// no domain concept and deliberately exposes the schema — see tables.go for
	// the catalogue it is driven by and the rule that keeps a request-supplied
	// name out of a query.
	Browser() Browser

	// PurgeSession deletes one session and every row that session owns across
	// the schema — plans, rules, moods, logs, proposals, wishes, materials,
	// memory, imports, themes, courses, assignments, chat, companion memory,
	// rhythm, attachments, temp contexts, and the session row itself. It is the
	// cascade behind DELETE /api/admin/users/{id}.
	//
	// Best-effort per entity: one failing table does not stop the rest, because
	// a half-deleted session is recoverable and a silently-stopped one is not.
	// It returns how many session rows it removed (0 or 1) and the first error.
	PurgeSession(ctx context.Context, sessionID string) (int, error)

	Migrate(ctx context.Context) error
	Ping(ctx context.Context) error
	Close() error
}

type SessionRepository interface {
	GetOrCreate(ctx context.Context, id string) (*Session, error)
	Get(ctx context.Context, id string) (*Session, error)
	Update(ctx context.Context, id string, upd SessionUpdate) (*Session, error)
	IncrementInteraction(ctx context.Context, id string) error
	GetByImportToken(ctx context.Context, token string) (*Session, error)
	// AddUsage folds one AI call into this session's three counters, rolling a
	// window over when it has expired.
	//
	// ⚠️ ONE statement, with the reset expressed as arithmetic (CASE), because
	// two instances handling two calls for the same account at once is ordinary.
	// A read-modify-write here would lose calls silently, and the number nobody
	// checks is exactly the number a quota is built on.
	//
	// `now` is passed rather than read from the clock so that the window
	// rollover is testable without sleeping three hours — the same reason
	// Leases().Acquire takes it.
	//
	// Best-effort at the call site: a lost counter update must never fail the
	// AI call it describes. See internal/server/ailog.go.
	AddUsage(ctx context.Context, sessionID string, promptTokens, compTokens int, now time.Time) error
}

type DayPlanRepository interface {
	Get(ctx context.Context, sessionID, date string) (*DayPlan, error)
	Upsert(ctx context.Context, plan *DayPlan) (*DayPlan, error)
	Range(ctx context.Context, sessionID, from, to string) ([]DayPlan, error)
}

type MoodRepository interface {
	List(ctx context.Context, sessionID string, limit int) ([]MoodCheckin, error)
	Create(ctx context.Context, m *MoodCheckin) (*MoodCheckin, error)
	MarkExerciseCompleted(ctx context.Context, sessionID, id string) error
	// Delete removes one check-in. It exists for undoing agent-recorded
	// check-ins (revert of mood_record); user check-ins are never deleted
	// through this path today.
	Delete(ctx context.Context, sessionID, id string) error
}

type CompanionRepository interface {
	Get(ctx context.Context, sessionID string) (*CompanionMemory, error)
	Upsert(ctx context.Context, sessionID string, history []Message, keyFacts []string) error
}

type ThemeLogRepository interface {
	// Add records one switch. familyID says which frontend it happened on —
	// see ThemeSwitch.FamilyID.
	Add(ctx context.Context, sessionID, theme, familyID string) error
}

// LogCursor marks a position in an append-only log. The id breaks ties:
// created_at is stored to the millisecond and two rows in the same millisecond
// are ordinary, so ordering on time alone would silently skip or repeat rows
// across pages.
//
// Two logs use it — operation_logs (forward replay, see oplog_cursor.go for the
// visibility-lag machinery that only the forward direction needs) and
// ai_call_logs (backward paging for the console). It was named OpLogCursor
// until the second one arrived; the type was always generic and only the
// safety rules around forward replay are specific to the ledger.
//
// The zero value means "from the beginning" going forward, and "from now"
// going backward.
type LogCursor struct {
	CreatedAt time.Time
	ID        string
}

type OperationLogRepository interface {
	Add(ctx context.Context, l *OperationLog) error
	Get(ctx context.Context, sessionID, id string) (*OperationLog, error)
	// List returns the newest entries first — the ledger view.
	List(ctx context.Context, sessionID string, limit int) ([]OperationLog, error)
	// Scan walks the log oldest-first from a cursor. This is the replay path:
	// rapport is defined as a reading of the ledger rather than a fact of its
	// own, so its cache must be rebuildable by re-folding every operation in
	// the order it happened. List cannot do that — it is newest-first and
	// capped.
	Scan(ctx context.Context, sessionID string, after LogCursor, limit int) ([]OperationLog, error)
	// RevertedBy returns the revert entry that already undid targetID, or
	// ErrNotFound when nothing has.
	//
	// The check used to be a scan of the most recent 200 entries, which meant a
	// session busy enough to push a revert past the 200th row could undo the
	// same operation twice — applying a compensation twice is not idempotent
	// (adding a block back twice adds two), so the window was a real hole and
	// not a theoretical one.
	RevertedBy(ctx context.Context, sessionID, targetID string) (*OperationLog, error)
}

type UserRepository interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Upsert(ctx context.Context, u *User) (*User, error)
	SetDataSession(ctx context.Context, userID, sessionID string) error
	// IncrementTokenVersion bumps the user's token version, invalidating every
	// previously issued JWT (used on logout).
	IncrementTokenVersion(ctx context.Context, userID string) error
	// SetOwner marks or unmarks the super-administrator flag.
	//
	// ⚠️ A NARROW setter, and it must stay one. Upsert takes a whole *User and
	// the OAuth callback builds one out of what the provider returned — so the
	// moment is_owner appears in Upsert's column list, logging in becomes a
	// privilege escalation write. The same arrangement already protects
	// TokenVersion and DataSessionID; unlike those, this one has a conformance
	// case pinning it across all four back ends.
	SetOwner(ctx context.Context, userID string, owner bool) error
	// List returns users for the console, newest first, capped.
	//
	// It exists because "who can export the database" has to be answerable, and
	// every safety rule in the permission model — refusing to remove the last
	// owner, showing how many people a role edit affects — assumes the set can
	// be enumerated.
	List(ctx context.Context, limit int) ([]User, error)
	// Delete removes a user and every account-scoped row they own — their
	// credential, OAuth identities, and role memberships. Those rows are not
	// session data, so PurgeSession does not cover them; leaving them behind
	// would let a re-sign-in collide on oauth_identities' unique provider key or
	// silently restore a deleted permission via a stray membership row.
	Delete(ctx context.Context, userID string) error
}

type AuthRepository interface {
	GetCredentialByUserID(ctx context.Context, userID string) (*Credential, error)
	UpsertCredential(ctx context.Context, c *Credential) error
	GetOAuthIdentity(ctx context.Context, provider, providerUserID string) (*OAuthIdentity, error)
	CreateOAuthIdentity(ctx context.Context, oi *OAuthIdentity) error
}

type PromptRepository interface {
	Get(ctx context.Context, key, locale string) (*Prompt, error)
	Set(ctx context.Context, key, locale, content string) error
	List(ctx context.Context) ([]Prompt, error)
}

type ChatRepository interface {
	ListThreads(ctx context.Context, sessionID string) ([]ChatThread, error)
	CreateThread(ctx context.Context, t *ChatThread) (*ChatThread, error)
	UpdateThread(ctx context.Context, sessionID, id string, upd ChatThreadUpdate) (*ChatThread, error)
	DeleteThread(ctx context.Context, sessionID, id string) error
	ListMessages(ctx context.Context, threadID, sessionID, before string, limit int) ([]ChatMessage, error)
	AppendMessages(ctx context.Context, msgs []ChatMessage) error
	GetMessage(ctx context.Context, sessionID, id string) (*ChatMessage, error)
	UpdateMessage(ctx context.Context, sessionID, id string, upd ChatMessageUpdate) error
	// FailPendingMessages marks every pending message as error — the startup
	// sweep for async placeholders orphaned by a crash.
	FailPendingMessages(ctx context.Context) (int64, error)
	DeleteThreadMessages(ctx context.Context, sessionID, threadID string) error
}

type ChatThreadUpdate struct {
	Title    *string
	Summary  *string
	Archived *bool
}

// ChatMessageUpdate is a partial message update (async turn finalization).
type ChatMessageUpdate struct {
	Content    *string
	ToolEvents *string
	Status     *string
}

// AILogFilter narrows the AI ledger for the console.
//
// Every field is optional and the zero value means "do not narrow on this".
// They combine with AND, which is the only combination an operations screen
// ever wants: the question is always "the failures, on this model, since the
// deploy" and never a disjunction.
type AILogFilter struct {
	SessionID string
	// Endpoint is one of the ep* constants in internal/server/ailog.go.
	Endpoint string
	Model    string
	// Status is "ok" or "error". Empty means both.
	Status string
	// Since bounds the window below. Zero means no lower bound.
	Since time.Time
	// Before pages BACKWARD: only rows strictly older than this point.
	//
	// ⚠️ Backward paging can miss a row that is written while somebody is
	// paging, because created_at is stamped in Go before the row is written and
	// there are no transactions — so a row can land with a timestamp inside a
	// page that was already fetched. See oplog_cursor.go, which measured this at
	// about eight per cent under eight concurrent writers.
	//
	// It is acceptable HERE and not there, and the difference is worth stating:
	// there, a forward replay advances its cursor and never looks back, so a
	// skipped row is skipped permanently and a cache built from it becomes
	// authoritative over the ledger. Here the reader is a person scrolling into
	// the past, and reloading the screen starts from now again — so the worst
	// case is "a call made during your scroll may not appear until you refresh",
	// which is how every list in every console behaves.
	Before LogCursor
}

type AICallLogRepository interface {
	Add(ctx context.Context, l *AICallLog) error
	Stats(ctx context.Context) (*AdminStats, error)
	// List returns matching calls newest first, capped by ListLimit.
	List(ctx context.Context, f AILogFilter, limit int) ([]AICallLog, error)
	// Prune deletes calls older than before and returns how many went.
	//
	// This table gets one row per model call forever and nothing else deletes
	// from it — on a deployment doing a few thousand calls a day it is the
	// fastest-growing table in the database. Retention and the leader gate live
	// in internal/server/leader.go beside the job_runs prune, which is the same
	// shape for the same reason.
	//
	// ⚠️ RollUpUsage must run first. See ai_usage.go: the prune deletes the rows
	// the rollup is computed from.
	Prune(ctx context.Context, before time.Time) (int64, error)

	// RollUpUsage folds every closed day the ledger still holds and the rollup
	// does not, and reports how many days it wrote.
	//
	// One server-side aggregate per day, no per-call bookkeeping, idempotent by
	// construction — the whole argument is in ai_usage.go. `today` is the day
	// key that is NOT closed yet; everything strictly before it is fair game.
	RollUpUsage(ctx context.Context, today string) (int, error)

	// UsageTotals sums the rollup over [from, to] inclusive. Empty bounds mean
	// unbounded, so UsageTotals(ctx, "", "") is the deployment's whole history.
	UsageTotals(ctx context.Context, from, to string) (*AIUsageTotals, error)
	// UsageDays returns per-day totals, newest day first, capped.
	UsageDays(ctx context.Context, from, to string, limit int) ([]AIUsageDay, error)
	// UsageByModel returns per-model totals over the window, biggest first.
	//
	// The question it answers is "what is costing money", which a per-day series
	// cannot: a spend that doubled is only actionable once you know which model
	// did it. Day is empty on these rows — they are a fold ACROSS days.
	UsageByModel(ctx context.Context, from, to string) ([]AIUsageDay, error)
}

type RuleRepository interface {
	Get(ctx context.Context, sessionID, id string) (*ScheduleRule, error)
	List(ctx context.Context, sessionID string) ([]ScheduleRule, error)
	Create(ctx context.Context, r *ScheduleRule) (*ScheduleRule, error)
	Update(ctx context.Context, sessionID, id string, upd ScheduleRuleUpdate) (*ScheduleRule, error)
	Delete(ctx context.Context, sessionID, id string) error
}

type CourseRepository interface {
	UpsertByCanvasID(ctx context.Context, c *Course) (*Course, error)
	List(ctx context.Context, sessionID string) ([]Course, error)
}

type AssignmentFilter struct {
	DueFrom *time.Time
	DueTo   *time.Time
	Status  string
}

type MemoryRepository interface {
	ListFacts(ctx context.Context, sessionID string) ([]MemoryFact, error)
	AddFact(ctx context.Context, f *MemoryFact) (*MemoryFact, error)
	DeleteFact(ctx context.Context, sessionID, id string) error
	ClearFacts(ctx context.Context, sessionID string) (int, error)
	AddImport(ctx context.Context, rec *ImportRecord) (*ImportRecord, error)
	ListImports(ctx context.Context, sessionID string, limit int) ([]ImportRecord, error)
}

type ThemeRepository interface {
	// Get does NOT take a family: an id is an id, and a theme fetched by id is
	// one the caller already had a reference to. Scoping it would only turn
	// "you sent the wrong header" into "your theme disappeared".
	Get(ctx context.Context, sessionID, id string) (*CustomTheme, error)
	// List is scoped to one family — see CustomTheme.FamilyID for why a theme
	// belongs to a token space and not to a session.
	List(ctx context.Context, sessionID, familyID string) ([]CustomTheme, error)
	// ListAcrossFamilies exists for ONE caller: merging an anonymous session
	// into a signed-in one.
	//
	// ⚠️ A separate method rather than List(sid, "") on purpose. Empty means
	// "the fallback family" everywhere else in this interface, and a single
	// spelling that means "the default" on write and "everything" on read is
	// how somebody eventually deletes a family's themes while meaning to list
	// them. The merge must carry every family's themes across or signing in
	// loses whatever the other device made.
	ListAcrossFamilies(ctx context.Context, sessionID string) ([]CustomTheme, error)
	// ScanFamily walks every theme in one family ACROSS EVERY SESSION, ordered
	// by id, starting after the given id. It is the backfill's only reader.
	//
	// ⚠️ Deployment-wide and therefore not session-scoped, which is why it is a
	// separate method rather than a flag on List: every other read here is a
	// user reading their own themes, and one method that is sometimes scoped and
	// sometimes not is one refactor away from being never scoped.
	//
	// Keyset paging on id rather than OFFSET: the sweep writes to the rows it is
	// walking, and OFFSET over a table being written to skips and repeats.
	ScanFamily(ctx context.Context, familyID, afterID string, limit int) ([]CustomTheme, error)
	Create(ctx context.Context, t *CustomTheme) (*CustomTheme, error)
	Update(ctx context.Context, sessionID, id string, upd CustomThemeUpdate) (*CustomTheme, error)
	Delete(ctx context.Context, sessionID, id string) error
}

type AssignmentRepository interface {
	Get(ctx context.Context, sessionID, id string) (*Assignment, error)
	List(ctx context.Context, sessionID string, f AssignmentFilter) ([]Assignment, error)
	UpsertByCanvasID(ctx context.Context, a *Assignment) (*Assignment, error)
	SetStatus(ctx context.Context, sessionID, id, status string) error
	// Delete removes one assignment. It exists for undo: reverting an
	// agent-created assignment means the row goes away, not that it gets
	// dismissed — dismissal is a user-visible workflow state, not an erasure.
	Delete(ctx context.Context, sessionID, id string) error
	// SetReminders silences or restores the deadline ladder for one item —
	// STRATEGY §1.3's "the fact track can only be turned off one item at a
	// time". Separate from SetStatus: status is the planner workflow, this is
	// whether the fact track may speak.
	SetReminders(ctx context.Context, sessionID, id string, on bool) error
}
