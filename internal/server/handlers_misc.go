package server

import (
	"net/http"

	"daycore/internal/ai"
	"daycore/internal/i18n"
	"daycore/internal/version"
)

func init() {
	registerRoutes("health & diagnostics", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/healthz", s.handleHealth)
		mux.HandleFunc("GET /api/version", s.handleAPIVersion)
		mux.HandleFunc("GET /api/models", s.handleModels)
	})
}

// GET /api/healthz — liveness + DB connectivity + build version.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		// Don't leak DSN/host fragments from the driver error to unauthenticated
		// callers; log the detail server-side instead.
		s.log.Error("healthz db ping failed", "err", err)
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "db": s.cfg.DBType, "error": "database unavailable"})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "db": s.cfg.DBType,
		"version": version.Full(), "channel": version.Channel, "env": s.cfg.Env,
	})
}

// GET /api/version — the API contract version, independent of the build
// version. Public: separated frontends and native clients call this first to
// decide compatibility (matching rules in api/FRONTEND_HANDOFF.md).
func (s *Server) handleAPIVersion(w http.ResponseWriter, r *http.Request) {
	// locales is what a client needs before it has a session: every language
	// this installation can render (available), and the pair a brand-new user
	// starts with (defaultPrimary / defaultSecondary). A signed-in user's own
	// pair comes from GET /api/session — this is only the starting point.
	//
	// available is not a compile-time list. Dropping a <locale>.json into the
	// locales directory or adding one from the console extends it, so clients
	// must read it rather than hardcoding what they think exists.
	s.writeJSON(w, http.StatusOK, map[string]any{
		"apiVersion": version.APIVersion,
		"apiMinor":   version.APIMinor,
		"build":      version.Full(),
		"channel":    version.Channel,
		"minClient":  version.MinClient,
		// features is the capability discovery layer: a client (web, app,
		// edge, channel) asks what this deployment can do instead of
		// hardcoding assumptions. Derived live from the model catalog, so a
		// deployment that adds a transcribing model starts advertising
		// transcribe with no code change and no restart of the answer.
		"features": s.capabilityFeatures(),
		"locales": map[string]any{
			"available":        i18n.Available(),
			"defaultPrimary":   s.defaultLocales.Primary,
			"defaultSecondary": s.defaultLocales.Secondary,
		},
	})
}

// capabilityFeatures derives the /api/version features block from the model
// catalog. All four are false when no catalog is wired (tests, pure-API
// deployments) — clients must treat false as "not here", not "broken".
func (s *Server) capabilityFeatures() map[string]bool {
	f := map[string]bool{"vision": false, "transcribe": false, "tts": false, "imagegen": false}
	if s.catalog == nil {
		return f
	}
	f["vision"] = s.catalog.HasVision()
	_, f["transcribe"] = s.catalog.Transcriber()
	_, f["tts"] = s.catalog.SpeechSynthesizer()
	_, f["imagegen"] = s.catalog.ImageGenerator()
	return f
}

// GET /api/models — diagnostics: the configured model catalog + registered formats.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"models":    s.catalog.List(),
		"formats":   ai.Formats(),
		"hasVision": s.catalog.HasVision(),
	})
}
