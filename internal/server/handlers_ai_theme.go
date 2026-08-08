package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"daycore/internal/ai"
	"daycore/internal/domain"
)

func init() {
	registerRoutes("ai", func(s *Server, mux Mux) {
		mux.HandleFunc("POST /api/ai/theme", s.handleAITheme)
	})
}

// POST /api/ai/theme — AI-assisted theme generation/editing. Returns a theme
// CANDIDATE {name, dark, variables, warnings} for the frontend to live-preview;
// nothing is saved here — the user confirms via POST /api/themes (create) or
// PATCH /api/themes/{id} (when editing an existing one).
func (s *Server) handleAITheme(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !s.rateLimit(w, r) {
		return
	}
	var body struct {
		Description string `json:"description"`
		Base        string `json:"base"`    // builtin id to start from
		ThemeID     string `json:"themeId"` // existing custom theme to edit
	}
	if err := s.readJSON(r, &body); err != nil || strings.TrimSpace(body.Description) == "" {
		s.writeErrL(w, s.requestLocale(r), http.StatusBadRequest, "bad_request", "err.aITheme.bad_request")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.AIRequestTimeout)
	defer cancel()

	data := ai.ThemeGenData{
		Description: strings.TrimSpace(body.Description),
		AllowedVars: allowedVarsMarkdown(),
	}
	if body.ThemeID != "" {
		cur, err := s.store.Themes().Get(ctx, sid, body.ThemeID)
		if errors.Is(err, domain.ErrNotFound) {
			s.writeErrL(w, s.requestLocale(r), http.StatusNotFound, "theme_not_found", "err.aITheme.theme_not_found")
			return
		}
		if err != nil {
			s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.aITheme.internal")
			return
		}
		data.CurrentName = cur.Name
		data.CurrentVariables = marshalCompact(cur.Variables)
		if body.Base == "" {
			body.Base = cur.Base
		}
	}
	if body.Base != "" {
		preset, ok := builtinThemePresets[body.Base]
		if !ok {
			s.writeErr(w, http.StatusBadRequest, "invalid_theme", "base must be one of the builtin theme ids")
			return
		}
		data.BaseName = preset.Name
		data.BaseVariables = marshalCompact(preset.Variables)
	}

	sys, err := s.prompts.Render(ctx, ai.PromptThemeGen, s.requestLocale(r), data)
	if err != nil {
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "internal", "err.aITheme.internal2")
		return
	}
	provider := s.catalog.DefaultChat()
	start := time.Now()
	resp, err := provider.Chat(ctx, ai.ChatRequest{
		Messages:    []ai.Message{{Role: ai.RoleSystem, Content: sys}, {Role: ai.RoleUser, Content: data.Description}},
		Temperature: 0.6, MaxTokens: 2000, JSONMode: true,
	})
	s.logAICall(ctx, sid, epThemeGen, provider.Model(), start, usageOf(resp), err)
	if err != nil {
		s.log.Error("ai theme", "err", err)
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server_error", "message": "主题生成出了点问题，请稍后再试"})
		return
	}
	result, ok := extractJSONObject(resp.Content)
	if !ok {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "parse_error", "message": "主题解析出了点问题，请重试"})
		return
	}
	if result["error"] != nil {
		s.writeJSON(w, http.StatusOK, result)
		return
	}

	rawVars, _ := result["variables"].(map[string]any)
	vars := map[string]string{}
	for k, v := range rawVars {
		if sv, ok := v.(string); ok {
			vars[k] = sv
		}
	}
	clean, dropped := sanitizeThemeVariables(vars)
	if len(clean) == 0 {
		s.writeJSON(w, http.StatusOK, map[string]any{"error": "parse_error", "message": "主题解析出了点问题，请重试"})
		return
	}
	warnings := []string{}
	if len(dropped) > 0 {
		sort.Strings(dropped)
		warnings = append(warnings, fmt.Sprintf("已丢弃无效变量：%s", strings.Join(dropped, ", ")))
	}
	name, _ := result["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = "自定义主题"
	}
	dark, _ := result["dark"].(bool)

	s.writeJSON(w, http.StatusOK, map[string]any{
		"name": name, "dark": dark, "base": body.Base, "variables": clean, "warnings": warnings,
	})
}

// allowedVarsMarkdown renders the whitelist for the prompt, in stable order.
func allowedVarsMarkdown() string {
	keys := make([]string, 0, len(themeVarWhitelist))
	for k := range themeVarWhitelist {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "- `%s` — %s\n", k, themeVarWhitelist[k])
	}
	return b.String()
}
