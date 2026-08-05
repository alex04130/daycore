package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"daycore/internal/ai"
	"daycore/internal/config"
	"daycore/internal/domain"

	_ "daycore/internal/ai/formats/openai" // register the wire format for LoadCatalog
)

// ── keep_manual: the server, not the client, guarantees the flip ────────────

func patchOne(t *testing.T, s *Server, sid, date, blockID, actor string) {
	t.Helper()
	_, _, matched, err := s.applyPlanPatch(context.Background(), sid, date, planAction{
		Action:  "update",
		Match:   map[string]any{"id": blockID},
		Changes: map[string]any{"title": "改过了"},
	}, actor)
	if err != nil || matched != 1 {
		t.Fatalf("patch (actor=%s): matched=%d err=%v", actor, matched, err)
	}
}

func planOrigin(t *testing.T, s *Server, sid, date string) string {
	t.Helper()
	p, err := s.store.DayPlans().Get(context.Background(), sid, date)
	if err != nil || len(p.Blocks) != 1 {
		t.Fatalf("plan: %v blocks=%d", err, len(p.Blocks))
	}
	return p.Blocks[0].Origin
}

func seedAutoPlan(t *testing.T, s *Server, sid, date string) {
	t.Helper()
	_, err := s.store.DayPlans().Upsert(context.Background(), &domain.DayPlan{
		SessionID: sid, Date: date, SourceType: "auto",
		Blocks: []domain.TimeBlock{{ID: "b1", Title: "自习", Origin: domain.OriginAuto, Type: domain.BlockTask}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestKeepManualFlipIsServerSide(t *testing.T) {
	s, sid := newAgentTestServer(t)

	// The flagship flow: the agent edits an auto block in chat. Without the
	// server-side flip this block was fair game for the next regeneration.
	seedAutoPlan(t, s, sid, "2026-08-06")
	patchOne(t, s, sid, "2026-08-06", "b1", domain.ActorAgent)
	if got := planOrigin(t, s, sid, "2026-08-06"); got != domain.OriginManual {
		t.Fatalf("after agent edit, origin = %q, want manual", got)
	}

	// A user edit from any HTTP client flips too.
	seedAutoPlan(t, s, sid, "2026-08-07")
	patchOne(t, s, sid, "2026-08-07", "b1", domain.ActorUser)
	if got := planOrigin(t, s, sid, "2026-08-07"); got != domain.OriginManual {
		t.Fatalf("after user edit, origin = %q, want manual", got)
	}

	// System writes (revert, regeneration) never flip.
	seedAutoPlan(t, s, sid, "2026-08-08")
	patchOne(t, s, sid, "2026-08-08", "b1", domain.ActorSystem)
	if got := planOrigin(t, s, sid, "2026-08-08"); got != domain.OriginAuto {
		t.Fatalf("system write flipped origin to %q", got)
	}
}

// ── PATCH validation ─────────────────────────────────────────────────────────

func patchPlanReq(t *testing.T, s *Server, sid, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/plan", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxSessionID, sid))
	s.handlePlanPatch(rec, req)
	return rec
}

func TestPlanPatchRejectsEmptyMatch(t *testing.T) {
	s, sid := newAgentTestServer(t)
	seedAutoPlan(t, s, sid, "2026-08-06")

	rec := patchPlanReq(t, s, sid, `{"date":"2026-08-06","action":{"action":"remove","match":{}}}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "empty_match") {
		t.Fatalf("empty match: code=%d body=%s", rec.Code, rec.Body.String())
	}
	// The day must be untouched.
	if got := planOrigin(t, s, sid, "2026-08-06"); got != domain.OriginAuto {
		t.Fatalf("plan was mutated despite the refusal: %q", got)
	}
}

func TestPlanPatchRejectsUnknownAction(t *testing.T) {
	s, sid := newAgentTestServer(t)
	rec := patchPlanReq(t, s, sid, `{"date":"2026-08-06","action":{"action":"shuffle"}}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown_action") {
		t.Fatalf("unknown action: code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// ── autoplan: ledger for every regenerated day, and empty means empty ────────

// fakeOpenAI serves one canned chat-completion response per call from script.
func fakeOpenAI(t *testing.T, script []string) *httptest.Server {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := calls
		if i >= len(script) {
			i = len(script) - 1
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%s},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":11,"completion_tokens":7}}`, script[i])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newAutoPlanServer(t *testing.T, srv *httptest.Server) (*Server, string) {
	t.Helper()
	s, sid := newAgentTestServer(t)

	catalogPath := t.TempDir() + "/models.yaml"
	yaml := "models:\n  - id: fake\n    format: openai\n    base_url: " + srv.URL + "\n    model: fake\n"
	if err := os.WriteFile(catalogPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := ai.LoadCatalog(catalogPath, "fake", "", "")
	if err != nil {
		t.Fatal(err)
	}
	prompts, err := ai.NewPromptService(s.store.Prompts())
	if err != nil {
		t.Fatal(err)
	}
	s2 := New(Deps{
		Config:  &config.Config{AgentMaxRounds: 3, RateLimitPerMin: 0, AutoPlanMaxDays: 7, AIRequestTimeout: 10 * time.Second},
		Store:   s.store,
		Catalog: cat,
		Prompts: prompts,
		Logger:  s.log,
	})
	return s2, sid
}

func autoPlanReq(t *testing.T, s *Server, sid, from, to string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	body := fmt.Sprintf(`{"mode":"replace_all","from":%q,"to":%q,"timezone":"UTC"}`, from, to)
	req := httptest.NewRequest("POST", "/api/ai/auto-plan", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), ctxSessionID, sid))
	s.handleAIAutoPlan(rec, req)
	return rec
}

func autoPlanOps(t *testing.T, s *Server, sid string) []domain.OperationLog {
	t.Helper()
	ops, err := s.store.OpLogs().List(context.Background(), sid, 50)
	if err != nil {
		t.Fatal(err)
	}
	out := ops[:0]
	for _, o := range ops {
		if o.Action == "plan_autoplan" {
			out = append(out, o)
		}
	}
	return out
}

func TestAutoPlanLogsFirstTimeDates(t *testing.T) {
	srv := fakeOpenAI(t, []string{
		`"{\"blocks\":[{\"date\":\"2026-08-06\",\"time\":\"09:00\",\"title\":\"自习\",\"type\":\"task\"}]}"`,
	})
	s, sid := newAutoPlanServer(t, srv)

	rec := autoPlanReq(t, s, sid, "2026-08-06", "2026-08-06")
	if rec.Code != http.StatusOK {
		t.Fatalf("auto-plan: %d %s", rec.Code, rec.Body.String())
	}
	// A first-time plan (no stored plan before) used to skip the ledger —
	// unlogged writes cannot be reverted.
	if ops := autoPlanOps(t, s, sid); len(ops) != 1 {
		t.Fatalf("plan_autoplan ops = %d, want 1", len(ops))
	}
	// And the model call itself is in the AI ledger.
	stats, err := s.store.AILogs().Stats(context.Background())
	if err != nil || stats.AICalls != 1 {
		t.Fatalf("ai calls = %+v err=%v", stats, err)
	}
}

func TestAutoPlanEmptyResultClearsOldPlan(t *testing.T) {
	srv := fakeOpenAI(t, []string{
		`"{\"blocks\":[{\"date\":\"2026-08-06\",\"time\":\"09:00\",\"title\":\"自习\",\"type\":\"task\"}]}"`,
		`"{\"blocks\":[]}"`,
	})
	s, sid := newAutoPlanServer(t, srv)

	if rec := autoPlanReq(t, s, sid, "2026-08-06", "2026-08-06"); rec.Code != http.StatusOK {
		t.Fatalf("first: %d %s", rec.Code, rec.Body.String())
	}
	if rec := autoPlanReq(t, s, sid, "2026-08-06", "2026-08-06"); rec.Code != http.StatusOK {
		t.Fatalf("second: %d %s", rec.Code, rec.Body.String())
	}
	// replace_all with an empty model answer must empty the stored plan —
	// before the fix the old auto blocks just stayed there.
	p, err := s.store.DayPlans().Get(context.Background(), sid, "2026-08-06")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Blocks) != 0 {
		t.Fatalf("blocks after empty regeneration = %d, want 0", len(p.Blocks))
	}
	if ops := autoPlanOps(t, s, sid); len(ops) != 2 {
		t.Fatalf("plan_autoplan ops = %d, want 2", len(ops))
	}
}

// ── the AI ledger sees the companion loop too ────────────────────────────────

func TestCompanionRoundLandsInAICallLog(t *testing.T) {
	s, sid := newAgentTestServer(t)
	fp := &fakeProvider{script: []*ai.ChatResponse{
		{Content: "好的", Usage: ai.Usage{PromptTokens: 10, CompletionTokens: 5}},
	}}
	runAgent(t, s, fp, sid, "你好")

	stats, err := s.store.AILogs().Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.AICalls != 1 {
		t.Fatalf("ai calls = %d, want 1", stats.AICalls)
	}
	if stats.TokenUsed != 5 {
		t.Fatalf("tokens = %d, want 5 (TokenUsed sums completion tokens)", stats.TokenUsed)
	}
}
