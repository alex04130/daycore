package sqlstore

import (
	"math"
	"testing"
	"time"

	"daycore/internal/domain"
)

func TestProbeNaNSwallowsOps(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "card with a NaN arg")
	p.Ops = []domain.ProposalOp{{Tool: "plan_add", Args: map[string]any{"score": math.NaN()}}}
	p.Rows = []domain.ProposalRow{{ID: "r1", Label: "row one", Ops: []domain.ProposalOp{{Tool: "t", Args: map[string]any{"x": math.Inf(1)}}}}}
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatalf("Create returned err (would be fine): %v", err)
	}
	got, err := s.Proposals().Get(ctx, "s1", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PROBE ops=%v (nil=%v) rows=%v (nil=%v)", got.Ops, got.Ops == nil, got.Rows, got.Rows == nil)
	var raw string
	if err := s.queryRow(ctx, `SELECT ops_json FROM proposals WHERE id = ?`, p.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	t.Logf("PROBE ops_json column = %q", raw)
	var rawRows string
	_ = s.queryRow(ctx, `SELECT rows_json FROM proposals WHERE id = ?`, p.ID).Scan(&rawRows)
	t.Logf("PROBE rows_json column = %q", rawRows)
}

func TestProbeArgsNumberTypeDrift(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "int arg")
	p.Ops = []domain.ProposalOp{{Tool: "plan_add", Args: map[string]any{
		"minutes": 45,
		"big":     int64(9007199254740993),
	}}}
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Proposals().Get(ctx, "s1", p.ID)
	for k, v := range got.Ops[0].Args {
		t.Logf("PROBE arg %q -> %v of type %T", k, v, v)
	}
}

func TestProbeUpdateDropsThreadID(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "thread")
	p.ThreadID = "th-original"
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Proposals().Get(ctx, "s1", p.ID)
	got.ThreadID = "th-new"
	if err := s.Proposals().Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	back, _ := s.Proposals().Get(ctx, "s1", p.ID)
	t.Logf("PROBE threadID after Update = %q (wrote th-new)", back.ThreadID)
}

func TestProbeUpdateSkipsValidate(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "valid at birth")
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Proposals().Get(ctx, "s1", p.ID)
	got.ExpiresAt = time.Time{}              // zero — Validate would reject
	got.TTLPolicy = domain.TTLSilenceAccepts // act-first with no applied ops
	got.AppliedOpIDs = nil
	got.Title = ""
	if err := s.Proposals().Update(ctx, got); err != nil {
		t.Fatalf("Update rejected it (good): %v", err)
	}
	var ms int64
	_ = s.queryRow(ctx, `SELECT expires_at FROM proposals WHERE id = ?`, p.ID).Scan(&ms)
	t.Logf("PROBE expires_at after Update with zero time = %d", ms)
	back, _ := s.Proposals().Get(ctx, "s1", p.ID)
	t.Logf("PROBE back: title=%q ttl=%q applied=%v expires=%v", back.Title, back.TTLPolicy, back.AppliedOpIDs, back.ExpiresAt)
	n, err := s.Proposals().Expire(ctx, time.Now())
	t.Logf("PROBE Expire swept %d rows err=%v", n, err)
	back2, _ := s.Proposals().Get(ctx, "s1", p.ID)
	t.Logf("PROBE after sweep: state=%q resolution=%q", back2.State, back2.Resolution)
}

func TestProbeSupersedeUnknownKeeper(t *testing.T) {
	s, ctx := newStore(t)
	old := pending("s1", "older")
	old.MergeKey = "mk"
	if err := s.Proposals().Create(ctx, old); err != nil {
		t.Fatal(err)
	}
	n, err := s.Proposals().Supersede(ctx, "s1", "mk", "does-not-exist")
	t.Logf("PROBE Supersede with unknown keepID -> n=%d err=%v", n, err)
}

func TestProbeSupersedeSameMillisecond(t *testing.T) {
	s, ctx := newStore(t)
	a := pending("s1", "a")
	a.MergeKey = "mk"
	b := pending("s1", "b")
	b.MergeKey = "mk"
	if err := s.Proposals().Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Proposals().Create(ctx, b); err != nil {
		t.Fatal(err)
	}
	var ca, cb int64
	_ = s.queryRow(ctx, `SELECT created_at FROM proposals WHERE id = ?`, a.ID).Scan(&ca)
	_ = s.queryRow(ctx, `SELECT created_at FROM proposals WHERE id = ?`, b.ID).Scan(&cb)
	n, err := s.Proposals().Supersede(ctx, "s1", "mk", b.ID)
	t.Logf("PROBE created_at a=%d b=%d same=%v -> Supersede retired %d err=%v", ca, cb, ca == cb, n, err)
}

func TestProbeCreateLeavesSubMillisExpiry(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "sub-ms")
	p.ExpiresAt = time.Date(2026, 7, 27, 8, 0, 0, 123456789, time.UTC)
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Proposals().Get(ctx, "s1", p.ID)
	t.Logf("PROBE in-struct after Create = %v ; stored/read back = %v ; equal=%v",
		p.ExpiresAt, got.ExpiresAt, p.ExpiresAt.Equal(got.ExpiresAt))
}

func TestProbeClaimAttemptsStale(t *testing.T) {
	s, ctx := newStore(t)
	first := &domain.JobRun{SessionID: "s1", Job: domain.JobMorningBrief, RunKey: "k", Instance: "a"}
	if _, err := s.JobRuns().Claim(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := s.JobRuns().Finish(ctx, first.ID, domain.JobFailed, "boom", time.Now()); err != nil {
		t.Fatal(err)
	}
	second := &domain.JobRun{SessionID: "s1", Job: domain.JobMorningBrief, RunKey: "k", Instance: "b"}
	ok, err := s.JobRuns().Claim(ctx, second)
	if !ok || err != nil {
		t.Fatalf("takeover: %v %v", ok, err)
	}
	rows, _ := s.JobRuns().List(ctx, "s1", 5)
	t.Logf("PROBE run.Attempts in struct = %d ; row attempts = %d", second.Attempts, rows[0].Attempts)
}

func TestProbeNullTextColumnsPanicScan(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "titled")
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.exec(ctx, `UPDATE proposals SET title = NULL, summary = NULL WHERE id = ?`, p.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Proposals().Get(ctx, "s1", p.ID)
	t.Logf("PROBE Get with NULL title -> %v / err=%v", got, err)
}

func TestProbeZeroPointerTimes(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "zero ptr")
	z := time.Time{}
	p.PushedAt = &z
	p.DeliveredAt = &z
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Proposals().Get(ctx, "s1", p.ID)
	t.Logf("PROBE pointer-to-zero PushedAt came back %v / DeliveredAt %v", got.PushedAt, got.DeliveredAt)
}

func TestProbeSupersedeExactTie(t *testing.T) {
	s, ctx := newStore(t)
	a := pending("s1", "a")
	a.MergeKey = "mk"
	b := pending("s1", "b")
	b.MergeKey = "mk"
	_ = s.Proposals().Create(ctx, a)
	_ = s.Proposals().Create(ctx, b)
	// Force both into the same millisecond, which is what two daemons on two
	// instances produce on a fast machine.
	if _, err := s.exec(ctx, `UPDATE proposals SET created_at = 1000 WHERE session_id = ?`, "s1"); err != nil {
		t.Fatal(err)
	}
	n, err := s.Proposals().Supersede(ctx, "s1", "mk", b.ID)
	t.Logf("PROBE exact created_at tie -> Supersede retired %d err=%v (want 1)", n, err)
	list, _ := s.Proposals().List(ctx, domain.ProposalFilter{SessionID: "s1", State: domain.ProposalPending, Undelivered: true})
	t.Logf("PROBE undelivered pending survivors = %d", len(list))
}

func TestProbeObserveMidnightZero(t *testing.T) {
	s, ctx := newStore(t)
	for _, m := range []int{0, 500, 300} {
		if err := s.Rhythm().Observe(ctx, "s1", "2026-07-26", m); err != nil {
			t.Fatal(err)
		}
	}
	d, _ := s.Rhythm().Days(ctx, "s1", 5)
	t.Logf("PROBE first=%d last=%d signals=%d (want 0..500 over 3)", d[0].FirstMin, d[0].LastMin, d[0].Signals)
}

func TestProbeEmptyEnumStrings(t *testing.T) {
	s, ctx := newStore(t)
	p := pending("s1", "empty enums")
	p.BType = ""
	p.LockLevel = ""
	p.Resolution = ""
	p.Origin = ""
	if err := s.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Proposals().Get(ctx, "s1", p.ID)
	t.Logf("PROBE btype=%q lock=%q resolution=%q origin=%q", got.BType, got.LockLevel, got.Resolution, got.Origin)
}
