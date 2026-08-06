package server

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"daycore/internal/domain"
	"daycore/internal/i18n"
)

func init() {
	registerRoutes("admin (prompts)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/prompts", s.handleAdminPromptList)
		mux.HandleFunc("GET /api/admin/prompts/{key}", s.handleAdminPromptGet)
		mux.HandleFunc("PUT /api/admin/prompts/{key}", s.handleAdminPromptSet)
	})
}

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
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminPromptList.unauthorized")
		return
	}
	ctx := r.Context()
	out := []adminPromptItem{}
	for _, key := range s.prompts.Keys() {
		for _, locale := range i18n.Embedded {
			content, _, err := s.prompts.Get(ctx, key, locale)
			if err != nil {
				continue
			}
			def, _ := s.prompts.Default(key, locale)
			out = append(out, adminPromptItem{Key: key, Locale: locale, Content: content, HasOverride: content != def})
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"prompts": out, "locales": i18n.Embedded})
}

// GET /api/admin/prompts/{key}?locale= — one prompt's active content + built-in default.
func (s *Server) handleAdminPromptGet(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminPromptGet.unauthorized")
		return
	}
	locale := adminLocale(r)
	if locale == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "unsupported_locale", "err.adminPromptGet.unsupported_locale")
		return
	}
	key := r.PathValue("key")
	content, ok, err := s.prompts.Get(r.Context(), key, locale)
	if errors.Is(err, domain.ErrNotFound) || !ok {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "unknown_prompt", "err.adminPromptGet.unknown_prompt")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.adminPromptGet.internal")
		return
	}
	def, _ := s.prompts.Default(key, locale)
	s.writeJSON(w, http.StatusOK, map[string]any{"key": key, "locale": locale, "content": content, "default": def})
}

// PUT /api/admin/prompts/{key}?locale= — override a prompt (validated as a template).
func (s *Server) handleAdminPromptSet(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminPromptSet.unauthorized")
		return
	}
	locale := adminLocale(r)
	if locale == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "unsupported_locale", "err.adminPromptSet.unsupported_locale")
		return
	}
	key := r.PathValue("key")
	var body struct {
		Content string `json:"content"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Content == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.adminPromptSet.bad_request")
		return
	}
	err := s.prompts.Set(r.Context(), key, locale, body.Content)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "unknown_prompt", "err.adminPromptSet.unknown_prompt")
		return
	}
	if err != nil {
		s.writeErrf(w, s.requestLocale(r), http.StatusBadRequest, "invalid_template", "err.fmt.invalidTemplate", err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
