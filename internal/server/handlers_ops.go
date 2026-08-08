package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"

	"daycore/internal/domain"
)

func init() {
	registerRoutes("operation logs & undo", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/ops", s.handleOpList)
		mux.HandleFunc("POST /api/ops/{id}/revert", s.handleOpRevert)
	})
}

// GET /api/ops?limit= — list recent operation logs for the session.
func (s *Server) handleOpList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	logs, err := s.store.OpLogs().List(r.Context(), sid, limit)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.opList.internal")
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
	sid, locale string, orig *domain.OperationLog, detail revertDetail)

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

// irreversibleActions names the actions that are logged but deliberately have no
// inverse, with the reason.
//
// It exists so that "no inverse" is a DECISION somebody wrote down rather than
// the default outcome of forgetting. Before it, a write with no registration and
// a write whose author had thought about it and concluded there is nothing to
// undo were indistinguishable — both produced a 400 at the moment the user
// pressed the button, which is the worst time to find out.
var irreversibleActions = map[string]string{}

// revertSites records which file declared each action, so a duplicate can name
// both sources. The panic happens during package init, before TestMain runs and
// before any test name is printed; without the file names the only way to find
// it is to read every registration by eye.
var revertSites = map[string]string{}

// registerRevert declares the inverse of an action. Declaring the same action
// twice panics — two inverses for one action means one of them is dead code, and
// which one wins would depend on link order.
//
// ⚠️ revertHandlers must stay a package-level var initialiser. Go initialises
// package-level variables before ANY init() in the package, whatever the file
// order; move the make() into an init() and the first file alphabetically to
// call this writes to a nil map.
func registerRevert(action string, h revertHandler) {
	claimAction(action)
	revertHandlers[action] = h
}

// registerIrreversible declares that an action has no inverse, and why.
//
// The reason is not decoration: it is what a future reader needs in order to
// tell "nobody got round to it" from "there is nothing to undo". Both look
// identical from the outside.
func registerIrreversible(action, reason string) {
	claimAction(action)
	irreversibleActions[action] = reason
}

func claimAction(action string) {
	site := callerFile(3)
	if prev, dup := revertSites[action]; dup {
		panic("server: action " + action + " is declared twice — in " + prev + " and in " + site +
			"; one of the two is dead code and which one wins depends on link order")
	}
	revertSites[action] = site
}

func callerFile(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}
	return filepath.Base(file) + ":" + strconv.Itoa(line)
}

// decodeInto re-materialises a snapshot from the ledger.
//
// The ledger stores JSON, so a Before/After comes back as map[string]any and has
// to go through a second round trip to become a typed value. Reporting failure
// rather than silently leaving a zero value is the point: a revert that "worked"
// and restored an empty row is worse than one that refused.
func decodeInto(snapshot any, dst any) bool {
	if snapshot == nil {
		return false
	}
	b, err := json.Marshal(snapshot)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, dst) == nil
}

// Actions that are logged and deliberately have no inverse.
//
// Declared rather than left out: "nobody registered one" and "there is nothing
// to undo" produce the same 400 at the moment the user presses the button, and
// only one of them is a bug. The reason is what tells them apart.
func init() {
	registerIrreversible("tool_get_weather",
		"a read. The ledger records that the agent looked something up, so the user can see why it said what it said; there is no state to put back.")
	registerIrreversible("tool_web_search",
		"a read, same as the weather lookup. Undoing a search would mean unseeing it.")
	registerIrreversible("proposal_accepted",
		"the record that the user answered a card. The WRITES an acceptance performs are logged separately and are individually undoable; undoing the answer itself would mean claiming they never decided.")
	registerIrreversible("proposal_rejected",
		"the record that the user declined or let a card lapse. Nothing was written, so there is nothing to put back — and re-showing a card they dismissed is the opposite of what saying no meant.")
	registerIrreversible("conflict_mark",
		"marking a scheduling clash creates a decision card and changes no plan. The card is answered or expires on its own; the way out is to answer it, not to erase the observation.")
	registerIrreversible("revert",
		"the compensation entry for undoing something else. Undoing an undo is redo, which is a different feature with different semantics (EXPERIENCE_CORE consensus 23). Chaining through this path would compound compensations rather than replay them.")
}

// The inverses themselves are registered by the files that own the writes — see
// handlers_plan.go, plan_patch.go, handlers_rules.go and the rest. A feature
// that is linked in is a feature whose writes are undoable, and the inverse sits
// next to the code that will need changing when the write changes.
//
// TestEveryLoggedActionIsDeclared is what makes that safe to spread out: a
// registration deleted during a move fails the build's tests rather than
// surfacing as a 400 the first time a user presses undo.

// The two one-liners that used to sit inline in the switch. They are named for
// what they undo, not for what they do — revertRuleCreate_delete undoes a
// rule_create, which means deleting the rule.
func (s *Server) revertRuleCreate_delete(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, _ revertDetail) {
	_ = s.store.Rules().Delete(ctx, sid, orig.TargetID)
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertMemoryAdd_delete(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, _ revertDetail) {
	_ = s.store.Memory().DeleteFact(ctx, sid, orig.TargetID)
	s.finishRevert(ctx, w, sid, orig)
}

// revertAssignmentUpsert undoes both halves of the tool: a creation is
// deleted outright (dismissal is a workflow state, not an erasure), an update
// is restored from its before snapshot via the same canvas-id upsert that
// wrote it.
func (s *Server) revertAssignmentUpsert(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	if detail.Before == nil {
		_ = s.store.Assignments().Delete(ctx, sid, orig.TargetID)
		s.finishRevert(ctx, w, sid, orig)
		return
	}
	var before domain.Assignment
	b, _ := json.Marshal(detail.Before)
	_ = json.Unmarshal(b, &before)
	if before.CanvasID == "" {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.assignmentUpsert.internal")
		return
	}
	if _, err := s.store.Assignments().UpsertByCanvasID(ctx, &before); err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.assignmentUpsert.internal")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertWishCreate_delete(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, _ revertDetail) {
	_ = s.store.Wishes().Delete(ctx, sid, orig.TargetID)
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertMoodRecord_delete(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, _ revertDetail) {
	_ = s.store.Moods().Delete(ctx, sid, orig.TargetID)
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertMaterialCreate_delete(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, _ revertDetail) {
	_ = s.store.Materials().Delete(ctx, sid, orig.TargetID)
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
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "op_not_found", "err.opRevert.op_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.opRevert.internal")
		return
	}

	// Prevent double-revert. This was a scan of the most recent 200 entries,
	// which meant a session busy enough to push the revert past the 200th row
	// could undo the same operation twice — and a compensation is not
	// idempotent, so "add the block back" applied twice adds two blocks.
	if _, err := s.store.OpLogs().RevertedBy(ctx, sid, id); err == nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusConflict, "already_reverted", "err.opRevert.already_reverted")
		return
	} else if !errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.opRevert.internal")
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
		s.writeErrf(w, s.requestLocale(r), http.StatusBadRequest, "irreversible", "err.fmt.irreversible", orig.Action)
		return
	}
	h(s, ctx, w, sid, s.requestLocale(r), orig, detail)
}

func (s *Server) revertPlanAdd(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	if _, _, _, err := s.applyPlanPatch(ctx, sid, orig.Date, "", planAction{Action: "remove", Match: map[string]any{"id": orig.TargetID}}, domain.ActorSystem); err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.planAdd.internal")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertPlanUpdate(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	blocks, _ := toBlockMaps(detail.Before)
	for _, b := range blocks {
		id, _ := b["id"].(string)
		ch := map[string]any{}
		for k, v := range b {
			if k != "id" && k != "rule_id" && k != "origin" && k != "hidden" {
				ch[k] = v
			}
		}
		if _, _, _, err := s.applyPlanPatch(ctx, sid, orig.Date, "", planAction{Action: "update", Match: map[string]any{"id": id}, Changes: ch}, domain.ActorSystem); err != nil {
			s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.planUpdate.internal")
			return
		}
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertPlanRemove(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	blocks, _ := toBlockMaps(detail.Before)
	for _, b := range blocks {
		if rid, _ := b["rule_id"].(string); rid != "" {
			// Un-tombstone a rule occurrence.
			if _, _, _, err := s.applyPlanPatch(ctx, sid, orig.Date, "", planAction{Action: "update", Match: map[string]any{"id": b["id"]}, Changes: map[string]any{"hidden": false}}, domain.ActorSystem); err != nil {
				s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.planRemove.internal")
				return
			}
		} else {
			if _, _, _, err := s.applyPlanPatch(ctx, sid, orig.Date, "", planAction{Action: "add", Block: b}, domain.ActorSystem); err != nil {
				s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.planRemove.internal")
				return
			}
		}
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertPlanUpsert(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
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
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.planUpsert.internal")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertRuleUpdate(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	before, ok := detail.Before.(map[string]any)
	if !ok || before == nil {
		s.writeErrL(w, locale, http.StatusBadRequest, "irreversible", "err.ruleUpdate.irreversible")
		return
	}
	raw := map[string]json.RawMessage{}
	for k, v := range before {
		b, _ := json.Marshal(v)
		raw[k] = b
	}
	upd, err := ruleUpdateFromRaw(raw)
	if err != nil {
		s.writeErrf(w, locale, http.StatusBadRequest, "irreversible", "err.fmt.ruleRestore", err)
		return
	}
	if _, err := s.store.Rules().Update(ctx, sid, orig.TargetID, upd); err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.ruleUpdate.internal")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertRuleCreate(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	var raw map[string]any
	b, _ := json.Marshal(detail.Before)
	_ = json.Unmarshal(b, &raw)
	if raw == nil {
		b, _ = json.Marshal(detail.After)
		_ = json.Unmarshal(b, &raw)
	}
	if raw == nil {
		s.writeErrL(w, locale, http.StatusBadRequest, "irreversible", "err.ruleCreate.irreversible")
		return
	}
	raw["source"] = "chat"
	input := ruleInput{}
	rb, _ := json.Marshal(raw)
	_ = json.Unmarshal(rb, &input)
	rule, err := input.toRule(sid)
	if err != nil {
		s.writeErrf(w, locale, http.StatusBadRequest, "irreversible", "err.fmt.ruleRebuild", err)
		return
	}
	if _, err := s.store.Rules().Create(ctx, rule); err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.ruleCreate.internal")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertRuleBatch(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
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

func (s *Server) revertMemoryAdd(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	var fact domain.MemoryFact
	b, _ := json.Marshal(detail.Before)
	_ = json.Unmarshal(b, &fact)
	if b == nil || fact.ID == "" {
		b, _ = json.Marshal(detail.After)
		_ = json.Unmarshal(b, &fact)
	}
	if fact.Fact == "" {
		s.writeErrL(w, locale, http.StatusBadRequest, "irreversible", "err.memoryAdd.irreversible")
		return
	}
	_, _ = s.store.Memory().AddFact(ctx, &domain.MemoryFact{
		SessionID: sid, Fact: fact.Fact, Source: fact.Source,
	})
	s.finishRevert(ctx, w, sid, orig)
}

func (s *Server) revertMemoryClear(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	items, _ := detail.Before.([]any)
	if items == nil {
		s.writeErrL(w, locale, http.StatusBadRequest, "irreversible", "err.memoryClear.irreversible")
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
