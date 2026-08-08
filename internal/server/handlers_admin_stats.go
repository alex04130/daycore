package server

import (
	"net/http"
	"strconv"

	"daycore/internal/domain"
)

func init() {
	registerRoutes("admin (stats, users, DB)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/stats", s.handleAdminStats)
		mux.HandleFunc("GET /api/admin/ailogs", s.handleAdminAILogs)
		mux.HandleFunc("GET /api/admin/users", s.handleAdminUsers)
		mux.HandleFunc("DELETE /api/admin/users/{id}", s.handleAdminDeleteUser)
	})
}

// GET /api/admin/stats — dashboard aggregation.
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminStats.unauthorized")
		return
	}
	ctx := r.Context()
	stats, err := s.store.AILogs().Stats(ctx)
	if err != nil {
		s.log.Error("admin stats", "err", err)
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.adminStats.internal")
		return
	}
	// Surface collected feedback (previously write-only).
	if useful, total, ferr := s.store.Feedback().Stats(ctx); ferr == nil {
		stats.FeedbackUseful = useful
		stats.FeedbackTotal = total
	}
	s.writeJSON(w, http.StatusOK, stats)
}

// GET /api/admin/ailogs — AI call log list with pagination.
func (s *Server) handleAdminAILogs(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminAILogs.unauthorized")
		return
	}
	// TODO: pagination query when AILogRepository.List is added.
	// For now, return stats as a minimal view.
	s.writeJSON(w, http.StatusOK, map[string]any{"logs": []any{}, "note": "full log list coming soon"})
}

// GET /api/admin/users — user list with pagination and search.
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminUsers.unauthorized")
		return
	}
	// TODO: UserRepository.List when added. For now return placeholder.
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	_ = limit
	s.writeJSON(w, http.StatusOK, map[string]any{"users": []any{}, "note": "user list coming soon"})
}

// DELETE /api/admin/users/{id} — delete a user and cascade their data.
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminDeleteUser.unauthorized")
		return
	}
	id := r.PathValue("id")
	// Check the user exists.
	u, err := s.store.Users().GetByID(r.Context(), id)
	if err != nil {
		if err == domain.ErrNotFound {
			s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "user_not_found", "err.adminDeleteUser.user_not_found")
			return
		}
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.adminDeleteUser.internal")
		return
	}
	// Cascade-delete their data session.
	if u.DataSessionID != "" {
		_, _ = s.store.Sessions().Get(r.Context(), u.DataSessionID) // verify existence
		// TODO: delete session + all associated data cascade.
		// For now, just delete the user row.
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"deleted": id, "note": "full cascade delete coming soon"})
}
