package server

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"daycore/internal/domain"
	"daycore/internal/i18n"
)

// adminAuthorized gates the prompt-editing endpoints. With ADMIN_TOKEN set, the
// X-Admin-Token header must match (constant-time); unset means open in dev,
// closed in prod.
func (s *Server) adminAuthorized(r *http.Request) bool {
	if s.cfg.AdminToken == "" {
		return !s.cfg.IsProduction()
	}
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Admin-Token")), []byte(s.cfg.AdminToken)) == 1
}

// adminLocale reads the ?locale= query param, defaulting to i18n.Default.
// Returns "" when an unsupported locale was requested explicitly.
func adminLocale(r *http.Request) string {
	q := r.URL.Query().Get("locale")
	if q == "" {
		return i18n.Default
	}
	return i18n.Normalize(q)
}

type adminPromptItem struct {
	Key         string `json:"key"`
	Locale      string `json:"locale"`
	Content     string `json:"content"`
	HasOverride bool   `json:"hasOverride"`
}

// GET /api/admin/prompts — every prompt key × supported locale with its active content.
func (s *Server) handleAdminPromptList(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErr(w, http.StatusUnauthorized, "unauthorized", "需要管理员令牌")
		return
	}
	ctx := r.Context()
	out := []adminPromptItem{}
	for _, key := range s.prompts.Keys() {
		for _, locale := range i18n.Supported {
			content, _, err := s.prompts.Get(ctx, key, locale)
			if err != nil {
				continue
			}
			def, _ := s.prompts.Default(key, locale)
			out = append(out, adminPromptItem{Key: key, Locale: locale, Content: content, HasOverride: content != def})
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"prompts": out, "locales": i18n.Supported})
}

// GET /api/admin/prompts/{key}?locale= — one prompt's active content + built-in default.
func (s *Server) handleAdminPromptGet(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErr(w, http.StatusUnauthorized, "unauthorized", "需要管理员令牌")
		return
	}
	locale := adminLocale(r)
	if locale == "" {
		s.writeErr(w, http.StatusBadRequest, "unsupported_locale", "不支持的 locale")
		return
	}
	key := r.PathValue("key")
	content, ok, err := s.prompts.Get(r.Context(), key, locale)
	if errors.Is(err, domain.ErrNotFound) || !ok {
		s.writeErr(w, http.StatusNotFound, "unknown_prompt", "没有这个提示词")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "读取提示词失败")
		return
	}
	def, _ := s.prompts.Default(key, locale)
	s.writeJSON(w, http.StatusOK, map[string]any{"key": key, "locale": locale, "content": content, "default": def})
}

// PUT /api/admin/prompts/{key}?locale= — override a prompt (validated as a template).
func (s *Server) handleAdminPromptSet(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErr(w, http.StatusUnauthorized, "unauthorized", "需要管理员令牌")
		return
	}
	locale := adminLocale(r)
	if locale == "" {
		s.writeErr(w, http.StatusBadRequest, "unsupported_locale", "不支持的 locale")
		return
	}
	key := r.PathValue("key")
	var body struct {
		Content string `json:"content"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Content == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 content")
		return
	}
	err := s.prompts.Set(r.Context(), key, locale, body.Content)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErr(w, http.StatusNotFound, "unknown_prompt", "没有这个提示词")
		return
	}
	if err != nil {
		s.writeErr(w, http.StatusBadRequest, "invalid_template", "提示词模板无效："+err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
