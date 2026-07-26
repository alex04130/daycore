package sqlstore

import (
	"context"
	"database/sql"
	"strings"

	"daycore/internal/domain"
)

// SearchMaterialsFTS implements domain.MaterialFTS over the engine-native
// full-text index created by the "materials_fts" conditional migration.
// ok=false (never a hard error) tells the caller to use the substring scan —
// index missing, term too short for the tokenizer, or a degraded index.
func (s *Store) SearchMaterialsFTS(ctx context.Context, sessionID string, q domain.SearchQuery) ([]domain.SearchResult, bool, error) {
	if !s.condApplied["materials_fts"] {
		return nil, false, nil
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	var (
		query string
		args  []any
	)
	switch s.d.Name() {
	case "sqlite":
		// The trigram tokenizer cannot match terms shorter than 3 runes.
		if len([]rune(q.Term)) < 3 {
			return nil, false, nil
		}
		// Quote the term so user input can't inject FTS5 query syntax.
		term := `"` + strings.ReplaceAll(q.Term, `"`, `""`) + `"`
		query = `SELECT m.id, m.title, snippet(materials_fts, 2, '', '', '…', 16), -bm25(materials_fts)
			FROM materials_fts JOIN materials m ON m.rowid = materials_fts.rowid
			WHERE materials_fts MATCH ? AND m.session_id = ?`
		args = []any{term, sessionID}
		if q.Category != "" {
			query += ` AND m.category = ?`
			args = append(args, q.Category)
		}
		query += ` ORDER BY bm25(materials_fts) LIMIT ? OFFSET ?`
		args = append(args, limit, q.Offset)
	case "postgres":
		query = `SELECT id, title, left(coalesce(nullif(summary, ''), body, ''), 160),
				ts_rank(fts, websearch_to_tsquery('simple', ?))
			FROM materials
			WHERE session_id = ? AND fts @@ websearch_to_tsquery('simple', ?)`
		args = []any{q.Term, sessionID, q.Term}
		if q.Category != "" {
			query += ` AND category = ?`
			args = append(args, q.Category)
		}
		query += ` ORDER BY 4 DESC LIMIT ? OFFSET ?`
		args = append(args, limit, q.Offset)
	case "mysql":
		query = `SELECT id, title, LEFT(COALESCE(NULLIF(summary, ''), body, ''), 160),
				MATCH(title, summary, body) AGAINST (? IN NATURAL LANGUAGE MODE)
			FROM materials
			WHERE session_id = ? AND MATCH(title, summary, body) AGAINST (? IN NATURAL LANGUAGE MODE)`
		args = []any{q.Term, sessionID, q.Term}
		if q.Category != "" {
			query += ` AND category = ?`
			args = append(args, q.Category)
		}
		query += ` ORDER BY 4 DESC LIMIT ? OFFSET ?`
		args = append(args, limit, q.Offset)
	default:
		return nil, false, nil
	}

	rows, err := s.query(ctx, query, args...)
	if err != nil {
		// Degraded index (e.g. FTS5 module unavailable at runtime despite the
		// table existing): report unsupported, not an error — the substring
		// path still serves the request.
		return nil, false, nil
	}
	defer rows.Close()
	out := []domain.SearchResult{}
	for rows.Next() {
		var (
			r              domain.SearchResult
			title, snippet sql.NullString
			score          float64
		)
		if err := rows.Scan(&r.ID, &title, &snippet, &score); err != nil {
			return nil, false, nil
		}
		r.Title = title.String
		r.Snippet = snippet.String
		r.Score = score
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil
	}
	return out, true, nil
}
