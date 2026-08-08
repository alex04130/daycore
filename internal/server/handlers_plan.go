package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
	"daycore/internal/schedule"
)

func init() {
	// 逆操作与写入放在同一个文件 —— 改写入的人正好看得见它。
	registerRevert("plan_upsert", (*Server).revertPlanUpsert)

	registerRoutes("plans", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/plan", s.handlePlanGet)
		mux.HandleFunc("POST /api/plan", s.handlePlanUpsert)
		mux.HandleFunc("PATCH /api/plan", s.handlePlanPatch)
		mux.HandleFunc("GET /api/plan/range", s.handlePlanRange)
		mux.HandleFunc("POST /api/plan/lock", s.handlePlanLock)
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
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.planGet.bad_request")
		return
	}
	ctx := r.Context()
	locale := s.requestLocale(r)
	occurrences := s.ruleOccurrences(ctx, sid, date, date)

	plan, err := s.store.DayPlans().Get(ctx, sid, date)
	if errors.Is(err, domain.ErrNotFound) {
		if len(occurrences) == 0 {
			s.writeJSON(w, http.StatusOK, nil) // → JSON null (parity with v1)
			return
		}
		// No stored plan, but rules produce blocks: synthesize a non-persisted
		// plan so standing commitments are always visible.
		occurrences = append(occurrences, s.spillInsFor(ctx, sid, date, locale)...)
		if len(occurrences) == 0 {
			s.writeJSON(w, http.StatusOK, nil)
			return
		}
		sortBlocks(occurrences)
		s.normalizePlanBlocks(occurrences, date, "", locale)
		localizeLockReasons(occurrences, locale)
		s.writeJSON(w, http.StatusOK, &domain.DayPlan{
			SessionID: sid, Date: date, Blocks: occurrences, SourceType: "rules",
		})
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.planGet.internal")
		return
	}
	plan.Blocks = schedule.Visible(schedule.Merge(plan.Blocks, occurrences))
	plan.Blocks = append(plan.Blocks, s.spillInsFor(ctx, sid, date, locale)...)
	sortBlocks(plan.Blocks)
	s.normalizePlanBlocks(plan.Blocks, date, "", locale) // backfill UTC anchors + locks for legacy/rule blocks
	localizeLockReasons(plan.Blocks, locale)
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
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.planUpsert.bad_request")
		return
	}
	ctx := r.Context()
	prev, getErr := s.store.DayPlans().Get(ctx, sid, body.Date)
	isNew := errors.Is(getErr, domain.ErrNotFound)

	// Anchor fixed/local blocks to a UTC instant before persisting.
	s.normalizePlanBlocks(body.Blocks, body.Date, "", s.requestLocale(r))

	plan, err := s.store.DayPlans().Upsert(ctx, &domain.DayPlan{
		SessionID: sid, Date: body.Date, Blocks: body.Blocks, Note: body.Note, SourceType: body.SourceType,
	})
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.planUpsert.internal2")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.planPatch.bad_request")
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
	updated, _, _, err := s.applyPlanPatch(r.Context(), sid, body.Date, s.requestLocale(r), body.Action, domain.ActorUser)
	if err != nil {
		var blocked *planBlocked
		if errors.As(err, &blocked) {
			s.writePlanBlocked(w, s.requestLocale(r), blocked)
			return
		}
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.planPatch.internal")
		return
	}
	updated.Blocks = schedule.Visible(updated.Blocks)
	localizeLockReasons(updated.Blocks, s.requestLocale(r))
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
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.planRange.bad_request")
		return
	}
	ctx := r.Context()
	plans, err := s.store.DayPlans().Range(ctx, sid, from, to)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.planRange.internal")
		return
	}

	locale := s.requestLocale(r)
	occByDate := map[string][]domain.TimeBlock{}
	for _, b := range s.ruleOccurrences(ctx, sid, from, to) {
		occByDate[b.Date] = append(occByDate[b.Date], b)
	}
	out := make([]domain.DayPlan, 0, len(plans)+len(occByDate))
	for _, p := range plans {
		p.Blocks = schedule.Visible(schedule.Merge(p.Blocks, occByDate[p.Date]))
		sortBlocks(p.Blocks)
		s.normalizePlanBlocks(p.Blocks, p.Date, "", locale)
		localizeLockReasons(p.Blocks, locale)
		delete(occByDate, p.Date)
		out = append(out, p)
	}
	for date, blocks := range occByDate {
		sortBlocks(blocks)
		s.normalizePlanBlocks(blocks, date, "", locale)
		localizeLockReasons(blocks, locale)
		out = append(out, domain.DayPlan{SessionID: sid, Date: date, Blocks: blocks, SourceType: "rules"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	s.writeJSON(w, http.StatusOK, out)
}

// ── manual lock ─────────────────────────────────────────────────────────────

const (
	keyPlanLockBadLevel = "plan.lock.badLevel"
	keyPlanLockNoBlock  = "plan.lock.noBlock"
)

func init() {
	i18n.Register(keyPlanLockBadLevel, i18n.Text{
		"zh-CN": "level 必须是 none / soft / hard 之一",
		"en-US": "level must be one of none / soft / hard",
	})
	i18n.Register(keyPlanLockNoBlock, i18n.Text{
		"zh-CN": "这一天没有这个块",
		"en-US": "no such block on that day",
	})
}

// maxLockReasonLen keeps a hand-written reason to a line the card can show.
const maxLockReasonLen = 200

// POST /api/plan/lock — pin a block's time, or let it go.
//
// This is the way out of a hard lock that the 409 envelope points at. Without
// it a refusal is a dead end: the gate says "the timetable decides this" and
// the user has no way to say "not this week, it doesn't".
//
// It deliberately goes through applyPlanPatch rather than writing the block
// itself. Everything a plan write owes — the ledger entry, a registered
// inverse, the gate — already lives on that path, and a second writer would
// owe them again and eventually forget one. The same reasoning the proposal
// design gives for accepting a card through the ordinary tool path.
//
// Setting a lock by hand marks it lockSource=user, which is what stops
// derivation from putting it back on the next write. That field exists for
// exactly this: without it, unlocking a class would last until the next read.
func (s *Server) handlePlanLock(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Date    string `json:"date"`
		BlockID string `json:"blockId"`
		Level   string `json:"level"`
		Reason  string `json:"reason"`
	}
	locale := s.requestLocale(r)
	if err := s.readJSON(r, &body); err != nil || body.Date == "" || body.BlockID == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.planLock.bad_request")
		return
	}
	level := domain.LockLevel(body.Level)
	switch level {
	case domain.LockNone, domain.LockSoft, domain.LockHard:
	default:
		// LockUnset is not offered. "Put it back to whatever the rules infer"
		// is a different operation from "I have decided", and nothing in the
		// product asks for it yet — offering it here would be inventing a
		// third state for the client to reason about.
		s.writeErr(w, http.StatusBadRequest, "bad_level", i18n.T(keyPlanLockBadLevel, locale))
		return
	}
	reason := clampRunes(strings.TrimSpace(body.Reason), maxLockReasonLen)
	if level == domain.LockNone {
		// An unlocked block has nothing to explain, and keeping the old reason
		// would leave "set by your class timetable" attached to something the
		// timetable no longer governs.
		reason = ""
	}

	updated, _, matched, err := s.applyPlanPatch(r.Context(), sid, body.Date, locale, planAction{
		Action: "update",
		Match:  map[string]any{"id": body.BlockID},
		Changes: map[string]any{
			"lock_level":  string(level),
			"lock_reason": reason,
			"lock_source": domain.LockSourceUser,
		},
	}, domain.ActorUser)
	if err != nil {
		var blocked *planBlocked
		if errors.As(err, &blocked) {
			s.writePlanBlocked(w, locale, blocked)
			return
		}
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.planLock.internal")
		return
	}
	if matched == 0 {
		s.writeErr(w, http.StatusNotFound, "not_found", i18n.T(keyPlanLockNoBlock, locale))
		return
	}
	updated.Blocks = schedule.Visible(updated.Blocks)
	localizeLockReasons(updated.Blocks, locale)
	s.writeJSON(w, http.StatusOK, updated)
}

// spillInsFor returns the previous day's blocks that are still running when
// this date begins (consensus 25: a cross-midnight block counts on both days).
//
// A read-time derivation, deliberately: storing a second row would make the
// two copies drift the first time someone edited one, and "数据一条" is half
// the consensus. Storage failures degrade to "no spill-ins" — a missing
// yesterday should not fail today.
func (s *Server) spillInsFor(ctx context.Context, sid, date, locale string) []domain.TimeBlock {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil
	}
	prev := d.AddDate(0, 0, -1).Format("2006-01-02")
	plan, err := s.store.DayPlans().Get(ctx, sid, prev)
	if err != nil {
		return nil
	}
	blocks := schedule.Merge(plan.Blocks, s.ruleOccurrences(ctx, sid, prev, prev))
	s.normalizePlanBlocks(blocks, prev, "", locale)
	return schedule.SpillIns(schedule.Visible(blocks), prev, s.planLocation(ctx, sid))
}
