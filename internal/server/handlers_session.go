package server

import (
	"net/http"

	"daycore/internal/auth"
	"daycore/internal/domain"
	"daycore/internal/i18n"
)

// POST /api/session/init — get-or-create the anonymous session. The server owns
// the session id: if there is no valid signed cookie, a fresh random id is
// generated and set as an httpOnly cookie (fixes v1's guessable query-param id).
func (s *Server) handleSessionInit(w http.ResponseWriter, r *http.Request) {
	// Optional body: {"tokenInBody":true} asks for the signed session token in
	// the response for clients without a cookie jar (native apps). Best-effort
	// parse — the endpoint has always accepted an empty body.
	var body struct {
		TokenInBody bool `json:"tokenInBody"`
	}
	_ = s.readJSON(r, &body)
	sid := sessionIDFrom(r.Context())
	if sid == "" {
		newID, err := auth.NewSessionID()
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, "internal", "无法创建会话")
			return
		}
		sid = newID
	}
	sess, err := s.store.Sessions().GetOrCreate(r.Context(), sid)
	if err != nil {
		s.log.Error("session init", "err", err)
		s.writeErr(w, http.StatusInternalServerError, "internal", "会话初始化失败")
		return
	}
	// First contact: adopt the browser's language so prompts and AI replies
	// match the user before they ever open settings.
	if sess.Language == "" {
		lang := i18n.Resolve("", r.Header.Get("Accept-Language"))
		if updated, err := s.store.Sessions().Update(r.Context(), sid, domain.SessionUpdate{Language: &lang}); err == nil {
			sess = updated
		}
	}
	s.setSessionCookie(w, sid)
	if body.TokenInBody {
		// Embed so the session's top-level JSON shape stays unchanged for
		// existing clients that Object.assign the response.
		s.writeJSON(w, http.StatusOK, struct {
			*domain.Session
			SessionToken string `json:"sessionToken"`
		}{sess, s.cookies.Sign(sid)})
		return
	}
	s.writeJSON(w, http.StatusOK, sess)
}

// POST /api/session/theme — record a theme switch and update current theme.
func (s *Server) handleSessionTheme(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		Theme string `json:"theme"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Theme == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 theme")
		return
	}
	ctx := r.Context()
	_ = s.store.ThemeLog().Add(ctx, sid, body.Theme) // audit log is best-effort
	if _, err := s.store.Sessions().Update(ctx, sid, domain.SessionUpdate{CurrentTheme: &body.Theme}); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "主题保存失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// PATCH /api/session/settings — update assistant name, theme, language, and/or persona.
func (s *Server) handleSessionSettings(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var body struct {
		AssistantName *string `json:"assistantName"`
		CurrentTheme  *string `json:"currentTheme"`
		Language      *string `json:"language"`
		PersonaPrompt *string `json:"personaPrompt"`
	}
	if err := s.readJSON(r, &body); err != nil {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if body.Language != nil {
		lang := i18n.Normalize(*body.Language)
		if lang == "" {
			s.writeErr(w, http.StatusBadRequest, "unsupported_locale", "不支持的语言")
			return
		}
		body.Language = &lang
	}
	if body.PersonaPrompt != nil && len([]rune(*body.PersonaPrompt)) > 2000 {
		s.writeErr(w, http.StatusBadRequest, "too_long", "个性化提示词不能超过 2000 字")
		return
	}
	sess, err := s.store.Sessions().Update(r.Context(), sid, domain.SessionUpdate{
		AssistantName: body.AssistantName,
		CurrentTheme:  body.CurrentTheme,
		Language:      body.Language,
		PersonaPrompt: body.PersonaPrompt,
	})
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "设置保存失败")
		return
	}
	s.writeJSON(w, http.StatusOK, sess)
}
