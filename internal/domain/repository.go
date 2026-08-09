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

	// Settings is the runtime half of the configuration layering: the boot half
	// stays in the environment because it built something already. See
	// setting.go and internal/config/layer.go.
	Settings() SettingRepository

	// ProviderOverrides is the console-editable half of an external capability
	// source. The other half lives in config/providers.yaml and is boot-layer —
	// see provider.go for the line between them and why the console never
	// writes the file.
	ProviderOverrides() ProviderOverrideRepository

	// Attachments is the ownership half of the file bus: internal/blob maps refs
	// to bytes and knows nothing about sessions, so these rows are what makes a
	// ref safe to resolve. See attachment.go.
	Attachments() AttachmentRepository

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
	Add(ctx context.Context, sessionID, theme string) error
}

// OpLogCursor marks a position in the append-only log. The id breaks ties:
// created_at is stored to the millisecond and two operations in the same
// millisecond are ordinary, so ordering on time alone would silently skip or
// repeat rows across pages.
//
// The zero value means "from the beginning".
type OpLogCursor struct {
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
	Scan(ctx context.Context, sessionID string, after OpLogCursor, limit int) ([]OperationLog, error)
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

type AICallLogRepository interface {
	Add(ctx context.Context, l *AICallLog) error
	Stats(ctx context.Context) (*AdminStats, error)
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
	Get(ctx context.Context, sessionID, id string) (*CustomTheme, error)
	List(ctx context.Context, sessionID string) ([]CustomTheme, error)
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
