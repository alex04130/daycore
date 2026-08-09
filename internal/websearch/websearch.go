// Package websearch resolves the configured web search sources and answers a
// query against a named one.
//
// # Why this is a new package and not the old internal/search
//
// internal/search held two unrelated things: a web search client, and
// MaterialSearcher — the站内 full-text search over the user's own imported
// material, which implements domain.Searcher and has nothing to do with the
// web. The package comment described only the first. Splitting them was
// overdue, and this batch forced it: web search is what gains sources, a
// registry and health, while material search is a repository query.
//
// # The shape, copied from internal/weather on purpose
//
// A registry at package level, implementations in subpackages that self-register
// from init(), and selection through configuration. Web search had none of the
// three: it was a struct with four fields and a hardcoded
// `if TavilyKey != "" { tavily } else { duckduckgo }`, and New() read
// TAVILY_API_KEY straight out of the environment — a credential the config
// layering could not see, because it was never a Config field and the
// classification gate walks Config's fields.
//
// ⚠️ That last point is worth keeping: the gate in internal/config is
// structurally blind to credentials named by a data file rather than by a
// struct field. TAVILY_API_KEY, DEEPSEEK_API_KEY and the rest were never
// covered by it. docs/CONFIG.md now says so.
package websearch

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"daycore/internal/adapters"
)

// Result is one search hit.
type Result struct {
	Title   string
	URL     string
	Snippet string
}

// Engine performs one search against one source.
//
// Deliberately not domain.Searcher: that interface is the material index
// (`{ID, Title, Snippet, Score}` — no URL, because a stored note has no URL).
// Merging them would mean adding a URL nothing sets and a Score nothing means.
type Engine interface {
	Name() string
	Search(ctx context.Context, query string, maxResults int) ([]Result, error)
}

// Config is passed to an engine factory.
type Config struct {
	APIKey string
	HTTP   *http.Client
	// BaseURL lets a deployment point an engine at a compatible mirror. Empty
	// means the public endpoint.
	BaseURL string
}

// Factory builds an Engine.
type Factory func(Config) Engine

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register adds an engine factory under a name, from a subpackage's init().
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	factories[name] = f
}

// Engines lists the registered implementation names, sorted.
func Engines() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func build(name string, cfg Config) Engine {
	mu.RLock()
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		return nil
	}
	return f(cfg)
}

// Options carries what a builtin engine needs beyond its entry.
type Options struct {
	TavilyKey string
	Timeout   time.Duration
}

// MaxResults is the ceiling on one query.
//
// Five, unchanged: every hit becomes tool output that lands in the model's
// context and is paid for on every subsequent turn of that conversation. This
// is a budget, not a limitation of any engine.
const MaxResults = 5

// Sources is every web search source this process knows about.
//
// Same shape as weather.Sources and for the same reasons — one capability, one
// tool, the source as a parameter, and a failure reported rather than silently
// retried somewhere else. There is no cache: a search is a question about right
// now, and a cached answer to "what happened today" is worse than a slow one.
type Sources struct {
	mu    sync.RWMutex
	order []string
	byID  map[string]*entry
}

type entry struct {
	src    *adapters.Source
	engine Engine
}

// NewSources builds the set. Sources whose engine cannot be constructed are
// dropped with a reason rather than kept as entries that fail every call.
func NewSources(resolved []*adapters.Source, o Options) (*Sources, []error) {
	s := &Sources{byID: map[string]*entry{}}
	var problems []error
	for _, src := range resolved {
		e, err := buildEngine(src, o)
		if err != nil {
			problems = append(problems, fmt.Errorf("search/%s: %w", src.Entry.ID, err))
			continue
		}
		s.byID[src.Entry.ID] = &entry{src: src, engine: e}
		s.order = append(s.order, src.Entry.ID)
	}
	sort.Strings(s.order)
	return s, problems
}

func buildEngine(src *adapters.Source, o Options) (Engine, error) {
	timeout := o.Timeout
	if timeout == 0 {
		timeout = 8 * time.Second
	}
	switch src.Entry.Format {
	case adapters.FormatBuiltin:
		e := build(src.Entry.Impl, Config{
			APIKey: keyFor(src.Entry.Impl, o),
			HTTP:   &http.Client{Timeout: timeout},
		})
		if e == nil {
			return nil, fmt.Errorf("no built-in engine %q (registered: %v)", src.Entry.Impl, Engines())
		}
		if src.Entry.Impl == "tavily" && o.TavilyKey == "" {
			return nil, fmt.Errorf("tavily needs TAVILY_API_KEY and none is set")
		}
		return e, nil
	case adapters.FormatHTTP:
		return &httpEngine{
			client: adapters.NewClient(src.Entry.ID, src.Entry.BaseURL, src.Entry.Token(), timeout),
			id:     src.Entry.ID,
		}, nil
	}
	return nil, fmt.Errorf("unsupported format %q", src.Entry.Format)
}

func keyFor(impl string, o Options) string {
	if impl == "tavily" {
		return o.TavilyKey
	}
	return ""
}

// IDs is the usable sources, sorted — the enum of the tool's `source`
// parameter, and a pure function of configuration and health for the reason
// weather.Sources.IDs gives.
func (s *Sources) IDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.order))
	for _, id := range s.order {
		if s.byID[id].src.Usable() {
			out = append(out, id)
		}
	}
	return out
}

// Describe returns each usable source's id and its prompt-safe text.
func (s *Sources) Describe(locale string) [][2]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := [][2]string{}
	for _, id := range s.order {
		if e := s.byID[id]; e.src.Usable() {
			out = append(out, [2]string{id, e.src.PromptDescription(locale)})
		}
	}
	return out
}

// Available reports whether any source can be used.
func (s *Sources) Available() bool { return len(s.IDs()) > 0 }

// Search queries one source. An empty id means "pick for me".
func (s *Sources) Search(ctx context.Context, id, query string, maxResults int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search: empty query")
	}
	switch {
	case maxResults < 1:
		maxResults = 1
	case maxResults > MaxResults:
		maxResults = MaxResults
	}
	e, err := s.pick(id)
	if err != nil {
		return nil, err
	}
	rs, err := e.engine.Search(ctx, query, maxResults)
	e.src.Health.Observe(err, time.Now())
	return rs, err
}

// pick resolves an id, or chooses one. A named source that is unusable is an
// error rather than a redirect: answering a question about one source with
// another's data is the cross-source chain under a new name, and the model
// cannot tell.
func (s *Sources) pick(id string) (*entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id != "" {
		e, ok := s.byID[id]
		if !ok {
			return nil, fmt.Errorf("no search source %q", id)
		}
		if !e.src.Usable() {
			return nil, fmt.Errorf("search source %q is unavailable", id)
		}
		return e, nil
	}
	for _, oid := range s.order {
		if e := s.byID[oid]; e.src.Usable() {
			return e, nil
		}
	}
	return nil, fmt.Errorf("no search source is available")
}

// AllIDs is every configured id, including disabled and unhealthy ones —
// unlike IDs, which is the tool enum. A reload has to reach the disabled ones
// too: they are exactly the ones an operator is about to turn back on.
func (s *Sources) AllIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.order...)
}

// Views renders every source for the console, disabled and unhealthy included.
func (s *Sources) Views(instance string) []adapters.AdminView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]adapters.AdminView, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.byID[id].src.View(instance))
	}
	return out
}

// Source returns the resolved source behind an id, for the admin write path.
func (s *Sources) Source(id string) *adapters.Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.byID[id]; ok {
		return e.src
	}
	return nil
}

// DefaultEntries is what a deployment with no providers.yaml gets.
//
// DuckDuckGo always, Tavily when a key is set — the same two the old hardcoded
// branch could reach, so no deployment's behaviour changes on upgrade. What
// changes is that they are now two sources rather than a preference and a
// fallback: the model picks, and a Tavily failure is reported instead of
// silently becoming a DuckDuckGo answer.
func DefaultEntries(o Options) []adapters.Entry {
	out := []adapters.Entry{{ID: "duckduckgo", Format: adapters.FormatBuiltin, Impl: "duckduckgo"}}
	if o.TavilyKey != "" {
		out = append(out, adapters.Entry{ID: "tavily", Format: adapters.FormatBuiltin, Impl: "tavily"})
	}
	return out
}
