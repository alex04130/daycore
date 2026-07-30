package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"

	"daycore/internal/domain"
	"daycore/internal/storage/storagetest"
)

// The behavioural suite against real PostgreSQL and MySQL.
//
// Until this file existed, only SQLite and MongoDB were ever executed. The two
// SQL dialects were written from documentation and checked by
// dialect_parity_test.go, which compares the three DDL strings to each other —
// a static check that cannot know whether any of them is legal. That gap let
// three real incidents through:
//
//   - sessions.preferences shipped as `TEXT NOT NULL DEFAULT '{}'`. MySQL
//     rejects a literal DEFAULT on TEXT, so MySQL would not boot at all.
//   - temp_contexts.key was backticked in the MySQL CREATE TABLE and in none of
//     the five statements that use it. `KEY` is reserved, so every temp-context
//     read and write was a syntax error.
//   - Three MySQL CREATE TABLEs were missing the comma before their first
//     inline KEY. Two 1064s each.
//
// Every one of those is a first-statement failure: the *only* thing needed to
// catch them is executing the DDL once against the real server. That is what
// this file does.
//
// Skips itself without a DSN, and CI asserts it did not skip.
func TestConformancePostgres(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set — see `make test-sql`")
	}
	storagetest.Run(t, func(t *testing.T) storagetest.Harness {
		return sqlHarness{newSchemaScopedStore(t, postgresDialect{}, dsn)}
	})
}

func TestConformanceMySQL(t *testing.T) {
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN not set — see `make test-sql`")
	}
	storagetest.Run(t, func(t *testing.T) storagetest.Harness {
		return sqlHarness{newSchemaScopedStore(t, mysqlDialect{}, dsn)}
	})
}

// testNamespace numbers the throwaway schemas. The suite builds a fresh harness
// per case, and cases assert on row counts and on "nothing else is in this
// table" — so they must not share a namespace, and a leftover from a failed run
// must not poison the next one.
var testNamespace atomic.Int64

func nextNamespace() string {
	return fmt.Sprintf("dc_test_%d_%d", os.Getpid(), testNamespace.Add(1))
}

// newSchemaScopedStore gives one test its own schema (Postgres) or database
// (MySQL, which has no separate notion of schema), migrates into it, and drops
// it afterwards.
//
// Isolation is per-namespace rather than per-table-truncate because Migrate is
// itself the thing under test here: a DDL that only fails on a fresh database
// (the sessions.preferences case above did exactly that) is invisible if the
// schema is created once and reused.
func newSchemaScopedStore(t *testing.T, d Dialect, adminDSN string) *Store {
	t.Helper()
	ns := nextNamespace()

	admin, err := sql.Open(d.DriverName(), d.NormalizeDSN(adminDSN))
	if err != nil {
		t.Fatalf("open admin connection (%s): %v", d.Name(), err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Fatalf("ping %s at %s: %v", d.Name(), redactDSN(adminDSN), err)
	}

	create, drop, scoped := namespaceSQL(t, d, adminDSN, ns)
	if _, err := admin.ExecContext(context.Background(), create); err != nil {
		t.Fatalf("create test namespace %q: %v", ns, err)
	}
	t.Cleanup(func() {
		// A separate connection: the store's pool is already closed by its own
		// cleanup, and dropping through a closed pool would leave the namespace
		// behind on every run.
		c, err := sql.Open(d.DriverName(), d.NormalizeDSN(adminDSN))
		if err != nil {
			t.Logf("could not reopen to drop %q: %v", ns, err)
			return
		}
		defer c.Close()
		if _, err := c.ExecContext(context.Background(), drop); err != nil {
			t.Logf("could not drop test namespace %q (harmless but it will accumulate): %v", ns, err)
		}
	})

	s, err := Open(d, scoped)
	if err != nil {
		t.Fatalf("open %s store in %q: %v", d.Name(), ns, err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(context.Background()); err != nil {
		// The most valuable failure in this file. Say so, because "migrate:
		// Error 1064" on its own reads like a test-harness problem.
		t.Fatalf("MIGRATE FAILED on real %s — the DDL for this dialect is not valid, "+
			"which dialect_parity_test.go cannot detect (it only compares the three DDLs to each other): %v",
			d.Name(), err)
	}
	// Best-effort DDL (native FTS) only warns. Surface the warnings: a dialect
	// where FTS never builds silently falls back to substring search forever.
	for _, w := range s.MigrationWarnings() {
		t.Logf("%s conditional migration warning: %s", d.Name(), w)
	}
	return s
}

// namespaceSQL returns the create statement, the drop statement, and a DSN
// scoped to the new namespace.
func namespaceSQL(t *testing.T, d Dialect, adminDSN, ns string) (create, drop, scoped string) {
	t.Helper()
	switch d.Name() {
	case "postgres":
		// A schema rather than a database: CREATE DATABASE cannot run inside a
		// transaction and needs its own connection to be usable, while a schema
		// is reachable immediately over the same connection. It also matches how
		// the dialect already scopes itself — postgresDialect's FTS check query
		// uses current_schema().
		return `CREATE SCHEMA ` + ns,
			`DROP SCHEMA IF EXISTS ` + ns + ` CASCADE`,
			withSearchPath(t, adminDSN, ns)
	case "mysql":
		return "CREATE DATABASE `" + ns + "`",
			"DROP DATABASE IF EXISTS `" + ns + "`",
			withMySQLDatabase(t, adminDSN, ns)
	default:
		t.Fatalf("namespaceSQL does not know how to isolate %q", d.Name())
		return "", "", ""
	}
}

// withSearchPath points a Postgres DSN at one schema. Both DSN spellings are
// handled because the URL form is what a human writes and the keyword form is
// what tooling often emits; guessing wrong would connect to the public schema
// and the tests would pass while proving nothing about isolation.
func withSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()
	if u, err := url.Parse(dsn); err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String()
	}
	return dsn + " search_path=" + schema
}

// withMySQLDatabase swaps the database name using the driver's own parser —
// string surgery on a MySQL DSN is how the production NormalizeDSN bug happened
// (a password containing '?' had a parameter appended to the database name).
func withMySQLDatabase(t *testing.T, dsn, name string) string {
	t.Helper()
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MYSQL_TEST_DSN: %v", err)
	}
	cfg.DBName = name
	return cfg.FormatDSN()
}

// redactDSN keeps a connection failure readable without printing the password
// into CI logs.
func redactDSN(dsn string) string {
	if u, err := url.Parse(dsn); err == nil && u.User != nil {
		if _, hasPW := u.User.Password(); hasPW {
			u.User = url.User(u.User.Username())
			return u.String()
		}
		return dsn
	}
	if at := strings.LastIndex(dsn, "@"); at > 0 {
		if colon := strings.Index(dsn[:at], ":"); colon > 0 {
			return dsn[:colon+1] + "***" + dsn[at:]
		}
	}
	return dsn
}

// Guard against the isolation itself being broken: two harnesses must not see
// each other's rows. Without this, a mistake in namespaceSQL would make every
// "this table contains exactly one row" assertion in the suite unreliable in a
// way that looks like a storage bug.
func TestRealDialectNamespacesAreIsolated(t *testing.T) {
	for _, tc := range []struct {
		env string
		d   Dialect
	}{
		{"PG_TEST_DSN", postgresDialect{}},
		{"MYSQL_TEST_DSN", mysqlDialect{}},
	} {
		dsn := os.Getenv(tc.env)
		if dsn == "" {
			continue
		}
		t.Run(tc.d.Name(), func(t *testing.T) {
			a := newSchemaScopedStore(t, tc.d, dsn)
			b := newSchemaScopedStore(t, tc.d, dsn)
			ctx := context.Background()
			if _, err := a.Sessions().GetOrCreate(ctx, "iso-"+strconv.Itoa(os.Getpid())); err != nil {
				t.Fatalf("write into namespace A: %v", err)
			}
			if _, err := b.Sessions().Get(ctx, "iso-"+strconv.Itoa(os.Getpid())); err == nil {
				t.Error("namespace B can see namespace A's session — the two harnesses share storage, so every count assertion in the suite is meaningless")
			} else if !isNotFound(err) {
				t.Errorf("namespace B failed for the wrong reason: %v", err)
			}
		})
	}
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), domain.ErrNotFound.Error())
}

// The native full-text index must actually get built on the real engines.
//
// It is a ConditionalMigration, which by design only warns on failure so a
// missing index cannot block startup. The cost of that design is this failure
// mode: the index never builds, `SearchMaterialsFTS` short-circuits on
// `condApplied["materials_fts"]`, and search silently runs the substring
// fallback forever. Nobody notices, because substring search returns results.
//
// docs/DATA.md flagged the pg/mysql syntax as "written from documentation, only
// verified on sqlite". This is that verification.
func TestNativeFTSBuildsOnRealEngines(t *testing.T) {
	for _, tc := range []struct {
		env string
		d   Dialect
	}{
		{"PG_TEST_DSN", postgresDialect{}},
		{"MYSQL_TEST_DSN", mysqlDialect{}},
	} {
		dsn := os.Getenv(tc.env)
		if dsn == "" {
			continue
		}
		t.Run(tc.d.Name(), func(t *testing.T) {
			s := newSchemaScopedStore(t, tc.d, dsn)
			if !s.condApplied["materials_fts"] {
				t.Errorf("materials_fts did not build on real %s — search will silently use the substring fallback forever. Warnings: %v",
					tc.d.Name(), s.MigrationWarnings())
				return
			}
			// Built is not the same as usable: the query syntax is per-engine too
			// (MATCH…AGAINST vs to_tsquery), and a wrong one errors only when
			// someone searches.
			ctx := context.Background()
			sid := "fts-" + strconv.Itoa(os.Getpid())
			if _, err := s.Sessions().GetOrCreate(ctx, sid); err != nil {
				t.Fatalf("create session: %v", err)
			}
			if _, err := s.Materials().Create(ctx, &domain.Material{
				SessionID: sid, Category: domain.CategoryNote,
				Title: "photosynthesis notes", Body: "chlorophyll absorbs light",
			}); err != nil {
				t.Fatalf("create material: %v", err)
			}
			hits, ok, err := s.SearchMaterialsFTS(ctx, sid, domain.SearchQuery{Term: "chlorophyll", Limit: 5})
			if err != nil {
				t.Errorf("native FTS query failed on real %s: %v", tc.d.Name(), err)
				return
			}
			if !ok {
				t.Errorf("native FTS reported itself unavailable on real %s despite having been applied", tc.d.Name())
				return
			}
			if len(hits) == 0 {
				t.Errorf("native FTS on real %s built and answered, but found nothing for a term that is in the body — the index or the query is wrong", tc.d.Name())
			}
		})
	}
}
