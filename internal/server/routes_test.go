package server

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The whole HTTP surface, readable in one place again after the registrations
// were scattered into the files that own their handlers.
func TestRouteTableIsComplete(t *testing.T) {
	routes := RouteTable(&Server{})
	if len(routes) < 90 {
		t.Fatalf("only %d routes registered — an init() is missing, or a file stopped being linked in", len(routes))
	}
	seen := map[string]string{}
	for _, r := range routes {
		if prev, dup := seen[r.Pattern]; dup {
			// http.ServeMux panics on a duplicate pattern, but at Handler() time
			// — which is a server that will not start, discovered late. Catch it
			// here, where the message can say which two groups collided.
			t.Errorf("%q registered twice: %q and %q", r.Pattern, prev, r.Group)
		}
		seen[r.Pattern] = r.Group
		if !strings.HasPrefix(r.Pattern, "GET ") && !strings.HasPrefix(r.Pattern, "POST ") &&
			!strings.HasPrefix(r.Pattern, "PATCH ") && !strings.HasPrefix(r.Pattern, "PUT ") &&
			!strings.HasPrefix(r.Pattern, "DELETE ") {
			t.Errorf("%q has no method — a pattern without one answers every verb, including the ones the handler does not check for", r.Pattern)
		}
	}
}

// The living-docs rule says a new route lands with its contract in the same
// change. This is that rule, enforced: every registered path must exist in
// api/openapi.yaml.
//
// It catches the failure that is otherwise invisible — a route that works, that
// no generated client knows about, and that nobody notices is undocumented until
// a frontend needs it.
func TestEveryRouteIsInTheOpenAPISpec(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse openapi: %v", err)
	}

	for _, r := range RouteTable(&Server{}) {
		method, path, ok := strings.Cut(r.Pattern, " ")
		if !ok {
			continue // already reported by the table test
		}
		ops, found := spec.Paths[path]
		if !found {
			t.Errorf("%s is served (group %q) but has no entry in api/openapi.yaml", r.Pattern, r.Group)
			continue
		}
		if _, found := ops[strings.ToLower(method)]; !found {
			var have []string
			for verb := range ops {
				have = append(have, strings.ToUpper(verb))
			}
			t.Errorf("%s is served but openapi.yaml only documents %v for %s", r.Pattern, have, path)
		}
	}
}

// The other direction: a documented path that nothing serves is a promise the
// server does not keep, and a generated client will call it and get the SPA
// fallback instead of an error.
func TestEveryOpenAPIPathIsServed(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse openapi: %v", err)
	}

	served := map[string]bool{}
	for _, r := range RouteTable(&Server{}) {
		if method, path, ok := strings.Cut(r.Pattern, " "); ok {
			served[strings.ToLower(method)+" "+path] = true
		}
	}
	for path, ops := range spec.Paths {
		for verb := range ops {
			switch verb {
			case "get", "post", "patch", "put", "delete":
			default:
				continue // parameters, summary, description, …
			}
			if !served[verb+" "+path] {
				t.Errorf("openapi.yaml documents %s %s but nothing serves it", strings.ToUpper(verb), path)
			}
		}
	}
}
