package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"daycore/internal/domain"

	"github.com/google/uuid"
)

// GET /api/courses — the session's imported Canvas courses with grades.
func (s *Server) handleCourseList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	courses, err := s.store.Courses().List(r.Context(), sid)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取课程失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"courses": courses})
}

// GET /api/assignments?from=&to=&status= — assignments, optionally filtered by
// due-date window (YYYY-MM-DD, inclusive) and planner status.
func (s *Server) handleAssignmentList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	var f domain.AssignmentFilter
	if v := q.Get("from"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			s.writeErr(w, http.StatusBadRequest, "bad_request", "from 格式应为 YYYY-MM-DD")
			return
		}
		f.DueFrom = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			s.writeErr(w, http.StatusBadRequest, "bad_request", "to 格式应为 YYYY-MM-DD")
			return
		}
		end := t.Add(24*time.Hour - time.Second) // inclusive end of day
		f.DueTo = &end
	}
	if v := q.Get("status"); v != "" {
		if !validAssignmentStatuses[v] {
			s.writeErr(w, http.StatusBadRequest, "bad_request", "无效的 status")
			return
		}
		f.Status = v
	}
	items, err := s.store.Assignments().List(r.Context(), sid, f)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取作业失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"assignments": items})
}

var validAssignmentStatuses = map[string]bool{
	domain.AssignmentPending: true, domain.AssignmentPlanned: true,
	domain.AssignmentDone: true, domain.AssignmentDismissed: true,
}

// POST /api/assignments — manually add a deadline (the quick-capture academic
// route and the assignments screen's "add" button).
func (s *Server) handleAssignmentCreate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Title    string `json:"title"`
		DueAt    string `json:"dueAt"`
		CourseID string `json:"courseId"`
	}
	if err := s.readJSON(r, &body); err != nil || strings.TrimSpace(body.Title) == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 title")
		return
	}
	var dueAt *time.Time
	if body.DueAt != "" {
		t, err := parseFlexibleTime(body.DueAt)
		if err != nil {
			s.writeErr(w, http.StatusBadRequest, "bad_request", "dueAt 格式应为 RFC3339、YYYY-MM-DDTHH:MM 或 YYYY-MM-DD")
			return
		}
		dueAt = &t
	}
	a, err := s.createManualAssignment(r.Context(), sid, strings.TrimSpace(body.Title), body.CourseID, dueAt)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "作业创建失败")
		return
	}
	s.writeJSON(w, http.StatusOK, a)
}

// createManualAssignment reuses UpsertByCanvasID with a unique "manual:" canvas
// id — the UPDATE-by-canvas-id always misses so it lands on the INSERT branch,
// no new repository method needed, and Canvas re-imports never touch manual
// rows (their canvas ids can't collide).
func (s *Server) createManualAssignment(ctx context.Context, sid, title, courseID string, dueAt *time.Time) (*domain.Assignment, error) {
	return s.store.Assignments().UpsertByCanvasID(ctx, &domain.Assignment{
		SessionID: sid,
		CourseID:  courseID,
		CanvasID:  "manual:" + uuid.NewString(),
		Title:     title,
		DueAt:     dueAt,
		Source:    "manual",
		Status:    domain.AssignmentPending,
	})
}

// parseFlexibleTime accepts the due-date formats quick captures produce.
func parseFlexibleTime(v string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time %q", v)
}

// PATCH /api/assignments/{id} — update the planner workflow status
// ("pending" | "planned" | "done" | "dismissed").
func (s *Server) handleAssignmentPatch(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := s.readJSON(r, &body); err != nil || !validAssignmentStatuses[body.Status] {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "status 必须是 pending/planned/done/dismissed 之一")
		return
	}
	err := s.store.Assignments().SetStatus(r.Context(), sid, id, body.Status)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "assignment_not_found", "没有这条作业")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "作业更新失败")
		return
	}
	a, err := s.store.Assignments().Get(r.Context(), sid, id)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "作业读取失败")
		return
	}
	s.writeJSON(w, http.StatusOK, a)
}
