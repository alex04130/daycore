package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/ai"
)

// streamChunks returns every chunk as-is (no fatal on Err), so edge cases can
// assert what the stream DID rather than what the happy path wants.
func streamChunks(t *testing.T, baseURL string) []ai.Chunk {
	t.Helper()
	p, err := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: baseURL, APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := p.ChatStream(context.Background(), ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	var out []ai.Chunk
	for c := range stream {
		out = append(out, c)
	}
	return out
}

func TestChatStreamSkipsNonDataLines(t *testing.T) {
	srv := sseServer(t, []string{
		"event: message",
		"id: 42",
		": a comment",
		`data: {"choices":[{"delta":{"content":"hi"}}]}`,
		"data: [DONE]",
	})
	defer srv.Close()
	chunks := streamChunks(t, srv.URL)
	if len(chunks) != 2 || chunks[0].ContentDelta != "hi" || !chunks[1].Done {
		t.Errorf("only data: frames may produce chunks, got %+v", chunks)
	}
}

// A truncated or malformed JSON frame is silently skipped and the stream
// continues — locked behaviour. The alternative (kill the stream) would turn
// one noisy proxy line into a lost answer.
func TestChatStreamTruncatedFrameSilentlySkipped(t *testing.T) {
	srv := sseServer(t, []string{
		"data: {",
		"data: {",
		`data: {"choices":[{"delta":{"content":"still alive"}}]}`,
		"data: [DONE]",
	})
	defer srv.Close()
	chunks := streamChunks(t, srv.URL)
	if len(chunks) != 2 || chunks[0].ContentDelta != "still alive" {
		t.Errorf("truncated frames must be skipped without killing the stream, got %+v", chunks)
	}
}

// An error frame ({"error":…}) carries no choices and no usage, so it is
// skipped; the stream then runs to EOF with no Err and no Done. Locked: the
// agent loop still ends the SSE exchange with done at a higher layer.
func TestChatStreamErrorFrameEmitsNothing(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"error":{"message":"boom"}}`,
	})
	defer srv.Close()
	chunks := streamChunks(t, srv.URL)
	if len(chunks) != 0 {
		t.Errorf("an error frame is currently ignored, got %+v", chunks)
	}
}

// [DONE] is case-sensitive and must have exactly one space.
func TestChatStreamDoneCaseSensitivity(t *testing.T) {
	srv := sseServer(t, []string{"data:[DONE]"})
	defer srv.Close()
	chunks := streamChunks(t, srv.URL)
	if len(chunks) != 1 || !chunks[0].Done {
		t.Errorf("data:[DONE] must terminate the stream, got %+v", chunks)
	}
	srv2 := sseServer(t, []string{"data: [done]"})
	defer srv2.Close()
	chunks = streamChunks(t, srv2.URL)
	if len(chunks) != 0 {
		t.Errorf("lower-case [done] is not recognised and the stream runs to EOF, got %+v", chunks)
	}
}

// The terminal usage frame arrives with empty choices and must still emit a
// Usage chunk — that is the accounting for the busiest path in the product.
func TestChatStreamTerminalUsageChunk(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"content":"x"}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3}}`,
		"data: [DONE]",
	})
	defer srv.Close()
	chunks := streamChunks(t, srv.URL)
	var usage *ai.Usage
	for i := range chunks {
		if chunks[i].Usage != nil {
			usage = chunks[i].Usage
		}
	}
	if usage == nil || usage.PromptTokens != 7 || usage.CompletionTokens != 3 {
		t.Errorf("the usage frame must survive the empty-choices skip, got %+v", usage)
	}
}

// Several tool calls can arrive inside ONE delta frame.
func TestChatStreamMultipleToolCallsInOneDelta(t *testing.T) {
	srv := sseServer(t, []string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"plan_add","arguments":"{}"}},{"index":1,"id":"b","function":{"name":"plan_update","arguments":"{}"}}]}}]}`,
		"data: [DONE]",
	})
	defer srv.Close()
	resp := streamAll(t, srv.URL, ai.ChatRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if len(resp.ToolCalls) != 2 || resp.ToolCalls[0].Name != "plan_add" || resp.ToolCalls[1].Name != "plan_update" {
		t.Errorf("both tool calls in one delta must survive, got %+v", resp.ToolCalls)
	}
}

func TestChatStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"no"}`))
	}))
	defer srv.Close()
	p, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv.URL, APIKey: "k"})
	if _, err := p.ChatStream(context.Background(), ai.ChatRequest{}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("an HTTP error must fail the stream setup, got %v", err)
	}
	if _, err := p.Chat(context.Background(), ai.ChatRequest{}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("an HTTP error must fail Chat, got %v", err)
	}
}

func TestChatErrorBodyAndEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()
	p, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv.URL, APIKey: "k"})
	if _, err := p.Chat(context.Background(), ai.ChatRequest{}); err == nil || !strings.Contains(err.Error(), "bad key") {
		t.Errorf("an error body must surface the message, got %v", err)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv2.Close()
	p2, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv2.URL, APIKey: "k"})
	if _, err := p2.Chat(context.Background(), ai.ChatRequest{}); err == nil || !strings.Contains(err.Error(), "empty choices") {
		t.Errorf("empty choices must be an error, got %v", err)
	}
}

// A single SSE line beyond the scanner's 1 MiB buffer emits an Err chunk —
// it must not be silently dropped as if it were content.
func TestChatStreamOversizeLineEmitsErr(t *testing.T) {
	srv := sseServer(t, []string{"data: " + strings.Repeat("x", 1100*1024)})
	defer srv.Close()
	chunks := streamChunks(t, srv.URL)
	found := false
	for _, c := range chunks {
		if c.Err != nil {
			found = true
		}
	}
	if !found {
		t.Error("an oversize line must emit an Err chunk")
	}
}
