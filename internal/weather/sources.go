package weather

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"daycore/internal/adapters"
	"daycore/internal/domain"
)

// Sources is every weather source this process knows about.
//
// # Why this exists, and what it replaced
//
// Weather used to be ONE provider — a configured primary with wttr.in chained
// behind it as a hardcoded fallback, wrapped in a cache. That chain is deleted
// here, and its removal is the point rather than a side effect:
//
//   - It was CROSS-SOURCE. When the primary failed, the answer came back under
//     a different source's name, silently. "Why is the forecast wrong" had no
//     answer anyone could reach, because the primary's error was discarded
//     without a log line — the package did not even have a logger.
//   - The order was not configurable, was exactly two deep, and the second
//     entry was compiled in.
//   - It contradicted the decision already recorded in docs/ARCHITECTURE.md:
//     with several sources available, the CHOICE belongs to the model, which
//     can weigh "which source suits this question", not to a chain that only
//     knows "the last one errored".
//
// So: one capability, one tool, and the source is a parameter. Failure is
// reported to the model as a failure. It does not silently try somebody else —
// a silent retry against a different source is the chain again under a new name.
//
// # The shape, and why it is this shape
//
// Lookup takes an id. That is the whole reason this is not still
// domain.WeatherProvider: a shared cache has to receive (id, query) together to
// key correctly, and Lookup(ctx, q) cannot express that. domain.WeatherProvider
// is untouched and still means "one source" — the four built-in providers
// implement it exactly as before.
type Sources struct {
	mu    sync.RWMutex
	order []string // ids, sorted; the order a nameless lookup walks
	byID  map[string]*entry
	cache *cached
	// defaultFirst is the id WEATHER_PROVIDER asks for, promoted to the front
	// of the walk. Kept as an id rather than reordering `order`, because
	// `order` is what the tool enum is built from and it must stay sorted.
	defaultFirst string
}

type entry struct {
	src  *adapters.Source
	prov domain.WeatherProvider
}

// NewSources builds the set from resolved sources.
//
// Sources whose provider could not be constructed are dropped with their reason
// recorded, not kept as a broken entry: an id in the tool enum that fails every
// call costs the model a round trip and leaves an error in the conversation,
// and docs/specs/transport.md already settled that a capability with nothing
// behind it should not be offered at all.
func NewSources(resolved []*adapters.Source, opts Options) (*Sources, []error) {
	s := &Sources{byID: map[string]*entry{}, cache: newCached(30 * time.Minute)}
	var problems []error
	for _, src := range resolved {
		p, err := buildProvider(src, opts)
		if err != nil {
			problems = append(problems, fmt.Errorf("weather/%s: %w", src.Entry.ID, err))
			continue
		}
		s.byID[src.Entry.ID] = &entry{src: src, prov: p}
		s.order = append(s.order, src.Entry.ID)
	}
	sort.Strings(s.order)
	return s, problems
}

// SetDefault names the source a lookup with no id prefers.
//
// This is what WEATHER_PROVIDER became. The variable was NOT deleted, and that
// was a deliberate call: removing it would have meant editing three branches of
// the installer and .env.example, and would have left every upgraded database
// holding a settings override row for a field that no longer exists — a row the
// console cannot show and cannot delete, warning on every reload. Keeping the
// name and changing what it means costs one sentence of documentation.
func (s *Sources) SetDefault(id string) {
	s.mu.Lock()
	s.defaultFirst = id
	s.mu.Unlock()
}

// Default reports the id a nameless lookup prefers, for tests and diagnostics
// that need to confirm a console change actually reached this object.
func (s *Sources) Default() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaultFirst
}

// IDs is the usable sources, sorted — the enum of the tool's `source`
// parameter.
//
// Sorted, and derived only from configuration and health: no timestamps, no map
// iteration. The tool band renders before the system prompt, and the ephemeral
// cache breakpoint sits on the system block, so a tool band whose bytes differ
// between two rounds rewrites that breakpoint and every layer behind it. This
// function being a pure function of state is what makes the cache reachable.
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

// Describe returns each usable source's id and the text that may be shown to
// the model, in enum order.
func (s *Sources) Describe(locale string) [][2]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := [][2]string{}
	for _, id := range s.order {
		e := s.byID[id]
		if e.src.Usable() {
			out = append(out, [2]string{id, e.src.PromptDescription(locale)})
		}
	}
	return out
}

// Available reports whether any source can be used at all. Callers that build a
// tool band use this to decide whether to offer the tool: a tool that always
// fails burns a round trip and leaves its error in the conversation.
func (s *Sources) Available() bool { return len(s.IDs()) > 0 }

// Lookup queries one source. An empty id means "pick for me".
//
// # The layering, which is load-bearing
//
//	Lookup ─→ cache ─→ provider
//	   └──────────────────┘  health observed HERE, outside the cache
//
// Health must not be updated on a cache hit. A cached forecast proves nothing
// about whether the source still answers, and counting it as a success would
// hold a dead source "up" for the whole 30-minute TTL — precisely the window in
// which an operator is trying to find out what broke.
func (s *Sources) Lookup(ctx context.Context, id string, q domain.WeatherQuery) (*domain.Forecast, error) {
	e, err := s.pick(id)
	if err != nil {
		return nil, err
	}
	key := cacheKey(e.src.Entry.ID, q)
	if fc, ok := s.cache.get(key); ok {
		return fc, nil
	}
	fc, err := e.prov.Lookup(ctx, q)
	e.src.Health.Observe(err, time.Now())
	if err != nil {
		return nil, err
	}
	s.cache.put(key, fc)
	return fc, nil
}

// pick resolves an id, or chooses one.
//
// A named source that is unusable is an ERROR, not a redirect to another
// source. The model asked for that one; answering with a different source's
// data under the same question is the cross-source chain again, and the model
// has no way to notice.
func (s *Sources) pick(id string) (*entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id != "" {
		e, ok := s.byID[id]
		if !ok {
			return nil, fmt.Errorf("no weather source %q (available: %s)", id, strings.Join(s.usableLocked(), ", "))
		}
		if !e.src.Usable() {
			return nil, fmt.Errorf("weather source %q is unavailable", id)
		}
		return e, nil
	}
	if s.defaultFirst != "" {
		if e, ok := s.byID[s.defaultFirst]; ok && e.src.Usable() {
			return e, nil
		}
	}
	for _, oid := range s.order {
		if e := s.byID[oid]; e.src.Usable() {
			return e, nil
		}
	}
	return nil, fmt.Errorf("no weather source is available")
}

func (s *Sources) usableLocked() []string {
	out := []string{}
	for _, id := range s.order {
		if s.byID[id].src.Usable() {
			out = append(out, id)
		}
	}
	return out
}

// AllIDs is every configured id, including disabled and unhealthy ones —
// unlike IDs, which is the tool enum. A reload has to reach the disabled ones
// too: they are exactly the ones an operator is about to turn back on.
func (s *Sources) AllIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.order...)
}

// Views renders every source for the console, including disabled and unhealthy
// ones — the operator's question is usually about one of those.
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

// cacheKey namespaces by the CONFIG ENTRY id, not by the implementation name.
//
// It used to be the implementation's Name() — a hardcoded string like
// "qweather". With one source wrapped that was nearly harmless. With a set, two
// entries both using qweather (different keys, different regions, which is why
// somebody would configure two) would share every cache slot: ask the Beijing
// entry, receive the other one's half-hour-old answer under its name. The fix
// costs one string and removes the whole class.
func cacheKey(id string, q domain.WeatherQuery) string {
	return id + "|" + strings.ToLower(strings.TrimSpace(q.Location)) + "|" +
		strconv.Itoa(q.Days) + "|" + q.Locale
}
