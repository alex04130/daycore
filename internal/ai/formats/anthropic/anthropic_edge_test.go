package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daycore/internal/ai"
)

func anthropicSSE(t *testing.T, lines []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, l := range lines {
			_, _ = w.Write([]byte(l + "\n\n"))
		}
	}))
}

func anthropicChunks(t *testing.T, baseURL string) []ai.Chunk {
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

func TestChatStreamEventDispatch(t *testing.T) {
	srv := anthropicSSE(t, []string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":4}}}`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`,
		`data: {"type":"message_delta","usage":{"output_tokens":3}}`,
		`data: {"type":"message_stop"}`,
	})
	defer srv.Close()
	chunks := anthropicChunks(t, srv.URL)
	if len(chunks) != 3 {
		t.Fatalf("want content + usage + done, got %+v", chunks)
	}
	if chunks[0].ContentDelta != "hello" {
		t.Errorf("chunk 0 = %+v", chunks[0])
	}
	if chunks[1].Usage == nil || chunks[1].Usage.PromptTokens != 10 || chunks[1].Usage.CachedTokens != 4 || chunks[1].Usage.CompletionTokens != 3 {
		t.Errorf("the paired usage must carry both halves, got %+v", chunks[1].Usage)
	}
	if !chunks[2].Done {
		t.Errorf("chunk 2 must be Done, got %+v", chunks[2])
	}
}

// Unknown event types — ping, error — match no case and are ignored; the
// stream continues. Locked behaviour.
func TestChatStreamUnknownAndErrorEventsIgnored(t *testing.T) {
	srv := anthropicSSE(t, []string{
		`data: {"type":"ping"}`,
		`data: {"type":"error","error":{"message":"boom"}}`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"still here"}}`,
		`data: {"type":"message_stop"}`,
	})
	defer srv.Close()
	chunks := anthropicChunks(t, srv.URL)
	if len(chunks) != 2 || chunks[0].ContentDelta != "still here" {
		t.Errorf("unknown events must be skipped without killing the stream, got %+v", chunks)
	}
}

// tool_use arguments stream as input_json_delta frames. The loop only reads
// delta.text, so those are dropped — and a stop_reason is never parsed, so a
// streamed call reports NO FinishReason. Both locked: routing for tool-capable
// models goes through StreamViaChat (see ai.ToolStreamer).
func TestChatStreamDropsToolDeltasAndFinishReason(t *testing.T) {
	srv := anthropicSSE(t, []string{
		`data: {"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"da"}}`,
		`data: {"type":"message_delta","stop_reason":"tool_use"}`,
		`data: {"type":"message_stop"}`,
	})
	defer srv.Close()
	chunks := anthropicChunks(t, srv.URL)
	if len(chunks) != 1 || !chunks[0].Done {
		t.Fatalf("input_json_delta must be dropped, got %+v", chunks)
	}
	var acc ai.StreamAccumulator
	for _, c := range chunks {
		acc.Add(c)
	}
	if acc.Response().FinishReason != "" {
		t.Error("streamed anthropic calls carry no FinishReason (locked)")
	}
}

// Usage pairing edge: a message_delta with no message_start emits a
// prompt-half of zero, and OutputTokens==0 emits nothing at all.
func TestChatStreamUsagePairing(t *testing.T) {
	srv := anthropicSSE(t, []string{
		`data: {"type":"message_delta","usage":{"output_tokens":5}}`,
		`data: {"type":"message_stop"}`,
	})
	defer srv.Close()
	chunks := anthropicChunks(t, srv.URL)
	if len(chunks) != 2 || chunks[0].Usage == nil || chunks[0].Usage.PromptTokens != 0 || chunks[0].Usage.CompletionTokens != 5 {
		t.Errorf("a delta without a start must pair with a zero prompt half, got %+v", chunks)
	}
	srv2 := anthropicSSE(t, []string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":9}}}`,
		`data: {"type":"message_delta","usage":{"output_tokens":0}}`,
		`data: {"type":"message_stop"}`,
	})
	defer srv2.Close()
	chunks = anthropicChunks(t, srv2.URL)
	if len(chunks) != 1 || !chunks[0].Done {
		t.Errorf("zero output tokens must not emit a usage frame, got %+v", chunks)
	}
}

func TestChatStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer srv.Close()
	p, _ := New(ai.ModelConfig{ID: "m", Model: "m", BaseURL: srv.URL, APIKey: "k"})
	if _, err := p.ChatStream(context.Background(), ai.ChatRequest{}); err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("an HTTP error must fail the stream setup, got %v", err)
	}
}

// foldContent: a server_tool_use is deliberately dropped (the provider ran
// it), a web_search_tool_result is dropped, an unknown block type is ignored,
// and a client tool_use becomes a ToolCall.
func TestFoldContentMatrix(t *testing.T) {
	var mr msgResp
	if err := json.Unmarshal([]byte(`{"content":[
		{"type":"text","text":"hi"},
		{"type":"tool_use","id":"t1","name":"plan_add","input":{"date":"x"}},
		{"type":"server_tool_use","id":"s1","name":"web_search"},
		{"type":"web_search_tool_result"},
		{"type":"thinking","text":"hmm"}
	]}`), &mr); err != nil {
		t.Fatal(err)
	}
	text, calls := foldContent(mr)
	if text != "hi" {
		t.Errorf("text = %q", text)
	}
	if len(calls) != 1 || calls[0].Name != "plan_add" || calls[0].Arguments != `{"date":"x"}` {
		t.Errorf("only the client tool_use may surface, got %+v", calls)
	}
}
