package server

import (
	"errors"
	"net/http"
	"strings"

	"daycore/internal/domain"
)

// wishInput is the JSON shape accepted by create and update.
type wishInput struct {
	Title     string `json:"title"`
	Note      string `json:"note"`
	EffortMin *int   `json:"effortMin"`
	Status    string `json:"status"`
}

// GET /api/wishes — list wishes, optionally filtered by ?status=.
func (s *Server) handleWishList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	wishes, err := s.store.Wishes().List(r.Context(), sid, status)
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取心愿列表失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"wishes": wishes})
}

// POST /api/wishes — create a wish.
func (s *Server) handleWishCreate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var in wishInput
	if err := s.readJSON(r, &in); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "title is required")
		return
	}
	status := orDefault(in.Status, "active")
	effort := 0
	if in.EffortMin != nil {
		effort = *in.EffortMin
	}
	created, err := s.store.Wishes().Create(r.Context(), &domain.Wish{
		SessionID: sid,
		Title:     in.Title,
		Note:      strings.TrimSpace(in.Note),
		EffortMin: effort,
		Status:    status,
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "心愿创建失败")
		return
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "wish_create", TargetID: created.ID,
		Summary: created.Title, Detail: marshalCompact(created),
	})
	s.writeJSON(w, http.StatusOK, created)
}

// GET /api/wishes/{id} — get a single wish.
func (s *Server) handleWishGet(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	wish, err := s.store.Wishes().Get(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "wish_not_found", "没有这条心愿")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取心愿失败")
		return
	}
	s.writeJSON(w, http.StatusOK, wish)
}

// PATCH /api/wishes/{id} — partial update of a wish.
func (s *Server) handleWishUpdate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")

	// Read the existing wish so we can merge the patch over it.
	prev, err := s.store.Wishes().Get(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "wish_not_found", "没有这条心愿")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取心愿失败")
		return
	}

	var in wishInput
	if err := s.readJSON(r, &in); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}

	// Merge: only override fields that were explicitly sent.
	if title := strings.TrimSpace(in.Title); title != "" {
		prev.Title = title
	}
	if note := strings.TrimSpace(in.Note); note != "" {
		prev.Note = note
	}
	if in.EffortMin != nil {
		prev.EffortMin = *in.EffortMin
	}
	if s := strings.TrimSpace(in.Status); s != "" {
		prev.Status = s
	}

	updated, err := s.store.Wishes().Update(r.Context(), sid, id, prev)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "wish_not_found", "没有这条心愿")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "心愿更新失败")
		return
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "wish_update", TargetID: updated.ID,
		Summary: updated.Title, Detail: marshalCompact(updated),
	})
	s.writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/wishes/{id} — delete a wish.
func (s *Server) handleWishDelete(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	err := s.store.Wishes().Delete(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "wish_not_found", "没有这条心愿")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "心愿删除失败")
		return
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "wish_delete", TargetID: id,
		Summary: id, Detail: marshalCompact(id),
	})
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
