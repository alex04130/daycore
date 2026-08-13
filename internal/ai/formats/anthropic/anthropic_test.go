package anthropic

import (
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
