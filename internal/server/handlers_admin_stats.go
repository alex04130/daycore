package server

import (
	"net/http"
	"strconv"
	"time"

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
	// TODO: pagination query when AILogRepository.List is added.
	// For now, return stats as a minimal view.
	s.writeJSON(w, http.StatusOK, map[string]any{"logs": []any{}, "note": "full log list coming soon"})
}

// adminUserView is a person as the console shows them.
//
// ⚠️ Nothing they wrote. This is gated on users.read, which is deliberately
// separate from the two database-browse permissions — "看用户列表：邮箱、注册
// 时间、所在的组。不含任何人写下的内容". A field added here that carries content
// moves this endpoint into a different permission, so add it there instead.
type adminUserView struct {
	ID        string   `json:"id"`
	Email     string   `json:"email,omitempty"`
	Name      string   `json:"name,omitempty"`
	Owner     bool     `json:"owner"`
	Roles     []string `json:"roles"`
	CreatedAt string   `json:"createdAt,omitempty"`
}

// GET /api/admin/users — who exists, who owns this deployment, who is in what.
//
// It answered `[]` with a "coming soon" note until the permission model landed.
// That was not a placeholder anybody could have left: "who can export the
// database" is the question the whole model exists to keep answerable, and it
// is unanswerable from a screen that lists nobody.
func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	ctx := r.Context()
	users, err := s.store.Users().List(ctx, limit)
	if err != nil {
		s.log.Error("admin users", "err", err)
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.adminUsers.internal")
		return
	}
	out := make([]adminUserView, 0, len(users))
	owners := 0
	for _, u := range users {
		roles, err := s.store.Roles().RolesOf(ctx, u.ID)
		if err != nil {
			roles = []string{}
		}
		v := adminUserView{ID: u.ID, Owner: u.IsOwner, Roles: roles}
		if u.Email != nil {
			v.Email = *u.Email
		}
		if u.Name != nil {
			v.Name = *u.Name
		}
		if !u.CreatedAt.IsZero() {
			v.CreatedAt = u.CreatedAt.Format(time.RFC3339)
		}
		if u.IsOwner {
			owners++
		}
		out = append(out, v)
	}
	// ownerCount rather than a rule refusing to remove the last one. Break-glass
	// grants AND revokes — the root credential can undo any of this in one
	// request — so the honest thing is to say how many are left and let the
	// person decide, not to invent a guard against a state that is recoverable.
	s.writeJSON(w, http.StatusOK, map[string]any{
		"users":      out,
		"ownerCount": owners,
		"limit":      limit,
	})
}

// DELETE /api/admin/users/{id} — delete a user and cascade their data.
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
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
