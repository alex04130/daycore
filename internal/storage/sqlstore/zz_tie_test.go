package sqlstore

import (
	"context"
	"testing"
	"time"

	"daycore/internal/domain"
)

func TestAffinityAndLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.Rhythm().Observe(ctx, "s1", "2026-07-27", 400); err != nil {
		t.Fatal(err)
	}
	var tf, tl, ts string
	if err := s.queryRow(ctx, `SELECT typeof(first_min), typeof(last_min), typeof(signals) FROM rhythm_days`).
		Scan(&tf, &tl, &ts); err != nil {
		t.Fatal(err)
	}
	t.Logf("rhythm_days affinity: first_min=%s last_min=%s signals=%s", tf, tl, ts)

	p := &domain.Proposal{SessionID: "s1", Title: "x", Level: domain.LevelL2, Kind: domain.KindCard,
		TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: time.Now().Add(time.Hour), Origin: domain.OriginDaemon}
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	var te, tc, tr string
	if err := s.queryRow(ctx, `SELECT typeof(expires_at), typeof(created_at), typeof(rev) FROM proposals`).
		Scan(&te, &tc, &tr); err != nil {
		t.Fatal(err)
	}
	t.Logf("proposals affinity: expires_at=%s created_at=%s rev=%s", te, tc, tr)

	t.Logf("itoa(100000000) = %q  itoa(250000000) = %q", itoa(100000000), itoa(250000000))
	got, err := s.Proposals().List(ctx, domain.ProposalFilter{SessionID: "s1", Limit: 100000000})
	if err != nil {
		t.Fatalf("List with a nine-digit limit: %v", err)
	}
	t.Logf("List(limit=100000000) returned %d rows (one exists)", len(got))

	// A NULL in a composite primary key.
	_, err = s.exec(ctx, `INSERT INTO rhythm_days (session_id, day, first_min, last_min, signals) VALUES (?, NULL, 0, 0, 1)`, "s2")
	t.Logf("NULL into rhythm_days.day: %v", err)
	_, err = s.exec(ctx, `INSERT INTO locale_overrides (message_key, locale, content, updated_at) VALUES (?, NULL, 'x', 0)`, "k")
	t.Logf("NULL into locale_overrides.locale: %v", err)
}
