package server

import (
	"net/http"
)

func init() {
	registerRoutes("admin (stats, users, DB)", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /api/admin/db/tables", s.handleAdminDBTables)
		mux.HandleFunc("GET /api/admin/db/table/{name}", s.handleAdminDBTableBrowse)
		mux.HandleFunc("DELETE /api/admin/db/table/{name}/{id}", s.handleAdminDBTableDelete)
		mux.HandleFunc("GET /api/admin/db/export", s.handleAdminDBExport)
		mux.HandleFunc("POST /api/admin/db/import", s.handleAdminDBImport)
		mux.HandleFunc("GET /api/admin/db/backup", s.handleAdminDBBackup)
	})
}

// GET /api/admin/db/tables
func (s *Server) handleAdminDBTables(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminDBTables.unauthorized")
		return
	}
	// TODO: reflect store schema metadata.
	s.writeJSON(w, http.StatusOK, map[string]any{
		"tables": []string{"sessions", "day_plans", "mood_checkins", "companion_memory",
			"theme_switch_log", "operation_logs", "users", "credentials", "oauth_identities",
			"prompts", "prompt_overrides", "schedule_rules", "courses", "assignments",
			"custom_themes", "memory_facts", "import_history", "chat_threads", "chat_messages", "ai_call_logs"},
	})
}

// GET /api/admin/db/table/{name}
func (s *Server) handleAdminDBTableBrowse(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminDBTableBrowse.unauthorized")
		return
	}
	_ = r.PathValue("name")
	s.writeJSON(w, http.StatusOK, map[string]any{
		"rows":    []any{},
		"columns": []string{},
		"note":    "table browser coming soon — depends on per-engine introspection",
	})
}

// DELETE /api/admin/db/table/{name}/{id}
func (s *Server) handleAdminDBTableDelete(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminDBTableDelete.unauthorized")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "per-table delete coming soon"})
}

// GET /api/admin/db/export
func (s *Server) handleAdminDBExport(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminDBExport.unauthorized")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"export": map[string]any{}, "note": "full export coming soon"})
}

// POST /api/admin/db/import
func (s *Server) handleAdminDBImport(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminDBImport.unauthorized")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "full import coming soon"})
}

// GET /api/admin/db/backup
func (s *Server) handleAdminDBBackup(w http.ResponseWriter, r *http.Request) {
	if !s.adminAuthorized(r) {
		s.writeErrL(w, s.requestLocale(r), http.StatusUnauthorized, "unauthorized", "err.adminDBBackup.unauthorized")
		return
	}
	// SQLite backup — write the db file directly.
	s.writeErrL(w, s.requestLocale(r), http.StatusNotImplemented, "not_implemented", "err.adminDBBackup.not_implemented")
}
