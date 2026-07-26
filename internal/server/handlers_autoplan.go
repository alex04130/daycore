package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"daycore/internal/ai"
	"daycore/internal/domain"
	"daycore/internal/schedule"
)

var (
	weekdaysZH = []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	weekdaysEN = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
)

// POST /api/ai/auto-plan — the autonomous planner. Aggregates everything known
// about the session (course/rule commitments, assignment deadlines, grades,
// long-term memory) and generates a plan for one date (default today) or a
// short range, saved with SourceType "auto".
//
// Regeneration modes:
//   - "keep_manual" (default): rule blocks, manual/edited blocks, and completed
//     blocks survive; only previous auto blocks are regenerated.
//   - "replace_all": only rule blocks survive.
func (s *Server) handleAIAutoPlan(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !s.rateLimit(w, r) {
		return
	}
	var body struct {
		Date         string `json:"date"`
		Weekday      string `json:"weekday"`
		Time         string `json:"time"`
		Timezone     string `json:"timezone"`
		From         string `json:"from"`
		To           string `json:"to"`
		Instructions string `json:"instructions"`
		Mode         string `json:"mode"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	mode := orDefault(body.Mode, "keep_manual")
	if mode != "keep_manual" && mode != "replace_all" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "mode 必须是 keep_manual 或 replace_all")
		return
	}

	locale := s.requestLocale(r)

	// Fill in "now" from the requested timezone when the client sent nothing.
	loc := time.UTC
	if body.Timezone != "" {
		if l, err := time.LoadLocation(body.Timezone); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)
	if body.Date == "" {
		body.Date = now.Format("2006-01-02")
	}
	if body.Time == "" {
		body.Time = now.Format("15:04")
	}
	if body.Weekday == "" {
		if d, err := time.Parse("2006-01-02", body.Date); err == nil {
			body.Weekday = localeWeekday(locale, int(d.Weekday()))
		}
	}

	from, to := body.From, body.To
	if from == "" || to == "" {
		from, to = body.Date, body.Date
	}
	fromT, err1 := time.Parse("2006-01-02", from)
	toT, err2 := time.Parse("2006-01-02", to)
	if err1 != nil || err2 != nil || toT.Before(fromT) {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "from/to 格式应为 YYYY-MM-DD 且 from <= to")
		return
	}
	days := int(toT.Sub(fromT).Hours()/24) + 1
	if days > s.cfg.AutoPlanMaxDays {
		s.writeErr(w, http.StatusBadRequest, "range_too_large",
			fmt.Sprintf("一次最多规划 %d 天", s.cfg.AutoPlanMaxDays))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.AIRequestTimeout)
	defer cancel()

	// ── gather materials ─────────────────────────────────────────────────
	occByDate := map[string][]domain.TimeBlock{}
	for _, b := range s.ruleOccurrences(ctx, sid, from, to) {
		occByDate[b.Date] = append(occByDate[b.Date], b)
	}
	storedByDate := map[string][]domain.TimeBlock{}
	noteByDate := map[string]*string{}
	if plans, err := s.store.DayPlans().Range(ctx, sid, from, to); err == nil {
		for _, p := range plans {
			storedByDate[p.Date] = p.Blocks
			noteByDate[p.Date] = p.Note
		}
	}

	// keptByDate is what survives regeneration (and gets persisted verbatim);
	// its visible subset is the "fixed commitments" context for the model.
	keptByDate := map[string][]domain.TimeBlock{}
	fixedForPrompt := []domain.TimeBlock{}
	for d := fromT; !d.After(toT); d = d.AddDate(0, 0, 1) {
		date := d.Format("2006-01-02")
		merged := schedule.Merge(storedByDate[date], occByDate[date])
		kept := make([]domain.TimeBlock, 0, len(merged))
		for _, b := range merged {
			switch {
			case b.RuleID != "": // rule occurrences (and their tombstones) always survive
				kept = append(kept, b)
			case mode == "keep_manual" && (b.Origin != domain.OriginAuto || b.Completed):
				kept = append(kept, b)
			}
		}
		keptByDate[date] = kept
		for _, b := range kept {
			if !b.Hidden {
				if b.Date == "" {
					b.Date = date
				}
				fixedForPrompt = append(fixedForPrompt, b)
			}
		}
	}

	horizon := toT.AddDate(0, 0, s.cfg.AssignmentLookaheadDays)
	dueFrom := now.Add(-24 * time.Hour)
	assignments, _ := s.store.Assignments().List(ctx, sid, domain.AssignmentFilter{DueFrom: &dueFrom, DueTo: &horizon})
	plannable := assignments[:0]
	for _, a := range assignments {
		if a.Status == domain.AssignmentDone || a.Status == domain.AssignmentDismissed {
			continue
		}
		plannable = append(plannable, a)
	}
	courses, _ := s.store.Courses().List(ctx, sid)

	// Long-term memory: server memory facts + legacy client-written key facts.
	factStrings := []string{}
	if facts, err := s.store.Memory().ListFacts(ctx, sid); err == nil {
		for _, f := range facts {
			factStrings = append(factStrings, f.Fact)
		}
	}
	if mem, err := s.store.Companion().Get(ctx, sid); err == nil {
		factStrings = append(factStrings, mem.KeyFacts...)
	}
	keyFacts := "[]"
	if len(factStrings) > 0 {
		keyFacts = marshalCompact(factStrings)
	}

	// ── render + call the model ──────────────────────────────────────────
	dc := ai.BuildDateContext(body.Date, body.Weekday, body.Time, body.Timezone, s.requestLocale(r))
	sys, err := s.prompts.Render(ctx, ai.PromptAutoPlan, locale, ai.AutoPlanData{
		Date: dc.Date, Weekday: dc.Weekday, Time: dc.Time, Timezone: dc.Timezone,
		RelativeDateMap: dc.RelativeDateMap,
		From:            from, To: to,
		Dates:        rangeDatesMarkdown(locale, fromT, toT),
		FixedBlocks:  marshalCompact(promptBlocks(fixedForPrompt)),
		Assignments:  assignmentsMarkdown(plannable, courses),
		Courses:      coursesMarkdown(courses),
		KeyFacts:     keyFacts,
		Instructions: strings.TrimSpace(body.Instructions),
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "提示词渲染失败")
		return
	}

	userMsg := "开始规划。"
	if locale == "en-US" {
		userMsg = "Generate the plan."
	}
	resp, err := s.catalog.Planner().Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: sys},
			{Role: ai.RoleUser, Content: userMsg},
		},
		Temperature: 0.3, MaxTokens: 8192, JSONMode: true,
	})
	if err != nil {
		s.log.Error("ai auto-plan", "err", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error", "message": "自主规划出了点问题，请稍后再试"})
		return
	}
	result, ok := extractJSONObject(resp.Content)
	if !ok {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "parse_error", "message": "规划解析出了点问题，请重试"})
		return
	}
	if result["error"] != nil {
		s.writeJSON(w, http.StatusOK, result)
		return
	}

	// ── merge new auto blocks with survivors and persist ─────────────────
	newBlocks, warnings := parseAutoBlocks(result["blocks"], from, to, body.Timezone)
	note, _ := result["note"].(string)

	plans := []domain.DayPlan{}
	for d := fromT; !d.After(toT); d = d.AddDate(0, 0, 1) {
		date := d.Format("2006-01-02")
		blocks := append(keptByDate[date], newBlocks[date]...)
		if len(blocks) == 0 {
			continue
		}
		sortBlocks(blocks)
		// Recovery snapshot: the blocks being overwritten, one log per date.
		if old, existed := storedByDate[date]; existed {
			s.logOp(ctx, &domain.OperationLog{
				SessionID: sid, Actor: domain.ActorAgent, Action: "plan_autoplan", Date: date,
				Summary: fmt.Sprintf("%s: replaced %d blocks", date, len(old)),
				Detail:  marshalCompact(map[string]any{"before": old}),
			})
		}
		planNote := noteByDate[date]
		if note != "" {
			planNote = &note
		}
		saved, err := s.store.DayPlans().Upsert(ctx, &domain.DayPlan{
			SessionID: sid, Date: date, Blocks: blocks, SourceType: "auto", Note: planNote,
		})
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, "internal", "日程保存失败")
			return
		}
		saved.Blocks = schedule.Visible(saved.Blocks)
		plans = append(plans, *saved)
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"plans": plans, "note": note, "mode": mode, "warnings": warnings,
	})
}

// parseAutoBlocks validates the model's blocks, stamps ids/origin, and groups
// them by date. Blocks dated outside [from, to] are dropped with a warning.
func parseAutoBlocks(v any, from, to, fallbackTZ string) (map[string][]domain.TimeBlock, []string) {
	byDate := map[string][]domain.TimeBlock{}
	var warnings []string
	arr, ok := v.([]any)
	if !ok {
		return byDate, warnings
	}
	raw, _ := json.Marshal(arr)
	var blocks []domain.TimeBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return byDate, []string{"部分时间块无法解析，已忽略"}
	}
	base := time.Now().UnixNano()
	for i, b := range blocks {
		if b.Date < from || b.Date > to || !isDate(b.Date) {
			warnings = append(warnings, fmt.Sprintf("忽略了日期越界的时间块 %q（%s）", b.Title, b.Date))
			continue
		}
		if strings.TrimSpace(b.Title) == "" {
			continue
		}
		b.ID = fmt.Sprintf("block-%d-%d", base, i)
		b.Origin = domain.OriginAuto
		b.RuleID = ""
		b.Hidden = false
		b.Completed = false
		if !validBlockTypes[b.Type] {
			b.Type = domain.BlockTask
		}
		if b.TimeMode != domain.TimeFixed {
			b.TimeMode = domain.TimeFloating
		}
		if b.Timezone == "" {
			b.Timezone = fallbackTZ
		}
		byDate[b.Date] = append(byDate[b.Date], b)
	}
	return byDate, warnings
}

// promptBlocks strips blocks down to what the model needs to plan around.
func promptBlocks(blocks []domain.TimeBlock) []map[string]any {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		m := map[string]any{"date": b.Date, "title": b.Title, "type": b.Type}
		if b.Time != nil {
			m["time"] = *b.Time
		}
		if b.DurationMin != nil {
			m["duration_min"] = *b.DurationMin
		}
		if b.Completed {
			m["completed"] = true
		}
		out = append(out, m)
	}
	return out
}

func rangeDatesMarkdown(locale string, from, to time.Time) string {
	var b strings.Builder
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		fmt.Fprintf(&b, "- %s（%s）\n", d.Format("2006-01-02"), localeWeekday(locale, int(d.Weekday())))
	}
	return b.String()
}

func localeWeekday(locale string, dow int) string {
	if locale == "en-US" {
		return weekdaysEN[dow]
	}
	return weekdaysZH[dow]
}
