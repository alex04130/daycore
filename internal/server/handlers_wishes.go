package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"daycore/internal/domain"
)

func init() {
	// The inverses live next to the writes, which is where somebody looks for
	// them. Registration from init() means "linked in" == "undoable".
	registerRevert("wish_create", (*Server).revertWishCreate_delete)
	registerRevert("wish_update", (*Server).revertWishUpdate)
	registerRevert("wish_delete", (*Server).revertWishDelete)

	registerRoutes("wishes", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/wishes", s.handleWishList)
		mux.HandleFunc("POST /api/wishes", s.handleWishCreate)
		mux.HandleFunc("GET /api/wishes/{id}", s.handleWishGet)
		mux.HandleFunc("PATCH /api/wishes/{id}", s.handleWishUpdate)
		mux.HandleFunc("DELETE /api/wishes/{id}", s.handleWishDelete)
	})
}

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
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.wishList.internal")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.wishCreate.bad_request")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.wishCreate.internal")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "wish_not_found", "err.wishGet.wish_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.wishGet.internal")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "wish_not_found", "err.wishUpdate.wish_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.wishUpdate.internal")
		return
	}

	// Copy before merging: the merge below writes through `prev`, so without
	// this the "before" snapshot would be the after.
	before := *prev

	var in wishInput
	if err := s.readJSON(r, &in); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.wishUpdate.bad_request")
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
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "wish_not_found", "err.wishUpdate.wish_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.wishUpdate.internal2")
		return
	}
	// before/after, not a bare snapshot of the result. `Detail: updated` was
	// what this used to store, which made the operation un-undoable in the one
	// way that is invisible: the ledger row looked complete, the undo button was
	// there, and there was simply nothing in it to restore from.
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "wish_update", TargetID: updated.ID,
		Summary: updated.Title,
		Detail:  marshalCompact(revertDetail{Before: before, After: updated}),
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
	// One extra read, and it is the whole difference between an undo button that
	// works and one that returns 400. The row is gone after the next statement
	// and nothing else anywhere remembers what was in it — `Detail` used to hold
	// the id string and nothing more.
	before, err := s.store.Wishes().Get(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "wish_not_found", "err.wishDelete.wish_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.wishDelete.internal")
		return
	}
	if err := s.store.Wishes().Delete(r.Context(), sid, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "wish_not_found", "err.wishDelete.wish_not_found")
			return
		}
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.wishDelete.internal")
		return
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "wish_delete", TargetID: id,
		Summary: before.Title,
		Detail:  marshalCompact(revertDetail{Before: before}),
	})
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// revertWishUpdate restores the wish as it was before the patch.
func (s *Server) revertWishUpdate(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	var before domain.Wish
	if !decodeInto(detail.Before, &before) {
		s.writeErrL(w, locale, http.StatusUnprocessableEntity, "no_snapshot", "err.opRevert.no_snapshot")
		return
	}
	if _, err := s.store.Wishes().Update(ctx, sid, orig.TargetID, &before); err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.opRevert.internal")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}

// revertWishDelete puts the wish back, keeping its original id so anything that
// referenced it still resolves.
func (s *Server) revertWishDelete(ctx context.Context, w http.ResponseWriter, sid, locale string, orig *domain.OperationLog, detail revertDetail) {
	var before domain.Wish
	if !decodeInto(detail.Before, &before) {
		s.writeErrL(w, locale, http.StatusUnprocessableEntity, "no_snapshot", "err.opRevert.no_snapshot")
		return
	}
	before.SessionID = sid
	before.ID = orig.TargetID
	if _, err := s.store.Wishes().Create(ctx, &before); err != nil {
		s.writeErrL(w, locale, http.StatusInternalServerError, "internal", "err.opRevert.internal")
		return
	}
	s.finishRevert(ctx, w, sid, orig)
}
