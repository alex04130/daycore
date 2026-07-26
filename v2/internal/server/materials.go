package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"daycore/internal/domain"
)

// companionMaterials builds the compact assignments/rules context strings for
// the companion prompt. Both are JSON so they stay language-neutral; failures
// degrade to empty context rather than blocking the chat.
func (s *Server) companionMaterials(ctx context.Context, sid string) (assignmentsCtx, rulesCtx string) {
	assignmentsCtx, rulesCtx = "[]", "[]"
	if sid == "" {
		return
	}
	if items := s.upcomingAssignments(ctx, sid, s.cfg.AssignmentLookaheadDays); items != nil {
		assignmentsCtx = marshalCompact(assignmentSummaries(items))
	}
	if rules, err := s.store.Rules().List(ctx, sid); err == nil {
		active := []map[string]any{}
		for _, r := range rules {
			if !r.Active {
				continue
			}
			active = append(active, ruleSummary(r))
		}
		rulesCtx = marshalCompact(active)
	}
	return
}

// activeWishesContext renders the session's active wishes as compact JSON for
// the companion prompt ("[]" when none), so the assistant can proactively
// suggest something from the wish pool (the wish ↔ mood linkage).
func (s *Server) activeWishesContext(ctx context.Context, sid string) string {
	if sid == "" {
		return "[]"
	}
	wishes, err := s.store.Wishes().List(ctx, sid, "active")
	if err != nil || len(wishes) == 0 {
		return "[]"
	}
	out := make([]map[string]any, 0, len(wishes))
	for _, w := range wishes {
		m := map[string]any{"title": w.Title}
		if w.Note != "" {
			m["note"] = w.Note
		}
		if w.EffortMin > 0 {
			m["effortMin"] = w.EffortMin
		}
		out = append(out, m)
	}
	return marshalCompact(out)
}

// upcomingAssignments lists unsubmitted, plannable assignments due between
// yesterday and now+days (nil on storage errors).
func (s *Server) upcomingAssignments(ctx context.Context, sid string, days int) []domain.Assignment {
	now := time.Now()
	from := now.Add(-24 * time.Hour)
	to := now.Add(time.Duration(days) * 24 * time.Hour)
	items, err := s.store.Assignments().List(ctx, sid, domain.AssignmentFilter{DueFrom: &from, DueTo: &to})
	if err != nil {
		return nil
	}
	out := items[:0]
	for _, a := range items {
		if a.Status == domain.AssignmentDone || a.Status == domain.AssignmentDismissed {
			continue
		}
		out = append(out, a)
	}
	return out
}

func assignmentSummaries(items []domain.Assignment) []map[string]any {
	out := []map[string]any{}
	for _, a := range items {
		m := map[string]any{"title": a.Title, "submitted": a.Submitted}
		if a.DueAt != nil {
			m["due"] = a.DueAt.Format(time.RFC3339)
		}
		if a.PointsPossible != nil {
			m["points"] = *a.PointsPossible
		}
		if a.CourseID != "" {
			m["course_id"] = a.CourseID
		}
		out = append(out, m)
	}
	return out
}

func ruleSummary(r domain.ScheduleRule) map[string]any {
	m := map[string]any{"id": r.ID, "title": r.Title, "kind": r.Kind, "type": r.Type}
	if r.Kind == domain.RuleOnce {
		if r.Date != nil {
			m["date"] = *r.Date
		}
	} else {
		m["freq"] = r.Freq
		m["interval"] = r.Interval
		if len(r.ByWeekday) > 0 {
			m["by_weekday"] = r.ByWeekday
		}
		if r.StartDate != "" {
			m["start_date"] = r.StartDate
		}
		if r.Until != nil {
			m["until"] = *r.Until
		}
	}
	if r.Time != nil {
		m["time"] = *r.Time
	}
	if r.DurationMin != nil {
		m["duration_min"] = *r.DurationMin
	}
	return m
}

// assignmentsMarkdown renders upcoming assignments as a markdown table for the
// auto-plan prompt, joining course names for readability.
func assignmentsMarkdown(items []domain.Assignment, courses []domain.Course) string {
	if len(items) == 0 {
		return "(none)"
	}
	nameByID := map[string]string{}
	for _, c := range courses {
		nameByID[c.ID] = c.Name
	}
	var b strings.Builder
	b.WriteString("| Assignment | Course | Due | Points | Submitted |\n|---|---|---|---|---|\n")
	for _, a := range items {
		due, points, submitted := "-", "-", "no"
		if a.DueAt != nil {
			due = a.DueAt.Format("2006-01-02 15:04 MST")
		}
		if a.PointsPossible != nil {
			points = fmt.Sprintf("%.0f", *a.PointsPossible)
		}
		if a.Submitted {
			submitted = "yes"
		}
		course := nameByID[a.CourseID]
		if course == "" {
			course = "-"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			sanitizeCell(a.Title), sanitizeCell(course), due, points, submitted)
	}
	return b.String()
}

// coursesMarkdown renders the course/grade snapshot for the auto-plan prompt.
func coursesMarkdown(courses []domain.Course) string {
	if len(courses) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for _, c := range courses {
		line := "- " + sanitizeCell(c.Name)
		if c.CourseCode != "" {
			line += " (" + c.CourseCode + ")"
		}
		if c.CurrentScore != nil {
			line += fmt.Sprintf(": %.1f", *c.CurrentScore)
			if c.CurrentGrade != nil {
				line += " / " + *c.CurrentGrade
			}
		} else if c.CurrentGrade != nil {
			line += ": " + *c.CurrentGrade
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// sanitizeCell keeps user-supplied text from breaking markdown tables.
func sanitizeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

func marshalCompact(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
