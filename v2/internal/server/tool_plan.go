package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"daycore/internal/domain"
)

func (s *Server) toolPlanAdd(ctx context.Context, sid, tz, rawArgs string) toolResult {
	var args struct {
		Date        string `json:"date"`
		Title       string `json:"title"`
		Time        string `json:"time"`
		Type        string `json:"type"`
		DurationMin int    `json:"duration_min"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return toolFail("invalid arguments: %v", err)
	}
	if !isDate(args.Date) {
		return toolFail("invalid date %q (want YYYY-MM-DD from the lookup table)", args.Date)
	}
	if strings.TrimSpace(args.Title) == "" {
		return toolFail("title is required")
	}
	block := map[string]any{
		"title": strings.TrimSpace(args.Title), "type": orDefault(args.Type, "task"),
		"time_mode": "floating", "timezone": orDefault(tz, "UTC"), "origin": "manual",
	}
	if args.Time != "" {
		if _, err := time.Parse("15:04", args.Time); err != nil {
			return toolFail("invalid time %q (want HH:MM)", args.Time)
		}
		block["time"] = args.Time
	}
	if args.DurationMin > 0 {
		block["duration_min"] = args.DurationMin
	}
	updated, opID, _, err := s.applyPlanPatch(ctx, sid, args.Date, planAction{Action: "add", Block: block}, domain.ActorAgent)
	if err != nil {
		return toolFail("plan_add failed: %v", err)
	}
	return toolResult{OK: true, OpID: opID,
		Data:    map[string]any{"date": args.Date, "plan": visiblePlan(updated)},
		Summary: fmt.Sprintf("%s · %s", args.Date, args.Title)}
}

func (s *Server) toolPlanPatch(ctx context.Context, sid, rawArgs, kind string) toolResult {
	var args struct {
		Date    string         `json:"date"`
		Match   map[string]any `json:"match"`
		Changes map[string]any `json:"changes"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return toolFail("invalid arguments: %v", err)
	}
	if !isDate(args.Date) {
		return toolFail("invalid date %q (want YYYY-MM-DD from the lookup table)", args.Date)
	}
	delete(args.Match, "date")
	if len(args.Match) == 0 {
		return toolFail("match must identify a block (id/title/time)")
	}
	if kind == "update" && len(args.Changes) == 0 {
		return toolFail("changes must not be empty")
	}
	updated, opID, matched, err := s.applyPlanPatch(ctx, sid, args.Date, planAction{Action: kind, Match: args.Match, Changes: args.Changes}, domain.ActorAgent)
	if err != nil {
		return toolFail("%s failed: %v", kind, err)
	}
	if matched == 0 {
		return toolFail("no block on %s matches %v — check the plan in context and retry with the block id", args.Date, args.Match)
	}
	return toolResult{OK: true, OpID: opID,
		Data:    map[string]any{"date": args.Date, "plan": visiblePlan(updated)},
		Summary: args.Date}
}
