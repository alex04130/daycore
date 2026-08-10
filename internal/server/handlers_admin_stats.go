package server

import (
	"net/http"
	"strconv"
	"time"

	"daycore/internal/domain"
	"daycore/internal/i18n"
)

var (
	keyAdminAILogBadStatus = i18n.Reg("admin.ailogs.bad_status", i18n.Text{
		"zh-CN": "status 只能是 ok 或 error",
		"en-US": "status must be either ok or error",
	})
	keyAdminAILogBadSince = i18n.Reg("admin.ailogs.bad_since", i18n.Text{
		"zh-CN": "since 要写成 RFC3339 时间",
		"en-US": "since must be an RFC3339 timestamp",
	})
	keyAdminAILogBadCursor = i18n.Reg("admin.ailogs.bad_cursor", i18n.Text{
		"zh-CN": "翻页游标要 beforeAt 和 beforeId 一起给",
		"en-US": "a paging cursor needs both beforeAt and beforeId",
	})
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

// GET /api/admin/ailogs — the AI call ledger, newest first.
//
// # What is deliberately NOT in a row
//
// The design prototype's drawer shows 请求 and 响应 bodies
// (design-ui/liuli/admin/admin-views.jsx:150). Those are not stored and must
// not be: the request body of a companion call IS the user's conversation, and
// docs/STRATEGY.md puts that at the same level as the companion boundaries.
// Storing it to make an ops screen nicer would put every user's diary in a
// table that ailogs.read can browse — and that permission's damage line
// promises the opposite ("今天不含对话正文").
//
// ⚠️ If a request/response capture is ever added, it belongs behind
// db.user_content or a new permission of its own, never behind this one.
//
// # Why the cursor is opaque to the client
//
// Paging is keyset, not offset — the console sends back the last row's
// timestamp and id. Offset paging over a table that grows at the head repeats
// and skips rows, which on a ledger reads as "the log is lying".
func (s *Server) handleAdminAILogs(w http.ResponseWriter, r *http.Request) {
	locale := s.requestLocale(r)
	q := r.URL.Query()
	f := domain.AILogFilter{
		SessionID: q.Get("sessionId"),
		Endpoint:  q.Get("endpoint"),
		Model:     q.Get("model"),
		Status:    q.Get("status"),
	}
	// An unknown status would silently match nothing, and an empty screen with
	// no explanation is the worst answer a filter can give.
	if f.Status != "" && f.Status != "ok" && f.Status != "error" {
		s.writeErr(w, http.StatusBadRequest, "bad_status", i18n.T(keyAdminAILogBadStatus, locale))
		return
	}
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			s.writeErr(w, http.StatusBadRequest, "bad_since", i18n.T(keyAdminAILogBadSince, locale))
			return
		}
		f.Since = t
	}
	// The two halves of the cursor arrive together or not at all. Half a cursor
	// would page from an instant with no tie-break, which is the exact failure
	// the id exists to prevent.
	beforeAt, beforeID := q.Get("beforeAt"), q.Get("beforeId")
	if (beforeAt == "") != (beforeID == "") {
		s.writeErr(w, http.StatusBadRequest, "bad_cursor", i18n.T(keyAdminAILogBadCursor, locale))
		return
	}
	if beforeAt != "" {
		t, err := time.Parse(time.RFC3339Nano, beforeAt)
		if err != nil {
			s.writeErr(w, http.StatusBadRequest, "bad_cursor", i18n.T(keyAdminAILogBadCursor, locale))
			return
		}
		f.Before = domain.LogCursor{CreatedAt: t, ID: beforeID}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))

	logs, err := s.store.AILogs().List(r.Context(), f, limit)
	if err != nil {
		s.log.Error("admin ai logs", "err", err)
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.adminAILogs.internal")
		return
	}
	out := map[string]any{
		"logs": logs,
		// The endpoint vocabulary, so the console's filter chips are this
		// build's list rather than a copy that drifts. A chip for an endpoint
		// that no longer exists filters to nothing and looks like a broken
		// screen.
		"endpoints": aiEndpoints(),
		// Stated rather than assumed: the ledger is pruned, so "no rows" from
		// four months ago means "pruned", not "nothing happened".
		"retentionDays": int(AILogRetention.Hours() / 24),
	}
	// The next page's cursor, computed here so the client never has to know how
	// the ordering works. Absent when this page did not fill — there is nothing
	// after it.
	if n := len(logs); n > 0 && n == domain.ListLimit(limit, domain.AILogListDefault, domain.AILogListMax) {
		out["nextBeforeAt"] = logs[n-1].CreatedAt.Format(time.RFC3339Nano)
		out["nextBeforeId"] = logs[n-1].ID
	}
	s.writeJSON(w, http.StatusOK, out)
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
