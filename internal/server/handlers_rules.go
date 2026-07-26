package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"daycore/internal/domain"
)

// GET /api/rules — every rule (active and paused) for the session.
func (s *Server) handleRuleList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	rules, err := s.store.Rules().List(r.Context(), sid)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取规则失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

// POST /api/rules — create one rule.
func (s *Server) handleRuleCreate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var in ruleInput
	if err := s.readJSON(r, &in); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	rule, err := in.toRule(sid)
	if err != nil {
		s.writeErr(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	created, err := s.store.Rules().Create(r.Context(), rule)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "规则保存失败")
		return
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "rule_create", TargetID: created.ID,
		Summary: created.Title, Detail: marshalCompact(created),
	})
	s.writeJSON(w, http.StatusOK, created)
}

// POST /api/rules/batch — create many rules at once (ICS import / timetable
// extraction confirmations). All-or-nothing validation, best-effort insert.
func (s *Server) handleRuleBatchCreate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Rules []ruleInput `json:"rules"`
	}
	if err := s.readJSON(r, &body); err != nil || len(body.Rules) == 0 {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 rules")
		return
	}
	rules := make([]*domain.ScheduleRule, 0, len(body.Rules))
	for i, in := range body.Rules {
		rule, err := in.toRule(sid)
		if err != nil {
			s.writeErr(w, http.StatusBadRequest, "invalid_rule", fmt.Sprintf("rules[%d]: %s", i, err))
			return
		}
		rules = append(rules, rule)
	}
	created := []domain.ScheduleRule{}
	for _, rule := range rules {
		c, err := s.store.Rules().Create(r.Context(), rule)
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, "internal", "规则保存失败")
			return
		}
		created = append(created, *c)
	}
	// Confirmed ICS/timetable-screenshot candidates land here — archive them so
	// the import history stays complete (direct ICS saves are archived in
	// handleImportICS instead).
	if len(created) > 0 && (created[0].Source == "ics" || created[0].Source == "image") {
		s.recordImport(r.Context(), sid, created[0].Source, len(created),
			fmt.Sprintf("%d rules confirmed", len(created)), marshalCompact(created))
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "rule_batch",
		Summary: fmt.Sprintf("%d rules", len(created)), Detail: marshalCompact(map[string]any{"after": created}),
	})
	s.writeJSON(w, http.StatusOK, map[string]any{"rules": created, "created": len(created)})
}

// PATCH /api/rules/{id} — partial update. Field presence is detected from the
// raw JSON so `"time": null` (clear) differs from the field being absent.
func (s *Server) handleRulePatch(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var raw map[string]json.RawMessage
	if err := s.readJSON(r, &raw); err != nil || len(raw) == 0 {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	upd, err := ruleUpdateFromRaw(raw)
	if err != nil {
		s.writeErr(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	prev, err := s.store.Rules().Get(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "rule_not_found", "没有这条规则")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取规则失败")
		return
	}
	// Validate the merged result BEFORE persisting: the combined state (e.g. kind
	// flipped to once without a date) must be rejected without corrupting the row.
	candidate := applyRuleUpdate(*prev, upd)
	if err := validateRule(&candidate); err != nil {
		s.writeErr(w, http.StatusBadRequest, "invalid_rule", err.Error())
		return
	}
	updated, err := s.store.Rules().Update(r.Context(), sid, id, upd)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "规则更新失败")
		return
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "rule_update", TargetID: id,
		Summary: updated.Title,
		Detail:  marshalCompact(map[string]any{"before": prev, "after": updated}),
	})
	s.writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/rules/{id}
func (s *Server) handleRuleDelete(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	prev, _ := s.store.Rules().Get(r.Context(), sid, id) // recovery snapshot for the audit log
	err := s.store.Rules().Delete(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "rule_not_found", "没有这条规则")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "规则删除失败")
		return
	}
	summary := id
	if prev != nil {
		summary = prev.Title
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "rule_delete", TargetID: id,
		Summary: summary, Detail: marshalCompact(map[string]any{"before": prev}),
	})
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
