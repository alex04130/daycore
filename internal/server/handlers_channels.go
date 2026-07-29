package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"daycore/internal/domain"
)

func init() {
	// Its own group: these are user-facing (bind your own QQ/OneBot account), not
	// operator endpoints. They rode along under "admin (prompts)" when the routes
	// were scattered out of server.go, which put four user routes under an admin
	// heading in the route table.
	registerRoutes("channels (user binds an account)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/channels", s.handleChannelList)
		mux.HandleFunc("POST /api/channels/{channel}/bind", s.handleChannelBind)
		mux.HandleFunc("POST /api/channels/{channel}/verify", s.handleChannelVerify)
		mux.HandleFunc("DELETE /api/channels/{channel}/unbind", s.handleChannelUnbind)
	})
}

// generateBindingToken creates a 12-char random token valid for 10 minutes.
func generateBindingToken() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err // never emit an all-zero token on a rand failure
	}
	return hex.EncodeToString(b), nil
}

// GET /api/channels — list available channels and binding status.
func (s *Server) handleChannelList(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	bindings, _ := s.store.ChannelBindings().ListBySession(r.Context(), sid)
	if bindings == nil {
		bindings = []domain.ChannelBinding{}
	}
	// For now, only onebot is available.
	channels := []map[string]any{
		{"name": "onebot", "label": "QQ (OneBot/NapCat)", "available": true},
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"channels": channels,
		"bindings": bindings,
	})
}

// POST /api/channels/{channel}/bind — generate a binding token.
func (s *Server) handleChannelBind(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	channel := r.PathValue("channel")
	token, err := generateBindingToken()
	if err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "生成绑定令牌失败")
		return
	}
	// Store an unverified binding with the token as external_id.
	b := &domain.ChannelBinding{
		SessionID:  sid,
		Channel:    channel,
		ExternalID: token,
		Metadata:   `{"expires_at":"` + time.Now().Add(10*time.Minute).UTC().Format(time.RFC3339) + `"}`,
	}
	if _, err := s.store.ChannelBindings().Create(r.Context(), b); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "生成绑定令牌失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"token":   token,
		"channel": channel,
		"note":    "请在对应平台把这段 token 发给机器人以完成绑定，有效 10 分钟",
	})
}

// POST /api/channels/{channel}/verify — verify a binding token. Unauthenticated
// (the platform user calls it), so it's rate-limited and TTL-enforced.
func (s *Server) handleChannelVerify(w http.ResponseWriter, r *http.Request) {
	if !s.authRateLimit(w, r) {
		return
	}
	channel := r.PathValue("channel")
	var body struct {
		Token      string `json:"token"`
		ExternalID string `json:"externalId"`
	}
	if err := s.readJSON(r, &body); err != nil || body.Token == "" || body.ExternalID == "" {
		s.writeErr(w, http.StatusBadRequest, "bad_request", "缺少 token 或 externalId")
		return
	}
	// Find the still-unverified binding by channel + token.
	pending, err := s.store.ChannelBindings().GetPendingByToken(r.Context(), channel, body.Token)
	if err != nil {
		s.writeErr(w, http.StatusNotFound, "invalid_token", "绑定令牌已过期或不存在")
		return
	}
	// Enforce the 10-minute TTL (belt-and-suspenders with the background sweep).
	if time.Since(pending.CreatedAt) > 10*time.Minute {
		s.writeErr(w, http.StatusGone, "token_expired", "绑定令牌已过期，请重新获取")
		return
	}
	// Promote the token row into the real, verified binding.
	if err := s.store.ChannelBindings().Promote(r.Context(), pending.ID, body.ExternalID, ""); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "绑定验证失败")
		return
	}
	// Now that the session has a channel, schedule its proactive jobs.
	if s.worker != nil {
		s.worker.ScheduleUser(pending.SessionID, s.cfg.WorkerDefaultTZ)
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "channel": channel, "externalId": body.ExternalID})
}

// DELETE /api/channels/{channel}/unbind — unbind a channel.
func (s *Server) handleChannelUnbind(w http.ResponseWriter, r *http.Request) {
	sid, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	channel := r.PathValue("channel")
	if err := s.store.ChannelBindings().Delete(r.Context(), sid, channel); err != nil {
		s.writeErr(w, http.StatusInternalServerError, "internal", "解绑失败")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
