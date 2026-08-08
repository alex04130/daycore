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

func init() {
	// 逆操作与写入放在同一个文件 —— 改写入的人正好看得见它。
	// 作业的写入方今天是 tool_capture.go，但这个实体的 REST 面归本文件。
	registerRevert("assignment_upsert", (*Server).revertAssignmentUpsert)

	registerRoutes("canvas materials", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/courses", s.handleCourseList)
		mux.HandleFunc("GET /api/assignments", s.handleAssignmentList)
		mux.HandleFunc("POST /api/assignments", s.handleAssignmentCreate)
		mux.HandleFunc("PATCH /api/assignments/{id}", s.handleAssignmentPatch)
	})
}

// GET /api/courses — the session's imported Canvas courses with grades.
func (s *Server) handleCourseList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	courses, err := s.store.Courses().List(r.Context(), sid)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.courseList.internal")
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
			s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.assignmentList.bad_request")
			return
		}
		f.DueFrom = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.assignmentList.bad_request2")
			return
		}
		end := t.Add(24*time.Hour - time.Second) // inclusive end of day
		f.DueTo = &end
	}
	if v := q.Get("status"); v != "" {
		if !validAssignmentStatuses[v] {
			s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.assignmentList.bad_request3")
			return
		}
		f.Status = v
	}
	items, err := s.store.Assignments().List(r.Context(), sid, f)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.assignmentList.internal")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.assignmentCreate.bad_request")
		return
	}
	var dueAt *time.Time
	if body.DueAt != "" {
		t, err := parseFlexibleTime(body.DueAt)
		if err != nil {
			s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.assignmentCreate.bad_request2")
			return
		}
		dueAt = &t
	}
	a, err := s.createManualAssignment(r.Context(), sid, strings.TrimSpace(body.Title), body.CourseID, dueAt)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.assignmentCreate.internal")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.assignmentPatch.bad_request")
		return
	}
	err := s.store.Assignments().SetStatus(r.Context(), sid, id, body.Status)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "assignment_not_found", "err.assignmentPatch.assignment_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.assignmentPatch.internal")
		return
	}
	a, err := s.store.Assignments().Get(r.Context(), sid, id)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.assignmentPatch.internal2")
		return
	}
	s.writeJSON(w, http.StatusOK, a)
}
