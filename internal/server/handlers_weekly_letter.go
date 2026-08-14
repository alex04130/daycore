package server

import (
	"errors"
	"net/http"

	"daycore/internal/domain"
)

func init() {
	registerRoutes("weekly", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/weekly-letter", s.handleWeeklyLetter)
	})
}

// GET /api/weekly-letter — the latest Sunday-evening prose letter (周信), or
// {letter:null} when none has been written yet. The Worker writes one per week;
// this is the read side.
func (s *Server) handleWeeklyLetter(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	l, err := s.store.WeeklyLetters().Latest(r.Context(), sid)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeJSON(w, http.StatusOK, map[string]any{"letter": nil})
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.weeklyLetter.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"letter": l})
}
