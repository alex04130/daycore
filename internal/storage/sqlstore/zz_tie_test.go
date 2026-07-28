package sqlstore

import (
	"context"
	"sync"
	"testing"
	"time"

	"daycore/internal/domain"
)

// Two daemons, same tick: both compute created_at in Go before inserting.
func TestTieCreatedAtConcurrent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	collisions, bothSurvived := 0, 0
	for round := 0; round < 30; round++ {
		mk := func(title string) *domain.Proposal {
			return &domain.Proposal{
				SessionID: "s1", Title: title, Level: domain.LevelL2, Kind: domain.KindCard,
				TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: time.Now().Add(time.Hour),
				Origin: domain.OriginDaemon, MergeKey: "gap",
			}
		}
		a, b := mk("daemonA"), mk("daemonB")
		var wg sync.WaitGroup
		start := make(chan struct{})
		for _, p := range []*domain.Proposal{a, b} {
			wg.Add(1)
			go func(p *domain.Proposal) {
				defer wg.Done()
				<-start
				if err := s.Proposals().Create(ctx, p); err != nil {
					t.Error(err)
				}
			}(p)
		}
		close(start)
		wg.Wait()
		if a.CreatedAt.UnixMilli() != b.CreatedAt.UnixMilli() {
			continue
		}
		collisions++
		if _, err := s.Proposals().Supersede(ctx, "s1", "gap", a.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Proposals().Supersede(ctx, "s1", "gap", b.ID); err != nil {
			t.Fatal(err)
		}
		live, err := s.Proposals().List(ctx, domain.ProposalFilter{
			SessionID: "s1", State: domain.ProposalPending, Undelivered: true, DeliverableAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		if len(live) > 1 {
			bothSurvived++
		}
		// clean up so the next round starts empty
		if _, err := s.exec(ctx, `DELETE FROM proposals`); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("same-millisecond rounds: %d/30, rounds where BOTH cards stayed deliverable: %d", collisions, bothSurvived)
}
