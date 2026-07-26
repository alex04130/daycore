package domain

import "context"

// SearchQuery represents the parameters for a search operation.
type SearchQuery struct {
	Term     string
	Category string
	Tags     []string
	Limit    int
	Offset   int
}

// SearchResult represents a single result from a search operation.
type SearchResult struct {
	ID      string
	Title   string
	Snippet string
	Score   float64
}

// Searcher defines the interface for search and indexing operations.
type Searcher interface {
	Search(ctx context.Context, sid string, query SearchQuery) ([]SearchResult, error)
	Index(ctx context.Context, sid string, id string, title string, body string) error
	Deindex(ctx context.Context, id string) error
}

// MaterialFTS is optionally implemented by stores with a native full-text
// index over materials (SQLite FTS5 / PG tsvector / MySQL FULLTEXT / Mongo
// text index). ok=false means the engine can't serve this query natively
// (index missing, term too short, …) and the caller should fall back to the
// substring scan.
type MaterialFTS interface {
	SearchMaterialsFTS(ctx context.Context, sessionID string, q SearchQuery) (results []SearchResult, ok bool, err error)
}
