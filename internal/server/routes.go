package server

import (
	"net/http"
	"path/filepath"
	"runtime"
	"sort"
)

// Routes are registered by the file that owns their handlers, from init(), and
// collected here.
//
// This was one hundred mux.HandleFunc calls inside Handler(), which made
// server.go the single most contended file in the repo — twelve parallel work
// items all had to edit the same list, and every one of them merged through it.
// A registry puts a route next to the handler it dispatches to, which is also
// where someone looking for it will go.
//
// Order does not matter. Go 1.22's ServeMux resolves patterns by specificity
// rather than by registration order, so scattering the registrations cannot
// change which handler wins — and the "/" static fallback loses to every real
// route no matter when it is added.

// Mux is the sliver of *http.ServeMux the registrations use.
//
// An interface rather than the concrete type so the route table can be read
// without standing up a server: *http.ServeMux is a struct with no way to
// enumerate what was registered into it, so the only way to know the HTTP
// surface is to replay the registrations against something that records. Losing
// the single list in Handler() also lost the one place you could read the whole
// surface; this puts it back.
type Mux interface {
	HandleFunc(pattern string, handler handlerFunc)
}

type handlerFunc = func(http.ResponseWriter, *http.Request)

type routeGroup struct {
	// name groups related routes the way the comment headers in Handler() used
	// to. It shows up in the route table and in duplicate-registration errors.
	name string
	// file is the source file that called registerRoutes, captured rather than
	// declared: docs/API_SURFACE.md answers "which file owns this route", and a
	// hand-written or regex-scraped answer to that question is exactly what went
	// stale before. runtime.Caller cannot disagree with the compiler.
	file     string
	register func(*Server, Mux)
}

var routeGroups []routeGroup

// registerRoutes declares one group of routes. Call it from init() in the file
// that defines the handlers.
func registerRoutes(name string, f func(*Server, Mux)) {
	if name == "" || f == nil {
		panic("server: registerRoutes needs a name and a function")
	}
	file := "?"
	if _, path, _, ok := runtime.Caller(1); ok {
		file = filepath.Base(path)
	}
	routeGroups = append(routeGroups, routeGroup{name: name, file: file, register: f})
}

// Route is one registered pattern, the group that owns it, and the file it was
// registered from.
// Route is one registration, seen two ways.
//
// ⚠️ Both, and named apart on purpose. They answer different questions and the
// wrong one is silently wrong:
//
//	Pattern   what the server SERVES — versioned. "GET /api/v2/plan". This is
//	          the wire, so it is what docs/API_SURFACE.md and the openapi
//	          cross-check are built from.
//	Logical   what the code REGISTERED — version-free. "GET /api/plan". This is
//	          what the permission table and the admin gate key on, because a
//	          permission is about a resource and not about which major serves it.
//
// One field would have forced every caller to strip or add the prefix at the
// point of use, and the ones that forgot would have failed open: an admin gate
// that does not recognise "/api/v2/admin/…" as an admin route waves it through.
type Route struct {
	Group   string
	Pattern string
	Logical string
	File    string
}

// recorder collects patterns instead of dispatching them.
type recorder struct {
	group string
	file  string
	out   *[]Route
}

func (r recorder) HandleFunc(pattern string, _ handlerFunc) {
	*r.out = append(*r.out, Route{
		Group: r.group, Pattern: versionPattern(pattern), Logical: pattern, File: r.file,
	})
}

// RouteTable replays every registration and returns the patterns, sorted.
//
// It takes a *Server only because the closures name s.handleX; a method value
// needs a receiver, but nothing is ever called, so a zero Server is fine — and
// the route-table test relies on that (it must not need a database).
func RouteTable(s *Server) []Route {
	var out []Route
	for _, g := range routeGroups {
		// The recorder versions the pattern itself and keeps the logical one
		// beside it — see Route. Wrapping it in versionedMux instead would have
		// produced a table with only the wire view, and the permission gates
		// read the other one.
		g.register(s, recorder{group: g.name, file: g.file, out: &out})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pattern != out[j].Pattern {
			return out[i].Pattern < out[j].Pattern
		}
		return out[i].Group < out[j].Group
	})
	return out
}
