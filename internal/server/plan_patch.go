package server

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"time"

	"daycore/internal/domain"
	"daycore/internal/schedule"

	"github.com/google/uuid"
)

type planAction struct {
	Action  string         `json:"action"` // "update" | "remove" | "add"
	Match   map[string]any `json:"match"`
	Changes map[string]any `json:"changes"`
	Block   map[string]any `json:"block"`
	// Confirm is the way through a soft lock: the user was told this one was
	// agreed with someone else and said go ahead anyway. It does nothing for a
	// hard lock or a petrified block — those have no way through, and letting a
	// flag open them would make the gate advisory.
	Confirm bool `json:"confirm"`
}

// applyPlanPatch materializes the date's rule occurrences into the stored
// plan, applies one incremental action, persists, and writes the audit log.
// It is shared by the HTTP PATCH handler and the companion agent tools.
// matched counts blocks hit by update/remove so tool callers can surface
// no-op matches instead of silently succeeding.
func (s *Server) applyPlanPatch(ctx context.Context, sid, date, locale string, action planAction, actor string) (updated *domain.DayPlan, opID string, matched int, err error) {
	plan, err := s.store.DayPlans().Get(ctx, sid, date)
	if errors.Is(err, domain.ErrNotFound) {
		plan = &domain.DayPlan{SessionID: sid, Date: date, SourceType: "rules"}
	} else if err != nil {
		return nil, "", 0, err
	}
	plan.Blocks = schedule.Merge(plan.Blocks, s.ruleOccurrences(ctx, sid, date, date))

	// Operate on generic maps so arbitrary fields in match/changes are honored.
	raw, _ := json.Marshal(plan.Blocks)
	var blocks []map[string]any
	_ = json.Unmarshal(raw, &blocks)

	// Strip routing keys from match so matchesAll never trips on them.
	delete(action.Match, "date")

	// Snapshot the affected blocks for the audit log before mutation.
	var affected []map[string]any
	var beforeJSON json.RawMessage
	switch action.Action {
	case "add":
		if action.Block == nil {
			action.Block = map[string]any{}
		}
		if _, ok := action.Block["id"]; !ok {
			action.Block["id"] = "block-" + uuid.NewString()
		}
		matched = 1
	case "update", "remove":
		for _, b := range blocks {
			if matchesAll(b, action.Match) {
				affected = append(affected, b)
			}
		}
		beforeJSON, _ = json.Marshal(affected)
		matched = len(affected)
	}

	// A retry names what it is retrying; the server works out the rest. Done
	// here rather than in applyPlanAction because it needs the ORIGINAL block,
	// which only exists before the action runs.
	if err := s.resolveReschedule(ctx, sid, blocks, &action); err != nil {
		return nil, "", 0, err
	}

	// The gate, before any mutation and before the keep_manual flip: a refused
	// write must leave the plan exactly as it found it, including origin.
	if err := s.guardPlanWrite(blocks, date, action, actor, time.Now(), s.planLocation(ctx, sid)); err != nil {
		return nil, "", 0, err
	}

	// keep_manual is a server-side promise, not a client convention: any edit a
	// user or the agent makes to an auto block turns it manual, so the next
	// regeneration keeps it. Until this lived here, the flip existed only as a
	// field the web frontend happened to send — the agent's own edits to auto
	// blocks were silently overwritten by the next auto-plan, which is the
	// product's flagship flow breaking its own word. ActorSystem (revert,
	// regeneration) never flips.
	if action.Action == "update" && (actor == domain.ActorUser || actor == domain.ActorAgent) {
		for _, b := range blocks {
			if !matchesAll(b, action.Match) {
				continue
			}
			if o, _ := b["origin"].(string); o == domain.OriginAuto {
				b["origin"] = domain.OriginManual
			}
		}
	}

	blocks = applyPlanAction(blocks, action)

	out, _ := json.Marshal(blocks)
	var newBlocks []domain.TimeBlock
	_ = json.Unmarshal(out, &newBlocks)

	plan.Blocks = newBlocks
	// Anchor fixed/local blocks to a UTC instant before persisting.
	s.normalizePlanBlocks(plan.Blocks, date, "", locale)
	updated, err = s.store.DayPlans().Upsert(ctx, plan)
	if err != nil {
		return nil, "", matched, err
	}
	if opAction := map[string]string{"add": "plan_add", "update": "plan_update", "remove": "plan_remove"}[action.Action]; opAction != "" {
		target, title, after := "", "", any(affected)
		switch action.Action {
		case "add":
			target, _ = action.Block["id"].(string)
			title, _ = action.Block["title"].(string)
			after = action.Block
		default:
			if len(affected) > 0 {
				target, _ = affected[0]["id"].(string)
				title, _ = affected[0]["title"].(string)
			}
			if action.Action == "remove" {
				// Only tombstoned rule occurrences survive a remove.
				survivors := []map[string]any{}
				for _, b := range affected {
					if hidden, _ := b["hidden"].(bool); hidden {
						survivors = append(survivors, b)
					}
				}
				after = survivors
			}
		}
		opID = s.logOp(ctx, &domain.OperationLog{
			SessionID: sid, Actor: actor, Action: opAction, TargetID: target, Date: date,
			Summary: title,
			Detail:  marshalCompact(map[string]any{"before": beforeJSON, "after": after}),
		})
	}
	return updated, opID, matched, nil
}

// applyPlanAction mirrors v1's incremental block mutation logic.
func applyPlanAction(blocks []map[string]any, a planAction) []map[string]any {
	switch a.Action {
	case "update":
		for _, b := range blocks {
			if matchesAll(b, a.Match) {
				for k, v := range a.Changes {
					b[k] = v
				}
			}
		}
		return blocks
	case "remove":
		out := blocks[:0]
		for _, b := range blocks {
			if !matchesAll(b, a.Match) {
				out = append(out, b)
				continue
			}
			// Rule occurrences are tombstoned instead of dropped: a dropped one
			// would resurface on the next read when the rule re-expands.
			if rid, _ := b["rule_id"].(string); rid != "" {
				b["hidden"] = true
				out = append(out, b)
			}
		}
		return out
	case "add":
		nb := map[string]any{}
		for k, v := range a.Block {
			nb[k] = v
		}
		if _, ok := nb["id"]; !ok {
			nb["id"] = "block-" + uuid.NewString()
		}
		blocks = append(blocks, nb)
		sort.SliceStable(blocks, func(i, j int) bool {
			return timeStr(blocks[i]) < timeStr(blocks[j])
		})
		return blocks
	}
	return blocks
}

func matchesAll(block, match map[string]any) bool {
	for k, v := range match {
		// reflect.DeepEqual instead of != so a non-comparable match value
		// (a map/array supplied in a raw PATCH body) can't panic the request.
		if !reflect.DeepEqual(block[k], v) {
			return false
		}
	}
	return true
}

func timeStr(b map[string]any) string {
	if t, ok := b["time"].(string); ok {
		return t
	}
	return "~" // nil/unscheduled sorts last
}
