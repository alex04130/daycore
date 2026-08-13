package server

import (
	"context"
	"testing"

	"daycore/internal/adapters"
	"daycore/internal/ai"
	"daycore/internal/websearch"
)

// The provider-executed search tool is only offered when BOTH halves hold.
//
// ⚠️ providers.yaml says whether the deployment wants one and which type; the
// PROVIDER says whether this wire format can actually carry it. Declaring one
// the format cannot serialise is a 400 on every request in that deployment, so
// the fallback direction is "leave it out" and the client-side web_search that
// already works keeps working.
//
// This exists because the first cut of the resolver had no test at all: a
// mutation removing the capability check survived the whole suite.

// ⚠️ Named apart from agent_test.go's fakeProvider: same package, and one of
// them would have to win.
type stubProvider struct{ runs string }

func (f stubProvider) Model() string                 { return "fake" }
func (f stubProvider) Capabilities() ai.Capabilities { return ai.Capabilities{Tools: true} }
func (f stubProvider) Chat(context.Context, ai.ChatRequest) (*ai.ChatResponse, error) {
	return nil, nil
}
func (f stubProvider) ChatStream(context.Context, ai.ChatRequest) (<-chan ai.Chunk, error) {
	return nil, nil
}

// runsServerTools implements the optional interface; plainProvider does not.
type runsServerTools struct{ stubProvider }

func (r runsServerTools) RunsServerTool(toolType string) bool { return toolType == r.stubProvider.runs }

func sourcesWith(t *testing.T, entries ...adapters.Entry) *websearch.Sources {
	t.Helper()
	var resolved []*adapters.Source
	for i := range entries {
		resolved = append(resolved, &adapters.Source{Kind: adapters.KindSearch, Entry: entries[i]})
	}
	s, _ := websearch.NewSources(resolved, websearch.Options{})
	return s
}

func TestNativeSearchNeedsBothTheConfigAndTheProvider(t *testing.T) {
	native := adapters.Entry{ID: "vendor", Format: adapters.FormatNative, ToolType: "web_search_20250305", MaxUses: 3}

	s := &Server{search: sourcesWith(t, native)}

	// A provider that does not implement the interface at all.
	if got := s.nativeSearchTool(stubProvider{}); got.ServerSide != "" {
		t.Errorf("offered %+v to a provider that never said it could carry one", got)
	}
	// One that implements it but for a DIFFERENT type.
	if got := s.nativeSearchTool(runsServerTools{stubProvider{runs: "code_execution_20250522"}}); got.ServerSide != "" {
		t.Errorf("offered %+v for a type this provider does not run", got)
	}
	// Both halves.
	got := s.nativeSearchTool(runsServerTools{stubProvider{runs: "web_search_20250305"}})
	if got.ServerSide != "web_search_20250305" {
		t.Fatalf("both halves hold and the tool was still withheld: %+v", got)
	}
	if got.MaxUses != 3 {
		t.Errorf("max_uses = %d, want the configured 3", got.MaxUses)
	}
	// ⚠️ The wire name is the provider's, not the operator's entry id —
	// Anthropic matches this tool type by name.
	if got.Name != "web_search" {
		t.Errorf("name = %q, want web_search", got.Name)
	}
}

func TestNoNativeSearchConfiguredMeansNoTool(t *testing.T) {
	s := &Server{search: sourcesWith(t)}
	if got := s.nativeSearchTool(runsServerTools{stubProvider{runs: "web_search_20250305"}}); got.ServerSide != "" {
		t.Errorf("a capable provider conjured a tool nobody configured: %+v", got)
	}
	// And with no search registry at all.
	if got := (&Server{}).nativeSearchTool(runsServerTools{stubProvider{runs: "web_search_20250305"}}); got.ServerSide != "" {
		t.Errorf("no registry, still offered %+v", got)
	}
}

// A native entry with no tool_type declares nothing, and must be reported rather
// than dropped: a deployment would otherwise believe its vendor search is on.
func TestNativeEntryWithoutAToolTypeIsAProblem(t *testing.T) {
	_, problems := websearch.NewSources([]*adapters.Source{{
		Kind: adapters.KindSearch, Entry: adapters.Entry{ID: "vendor", Format: adapters.FormatNative},
	}}, websearch.Options{})
	if len(problems) == 0 {
		t.Error("a native entry with no tool_type was accepted silently")
	}
}

// Native sources are not engines and must never appear as a routable id — the
// `source` enum of the client-side web_search tool is a list of things the
// backend can call, and this is not one of them.
func TestNativeSourcesAreNotRoutableSearchIDs(t *testing.T) {
	s := sourcesWith(t, adapters.Entry{ID: "vendor", Format: adapters.FormatNative, ToolType: "web_search_20250305"})
	for _, id := range s.IDs() {
		if id == "vendor" {
			t.Error("the native source turned up in IDs(); the model would try to route to it")
		}
	}
	if len(s.NativeTools()) != 1 {
		t.Errorf("NativeTools() = %d, want the one declared", len(s.NativeTools()))
	}
}
