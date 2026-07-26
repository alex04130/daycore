package server

import (
	"net/http"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

// POST /api/feedback
func (s *Server) handleFeedbackAdd(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		MessageID string `json:"messageId"`
		Useful    bool   `json:"useful"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}

	fb := &domain.FeedbackLog{
		ID:        uuid.NewString(),
		SessionID: sid,
		MessageID: body.MessageID,
		Useful:    body.Useful,
		CreatedAt: time.Now(),
	}
	if err := s.store.Feedback().Add(r.Context(), fb); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "保存反馈失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
