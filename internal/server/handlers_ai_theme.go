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
	if !s.requireAI(w, r) {
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

	ctx, cancel := context.WithTimeout(r.Context(), s.runtime().AIRequestTimeout)
	defer cancel()

	// ⚠️ The family is resolved ONCE and used for both the prompt and the
	// filtering below. Describing one token space and validating against
	// another is the bug this replaces — see themePromptData.
	fam := s.familyFor(r)
	allowed, rules, builtin := s.themePromptData(fam)
	data := ai.ThemeGenData{
		Description: strings.TrimSpace(body.Description),
		AllowedVars: allowed,
		FamilyRules: rules,
		Builtin:     builtin,
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
		// ⚠️ The four builtin presets are the FALLBACK family's. Handing them to
		// another family as a starting point would show the model twelve token
		// names it must not use, right under a list of the ones it must — the
		// most direct way to get an answer that is entirely discarded.
		if builtin {
			data.BaseName = preset.Name
			data.BaseVariables = marshalCompact(preset.Variables)
		}
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
		s.writeErrL(w, s.requestLocale(r), http.StatusInternalServerError, "server_error", "err.aITheme.server_error")
		return
	}
	result, ok := extractJSONObject(resp.Content)
	if !ok {
		s.writeErrL(w, s.requestLocale(r), http.StatusOK, "parse_error", "err.aITheme.parse_error")
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
	clean, dropped := s.sanitizeThemeVariables(fam, vars)
	if len(clean) == 0 {
		// ⚠️ Same key as the parse failure above, and that is a choice rather
		// than a copy-paste. The CAUSE differs — there the JSON never parsed,
		// here every variable it did contain was refused by the token
		// sanitiser — but what the reader can DO is identical, so one message
		// says it once. Split the key when the two want different words, not
		// before: two keys with the same text is a translator being asked to
		// write the same sentence twice and getting no say in whether they
		// diverge.
		s.writeErrL(w, s.requestLocale(r), http.StatusOK, "parse_error", "err.aITheme.parse_error")
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
