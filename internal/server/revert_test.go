package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/domain"
)

// revertOp drives handleOpRevert for one op id and asserts it succeeded.
func revertOp(t *testing.T, s *Server, sid, opID string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/ops/"+opID+"/revert", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxSessionID, sid))
	req.SetPathValue("id", opID)
	s.handleOpRevert(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revert %s: status %d body %s", opID, rec.Code, rec.Body.String())
	}
}

func newTestRule(sid, title string) *domain.ScheduleRule {
	return &domain.ScheduleRule{
		SessionID: sid, Title: title, Type: domain.BlockTask, Kind: domain.RuleRecurring,
		Freq: domain.FreqDaily, Interval: 1, StartDate: "2026-01-01", Active: true,
		Source: "user", Timezone: "UTC", TimeMode: domain.TimeFloating,
	}
}

// Before the {before,after} Detail fix, reverting a rule_delete read a nil Before
// (the op stored a bare snapshot) and returned 400 — the rule was gone for good.
func TestRevertRuleDeleteRecreatesRule(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()
	rule, err := s.store.Rules().Create(ctx, newTestRule(sid, "Gym"))
	if err != nil {
		t.Fatal(err)
	}
	res := s.toolRuleRemove(ctx, sid, `{"id":"`+rule.ID+`"}`)
	if !res.OK {
		t.Fatalf("delete: %s", res.ErrMsg)
	}
	revertOp(t, s, sid, res.OpID)

	rules, _ := s.store.Rules().List(ctx, sid)
	found := false
	for _, r := range rules {
		if r.Title == "Gym" {
			found = true
		}
	}
	if !found {
		t.Fatal("rule_delete revert did not recreate the rule")
	}
}

// Before the fix, memory_delete revert read a nil Before and 400'd.
func TestRevertMemoryDeleteRestoresFact(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()
	fact, err := s.store.Memory().AddFact(ctx, &domain.MemoryFact{SessionID: sid, Fact: "likes tea", Source: "user"})
	if err != nil {
		t.Fatal(err)
	}
	res := s.toolMemoryRemove(ctx, sid, `{"id":"`+fact.ID+`"}`)
	if !res.OK {
		t.Fatalf("remove: %s", res.ErrMsg)
	}
	revertOp(t, s, sid, res.OpID)

	facts, _ := s.store.Memory().ListFacts(ctx, sid)
	found := false
	for _, f := range facts {
		if f.Fact == "likes tea" {
			found = true
		}
	}
	if !found {
		t.Fatal("memory_delete revert did not restore the fact")
	}
}

// A PATCH that would leave the rule inconsistent (kind=once without a date) must
// be rejected AND must not have mutated the stored row.
func TestRulePatchRejectsInvalidWithoutCorrupting(t *testing.T) {
	s, sid := newAgentTestServer(t)
	ctx := context.Background()
	rule, err := s.store.Rules().Create(ctx, newTestRule(sid, "Class"))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PATCH", "/api/rules/"+rule.ID, strings.NewReader(`{"kind":"once"}`))
	req = req.WithContext(context.WithValue(req.Context(), ctxSessionID, sid))
	req.SetPathValue("id", rule.ID)
	s.handleRulePatch(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (%s)", rec.Code, rec.Body.String())
	}
	got, err := s.store.Rules().Get(ctx, sid, rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != domain.RuleRecurring {
		t.Fatalf("rule was corrupted by a rejected patch: kind=%s", got.Kind)
	}
}
