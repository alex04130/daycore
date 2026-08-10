package sqlstore

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"testing"

	"daycore/internal/domain"
)

// Every table the schema creates is in the catalogue, and vice versa.
//
// # What this catches
//
// A new table lands, and nobody classifies it. The browser then does not show
// it — which is invisible, because the screen still works and simply has one
// fewer card. The table grows, and the first time anybody looks for it is when
// they are trying to work out why the disk is full.
//
// The other direction matters more: a catalogue entry whose table does not
// exist is a card that 500s when clicked, and a redaction rule naming a column
// that was renamed is a SECRET THAT STOPS BEING REDACTED with nothing failing.
// That is the reason the column probe below exists as well.
func TestEveryTableIsCatalogued(t *testing.T) {
	inSchema := map[string]bool{}
	re := regexp.MustCompile(`(?is)CREATE TABLE IF NOT EXISTS\s+(\w+)`)
	for _, stmt := range (sqliteDialect{}).Migrations() {
		if m := re.FindStringSubmatch(stmt); m != nil {
			inSchema[m[1]] = true
		}
	}
	// Without this the walk can find nothing and the loops below assert about
	// two empty sets — the failure mode of every list-comparing check.
	if len(inSchema) < 30 {
		t.Fatalf("only found %d CREATE TABLE statements; this gate is not looking at what it thinks it is", len(inSchema))
	}

	catalogued := map[string]bool{}
	for _, tb := range domain.Tables {
		if catalogued[tb.Name] {
			t.Errorf("%s appears twice in domain.Tables", tb.Name)
		}
		catalogued[tb.Name] = true
		if tb.OrderBy == "" {
			t.Errorf("%s has no OrderBy — without one the engine returns rows in any order it likes, "+
				"so page two can repeat page one and the operator concludes the data is duplicated", tb.Name)
		}
		if tb.Class != domain.TableOperational && tb.Class != domain.TableUserContent {
			t.Errorf("%s has class %q, which is neither of the two", tb.Name, tb.Class)
		}
		if !tb.Deletable() && tb.NotDeletableWhy == "" {
			t.Errorf("%s cannot be deleted from and does not say why — the console would grey out a "+
				"button with no visible cause, which reads as a bug", tb.Name)
		}
	}

	var missing, extra []string
	for name := range inSchema {
		if !catalogued[name] {
			missing = append(missing, name)
		}
	}
	for name := range catalogued {
		if !inSchema[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("these tables exist and are not in domain.Tables: %s\n"+
			"  An uncatalogued table is invisible in the console — including its row count, which is\n"+
			"  how anybody would notice it growing. Classify it as operational or user_content.",
			strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		t.Errorf("these are in domain.Tables and are not in the schema: %s\n"+
			"  Each is a card that 500s when clicked.", strings.Join(extra, ", "))
	}
}

// Every identifier the browser will use actually exists, checked against a real
// engine rather than against the DDL text.
//
// The redaction rules are the reason. `Redact: []string{"password_hash"}` is a
// string, and a string that no longer names a column does not fail — it just
// stops redacting, and the browser starts serving the hash. Nothing about that
// looks wrong on any screen.
func TestCataloguedColumnsExist(t *testing.T) {
	s := newTestStore(t)
	b := browserRepo{s}
	ctx := context.Background()
	probed := 0
	for _, tb := range domain.Tables {
		if err := b.probe(ctx, tb); err != nil {
			t.Errorf("%s: the browser cannot select the columns the catalogue names (%v).\n"+
				"  OrderBy=%q KeyColumn=%q Redact=%v", tb.Name, err, tb.OrderBy, tb.KeyColumn, tb.Redact)
			continue
		}
		probed++
	}
	if probed < 30 {
		t.Fatalf("only probed %d tables; this gate is not looking at what it thinks it is", probed)
	}
}

// A redaction that names a column the table does not have is worse than no
// redaction, because it reads like protection. The probe above proves the
// column exists; this proves we did not forget one that should be there.
//
// It is a deliberately small, hand-written list: "which columns are secrets" is
// a judgement, and a heuristic over column names would either miss
// `import_token` or flag every column with `key` in it.
func TestTheKnownSecretsAreRedacted(t *testing.T) {
	mustRedact := map[string][]string{
		"credentials": {"password_hash"},
		"sessions":    {"import_token"},
	}
	for name, cols := range mustRedact {
		tb, ok := domain.TableByName(name)
		if !ok {
			t.Errorf("%s is not catalogued at all", name)
			continue
		}
		for _, c := range cols {
			found := false
			for _, r := range tb.Redact {
				if r == c {
					found = true
				}
			}
			if !found {
				t.Errorf("%s.%s is served in full by the database browser.\n"+
					"  Anybody holding db.operational can read it, and the permission's damage line does\n"+
					"  not say that.", name, c)
			}
		}
	}
}
