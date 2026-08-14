package server

import (
	"context"
	"strconv"
	"strings"
	"time"

	"daycore/internal/ai"
	"daycore/internal/domain"
)

// runWeeklyLetter writes the Sunday-evening prose letter (周信). It reads the
// week just ended through the river and the mood check-ins, asks the model to
// write it as prose (not a chart), and stores the result for the app to show.
func (w *Worker) runWeeklyLetter(sid, tz string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sess, err := w.s.store.Sessions().Get(ctx, sid)
	if err != nil {
		w.log.Warn("worker runWeeklyLetter: get session", "sid", sid, "err", err)
		return
	}
	locale := w.s.localePair(ctx, sid).Resolve(sess.Language, "")
	name := sess.AssistantName
	if name == "" {
		name = w.s.cfg.DefaultAssistantName
	}

	loc := resolveLocation(tz)
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")
	// The week that just ended: Monday..Sunday. monday is the run key, so one
	// letter per session per week.
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday -> 7 so monday is six days back
	}
	monday := now.AddDate(0, 0, -(weekday - 1)).Format("2006-01-02")

	run, ok := w.claim(ctx, sid, domain.JobWeeklyLetter, monday)
	if !ok {
		return
	}
	var jobErr error
	defer func() { w.finish(ctx, run, jobErr) }()

	// Guard: if a letter for this week already exists, the claim is what should
	// have stopped us; a second guard is cheap insurance against a re-fire.
	if l, err := w.s.store.WeeklyLetters().Latest(ctx, sid); err == nil && l != nil && l.WeekStart == monday {
		return
	}

	// Gather the week's river + moods as prose-friendly context.
	river := ""
	if days, err := w.s.store.River().Days(ctx, sid, 7); err == nil {
		parts := make([]string, 0, len(days))
		for _, d := range days {
			parts = append(parts, d.Date+"("+strconv.Itoa(int(d.Count))+")")
		}
		river = strings.Join(parts, ", ")
	}
	moods := ""
	if ms, err := w.s.store.Moods().List(ctx, sid, 14); err == nil && len(ms) > 0 {
		seen := map[string]bool{}
		parts := make([]string, 0, len(ms))
		for _, m := range ms {
			if m.CreatedAt.Before(now.AddDate(0, 0, -7)) {
				continue
			}
			if seen[m.Mood] {
				continue
			}
			seen[m.Mood] = true
			if k, ok := domain.MoodKindByID(m.Mood); ok {
				parts = append(parts, k.Emoji)
			}
		}
		moods = strings.Join(parts, " ")
	}

	sysPrompt, err := w.s.prompts.Render(ctx, ai.PromptWeeklyLetter, locale, map[string]any{
		"Name": name, "WeekStart": monday, "WeekEnd": today,
		"River": river, "Moods": moods,
	})
	if err != nil {
		w.log.Error("worker runWeeklyLetter: prompt", "sid", sid, "err", err)
		jobErr = err
		return
	}

	provider := w.s.catalog.DefaultChat()
	start := time.Now()
	resp, err := provider.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: ai.RoleSystem, Content: sysPrompt}},
		Temperature: 0.7,
		MaxTokens:   1024,
	})
	w.s.logAICall(ctx, sid, epWeeklyLetter, provider.Model(), start, usageOf(resp), err)
	if err != nil {
		w.log.Error("worker runWeeklyLetter: agent call", "sid", sid, "err", err)
		jobErr = err
		return
	}

	text := strings.TrimSpace(resp.Content)
	if text == "" {
		return
	}
	if _, err := w.s.store.WeeklyLetters().Create(ctx, &domain.WeeklyLetter{
		SessionID: sid, WeekStart: monday, WeekEnd: today, Body: text, Locale: locale,
	}); err != nil {
		w.log.Error("worker runWeeklyLetter: store", "sid", sid, "err", err)
		jobErr = err
	}
}
