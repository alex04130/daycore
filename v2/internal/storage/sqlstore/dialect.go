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
func sessionColumnMigrations(textType string) []ColumnMigration {
	migs := []ColumnMigration{
		{Table: "sessions", Column: "language",
			DDL: `ALTER TABLE sessions ADD COLUMN language ` + textType + ` NOT NULL DEFAULT ''`},
		{Table: "sessions", Column: "import_token",
			DDL: `ALTER TABLE sessions ADD COLUMN import_token ` + textType + ` NOT NULL DEFAULT ''`},
		{Table: "sessions", Column: "persona_prompt",
			DDL: `ALTER TABLE sessions ADD COLUMN persona_prompt ` + textType + ` NOT NULL DEFAULT ''`},
		{Table: "sessions", Column: "preferences",
			DDL: `ALTER TABLE sessions ADD COLUMN preferences TEXT NOT NULL DEFAULT '{}'`},
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
	)
	return migs
}
