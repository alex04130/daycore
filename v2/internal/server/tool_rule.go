package server

import (
	"context"
	"encoding/json"

	"daycore/internal/domain"
)

func (s *Server) toolRuleUpsert(ctx context.Context, sid, tz, rawArgs string) toolResult {
	var probe struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(rawArgs), &probe)

	if probe.ID != "" { // update path — reuse the PATCH /api/rules machinery
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(rawArgs), &raw); err != nil {
			return toolFail("invalid arguments: %v", err)
		}
		delete(raw, "id")
		if len(raw) == 0 {
			return toolFail("nothing to update")
		}
		upd, err := ruleUpdateFromRaw(raw)
		if err != nil {
			return toolFail("invalid rule update: %v", err)
		}
		prev, err := s.store.Rules().Get(ctx, sid, probe.ID)
		if err != nil {
			return toolFail("rule not found: %v", err)
		}
		// Validate the merged result before persisting so a bad update can't
		// corrupt the stored rule.
		candidate := applyRuleUpdate(*prev, upd)
		if err := validateRule(&candidate); err != nil {
			return toolFail("rule update leaves the rule inconsistent: %v", err)
		}
		updated, err := s.store.Rules().Update(ctx, sid, probe.ID, upd)
		if err != nil {
			return toolFail("rule update failed: %v", err)
		}
		opID := s.logOp(ctx, &domain.OperationLog{
			SessionID: sid, Actor: domain.ActorAgent, Action: "rule_update", TargetID: updated.ID,
			Summary: updated.Title,
			Detail:  marshalCompact(map[string]any{"before": prev, "after": updated}),
		})
		return toolResult{OK: true, OpID: opID, Data: map[string]any{"rule": updated}, Summary: updated.Title}
	}

	var input ruleInput
	if err := json.Unmarshal([]byte(rawArgs), &input); err != nil {
		return toolFail("invalid arguments: %v", err)
	}
	input.Source = "chat"
	if input.Timezone == "" {
		input.Timezone = orDefault(tz, "UTC")
	}
	rule, err := input.toRule(sid)
	if err != nil {
		return toolFail("invalid rule: %v", err)
	}
	created, err := s.store.Rules().Create(ctx, rule)
	if err != nil {
		return toolFail("rule create failed: %v", err)
	}
	opID := s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "rule_create", TargetID: created.ID,
		Summary: created.Title, Detail: marshalCompact(created),
	})
	return toolResult{OK: true, OpID: opID, Data: map[string]any{"rule": created}, Summary: created.Title}
}

func (s *Server) toolRuleRemove(ctx context.Context, sid, rawArgs string) toolResult {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil || args.ID == "" {
		return toolFail("id is required")
	}
	prev, _ := s.store.Rules().Get(ctx, sid, args.ID)
	if err := s.store.Rules().Delete(ctx, sid, args.ID); err != nil {
		return toolFail("rule delete failed: %v", err)
	}
	title := args.ID
	if prev != nil {
		title = prev.Title
	}
	opID := s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "rule_delete", TargetID: args.ID,
		Summary: title, Detail: marshalCompact(map[string]any{"before": prev}),
	})
	return toolResult{OK: true, OpID: opID, Data: map[string]any{"deleted": args.ID}, Summary: title}
}
