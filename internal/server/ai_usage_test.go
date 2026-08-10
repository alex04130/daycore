package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"daycore/internal/domain"
)

// The overview's AI figures are TOTALS, and they survive the prune.
//
// This is the whole reason the rollup exists. Before it, aiCalls and tokenUsed
// were COUNT and SUM over a table pruned at ninety days — so the console showed
// a window figure under a label that said total, and it went DOWN as the window
// slid. Nothing reported that, because nothing could: the number was still a
// correct answer to a question nobody had asked.
func TestTheOverviewTotalsSurviveThePrune(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	repo := s.store.AILogs()

	// One call on a day far enough back that the prune will take its row.
	old := &domain.AICallLog{
		SessionID: "old", Endpoint: epCompanion, Model: "glm-5",
		Status: domain.AICallStatusOK, PromptTokens: 120, CompTokens: 30,
	}
	if err := repo.Add(ctx, old); err != nil {
		t.Fatal(err)
	}

	// The job, in the order the job runs it. `today` is passed in rather than
	// read from the clock precisely so a test can close a day — Add stamps
	// created_at itself, so there is no other way to produce a closed day
	// without a backdating hook in production code.
	tomorrow := domain.UTCDay(time.Now().AddDate(0, 0, 1))
	if _, err := repo.RollUpUsage(ctx, tomorrow); err != nil {
		t.Fatal(err)
	}
	// …and then the ledger is emptied under it, which is what the prune does
	// ninety days later.
	if _, err := repo.Prune(ctx, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if rows, _ := repo.List(ctx, domain.AILogFilter{}, 0); len(rows) != 0 {
		t.Fatalf("the ledger still has %d rows; the prune did not run", len(rows))
	}

	stats := readStats(t, s)
	if stats.AICalls != 1 {
		t.Errorf("aiCalls = %d after the row was pruned, want 1 — the total is still coming from the ledger", stats.AICalls)
	}
	if stats.TokenUsed != 30 || stats.PromptTokens != 120 {
		t.Errorf("tokens = %d out / %d in, want 30 / 120", stats.TokenUsed, stats.PromptTokens)
	}
	// The date the count starts from has to be on the wire, or the screen has no
	// honest way to label a figure that does not begin at the beginning of time.
	if stats.Since == "" {
		t.Error("stats carries no `since`; a total with no start date is the same lie the rollup fixes")
	}
	// And the ledger's own size stays available as its own fact — it answers a
	// different question ("how much disk") and must not be confused with the total.
	if stats.LedgerRows != 0 {
		t.Errorf("ledgerRows = %d, want 0 after the prune", stats.LedgerRows)
	}
}

// The job folds before it prunes, and a swap is caught here.
//
// ⚠️ This is the assertion behind the one comment in leader.go that matters.
// The prune deletes the rows the fold reads, so running it first loses that day
// forever — with no error anywhere, and the only symptom a total that is lower
// than it was yesterday. Nothing else in the suite exercises the job body: the
// other tests call the two repository methods in the right order by hand, which
// proves the repository works and proves nothing about the caller.
func TestTheJobFoldsBeforeItPrunes(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	l := &domain.AICallLog{
		SessionID: "job", Endpoint: epCompanion, Model: "glm-5",
		Status: domain.AICallStatusOK, PromptTokens: 40, CompTokens: 9,
	}
	if err := s.store.AILogs().Add(ctx, l); err != nil {
		t.Fatal(err)
	}

	// A boundary INSIDE today prunes nothing, because the job aligns it back to
	// the start of its day. That is the invariant, not an accident: an unaligned
	// prune leaves a day half present, and the fold would then recompute that
	// day from the survivors and write a smaller number over the right one.
	s.foldAndPruneAILogs(ctx, domain.UTCDay(time.Now().AddDate(0, 0, 1)), time.Now().Add(time.Minute))
	if rows, _ := s.store.AILogs().List(ctx, domain.AILogFilter{}, 0); len(rows) != 1 {
		t.Fatalf("a mid-day prune boundary took %d of today's rows; it must align back to the day start",
			1-len(rows))
	}

	// A boundary in the NEXT day takes the whole of today, which is what the
	// real job does ninety days later.
	s.foldAndPruneAILogs(ctx, domain.UTCDay(time.Now().AddDate(0, 0, 1)), time.Now().AddDate(0, 0, 1).Add(time.Minute))
	if rows, _ := s.store.AILogs().List(ctx, domain.AILogFilter{}, 0); len(rows) != 0 {
		t.Fatalf("the prune did not run; %d ledger rows left", len(rows))
	}
	stats := readStats(t, s)
	if stats.AICalls != 1 || stats.PromptTokens != 40 || stats.TokenUsed != 9 {
		t.Errorf("the job pruned before it folded: %+v — that day is gone for good", stats)
	}

	// And a fold that FAILS must stop the job rather than fall through.
	//
	// Provoked with an unparseable day rather than a broken database, because
	// the code path is the same one — `err != nil` → return — and a wrapper
	// store that fails on command would be forty lines to reach the same line.
	// What is being pinned is the `return`, not the cause.
	l2 := &domain.AICallLog{
		SessionID: "job2", Endpoint: epCompanion, Model: "glm-5",
		Status: domain.AICallStatusOK, PromptTokens: 3, CompTokens: 1,
	}
	if err := s.store.AILogs().Add(ctx, l2); err != nil {
		t.Fatal(err)
	}
	s.foldAndPruneAILogs(ctx, "not-a-day", time.Now().Add(time.Minute))
	rows, _ := s.store.AILogs().List(ctx, domain.AILogFilter{}, 0)
	if len(rows) != 1 {
		t.Errorf("a failed fold still pruned: %d ledger rows left, want the 1 that was never folded.\n"+
			"  Skipping a prune costs disk; pruning after a failed fold costs the history.", len(rows))
	}
}

// Today counts, and counts exactly once.
//
// The rollup deliberately stops at yesterday, so today's calls are added live.
// The failure this guards is the one that only appears after the job runs: if
// today were ever folded AND added live, every figure would silently double for
// the rest of the day.
func TestTodayIsCountedOnceNotTwice(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	repo := s.store.AILogs()

	for i := 0; i < 3; i++ {
		l := &domain.AICallLog{
			SessionID: "now", Endpoint: epCompanion, Model: "glm-5",
			Status: domain.AICallStatusOK, PromptTokens: 10, CompTokens: 2,
		}
		if err := repo.Add(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	before := readStats(t, s)
	if before.AICalls != 3 {
		t.Fatalf("today's three calls counted as %d", before.AICalls)
	}

	// Run the fold. Today must not be touched by it.
	if n, err := repo.RollUpUsage(ctx, domain.UTCDay(time.Now())); err != nil || n != 0 {
		t.Fatalf("the fold wrote %d days with only today's rows present (err=%v)", n, err)
	}
	after := readStats(t, s)
	if after.AICalls != 3 {
		t.Errorf("after folding, today's calls count as %d — today is being counted twice", after.AICalls)
	}
	if after.TokenUsed != before.TokenUsed || after.PromptTokens != before.PromptTokens {
		t.Errorf("token counts moved across a fold that wrote nothing: %+v then %+v", before, after)
	}
}

// The usage endpoint is behind the ledger's permission, not the overview's.
//
// overview.read is the permission everybody with a console session gets. A
// per-model cost breakdown is not "is it up".
func TestSpendHistoryIsNotBehindOverviewRead(t *testing.T) {
	s := adminServer(t)
	makeAdminUser(t, s, "watcher", []string{PermOverview})
	makeAdminUser(t, s, "auditor", []string{PermAILogsRead})

	if rec := adminReq(t, s, http.MethodGet, "/api/admin/usage", "", withAdminCookie(t, s, "watcher")); rec.Code != http.StatusForbidden {
		t.Errorf("overview.read opened the spend breakdown: %d", rec.Code)
	}
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/usage", "", withAdminCookie(t, s, "auditor")); rec.Code != http.StatusOK {
		t.Errorf("ailogs.read could not read the spend breakdown: %d %s", rec.Code, rec.Body)
	}
}

// The usage endpoint reports both folds and says where it stops.
func TestUsageReportsBothFoldsAndItsHorizon(t *testing.T) {
	s := adminServer(t)
	ctx := context.Background()
	repo := s.store.AILogs()

	for _, model := range []string{"glm-5", "glm-5", "deepseek-v4"} {
		l := &domain.AICallLog{
			SessionID: "back", Endpoint: epBrief, Model: model,
			Status: domain.AICallStatusOK, PromptTokens: 5, CompTokens: 1,
		}
		if err := repo.Add(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	// Close today by telling the fold that tomorrow has begun.
	now := time.Now()
	if _, err := repo.RollUpUsage(ctx, domain.UTCDay(now.AddDate(0, 0, 1))); err != nil {
		t.Fatal(err)
	}

	rec := adminReq(t, s, http.MethodGet, "/api/admin/usage", "", withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("usage: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		Totals     domain.AIUsageTotals `json:"totals"`
		Days       []domain.AIUsageDay  `json:"days"`
		ByModel    []domain.AIUsageDay  `json:"byModel"`
		ThroughDay string               `json:"throughDay"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Totals.Calls != 3 {
		t.Errorf("totals = %+v, want 3 calls", body.Totals)
	}
	if len(body.Days) != 1 || body.Days[0].Calls != 3 || body.Days[0].Day != domain.UTCDay(now) {
		t.Errorf("day series = %+v, want one day (%s) of 3", body.Days, domain.UTCDay(now))
	}
	if len(body.ByModel) != 2 {
		t.Errorf("model fold = %+v, want two models", body.ByModel)
	}
	// The horizon is stated. Without it, "today is missing" is the first bug
	// report this screen gets.
	// ⚠️ throughDay is computed from the real clock, so it is YESTERDAY even
	// though this test folded today by hand. That is the point of asserting it:
	// the endpoint states the newest day it can ever cover, and it must not
	// infer that from whatever happens to be in the table.
	if body.ThroughDay != domain.UTCDay(now.AddDate(0, 0, -1)) {
		t.Errorf("throughDay = %q, want yesterday (%q)", body.ThroughDay, domain.UTCDay(now.AddDate(0, 0, -1)))
	}
	// A malformed window is refused rather than silently matching nothing.
	if rec := adminReq(t, s, http.MethodGet, "/api/admin/usage?from=last-tuesday", "", withRootHeader(s)); rec.Code != http.StatusBadRequest {
		t.Errorf("a malformed `from` answered %d, want 400", rec.Code)
	}
}

type statsBody struct {
	AICalls      int64  `json:"aiCalls"`
	AIErrors     int64  `json:"aiErrors"`
	TokenUsed    int64  `json:"tokenUsed"`
	PromptTokens int64  `json:"promptTokens"`
	Since        string `json:"since"`
	LedgerRows   int64  `json:"ledgerRows"`
}

func readStats(t *testing.T, s *Server) statsBody {
	t.Helper()
	rec := adminReq(t, s, http.MethodGet, "/api/admin/stats", "", withRootHeader(s))
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: %d %s", rec.Code, rec.Body)
	}
	var out statsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
