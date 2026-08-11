package domain

import "context"

// The browsable table catalogue.
//
// # Why a hand-written list and not runtime introspection
//
// Every engine can be asked what tables it has — `sqlite_master`,
// `information_schema.tables`, `listCollections`. Four implementations of a
// question, each returning a slightly different set (Mongo has no empty
// collections; Postgres has a schema qualifier; MySQL folds case), and the
// answer would still not carry the two things that actually matter: **which of
// these rows are somebody's diary, and which column of this table must never
// leave the process.**
//
// So the list is written down. It is also the only shape in which "a new table
// appeared and nobody classified it" can be a failing test rather than a
// discovery — see TestEveryTableIsCatalogued, which walks the DDL.
//
// # This is an allowlist, and that is its main job
//
// A table name arriving in a URL is never put into SQL. It is matched against
// this list, and the MATCHED ENTRY'S OWN CONSTANT is what reaches the query,
// quoted by the dialect. The difference matters even though a `[a-z_]+`
// validation would also "work": a validation is a claim about the shape of
// hostile input, and an allowlist is a claim about our own data. Only the
// second one stays true when somebody adds a feature.
//
// ⚠️ Every identifier in a browse query — table, order column, key column —
// comes from here. If you find yourself passing a column name in from an HTTP
// handler, stop: that is the injection this file exists to make impossible.

// TableClass says what kind of data a table holds, which decides who may read
// it.
//
// Two classes, because the author's decision that user content is *assignable*
// only means something if it can be assigned separately: "一个查问题的运维要看
// operation_logs，不需要看日记". A third class ("secret") does not exist — a
// table with secrets in it has those COLUMNS redacted instead, because the rest
// of the row is ordinary and hiding the whole table would make the browser
// useless exactly where debugging needs it.
type TableClass string

const (
	// TableOperational is what an operator debugs with. No user-authored text.
	TableOperational TableClass = "operational"
	// TableUserContent is what a person wrote or felt: conversations, moods,
	// remembered facts, plans, wishes. docs/STRATEGY.md puts this at the same
	// level as the companion boundaries.
	TableUserContent TableClass = "user_content"
)

// Table is one browsable table.
type Table struct {
	// Name is both the catalogue key and the identifier used in queries. It is
	// never taken from a request.
	Name  string
	Class TableClass
	// OrderBy is the column rows are sorted by, newest first.
	//
	// Required, and not defaulted to "created_at": ten of these tables do not
	// have one, and a browser with no ORDER BY returns rows in whatever order
	// the engine feels like — which means page two can repeat page one, and the
	// operator concludes the data is duplicated.
	OrderBy string
	// KeyColumn identifies one row for deletion. EMPTY MEANS NOT DELETABLE, and
	// that is a real state rather than an oversight:
	//
	//   - Ten tables have composite keys, so "delete row X" has no meaning.
	//   - Two of them (roles, role_members) have an invariant a raw delete
	//     breaks — DeleteRole removes the definition AND the membership, and
	//     deleting just the definition leaves rows that silently restore a
	//     deleted permission set if the name is ever reused.
	//
	// NotDeletableWhy carries the reason so the console can say it rather than
	// grey out a button for no visible cause.
	KeyColumn       string
	NotDeletableWhy string
	// Redact lists columns whose values must never leave the process.
	//
	// Redacted rather than the table being hidden: a credentials row's user_id
	// and timestamps are ordinary operational facts ("did this person ever set
	// a password"), and only the hash itself is dangerous. The browser returns
	// the column with a fixed marker so its presence is still visible.
	Redact []string
}

// Deletable reports whether a single row of this table can be removed through
// the browser.
func (t Table) Deletable() bool { return t.KeyColumn != "" }

// Tables is the catalogue, in the order the console shows it: the operational
// tables an operator reaches for first, then the rest, then user content last —
// so the sensitive half is not what somebody lands on.
//
// ⚠️ Adding a table to the schema without adding it here fails
// TestEveryTableIsCatalogued. Adding it here without the schema fails the same
// test from the other side.
var Tables = []Table{
	// ── operational: what is running, what ran, what broke ──────────────────
	{Name: "operation_logs", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "ai_call_logs", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "ai_usage_daily", Class: TableOperational, OrderBy: "day",
		// Composite key, and deleting from it by hand would be a mistake anyway:
		// the rollup is a pure function of the ledger for every day the ledger
		// still holds, so a hand-deleted row comes back on the next fold — or
		// worse, does not, because the ledger's rows for that day are gone and
		// the number is silently lower forever.
		NotDeletableWhy: "复合主键（day + model + endpoint），而且它是账本折出来的 —— 手删只会在下一次折叠时回来，或者永远回不来"},
	{Name: "job_runs", Class: TableOperational, OrderBy: "started_at", KeyColumn: "id"},
	{Name: "leases", Class: TableOperational, OrderBy: "acquired_at",
		NotDeletableWhy: "复合主键（lease_name + holder），没有单行 id 可指"},
	{Name: "sessions", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id",
		// The import token is a bearer credential: whoever has it can push
		// Canvas data into that session from anywhere.
		Redact: []string{"import_token"}},
	{Name: "users", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "credentials", Class: TableOperational, OrderBy: "created_at",
		NotDeletableWhy: "主键是 user_id，删密码要走用户删除",
		Redact:          []string{"password_hash"}},
	{Name: "oauth_identities", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "channel_bindings", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "temp_contexts", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id"},

	// ── operational: the operator's own settings ────────────────────────────
	{Name: "settings", Class: TableOperational, OrderBy: "updated_at",
		NotDeletableWhy: "主键是 setting_key，改配置走服务配置那一屏"},
	{Name: "provider_overrides", Class: TableOperational, OrderBy: "updated_at",
		NotDeletableWhy: "复合主键（kind + provider_id），改能力源走能力源那一屏"},
	{Name: "prompts", Class: TableOperational, OrderBy: "updated_at",
		NotDeletableWhy: "主键是 prompt_key，改提示词走提示词那一屏"},
	{Name: "prompt_overrides", Class: TableOperational, OrderBy: "updated_at",
		NotDeletableWhy: "复合主键（prompt_key + locale），改提示词走提示词那一屏"},
	{Name: "locale_overrides", Class: TableOperational, OrderBy: "updated_at",
		NotDeletableWhy: "复合主键（message_key + locale）"},
	{Name: "roles", Class: TableOperational, OrderBy: "created_at",
		// Not just "composite key" — this one would break an invariant.
		NotDeletableWhy: "删组要连成员一起删（见 domain/role.go），只删定义会让重名的新组静默恢复一批人的权限。走用户与权限那一屏"},
	{Name: "frontend_families", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "frontend_builds", Class: TableOperational, OrderBy: "first_seen_at", KeyColumn: "build_hash"},
	// ⚠️ Deletable from here, deliberately, even though there is a screen for
	// it. The screen refuses to delete an APPROVED kind that tokens still
	// declare — this is the escape hatch for the case that screen cannot help
	// with: a pattern that turned out to be wrong on a deployment whose console
	// is the thing it broke.
	{Name: "theme_kinds", Class: TableOperational, OrderBy: "created_at", KeyColumn: "name"},
	{Name: "pairings", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id",
		// ⚠️ The hash is a verifier, not a secret to steal — but it is still the
		// only thing standing between a leaked row and an attached console, and
		// there is no operational question the browser answers by showing it.
		Redact: []string{"secret_hash"}},
	{Name: "role_members", Class: TableOperational, OrderBy: "created_at",
		NotDeletableWhy: "复合主键（role_name + user_id），改成员走用户与权限那一屏"},

	// ── derived and cached: readable, but the source of truth is elsewhere ───
	{Name: "rapport_states", Class: TableOperational, OrderBy: "updated_at",
		NotDeletableWhy: "主键是 session_id；它是账本的折叠结果，删了会自己重算"},
	{Name: "rhythm_profiles", Class: TableOperational, OrderBy: "updated_at",
		NotDeletableWhy: "主键是 session_id"},
	{Name: "rhythm_days", Class: TableOperational, OrderBy: "day",
		NotDeletableWhy: "复合主键（session_id + day）"},
	{Name: "theme_switch_log", Class: TableOperational, OrderBy: "switched_at", KeyColumn: "id"},
	{Name: "feedback_logs", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "proposals", Class: TableOperational, OrderBy: "created_at", KeyColumn: "id"},

	// ── the schedule: structure a person built, not text they wrote ─────────
	{Name: "day_plans", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "schedule_rules", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "courses", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "assignments", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "custom_themes", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "attachments", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "import_history", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},

	// ── what a person wrote or felt ─────────────────────────────────────────
	{Name: "chat_threads", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "chat_messages", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "mood_checkins", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "memory_facts", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "companion_memory", Class: TableUserContent, OrderBy: "updated_at", KeyColumn: "id"},
	{Name: "materials", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
	{Name: "wishes", Class: TableUserContent, OrderBy: "created_at", KeyColumn: "id"},
}

// TableByName returns the catalogue entry, and whether the name is one we serve.
//
// ⚠️ This is the ONLY way a request-supplied table name may become a query. The
// returned Table carries our own constants; the caller must use those and never
// the string it was given, even though they compare equal — because the next
// person to add a case here will not know that rule unless the code shows it.
func TableByName(name string) (Table, bool) {
	for _, t := range Tables {
		if t.Name == name {
			return t, true
		}
	}
	return Table{}, false
}

// RedactedValue is what a redacted column returns instead of its value.
//
// A fixed marker rather than omitting the column: the operator needs to see
// that the field EXISTS and is set, and a column that silently disappears from
// one table reads as a schema difference between engines.
const RedactedValue = "«redacted»"

// TableRowPage is one page of a browsed table.
type TableRowPage struct {
	// Columns in the order the engine returned them.
	Columns []string `json:"columns"`
	// Rows are JSON-safe values: strings, numbers, booleans and nil. Byte slices
	// come back as strings; anything a driver returns that is none of those is
	// rendered with %v rather than dropped, because a cell that vanishes is
	// worse than one that reads oddly.
	Rows [][]any `json:"rows"`
	// Total is the whole table's row count, for the pager.
	Total int64 `json:"total"`
}

// Browser is the operations console's read-only-ish window onto storage.
//
// It sits BESIDE the repositories rather than among them, and the distinction
// is worth stating: every other interface in this package models a domain
// concept and hides the schema. This one deliberately exposes the schema,
// because "show me the actual rows" is the question it exists to answer — and a
// domain-shaped answer to that question is a different feature.
//
// ⚠️ Which means it must never become the easy path for product code. Nothing
// outside internal/server's admin handlers may call it; a feature that wants
// rows wants a repository method.
type Browser interface {
	// CountRows counts one catalogued table. An unknown name is ErrNotFound
	// rather than zero — zero is a real answer and must not double as "no such
	// table".
	CountRows(ctx context.Context, table string) (int64, error)
	// BrowseRows returns one page, newest first by the catalogue's OrderBy,
	// with redacted columns replaced.
	BrowseRows(ctx context.Context, table string, limit, offset int) (*TableRowPage, error)
	// DeleteRow removes one row by the catalogue's KeyColumn. It reports whether
	// a row was there. A table with no KeyColumn returns ErrUnsupported.
	DeleteRow(ctx context.Context, table, id string) (bool, error)
}
