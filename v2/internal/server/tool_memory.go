package server

import (
	"context"
	"encoding/json"
	"strings"

	"daycore/internal/domain"
)

func (s *Server) toolMemoryAdd(ctx context.Context, sid, rawArgs string) toolResult {
	var args struct {
		Fact string `json:"fact"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil || strings.TrimSpace(args.Fact) == "" {
		return toolFail("fact is required")
	}
	fact := strings.TrimSpace(args.Fact)
	// Clamp to the same limit the user-facing endpoint enforces, so a looping or
	// jailbroken model can't write oversized rows.
	if r := []rune(fact); len(r) > maxFactLen {
		fact = string(r[:maxFactLen])
	}
	created, err := s.store.Memory().AddFact(ctx, &domain.MemoryFact{
		SessionID: sid, Fact: fact, Source: "chat",
	})
	if err != nil {
		return toolFail("memory add failed: %v", err)
	}
	opID := s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "memory_add", TargetID: created.ID,
		Summary: created.Fact, Detail: marshalCompact(created),
	})
	return toolResult{OK: true, OpID: opID, Data: map[string]any{"fact": created}, Summary: created.Fact}
}

func (s *Server) toolMemoryRemove(ctx context.Context, sid, rawArgs string) toolResult {
	var args struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil || args.ID == "" {
		return toolFail("id is required")
	}
	// Snapshot the fact before deleting so the operation is revertible.
	var prev *domain.MemoryFact
	if facts, err := s.store.Memory().ListFacts(ctx, sid); err == nil {
		for i := range facts {
			if facts[i].ID == args.ID {
				prev = &facts[i]
				break
			}
		}
	}
	if err := s.store.Memory().DeleteFact(ctx, sid, args.ID); err != nil {
		return toolFail("memory delete failed: %v", err)
	}
	summary := args.ID
	if prev != nil {
		summary = prev.Fact
	}
	opID := s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "memory_delete", TargetID: args.ID,
		Summary: summary, Detail: marshalCompact(map[string]any{"before": prev}),
	})
	return toolResult{OK: true, OpID: opID, Data: map[string]any{"deleted": args.ID}, Summary: summary}
}
