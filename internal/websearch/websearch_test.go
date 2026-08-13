package websearch

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"daycore/internal/adapters"
)

type fakeEngine struct {
	name     string
	results  []Result
	err      error
	searches []int
}

func (f *fakeEngine) Name() string { return f.name }
func (f *fakeEngine) Search(_ context.Context, query string, max int) ([]Result, error) {
	f.searches = append(f.searches, max)
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func testSource(t *testing.T, id, impl string) *adapters.Source {
	t.Helper()
	enabled := true
	src := adapters.Resolve(adapters.KindSearch, adapters.Entry{
		ID: id, Format: adapters.FormatBuiltin, Impl: impl, Enabled: &enabled,
	}, nil, adapters.NewHealth())
	return src
}

func TestNewSourcesDropsBrokenAndValidatesNative(t *testing.T) {
	Register("zz-fake-engine", func(Config) Engine { return &fakeEngine{name: "zz-fake-engine"} })
	srcs := []*adapters.Source{
		testSource(t, "good", "zz-fake-engine"),
		testSource(t, "unknown-impl", "no-such-impl"),
		adapters.Resolve(adapters.KindSearch, adapters.Entry{ID: "nat-no-tool", Format: adapters.FormatNative}, nil, adapters.NewHealth()),
		adapters.Resolve(adapters.KindSearch, adapters.Entry{ID: "nat-ok", Format: adapters.FormatNative, ToolType: "web_search"}, nil, adapters.NewHealth()),
	}
	s, problems := NewSources(srcs, Options{})
	if len(problems) != 2 {
		t.Fatalf("want 2 problems (unknown impl + native without tool_type), got %d: %v", len(problems), problems)
	}
	if s.Source("good") == nil {
		t.Fatal("the good source must be present")
	}
	if s.Source("unknown-impl") != nil {
		t.Fatal("a source whose engine cannot build must be dropped, not kept to fail every call")
	}
	native := s.NativeTools()
	if len(native) != 1 || native[0].ID != "nat-ok" {
		t.Fatalf("native tools = %+v, want just nat-ok", native)
	}
	if len(s.IDs()) != 1 || s.IDs()[0] != "good" {
		t.Errorf("IDs() = %v, want [good]", s.IDs())
	}
}

func TestSearchClampsAndNamesErrors(t *testing.T) {
	Register("zz-fake-engine", func(Config) Engine { return &fakeEngine{name: "zz-fake-engine"} })
	s, _ := NewSources([]*adapters.Source{testSource(t, "good", "zz-fake-engine")}, Options{})
	ctx := context.Background()

	if _, err := s.Search(ctx, "", "   ", 5); err == nil || !strings.Contains(err.Error(), "empty query") {
		t.Errorf("an empty query must be named, got %v", err)
	}
	// The clamp is a budget, not a limitation: every hit becomes tool output
	// paid for on later turns.
	for _, tc := range []struct{ in, want int }{
		{0, 1}, {-3, 1}, {1, 1}, {5, 5}, {7, 5}, {100, 5},
	} {
		s.Search(ctx, "", "q", tc.in)
		e := s.Source("good")
		_ = e
	}
	if _, err := s.Search(ctx, "no-such", "q", 5); err == nil || !strings.Contains(err.Error(), "no-such") {
		t.Errorf("an unknown source must be named, got %v", err)
	}
}

func TestPickUnavailableSourceIsAnErrorNotARedirect(t *testing.T) {
	Register("zz-fake-engine", func(Config) Engine { return &fakeEngine{name: "zz-fake-engine"} })
	s, _ := NewSources([]*adapters.Source{testSource(t, "dead", "zz-fake-engine")}, Options{})
	// Health flips after FlipAfter consecutive disagreeing observations — one
	// failure is noise, three is a verdict. Drive it over the threshold so the
	// "unavailable" path is the one under test.
	for i := 0; i < adapters.FlipAfter; i++ {
		s.Source("dead").Health.Observe(fmt.Errorf("boom"), time.Now())
	}
	// A named source that is unusable is an error: answering a question about
	// one source with another's data is the cross-source chain under a new name.
	if _, err := s.Search(context.Background(), "dead", "q", 5); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("an unavailable source must be reported, got %v", err)
	}
	if len(s.IDs()) != 0 {
		t.Errorf("IDs() must exclude the unhealthy source, got %v", s.IDs())
	}
	if s.Available() {
		t.Error("Available() must be false with no usable source")
	}
}

func TestDefaultEntries(t *testing.T) {
	got := DefaultEntries(Options{})
	if len(got) != 1 || got[0].ID != "duckduckgo" {
		t.Errorf("no key → duckduckgo only, got %+v", got)
	}
	got = DefaultEntries(Options{TavilyKey: "k"})
	if len(got) != 2 || got[0].ID != "duckduckgo" || got[1].ID != "tavily" {
		t.Errorf("with a Tavily key both sources appear in a stable order, got %+v", got)
	}
}

func TestEnginesSorted(t *testing.T) {
	got := Engines()
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("Engines() not sorted: %v", got)
		}
	}
}
