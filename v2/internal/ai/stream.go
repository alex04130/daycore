package ai

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// StreamAccumulator folds streamed Chunks back into a complete ChatResponse,
// reassembling tool calls that arrive as per-index argument fragments. The zero
// value is ready to use.
type StreamAccumulator struct {
	content      strings.Builder
	calls        map[int]*ToolCall
	finishReason string
}

// Add folds one chunk into the accumulator. Err/Done chunks are ignored — the
// caller decides how to end the stream.
func (a *StreamAccumulator) Add(c Chunk) {
	a.content.WriteString(c.ContentDelta)
	if d := c.ToolCallDelta; d != nil {
		if a.calls == nil {
			a.calls = map[int]*ToolCall{}
		}
		tc := a.calls[d.Index]
		if tc == nil {
			tc = &ToolCall{}
			a.calls[d.Index] = tc
		}
		if d.ID != "" {
			tc.ID = d.ID
		}
		if d.Name != "" {
			tc.Name = d.Name
		}
		tc.Arguments += d.ArgsDelta
	}
	if c.FinishReason != "" {
		a.finishReason = c.FinishReason
	}
}

// Response assembles the accumulated stream. Tool calls come out in Index
// order; a call whose fragments never carried an ID gets a synthesized one.
func (a *StreamAccumulator) Response() *ChatResponse {
	resp := &ChatResponse{Content: a.content.String(), FinishReason: a.finishReason}
	idxs := make([]int, 0, len(a.calls))
	for i := range a.calls {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for _, i := range idxs {
		tc := *a.calls[i]
		if tc.ID == "" {
			tc.ID = fmt.Sprintf("call_%d", i)
		}
		resp.ToolCalls = append(resp.ToolCalls, tc)
	}
	return resp
}

// StreamViaChat wraps a non-streaming Chat call in the Chunk stream shape:
// one content frame, one complete ToolCallDelta per tool call, a FinishReason
// frame, then Done. It lets callers keep a single streaming code path for
// formats whose ChatStream cannot carry tool calls yet (anthropic/ollama).
func StreamViaChat(ctx context.Context, p AIProvider, req ChatRequest) (<-chan Chunk, error) {
	resp, err := p.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make(chan Chunk, len(resp.ToolCalls)+3)
	if resp.Content != "" {
		out <- Chunk{ContentDelta: resp.Content}
	}
	for i, tc := range resp.ToolCalls {
		out <- Chunk{ToolCallDelta: &ToolCallDelta{Index: i, ID: tc.ID, Name: tc.Name, ArgsDelta: tc.Arguments}}
	}
	out <- Chunk{FinishReason: normalizeFinish(resp.FinishReason, len(resp.ToolCalls) > 0)}
	out <- Chunk{Done: true}
	close(out)
	return out, nil
}

// normalizeFinish maps provider-specific stop reasons onto the Chunk contract
// ("stop" | "tool_calls" | "length"); anthropic says tool_use/max_tokens and
// ollama says nothing at all.
func normalizeFinish(reason string, hasToolCalls bool) string {
	switch reason {
	case "tool_calls", "tool_use":
		return "tool_calls"
	case "length", "max_tokens":
		return "length"
	case "":
		if hasToolCalls {
			return "tool_calls"
		}
		return "stop"
	default:
		return "stop"
	}
}
