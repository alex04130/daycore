package server

import (
	"io/fs"
	"net/http"
	"strings"

	"daycore/internal/resources"
)

func init() {
	registerRoutes("console", func(s *Server, mux Mux) {
		mux.HandleFunc("GET /admin", s.handleConsole)
		mux.HandleFunc("GET /admin/", s.handleConsole)
	})
}

// The operations console, served by the binary itself at /admin.
//
// # Why it is embedded rather than deployed like the app frontend
//
// The four user-facing frontends deploy separately, on their own schedules, in
// their own repositories. The console does the opposite and for a reason that
// is not symmetry:
//
//   - **It is version-locked to this backend.** It renders this build's config
//     classification, this build's provider fields, this build's prompt keys.
//     A console one release behind shows an operator a screen that quietly
//     disagrees with the process it is pointed at.
//   - **It is needed exactly when things are broken.** Degraded boot exists so
//     that a process with no database still serves the admin surface — and a
//     console that had to be deployed separately would be the one thing missing
//     from the deployment you are trying to rescue. "Download one binary, run
//     it, open /admin" has to hold in the case that matters.
//
// # Boundary: this route is NOT authentication
//
// It serves static files, and static files are not secret — every byte here is
// in the public repository. The credential check happens on /api/admin/*, where
// the data is. Gating the HTML too would only produce a login page that cannot
// render its own login form.
//
// ⚠️ Which means: nothing in the console bundle may contain a secret, a
// deployment-specific value, or anything the API would refuse to serve
// unauthenticated. It is a client, and it has to be treated as one.
func (s *Server) handleConsole(w http.ResponseWriter, r *http.Request) {
	sub, err := consoleFS()
	if err != nil {
		// A build that shipped without a console. Say so in words rather than
		// 404: an operator who typed /admin has a question, and "not found" is
		// the one answer that does not help them.
		http.Error(w, "the operations console was not built into this binary; see web/console/README.md", http.StatusNotFound)
		return
	}
	// Everything under /admin is the SPA: an unknown path is a client-side
	// route, not a missing file. Only real files are served as themselves.
	path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/admin"), "/")
	if path != "" {
		if f, err := sub.Open(path); err == nil {
			f.Close()
			// Hashed build assets are immutable; index.html must revalidate, or
			// an operator gets yesterday's console against today's backend —
			// which is the exact failure the version lock above exists to
			// prevent.
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.FileServer(http.FS(sub)).ServeHTTP(w, withPath(r, "/"+path))
			return
		}
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.Error(w, "the operations console was not built into this binary", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

// consoleFS is the built console, or an error when this build has none.
func consoleFS() (fs.FS, error) { return fs.Sub(resources.FS(), "console") }

// ConsoleBuilt reports whether this binary carries a console, for the startup
// line. An operator should not have to guess by visiting a URL.
func ConsoleBuilt() bool {
	sub, err := consoleFS()
	if err != nil {
		return false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return false
	}
	// A placeholder index.html is committed so `go build ./...` works on a fresh
	// clone with no npm run. It is not a console, and reporting it as one at
	// startup would be worse than reporting nothing.
	b, err := fs.ReadFile(sub, "index.html")
	return err == nil && !strings.Contains(string(b), consolePlaceholderMark)
}

// consolePlaceholderMark is in the committed stub and in nothing the build
// produces.
const consolePlaceholderMark = "daycore-console-placeholder"

func withPath(r *http.Request, p string) *http.Request {
	r2 := r.Clone(r.Context())
	r2.URL.Path = p
	return r2
}
