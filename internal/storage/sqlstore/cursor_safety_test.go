package sqlstore

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"daycore/internal/domain"
)

// The ledger's timestamps are not commit-ordered, so a consumer that advances
// its cursor to the newest row it can see loses everything that was stamped
// earlier and landed later — permanently, because the predicate is
// `created_at > cursor`.
//
// The same workload runs two ways. The naive consumer is here to prove the
// hazard is live rather than theoretical: if it ever stops losing rows, either
// the write path grew an ordering guarantee (and domain.OpLogVisibilityLag can
// go) or this test stopped exercising the race and needs more pressure. The
// safe consumer, committing its cursor through domain.AdvanceCursor, must lose
// nothing.
//
// Both read forward to exhaustion on every pass — that is what a real catch-up
// does. The only difference is which cursor gets persisted: the newest row read,
// or the newest row old enough that nothing can still be landing behind it.
//
// Measured when this was written: naive lost 34-39 of 480, safe lost 0.
func TestCursorAdvanceSurvivesUnorderedTimestamps(t *testing.T) {
	for _, mode := range []string{"naive", "safe"} {
		t.Run(mode, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			const writers, perWriter = 8, 60

			var wg sync.WaitGroup
			for w := 0; w < writers; w++ {
				wg.Add(1)
				go func(w int) {
					defer wg.Done()
					for i := 0; i < perWriter; i++ {
						_ = s.OpLogs().Add(ctx, &domain.OperationLog{
							SessionID: "s1", Actor: domain.ActorAgent, Action: "plan_add",
							TargetID: fmt.Sprintf("w%d-%d", w, i),
						})
					}
				}(w)
			}

			seen := map[string]bool{}
			cursor := domain.LogCursor{}
			// One catch-up pass: read from the persisted cursor to exhaustion,
			// fold everything encountered, then persist whichever cursor this
			// mode considers safe.
			pass := func(now time.Time) {
				read, commit := cursor, cursor
				for {
					page, err := s.OpLogs().Scan(ctx, "s1", read, 200)
					if err != nil || len(page) == 0 {
						break
					}
					for _, l := range page {
						seen[l.TargetID] = true
					}
					if mode == "safe" {
						commit = domain.AdvanceCursor(commit, page, now)
					} else {
						last := page[len(page)-1]
						commit = domain.LogCursor{CreatedAt: last.CreatedAt, ID: last.ID}
					}
					last := page[len(page)-1]
					read = domain.LogCursor{CreatedAt: last.CreatedAt, ID: last.ID}
				}
				cursor = commit
			}

			stop := make(chan struct{})
			var consumer sync.WaitGroup
			consumer.Add(1)
			go func() {
				defer consumer.Done()
				for {
					select {
					case <-stop:
						return
					default:
					}
					pass(time.Now())
					time.Sleep(time.Millisecond)
				}
			}()

			wg.Wait()
			close(stop)
			consumer.Wait()
			// A real consumer reaches this state by simply running again once
			// the tail has aged out of the unsafe window.
			pass(time.Now().Add(2 * domain.OpLogVisibilityLag))

			var total int
			var c domain.LogCursor
			for {
				page, err := s.OpLogs().Scan(ctx, "s1", c, 500)
				if err != nil {
					t.Fatal(err)
				}
				if len(page) == 0 {
					break
				}
				total += len(page)
				last := page[len(page)-1]
				c = domain.LogCursor{CreatedAt: last.CreatedAt, ID: last.ID}
			}

			missed := total - len(seen)
			t.Logf("%s: 库里 %d 行，消费者见到 %d 行，漏 %d", mode, total, len(seen), missed)
			switch mode {
			case "safe":
				if missed != 0 {
					t.Errorf("安全游标仍漏了 %d/%d 行", missed, total)
				}
			case "naive":
				if missed == 0 {
					t.Skip("朴素游标这一轮没丢行 —— 竞态没被压出来，或写路径有了顺序保证；" +
						"若长期如此，先确认 domain.OpLogVisibilityLag 是否还需要存在")
				}
			}
		})
	}
}
