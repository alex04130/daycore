package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"daycore/internal/domain"
)

func init() {
	registerRoutes("custom themes", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/themes", s.handleThemeList)
		mux.HandleFunc("POST /api/themes", s.handleThemeCreate)
		mux.HandleFunc("PATCH /api/themes/{id}", s.handleThemePatch)
		mux.HandleFunc("DELETE /api/themes/{id}", s.handleThemeDelete)
	})
}

// themeVarWhitelist is the design system's themeable token set (kept in sync
// with api/FRONTEND_HANDOFF.md §5). Anything outside it is rejected so stored
// themes can never smuggle arbitrary CSS into the page.
var themeVarWhitelist = map[string]string{
	"--primary":        "主色（按钮/选中态）",
	"--accent":         "强调色（次级高亮）",
	"--bg-start":       "背景渐变起点",
	"--bg-end":         "背景渐变终点",
	"--text-primary":   "正文主色",
	"--text-secondary": "次级文字",
	"--text-muted":     "弱化文字",
	"--surface":        "卡片表面色（半透明）",
	"--surface-hover":  "卡片 hover 表面色",
	"--success":        "成功状态色",
	"--warning":        "警告状态色",
	"--error":          "危险状态色",
}

// themeColorRe accepts only plain color literals: hex, rgb()/rgba(),
// hsl()/hsla(), or "transparent". The tight charset (no ";", "}", "u", …)
// makes CSS/HTML injection through a variable value impossible.
var themeColorRe = regexp.MustCompile(
	`^(#[0-9a-fA-F]{3,8}|(rgb|rgba|hsl|hsla)\([0-9.,%\s/deg]+\)|transparent)$`)

// builtinThemePresets mirrors the four shipped themes' tokens so AI generation
// can start from a real base. Values follow FRONTEND_HANDOFF §5.1/§5.2.
var builtinThemePresets = map[string]domain.CustomTheme{
	"sky": {Name: "天空蓝", Dark: false, Variables: map[string]string{
		"--primary": "#3b82f6", "--accent": "#60a5fa",
		"--bg-start": "#e0f2fe", "--bg-end": "#f0f9ff",
		"--text-primary": "#0f172a", "--text-secondary": "#334155", "--text-muted": "#64748b",
		"--surface": "rgba(255,255,255,0.72)", "--surface-hover": "rgba(255,255,255,0.88)",
		"--success": "#22c55e", "--warning": "#f59e0b", "--error": "#ef4444",
	}},
	"sunset": {Name: "暖橙日落", Dark: false, Variables: map[string]string{
		"--primary": "#f97316", "--accent": "#fb923c",
		"--bg-start": "#fff7ed", "--bg-end": "#fef3c7",
		"--text-primary": "#1c1917", "--text-secondary": "#44403c", "--text-muted": "#78716c",
		"--surface": "rgba(255,255,255,0.72)", "--surface-hover": "rgba(255,255,255,0.88)",
		"--success": "#22c55e", "--warning": "#f59e0b", "--error": "#ef4444",
	}},
	"night": {Name: "深夜紫", Dark: true, Variables: map[string]string{
		"--primary": "#a78bfa", "--accent": "#c4b5fd",
		"--bg-start": "#1e1b4b", "--bg-end": "#312e81",
		"--text-primary": "#f0edff", "--text-secondary": "#c4b5fd", "--text-muted": "#8b7fc2",
		"--surface": "rgba(55,50,120,0.75)", "--surface-hover": "rgba(70,64,140,0.85)",
		"--success": "#22c55e", "--warning": "#f59e0b", "--error": "#ef4444",
	}},
	"nature": {Name: "自然绿", Dark: false, Variables: map[string]string{
		"--primary": "#16a34a", "--accent": "#22c55e",
		"--bg-start": "#f0fdf4", "--bg-end": "#ecfdf5",
		"--text-primary": "#052e16", "--text-secondary": "#14532d", "--text-muted": "#4d7c5f",
		"--surface": "rgba(255,255,255,0.72)", "--surface-hover": "rgba(255,255,255,0.88)",
		"--success": "#22c55e", "--warning": "#f59e0b", "--error": "#ef4444",
	}},
}

// validateThemeVariables checks every entry against the whitelist and the
// color-literal grammar. Empty maps are rejected (a theme must change something).
func validateThemeVariables(vars map[string]string) error {
	if len(vars) == 0 {
		return fmt.Errorf("variables must not be empty")
	}
	for k, v := range vars {
		if _, ok := themeVarWhitelist[k]; !ok {
			return fmt.Errorf("unknown variable %q", k)
		}
		if !themeColorRe.MatchString(strings.TrimSpace(v)) {
			return fmt.Errorf("invalid color value for %s: %q", k, v)
		}
	}
	return nil
}

// sanitizeThemeVariables keeps only whitelisted keys with valid color values,
// reporting what was dropped (used on AI output, which must degrade gracefully
// rather than hard-fail).
func sanitizeThemeVariables(vars map[string]string) (map[string]string, []string) {
	out := map[string]string{}
	var dropped []string
	for k, v := range vars {
		v = strings.TrimSpace(v)
		if _, ok := themeVarWhitelist[k]; !ok || !themeColorRe.MatchString(v) {
			dropped = append(dropped, k)
			continue
		}
		out[k] = v
	}
	return out, dropped
}

type themeInput struct {
	Name      string            `json:"name"`
	Base      string            `json:"base"`
	Dark      *bool             `json:"dark"`
	Variables map[string]string `json:"variables"`
}

// GET /api/themes — the session's custom themes plus the builtin ids.
func (s *Server) handleThemeList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	themes, err := s.store.Themes().List(r.Context(), sid)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.themeList.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"themes": themes, "builtin": domain.BuiltinThemes,
	})
}

// POST /api/themes — create a custom theme (validated against the whitelist).
func (s *Server) handleThemeCreate(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var in themeInput
	if err := s.readJSON(r, &in); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.themeCreate.bad_request")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		s.writeErr(w, http.StatusBadRequest, "invalid_theme", "name is required")
		return
	}
	if in.Base != "" {
		if _, ok := builtinThemePresets[in.Base]; !ok {
			s.writeErr(w, http.StatusBadRequest, "invalid_theme", "base must be one of the builtin theme ids")
			return
		}
	}
	if err := validateThemeVariables(in.Variables); err != nil {
		s.writeErr(w, http.StatusBadRequest, "invalid_theme", err.Error())
		return
	}
	dark := false
	if in.Dark != nil {
		dark = *in.Dark
	}
	created, err := s.store.Themes().Create(r.Context(), &domain.CustomTheme{
		SessionID: sid, Name: in.Name, Base: in.Base, Dark: dark, Variables: in.Variables,
	})
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.themeCreate.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, created)
}

// PATCH /api/themes/{id} — partial update (name / dark / variables).
func (s *Server) handleThemePatch(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var in themeInput
	if err := s.readJSON(r, &in); err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.themePatch.bad_request")
		return
	}
	var upd domain.CustomThemeUpdate
	if name := strings.TrimSpace(in.Name); name != "" {
		upd.Name = &name
	}
	upd.Dark = in.Dark
	if in.Variables != nil {
		if err := validateThemeVariables(in.Variables); err != nil {
			s.writeErr(w, http.StatusBadRequest, "invalid_theme", err.Error())
			return
		}
		upd.Variables = &in.Variables
	}
	updated, err := s.store.Themes().Update(r.Context(), sid, id, upd)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "theme_not_found", "err.themePatch.theme_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.themePatch.internal")
		return
	}
	s.writeJSON(w, http.StatusOK, updated)
}

// DELETE /api/themes/{id} — delete; a session currently using the theme is
// reset to the default builtin so the UI never points at a dangling id.
func (s *Server) handleThemeDelete(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()
	err := s.store.Themes().Delete(ctx, sid, id)
	if errors.Is(err, domain.ErrNotFound) {
		s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "theme_not_found", "err.themeDelete.theme_not_found")
		return
	}
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.themeDelete.internal")
		return
	}
	reset := false
	if sess, err := s.store.Sessions().Get(ctx, sid); err == nil && sess.CurrentTheme == id {
		def := domain.BuiltinThemes[0]
		if _, err := s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{CurrentTheme: &def}); err == nil {
			reset = true
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "currentThemeReset": reset})
}
