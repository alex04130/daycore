package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"daycore/internal/domain"
)

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
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取素材失败")
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
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if strings.TrimSpace(in.Title) == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "标题不能为空")
		return
	}
	category, ok2 := normalizeCategory(in.Category)
	if !ok2 {
		s.writeErr(w, http.StatusBadRequest, "bad_category", "未知的资料类别: "+in.Category)
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
		s.writeErr(w, http.StatusInternalServerError, "internal", "素材创建失败")
		return
	}
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
		s.writeErr(w, http.StatusNotFound, "material_not_found", "没有这个素材")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取素材失败")
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
		s.writeErr(w, http.StatusNotFound, "material_not_found", "没有这个素材")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取素材失败")
		return
	}
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
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if in.Category != nil {
		category, ok2 := normalizeCategory(*in.Category)
		if !ok2 {
			s.writeErr(w, http.StatusBadRequest, "bad_category", "未知的资料类别: "+*in.Category)
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
		s.writeErr(w, http.StatusInternalServerError, "internal", "素材更新失败")
		return
	}
	s.writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/materials/{id} — delete a material.
func (s *Server) handleMaterialDelete(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	err := s.store.Materials().Delete(r.Context(), sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "material_not_found", "没有这个素材")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "素材删除失败")
		return
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
		name := c.NameEN
		if strings.HasPrefix(locale, "zh") {
			name = c.NameZH
		}
		out = append(out, map[string]any{
			"id": c.ID, "name": name, "icon": c.Icon,
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
		s.writeErr(w, http.StatusNotImplemented, "search_not_available", "搜索服务未配置")
		return
	}
	q := r.URL.Query()
	term := q.Get("q")
	if strings.TrimSpace(term) == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "搜索关键词不能为空")
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
		s.writeErr(w, http.StatusInternalServerError, "internal", "搜索失败")
		return
	}
	if results == nil {
		results = []domain.SearchResult{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
