// Package sqlstore implements domain.Store for SQLite, PostgreSQL, and MySQL
// from a single codebase, differing only via the Dialect. Timestamps are stored
// as Unix-millis BIGINT and booleans as 0/1 to stay fully portable across the
// three engines (no driver-specific datetime quirks). Registered as "sqlite",
// "postgres", and "mysql".
package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"daycore/internal/domain"
	"daycore/internal/storage"

	_ "github.com/go-sql-driver/mysql" // driver "mysql"
	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx"
	_ "modernc.org/sqlite"             // driver "sqlite" (pure Go, no CGO)
)

func init() {
	storage.Register("sqlite", func(dsn string) (domain.Store, error) { return Open(sqliteDialect{}, dsn) })
	storage.Register("postgres", func(dsn string) (domain.Store, error) { return Open(postgresDialect{}, dsn) })
	storage.Register("mysql", func(dsn string) (domain.Store, error) { return Open(mysqlDialect{}, dsn) })
}

// Store is the SQL-backed domain.Store.
type Store struct {
	db *sql.DB
	d  Dialect

	// condApplied records which named conditional migrations are present
	// (pre-existing or just created). Optional features (native FTS) key off
	// it instead of failing startup.
	condApplied map[string]bool
	// warnings collects non-fatal migration problems; main.go logs them.
	warnings []string
}

// Open connects using the dialect's driver and tunes the connection pool.
func Open(d Dialect, dsn string) (*Store, error) {
	db, err := sql.Open(d.DriverName(), d.NormalizeDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", d.Name(), err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s: %w", d.Name(), err)
	}
	return &Store{db: db, d: d, condApplied: map[string]bool{}}, nil
}

// Migrate runs the dialect's idempotent DDL, then adds any columns that older
// installs are missing (ALTER TABLE … ADD COLUMN guarded by an existence check,
// since the ALTER itself is not idempotent on SQLite/MySQL).
func (s *Store) Migrate(ctx context.Context) error {
	// Serialise against another instance booting at the same instant. The whole
	// point of the batch C tables is that two instances can run, so two of them
	// reaching Migrate together is ordinary rather than exceptional.
	//
	// The lock must be held on ONE connection for the duration, so this takes a
	// connection out of the pool and runs everything on it. Acquiring on a
	// pooled *sql.DB would let the pool hand the next statement to a different
	// connection, which for a session-level advisory lock means the lock is held
	// by a connection nobody is using.
	acquire, release := s.d.MigrationLock()
	if len(acquire) > 0 {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			return fmt.Errorf("migrate lock (%s): %w", s.d.Name(), err)
		}
		defer conn.Close()
		for _, stmt := range acquire {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migrate lock (%s): %w", s.d.Name(), err)
			}
		}
		defer func() {
			for _, stmt := range release {
				// A failed unlock is not worth failing the boot over: the lock
				// is session-level and dies with the connection we are closing.
				_, _ = conn.ExecContext(ctx, stmt)
			}
		}()
	}
	for _, stmt := range s.d.Migrations() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate (%s): %w", s.d.Name(), err)
		}
	}
	for _, cm := range s.d.ColumnMigrations() {
		var one int
		err := s.db.QueryRowContext(ctx, s.d.Rebind(s.d.ColumnExistsQuery()), cm.Table, cm.Column).Scan(&one)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := s.db.ExecContext(ctx, cm.DDL); err != nil {
				return fmt.Errorf("migrate add column %s.%s (%s): %w", cm.Table, cm.Column, s.d.Name(), err)
			}
		case err != nil:
			return fmt.Errorf("migrate column check %s.%s (%s): %w", cm.Table, cm.Column, s.d.Name(), err)
		}
	}
	// Conditional (best-effort) migrations: optional features like the native
	// FTS index. A failure is a warning, never a startup error.
	for _, cm := range s.d.ConditionalMigrations() {
		var one int
		err := s.db.QueryRowContext(ctx, cm.CheckQuery).Scan(&one)
		switch {
		case err == nil:
			s.condApplied[cm.Name] = true // already present
		case errors.Is(err, sql.ErrNoRows):
			applied := true
			for _, ddl := range cm.DDLs {
				if _, err := s.db.ExecContext(ctx, ddl); err != nil {
					s.warnings = append(s.warnings,
						fmt.Sprintf("conditional migration %q (%s) skipped: %v — feature stays off, substring search fallback remains", cm.Name, s.d.Name(), err))
					applied = false
					break
				}
			}
			s.condApplied[cm.Name] = applied
		default:
			s.warnings = append(s.warnings,
				fmt.Sprintf("conditional migration %q (%s) check failed: %v", cm.Name, s.d.Name(), err))
		}
	}
	return nil
}

// MigrationWarnings returns non-fatal problems from the last Migrate (main.go
// logs them at startup).
func (s *Store) MigrationWarnings() []string { return s.warnings }

// Ping verifies connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Close closes the pool.
func (s *Store) Close() error { return s.db.Close() }

// Repository accessors.
func (s *Store) Sessions() domain.SessionRepository               { return sessionRepo{s} }
func (s *Store) DayPlans() domain.DayPlanRepository               { return dayPlanRepo{s} }
func (s *Store) Moods() domain.MoodRepository                     { return moodRepo{s} }
func (s *Store) Companion() domain.CompanionRepository            { return companionRepo{s} }
func (s *Store) ThemeLog() domain.ThemeLogRepository              { return themeLogRepo{s} }
func (s *Store) Chats() domain.ChatRepository                     { return chatRepo{s} }
func (s *Store) AILogs() domain.AICallLogRepository               { return aiLogRepo{s} }
func (s *Store) OpLogs() domain.OperationLogRepository            { return opLogRepo{s} }
func (s *Store) Users() domain.UserRepository                     { return userRepo{s} }
func (s *Store) Auth() domain.AuthRepository                      { return authRepo{s} }
func (s *Store) Prompts() domain.PromptRepository                 { return promptRepo{s} }
func (s *Store) Rules() domain.RuleRepository                     { return ruleRepo{s} }
func (s *Store) Courses() domain.CourseRepository                 { return courseRepo{s} }
func (s *Store) Assignments() domain.AssignmentRepository         { return assignmentRepo{s} }
func (s *Store) Themes() domain.ThemeRepository                   { return themeRepo{s} }
func (s *Store) Memory() domain.MemoryRepository                  { return memoryRepo{s} }
func (s *Store) Materials() domain.MaterialRepository             { return materialRepo{s} }
func (s *Store) ChannelBindings() domain.ChannelBindingRepository { return channelBindingRepo{s} }
func (s *Store) Proposals() domain.ProposalRepository             { return proposalRepo{s} }
func (s *Store) Leases() domain.LeaseRepository                   { return leaseRepo{s} }
func (s *Store) JobRuns() domain.JobRunRepository                 { return jobRunRepo{s} }
func (s *Store) Rapport() domain.RapportRepository                { return rapportRepo{s} }
func (s *Store) Rhythm() domain.RhythmRepository                  { return rhythmRepo{s} }
func (s *Store) Locales() domain.LocaleRepository                 { return localeRepo{s} }
func (s *Store) Attachments() domain.AttachmentRepository         { return attachmentRepo{s} }
func (s *Store) Settings() domain.SettingRepository               { return settingRepo{s} }
func (s *Store) Roles() domain.RoleRepository                     { return roleRepo{s} }

// Browser is the console's table window. It is NOT a repository — see
// domain/tables.go for why it sits beside them and why nothing outside the
// admin handlers may call it.
func (s *Store) Browser() domain.Browser { return browserRepo{s} }
func (s *Store) ProviderOverrides() domain.ProviderOverrideRepository {
	return providerOverrideRepo{s}
}
func (s *Store) Wishes() domain.WishRepository              { return wishRepo{s} }
func (s *Store) Feedback() domain.FeedbackLogRepository     { return feedbackRepo{s} }
func (s *Store) TempContexts() domain.TempContextRepository { return tempContextRepo{s} }

// ─── low-level helpers (placeholder rebinding) ──────────────────────────────

func (s *Store) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, s.d.Rebind(query), args...)
}

func (s *Store) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, s.d.Rebind(query), args...)
}

func (s *Store) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, s.d.Rebind(query), args...)
}

// ─── value conversions (portable timestamps/bools/nullable/JSON) ────────────

func nowMillis() int64              { return time.Now().UnixMilli() }
func toMillis(t time.Time) int64    { return t.UnixMilli() }
func fromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullString returns *string as a driver arg (nil → SQL NULL).
// nullMillis and ptrMillis are the two halves of a nullable timestamp. Before
// them every caller open-coded the same `var x any` / `if v.Valid { t := ... }`
// dance, which is how assignments.go ended up with three copies of it.
func nullMillis(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return toMillis(*t)
}

func ptrMillis(ms sql.NullInt64) *time.Time {
	if !ms.Valid {
		return nil
	}
	t := fromMillis(ms.Int64)
	return &t
}

func nullString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// ptrString converts a scanned NullString to *string.
func ptrString(ns sql.NullString) *string {
	if ns.Valid {
		v := ns.String
		return &v
	}
	return nil
}

// marshalStrict is marshalJSON without the swallow. Use it where losing the
// value would be worse than failing the write.
//
// marshalJSON's "null" fallback is fine for a display field, but a Proposal
// whose ops will not marshal — a NaN anywhere in a tool argument is enough —
// would be stored claiming it has work to do and holding none. The card then
// looks live, the user accepts it, and nothing happens.
func marshalStrict(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}
