package sqlstore

import (
	"strings"
)

// Dialect abstracts the differences between SQL engines: how bind placeholders
// are written and what the schema DDL looks like. A new SQL engine = add a
// Dialect implementation + register it. Queries in this package are written with
// `?` placeholders and passed through Rebind.
type Dialect interface {
	Name() string
	DriverName() string
	// Rebind converts `?`-placeholder SQL into this engine's placeholder style.
	Rebind(query string) string
	// MigrationLock returns statements that serialise Migrate against another
	// instance running it at the same moment, and the statements to undo that.
	// Both may be empty for an engine that does not need it.
	//
	// This exists because IF NOT EXISTS is not a concurrency primitive. On
	// Postgres two processes can both pass the catalog check for the same
	// CREATE TABLE and one then loses the insert into pg_type with a duplicate
	// key — and running two instances is precisely what the batch C tables are
	// for, so "both boot at once" is the normal case, not the edge one.
	MigrationLock() (acquire, release []string)
	// NormalizeDSN lets a dialect enforce connection settings the code depends on,
	// rather than documenting them and hoping the operator copied the whole
	// string. MySQL needs one; the others return the DSN unchanged.
	NormalizeDSN(dsn string) string
	// Quote wraps an identifier so a column whose name is a reserved word still
	// parses. Rebind only rewrites placeholders, so a bare `key` in a query is
	// a syntax error on MySQL no matter how the DDL spelled it — which is
	// exactly the bug temp_contexts shipped with.
	//
	// New tables should not need this: pick a column name that is not a
	// keyword anywhere. It exists for the one table that already did.
	Quote(ident string) string
	// Migrations returns idempotent DDL statements (CREATE TABLE IF NOT EXISTS …).
	Migrations() []string
	// ColumnMigrations returns ALTER TABLE statements that Migrate runs only when
	// the column is still missing (ALTER … ADD COLUMN is not idempotent itself).
	ColumnMigrations() []ColumnMigration
	// ColumnExistsQuery returns a `?`-placeholder query taking (table, column)
	// that yields a row iff the column exists.
	ColumnExistsQuery() string
	// ConditionalMigrations returns best-effort DDL groups for optional
	// features (native FTS). Unlike Migrations/ColumnMigrations, a failure here
	// is recorded as a warning, never a startup error — the feature keying off
	// the migration (Store.condApplied[name]) simply stays off.
	ConditionalMigrations() []ConditionalMigration
}

// ColumnMigration describes one column added to an existing table.
type ColumnMigration struct {
	Table  string
	Column string
	DDL    string
}

// ConditionalMigration is a named group of non-idempotent DDL statements run
// only when CheckQuery (no args) returns no rows. The statements run in order;
// the first failure abandons the group with a warning.
type ConditionalMigration struct {
	Name       string
	CheckQuery string
	DDLs       []string
}

// legacyPromptCopy backfills prompt_overrides from the pre-i18n prompts table.
const legacyPromptCopy = `INSERT INTO prompt_overrides (prompt_key, locale, content, updated_at)
	SELECT prompt_key, 'zh-CN', content, updated_at FROM prompts
	WHERE prompt_key NOT IN (SELECT prompt_key FROM prompt_overrides WHERE locale = 'zh-CN')`

// sessionColumnMigrations builds the shared sessions-table column additions;
// only the column type token differs per engine.
// sessionColumnMigrations is the shared ALTER list. It takes the dialect's own
// spelling for the types it needs, because the DDL a fresh database gets from
// CREATE TABLE and the DDL an existing one gets from ALTER must produce the SAME
// column — dialect_parity_test.go asserts exactly that, and it is the test that
// caught reminders_off being TINYINT(1) in one and INTEGER in the other.
func sessionColumnMigrations(textType, boolType string) []ColumnMigration {
	migs := []ColumnMigration{
		{Table: "sessions", Column: "language",
			DDL: `ALTER TABLE sessions ADD COLUMN language ` + textType + ` NOT NULL DEFAULT ''`},
		{Table: "sessions", Column: "import_token",
			DDL: `ALTER TABLE sessions ADD COLUMN import_token ` + textType + ` NOT NULL DEFAULT ''`},
		// Both are TEXT and therefore NULLABLE WITH NO DEFAULT — MySQL rejects a
		// literal DEFAULT on TEXT/BLOB, and this list is shared by all three
		// dialects. Reads coalesce NULL to "" (sessions.go).
		//
		// preferences used to be `TEXT NOT NULL DEFAULT '{}'`, which made every
		// MySQL boot fail: the column is absent from CREATE TABLE, so the ALTER
		// ran even on a fresh database, and MySQL refused it. persona_prompt
		// used textType, which is VARCHAR(64) on MySQL — while the handler
		// accepts 2000 runes, so any real persona prompt was a data-too-long
		// error there.
		{Table: "sessions", Column: "persona_prompt",
			DDL: `ALTER TABLE sessions ADD COLUMN persona_prompt TEXT`},
		{Table: "sessions", Column: "preferences",
			DDL: `ALTER TABLE sessions ADD COLUMN preferences TEXT`},
		// The fact track's per-item mute (ζ½/η). INTEGER rather than the
		// dialect's boolean spelling for the same reason every other flag here
		// is: SQLite has no BOOLEAN, and the three dialects share this list.
		{Table: "assignments", Column: "reminders_off",
			DDL: `ALTER TABLE assignments ADD COLUMN reminders_off ` + boolType + ` NOT NULL DEFAULT 0`},
	}
	intType := "INTEGER"
	if strings.Contains(textType, "VARCHAR") {
		intType = "INTEGER"
	}
	userText := "TEXT"
	if strings.Contains(textType, "VARCHAR") {
		userText = "VARCHAR(191)"
	}
	migs = append(migs,
		ColumnMigration{Table: "chat_messages", Column: "status",
			DDL: `ALTER TABLE chat_messages ADD COLUMN status ` + textType + ` NOT NULL DEFAULT ''`},
		ColumnMigration{Table: "memory_facts", Column: "type",
			DDL: `ALTER TABLE memory_facts ADD COLUMN type ` + textType + ` NOT NULL DEFAULT ''`},
		ColumnMigration{Table: "users", Column: "is_anonymous",
			DDL: `ALTER TABLE users ADD COLUMN is_anonymous ` + intType + ` NOT NULL DEFAULT 0`},
		ColumnMigration{Table: "users", Column: "data_session_id",
			DDL: `ALTER TABLE users ADD COLUMN data_session_id ` + userText + ` NOT NULL DEFAULT ''`},
		ColumnMigration{Table: "users", Column: "token_version",
			DDL: `ALTER TABLE users ADD COLUMN token_version ` + intType + ` NOT NULL DEFAULT 0`},
		// Existing rows get '' rather than a guessed domain: OpDomainOf can
		// derive one from the action at read time, and a wrong stored value
		// would quietly skew rapport scores that are meant to be replayable
		// from the log itself.
		ColumnMigration{Table: "operation_logs", Column: "domain",
			DDL: `ALTER TABLE operation_logs ADD COLUMN domain ` + textType + ` NOT NULL DEFAULT ''`},
		// '' reads as a user check-in: the agent could not record one before
		// this column existed, so every pre-existing row is one the user
		// entered themselves.
		ColumnMigration{Table: "mood_checkins", Column: "source",
			DDL: `ALTER TABLE mood_checkins ADD COLUMN source ` + textType + ` NOT NULL DEFAULT ''`},
		// note has no DEFAULT: MySQL rejects a literal default on TEXT, and the
		// three dialects share one migration list. Reads coalesce NULL to "".
		ColumnMigration{Table: "mood_checkins", Column: "note",
			DDL: `ALTER TABLE mood_checkins ADD COLUMN note TEXT`},
	)
	return migs
}
