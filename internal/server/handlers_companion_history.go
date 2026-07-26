package server

import (
	"errors"
	"net/http"

	"daycore/internal/domain"
)

// GET /api/companion-history — persisted chat history + key facts for the session.
func (s *Server) handleCompanionHistoryGet(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	mem, err := s.store.Companion().Get(r.Context(), sid)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeJSON(w, http.StatusOK, map[string]any{"history": []any{}, "keyFacts": []any{}})
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取对话历史失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"history":  mem.ConversationHistory,
		"keyFacts": mem.KeyFacts,
	})
}

// POST /api/companion-history — persist chat history + key facts.
func (s *Server) handleCompanionHistorySet(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		History  []domain.Message `json:"history"`
		KeyFacts []string         `json:"keyFacts"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if err := s.store.Companion().Upsert(r.Context(), sid, body.History, body.KeyFacts); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "保存对话历史失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
