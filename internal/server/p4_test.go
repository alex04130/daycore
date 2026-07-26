package server

import (
	"context"
	"testing"
	"time"

	"daycore/internal/domain"
	"daycore/internal/search"
)

// Before the fix, an empty id made every Set collide on the empty primary key,
// collapsing temp_context to a single global row. This proves keys are isolated,
// overwrite in place, and expired entries are hidden.
func TestTempContextIsolation(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()
	set := func(key, payload string, ttl time.Duration) {
		if err := s.store.TempContexts().Set(ctx, &domain.TempContext{
			SessionID: sid, Key: key, Payload: payload, TTL: time.Now().Add(ttl),
		}); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
	}
	set("a", "AAA", time.Hour)
	set("b", "BBB", time.Hour)

	if a, err := s.store.TempContexts().Get(ctx, sid, "a"); err != nil || a.Payload != "AAA" {
		t.Fatalf("key a clobbered: %+v (%v)", a, err)
	}
	if b, err := s.store.TempContexts().Get(ctx, sid, "b"); err != nil || b.Payload != "BBB" {
		t.Fatalf("key b: %+v (%v)", b, err)
	}

	// same key overwrites in place.
	set("a", "AAA2", time.Hour)
	if a, _ := s.store.TempContexts().Get(ctx, sid, "a"); a == nil || a.Payload != "AAA2" {
		t.Fatalf("overwrite failed: %+v", a)
	}

	// expired entries are not returned.
	set("exp", "X", -time.Hour)
	if _, err := s.store.TempContexts().Get(ctx, sid, "exp"); err == nil {
		t.Fatal("expired temp-context should not be returned")
	}
}

// The Searcher was 0% implemented and never wired; /materials/search always 501'd.
func TestMaterialSearcher(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()
	if _, err := s.store.Materials().Create(ctx, &domain.Material{
		SessionID: sid, Title: "Calculus notes", Body: "derivatives and integrals", Category: "study",
	}); err != nil {
		t.Fatal(err)
	}
	searcher := search.NewMaterialSearcher(s.store)

	res, err := searcher.Search(ctx, sid, domain.SearchQuery{Term: "integrals"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 || res[0].Title != "Calculus notes" {
		t.Fatalf("search miss: %+v", res)
	}

	none, err := searcher.Search(ctx, sid, domain.SearchQuery{Term: "zzznomatch"})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no results, got %d", len(none))
	}
}
