package mongostore

import (
	"context"
	"strings"

	"daycore/internal/domain"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SearchMaterialsFTS implements domain.MaterialFTS via Mongo's $text index
// (created in Migrate). Note the basic text index does not tokenize CJK — for
// Chinese content the substring fallback usually recalls better; ok=false is
// returned on any failure so the caller can fall back.
func (s *Store) SearchMaterialsFTS(ctx context.Context, sessionID string, q domain.SearchQuery) ([]domain.SearchResult, bool, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	filter := bson.M{"session_id": sessionID, "$text": bson.M{"$search": q.Term}}
	if q.Category != "" {
		filter["category"] = q.Category
	}
	opts := options.Find().
		SetProjection(bson.M{"title": 1, "summary": 1, "body": 1, "score": bson.M{"$meta": "textScore"}}).
		SetSort(bson.D{{Key: "score", Value: bson.M{"$meta": "textScore"}}}).
		SetLimit(int64(limit)).
		SetSkip(int64(q.Offset))
	cur, err := s.c("materials").Find(ctx, filter, opts)
	if err != nil {
		return nil, false, nil
	}
	defer cur.Close(ctx)
	out := []domain.SearchResult{}
	for cur.Next(ctx) {
		var d struct {
			ID      string  `bson:"_id"`
			Title   string  `bson:"title"`
			Summary string  `bson:"summary"`
			Body    string  `bson:"body"`
			Score   float64 `bson:"score"`
		}
		if err := cur.Decode(&d); err != nil {
			return nil, false, nil
		}
		snippet := d.Summary
		if strings.TrimSpace(snippet) == "" {
			if r := []rune(d.Body); len(r) > 160 {
				snippet = string(r[:160]) + "…"
			} else {
				snippet = d.Body
			}
		}
		out = append(out, domain.SearchResult{ID: d.ID, Title: d.Title, Snippet: snippet, Score: d.Score})
	}
	if cur.Err() != nil {
		return nil, false, nil
	}
	// Mongo $text on CJK terms typically matches nothing (no tokenizer) —
	// an empty result set falls back so Chinese substring search still works.
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}
