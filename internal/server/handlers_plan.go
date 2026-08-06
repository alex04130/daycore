package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/schedule"
)

func init() {
	registerRoutes("plans", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/plan", s.handlePlanGet)
		mux.HandleFunc("POST /api/plan", s.handlePlanUpsert)
		mux.HandleFunc("PATCH /api/plan", s.handlePlanPatch)
		mux.HandleFunc("GET /api/plan/range", s.handlePlanRange)
	})
}

// ruleOccurrences expands the session's rules for [from, to]; storage failures
// degrade to "no occurrences" rather than failing the whole read.
func (s *Server) ruleOccurrences(ctx context.Context, sid, from, to string) []domain.TimeBlock {
	rules, err := s.store.Rules().List(ctx, sid)
	if err != nil || len(rules) == 0 {
		return nil
	}
	return schedule.Expand(rules, from, to)
}

// sortBlocks orders blocks by time (unscheduled last), mirroring
// applyPlanAction's "add" ordering.
func sortBlocks(blocks []domain.TimeBlock) {
	sort.SliceStable(blocks, func(i, j int) bool {
		ti, tj := "~", "~"
		if blocks[i].Time != nil {
			ti = *blocks[i].Time
		}
		if blocks[j].Time != nil {
			tj = *blocks[j].Time
		}
		return ti < tj
	})
}

// GET /api/plan?date=YYYY-MM-DD — returns the plan with rule occurrences merged
// in (virtually — nothing is written), or JSON null when neither exists.
func (s *Server) handlePlanGet(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 date")
		return
	}
	ctx := r.Context()
	occurrences := s.ruleOccurrences(ctx, sid, date, date)

	plan, err := s.store.DayPlans().Get(ctx, sid, date)
	if errors.Is(err, domain.ErrNotFound) {
		if len(occurrences) == 0 {
			s.writeJSON(w, http.StatusOK, nil) // → JSON null (parity with v1)
			return
		}
		// No stored plan, but rules produce blocks: synthesize a non-persisted
		// plan so standing commitments are always visible.
		sortBlocks(occurrences)
		s.normalizePlanBlocks(occurrences, date, "")
		s.writeJSON(w, http.StatusOK, &domain.DayPlan{
			SessionID: sid, Date: date, Blocks: occurrences, SourceType: "rules",
		})
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取日程失败")
		return
	}
	plan.Blocks = schedule.Visible(schedule.Merge(plan.Blocks, occurrences))
	sortBlocks(plan.Blocks)
	s.normalizePlanBlocks(plan.Blocks, date, "") // backfill UTC anchors for legacy/rule blocks
	s.writeJSON(w, http.StatusOK, plan)
}

// POST /api/plan — upsert the plan for a date. Increments interaction count only
// on first creation (matches v1).
func (s *Server) handlePlanUpsert(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Date       string             `json:"date"`
		Blocks     []domain.TimeBlock `json:"blocks"`
		Note       *string            `json:"note"`
		SourceType string             `json:"sourceType"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Date == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 date")
		return
	}
	ctx := r.Context()
	prev, getErr := s.store.DayPlans().Get(ctx, sid, body.Date)
	isNew := errors.Is(getErr, domain.ErrNotFound)

	// Anchor fixed/local blocks to a UTC instant before persisting.
	s.normalizePlanBlocks(body.Blocks, body.Date, "")

	plan, err := s.store.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID: sid, Date: body.Date, Blocks: body.Blocks, Note: body.Note, SourceType: body.SourceType,
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "日程保存失败")
		return
	}
	if isNew {
		_ = s.store.Sessions().IncrementInteraction(ctx, sid)
	}
	var before any
	if prev != nil {
		before = prev.Blocks
	}
	s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Action: "plan_upsert", Date: body.Date,
		Summary: fmt.Sprintf("%s: %d blocks", body.Date, len(plan.Blocks)),
		Detail:  marshalCompact(map[string]any{"before": before, "after": plan.Blocks}),
	})
	s.writeJSON(w, http.StatusOK, plan)
}

// PATCH /api/plan — apply one incremental action (add/update/remove a block).
// Rule occurrences for the date are materialized into the stored plan first, so
// actions can target them and their state (completion, edits, tombstones)
// persists.

const (
	keyPlanEmptyMatch    = "plan.patch.emptyMatch"
	keyPlanUnknownAction = "plan.patch.unknownAction"
)

func init() {
	i18n.Register(keyPlanEmptyMatch, i18n.Text{
		"zh-CN": "match 不能为空——不带条件的 update/remove 会波及全天所有块",
		"en-US": "match must not be empty — an unconditional update/remove would hit every block of the day",
	})
	i18n.Register(keyPlanUnknownAction, i18n.Text{
		"zh-CN": "action 必须是 add / update / remove 之一",
		"en-US": "action must be one of add / update / remove",
	})
}

func (s *Server) handlePlanPatch(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Date   string     `json:"date"`
		Action planAction `json:"action"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Date == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 date 或 action")
		return
	}
	// The tool layer refuses an empty match; the HTTP layer did not, so a
	// {"action":"remove","match":{}} request tombstoned the whole day. The
	// guard belongs on both sides of the door — a client bug should not be
	// able to reach what the model is not allowed to reach.
	switch body.Action.Action {
	case "update", "remove":
		if len(body.Action.Match) == 0 {
			s.writeErr(w, http.StatusBadRequest, "empty_match", i18n.T(keyPlanEmptyMatch, s.requestLocale(r)))
			return
		}
	case "add":
	default:
		s.writeErr(w, http.StatusBadRequest, "unknown_action", i18n.T(keyPlanUnknownAction, s.requestLocale(r)))
		return
	}
	updated, _, _, err := s.applyPlanPatch(r.Context(), sid, body.Date, body.Action, domain.ActorUser)
	if err != nil {
		var blocked *planBlocked
		if errors.As(err, &blocked) {
			s.writePlanBlocked(w, s.requestLocale(r), blocked)
			return
		}
		s.writeErr(w, http.StatusInternalServerError, "internal", "日程更新失败")
		return
	}
	updated.Blocks = schedule.Visible(updated.Blocks)
	s.writeJSON(w, http.StatusOK, updated)
}

// planBlocksForDate returns the date's visible blocks with rule occurrences
// merged (read-only), sorted by time. Empty when nothing is scheduled.
func (s *Server) planBlocksForDate(ctx context.Context, sid, date string) []domain.TimeBlock {
	occ := s.ruleOccurrences(ctx, sid, date, date)
	plan, err := s.store.DayPlans().Get(ctx, sid, date)
	if errors.Is(err, domain.ErrNotFound) {
		sortBlocks(occ)
		return occ
	}
	if err != nil {
		return nil
	}
	return visibleSorted(schedule.Merge(plan.Blocks, occ))
}

// visibleSorted filters tombstoned blocks and orders by time.
func visibleSorted(blocks []domain.TimeBlock) []domain.TimeBlock {
	v := schedule.Visible(blocks)
	sortBlocks(v)
	return v
}

// GET /api/plan/range?from=&to= — batch fetch (lets the frontend avoid N+1),
// with rule occurrences merged in per date; dates that only have rule
// occurrences appear as synthetic non-persisted plans.
func (s *Server) handlePlanRange(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" || to == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 from/to")
		return
	}
	ctx := r.Context()
	plans, err := s.store.DayPlans().Range(ctx, sid, from, to)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取日程失败")
		return
	}

	occByDate := map[string][]domain.TimeBlock{}
	for _, b := range s.ruleOccurrences(ctx, sid, from, to) {
		occByDate[b.Date] = append(occByDate[b.Date], b)
	}
	out := make([]domain.DayPlan, 0, len(plans)+len(occByDate))
	for _, p := range plans {
		p.Blocks = schedule.Visible(schedule.Merge(p.Blocks, occByDate[p.Date]))
		sortBlocks(p.Blocks)
		s.normalizePlanBlocks(p.Blocks, p.Date, "")
		delete(occByDate, p.Date)
		out = append(out, p)
	}
	for date, blocks := range occByDate {
		sortBlocks(blocks)
		s.normalizePlanBlocks(blocks, date, "")
		out = append(out, domain.DayPlan{SessionID: sid, Date: date, Blocks: blocks, SourceType: "rules"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	s.writeJSON(w, http.StatusOK, out)
}
