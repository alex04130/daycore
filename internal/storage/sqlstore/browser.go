package sqlstore

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"daycore/internal/domain"
)

// The console's table browser.
//
// # The only place in this package that puts an identifier into SQL
//
// Everything else is a hand-written query with a literal table name. Here the
// table comes from a URL, and that is the whole risk — so it goes through
// exactly one door:
//
//	domain.TableByName(name)   → an entry, or "no such table" and we stop
//	t.Name, t.OrderBy, …       → OUR constants, from the catalogue
//	s.d.Quote(…)               → the dialect's own quoting
//
// The caller's string is never the string that reaches the query, even when the
// two compare equal. That looks like superstition on the line where it is
// written and it is not: the next person to add a filter here will copy the
// shape they see, and the shape they see must be the safe one.
//
// ⚠️ There is no path for a caller-supplied column, sort direction, or
// predicate. If one is ever wanted, it goes in the catalogue first.
//
// # Why offset paging here and keyset paging for the AI ledger
//
// Opposite choices in the same batch, so the reason should be on the page. The
// ledger is read newest-first while it is being written to, and offset paging
// over a table growing at the head repeats and skips rows. A browsed table is
// read while nothing much is happening to it, the operator wants "page 7 of
// 40", and a keyset cursor cannot express that. The cost is the ordinary one:
// if rows are inserted mid-browse the pages shift under you.
type browserRepo struct{ *Store }

func (r browserRepo) CountRows(ctx context.Context, table string) (int64, error) {
	t, ok := domain.TableByName(table)
	if !ok {
		return 0, domain.ErrNotFound
	}
	var n int64
	// t.Name, not table.
	if err := r.queryRow(ctx, `SELECT COUNT(*) FROM `+r.d.Quote(t.Name)).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (r browserRepo) BrowseRows(ctx context.Context, table string, limit, offset int) (*domain.TableRowPage, error) {
	t, ok := domain.TableByName(table)
	if !ok {
		return nil, domain.ErrNotFound
	}
	limit = domain.ListLimit(limit, domain.BrowseListDefault, domain.BrowseListMax)
	if offset < 0 {
		offset = 0
	}
	total, err := r.CountRows(ctx, t.Name)
	if err != nil {
		return nil, err
	}

	// SELECT * rather than a column list, because the catalogue deliberately
	// does not carry one: 37 tables × their columns is an artifact that drifts
	// from the schema the first time somebody adds a field, and a browser
	// showing stale columns is worse than one showing all of them. Redaction is
	// applied by name after the fact, which is why Redact must name a column
	// that exists — TestCataloguedColumnsExist checks that against the engine.
	q := `SELECT * FROM ` + r.d.Quote(t.Name) +
		` ORDER BY ` + r.d.Quote(t.OrderBy) + ` DESC` +
		` LIMIT ` + strconv.Itoa(limit) + ` OFFSET ` + strconv.Itoa(offset)
	rows, err := r.query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	redact := make([]bool, len(cols))
	for i, c := range cols {
		for _, rc := range t.Redact {
			if c == rc {
				redact[i] = true
			}
		}
	}

	page := &domain.TableRowPage{Columns: cols, Rows: [][]any{}, Total: total}
	for rows.Next() {
		cells := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		out := make([]any, len(cols))
		for i, v := range cells {
			if redact[i] {
				// Marked even when NULL: "this column is redacted" is the fact
				// the operator needs, and showing an empty cell for an unset
				// secret would let them conclude one is not configured.
				out[i] = domain.RedactedValue
				continue
			}
			out[i] = jsonSafe(v)
		}
		page.Rows = append(page.Rows, out)
	}
	return page, rows.Err()
}

// jsonSafe turns a driver value into something json.Marshal renders usefully.
//
// The three SQL drivers disagree about what they hand back for the same column:
// SQLite gives int64 and []byte, MySQL gives []byte for almost everything,
// Postgres gives typed values. A browser that rendered []byte would show
// base64 for every string on MySQL — the same table, unreadable on one engine.
func jsonSafe(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(x)
	case string, bool, int64, int32, int, float64, float32:
		return x
	default:
		// Times and anything a driver invents. %v rather than dropping the cell:
		// a value that reads oddly is debuggable, a cell that vanishes is not.
		return fmt.Sprintf("%v", x)
	}
}

func (r browserRepo) DeleteRow(ctx context.Context, table, id string) (bool, error) {
	t, ok := domain.TableByName(table)
	if !ok {
		return false, domain.ErrNotFound
	}
	if !t.Deletable() {
		return false, domain.ErrUnsupported
	}
	res, err := r.exec(ctx,
		`DELETE FROM `+r.d.Quote(t.Name)+` WHERE `+r.d.Quote(t.KeyColumn)+` = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// browserColumnsExist is used by the parity test to check the catalogue against
// a real engine: it asks for zero rows, which still makes the engine parse every
// identifier the browser will ever use for this table.
//
// It lives here rather than in the test file because it must run the SAME
// quoting and the same column list as the real query — a test that built its own
// SQL would be checking its own spelling.
func (r browserRepo) probe(ctx context.Context, t domain.Table) error {
	cols := append([]string{t.OrderBy}, t.Redact...)
	if t.KeyColumn != "" {
		cols = append(cols, t.KeyColumn)
	}
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = r.d.Quote(c)
	}
	rows, err := r.query(ctx,
		`SELECT `+strings.Join(quoted, ", ")+` FROM `+r.d.Quote(t.Name)+` LIMIT 0`)
	if err != nil {
		return err
	}
	return closeRows(rows)
}

func closeRows(rows *sql.Rows) error {
	err := rows.Err()
	if cerr := rows.Close(); err == nil {
		err = cerr
	}
	return err
}
