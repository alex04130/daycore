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

// revertDetail is the before/after snapshot every logged operation carries.
// Reversal is compensation, not a rollback: the ledger is append-only, so undoing
// something means writing the inverse from this snapshot and logging that too.
type revertDetail struct{ Before, After any }

// revertHandler applies the inverse of one action.
type revertHandler func(s *Server, ctx context.Context, w http.ResponseWriter,
	sid string, orig *domain.OperationLog, detail revertDetail)

// revertHandlers maps an action to its inverse.
//
// This was a switch, and a switch is the wrong shape for it: every feature that
// gains a write has to come back and edit one function in one file, so a batch
// of parallel work on unrelated features all collides here. A map that features
// register into lets the inverse live next to the code that writes the operation
// — which is also where someone will look for it.
//
// Registration happens from init(), so a feature file that is linked in is a
// feature whose writes are undoable. Forgetting to register is not a silent
// gap: handleOpRevert refuses with "irreversible" and says which action.
var revertHandlers = map[string]revertHandler{}

// registerRevert declares the inverse of an action. Registering the same action
// twice panics — two inverses for one action means one of them is dead code, and
// which one wins would depend on link order.
func registerRevert(action string, h revertHandler) {
	if _, dup := revertHandlers[action]; dup {
		panic("server: duplicate revert handler for " + action)
	}
	revertHandlers[action] = h
}

func init() {
	registerRevert("plan_add", (*Server).revertPlanAdd)
	registerRevert("plan_update", (*Server).revertPlanUpdate)
	registerRevert("plan_remove", (*Server).revertPlanRemove)
	registerRevert("plan_upsert", (*Server).revertPlanUpsert)
	registerRevert("plan_autoplan", (*Server).revertPlanUpsert)
	registerRevert("rule_create", (*Server).revertRuleCreate_delete)
	registerRevert("rule_update", (*Server).revertRuleUpdate)
	registerRevert("rule_delete", (*Server).revertRuleCreate)
	registerRevert("rule_batch", (*Server).revertRuleBatch)
	registerRevert("memory_add", (*Server).revertMemoryAdd_delete)
	registerRevert("memory_delete", (*Server).revertMemoryAdd)
	registerRevert("memory_clear", (*Server).revertMemoryClear)
}

// The two one-liners that used to sit inline in the switch. They are named for
// what they undo, not for what they do — revertRuleCreate_delete undoes a
// rule_create, which means deleting the rule.
func (s *Server) revertRuleCreate_delete(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, _ revertDetail) {
	_ = s.store.Rules().Delete(ctx, sid, orig.TargetID)
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertMemoryAdd_delete(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, _ revertDetail) {
	_ = s.store.Memory().DeleteFact(ctx, sid, orig.TargetID)
	s.finishRevert(ctx, w, sid, orig)
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

	// Prevent double-revert. This was a scan of the most recent 200 entries,
	// which meant a session busy enough to push the revert past the 200th row
	// could undo the same operation twice — and a compensation is not
	// idempotent, so "add the block back" applied twice adds two blocks.
	if _, err := s.store.OpLogs().RevertedBy(ctx, sid, id); err == nil {
		s.writeErr(w, http.StatusConflict, "already_reverted", "这条操作已经被撤销过了")
		return
	} else if !errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取操作记录失败")
		return
	}

	var detail revertDetail
	_ = json.Unmarshal([]byte(orig.Detail), &detail)

	h, ok := revertHandlers[orig.Action]
	if !ok {
		// Iron rule 3 says everything is reversible, and this is where that
		// promise is kept or broken: an action with no registered inverse is one
		// the ledger can show and not undo. Refusing loudly is the honest
		// answer, and it is also the reminder to whoever added the action.
		s.writeErr(w, http.StatusBadRequest, "irreversible", fmt.Sprintf("操作 %s 不可撤销", orig.Action))
		return
	}
	h(s, ctx, w, sid, orig, detail)
}

func (s *Server) revertPlanAdd(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail revertDetail) {
	if _, _, _, err := s.applyPlanPatch(ctx, sid, orig.Date, planAction{Action: "remove", Match: map[string]any{"id": orig.TargetID}}, domain.ActorSystem); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "撤销失败")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertPlanUpdate(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail revertDetail) {
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

func (s *Server) revertPlanRemove(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail revertDetail) {
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

func (s *Server) revertPlanUpsert(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail revertDetail) {
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

func (s *Server) revertRuleUpdate(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail revertDetail) {
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

func (s *Server) revertRuleCreate(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail revertDetail) {
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

func (s *Server) revertRuleBatch(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail revertDetail) {
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

func (s *Server) revertMemoryAdd(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail revertDetail) {
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

func (s *Server) revertMemoryClear(ctx context.Context, w http.ResponseWriter, sid string, orig *domain.OperationLog, detail revertDetail) {
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
