// Package search is the站内 full-text index over a user's own imported
// material — notes, assignments, screenshots, anything the file bus took in.
//
// # It used to hold two unrelated things
//
// Web search lived here too, and the package comment described only that half.
// The two share a verb and nothing else: this one is a repository query against
// rows this deployment owns, returning {ID, Title, Snippet, Score} with no URL
// because a stored note has none. Web search is a network call to a third party
// with sources, health and a registry. They are now internal/search and
// internal/websearch.
package search

import (
	"context"
	"strings"

	"daycore/internal/domain"
)

// MaterialSearcher implements domain.Searcher over the materials table. When
// the store also implements domain.MaterialFTS (SQLite FTS5 / Postgres
// tsvector / MySQL FULLTEXT / Mongo text — all four backends do), the
// engine-native index serves the query with real ranking; the original
// case-insensitive substring scan remains as the guaranteed-recall fallback
// (index unavailable, term too short for the tokenizer, engine can't tokenize
// the script, …).
type MaterialSearcher struct {
	store domain.Store
}

// NewMaterialSearcher builds a Searcher backed by the material repository.
func NewMaterialSearcher(store domain.Store) *MaterialSearcher {
	return &MaterialSearcher{store: store}
}

func (m *MaterialSearcher) Search(ctx context.Context, sid string, q domain.SearchQuery) ([]domain.SearchResult, error) {
	// Native FTS first; any ok=false (or error) falls through to the substring
	// scan below, which guarantees the recall floor.
	if fts, capable := m.store.(domain.MaterialFTS); capable && strings.TrimSpace(q.Term) != "" {
		if res, ok, err := fts.SearchMaterialsFTS(ctx, sid, q); err == nil && ok {
			return res, nil
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	mats, err := m.store.Materials().List(ctx, sid, q.Category, q.Term, limit, q.Offset)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SearchResult, 0, len(mats))
	for _, mat := range mats {
		out = append(out, domain.SearchResult{
			ID:      mat.ID,
			Title:   mat.Title,
			Snippet: buildSnippet(mat.Summary, mat.Body, q.Term),
			Score:   1,
		})
	}
	return out, nil
}

// Index/Deindex are no-ops on every backend: all four native indexes are
// self-maintaining (SQLite triggers / PG generated column / MySQL FULLTEXT /
// Mongo text index), and the substring fallback reads the table directly.
func (m *MaterialSearcher) Index(ctx context.Context, sid, id, title, body string) error { return nil }
func (m *MaterialSearcher) Deindex(ctx context.Context, id string) error                 { return nil }

func buildSnippet(summary, body, term string) string {
	if strings.TrimSpace(summary) != "" {
		return truncateRunes(summary, 160)
	}
	if term != "" {
		if i := strings.Index(strings.ToLower(body), strings.ToLower(term)); i >= 0 {
			start := i - 40
			if start < 0 {
				start = 0
			}
			return truncateRunes(body[start:], 160)
		}
	}
	return truncateRunes(body, 160)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
