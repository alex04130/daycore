package ai_test

import (
	"context"
	"os"
	"testing"

	"daycore/internal/ai"
	_ "daycore/internal/ai/formats/anthropic"
	_ "daycore/internal/ai/formats/ollama"
	_ "daycore/internal/ai/formats/openai"
)

// Every registered format must be honest about whether its ChatStream carries
// tool calls.
//
// The failure this guards is configuration-shaped, which is why it is not caught
// by any test of a single format: config/models.yaml ships `claude` and
// `chat-search` on the anthropic format with `tools: true`, and pointing
// DEFAULT_CHAT_MODEL at either used to give a companion whose every tool call
// vanished. The model asked to write to the plan, the format dropped the
// tool_use block, and the loop saw a turn with no tool calls and ended — no
// error anywhere.
//
// The fix routes such formats through StreamViaChat. This test asserts the
// declaration matches reality, because the declaration is what the routing reads.
func TestFormatsDeclareToolStreamingHonestly(t *testing.T) {
	// Reality, established by reading each format's stream loop: only openai
	// emits ToolCallDelta. Updating a format to stream tool calls means updating
	// this table in the same change — which is the point.
	want := map[string]bool{
		"openai":    true,
		"anthropic": false, // handles content_block_delta + message_stop only
		"ollama":    false, // reads Message.Content only
	}

	formats := ai.Formats()
	if len(formats) != len(want) {
		t.Fatalf("registered formats %v, table has %d — a new format needs a row here", formats, len(want))
	}
	for _, name := range formats {
		expect, ok := want[name]
		if !ok {
			t.Errorf("format %q is registered but this test does not know whether it streams tool calls", name)
			continue
		}
		p := buildFormat(t, name)
		if got := ai.StreamsToolCalls(p); got != expect {
			t.Errorf("format %q: StreamsToolCalls()=%v, want %v", name, got, expect)
		}
	}
}

// A provider that says nothing must be treated as not streaming tool calls. The
// two failure directions are not symmetric: guessing "no" costs one
// non-incremental round, guessing "yes" loses every tool call silently.
func TestUnknownProviderIsAssumedNotToStreamToolCalls(t *testing.T) {
	if ai.StreamsToolCalls(silentProvider{}) {
		t.Error("a provider that does not implement ToolStreamer was assumed to stream tool calls")
	}
}

type silentProvider struct{}

func (silentProvider) Chat(context.Context, ai.ChatRequest) (*ai.ChatResponse, error) {
	return nil, nil
}
func (silentProvider) ChatStream(context.Context, ai.ChatRequest) (<-chan ai.Chunk, error) {
	return nil, nil
}
func (silentProvider) Capabilities() ai.Capabilities { return ai.Capabilities{} }
func (silentProvider) Model() string                 { return "silent" }

func buildFormat(t *testing.T, name string) ai.AIProvider {
	t.Helper()
	dir := t.TempDir()
	path := dir + "/models.yaml"
	writeModelsYAML(t, path, name)
	cat, err := ai.LoadCatalog(path, "m", "", "")
	if err != nil {
		t.Fatalf("load catalog for format %q: %v", name, err)
	}
	p, ok := cat.Provider("m")
	if !ok {
		t.Fatalf("catalog has no provider for format %q", name)
	}
	return p
}

func writeModelsYAML(t *testing.T, path, format string) {
	t.Helper()
	body := "models:\n  - id: m\n    format: " + format +
		"\n    base_url: http://127.0.0.1:1\n    model: m\n    tools: true\n    stream: true\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
