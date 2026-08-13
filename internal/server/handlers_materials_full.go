package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"daycore/internal/domain"
)

func init() {
	// 逆操作与写入放在同一个文件 —— 改写入的人正好看得见它。
	registerRevert("material_create", (*Server).revertMaterialCreate_delete)
	registerRevert("material_delete", (*Server).revertMaterialDelete)
	registerRevert("material_update", (*Server).revertMaterialUpdate)

	registerRoutes("materials", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/materials", s.handleMaterialList)
		mux.HandleFunc("POST /api/materials", s.handleMaterialCreate)
		mux.HandleFunc("GET /api/materials/search", s.handleMaterialSearch)
		mux.HandleFunc("GET /api/materials/categories", s.handleMaterialCategories)
		mux.HandleFunc("GET /api/materials/{id}", s.handleMaterialGet)
		mux.HandleFunc("PATCH /api/materials/{id}", s.handleMaterialUpdate)
		mux.HandleFunc("DELETE /api/materials/{id}", s.handleMaterialDelete)
	})
}

// GET /api/materials — list materials, optionally filtered by category and query.
func (s *Server) handleMaterialList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	category := q.Get("category")
	query := q.Get("q")
	limit, offset := 0, 0
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	materials, err := s.store.Materials().List(r.Context(), sid, category, query, limit, offset)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.materialList.internal")
		return
	}
	if materials == nil {
		materials = []domain.Material{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"materials": materials})
}

// POST /api/materials — create a new material.
func (s *Server) handleMaterialCreate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var in struct {
		Category   string   `json:"category"`
		Title      string   `json:"title"`
		Summary    string   `json:"summary"`
		Body       string   `json:"body"`
		Source     string   `json:"source"`
		MimeType   string   `json:"mime_type"`
		StorageRef string   `json:"storage_ref"`
		Tags       []string `json:"tags"`
	}
	if err := s.readJSON(r, &in); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.materialCreate.bad_request")
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.materialCreate.bad_request2")
		return
	}
	category, ok2 := normalizeCategory(in.Category)
	if !ok2 {
		s.writeErrf(w, s.requestLocale(r), http.StatusBadRequest, "bad_category", "err.fmt.badCategory", in.Category)
		return
	}
	m := &domain.Material{
		SessionID:  sid,
		Category:   category,
		Title:      strings.TrimSpace(in.Title),
		Summary:    in.Summary,
		Body:       in.Body,
		Source:     in.Source,
		MimeType:   in.MimeType,
		StorageRef: in.StorageRef,
		Tags:       in.Tags,
	}
	created, err := s.store.Materials().Create(r.Context(), m)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.materialCreate.internal")
		return
	}
	s.logOp(r.Context(), &domain.OperationLog{
		SessionID: sid, Action: "material_create", TargetID: created.ID,
		Summary: created.Title,
		Detail:  marshalCompact(map[string]any{"before": nil, "after": created}),
	})
	s.writeJSON(w, http.StatusOK, created)
}

// GET /api/materials/{id} — get a single material.
func (s *Server) handleMaterialGet(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	mat, err := s.store.Materials().Get(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "material_not_found", "err.materialGet.material_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.materialGet.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, mat)
}

// PATCH /api/materials/{id} — partial update of a material.
func (s *Server) handleMaterialUpdate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()
	existing, err := s.store.Materials().Get(ctx, sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "material_not_found", "err.materialUpdate.material_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.materialUpdate.internal")
		return
	}
	// ⚠️ A COPY, taken before the patch loop below mutates `existing` in place.
	// Logging `existing` as the before-snapshot would record the after-state
	// twice, and an undo built from that restores nothing while reporting
	// success — the worst of the available outcomes.
	before := *existing

	var in struct {
		Category   *string   `json:"category"`
		Title      *string   `json:"title"`
		Summary    *string   `json:"summary"`
		Body       *string   `json:"body"`
		Source     *string   `json:"source"`
		MimeType   *string   `json:"mime_type"`
		StorageRef *string   `json:"storage_ref"`
		Tags       *[]string `json:"tags"`
	}
	if err := s.readJSON(r, &in); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.materialUpdate.bad_request")
		return
	}
	if in.Category != nil {
		category, ok2 := normalizeCategory(*in.Category)
		if !ok2 {
			s.writeErrf(w, s.requestLocale(r), http.StatusBadRequest, "bad_category", "err.fmt.badCategory", *in.Category)
			return
		}
		existing.Category = category
	}
	if in.Title != nil {
		existing.Title = strings.TrimSpace(*in.Title)
	}
	if in.Summary != nil {
		existing.Summary = *in.Summary
	}
	if in.Body != nil {
		existing.Body = *in.Body
	}
	if in.Source != nil {
		existing.Source = *in.Source
	}
	if in.MimeType != nil {
		existing.MimeType = *in.MimeType
	}
	if in.StorageRef != nil {
		existing.StorageRef = *in.StorageRef
	}
	if in.Tags != nil {
		existing.Tags = *in.Tags
	}
	updated, err := s.store.Materials().Update(ctx, sid, id, existing)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.materialUpdate.internal2")
		return
	}
	s.logOp(ctx, &domain.OperationLog{
		SessionID: sid, Action: "material_update", TargetID: id,
		Summary: updated.Title,
		Detail:  marshalCompact(map[string]any{"before": before, "after": updated}),
	})
	s.writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/materials/{id} — delete a material.
func (s *Server) handleMaterialDelete(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	// ⚠️ Read it before deleting it. This is the only op in the group whose undo
	// needs the row itself — everything else can be reconstructed from an id —
	// and after the DELETE there is nowhere left to read it from.
	before, _ := s.store.Materials().Get(r.Context(), sid, id)
	err := s.store.Materials().Delete(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "material_not_found", "err.materialDelete.material_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.materialDelete.internal")
		return
	}
	if before != nil {
		s.logOp(r.Context(), &domain.OperationLog{
			SessionID: sid, Action: "material_delete", TargetID: id,
			Summary: before.Title,
			Detail:  marshalCompact(map[string]any{"before": before, "after": nil}),
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// normalizeCategory collapses writes onto the registry whitelist: empty → note,
// unknown → rejected. Reads stay untouched — legacy free-text categories remain
// listable and filterable, only new writes converge.
func normalizeCategory(category string) (string, bool) {
	category = strings.TrimSpace(category)
	if category == "" {
		return domain.CategoryNote, true
	}
	if _, ok := domain.MaterialCategoryByID(category); !ok {
		return "", false
	}
	return category, true
}

// enabledMaterialCategories resolves the session's effective category set:
// registry DefaultOn + per-session overrides; note is always on.
func (s *Server) enabledMaterialCategories(ctx context.Context, sid string) map[string]bool {
	prefs := s.sessionPrefs(ctx, sid)
	out := map[string]bool{}
	for _, c := range domain.MaterialCategories() {
		on := c.DefaultOn
		if v, ok := prefs.MaterialCategories[c.ID]; ok {
			on = v
		}
		out[c.ID] = on
	}
	out[domain.CategoryNote] = true
	return out
}

// GET /api/materials/categories — the full category registry with the
// session's enablement flags (frontends render the toggle panel from the full
// list and the capture UI from the enabled subset).
func (s *Server) handleMaterialCategories(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	locale := s.requestLocale(r)
	enabled := s.enabledMaterialCategories(r.Context(), sid)
	out := make([]map[string]any, 0, len(enabled))
	for _, c := range domain.MaterialCategories() {
		out = append(out, map[string]any{
			"id": c.ID, "name": c.Name(locale), "icon": c.Icon,
			"enabled": enabled[c.ID], "default": c.DefaultOn,
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"categories": out})
}

// GET /api/materials/search — full-text search across indexed materials using
// the Searcher interface. Returns 501 when no searcher is configured.
func (s *Server) handleMaterialSearch(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if s.searcher == nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotImplemented, "search_not_available", "err.materialSearch.search_not_available")
		return
	}
	q := r.URL.Query()
	term := q.Get("q")
	if strings.TrimSpace(term) == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.materialSearch.bad_request")
		return
	}
	sq := domain.SearchQuery{
		Term:     strings.TrimSpace(term),
		Category: q.Get("category"),
		Limit:    20,
		Offset:   0,
	}
	if v := q.Get("tags"); v != "" {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				sq.Tags = append(sq.Tags, t)
			}
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			sq.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			sq.Offset = n
		}
	}
	results, err := s.searcher.Search(r.Context(), sid, sq)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.materialSearch.internal")
		return
	}
	if results == nil {
		results = []domain.SearchResult{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
