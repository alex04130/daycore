package sqlstore

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Only SQLite is ever opened by a test, and CI runs no Postgres or MySQL
// container. That left a hole big enough to ship real outages through:
//
//   - feedback_logs had a full repository and no CREATE TABLE in any dialect,
//     so every insert failed on every engine
//   - sessions.preferences was added by an ALTER carrying a literal DEFAULT on
//     a TEXT column, which MySQL rejects — and since the column was absent from
//     CREATE TABLE the ALTER ran on fresh databases too, so MySQL could not boot
//   - temp_contexts.key was back-quoted in the MySQL DDL and bare in all five
//     statements that touch it, making every temp-context read a parse error
//
// None of those need a running server to catch. They are properties of the DDL
// strings themselves, so this file reads what the three dialects actually
// return and compares them. It is not a substitute for running the engines; it
// is the floor under the review that would otherwise be the only line of
// defence.
//
// When one of these fails, fix the schema — do not relax the check. Each one
// exists because the thing it forbids already happened.

var dialects = map[string]Dialect{
	"sqlite":   sqliteDialect{},
	"postgres": postgresDialect{},
	"mysql":    mysqlDialect{},
}

type column struct {
	typ  string // the type token alone, lowercased: "text", "varchar(191)", "bigint"
	rest string // everything after it, lowercased
}

type table struct {
	columns map[string]column
	indexes map[string]bool // "col,col" of each index over this table
	uniques map[string]bool
}

var (
	reCreateTable = regexp.MustCompile(`(?is)CREATE TABLE IF NOT EXISTS\s+(\w+)\s*\((.*)\)\s*$`)
	reCreateIndex = regexp.MustCompile(`(?is)^\s*CREATE\s+(UNIQUE\s+)?INDEX IF NOT EXISTS\s+(\w+)\s+ON\s+(\w+)\s*\(([^)]*)\)`)
	reInlineKey   = regexp.MustCompile(`(?i)^\s*(UNIQUE\s+)?KEY\s+(\w+)\s*\(([^)]*)\)\s*,?\s*$`)
	reOtherClause = regexp.MustCompile(`(?i)^\s*(PRIMARY KEY|CONSTRAINT|FOREIGN KEY|FULLTEXT|CHECK)\b`)
)

// parseSchema turns one dialect's Migrations() into a table map.
func parseSchema(t *testing.T, name string, d Dialect) map[string]*table {
	t.Helper()
	out := map[string]*table{}
	get := func(n string) *table {
		if out[n] == nil {
			out[n] = &table{columns: map[string]column{}, indexes: map[string]bool{}, uniques: map[string]bool{}}
		}
		return out[n]
	}

	for _, stmt := range d.Migrations() {
		if m := reCreateIndex.FindStringSubmatch(stmt); m != nil {
			tbl := get(m[3])
			cols := normalizeCols(m[4])
			if strings.TrimSpace(m[1]) != "" {
				tbl.uniques[cols] = true
			} else {
				tbl.indexes[cols] = true
			}
			if len(m[2]) > 63 {
				t.Errorf("%s: index name %q is %d bytes; Postgres silently truncates at 63, which turns two long names into one index and a no-op",
					name, m[2], len(m[2]))
			}
			continue
		}
		m := reCreateTable.FindStringSubmatch(strings.TrimSpace(stmt))
		if m == nil {
			continue // ALTER, CREATE VIRTUAL TABLE, trigger, etc.
		}
		tbl := get(m[1])
		for _, line := range strings.Split(m[2], "\n") {
			line = strings.TrimSpace(line)
			if line == "" || reOtherClause.MatchString(line) {
				continue
			}
			if k := reInlineKey.FindStringSubmatch(line); k != nil {
				cols := normalizeCols(k[3])
				if strings.TrimSpace(k[1]) != "" {
					tbl.uniques[cols] = true
				} else {
					tbl.indexes[cols] = true
				}
				if len(k[2]) > 63 {
					t.Errorf("%s: index name %q exceeds Postgres's 63-byte identifier limit", name, k[2])
				}
				continue
			}
			fields := strings.Fields(strings.TrimSuffix(line, ","))
			if len(fields) < 2 {
				continue
			}
			col := strings.Trim(fields[0], "`\"")
			rest := ""
			if len(fields) > 2 {
				rest = strings.ToLower(strings.Join(fields[2:], " "))
			}
			tbl.columns[col] = column{typ: strings.ToLower(fields[1]), rest: rest}
		}
	}
	return out
}

func normalizeCols(s string) string {
	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(p), "`\"")
	}
	return strings.Join(parts, ",")
}

// Every table must exist in all three dialects with the same columns. A table
// present in one and missing from another is an outage on that engine only, and
// nothing else in the test suite would notice.
func TestDialectsDeclareTheSameSchema(t *testing.T) {
	schemas := map[string]map[string]*table{}
	for name, d := range dialects {
		schemas[name] = parseSchema(t, name, d)
	}

	all := map[string]bool{}
	for _, s := range schemas {
		for tbl := range s {
			all[tbl] = true
		}
	}
	for _, tbl := range sortedSet(all) {
		var missing []string
		for _, name := range sortedKeys(schemas) {
			if schemas[name][tbl] == nil {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			t.Errorf("table %q is missing from %v", tbl, missing)
			continue
		}
		cols := map[string]bool{}
		for _, name := range sortedKeys(schemas) {
			for c := range schemas[name][tbl].columns {
				cols[c] = true
			}
		}
		for _, c := range sortedSet(cols) {
			var lack []string
			for _, name := range sortedKeys(schemas) {
				if _, ok := schemas[name][tbl].columns[c]; !ok {
					lack = append(lack, name)
				}
			}
			if len(lack) > 0 {
				t.Errorf("%s.%s is missing from %v", tbl, c, lack)
			}
		}
	}
}

// An index that exists on one engine and not another is a table scan in
// production on the engine that lacks it, and nothing local would ever show it.
func TestDialectsDeclareTheSameIndexes(t *testing.T) {
	schemas := map[string]map[string]*table{}
	for name, d := range dialects {
		schemas[name] = parseSchema(t, name, d)
	}
	ref := schemas["sqlite"]
	for _, tbl := range sortedKeys(ref) {
		for _, name := range []string{"postgres", "mysql"} {
			other := schemas[name][tbl]
			if other == nil {
				continue // already reported by the schema test
			}
			for cols := range ref[tbl].indexes {
				if !other.indexes[cols] && !other.uniques[cols] {
					t.Errorf("%s: index on %s(%s) exists in sqlite but not here", name, tbl, cols)
				}
			}
			for cols := range ref[tbl].uniques {
				if !other.uniques[cols] {
					t.Errorf("%s: UNIQUE index on %s(%s) exists in sqlite but not here — a uniqueness invariant enforced on one engine only is not enforced",
						name, tbl, cols)
				}
			}
		}
	}
}

// Comma placement inside a CREATE TABLE body: every element but the last needs
// a trailing comma, and the last must not have one.
//
// This is here because it is exactly what the batch C generator got wrong —
// three MySQL tables came out with no comma before their first inline KEY and a
// stray one before the closing paren, which is two syntax errors per table and a
// MySQL that will not boot. The index-parity check could not see it: it matches
// each KEY line on its own and never looks at the punctuation joining them.
func TestCreateTableCommasAreWellFormed(t *testing.T) {
	for name, d := range dialects {
		for _, stmt := range d.Migrations() {
			m := reCreateTable.FindStringSubmatch(strings.TrimSpace(stmt))
			if m == nil {
				continue
			}
			tbl := m[1]
			var lines []string
			for _, l := range strings.Split(m[2], "\n") {
				if strings.TrimSpace(l) != "" {
					lines = append(lines, strings.TrimSpace(l))
				}
			}
			for i, l := range lines {
				last := i == len(lines)-1
				// A multi-line element (none today, but a CHECK or a long
				// expression could be one) would make this too strict; flag it
				// rather than guess.
				if strings.HasSuffix(l, "(") {
					t.Errorf("%s: %s has a line ending in '(' — the comma check assumes one element per line", name, tbl)
					continue
				}
				if last && strings.HasSuffix(l, ",") {
					t.Errorf("%s: %s's last element %q has a trailing comma before the closing paren — syntax error", name, tbl, l)
				}
				if !last && !strings.HasSuffix(l, ",") {
					t.Errorf("%s: %s element %q is missing its trailing comma — the next line gets parsed as part of it", name, tbl, l)
				}
			}
		}
	}
}

// MySQL rejects a literal DEFAULT on TEXT/BLOB. Postgres and SQLite accept one,
// so this is invisible until a MySQL box refuses to start.
func TestMySQLTextColumnsHaveNoLiteralDefault(t *testing.T) {
	for tbl, def := range parseSchema(t, "mysql", mysqlDialect{}) {
		for col, c := range def.columns {
			// The TYPE token, not the whole line: `source_type VARCHAR(32) NOT
			// NULL DEFAULT 'text'` is a varchar whose default happens to be the
			// word "text".
			blobby := strings.HasSuffix(c.typ, "text") || strings.HasSuffix(c.typ, "blob")
			if blobby && strings.Contains(c.rest, "default") {
				t.Errorf("mysql: %s.%s is %s %s — MySQL refuses a literal DEFAULT on TEXT/BLOB. Make it nullable and COALESCE on read, or use VARCHAR(n).",
					tbl, col, c.typ, c.rest)
			}
		}
	}
}

// ALTER TABLE ADD COLUMN is where the same trap bites hardest: the statement is
// shared by all three dialects, and a failure here is a hard boot error rather
// than a warning.
func TestColumnMigrationsAreLegalEverywhere(t *testing.T) {
	for name, d := range dialects {
		schema := parseSchema(t, name, d)
		for _, m := range d.ColumnMigrations() {
			ddl := strings.ToLower(m.DDL)

			if strings.Contains(ddl, "not null") && !strings.Contains(ddl, "default") {
				t.Errorf("%s: %s.%s adds NOT NULL with no DEFAULT — SQLite rejects that outright and MySQL does under strict mode",
					name, m.Table, m.Column)
			}
			blobby := strings.Contains(ddl, " text") || strings.Contains(ddl, "longtext") || strings.Contains(ddl, "blob")
			if name == "mysql" && blobby && strings.Contains(ddl, "default") {
				t.Errorf("mysql: %s.%s adds a TEXT column with a literal DEFAULT; MySQL refuses it and the server will not boot",
					m.Table, m.Column)
			}
			// Postgres matches information_schema's case-folded names, so an
			// uppercase letter here means the guard never matches, the ALTER
			// re-runs on every boot, and the second boot dies on "column
			// already exists".
			if m.Table != strings.ToLower(m.Table) || m.Column != strings.ToLower(m.Column) {
				t.Errorf("%s: ColumnMigration{%s, %s} must be lowercase for the Postgres column-exists guard", name, m.Table, m.Column)
			}
			if schema[m.Table] == nil {
				t.Errorf("%s: ColumnMigration targets %q, which has no CREATE TABLE — the ALTER can only ever fail", name, m.Table)
				continue
			}
			// A column reachable only through an ALTER means a fresh database
			// runs a migration it should not need, and fresh vs upgraded
			// schemas drift apart permanently.
			c, ok := schema[m.Table].columns[m.Column]
			if !ok {
				t.Errorf("%s: %s.%s exists only as a ColumnMigration and not in CREATE TABLE, so even a brand-new database runs the ALTER",
					name, m.Table, m.Column)
				continue
			}
			// The two paths must declare the SAME type. When they disagree the
			// column-exists guard skips the ALTER on a fresh database, so a new
			// install keeps the CREATE TABLE type forever while an upgraded one
			// gets the ALTER type — a permanent split between two databases
			// running identical code. MySQL had three of these (language,
			// operation_logs.domain, mood_checkins.source: VARCHAR(16/16/32) in
			// CREATE TABLE against VARCHAR(64) in the ALTER).
			if !strings.Contains(ddl, " "+c.typ) {
				t.Errorf("%s: %s.%s is %q in CREATE TABLE but the ALTER says %q — fresh and upgraded databases would differ permanently",
					name, m.Table, m.Column, c.typ, m.DDL)
			}
		}
	}
}

// The feedback_logs catcher: a repository can reference a table that no dialect
// creates, and everything compiles, and the sqlite test suite never touches it.
func TestEveryTableUsedBySQLIsCreated(t *testing.T) {
	created := parseSchema(t, "sqlite", sqliteDialect{})
	// Tables created outside Migrations() (conditional feature DDL) or supplied
	// by the engine itself.
	exempt := map[string]bool{
		"materials_fts":             true, // ConditionalMigration (SQLite FTS5)
		"pragma_table_info":         true,
		"information_schema":        true,
		"sqlite_master":             true,
		"information_schema.tables": true,
	}

	// Only look inside backtick-quoted string literals. Scanning whole files
	// picks up prose in comments — "FROM the ledger" would report a table named
	// "the".
	reLiteral := regexp.MustCompile("(?s)`[^`]*`")
	re := regexp.MustCompile(`(?i)\b(?:INSERT INTO|UPDATE|DELETE FROM|FROM|JOIN)\s+([a-z_][a-z0-9_]*)`)
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") ||
			strings.HasSuffix(e.Name(), "_test.go") || strings.HasPrefix(e.Name(), "dialect") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, lit := range reLiteral.FindAllString(string(src), -1) {
			for _, m := range re.FindAllStringSubmatch(lit, -1) {
				tbl := strings.ToLower(m[1])
				seen[tbl] = append(seen[tbl], e.Name())
			}
		}
	}
	for _, tbl := range sortedKeys(seen) {
		if exempt[tbl] || created[tbl] != nil {
			continue
		}
		// Skip SQL keywords the regex can pick up after FROM in a subquery.
		if tbl == "select" || tbl == "dual" || tbl == "values" {
			continue
		}
		t.Errorf("%s is queried by %v but no dialect creates it", tbl, dedupe(seen[tbl]))
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSet(m map[string]bool) []string { return sortedKeys(m) }

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
