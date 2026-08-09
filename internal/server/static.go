package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// staticHandler serves the built frontend (cfg.StaticDir) with an SPA
// fallback: unknown non-file paths get index.html so client-side routing
// works. Returns nil when no build is present — the server then runs API-only
// (e.g. during frontend development behind the Vite dev server).
// StaticRoot is the frontend directory this process is serving, or "" when it
// is running API-only. Reported at startup instead of the configured path so
// the line says what is true — see the staticRoot field in server.go.
func (s *Server) StaticRoot() string { return s.staticRoot }

func (s *Server) staticHandler() http.Handler {
	dir := s.cfg.StaticDir
	if dir == "" {
		return nil
	}
	index := filepath.Join(dir, "index.html")
	if _, err := os.Stat(index); err != nil {
		return nil
	}
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Never shadow the API namespace, even for typos — those must 404 as API.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		clean := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(clean); err == nil && !st.IsDir() {
			// Hashed build assets are immutable; index.html must revalidate.
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fs.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}
