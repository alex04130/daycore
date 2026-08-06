package server

import (
	"net/http"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

func init() {
	registerRoutes("feedback", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/feedback", s.handleFeedbackAdd)
	})
}

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
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.feedbackAdd.bad_request")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.feedbackAdd.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
