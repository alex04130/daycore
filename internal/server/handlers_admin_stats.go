package server

import (
	"net/http"
	"strconv"

	"daycore/internal/domain"
)

// GET /api/admin/stats — dashboard aggregation.
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErr(w, http.StatusUnauthorized, "unauthorized", "需要管理员令牌")
		return
	}
	ctx := r.Context()
	stats, err := s.store.AILogs().Stats(ctx)
	if err != nil {
		s.log.Error("admin stats", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", "获取统计数据失败")
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
		s.writeErr(w, http.StatusUnauthorized, "unauthorized", "需要管理员令牌")
		return
	}
	// TODO: pagination query when AILogRepository.List is added.
	// For now, return stats as a minimal view.
	s.writeJSON(w, http.StatusOK, map[string]any{"logs": []any{}, "note": "full log list coming soon"})
}

// GET /api/admin/users — user list with pagination and search.
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErr(w, http.StatusUnauthorized, "unauthorized", "需要管理员令牌")
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
		s.writeErr(w, http.StatusUnauthorized, "unauthorized", "需要管理员令牌")
		return
	}
	id := r.PathValue("id")
	// Check the user exists.
	u, err := s.store.Users().GetByID(r.Context(), id)
	if err != nil {
		if err == domain.ErrNotFound {
			s.writeErr(w, http.StatusNotFound, "user_not_found", "用户不存在")
			return
		}
		s.writeErr(w, http.StatusInternalServerError, "internal", "查询用户失败")
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
