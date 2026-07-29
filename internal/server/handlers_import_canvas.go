package server

import (
	"fmt"
	"net/http"
	"time"

	"daycore/internal/domain"
)

func init() {
	registerRoutes("imports", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/import/canvas", s.handleImportCanvas)
	})
}

// canvasExportVersion is the wire contract shared with extension/export.js.
const canvasExportVersion = "daycore.canvas.v1"

type canvasExportCourse struct {
	CanvasID     string   `json:"canvasId"`
	Name         string   `json:"name"`
	CourseCode   string   `json:"courseCode"`
	CurrentScore *float64 `json:"currentScore"`
	CurrentGrade *string  `json:"currentGrade"`
}

type canvasExportAssignment struct {
	CanvasID       string   `json:"canvasId"`
	CourseCanvasID string   `json:"courseCanvasId"`
	Title          string   `json:"title"`
	DueAt          *string  `json:"dueAt"` // RFC3339
	PointsPossible *float64 `json:"pointsPossible"`
	Submitted      bool     `json:"submitted"`
	Graded         bool     `json:"graded"`
	Score          *float64 `json:"score"`
	HTMLURL        string   `json:"htmlUrl"`
}

// POST /api/import/canvas — ingest the browser extension's export (uploaded as
// a file through the frontend, or pushed directly with X-Import-Token).
func (s *Server) handleImportCanvas(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.importSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Version     string                   `json:"version"`
		BaseURL     string                   `json:"baseURL"`
		Courses     []canvasExportCourse     `json:"courses"`
		Assignments []canvasExportAssignment `json:"assignments"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if body.Version != canvasExportVersion {
		s.writeErr(w, http.StatusBadRequest, "unsupported_export_version",
			fmt.Sprintf("导出文件版本不支持（%q，期望 %q）", body.Version, canvasExportVersion))
		return
	}

	ctx := r.Context()
	warnings := []string{}
	courseIDByCanvas := map[string]string{}
	for i, c := range body.Courses {
		if c.CanvasID == "" || c.Name == "" {
			warnings = append(warnings, fmt.Sprintf("courses[%d]: 缺少 canvasId/name，已跳过", i))
			continue
		}
		saved, err := s.store.Courses().UpsertByCanvasID(ctx, &domain.Course{
			SessionID: sid, CanvasID: c.CanvasID, Name: c.Name, CourseCode: c.CourseCode,
			CurrentScore: c.CurrentScore, CurrentGrade: c.CurrentGrade,
		})
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, "internal", "课程保存失败")
			return
		}
		courseIDByCanvas[c.CanvasID] = saved.ID
	}

	imported := 0
	for i, a := range body.Assignments {
		if a.CanvasID == "" || a.Title == "" {
			warnings = append(warnings, fmt.Sprintf("assignments[%d]: 缺少 canvasId/title，已跳过", i))
			continue
		}
		var dueAt *time.Time
		if a.DueAt != nil && *a.DueAt != "" {
			t, err := time.Parse(time.RFC3339, *a.DueAt)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("assignments[%d]: dueAt 无法解析（%q）", i, *a.DueAt))
			} else {
				dueAt = &t
			}
		}
		_, err := s.store.Assignments().UpsertByCanvasID(ctx, &domain.Assignment{
			SessionID: sid, CourseID: courseIDByCanvas[a.CourseCanvasID], CanvasID: a.CanvasID,
			Title: a.Title, DueAt: dueAt, PointsPossible: a.PointsPossible,
			Submitted: a.Submitted, Graded: a.Graded, Score: a.Score,
			HTMLURL: a.HTMLURL, Source: "canvas",
		})
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, "internal", "作业保存失败")
			return
		}
		imported++
	}
	s.recordImport(ctx, sid, "canvas", len(courseIDByCanvas)+imported,
		fmt.Sprintf("%d courses, %d assignments (%s)", len(courseIDByCanvas), imported, body.BaseURL),
		marshalCompact(body))
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "courses": len(courseIDByCanvas), "assignments": imported, "warnings": warnings,
	})
}
