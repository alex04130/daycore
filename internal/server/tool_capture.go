package server

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"daycore/internal/domain"
	"daycore/internal/timeutil"
)

// The four capture tools: what the prompt has always promised the agent would
// do with time-bound facts, open loops, moods and reference material said in
// conversation (EXPERIENCE_CORE §12.1). The data layer and the ledger domains
// were ready long before these existed — an agent that "noted it down" without
// them was making a promise nothing kept.
//
// Every tool follows the toolMemoryAdd shape: validate → clamp → store →
// s.logOp with a full before/after snapshot → toolResult carrying the OpID.
// The op action names are the ones OpDomainOf already maps, so each record
// lands in the right ledger domain (and thereby the right boldness score).
//
// maxCaptureTitleLen keeps agent-written titles UI-sized; bodies (material
// content, wish notes) are deliberately unclamped — EXPERIENCE_CORE §12.3
// exempts them from the 50-rune discipline.
const maxCaptureTitleLen = 200

func clampRunes(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

// toolAssignmentUpsert creates or updates an assignment/deadline from
// conversation ("下周三交离散" → create; "考试提前到周五" → update by id).
func (s *Server) toolAssignmentUpsert(ctx context.Context, sid, rawArgs string) toolResult {
	var args struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		CourseID   string `json:"courseId"`
		CourseName string `json:"courseName"`
		DueAt      string `json:"dueAt"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return toolFail("invalid arguments")
	}
	args.Title = clampRunes(strings.TrimSpace(args.Title), maxCaptureTitleLen)

	// A course name the model inferred is resolved against the imported course
	// list; an unknown name is not an error — the assignment just stands alone.
	if args.CourseID == "" && args.CourseName != "" {
		if courses, err := s.store.Courses().List(ctx, sid); err == nil {
			for _, c := range courses {
				if strings.EqualFold(c.Name, args.CourseName) || strings.EqualFold(c.CourseCode, args.CourseName) {
					args.CourseID = c.ID
					break
				}
			}
		}
	}
	var dueAt *time.Time
	if args.DueAt != "" {
		t, err := parseFlexibleTime(args.DueAt)
		if err != nil {
			return toolFail("dueAt must be RFC3339, YYYY-MM-DDTHH:MM or YYYY-MM-DD")
		}
		dueAt = &t
	}

	if args.ID != "" { // update path: mutate the fetched row, upsert by its canvas id
		existing, err := s.store.Assignments().Get(ctx, sid, args.ID)
		if err != nil {
			return toolFail("assignment not found: %s", args.ID)
		}
		before := *existing
		if args.Title != "" {
			existing.Title = args.Title
		}
		if args.CourseID != "" {
			existing.CourseID = args.CourseID
		}
		if args.DueAt != "" {
			existing.DueAt = dueAt
		}
		updated, err := s.store.Assignments().UpsertByCanvasID(ctx, existing)
		if err != nil {
			return toolFail("assignment update failed: %v", err)
		}
		opID := s.logOp(ctx, &domain.OperationLog{
			SessionID: sid, Actor: domain.ActorAgent, Action: "assignment_upsert", TargetID: updated.ID,
			Summary: updated.Title,
			Detail:  marshalCompact(map[string]any{"before": before, "after": updated}),
		})
		return toolResult{OK: true, OpID: opID, Data: map[string]any{"assignment": updated}, Summary: updated.Title}
	}

	if args.Title == "" {
		return toolFail("title is required")
	}
	created, err := s.createManualAssignment(ctx, sid, args.Title, args.CourseID, dueAt)
	if err != nil {
		return toolFail("assignment create failed: %v", err)
	}
	opID := s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "assignment_upsert", TargetID: created.ID,
		Summary: created.Title,
		Detail:  marshalCompact(map[string]any{"before": nil, "after": created}),
	})
	return toolResult{OK: true, OpID: opID, Data: map[string]any{"assignment": created}, Summary: created.Title}
}

// toolWishAdd drops an open loop into the wish pool. Exact-duplicate titles are
// not recorded twice — the pool's value is that it stays small enough to read.
func (s *Server) toolWishAdd(ctx context.Context, sid, rawArgs string) toolResult {
	var args struct {
		Text      string `json:"text"`
		EffortMin int    `json:"effortMin"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return toolFail("invalid arguments")
	}
	args.Text = clampRunes(strings.TrimSpace(args.Text), maxCaptureTitleLen)
	if args.Text == "" {
		return toolFail("text is required")
	}
	if wishes, err := s.store.Wishes().List(ctx, sid, "active"); err == nil {
		for _, w := range wishes {
			if strings.EqualFold(strings.TrimSpace(w.Title), args.Text) {
				return toolResult{OK: true, Data: map[string]any{"duplicate": true, "wish": w}, Summary: w.Title}
			}
		}
	}
	created, err := s.store.Wishes().Create(ctx, &domain.Wish{
		SessionID: sid, Title: args.Text, EffortMin: args.EffortMin, Status: domain.WishActive,
	})
	if err != nil {
		return toolFail("wish create failed: %v", err)
	}
	opID := s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "wish_create", TargetID: created.ID,
		Summary: created.Title,
		Detail:  marshalCompact(map[string]any{"before": nil, "after": created}),
	})
	return toolResult{OK: true, OpID: opID, Data: map[string]any{"wish": created}, Summary: created.Title}
}

// toolMoodRecord lets the agent check in on the user's behalf (source=agent).
// A manual check-in already on the day wins: the agent's inference never
// overwrites what the user pressed (EXPERIENCE_CORE §12.1 情绪打卡三边界).
func (s *Server) toolMoodRecord(ctx context.Context, sid, tz, rawArgs string) toolResult {
	var args struct {
		Mood string `json:"mood"`
		Note string `json:"note"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return toolFail("invalid arguments")
	}
	if _, ok := domain.MoodKindByID(args.Mood); !ok {
		return toolFail("unknown mood %q — pick the closest of the twelve", args.Mood)
	}
	args.Note = clampRunes(strings.TrimSpace(args.Note), maxFactLen)

	loc := resolveLocation(tz)
	now := time.Now().In(loc)
	// timeutil, not time.Date: on a zone that shifts its clocks at midnight the
	// naive form returns the PREVIOUS day's 23:00, and "already checked in
	// today" would then swallow last night's check-in.
	dayStart := timeutil.StartOfDay(now, loc)
	if recent, err := s.store.Moods().List(ctx, sid, 50); err == nil {
		for _, m := range recent {
			// Rows predating the Source column read as user check-ins.
			if m.Source != "" && m.Source != domain.MoodSourceUser {
				continue
			}
			if !m.CreatedAt.In(loc).Before(dayStart) {
				return toolResult{OK: true, Data: map[string]any{"alreadyCheckedIn": true, "mood": m.Mood}, Summary: m.Mood}
			}
		}
	}
	created, err := s.store.Moods().Create(ctx, &domain.MoodCheckin{
		SessionID: sid, Mood: args.Mood, Note: args.Note, Source: domain.MoodSourceAgent,
	})
	if err != nil {
		return toolFail("mood record failed: %v", err)
	}
	opID := s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "mood_record", TargetID: created.ID,
		Summary: created.Mood,
		Detail:  marshalCompact(map[string]any{"before": nil, "after": created}),
	})
	return toolResult{OK: true, OpID: opID, Data: map[string]any{"checkin": created}, Summary: created.Mood}
}

// toolMaterialAdd files reference material (exam scope, room numbers, …) into
// the library. The body is exempt from the capture length discipline.
func (s *Server) toolMaterialAdd(ctx context.Context, sid, rawArgs string) toolResult {
	var args struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		Category string `json:"category"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return toolFail("invalid arguments")
	}
	args.Title = clampRunes(strings.TrimSpace(args.Title), maxCaptureTitleLen)
	if args.Title == "" || strings.TrimSpace(args.Body) == "" {
		return toolFail("title and body are required")
	}
	category, ok := normalizeCategory(args.Category)
	if !ok {
		return toolFail("unknown category %q", args.Category)
	}
	created, err := s.store.Materials().Create(ctx, &domain.Material{
		SessionID: sid, Category: category, Title: args.Title, Body: args.Body, Source: "chat",
	})
	if err != nil {
		return toolFail("material create failed: %v", err)
	}
	opID := s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Actor: domain.ActorAgent, Action: "material_create", TargetID: created.ID,
		Summary: created.Title,
		Detail:  marshalCompact(map[string]any{"before": nil, "after": created}),
	})
	return toolResult{OK: true, OpID: opID, Data: map[string]any{"material": created}, Summary: created.Title}
}
