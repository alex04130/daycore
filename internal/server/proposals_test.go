package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daycore/internal/auth"
	"daycore/internal/domain"
)

func proposalTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, sid := newAgentTestServer(t)
	s.cookies = auth.NewCookieSigner("proposal-test-secret")
	return s, sid
}

func do(t *testing.T, s *Server, sid, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Session-Token", s.cookies.Sign(sid))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// 标记冲突 is the third way out of a lock refusal — for when neither taking
// leave nor unlocking is right, because the class really is immovable AND
// something else really does need that time.
//
// It creates a decision rather than resolving one: the system does not know
// which of the two matters more, and guessing is how an assistant moves the
// thing the user cared about.
func TestMarkConflictCreatesARealProposal(t *testing.T) {
	s, sid := proposalTestServer(t)
	ctx := context.Background()
	date := time.Now().In(s.planLocation()).AddDate(0, 0, 1).Format("2006-01-02")

	if _, err := s.store.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID: sid, Date: date, SourceType: "manual",
		Blocks: []domain.TimeBlock{
			{ID: "class", Title: "高等数学（课）", Type: domain.BlockAppointment,
				Time: hhmm("09:00"), DurationMin: mins(90), LockLevel: domain.LockHard},
			{ID: "lab", Title: "实验报告答辩", Type: domain.BlockAppointment,
				Time: hhmm("10:00"), DurationMin: mins(60)},
		},
	}); err != nil {
		t.Fatal(err)
	}

	rec := do(t, s, sid, http.MethodPost, "/api/plan/conflict", `{"date":"`+date+`","blockId":"class"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var p domain.Proposal
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.State != domain.ProposalPending || p.Origin != domain.OriginUser {
		t.Errorf("state=%q origin=%q, want pending/user", p.State, p.Origin)
	}
	if len(p.Rows) != 3 {
		t.Errorf("want three ways forward, got %d", len(p.Rows))
	}
	// The summary must name the contested span — "these two clash" is not
	// actionable, "both want 10:00–10:30" is.
	if !strings.Contains(p.Summary, "10:00") || !strings.Contains(p.Summary, "10:30") {
		t.Errorf("summary does not name the contested span: %q", p.Summary)
	}
	// A real row, not a card in a map that dies with the process.
	stored, err := s.store.Proposals().Get(ctx, sid, p.ID)
	if err != nil {
		t.Fatalf("the proposal was not persisted: %v", err)
	}
	if stored.DeliveredAt == nil {
		t.Error("a card the user is looking at should be marked delivered")
	}

	// It shows up in the stack.
	rec = do(t, s, sid, http.MethodGet, "/api/proposals", "")
	var list struct {
		Proposals []domain.Proposal `json:"proposals"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Proposals) != 1 || list.Proposals[0].ID != p.ID {
		t.Fatalf("stack = %d cards, want the one just created", len(list.Proposals))
	}

	// Marking it twice is one question, not two: the merge key retires the older.
	rec = do(t, s, sid, http.MethodPost, "/api/plan/conflict", `{"date":"`+date+`","blockId":"class"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("second mark: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, s, sid, http.MethodGet, "/api/proposals", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Proposals) != 1 {
		t.Errorf("marking the same clash twice left %d cards", len(list.Proposals))
	}
}

// A card claiming a conflict that is not there would teach the user to distrust
// the cards, so the marker refuses rather than inventing one.
func TestMarkConflictRefusesToInventOne(t *testing.T) {
	s, sid := proposalTestServer(t)
	date := time.Now().In(s.planLocation()).AddDate(0, 0, 1).Format("2006-01-02")
	if _, err := s.store.DayPlans().Upsert(context.Background(), &domain.DayPlan{
		SessionID: sid, Date: date, SourceType: "manual",
		Blocks: []domain.TimeBlock{
			{ID: "a", Title: "课", Time: hhmm("09:00"), DurationMin: mins(60)},
			{ID: "b", Title: "另一件", Time: hhmm("10:00"), DurationMin: mins(60)}, // back to back
		},
	}); err != nil {
		t.Fatal(err)
	}
	rec := do(t, s, sid, http.MethodPost, "/api/plan/conflict", `{"date":"`+date+`","blockId":"a"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("back-to-back is not a conflict; status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProposalRespond(t *testing.T) {
	s, sid := proposalTestServer(t)
	ctx := context.Background()
	p := &domain.Proposal{
		ID: "p1", SessionID: sid, State: domain.ProposalPending,
		Level: domain.LevelL2, Kind: domain.KindCard, Title: "两件事撞在一起了",
		TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: time.Now().Add(time.Hour),
		Origin: domain.OriginUser,
		Rows: []domain.ProposalRow{
			{ID: "move_other", Label: "把另一件挪开", State: domain.ProposalPending},
			{ID: "skip_this", Label: "这次不去", State: domain.ProposalPending},
		},
	}
	if err := s.store.Proposals().Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	rec := do(t, s, sid, http.MethodPost, "/api/proposals/p1/respond", `{"choice":"skip_this"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got, err := s.store.Proposals().Get(ctx, sid, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.ProposalAccepted || got.Resolution != domain.ResolutionUser {
		t.Errorf("state=%q resolution=%q", got.State, got.Resolution)
	}
	// Row-level: the chosen one accepted, the others explicitly rejected — not
	// left pending, which would read as "still deciding".
	for _, r := range got.Rows {
		want := domain.ProposalRejected
		if r.ID == "skip_this" {
			want = domain.ProposalAccepted
		}
		if r.State != want {
			t.Errorf("row %s = %q, want %q", r.ID, r.State, want)
		}
	}

	// Answering twice is a conflict, not a silent success and not a 500.
	if rec := do(t, s, sid, http.MethodPost, "/api/proposals/p1/respond", `{"choice":"move_other"}`); rec.Code != http.StatusConflict {
		t.Errorf("second answer: status %d, want 409", rec.Code)
	}
	if rec := do(t, s, sid, http.MethodPost, "/api/proposals/nope/respond", `{"choice":"x"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown card: status %d, want 404", rec.Code)
	}
	// Another session must not be able to answer this one.
	other := "someone-else"
	if _, err := s.store.Sessions().GetOrCreate(ctx, other); err != nil {
		t.Fatal(err)
	}
	if rec := do(t, s, other, http.MethodPost, "/api/proposals/p1/respond", `{"choice":"x"}`); rec.Code != http.StatusNotFound {
		t.Errorf("cross-session answer: status %d, want 404", rec.Code)
	}
}

// The stack is a conjunction, not a question about deliveredAt — that stamp is
// permanent, so asking about it alone would return every card ever shown and
// the ≤3 budget would fill up for good.
func TestProposalStackExcludesSettledAndLapsed(t *testing.T) {
	s, sid := proposalTestServer(t)
	ctx := context.Background()
	now := time.Now()
	delivered := now.Add(-time.Hour)

	mk := func(id string, state domain.ProposalState, expires time.Time) {
		t.Helper()
		p := &domain.Proposal{
			ID: id, SessionID: sid, State: state, Level: domain.LevelL2, Kind: domain.KindCard,
			Title: id, TTLPolicy: domain.TTLSilenceRejects, ExpiresAt: expires,
			Origin: domain.OriginUser, DeliveredAt: &delivered,
		}
		if state != domain.ProposalPending {
			p.Resolution = domain.ResolutionUser
		}
		if err := s.store.Proposals().Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	mk("live", domain.ProposalPending, now.Add(time.Hour))
	mk("answered", domain.ProposalAccepted, now.Add(time.Hour))
	mk("lapsed", domain.ProposalPending, now.Add(-time.Minute))

	rec := do(t, s, sid, http.MethodGet, "/api/proposals", "")
	var list struct {
		Proposals []domain.Proposal `json:"proposals"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Proposals) != 1 || list.Proposals[0].ID != "live" {
		ids := []string{}
		for _, p := range list.Proposals {
			ids = append(ids, p.ID)
		}
		t.Errorf("stack = %v, want just [live]", ids)
	}
	// The console view sees all three.
	rec = do(t, s, sid, http.MethodGet, "/api/proposals?all=1", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Proposals) != 3 {
		t.Errorf("all=1 returned %d, want 3", len(list.Proposals))
	}
}

// A decision card is now a projection of a row, not a thing that exists only
// in one process's memory. Until it was, a restart lost every question in
// flight and nothing about them ever reached the ledger — and the product's
// own semantics say ghosts, cards and decision cards are ONE resource.
func TestDecisionCardIsPersistedAndSettled(t *testing.T) {
	s, sid := proposalTestServer(t)
	ctx := context.Background()

	s.persistDecision(ctx, sid, "dc_1", "", "排到哪天？", "周三和周四都空着",
		[]decisionOption{{ID: "wed", Label: "周三"}, {ID: "thu", Label: "周四"}},
		45*time.Second)

	p, err := s.store.Proposals().Get(ctx, sid, "dc_1")
	if err != nil {
		t.Fatalf("the decision card never reached the table: %v", err)
	}
	if p.Kind != domain.KindDecision || p.Origin != domain.OriginAgent {
		t.Errorf("kind=%q origin=%q, want decision/agent", p.Kind, p.Origin)
	}
	// Ask-first: nothing has been done, so silence must void it rather than
	// confirm it. Backwards, an unanswered card would execute.
	if p.TTLPolicy != domain.TTLSilenceRejects {
		t.Errorf("ttlPolicy = %q, want silence_rejects", p.TTLPolicy)
	}
	if p.DeliveredAt == nil {
		t.Error("a card on screen is delivered")
	}
	if len(p.Rows) != 2 {
		t.Fatalf("options did not become rows: %+v", p.Rows)
	}

	s.settleDecision(ctx, sid, "dc_1", "thu")
	p, _ = s.store.Proposals().Get(ctx, sid, "dc_1")
	if p.State != domain.ProposalAccepted || p.Resolution != domain.ResolutionUser {
		t.Errorf("after an answer: state=%q resolution=%q", p.State, p.Resolution)
	}
	for _, r := range p.Rows {
		want := domain.ProposalRejected
		if r.ID == "thu" {
			want = domain.ProposalAccepted
		}
		if r.State != want {
			t.Errorf("row %s = %q, want %q", r.ID, r.State, want)
		}
	}
}

// Timeout and cancellation are both silence, not refusal. Consensus 15 keeps
// "nobody ever looked" distinguishable from "they said no" — and a card the
// user never answered was never looked at.
func TestUnansweredDecisionIsSilenceNotRejection(t *testing.T) {
	s, sid := proposalTestServer(t)
	ctx := context.Background()
	for _, c := range []struct{ id, choice string }{{"dc_t", "timeout"}, {"dc_c", "cancelled"}} {
		s.persistDecision(ctx, sid, c.id, "", "排到哪天？", "",
			[]decisionOption{{ID: "wed", Label: "周三"}}, time.Minute)
		s.settleDecision(ctx, sid, c.id, c.choice)
		p, err := s.store.Proposals().Get(ctx, sid, c.id)
		if err != nil {
			t.Fatal(err)
		}
		if p.State != domain.ProposalExpired || p.Resolution != domain.ResolutionSilence {
			t.Errorf("%s: state=%q resolution=%q, want expired/silence", c.choice, p.State, p.Resolution)
		}
	}
}
