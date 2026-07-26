package server

import (
	"net/http"

	"daycore/internal/ai"
	"daycore/internal/version"
)

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
	s.writeJSON(w, http.StatusOK, map[string]any{
		"apiVersion": version.APIVersion,
		"apiMinor":   version.APIMinor,
		"build":      version.Full(),
		"channel":    version.Channel,
		"minClient":  version.MinClient,
	})
}

// GET /api/models — diagnostics: the configured model catalog + registered formats.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"models":    s.catalog.List(),
		"formats":   ai.Formats(),
		"hasVision": s.catalog.HasVision(),
	})
}
