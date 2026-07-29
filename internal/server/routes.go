package server

import (
	"net/http"
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
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

type routeGroup struct {
	// name groups related routes the way the comment headers in Handler() used
	// to. It shows up in the route table and in duplicate-registration errors.
	name     string
	register func(*Server, Mux)
}

var routeGroups []routeGroup

// registerRoutes declares one group of routes. Call it from init() in the file
// that defines the handlers.
func registerRoutes(name string, f func(*Server, Mux)) {
	if name == "" || f == nil {
		panic("server: registerRoutes needs a name and a function")
	}
	routeGroups = append(routeGroups, routeGroup{name: name, register: f})
}

// Route is one registered pattern and the group that owns it.
type Route struct {
	Group   string
	Pattern string
}

// recorder collects patterns instead of dispatching them.
type recorder struct {
	group string
	out   *[]Route
}

func (r recorder) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	*r.out = append(*r.out, Route{Group: r.group, Pattern: pattern})
}

// RouteTable replays every registration and returns the patterns, sorted.
//
// It takes a *Server only because the closures name s.handleX; a method value
// needs a receiver, but nothing is ever called, so a zero Server is fine — and
// the route-table test relies on that (it must not need a database).
func RouteTable(s *Server) []Route {
	var out []Route
	for _, g := range routeGroups {
		g.register(s, recorder{group: g.name, out: &out})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pattern != out[j].Pattern {
			return out[i].Pattern < out[j].Pattern
		}
		return out[i].Group < out[j].Group
	})
	return out
}
