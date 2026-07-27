package server

import (
	"net/http"
	"strconv"
	"strings"

	"daycore/internal/domain"
)

// GET /api/mood?limit=10 — recent check-ins (newest first).
func (s *Server) handleMoodList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := s.store.Moods().List(r.Context(), sid, limit)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取心情记录失败")
		return
	}
	s.writeJSON(w, http.StatusOK, rows)
}

// POST /api/mood — record a check-in; bumps interaction count.
func (s *Server) handleMoodCreate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Mood            string  `json:"mood"`
		AIResponse      *string `json:"aiResponse"`
		ExerciseOffered *string `json:"exerciseOffered"`
		Theme           *string `json:"theme"`
		Note            string  `json:"note"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Mood == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 mood")
		return
	}
	// Source is set here, never read from the body. A check-in that arrives on
	// this endpoint is the user pressing a button; the agent records its own
	// through its tool. Letting a client claim source=agent would let it write
	// check-ins that the mood window quietly discounts — or, worse, let a
	// frontend bug relabel real ones as inferences.
	ctx := r.Context()
	checkin, err := s.store.Moods().Create(ctx, &domain.MoodCheckin{
		SessionID: sid, Mood: body.Mood, AIResponse: body.AIResponse,
		ExerciseOffered: body.ExerciseOffered, Theme: body.Theme,
		Source: domain.MoodSourceUser, Note: strings.TrimSpace(body.Note),
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "保存心情失败")
		return
	}
	_ = s.store.Sessions().IncrementInteraction(ctx, sid)
	s.writeJSON(w, http.StatusOK, checkin)
}

// PATCH /api/mood — mark a check-in's exercise as completed.
func (s *Server) handleMoodPatch(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := s.readJSON(r, &body); err != nil || body.ID == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 id")
		return
	}
	if err := s.store.Moods().MarkExerciseCompleted(r.Context(), sid, body.ID); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "更新失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
