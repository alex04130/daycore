package server

import (
	"net/http"

	"daycore/internal/i18n"
)

func init() {
	i18n.Register("err.ai.not_configured", i18n.Text{
		"zh-CN": "还没配置 AI 模型 —— 请设置默认模型的 API key 后重试",
		"en-US": "No AI model is configured — set the default model's API key and retry",
	})
}

// requireAI writes 503 ai_not_configured and returns false when the default
// chat model has no key. It is the difference the frontend renders as "还没配
// AI" rather than a generic failure: a call without a key can only fail with an
// auth error, and that is not a reason to retry or to show "something went
// wrong". Every endpoint that talks to DefaultChat calls it right after
// requireSession.
func (s *Server) requireAI(w http.ResponseWriter, r *http.Request) bool {
	if s.catalog.DefaultChatConfigured() {
		return true
	}
	s.writeErrL(w, s.requestLocale(r), http.StatusServiceUnavailable, "ai_not_configured", "err.ai.not_configured")
	return false
}
