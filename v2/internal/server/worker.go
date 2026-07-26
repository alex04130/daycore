// Package server — background worker for proactive features: morning/evening
// briefs, deadline warnings, rolling replan checks, and auto-plan on first open.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"daycore/internal/ai"
	"daycore/internal/channels"
	"daycore/internal/domain"
	"daycore/internal/i18n"

	"github.com/robfig/cron/v3"
)

// Worker runs background scheduled tasks on a per-user-timezone cadence.
type Worker struct {
	cron     *cron.Cron
	s        *Server
	log      *slog.Logger
	mu       sync.Mutex
	jobs     map[string]cron.EntryID         // session_id:timezone_hash → entry
	channels *channels.Registry              // nil when channels are not wired
}

// NewWorker creates a background worker. Call Start() to begin.
// If chReg is nil, channel-delivered messages will only be logged.
func NewWorker(s *Server, chReg *channels.Registry) *Worker {
	return &Worker{
		cron:     cron.New(cron.WithLocation(time.UTC)),
		s:        s,
		log:      s.log,
		jobs:     make(map[string]cron.EntryID),
		channels: chReg,
	}
}

// Start begins the cron scheduler.
func (w *Worker) Start() {
	w.cron.Start()
}

// Stop gracefully shuts down the scheduler and waits for running jobs to
// finish (cron.Stop returns a context that closes when they have).
func (w *Worker) Stop() {
	<-w.cron.Stop().Done()
}

// ScheduleUser runs the standard proactive jobs for a session at its timezone.
func (w *Worker) ScheduleUser(sid, tz string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := sid + ":" + tz
	if _, ok := w.jobs[key]; ok {
		return // already scheduled
	}
	// Morning brief: 07:30 in the user's own timezone (CRON_TZ prefix).
	morningSpec := cronScheduleAt(7, 30, tz)
	id, err := w.cron.AddFunc(morningSpec, func() {
		w.runBrief(sid, tz, "morning")
	})
	if err != nil {
		w.log.Error("schedule morning brief", "sid", sid, "spec", morningSpec, "err", err)
	} else {
		w.jobs[key+":morning"] = id
	}
	// Evening review: 21:00 in the user's own timezone.
	eveningSpec := cronScheduleAt(21, 0, tz)
	id, err = w.cron.AddFunc(eveningSpec, func() {
		w.runBrief(sid, tz, "evening")
	})
	if err != nil {
		w.log.Error("schedule evening review", "sid", sid, "spec", eveningSpec, "err", err)
	} else {
		w.jobs[key+":evening"] = id
	}
	// Deadline check: every 2 hours.
	id, err = w.cron.AddFunc("0 */2 * * *", func() {
		w.checkDeadlines(sid, tz)
	})
	if err == nil {
		w.jobs[key+":deadline"] = id
	}
	// Rolling replan: every 30 minutes.
	id, err = w.cron.AddFunc("*/30 * * * *", func() {
		w.checkRollingReplan(sid, tz)
	})
	if err == nil {
		w.jobs[key+":replan"] = id
	}
	w.log.Info("scheduled proactive jobs", "sid", sid, "tz", tz)
}

// cronScheduleAt builds a 5-field cron spec that fires at hour:min in the given
// IANA timezone. The CRON_TZ prefix makes robfig/cron evaluate the spec in that
// zone (the scheduler itself runs in UTC). Empty tz falls back to UTC.
//
// This also fixes the prior 6-field spec: cron.New was created without
// WithSeconds(), so a 6-field spec failed to parse and the brief jobs were
// silently dropped.
func cronScheduleAt(hour, min int, tz string) string {
	if tz == "" {
		tz = "UTC"
	}
	return fmt.Sprintf("CRON_TZ=%s %d %d * * *", tz, min, hour)
}

// ─── Brief generation ────────────────────────────────────────────────────────

// runBrief generates and sends a morning or evening brief for a session.
func (w *Worker) runBrief(sid, tz, kind string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := w.s.store.Sessions().Get(ctx, sid)
	if err != nil {
		w.log.Warn("worker runBrief: get session", "sid", sid, "err", err)
		return
	}

	prefs := parsePrefs(sess.Preferences)
	locale := i18n.Resolve(sess.Language, "")
	name := sess.AssistantName
	if name == "" {
		name = "Daycore"
	}

	switch kind {
	case "morning":
		if !prefs.MorningBrief || prefs.DoNotDisturb {
			return
		}
		w.log.Info("running morning brief", "sid", sid)
	case "evening":
		if !prefs.EveningReview || prefs.DoNotDisturb {
			return
		}
		w.log.Info("running evening review", "sid", sid)
	}

	loc := resolveLocation(tz)
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")

	// Gather context: weather, today's plan, upcoming deadlines.
	weatherSummary := w.lookupWeather(ctx, locale)
	planSummary := w.loadPlanSummary(ctx, sid, today)

	// Build the prompt and call the agent.
	sysPrompt := buildBriefSystemPrompt(locale, name, today, now.Format("15:04"), tz, weatherSummary, planSummary)
	if kind == "morning" && prefs.GapSuggestions {
		sysPrompt += gapSuggestionHint(locale)
	}

	var userMsg string
	if kind == "morning" {
		userMsg = morningUserPrompt(locale)
	} else {
		userMsg = eveningUserPrompt(locale)
	}

	resp, err := w.s.catalog.DefaultChat().Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: sysPrompt},
			{Role: ai.RoleUser, Content: userMsg},
		},
		Temperature: 0.7,
		MaxTokens:   1024,
	})
	if err != nil {
		w.log.Error("worker runBrief: agent call", "sid", sid, "kind", kind, "err", err)
		return
	}

	text := strings.TrimSpace(resp.Content)
	if text == "" {
		return
	}

	w.sendToChannels(ctx, sid, text)
}

// ─── Deadline warnings ───────────────────────────────────────────────────────

// checkDeadlines scans upcoming assignments and warns about imminent deadlines.
func (w *Worker) checkDeadlines(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prefs := w.loadSessionPrefs(ctx, sid)
	if !prefs.DeadlineAlerts || prefs.DoNotDisturb {
		return
	}

	loc := resolveLocation(tz)
	now := time.Now().In(loc)
	deadline := now.Add(48 * time.Hour)

	assigns, err := w.s.store.Assignments().List(ctx, sid, domain.AssignmentFilter{})
	if err != nil {
		w.log.Warn("worker checkDeadlines: list assignments", "sid", sid, "err", err)
		return
	}

	var urgent []domain.Assignment
	for _, a := range assigns {
		if a.DueAt == nil {
			continue
		}
		if a.Status == domain.AssignmentDone || a.Status == domain.AssignmentDismissed {
			continue
		}
		if a.DueAt.After(now) && a.DueAt.Before(deadline) {
			urgent = append(urgent, a)
		}
		// Also flag overdue assignments (due_at < now) that aren't done yet.
		if a.DueAt.Before(now) && a.Status != domain.AssignmentDone && a.Status != domain.AssignmentDismissed {
			urgent = append(urgent, a)
		}
	}

	if len(urgent) == 0 {
		return
	}

	w.log.Info("deadline check", "sid", sid, "urgent", len(urgent))

	sess, err := w.s.store.Sessions().Get(ctx, sid)
	if err != nil {
		w.log.Warn("worker checkDeadlines: get session", "sid", sid, "err", err)
		return
	}
	locale := i18n.Resolve(sess.Language, "")

	msg := formatDeadlineWarning(locale, urgent, now)
	w.sendToChannels(ctx, sid, msg)
}

// ─── Rolling replan ──────────────────────────────────────────────────────────

// checkRollingReplan checks if any today blocks are overdue and proposes replan.
func (w *Worker) checkRollingReplan(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	prefs := w.loadSessionPrefs(ctx, sid)
	if !prefs.RollingReplan || prefs.DoNotDisturb {
		return
	}

	loc := resolveLocation(tz)
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")

	dp, err := w.s.store.DayPlans().Get(ctx, sid, today)
	if err != nil || dp == nil {
		return
	}

	var overdue []domain.TimeBlock
	for _, b := range dp.Blocks {
		if b.Completed || b.Hidden || b.Time == nil {
			continue
		}
		bt, err := parseBlockTime(today, *b.Time, loc)
		if err != nil {
			continue
		}
		dur := 0 // default: 30 min buffer
		if b.DurationMin != nil {
			dur = *b.DurationMin
		}
		endTime := bt.Add(time.Duration(dur+15) * time.Minute)
		if now.After(endTime) {
			overdue = append(overdue, b)
		}
	}

	if len(overdue) == 0 {
		return
	}

	w.log.Info("rolling replan", "sid", sid, "overdue", len(overdue))

	sess, err := w.s.store.Sessions().Get(ctx, sid)
	if err != nil {
		w.log.Warn("worker checkRollingReplan: get session", "sid", sid, "err", err)
		return
	}
	locale := i18n.Resolve(sess.Language, "")

	// Call the agent to evaluate and suggest a replan.
	overdueJSON, _ := json.Marshal(overdue)
	sysPrompt := buildReplanSystemPrompt(locale, today, now.Format("15:04"), tz)
	userMsg := buildReplanUserPrompt(locale, string(overdueJSON))

	resp, err := w.s.catalog.DefaultChat().Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: ai.RoleSystem, Content: sysPrompt},
			{Role: ai.RoleUser, Content: userMsg},
		},
		Temperature: 0.3,
		MaxTokens:   800,
	})
	if err != nil {
		w.log.Error("worker checkRollingReplan: agent call", "sid", sid, "err", err)
		return
	}

	text := strings.TrimSpace(resp.Content)
	if text == "" {
		return
	}

	w.sendToChannels(ctx, sid, text)
}

// ─── Channel dispatch ────────────────────────────────────────────────────────

// sendToChannels delivers a message through every channel bound to the session.
// When the channels registry is not wired, the message is only logged.
func (w *Worker) sendToChannels(ctx context.Context, sid, text string) {
	bindings, err := w.s.store.ChannelBindings().ListBySession(ctx, sid)
	if err != nil {
		w.log.Warn("worker: list channel bindings", "sid", sid, "err", err)
		return
	}
	if len(bindings) == 0 {
		w.log.Debug("worker: no channel bindings for session", "sid", sid)
		return
	}

	for _, b := range bindings {
		if w.channels == nil {
			w.log.Info("worker: would send to channel (registry not wired)",
				"sid", sid, "channel", b.Channel, "externalId", b.ExternalID,
				"len", len(text))
			continue
		}
		ch := w.channels.Channel(b.Channel)
		if ch == nil {
			w.log.Warn("worker: unknown channel", "channel", b.Channel)
			continue
		}
		if err := ch.Send(ctx, b.ExternalID, text); err != nil {
			w.log.Warn("worker: channel send failed",
				"sid", sid, "channel", b.Channel, "externalId", b.ExternalID, "err", err)
		}
	}
	w.log.Info("worker: message dispatched",
		"sid", sid, "bindings", len(bindings), "len", len(text))
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// loadSessionPrefs fetches session prefs, falling back to defaults.
func (w *Worker) loadSessionPrefs(ctx context.Context, sid string) SessionPrefs {
	sess, err := w.s.store.Sessions().Get(ctx, sid)
	if err != nil {
		return DefaultPrefs()
	}
	return parsePrefs(sess.Preferences)
}

// parsePrefs decodes the JSON preferences string or returns defaults on any error.
func parsePrefs(raw string) SessionPrefs {
	var p SessionPrefs
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return DefaultPrefs()
	}
	return p
}

// resolveLocation parses tz into a *time.Location, falling back to UTC.
func resolveLocation(tz string) *time.Location {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// parseBlockTime parses a "HH:MM" time on the given date in a location.
func parseBlockTime(dateStr, timeStr string, loc *time.Location) (time.Time, error) {
	var h, m int
	if _, err := fmt.Sscanf(timeStr, "%d:%d", &h, &m); err != nil {
		return time.Time{}, err
	}
	return time.Date(
		time.Now().In(loc).Year(), time.Now().In(loc).Month(), time.Now().In(loc).Day(),
		h, m, 0, 0, loc,
	), nil
}

// lookupWeather tries to get a 2-day forecast for Beijing (future: session setting).
func (w *Worker) lookupWeather(ctx context.Context, locale string) string {
	if w.s.weather == nil {
		return ""
	}
	fc, err := w.s.weather.Lookup(ctx, domain.WeatherQuery{Location: "北京", Days: 2, Locale: locale})
	if err != nil {
		return ""
	}
	return fc.Summary(locale)
}

// loadPlanSummary loads today's visible plan blocks as a compact summary.
func (w *Worker) loadPlanSummary(ctx context.Context, sid, date string) string {
	dp, err := w.s.store.DayPlans().Get(ctx, sid, date)
	if err != nil || dp == nil || len(dp.Blocks) == 0 {
		return ""
	}
	vp := visiblePlan(dp)
	if len(vp.Blocks) == 0 {
		return ""
	}
	b, err := json.Marshal(vp.Blocks)
	if err != nil {
		return ""
	}
	return string(b)
}

// ─── Prompt builders ─────────────────────────────────────────────────────────

// buildBriefSystemPrompt constructs a system prompt for the brief agent.
func buildBriefSystemPrompt(locale, name, date, clock, tz, weather, planJSON string) string {
	var b strings.Builder

	if strings.HasPrefix(locale, "zh") {
		b.WriteString(fmt.Sprintf("你是 %s，一个亲切的日程助手。现在是 %s %s（时区 %s）。\n", name, date, clock, tz))
		if weather != "" {
			b.WriteString(fmt.Sprintf("天气：%s\n", weather))
		}
		if planJSON != "" {
			b.WriteString(fmt.Sprintf("今天的计划：%s\n", planJSON))
		}
		b.WriteString("你需要为用户生成一段简短的简报，然后直接发送即可，不需要追问用户任何问题。\n")
		b.WriteString("语气：温暖友好但简洁，像朋友发消息。不要说\"作为 AI\"或\"我理解你的感受\"这类套话。\n")
		b.WriteString("格式：纯文本，2-4 句话即可，不要太长。直接开始，不要用\"早上好\"之类的标题。\n")
	} else {
		b.WriteString(fmt.Sprintf("You are %s, a warm schedule assistant. The time is %s %s (timezone %s).\n", name, date, clock, tz))
		if weather != "" {
			b.WriteString(fmt.Sprintf("Weather: %s\n", weather))
		}
		if planJSON != "" {
			b.WriteString(fmt.Sprintf("Today's plan: %s\n", planJSON))
		}
		b.WriteString("Generate a brief summary for the user. Be direct, no follow-up questions.\n")
		b.WriteString("Tone: warm and friendly but concise, like texting a friend. Skip robotic filler.\n")
		b.WriteString("Format: plain text, 2-4 sentences max. No greeting header like \"Good morning\".\n")
	}

	return b.String()
}

// gapSuggestionHint appends the GapSuggestions behavior to the morning brief so
// the toggle actually does something (a full standalone gap-scan job is a later
// optimization).
func gapSuggestionHint(locale string) string {
	if strings.HasPrefix(locale, "zh") {
		return "\n另外：如果今天日程里有较长的空档，可以顺带建议用户安排一件小事（休息、复习或愿望池里的事），一句话即可。"
	}
	return "\nAlso: if today's schedule has a long open gap, suggest one small thing to slot in (a break, review, or a wish) — one sentence."
}

func morningUserPrompt(locale string) string {
	if strings.HasPrefix(locale, "zh") {
		return "请给我生成一条早安简报，总结今天的日程和天气。"
	}
	return "Please generate a friendly morning brief summarizing today's schedule and weather."
}

func eveningUserPrompt(locale string) string {
	if strings.HasPrefix(locale, "zh") {
		return "请回顾一下今天的完成情况，并给出明天优先事项的建议。"
	}
	return "Please review today's accomplishments and suggest tomorrow's priorities."
}

// buildReplanSystemPrompt constructs a system prompt for the rolling replan agent.
func buildReplanSystemPrompt(locale, date, clock, tz string) string {
	if strings.HasPrefix(locale, "zh") {
		return fmt.Sprintf(
			"你是 Daycore 日程助手。现在是 %s %s（时区 %s）。用户今天有几个时间块已经超时但没有标记完成。\n"+
				"请分析这些超时块，提出一个简短的重排建议。比如：把某件事推迟到下午、删掉、或是放到明天。\n"+
				"语气：像一个朋友在帮你理顺日程，而不是出报告。1-3 句就够了，不需要长篇大论。\n"+
				"直接说你的建议，不要问用户问题，不要说\"你好\"之类的开场白。",
			date, clock, tz)
	}
	return fmt.Sprintf(
		"You are the Daycore scheduling assistant. The time is %s %s (timezone %s). The user has overdue time blocks that weren't marked completed.\n"+
			"Analyze them and suggest a brief replan: reschedule, drop, or move to tomorrow.\n"+
			"Tone: like a friend helping sort things out, not a report. 2-4 sentences max.\n"+
			"Give your suggestion directly. No greetings, no questions back to the user.",
		date, clock, tz)
}

func buildReplanUserPrompt(locale, overdueJSON string) string {
	if strings.HasPrefix(locale, "zh") {
		return fmt.Sprintf("以下是我今天超时未完成的时间块（JSON）：%s\n请帮我提出重排建议。", overdueJSON)
	}
	return fmt.Sprintf("Here are my overdue time blocks for today (JSON): %s\nPlease suggest a replan.", overdueJSON)
}

// ─── Deadline formatting ─────────────────────────────────────────────────────

// formatDeadlineWarning builds a human-readable deadline warning message.
func formatDeadlineWarning(locale string, urgent []domain.Assignment, now time.Time) string {
	var b strings.Builder

	if strings.HasPrefix(locale, "zh") {
		overdueCount, upcomingCount := partitionDeadlines(urgent, now)

		b.WriteString("提醒：")
		if overdueCount > 0 {
			b.WriteString(fmt.Sprintf("你有 %d 个已逾期的任务", overdueCount))
			if upcomingCount > 0 {
				b.WriteString(fmt.Sprintf("，还有 %d 个在 48 小时内截止", upcomingCount))
			}
		} else {
			b.WriteString(fmt.Sprintf("你有 %d 个任务在 48 小时内截止", upcomingCount))
		}
		b.WriteString("：\n")

		for i, a := range urgent {
			due := "无截止日期"
			if a.DueAt != nil {
				if a.DueAt.Before(now) {
					due = fmt.Sprintf("%s（已逾期）", a.DueAt.Format("01-02 15:04"))
				} else {
					due = a.DueAt.Format("01-02 15:04")
				}
			}
			b.WriteString(fmt.Sprintf("%d. %s — %s\n", i+1, a.Title, due))
		}
	} else {
		overdueCount, upcomingCount := partitionDeadlines(urgent, now)

		b.WriteString("Reminder: ")
		if overdueCount > 0 {
			b.WriteString(fmt.Sprintf("you have %d overdue assignment(s)", overdueCount))
			if upcomingCount > 0 {
				b.WriteString(fmt.Sprintf(" and %d due within 48 hours", upcomingCount))
			}
		} else {
			b.WriteString(fmt.Sprintf("you have %d assignment(s) due within 48 hours", upcomingCount))
		}
		b.WriteString(":\n")

		for i, a := range urgent {
			due := "No due date"
			if a.DueAt != nil {
				if a.DueAt.Before(now) {
					due = fmt.Sprintf("%s (overdue)", a.DueAt.Format("01-02 15:04"))
				} else {
					due = a.DueAt.Format("01-02 15:04")
				}
			}
			b.WriteString(fmt.Sprintf("%d. %s — %s\n", i+1, a.Title, due))
		}
	}

	return b.String()
}

// partitionDeadlines counts overdue vs upcoming assignments against now.
func partitionDeadlines(urgent []domain.Assignment, now time.Time) (overdue, upcoming int) {
	for _, a := range urgent {
		if a.DueAt != nil && a.DueAt.Before(now) {
			overdue++
		} else {
			upcoming++
		}
	}
	return
}

// ─── Session preferences ─────────────────────────────────────────────────────

// SessionPrefs holds user-controlled toggles for proactive features.
type SessionPrefs struct {
	MorningBrief   bool `json:"morningBrief"`
	EveningReview  bool `json:"eveningReview"`
	DeadlineAlerts bool `json:"deadlineAlerts"`
	RollingReplan  bool `json:"rollingReplan"`
	GapSuggestions bool `json:"gapSuggestions"`
	DoNotDisturb   bool `json:"doNotDisturb"`
	AutoPlan       bool `json:"autoPlan"`

	// MaterialCategories holds per-category enablement overrides (category id →
	// on/off). A missing entry falls back to the registry's DefaultOn; "note"
	// can never be turned off. See domain.MaterialCategories.
	MaterialCategories map[string]bool `json:"materialCategories,omitempty"`
}

// DefaultPrefs returns the default (all-on) preferences.
func DefaultPrefs() SessionPrefs {
	return SessionPrefs{
		MorningBrief:   true,
		EveningReview:  true,
		DeadlineAlerts: true,
		RollingReplan:  true,
		GapSuggestions: true,
		DoNotDisturb:   false,
		AutoPlan:       true,
	}
}
