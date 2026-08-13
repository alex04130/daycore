package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"daycore/internal/ai"
)

// The cache breakpoint cap.
//
// ⚠️ There is no way to check this against the real API from here: the only
// anthropic-format entries in config/models.yaml are a vision model (one system
// turn, cannot reach the cap) and a DeepSeek compatibility endpoint (which
// almost certainly does not enforce the limit). So a request that would be a 400
// against api.anthropic.com passes every check this machine can run.
//
// buildReq is a pure function, which is what makes the property testable at all.

func breakpointsIn(blocks []map[string]any) []int {
	var at []int
	for i, b := range blocks {
		if _, ok := b["cache_control"]; ok {
			at = append(at, i)
		}
	}
	return at
}

func systemsOf(t *testing.T, n int) ai.ChatRequest {
	t.Helper()
	req := ai.ChatRequest{}
	for i := 0; i < n; i++ {
		req.Messages = append(req.Messages, ai.Message{Role: ai.RoleSystem, Content: string(rune('a' + i))})
	}
	req.Messages = append(req.Messages, ai.Message{Role: ai.RoleUser, Content: "hi"})
	return req
}

func blocksFor(t *testing.T, n int) []map[string]any {
	t.Helper()
	p := &provider{cfg: ai.ModelConfig{Model: "m"}}
	out := p.buildReq(systemsOf(t, n), false)
	blocks, ok := out.System.([]map[string]any)
	if !ok {
		t.Fatalf("System is %T, not the block list this test reads", out.System)
	}
	if len(blocks) != n {
		t.Fatalf("got %d system blocks, want all %d — capping breakpoints must not drop CONTENT", len(blocks), n)
	}
	return blocks
}

// ⚠️ A mutation that SURVIVES, recorded rather than hidden: raising
// maxCacheBreakpoints from 4 to 10 leaves every test here green.
//
// That is not a hole in the tests, it is a fact about the code — `mark` holds at
// most two entries (first and last), so the cap is never reached and the
// constant is a belt on top of braces. What the suite really pins is the BRACES:
// that the mark set stays {first, last}. A future change that marks more blocks
// starts consuming the cap, and at that point TestNeverMoreThanFour becomes the
// live guard it currently only looks like.
//
// The constant itself is unverifiable from here — Anthropic's limit lives in
// their documentation, and nothing in this repository can testify to it.

func TestNeverMoreThanFourCacheBreakpoints(t *testing.T) {
	// ⚠️ 50 is not a hypothetical. A client could push `role: "system"` rows into
	// a thread and they arrived here as system turns, so the count was chosen by
	// the caller. Both halves are fixed; this is the half that holds even if the
	// other regresses.
	for _, n := range []int{1, 2, 3, 5, 10, 50} {
		if at := breakpointsIn(blocksFor(t, n)); len(at) > maxCacheBreakpoints {
			t.Errorf("%d system turns → %d breakpoints at %v; Anthropic answers 400 above %d",
				n, len(at), at, maxCacheBreakpoints)
		}
	}
}

func TestTheLastSystemBlockAlwaysCarriesABreakpoint(t *testing.T) {
	// ⚠️ THE assertion, and the one an obvious "keep the first four" gets wrong.
	// A breakpoint caches the prefix UP TO ITSELF, so the last block is the only
	// one whose prefix covers the tools and every system turn. Dropping it raises
	// the cap and makes caching worse — silently, visible only as
	// cache_read_input_tokens drifting down.
	for _, n := range []int{1, 2, 3, 10, 50} {
		blocks := blocksFor(t, n)
		if _, ok := blocks[n-1]["cache_control"]; !ok {
			t.Errorf("%d system turns: the LAST block has no breakpoint, so nothing caches "+
				"the tools or the earlier system turns", n)
		}
	}
}

func TestTheFirstSystemBlockCarriesABreakpointToo(t *testing.T) {
	// The fallback read point: the rolling summary is its own system turn and
	// changes on every compression, so an end-only breakpoint misses every time
	// the window compresses. The first block still covers the L1 boundaries and
	// the persona, which never move.
	for _, n := range []int{2, 3, 10} {
		if _, ok := blocksFor(t, n)[0]["cache_control"]; !ok {
			t.Errorf("%d system turns: the FIRST block has no breakpoint", n)
		}
	}
}

func TestNothingInTheMiddleIsMarked(t *testing.T) {
	// Middle blocks buy a shorter prefix and each costs a 1.25× write of its own.
	blocks := blocksFor(t, 6)
	for i := 1; i < 5; i++ {
		if _, ok := blocks[i]["cache_control"]; ok {
			t.Errorf("block %d of 6 is marked; only the first and last should be", i)
		}
	}
}

func TestASingleSystemBlockIsMarkedExactlyOnce(t *testing.T) {
	// first == last. Marking it twice is not expressible in a map, but a
	// rewrite that appends breakpoints to a list could double-count against the
	// cap — which is the kind of thing that only shows up at the limit.
	if at := breakpointsIn(blocksFor(t, 1)); len(at) != 1 {
		t.Errorf("one system turn → %d breakpoints, want exactly 1", len(at))
	}
}

func TestNoSystemTurnsMeansNoSystemField(t *testing.T) {
	// Anthropic rejects an empty system array; omitting the field is the correct
	// shape and `omitempty` only does that for a nil `any`.
	p := &provider{cfg: ai.ModelConfig{Model: "m"}}
	out := p.buildReq(ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}}, false)
	if out.System != nil {
		t.Errorf("System is %#v with no system turns, want nil", out.System)
	}
}

// Provider-executed tools go on the wire as a type, not as a schema.
//
// ⚠️ Untestable against the real API from here, and more so than the cache cap:
// no anthropic-format entry in the default config is even reachable by the
// companion path (the vision model runs a different pipeline, and chat-search
// points at DeepSeek's compatibility endpoint). buildReq being pure is the only
// reason this is checkable at all.
func TestServerSideToolsSerialiseAsATypeNotASchema(t *testing.T) {
	p := &provider{cfg: ai.ModelConfig{Model: "m"}}
	out := p.buildReq(ai.ChatRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}},
		Tools: []ai.ToolDef{
			{Name: "plan_add", Description: "…", Parameters: map[string]any{"type": "object"}},
			{Name: "web_search", ServerSide: "web_search_20250305", MaxUses: 3},
		},
	}, false)

	if len(out.Tools) != 2 {
		t.Fatalf("got %d tools, want both", len(out.Tools))
	}
	client, server := out.Tools[0], out.Tools[1]

	if client.Type != "" || client.InputSchema == nil {
		t.Errorf("client tool went out as %+v; it needs a schema and no type", client)
	}
	if server.Type != "web_search_20250305" {
		t.Errorf("server tool type = %q, want the declared one", server.Type)
	}
	// ⚠️ A schema on a server tool is a 400: the provider owns the parameters.
	if server.InputSchema != nil {
		t.Errorf("server tool carries an input_schema (%v) — the provider rejects that", server.InputSchema)
	}
	if server.MaxUses != 3 {
		t.Errorf("max_uses = %d, want 3", server.MaxUses)
	}
	if server.Name != "web_search" {
		t.Errorf("server tool name = %q; Anthropic matches this type by name", server.Name)
	}
}

func TestMaxUsesIsOmittedWhenUnset(t *testing.T) {
	// Zero means "the provider's default", and `max_uses: 0` is not that.
	p := &provider{cfg: ai.ModelConfig{Model: "m"}}
	out := p.buildReq(ai.ChatRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}},
		Tools:    []ai.ToolDef{{Name: "web_search", ServerSide: "web_search_20250305"}},
	}, false)
	blob, err := json.Marshal(out.Tools[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), "max_uses") {
		t.Errorf("unset max_uses reached the wire: %s", blob)
	}
}

// A server tool's frames must NOT become client tool calls.
//
// ⚠️ THE assertion of this pair. The provider already ran the tool and the text
// blocks already cite it; surfacing `server_tool_use` as a ToolCall would send
// the agent loop off to execute a tool it does not implement, fail, and report
// that failure back to the model as if the search had gone wrong.
func TestServerToolFramesAreNotClientToolCalls(t *testing.T) {
	body := `{"content":[
		{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"x"}},
		{"type":"web_search_tool_result","tool_use_id":"srvtoolu_1","content":[{"type":"web_search_result","url":"https://e.example","title":"E"}]},
		{"type":"text","text":"根据搜索结果，…"},
		{"type":"tool_use","id":"toolu_2","name":"plan_add","input":{"title":"t"}}
	],"stop_reason":"tool_use"}`

	var mr msgResp
	if err := json.Unmarshal([]byte(body), &mr); err != nil {
		t.Fatal(err)
	}
	// ⚠️ The REAL fold, not a copy of it. The first version of this test walked
	// mr.Content with its own switch — so it asserted a duplicate of the logic
	// and stayed green when the logic itself was mutated to surface
	// server_tool_use as a client tool call.
	var out ai.ChatResponse
	out.Content, out.ToolCalls = foldContent(mr)

	if len(out.ToolCalls) != 1 || out.ToolCalls[0].Name != "plan_add" {
		t.Errorf("tool calls = %+v; only the CLIENT tool may come through", out.ToolCalls)
	}
	if !strings.Contains(out.Content, "根据搜索结果") {
		t.Errorf("the text block was lost: %q", out.Content)
	}
}

func TestRunsServerToolIsAWhitelist(t *testing.T) {
	p := &provider{cfg: ai.ModelConfig{Model: "m"}}
	if !p.RunsServerTool("web_search_20250305") {
		t.Error("the one type this format handles was refused")
	}
	// ⚠️ Each type needs its own serialisation shape AND its own result frames
	// dropped. "We handle web search" does not imply "we handle code execution",
	// and a blanket yes turns the next type into a 400 on every request.
	for _, other := range []string{"code_execution_20250522", "web_search", "", "bash"} {
		if p.RunsServerTool(other) {
			t.Errorf("RunsServerTool(%q) said yes; nothing here handles it", other)
		}
	}
}
