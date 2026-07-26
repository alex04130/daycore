package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"daycore/internal/domain"
)

// GET /api/ops?limit= — list recent operation logs for the session.
func (s *Server) handleOpList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs, err := s.store.OpLogs().List(r.Context(), sid, limit)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取操作日志失败")
		return
	}
	if logs == nil {
		logs = []domain.OperationLog{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ops": logs})
}

// POST /api/ops/{id}/revert — undo one logged operation.
func (s *Server) handleOpRevert(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()

	orig, err := s.store.OpLogs().Get(ctx, sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "op_not_found", "找不到这条操作记录")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取操作记录失败")
		return
	}

	// Prevent double-revert.
	recent, _ := s.store.OpLogs().List(ctx, sid, 200)
	for _, l := range recent {
		if l.Action == "revert" && l.TargetID == id {
			s.writeErr(w, http.StatusConflict, "already_reverted", "这条操作已经被撤销过了")
			return
		}
	}

	var detail struct{ Before, After any }
	_ = json.Unmarshal([]byte(orig.Detail), &detail)

	switch orig.Action {
	case "plan_add":
		s.revertPlanAdd(ctx, w, sid, orig, detail)
	case "plan_update":
		s.revertPlanUpdate(ctx, w, sid, orig, detail)
	case "plan_remove":
		s.revertPlanRemove(ctx, w, sid, orig, detail)
	case "plan_upsert", "plan_autoplan":
		s.revertPlanUpsert(ctx, w, sid, orig, detail)
	case "rule_create":
		_ = s.store.Rules().Delete(ctx, sid, orig.TargetID)
		s.finishRevert(ctx, w, sid, orig)
	case "rule_update":
		s.revertRuleUpdate(ctx, w, sid, orig, detail)
	case "rule_delete":
		s.revertRuleCreate(ctx, w, sid, orig, detail)
	case "rule_batch":
		s.revertRuleBatch(ctx, w, sid, orig, detail)
	case "memory_add":
		_ = s.store.Memory().DeleteFact(ctx, sid, orig.TargetID)
		s.finishRevert(ctx, w, sid, orig)
	case "memory_delete":
		s.revertMemoryAdd(ctx, w, sid, orig, detail)
	case "memory_clear":
		s.revertMemoryClear(ctx, w, sid, orig, detail)
	default:
		s.writeErr(w, http.StatusBadRequest, "irreversible", fmt.Sprintf("操作 %s 不可撤销", orig.Action))
	}
}

func (s *Server) revertPlanAdd(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail struct{ Before, After any }) {
	if _, _, _, err := s.applyPlanPatch(ctx, sid, orig.Date, planAction{Action: "remove", Match: map[string]any{"id": orig.TargetID}}, domain.ActorSystem); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "撤销失败")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertPlanUpdate(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail struct{ Before, After any }) {
	blocks, _ := toBlockMaps(detail.Before)
	for _, b := range blocks {
		id, _ := b["id"].(string)
		ch := map[string]any{}
		for k, v := range b {
			if k != "id" && k != "rule_id" && k != "origin" && k != "hidden" {
				ch[k] = v
			}
		}
		if _, _, _, err := s.applyPlanPatch(ctx, sid, orig.Date, planAction{Action: "update", Match: map[string]any{"id": id}, Changes: ch}, domain.ActorSystem); err != nil {
			s.writeErr(w, http.StatusInternalServerError, "internal", "撤销失败")
			return
		}
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertPlanRemove(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail struct{ Before, After any }) {
	blocks, _ := toBlockMaps(detail.Before)
	for _, b := range blocks {
		if rid, _ := b["rule_id"].(string); rid != "" {
			// Un-tombstone a rule occurrence.
			if _, _, _, err := s.applyPlanPatch(ctx, sid, orig.Date, planAction{Action: "update", Match: map[string]any{"id": b["id"]}, Changes: map[string]any{"hidden": false}}, domain.ActorSystem); err != nil {
				s.writeErr(w, http.StatusInternalServerError, "internal", "撤销失败")
				return
			}
		} else {
			if _, _, _, err := s.applyPlanPatch(ctx, sid, orig.Date, planAction{Action: "add", Block: b}, domain.ActorSystem); err != nil {
				s.writeErr(w, http.StatusInternalServerError, "internal", "撤销失败")
				return
			}
		}
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertPlanUpsert(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail struct{ Before, After any }) {
	blocks, _ := toBlockMaps(detail.Before)
	if blocks == nil {
		blocks = []map[string]any{}
	}
	raw, _ := json.Marshal(blocks)
	var tb []domain.TimeBlock
	_ = json.Unmarshal(raw, &tb)
	_, err := s.store.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID: sid, Date: orig.Date, Blocks: tb, SourceType: "revert",
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "撤销失败")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertRuleUpdate(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail struct{ Before, After any }) {
	before, ok := detail.Before.(map[string]any)
	if !ok || before == nil {
		s.writeErr(w, http.StatusBadRequest, "irreversible", "缺少恢复快照")
		return
	}
	raw := map[string]json.RawMessage{}
	for k, v := range before {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	upd, err := ruleUpdateFromRaw(raw)
	if err != nil {
		s.writeErr(w, http.StatusBadRequest, "irreversible", fmt.Sprintf("规则恢复失败: %v", err))
		return
	}
	if _, err := s.store.Rules().Update(ctx, sid, orig.TargetID, upd); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "撤销失败")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertRuleCreate(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail struct{ Before, After any }) {
	var raw map[string]any
	b, _ := json.Marshal(detail.Before)
	_ = json.Unmarshal(b, &raw)
	if raw == nil {
		b, _ = json.Marshal(detail.After)
		_ = json.Unmarshal(b, &raw)
	}
	if raw == nil {
		s.writeErr(w, http.StatusBadRequest, "irreversible", "缺少规则数据")
		return
	}
	raw["source"] = "chat"
	input := ruleInput{}
	rb, _ := json.Marshal(raw)
	_ = json.Unmarshal(rb, &input)
	rule, err := input.toRule(sid)
	if err != nil {
		s.writeErr(w, http.StatusBadRequest, "irreversible", fmt.Sprintf("规则重建失败: %v", err))
		return
	}
	if _, err := s.store.Rules().Create(ctx, rule); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "撤销失败")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertRuleBatch(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail struct{ Before, After any }) {
	items, _ := detail.After.([]any)
	if items == nil {
		items, _ = detail.Before.([]any)
	}
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			if id, _ := m["id"].(string); id != "" {
				_ = s.store.Rules().Delete(ctx, sid, id)
			}
		}
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertMemoryAdd(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail struct{ Before, After any }) {
	var fact domain.MemoryFact
	b, _ := json.Marshal(detail.Before)
	_ = json.Unmarshal(b, &fact)
	if b == nil || fact.ID == "" {
		b, _ = json.Marshal(detail.After)
		_ = json.Unmarshal(b, &fact)
	}
	if fact.Fact == "" {
		s.writeErr(w, http.StatusBadRequest, "irreversible", "缺少记忆内容")
		return
	}
	_, _ = s.store.Memory().AddFact(ctx, &domain.MemoryFact{
		SessionID: sid, Fact: fact.Fact, Source: fact.Source,
	})
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertMemoryClear(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail struct{ Before, After any }) {
	items, _ := detail.Before.([]any)
	if items == nil {
		s.writeErr(w, http.StatusBadRequest, "irreversible", "缺少清空前快照")
		return
	}
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			fact, _ := m["fact"].(string)
			src, _ := m["source"].(string)
			if src == "" {
				src = "chat"
			}
			_, _ = s.store.Memory().AddFact(ctx, &domain.MemoryFact{
				SessionID: sid, Fact: fact, Source: src,
			})
		}
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) finishRevert(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog) {
	s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorSystem, Action: "revert",
		TargetID: orig.ID, Summary: orig.Action, Detail: marshalCompact(orig),
	})
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func toBlockMaps(v any) ([]map[string]any, bool) {
	raw, _ := json.Marshal(v)
	if raw == nil || string(raw) == "null" {
		return nil, true
	}
	var out []map[string]any
	err := json.Unmarshal(raw, &out)
	return out, err == nil
}
