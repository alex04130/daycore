package server

import (
	"context"
	"net/http"
	"strings"

	"daycore/internal/ai"
)

// POST /api/ai/mood — mood → warm text response.
func (s *Server) handleAIMood(w http.ResponseWriter, r *http.Request) {
	if !s.rateLimit(w, r) {
		return
	}
	var body struct {
		Mood string `json:"mood"`
	}
	if err := s.readJSON(r, &body); err != nil || strings.TrimSpace(body.Mood) == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 mood")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.AIRequestTimeout)
	defer cancel()

	sys, err := s.prompts.Render(ctx, ai.PromptMood, s.requestLocale(r), ai.MoodData{
		Mood:        body.Mood,
		MemoryFacts: s.memoryFactsContext(ctx, sessionIDFrom(r.Context())),
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "提示词渲染失败")
		return
	}
	resp, err := s.catalog.DefaultChat().Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: ai.RoleUser, Content: sys}},
		Temperature: 0.7, MaxTokens: 300,
	})
	if err != nil {
		s.log.Error("ai mood", "err", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error", "response": "遇到了点问题，稍后再试一下吧。"})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"response": resp.Content})
}

// moodHistoryContext renders the latest mood check-ins as compact JSON.
func (s *Server) moodHistoryContext(ctx context.Context, sid string) string {
	moods, err := s.store.Moods().List(ctx, sid, 5)
	if err != nil || len(moods) == 0 {
		return "[]"
	}
	out := make([]map[string]any, 0, len(moods))
	for _, m := range moods {
		out = append(out, map[string]any{"mood": m.Mood, "at": m.CreatedAt.Format("01-02")})
	}
	return marshalCompact(out)
}
