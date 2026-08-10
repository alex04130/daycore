package mongostore

import (
	"context"
	"fmt"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// The console's collection browser.
//
// # Documents have no columns, and the console needs some
//
// This is the one place the two storage shapes genuinely disagree. A SQL table
// hands back a fixed column list; a Mongo collection hands back documents that
// may each carry different keys. Rendering that faithfully would mean a
// different table shape per row, which is not a screen anybody can read.
//
// So the columns are the UNION of the keys on the page, ordered by first
// appearance, and a document missing one gets nil there. Two consequences worth
// stating rather than discovering:
//
//   - Page 2 can have a column page 1 did not. That is the truth about the
//     data, not a bug in the browser.
//   - `_id` comes first, because it is what every other screen refers to.
//
// # Redaction is by key name, exactly as on SQL
//
// The catalogue names columns; Mongo's field names are the same strings (the
// bson tags in this package mirror the SQL column names deliberately), so one
// catalogue serves both. ⚠️ If a bson tag ever diverges from its column name,
// redaction silently stops applying on this back end — which is why
// TestCatalogueRedactionMatchesBothBackends exists.
type browserRepo struct{ *Store }

func (r browserRepo) CountRows(ctx context.Context, table string) (int64, error) {
	t, ok := domain.TableByName(table)
	if !ok {
		return 0, domain.ErrNotFound
	}
	return r.c(t.Name).CountDocuments(ctx, bson.M{})
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

	// bson.D preserves field order, which bson.M does not — and the column
	// order IS the rendering. Decoding into a map would give a different column
	// order on every request, so the operator's eye never settles.
	cur, err := r.c(t.Name).Find(ctx, bson.M{},
		options.Find().
			SetSort(bson.D{{Key: t.OrderBy, Value: -1}, {Key: "_id", Value: -1}}).
			SetSkip(int64(offset)).
			SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	redact := map[string]bool{}
	for _, c := range t.Redact {
		redact[c] = true
	}

	var docs []bson.D
	colIndex := map[string]int{"_id": 0}
	cols := []string{"_id"}
	for cur.Next(ctx) {
		var d bson.D
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		for _, e := range d {
			if _, seen := colIndex[e.Key]; !seen {
				colIndex[e.Key] = len(cols)
				cols = append(cols, e.Key)
			}
		}
		docs = append(docs, d)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}

	page := &domain.TableRowPage{Columns: cols, Rows: [][]any{}, Total: total}
	for _, d := range docs {
		row := make([]any, len(cols))
		for _, e := range d {
			i := colIndex[e.Key]
			if redact[e.Key] {
				row[i] = domain.RedactedValue
				continue
			}
			row[i] = bsonSafe(e.Value)
		}
		page.Rows = append(page.Rows, row)
	}
	return page, nil
}

// bsonSafe turns a decoded BSON value into something json.Marshal renders the
// same way the SQL side would.
//
// Without it a nested document renders as Go's map syntax and an ObjectID as a
// twelve-byte array — the same logical row looking like two different things
// depending on which engine the deployment happens to run, which is the class
// of difference the four-backend suite exists to remove.
func bsonSafe(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string, bool, int32, int64, float64:
		return x
	case primitive.ObjectID:
		return x.Hex()
	case primitive.DateTime:
		return x.Time().UTC().Format("2006-01-02T15:04:05.000Z")
	case primitive.Binary:
		return string(x.Data)
	case bson.D:
		// A nested document, rendered as JSON text rather than as a nested
		// structure: the console draws a table of cells, and a cell holding an
		// object has nowhere to put it. The SQL side reaches the same place
		// because these columns are stored as JSON text there.
		m := bson.M{}
		for _, e := range x {
			m[e.Key] = bsonSafe(e.Value)
		}
		return fmt.Sprintf("%v", m)
	case bson.A:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = bsonSafe(e)
		}
		return fmt.Sprintf("%v", out)
	default:
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
	// The catalogue says "id" because that is the SQL column; this package
	// stores it as _id. One translation, in one place, rather than a second
	// column name in the catalogue that only one back end would read.
	key := t.KeyColumn
	if key == "id" {
		key = "_id"
	}
	res, err := r.c(t.Name).DeleteOne(ctx, bson.M{key: id})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}
