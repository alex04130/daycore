package ai

import (
	"context"
	"strings"
	"testing"
)

// ── StreamAccumulator ──────────────────────────────────────────────────────

func TestAccumulatorNegativeAndHugeIndex(t *testing.T) {
	// Locked behaviour: a negative or absurd index produces a call with a
	// synthesised id — never a panic, never an out-of-range write.
	a := StreamAccumulator{}
	a.Add(Chunk{ToolCallDelta: &ToolCallDelta{Index: -1, Name: "ghost", ArgsDelta: "{}"}})
	resp := a.Response()
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call_-1" {
		t.Errorf("negative index must survive as call_-1, got %+v", resp.ToolCalls)
	}
}

func TestAccumulatorDuplicateIndexLastWins(t *testing.T) {
	// Locked behaviour: a second delta for the same index overwrites ID/Name
	// (arguments still append). Protocol violations are tolerated, not crashed
	// on.
	a := StreamAccumulator{}
	a.Add(Chunk{ToolCallDelta: &ToolCallDelta{Index: 0, ID: "call_a", Name: "plan_add", ArgsDelta: `{"date":`}})
	a.Add(Chunk{ToolCallDelta: &ToolCallDelta{Index: 0, ID: "call_b", Name: "plan_update"}})
	resp := a.Response()
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("one index must fold into one call, got %+v", resp.ToolCalls)
	}
	if resp.ToolCalls[0].ID != "call_b" || resp.ToolCalls[0].Name != "plan_update" {
		t.Errorf("the later delta must win the identity fields, got %+v", resp.ToolCalls[0])
	}
	if !strings.Contains(resp.ToolCalls[0].Arguments, `"date":`) {
		t.Errorf("arguments must still append, got %q", resp.ToolCalls[0].Arguments)
	}
}

func TestAccumulatorEmptyDeltaCreatesPhantomCall(t *testing.T) {
	// Locked behaviour: an empty tool_calls entry ({"tool_calls":[{}]}) still
	// materialises as a call with no name. The agent loop is what refuses an
	// unnamed tool, not the accumulator.
	a := StreamAccumulator{}
	a.Add(Chunk{ToolCallDelta: &ToolCallDelta{Index: 0}})
	resp := a.Response()
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "" || resp.ToolCalls[0].ID != "call_0" {
		t.Errorf("an empty delta must materialise as call_0 with no name, got %+v", resp.ToolCalls)
	}
}

func TestAccumulatorDropsReasoning(t *testing.T) {
	// Locked behaviour: ReasoningDelta is not accumulated into the response.
	a := StreamAccumulator{}
	a.Add(Chunk{ReasoningDelta: "thinking out loud"})
	resp := a.Response()
	if resp.Content != "" {
		t.Errorf("reasoning must not leak into Content, got %q", resp.Content)
	}
}

func TestAccumulatorLastFinishAndUsageWin(t *testing.T) {
	a := StreamAccumulator{}
	a.Add(Chunk{FinishReason: "stop", Usage: &Usage{PromptTokens: 1}})
	a.Add(Chunk{FinishReason: "tool_calls", Usage: &Usage{PromptTokens: 2}})
	resp := a.Response()
	if resp.FinishReason != "tool_calls" || resp.Usage.PromptTokens != 2 {
		t.Errorf("the last frame must win both fields, got %q %+v", resp.FinishReason, resp.Usage)
	}
}

func TestNormalizeFinishMatrix(t *testing.T) {
	cases := []struct {
		reason       string
		hasToolCalls bool
		want         string
	}{
		{"tool_calls", false, "tool_calls"},
		{"tool_use", false, "tool_calls"},
		{"length", false, "length"},
		{"max_tokens", false, "length"},
		{"", true, "tool_calls"},
		{"", false, "stop"},
		{"whatever", false, "stop"},
	}
	for _, tc := range cases {
		if got := normalizeFinish(tc.reason, tc.hasToolCalls); got != tc.want {
			t.Errorf("normalizeFinish(%q, %v) = %q, want %q", tc.reason, tc.hasToolCalls, got, tc.want)
		}
	}
}

func TestStreamViaChatEmptyResponse(t *testing.T) {
	// An empty response still produces a finish frame and a done frame —
	// callers see a complete stream, not a hang.
	p := &fakeNilProvider{resp: &ChatResponse{}}
	ch, err := StreamViaChat(context.Background(), p, ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var frames []Chunk
	for c := range ch {
		frames = append(frames, c)
	}
	if len(frames) != 2 || frames[0].FinishReason != "stop" || !frames[1].Done {
		t.Errorf("an empty response must stream finish+done, got %+v", frames)
	}
}

// ── Modalities ─────────────────────────────────────────────────────────────

func TestNewModalitiesDedupesAndSorts(t *testing.T) {
	got := NewModalities(ModalityImage, ModalityText, ModalityAudio, ModalityImage, "")
	if len(got) != 3 {
		t.Fatalf("dedupe/empty-strip failed: %v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("not sorted: %v", got)
		}
	}
	// The order is a byte-stability contract: anything derived from it (tool
	// definitions, prompt capability lines) must not change run to run.
	again := NewModalities(ModalityImage, ModalityText, ModalityAudio)
	for i := range got {
		if got[i] != again[i] {
			t.Errorf("order must be stable: %v vs %v", got, again)
		}
	}
	if !got.Has(ModalityImage) || got.Has(ModalityVideo) {
		t.Errorf("Has is wrong: %v", got)
	}
}

// ── Capabilities ────────────────────────────────────────────────────────────

func TestCapabilitiesAccepts(t *testing.T) {
	// Text is always accepted — a model that cannot read text is not one this
	// repo can drive.
	if !(Capabilities{}).Accepts(ModalityText) {
		t.Error("empty capabilities must accept text")
	}
	// Vision folds into In: declared in models.yaml, folded by LoadCatalog.
	if !(Capabilities{Vision: true}).Accepts(ModalityImage) {
		t.Error("Vision must accept image")
	}
	if !(Capabilities{In: NewModalities(ModalityImage)}).Accepts(ModalityImage) {
		t.Error("an In entry must accept image without Vision")
	}
	if (Capabilities{}).Accepts(ModalityAudio) {
		t.Error("no capabilities must refuse audio")
	}
}

// ── CarriagesFor ───────────────────────────────────────────────────────────

type carrierProvider struct {
	AIProvider
	carries Carriages
}

func (c carrierProvider) Carries(Modality) Carriages { return c.carries }

func TestCarriagesForFallbackAndPassthrough(t *testing.T) {
	// A provider that does not implement PartCarrier is assumed inline-only.
	got := CarriagesFor(fakeNilProvider{}, ModalityImage)
	if len(got) != 1 || got[0] != CarriageInline {
		t.Errorf("a silent provider must be inline-only, got %v", got)
	}
	// An empty declaration means the same.
	got = CarriagesFor(carrierProvider{carries: nil}, ModalityImage)
	if len(got) != 1 || got[0] != CarriageInline {
		t.Errorf("an empty declaration must fall back to inline, got %v", got)
	}
	// A declaration passes through in order.
	got = CarriagesFor(carrierProvider{carries: Carriages{CarriageURL, CarriageFileID}}, ModalityDocument)
	if len(got) != 2 || got[0] != CarriageURL || got[1] != CarriageFileID {
		t.Errorf("declared carriages must pass through in order, got %v", got)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

type fakeNilProvider struct {
	resp *ChatResponse
	err  error
}

func (f fakeNilProvider) Chat(context.Context, ChatRequest) (*ChatResponse, error) {
	return f.resp, f.err
}
func (f fakeNilProvider) ChatStream(context.Context, ChatRequest) (<-chan Chunk, error) {
	return nil, nil
}
func (f fakeNilProvider) Capabilities() Capabilities { return Capabilities{} }
func (f fakeNilProvider) Model() string              { return "fake" }
